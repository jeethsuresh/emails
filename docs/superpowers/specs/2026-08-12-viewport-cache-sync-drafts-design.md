# Viewport mail cache, sync interval, newest-first AI, draft todos/events — Design Spec (2026-08-12)

## Goal

Lower renderer memory for large mailboxes while keeping infinite-scroll UX; make IMAP sync interval configurable (default 5 minutes); process LLM analysis newest-first; and introduce draft todos/events that auto-appear from analysis and can be approved or dismissed in the UI.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Mail memory | **A** — viewport page cache + infinite scroll (not full-folder `getFullList`) |
| Sync interval | **C** — configurable in Settings; default **5** minutes; clamp 1–60 |
| AI queue order | Newest message first (`date` DESC, then `uid` DESC) |
| Drafts | **C** — auto-create drafts when analysis suggests `add_todo` / `add_event`; Apply approves/focuses existing draft |

Approach: **Option 1** (lean list + backend drafts + settings interval).

## Non-goals

- Calendar UI
- Changing move-to-folder/spam Apply behavior beyond current IMAP move
- Showing `suggested_reply` in the reader
- Replacing PocketBase as source of truth
- Server-side SQL sort indexes as a hard dependency for this pass (list pages use newest-first via client merge of fetched pages or a cheap query strategy documented below)

---

## 1. Mail viewport page cache

### Problem

Holding ~4k light message rows in React state is unnecessary when the DOM is already virtualized. Full-folder `getFullList` also burns time and peak memory on folder switch.

### Behavior

- Message list remains a **virtualized** infinite scroll over the **full** folder (or search result) count.
- Renderer keeps only a **sparse page cache** around the viewport:
  - Page size: **75** messages (tunable constant).
  - Retain about **3 pages** centered on the visible range (current page ± 1); evict pages outside that window.
- On folder change / search change: clear cache, reset scroll, fetch page 1 (and total count).
- Scrolling into an uncached range: show lightweight loading placeholders for missing rows; fetch that page; insert into cache.
- Flag/seen toggles update the cached row in place (and PB); do not reload the whole folder.
- Message bodies stay **on-demand** (existing `getOne` / fetch-body path).
- Analysis fetch stays **visible-ids only** (existing behavior).

### Data loading

- Use PocketBase `getList(page, pageSize, { filter, fields: LIST_FIELDS })` — **no** `getFullList` for the mail list.
- **Sort:** Prefer `sort: "-date,-uid"` once acceptable; if page latency remains high on large folders, fetch unsorted pages is **not** correct for infinite scroll (offsets would be unstable). Mitigation for this pass:
  1. Primary: `sort: "-date,-uid"` with `fields` always applied via `pb.send` query normalization (already required).
  2. If sort stays too slow: add a SQLite index on `messages(folder, date, uid)` in the Go mailstore bootstrap (allowed as part of this work if needed for usable scroll).
- `totalItems` from the first list response drives virtual list height / “N messages” header.
- Search uses the same windowed loader with the existing multi-field filter.

### IPC / PB client

- Keep `normalizeUnknownQueryParams` in `pb.send` so `filter` / `fields` / `sort` always reach the API (critical; missing this OOMs via full bodies).

### Out of scope for list cache

- Persisting the page cache across app restarts.
- Prefetching an entire folder in a background worker.

---

## 2. Configurable sync interval

### Current state

Go syncer ticker uses a hard-coded **60s** interval (`backend/internal/syncer`).

### Behavior

- Store `sync_interval_minutes` on `app_settings` (number, integer).
- Default when unset/invalid: **5**.
- Clamp on read/write: **1–60**.
- Settings UI: numeric field under connection/LLM settings (“Sync every N minutes”).
- Extend existing settings GET/POST (`/api/email/analyzer/settings` or a renamed `/api/email/settings`) to include `syncIntervalMinutes` alongside `model` / `baseUrl`. Prefer **one settings payload** to avoid a second round-trip.
- Sync loop: read interval from settings each tick (or on settings save via wake channel) so changes apply **without app restart**.
- Manual **Sync** button unchanged (immediate `Trigger`).

