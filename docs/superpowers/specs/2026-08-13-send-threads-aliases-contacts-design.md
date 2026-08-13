# Send, Threads, Aliases, Contacts & Suggested Replies — Design Spec (2026-08-13)

## Goal

Ship in **one release**: SMTP send, AI suggested replies (alongside other actions), conversation threading, filter-by-receiving-alias, and contacts derived from senders. **All durable logic and indexing live in Go + PocketBase.** The React/Electron UI is display and thin IPC only—no client-side threading, alias parsing, or contact aggregation.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Scope | Combined v1 (send + replies + threads + alias filter + contacts) |
| Authority | **Go sidecar** owns MIME, SMTP, thread assignment, `received_for`, contacts upsert, list/filter APIs |
| Alias detection | `Delivered-To` / `X-Original-To` / `Envelope-To`, then To/Cc matching account domain |
| Threading | Prefer `In-Reply-To` / `References` + Message-ID; fall back to normalized subject + participants |
| Reply From | Default to message `received_for`; user may override in Compose |
| Suggested reply | Stored on `message_analysis.suggested_reply`; Apply opens Compose prefilled via Go (does not replace other actions) |

## Non-goals (v1)

- Multiple SMTP identities / OAuth send
- Server-side search engine beyond PocketBase filters/indexes
- Full CRM (notes, companies)—contacts are email-centric only
- Editing sent messages; recall
- Cross-account unified inbox (single account remains)

---

## 1. Principle: Go + PocketBase as source of truth

| Concern | Owner |
| --- | --- |
| Parse headers, assign `thread_id`, `received_for` | Go on IMAP ingest (+ backfill) |
| Upsert contacts from `from_addr` | Go on ingest |
| List threads / filter by alias / contact timeline | Go HTTP APIs over PocketBase data |
| Build MIME + SMTP send + Sent-folder append | Go |
| Suggested reply text | Analyzer (already); Compose prefills via Go helper |
| UI | Renders lists/detail; calls APIs; does not re-derive threads/aliases/contacts |

If the UI could compute something by scanning messages, **don’t**—persist or serve it from Go instead.

---

## 2. Data model (PocketBase)

### `messages` (extend)

| Field | Type | Notes |
| --- | --- | --- |
| existing | | `message_id`, `from_addr`, `to_addrs`, `subject`, `date`, bodies, `folder`, … |
| `in_reply_to` | text | Raw Message-ID |
| `references` | text | Space/comma-separated Message-IDs (raw header preserved lightly) |
| `thread_id` | text | Stable id for the conversation (see §3) |
| `received_for` | text | Normalized email of **our** alias that received this message |
| `normalized_subject` | text | Lowercased subject with `re:`/`fwd:` stripped (for fallback threading + display grouping) |

Indexes: `(folder, thread_id, date)`, `(received_for, date)`, `(from_addr, date)`, `(thread_id, date)`.

### `threads` (new collection — maintained by Go)

Materialized for list UX (avoid N-message aggregation in the renderer).

| Field | Type | Notes |
| --- | --- | --- |
| `id` | text | Same as `thread_id` on messages |
| `subject` | text | Subject of newest message (or root) |
| `normalized_subject` | text | |
| `snippet` | text | Newest snippet |
| `last_date` | text | RFC3339 |
| `message_count` | number | |
| `participants` | text | Normalized emails, comma-separated |
| `received_for` | text | Alias of the **newest** message (filter still uses message-level match—see §4) |
| `folder` | text | Primary folder for list context (e.g. inbox thread); updates when newest in that folder changes |
| `unread_count` | number | |
| `updated_at` | text | |

Go updates thread rows on every message upsert/delete affecting that thread.

### `contacts` (new)

| Field | Type | Notes |
| --- | --- | --- |
| `email` | text | Normalized unique key |
| `name` | text | Best display name seen |
| `last_message_at` | text | |
| `message_count` | number | Inbound from this address |
| `updated_at` | text | |

Unique index on `email`. Upsert when ingesting inbound mail (`from_addr`).

### `drafts` / outbox (extend existing `drafts`)

| Field | Type | Notes |
| --- | --- | --- |
| existing | | `account`, `to_addrs`, `subject`, `body_text` |
| `from_addr` | text | Send-as alias |
| `cc_addrs` | text | |
| `in_reply_to` | text | |
| `references` | text | |
| `thread_id` | text | |
| `status` | text | `draft` \| `queued` \| `sending` \| `sent` \| `failed` |
| `last_error` | text | |
| `sent_at` | text | |
| `message_id` | text | Assigned at send time |

### `message_analysis` (existing)

Keep `suggested_action` / `action_target` / `suggested_reply`. Suggested reply is **additive**—UI can Apply action and/or Use reply independently.

### Account domain helper

From `accounts.email` (and optional future alias list), Go derives `account_domain` for To/Cc fallback matching when delivery headers are absent.

---

## 3. Threading algorithm (Go)

On message upsert:

