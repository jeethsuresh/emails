# Event Required Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Require title/start/end on every event; optional attendees; AI extracts times or Apply forces the modal; add list_events_and_todos for dedup.

**Architecture:** Persist optional event times/attendees on `message_analysis`; enforce required fields in calendar upsert; Apply opens CreateEventModal when times missing; analyzer tool lists events/todos including drafts.

**Tech Stack:** Go PocketBase sidecar, React UI, existing analyzer tools/parse.

## Global Constraints

- Never invent AI event times; omit when unknown.
- Attendees are email strings only.
- End must be after start (server rejects otherwise).

---

### Task 1: Schema + parse + prompt + tool

- [x] Add `attendees` on events; `event_starts_at`, `event_ends_at`, `event_attendees` on message_analysis (defs + ensure).
- [x] Extend `Result` / `ParseResult` + tests; update system prompt.
- [x] Add `list_events_and_todos` tool; wire in `runAnalysisTool`.
- [x] Save new analysis fields in queue; drafts only set times when present.

### Task 2: Calendar API + CreateEventModal

- [x] Require title/start/end in `upsertEventRecord`; persist attendees; no silent end default when EndWall empty.
- [x] UI: end required; attendees field; wire write input + WindowEvent.

### Task 3: Apply flow

- [x] `applyAnalysisAction` returns need-details or completes with times; App/MessageView opens modal when needed.
- [x] Remove inventing `defaultEventTimes` on Apply.
- [x] Rebuild backend + typecheck + restart app; commit.