### Notes

- Ticker should reschedule when the interval changes (reset timer), not only sleep the old period.

---

## 3. AI analysis queue: newest first

### Current state

Worker picks pending via `status = 'pending'` ordered by `+created` (oldest analysis row first).

### Behavior

- Select the next pending `message_analysis` row ordered by the linked message’s **`date` DESC**, then **`uid` DESC** (newest mail first).
- Implementation: SQL join (or two-step query) in the analyzer package; avoid loading all pending ids into memory.
- Backlog **sweep** still creates missing pending rows for eligible messages; sweep batch order should also prefer newer messages when inserting backlog so the queue fills newest-first.
- Status API `queueDepth` unchanged (count of pending).

### Non-goals

- Priority-weighted skipping within the newest-first order.
- Parallel LLM workers.

---

## 4. Draft todos / events

### Schema

Add to both `todos` and `events`:

| Field | Type | Values / notes |
| --- | --- | --- |
| `status` | text | `draft` \| `approved` (default existing rows → `approved` on migrate) |

Optional later (not required now): `dismissed`. Dismiss = **delete** the draft record for this pass.

Ensure fields via existing mailstore `ensure*` migration pattern.

Idempotency: at most **one draft** per `(source_message, collection)` for auto-created suggestions. Unique index optional; enforce in write path (find existing draft for `source_message`, update title/times if present).

### When drafts are created

When `message_analysis` reaches `status=done` and `suggested_action` is `add_todo` or `add_event`:

- Upsert a **draft** todo/event:
  - `title` ← `action_target` or message subject fallback
  - `source_message` ← message id
  - `notes` ← `""` (or short empty)
  - `created_at` ← now
  - times: `deadline` / `starts_at` / `ends_at` from analysis if/when available; else `""`
  - `status` ← `draft`
- Do **not** create drafts for move actions.
- If an **approved** row already exists for that `source_message`, do not create another draft (leave approved alone).

### Reader Apply

| Situation | Apply behavior |
| --- | --- |
| Draft exists for this message + action | Set `status=approved`; optionally navigate focus is UI-only (Todos/Events tab already available) |
| No draft | Create record as `approved` directly (same fields as today’s create) |
| Move actions | Unchanged (IMAP move API) |

### Todos / Events screens

- List **drafts first** (visually distinct: e.g. “Draft” label + Approve / Dismiss actions).
- Then **approved** items (current sorting rules: todos by deadline, events by start).
- **Approve** → `status=approved`.
- **Dismiss** → delete draft record.
- Poll/refresh on tab focus (existing pattern) so analyzer-created drafts appear without restart.

### Types

Extend shared `TodoItem` / `EventItem` with `status: "draft" | "approved"`.

---

## 5. Settings API shape (combined)

```ts
interface AppSettings {
  model: string;
  baseUrl: string;
  syncIntervalMinutes: number; // 1–60, default 5
}
```

GET/POST `/api/email/analyzer/settings` (keep path for compatibility) returns/accepts the extended object. UI labels: “Local LLM” + “Sync every (minutes)”.

---

## 6. Verification

- Inbox with ~4k messages: renderer stays stable (no OOM); scroll loads pages; header shows full total.
- Change sync interval in Settings to 5 (or other); confirm ticker cadence without restart (log or status timing).
- Pending analysis processes a newly dated message before older backlog.
- Completing analysis with `add_todo` / `add_event` creates a draft visible on the matching tab; Approve and Dismiss work; Apply on reader approves existing draft without duplicate.

## Risks

- Sorted `getList` latency on large folders — mitigate with index if needed.
- Draft spam if model over-triggers `add_*` — mitigated by one-draft-per-source_message upsert.
- Settings path name still says `analyzer` while holding sync interval — acceptable for this pass; rename later if desired.
