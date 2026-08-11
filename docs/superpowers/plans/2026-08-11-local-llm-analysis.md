# Local LLM Email Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Asynchronously analyze every downloaded non-trash/spam email with local LM Studio (default `google/gemma-4-e4b`), persist priority/action/reply suggestions, show them in the list/reader, and apply move/event/todo actions.

**Architecture:** A single-flight Go analyzer worker in the PocketBase sidecar enqueues after ingest/body-fill, calls LM Studio’s OpenAI-compatible API one message at a time, writes `message_analysis` rows, and pauses with ~3s `/v1/models` polling when unreachable. React reads analysis from PB; IMAP moves go through a Go API.

**Tech Stack:** Go 1.24 + PocketBase sidecar, LM Studio HTTP API (`127.0.0.1:1234`), React + TypeScript UI, existing Electron IPC/PB proxy.

## Global Constraints

- Default model slug: `google/gemma-4-e4b`
- Default base URL: `http://127.0.0.1:1234`
- One email analyzed at a time; never block IMAP sync
- Skip trash and spam/junk folders
- Pause analyzer when LM unreachable; poll every ~3s indefinitely
- Always resolve to a valid loaded chat model before calling completions
- Suggested reply stored but not shown in reader yet
- Events/todos: scaffold collections + create stub on apply only
- Analyze once to `done` in v1 (no re-analyze)
- Parse/validation failures: skip message after 3 consecutive failures
- Follow existing `mailstore` ensure-collections patterns and syncer route style
- Work on a feature branch (not bare commits dumping unrelated untracked tree unless needed for build)
- Commit after each task

## File map

| Path | Responsibility |
|------|----------------|
| `backend/internal/mailstore/collections.go` | Register `message_analysis`, `app_settings`, `events`, `todos` |
| `backend/internal/analyzer/model.go` | Model list fetch + resolve preferred/Gemma/first chat |
| `backend/internal/analyzer/model_test.go` | Unit tests for resolution |
| `backend/internal/analyzer/parse.go` | Parse/validate LLM JSON result |
| `backend/internal/analyzer/parse_test.go` | Unit tests for parse/enums |
| `backend/internal/analyzer/client.go` | LM Studio chat completions HTTP client |
| `backend/internal/analyzer/queue.go` | Enqueue, worker loop, pause/poll, startup sweep |
| `backend/internal/analyzer/status.go` | Analyzer status snapshot + HTTP GET |
| `backend/internal/analyzer/register.go` | `Register(app)`, start worker, routes |
| `backend/internal/syncer/sync.go` | Call `analyzer.Enqueue` after ingest/body fill; export folder skip helpers if needed |
| `backend/internal/syncer/move.go` | IMAP move message to folder/spam + update PB |
| `backend/cmd/native/main.go` | `analyzer.Register(app)` |
| `shared/types.ts` | Analyzer status + analysis types |
| `src/components/MessageList.tsx` | Priority indicator |
| `src/components/MessageView.tsx` | Analysis panel + Apply |
| `src/components/SettingsScreen.tsx` | LLM model + base URL fields |
| `src/App.tsx` | Load analysis map; wire apply handlers |
| `src/styles.css` | Priority + analysis panel styles |
| `electron/preload.ts` / `main.ts` | Only if new IPC needed; prefer PB + existing `pb:fetch` HTTP proxy to Go routes |

---

### Task 1: PocketBase collections

**Files:**
- Modify: `backend/internal/mailstore/collections.go`
- Test: verify with `go test ./internal/mailstore/...` if tests exist; else `go build ./...` from `backend/`

**Interfaces:**
- Produces: collections `message_analysis`, `app_settings`, `events`, `todos` with fields from the design spec (including `fail_count` on `message_analysis`)

- [ ] **Step 1: Extend `ensureCollections` defs**

Add these collection defs alongside existing ones (same open rules as other collections):

`message_analysis`: `message` (text, required), `status`, `priority`, `suggested_action`, `action_target`, `suggested_reply` (Max 100_000), `model`, `error`, `fail_count` (number), `analyzed_at`

`app_settings`: `llm_model`, `llm_base_url`

`events` / `todos`: `title`, `notes` (Max 20_000), `source_message`, `created_at`

Also add an ensure-migration helper (like `ensureAccountSecurityFields`) that adds any missing fields if collections already exist from a partial create — call it at the end of the ensure chain.

Default settings row is **not** required at schema time; analyzer will upsert defaults on first read.

- [ ] **Step 2: Build**

