# Local LLM email analysis — Design Spec (2026-08-11)

## Goal

For every successfully downloaded email (except trash/spam), asynchronously analyze it with a local LM Studio model and persist structured suggestions the UI can show and act on.

## Decisions (locked)

- Default model: `google/gemma-4-e4b` (configurable; always resolve to a valid loaded model)
- Analyze **all** eligible mail (backlog + new), one at a time, never blocking IMAP sync
- Skip trash and spam/junk folders
- UI: priority in message list + full analysis in reader
- Suggested actions are actionable: move folder/spam implemented; events/todos scaffolded only
- Suggested reply stored for a future Reply button (not shown in reader yet)
- When LM Studio is unreachable: pause the analyzer and poll reachability every ~3s indefinitely
- Architecture: Go sidecar analyzer queue (Approach 1)

## Architecture

```
IMAP sync (Go) --ingest--> PocketBase messages
                 |
                 +--enqueue--> analyzer worker (Go, single-flight)
                                   |
                                   | HTTP OpenAI-compatible API
                                   v
                             LM Studio :1234
                                   |
                                   v
                          message_analysis (PB)
                                   |
                                   v
                          React list + reader
```

- Package: `backend/internal/analyzer`
- Started with the native PocketBase sidecar
- Enqueue is non-blocking; worker processes exactly one email at a time
- Renderer continues to talk only through PocketBase (+ existing IPC for account/sync); action apply uses small Go HTTP/IPC endpoints where IMAP move is required

## Data model

### `message_analysis` (1:1 with `messages`)

| Field | Type | Notes |
|-------|------|-------|
| `message` | text, required, unique | PocketBase message id |
| `status` | text | `pending` \| `running` \| `done` \| `skipped` |
| `priority` | text | `high` \| `medium` \| `low` (when done) |
| `suggested_action` | text | `move_to_folder` \| `move_to_spam` \| `add_event` \| `add_todo` |
| `action_target` | text | Folder name for moves; title/notes hint for event/todo |
| `suggested_reply` | text | Optional; stored for future Reply UI |
| `model` | text | Model id actually used |
| `error` | text | Last error; cleared on success |
| `fail_count` | number | Consecutive parse/validation failures; reset on success |
| `analyzed_at` | text | RFC3339 when done |

Trash/spam: do not enqueue (or mark `skipped` if discovered later). Do not re-analyze `done` rows in v1 unless we later key off `content_hash` changes.

### `app_settings` (single logical row)

| Field | Default | Notes |
|-------|---------|-------|
| `llm_model` | `google/gemma-4-e4b` | Preferred model slug |
| `llm_base_url` | `http://127.0.0.1:1234` | LM Studio base URL |

### Scaffold collections

- `events`: `title`, `notes`, `source_message`, `created_at`
- `todos`: `title`, `notes`, `source_message`, `created_at`

Enough to insert a stub when the user applies those suggestions. No calendar/todo UI beyond confirmation.

## Model resolution

On each analysis attempt and while paused:

1. `GET {llm_base_url}/v1/models`
2. If configured `llm_model` is in the list and is a chat model → use it
3. Else prefer any id matching `google/gemma-4*` (prefer `e4b` if present)
4. Else first non-embedding chat model
5. If no usable model → remain paused and keep polling

Never send a completion request with an invalid/missing model id. Skip embedding-only models (e.g. `text-embedding-*`).

## Analyzer queue & worker

### Enqueue hooks

- After successful `ingestBuffer` when `body_text` or `body_html` is non-empty
- After on-demand / repair body fill paths (`FetchMessageBody`, `fillMissingBodies`)
- Startup sweep: reset any `running` rows to `pending` (crash recovery); enqueue any eligible message with a body and no `done`/`skipped` analysis row as `pending`

Skip when the message’s folder role/name matches trash or spam/junk (reuse existing syncer helpers).

### Worker loop

1. If LM Studio unreachable → state `paused`; poll `/v1/models` every ~3s indefinitely; resume when reachable
2. Resolve model per rules above
3. Take oldest `pending` analysis; set `running`
4. Build prompt from subject, from, to, date, folder, snippet, full body (prefer `body_text`; strip/truncate HTML if needed for context limits)
5. `POST /v1/chat/completions` asking for strict JSON:
   ```json
   {
     "priority": "high|medium|low",
     "suggested_action": "move_to_folder|move_to_spam|add_event|add_todo",
     "action_target": "optional string",
     "suggested_reply": "optional string or null"
   }
   ```
6. Validate enums; save fields; `status=done`; set `analyzed_at` and `model`
7. On transport/connectivity failure: reset row to `pending`, enter `paused`, resume polling
8. On JSON/parse/validation failure: leave `pending`, record `error`, increment a per-message fail count (stored in `error` or a small `fail_count` number field). After **3** consecutive failures on the same message → `skipped` with `error` so one bad message cannot stall the queue forever

IMAP sync must never wait on the worker.

### Analyzer status

Expose lightweight status (`idle` \| `running` \| `paused`), queue depth, and optional current message id — same spirit as sync status — for debugging. List priority badges do not depend on this.

## UI

### Message list

- Batch-load `message_analysis` for visible message ids
- When `status=done`, show a compact High / Med / Low priority indicator on the row
- Do not show pending/running chrome (keep the list calm)

### Reader

When analysis is `done`, show a panel under the header:

- Priority
- Suggested action + target
- **Apply suggestion** button

Do **not** display `suggested_reply` yet; persist it for a future Reply affordance that pre-fills the draft.

### Settings

Add LLM fields to Settings (alongside account connection):

- Model slug (default `google/gemma-4-e4b`)
- Base URL (default `http://127.0.0.1:1234`)

Persist to `app_settings`. Next analysis uses the new preference via model resolution.

### Apply suggestion

| Action | Behavior |
|--------|----------|
| `move_to_folder` | Match `action_target` to an existing folder (name/fuzzy). Perform IMAP move + update local `messages.folder`. If no match, surface an error (folder picker can come later). |
| `move_to_spam` | Move to the account’s spam/junk folder. |
| `add_event` | Insert scaffold `events` row; confirm to the user. |
| `add_todo` | Insert scaffold `todos` row; confirm to the user. |

Moves require a Go endpoint (IMAP + PB update). Event/todo stubs can be PB creates from the renderer or a thin API.

## Out of scope

- Reply / Reply-All UI and sending
- Full calendar or todo product surfaces
- Auto-applying suggestions without a user click
- Re-analysis on every body edit/`content_hash` change (v1: analyze once to `done`)
- Multi-model routing or cloud LLM providers

## Testing

- Unit: model resolution (preferred, Gemma fallback, first chat, skip embeddings, empty list → paused)
- Unit: completion JSON parse + enum validation
- Manual: LM Studio up — backlog drains one-at-a-time; list priorities appear; reader apply move + event/todo stubs
- Manual: stop LM Studio mid-queue — analyzer pauses, polls, resumes without dropping pending work
- Manual: trash/spam messages never analyzed

## Implementation notes

- Follow existing PocketBase collection registration / field-ensure patterns in `mailstore`
- Hook enqueue at the end of ingest/body-fill without holding the IMAP fetch loop
- Prefer structured prompt + validation over trusting free-form model prose
- Keep analysis payload offline-first in PB so the UI works when LM Studio is later offline (already-analyzed mail remains visible)