1. Parse `Message-ID`, `In-Reply-To`, `References`.
2. Normalize subject → `normalized_subject`.
3. Resolve `thread_id`:
   - If any referenced Message-ID maps to an existing message with `thread_id`, adopt that id (prefer longest References chain / existing root).
   - Else if `In-Reply-To` matches a known `message_id`, adopt that message’s `thread_id`.
   - Else lookup open threads by `normalized_subject` + overlapping participants (from/to) within a time window (e.g. 90 days); if unique hit, adopt.
   - Else `thread_id = hash(message_id)` or new UUID; first message is root.
4. Upsert `threads` row: bump `last_date`, `message_count`, `snippet`, `participants`, `unread_count`, `received_for` from newest.

Backfill: one-shot migrator on serve for existing rows missing `thread_id` / `received_for`.

---

## 4. Alias / `received_for` (Go)

On ingest, set `received_for` (normalized lowercase email):

1. First matching header among: `Delivered-To`, `X-Original-To`, `X-Delivered-To`, `Envelope-To`.
2. Else first To/Cc address whose domain equals the account email domain (or equals the account local-part@domain).
3. Else empty (UI shows under “Unknown” / unfiltered only).

**Alias list API:** `GET /api/email/aliases` → distinct non-empty `received_for` values (and counts), computed in Go (SQL `GROUP BY`), not in the renderer.

**Thread filter:** `GET /api/email/threads?received_for=uber@jeeth.dev` returns threads that have **at least one** message with that `received_for` (Go query), not merely `threads.received_for == alias`.

---

## 5. Contacts (Go)

On inbound upsert: normalize `from_addr` → upsert `contacts` (`message_count++`, `last_message_at`, refresh `name` if present).

APIs:

- `GET /api/email/contacts?q=` — sorted by `last_message_at`
- `GET /api/email/contacts/{email}/messages` — messages where `from_addr` matches (paginated)

UI: Contacts rail or Mail sub-view; selecting a contact shows that message list (display only).

---

## 6. Send (Go)

`POST /api/email/send` body:

```json
{
  "draftId": optional,
  "from": "uber@jeeth.dev",
  "to": ["a@x.com"],
  "cc": [],
  "subject": "...",
  "bodyText": "...",
  "inReplyTo": optional,
  "references": optional,
  "threadId": optional
}
```

Go:

1. Validate account SMTP settings; build RFC822 (Message-ID, Date, From, To, In-Reply-To, References).
2. Send via SMTP (reuse TLS modes from account: none/starttls/tls).
3. Mark draft `sent` or create sent record; best-effort APPEND to IMAP Sent.
4. Upsert a local `messages` row in Sent (and thread update) so UI stays consistent offline-first.

`POST /api/email/compose/reply?messageId=` returns JSON prefills: `from`=`received_for`, `to`=counterpart, `subject`, `bodyText` (quoted), `inReplyTo`, `references`, `threadId`. Optional `?useSuggestedReply=1` seeds body from analysis.

Compose UI: Save draft (PB) and **Send** (API). No SMTP in the renderer.

---

## 7. Suggested replies (analyzer + UI)

- Prompt/schema already allow `suggested_reply` alongside `suggested_action`.
- Ensure worker always persists reply text when model returns it (null OK).
- Message detail: show suggested reply block + **Use reply** → compose reply API with suggested text; existing Apply for folder/todo/event unchanged.

---

## 8. UI surfaces (display only)

| Surface | Behavior |
| --- | --- |
| Mail list | Switch to **thread list** from Go threads API; open thread → messages in thread ordered by date |
| Alias filter | Chip/list from `/aliases`; selecting filters threads API |
| Contacts | List + “messages from contact” via contacts APIs |
| Compose | From selector (aliases + account); Send calls Go |
| Reading pane | Suggested reply + Reply/Forward actions calling compose helpers |

Progressive layout (bottom tabs / mail stack) unchanged.

---

## 9. API summary

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/email/threads` | `folder`, `received_for`, `q`, pagination |
| GET | `/api/email/threads/{id}` | Thread + messages |
| GET | `/api/email/aliases` | Distinct receiving aliases + counts |
| GET | `/api/email/contacts` | Contact list |
| GET | `/api/email/contacts/{email}/messages` | Timeline |
| POST | `/api/email/compose/reply` | Prefill payload |
| POST | `/api/email/send` | SMTP send + persist |
| POST | `/api/email/drafts` | Optional upsert helper (or keep PB CRUD for drafts only) |

---

## 10. Verification

- Send mail via SMTP; appears in Sent / thread updates.
- Reply uses correct `received_for` as From by default.
- Suggested reply prefills Compose without clearing other analysis actions.
- Two aliases produce two filter chips; filtering hides threads with no matching messages.
- Contact page lists only that sender’s mail.
- Resync/backfill fills `thread_id` / `received_for` / contacts for old messages.
- Renderer performs no thread/alias/contact aggregation logic.

## Risks

- Catch-all hosts omitting delivery headers → weaker `received_for` (document; domain To/Cc fallback).
- Subject-fallback threading false merges — keep window + participant overlap tight.
- SMTP + IMAP Sent APPEND failures — persist sent locally and surface `last_error`.
- Large backfill — run async with status; don’t block serve.

## Open notes

- Thread “folder” for multi-folder conversations: v1 lists threads by newest message’s folder filter; cross-folder continuity via `thread_id` when opening a thread.
- HTML compose later; v1 send is text (+ optional simple multipart later).