```bash
cd /Users/jeeth/projects/email/backend && go build ./...
```

Expected: success

- [ ] **Step 3: Commit**

```bash
git add backend/internal/mailstore/collections.go
git commit -m "feat(mailstore): add LLM analysis and scaffold collections"
```

---

### Task 2: Model resolution (TDD)

**Files:**
- Create: `backend/internal/analyzer/model.go`
- Create: `backend/internal/analyzer/model_test.go`

**Interfaces:**
- Produces:
  - `type ModelInfo struct { ID string }`
  - `func IsChatModel(id string) bool` — false if id contains `embed` (case-insensitive) or `text-embedding`
  - `func ResolveModel(preferred string, available []ModelInfo) (string, bool)` — returns chosen id and ok

- [ ] **Step 1: Write failing tests** in `model_test.go`:

```go
package analyzer

import "testing"

func TestResolveModelPreferred(t *testing.T) {
	got, ok := ResolveModel("google/gemma-4-e4b", []ModelInfo{
		{ID: "qwen2.5-0.5b-instruct-mlx"},
		{ID: "google/gemma-4-e4b"},
		{ID: "text-embedding-nomic-embed-text-v1.5"},
	})
	if !ok || got != "google/gemma-4-e4b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelGemmaFallback(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "google/gemma-4-12b"},
		{ID: "text-embedding-nomic-embed-text-v1.5"},
	})
	if !ok || got != "google/gemma-4-12b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelPreferE4BAmongGemma(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "google/gemma-4-12b"},
		{ID: "google/gemma-4-e4b"},
	})
	if !ok || got != "google/gemma-4-e4b" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelFirstChatSkipsEmbedding(t *testing.T) {
	got, ok := ResolveModel("missing", []ModelInfo{
		{ID: "text-embedding-nomic-embed-text-v1.5"},
		{ID: "qwen2.5-0.5b-instruct-mlx"},
	})
	if !ok || got != "qwen2.5-0.5b-instruct-mlx" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveModelEmpty(t *testing.T) {
	_, ok := ResolveModel("google/gemma-4-e4b", nil)
	if ok {
		t.Fatal("expected not ok")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
cd /Users/jeeth/projects/email/backend && go test ./internal/analyzer/ -run ResolveModel -v
```

- [ ] **Step 3: Implement `model.go`**

Logic order: preferred if present and `IsChatModel` → any `google/gemma-4*` preferring id containing `e4b` → first `IsChatModel` → not ok.

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd /Users/jeeth/projects/email/backend && go test ./internal/analyzer/ -run ResolveModel -v
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/analyzer/model.go backend/internal/analyzer/model_test.go
git commit -m "feat(analyzer): resolve LM Studio model with Gemma fallback"
```

---

### Task 3: Parse LLM JSON result (TDD)

**Files:**
- Create: `backend/internal/analyzer/parse.go`
- Create: `backend/internal/analyzer/parse_test.go`

**Interfaces:**
- Produces:
  - `type Priority string` with `high|medium|low`
  - `type SuggestedAction string` with `move_to_folder|move_to_spam|add_event|add_todo`
  - `type Result struct { Priority Priority; SuggestedAction SuggestedAction; ActionTarget string; SuggestedReply string }`
  - `func ParseResult(raw string) (Result, error)` — accepts raw model content; strips optional ```json fences; validates enums

- [ ] **Step 1: Write failing tests** covering valid JSON, fenced JSON, invalid priority, invalid action, missing fields

- [ ] **Step 2: Run — expect FAIL**

```bash
cd /Users/jeeth/projects/email/backend && go test ./internal/analyzer/ -run ParseResult -v
```

