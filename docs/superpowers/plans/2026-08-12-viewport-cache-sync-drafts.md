# Viewport cache, sync interval, newest-first AI, drafts — Implementation Plan

> **For agentic workers:** Execute task-by-task. Spec: `docs/superpowers/specs/2026-08-12-viewport-cache-sync-drafts-design.md`

**Goal:** Page-cached infinite scroll mail list; configurable sync interval (default 5m); AI pending newest-first; draft/approved todos & events.

**Tech:** React+Vite UI, Go PocketBase sidecar, existing `app_settings` / analyzer / syncer.

## File map

| Area | Files |
| --- | --- |
| Mail cache | `src/App.tsx`, `src/components/MessageList.tsx`, maybe `src/lib/messageCache.ts` |
| Settings | `backend/internal/mailstore/collections.go`, analyzer settings handlers, `shared/types.ts`, `src/lib/analysis.ts`, `SettingsScreen.tsx`, `syncer` ticker |
| AI order | `backend/internal/analyzer/queue.go` (+ sweep SQL) |
| Drafts | mailstore schema, analyzer after-save upsert, `analysis.ts` Apply, `TodoList.tsx`, `EventList.tsx` |

## Tasks

### Task 1: Message page cache
- Replace `getFullList` with page cache (75/page, keep ±1 page)
- MessageList reports visible index range; App ensures pages loaded
- Add messages index if sort is slow

### Task 2: Sync interval
- `sync_interval_minutes` on `app_settings`
- Extend settings GET/POST; Settings UI field
- Syncer ticker uses setting (default 5, clamp 1–60), hot-reload

### Task 3: Newest-first AI
- `oldestPending` → newest by message date/uid
- Sweep prefers newer messages

### Task 4: Draft todos/events
- `status` field draft|approved; migrate existing → approved
- On analysis done add_*: upsert draft
- UI Approve/Dismiss; Apply approves existing draft

### Task 5: Verify + commit
- `tsc --noEmit`; commit with clear message
