# Calendar + CalDAV/ICS Implementation Plan

> Spec: `docs/superpowers/specs/2026-08-12-calendar-caldav-ics-design.md`

**Goal:** Replace Events with Calendar (day/multi-day/month/year/list), local calendars, ICS, CalDAV.

**Timezone:** All TZ math is Go-only (`/api/email/calendar/window`, `/bounds`, event write APIs). Renderer never converts zones.

## Tasks

1. ~~Schema + bootstrap Personal calendar + migrate events~~
2. ~~Go `internal/calendar` — window/bounds/write + ICS + CalDAV pull/push; register in main; sync loop~~
3. ~~Shared types + analyzer draft → default calendar~~
4. ~~React Calendar shell + views + styles; App tab swap~~
5. ~~Create/edit event modal + calendar manager (local/ICS/CalDAV)~~
6. ~~display_timezone + System chip; typecheck + build + restart~~

## Done notes

- Views: day / multi-day (2–7) / month / year / list (grouped)
- Manager: local create, ICS file/URL import + export, CalDAV discover/subscribe/sync
- Drafts: approve/dismiss from modal + list
- RRULE: basic DAILY/WEEKLY(/MONTHLY) expansion in window API