- [ ] **Step 3: Implement `parse.go`**

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add backend/internal/analyzer/parse.go backend/internal/analyzer/parse_test.go
git commit -m "feat(analyzer): parse and validate LLM analysis JSON"
```

---

### Task 4: LM Studio HTTP client + settings helpers

**Files:**
- Create: `backend/internal/analyzer/client.go`
- Create: `backend/internal/analyzer/settings.go`

**Interfaces:**
- Produces:
  - `func ListModels(baseURL string) ([]ModelInfo, error)` — GET `{base}/v1/models`
  - `func ChatJSON(baseURL, model, system, user string) (string, error)` — POST `{base}/v1/chat/completions` with `temperature` ~0.2; return assistant message content
  - `func LoadSettings(app core.App) (model, baseURL string, err error)` — read/create `app_settings` row with defaults
  - `func SaveSettings(app core.App, model, baseURL string) error`
  - `func Reachable(baseURL string) bool` — ListModels succeeds

Use `net/http` with ~60s timeout for chat, ~5s for models/reachability.

System prompt must instruct: return ONLY JSON with keys `priority`, `suggested_action`, `action_target`, `suggested_reply`; enums exact as spec; `suggested_reply` null/omit if not appropriate.

- [ ] **Step 1: Implement client + settings**

- [ ] **Step 2: Build**

```bash
cd /Users/jeeth/projects/email/backend && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/analyzer/client.go backend/internal/analyzer/settings.go
git commit -m "feat(analyzer): LM Studio client and app_settings helpers"
```

---

### Task 5: Analyzer queue worker + status + register

**Files:**
- Create: `backend/internal/analyzer/queue.go`
- Create: `backend/internal/analyzer/status.go`
- Create: `backend/internal/analyzer/register.go`
- Modify: `backend/cmd/native/main.go`

**Interfaces:**
- Produces:
  - `func Enqueue(app core.App, messageID string)` — non-blocking; upserts `pending` if not `done`/`skipped`; no-op if message in trash/spam folder
  - `func Start(app core.App)` — startup sweep + worker goroutine
  - `func Register(app core.App)` — OnServe routes + `Start`
  - `GET /api/email/analyzer/status` → `{state, queueDepth, currentMessageId, message, model}`
  - `GET/POST /api/email/analyzer/settings` for model + base URL

**Worker behavior (exact):**
1. Loop forever
2. While `!Reachable(baseURL)` → set status `paused`, sleep 3s, reload settings, continue
3. Resolve model via `ListModels` + `ResolveModel`; if !ok → paused/sleep 3s
4. Find oldest `pending` analysis (sort by `created` ascending); if none → status `idle`, sleep 1s
5. Set `running`; build user prompt from message fields + folder name; call `ChatJSON`
6. On transport error → reset `pending`, status paused path
7. On parse error → `fail_count++`, `error=...`, if `fail_count>=3` then `skipped` else `pending`
8. On success → write result fields, `status=done`, clear error, `fail_count=0`, `analyzed_at=now`, `model=resolved`

**Startup sweep:**
- Reset all `running` → `pending`
- For each message with non-empty body whose folder is not trash/spam and no analysis row (or only missing): create `pending`
- Process in batches (e.g. 100) to avoid loading entire DB at once

**Skip helpers:** use folder `role` or name contains trash/deleted/spam/junk (align with syncer). Prefer exporting `FolderIsExcludedFromAnalysis(name, role string) bool` from analyzer or syncer to avoid import cycles — keep helper in `analyzer/skip.go` duplicating the small string checks if needed.

- [ ] **Step 1: Implement queue/status/register**

- [ ] **Step 2: Wire `analyzer.Register(app)` in `backend/cmd/native/main.go` after `syncer.Register`**

- [ ] **Step 3: Build**

```bash
cd /Users/jeeth/projects/email/backend && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add backend/internal/analyzer backend/cmd/native/main.go
git commit -m "feat(analyzer): single-flight LLM analysis worker and status API"
```

---

### Task 6: Enqueue hooks from syncer

**Files:**
- Modify: `backend/internal/syncer/sync.go` (end of `ingestBuffer`, end of `FetchMessageBody` success path, after body fills in `fillMissingBodies`)

**Interfaces:**
- Consumes: `analyzer.Enqueue(app, messageID)`
- Avoid import cycle: if `analyzer` imports `syncer`, stop — keep skip logic in analyzer; syncer may import analyzer

- [ ] **Step 1: After successful `app.Save(rec)` in `ingestBuffer`, if body non-empty call `analyzer.Enqueue(app, rec.Id)`**

- [ ] **Step 2: Same after `FetchMessageBody` saves a body**

- [ ] **Step 3: After each successfully filled body in `fillMissingBodies`**

- [ ] **Step 4: Build**

```bash
cd /Users/jeeth/projects/email/backend && go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/syncer/sync.go
git commit -m "feat(syncer): enqueue LLM analysis after body ingest"
```

---

### Task 7: IMAP move API for apply suggestion

**Files:**
- Create: `backend/internal/syncer/move.go` (or `analyzer` must not own IMAP — keep in syncer)
- Modify: `backend/internal/syncer/sync.go` Register routes

**Interfaces:**
- Produces: `POST /api/email/messages/{id}/move` body `{ "folderId": "..." }` or `{ "folderName": "..." }` or `{ "toSpam": true }`
- Behavior: IMAP UID MOVE (or COPY+STORE deleted) to target mailbox; update local `messages.folder` to target folder record id; return updated message

Folder match for spam: find folder where name/role matches spam/junk. For `folderName`: case-insensitive equality first, then contains match; error if zero or ambiguous.

- [ ] **Step 1: Implement move helper + route**

- [ ] **Step 2: Build**

```bash
cd /Users/jeeth/projects/email/backend && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add backend/internal/syncer/move.go backend/internal/syncer/sync.go
git commit -m "feat(syncer): move message to folder or spam via IMAP"
```

---

### Task 8: Shared types + UI list/reader/settings/apply

**Files:**
- Modify: `shared/types.ts`
- Modify: `src/App.tsx`
- Modify: `src/components/MessageList.tsx`
- Modify: `src/components/MessageView.tsx`
- Modify: `src/components/SettingsScreen.tsx`
- Modify: `src/styles.css`
- Modify: `electron/preload.ts` / `electron/main.ts` only if a dedicated IPC is cleaner than `pb.send` to custom routes — prefer:

```ts
await pb.send("/api/email/messages/"+id+"/move", { method: "POST", body: { toSpam: true } })
```

(Verify `pb.send` works for custom routes via existing IPC proxy; if not, add thin `window.email.moveMessage` IPC mirroring `fetchMessageBody`.)

**UI requirements:**
- List: show `High`/`Med`/`Low` when analysis done (map high→High, medium→Med, low→Low); load analyses for visible ids via `pb.collection("message_analysis").getList` filter `message ?= id1 || ...` or per-id map refreshed on sync poll
- Reader: analysis panel with priority, action, target, Apply button
- Apply:
  - `move_to_folder` → POST move with folderName=action_target
  - `move_to_spam` → POST move toSpam
  - `add_event` / `add_todo` → `pb.collection("events"|"todos").create({ title: action_target || subject, notes: "", source_message: id, created_at: new Date().toISOString() })`
- Settings: load/save analyzer settings via `/api/email/analyzer/settings`
- Do not render `suggested_reply` in reader
- Styles: use existing CSS variables; priority colors via `--danger`/`--warn`/`--accent` — no new purple theme

- [ ] **Step 1: Implement types + UI wiring**

- [ ] **Step 2: Typecheck**

```bash
cd /Users/jeeth/projects/email && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add shared/types.ts src electron
git commit -m "feat(ui): show LLM priority and apply analysis suggestions"
```

---

### Task 9: Rebuild native backend binary + smoke verify

**Files:**
- Follow existing build script for `assets/email-backend` (check `package.json` scripts / `scripts/`)

- [ ] **Step 1: Rebuild sidecar so Electron picks up analyzer**

```bash
cd /Users/jeeth/projects/email && npm run build:backend
```

(If script name differs, use the repo’s actual backend build command.)

- [ ] **Step 2: Unit tests**

```bash
cd /Users/jeeth/projects/email/backend && go test ./internal/analyzer/ -v
```

Expected: PASS

- [ ] **Step 3: Manual smoke if LM Studio is up**

```bash
curl -s http://127.0.0.1:1234/v1/models | head
curl -s http://127.0.0.1:8090/api/email/analyzer/status
```

(Only if backend is running.)

- [ ] **Step 4: Final commit if build artifacts are tracked**

```bash
git add assets/email-backend
git commit -m "build: refresh email-backend with analyzer"
```

(Skip asset commit if binary is gitignored.)

---

## Spec coverage checklist

| Spec item | Task |
|-----------|------|
| `message_analysis` + settings + events/todos | 1 |
| Model resolve + always valid | 2, 4, 5 |
| Parse priority/action/reply | 3 |
| Async single-flight queue | 5 |
| Pause + 3s poll | 5 |
| Enqueue all mail except trash/spam | 5, 6 |
| Startup sweep / crash recovery | 5 |
| Status API | 5 |
| Settings configurable | 4, 5, 8 |
| List priority + reader panel | 8 |
| Apply move/spam + scaffold event/todo | 7, 8 |
| Reply stored not shown | 3, 5, 8 |
| Tests for resolve/parse | 2, 3 |

## Self-review notes

- No TBD placeholders
- Types consistent: `SuggestedAction` snake_case values match PB + UI
- Syncer owns IMAP move; analyzer owns LLM queue — no cycles
- `fail_count` field included in Task 1
