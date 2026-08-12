# Calendar UI + CalDAV/ICS — Design Spec (2026-08-12)

## Goal

Replace the Events list tab with a modern, minimal calendar that supports day / multi-day (up to week) / month / year / list views, all-day events, multiple calendars, display + per-event timezones, and color coding that matches the existing warm paper + pine accent UI. Ship local calendars, ICS import/export, and CalDAV two-way sync in the same release.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Approach | **1** — custom React calendar + Go CalDAV/ICS in the PocketBase sidecar |
| Navigation | **Calendar** tab replaces **Events**; list is a mode inside Calendar |
| Timezones | **A** — app display TZ (default system) + per-event IANA TZ; layout/conflicts in display TZ |
| Sync scope | Local calendars + **ICS** + **CalDAV** in one release (not Google/Outlook proprietary APIs) |
| Visual | Sleek, minimal; palette derived from existing tokens (`--bg` `#f3efe6`, `--panel` `#fffaf2`, `--accent` `#0f6e56`, warm neutrals) |

## Non-goals (v1)

- Google Calendar / Microsoft Graph OAuth
- Invitees, RSVP, free-busy of other people
- Full recurrence *editor* UI (import/expand basic `RRULE` when present; editing recurrence series can be “edit this instance” only at first)
- Drag-resize polish beyond click-to-create / click-to-edit if time-boxed — prefer correct data + views first
- Showing todos/deadlines on the calendar grid (todos stay on Todos tab)

---

## 1. Navigation & modes

Top-level tabs: **Mail | Todos | Calendar** (remove standalone Events).

Inside Calendar toolbar:

- View: **Day | Multi-day | Month | Year | List**
- Multi-day: control for **2–7** days (default 7 = week)
- **Today**, prev/next (unit depends on view)
- Display timezone chip (opens picker / “use system”)
- **New event** (existing modal, extended)
- Calendar sidebar or popover: checklist of calendars with color dots; toggle visibility; manage calendars (add local / ICS / CalDAV)

Draft events (`status=draft`) appear muted in grid/list; Approve/Dismiss available from event detail (same semantics as today’s Events list).

---

## 2. Data model

### `calendars` (new collection)

| Field | Type | Notes |
| --- | --- | --- |
| `name` | text | Required |
| `color` | text | Token id or hex from app palette |
| `timezone` | text | Default IANA TZ for new events on this calendar |
| `source` | text | `local` \| `ics` \| `caldav` |
| `is_visible` | bool | UI toggle |
| `is_default` | bool | Target for analysis drafts / quick create |
| `ics_url` | text | Optional subscription/import URL |
| `caldav_url` | text | Calendar home or calendar URL |
| `caldav_username` | text | |
| `caldav_secret` | text | Password/token (stored like mail password; local only) |
| `caldav_calendar_path` | text | Selected calendar href after discover |
| `sync_token` | text | CalDAV sync-token / CTag cache |
| `last_sync_at` | text | RFC3339 |
| `last_error` | text | Last sync error for UI |

Bootstrap: create one local calendar **Personal** (accent color, `is_default=true`) if none exist. Migrate existing `events` rows onto that calendar.

### `events` (extend)

| Field | Type | Notes |
| --- | --- | --- |
| existing | | `title`, `notes`, `source_message`, `created_at`, `status` |
| `calendar` | text | Calendar record id (required after migrate) |
| `all_day` | bool | |
| `timezone` | text | IANA; ignored for layout when `all_day` (date-based) |
| `starts_at` | text | Timed: UTC instant (RFC3339 `Z`). All-day: `YYYY-MM-DD` start date (inclusive) |
| `ends_at` | text | Timed: UTC instant. All-day: `YYYY-MM-DD` end date (**exclusive**, iCal-style) |
| `uid` | text | Stable iCal UID for sync |
| `etag` | text | CalDAV/ICS revision |
| `rdate` / `rrule` / `exdate` | text | Optional; store raw for round-trip; expand for display when present |

### App settings

- `display_timezone` (text, IANA; empty = use system at runtime)

---

## 3. Timezone & conflict layout

- **Storage:** timed events always persist UTC start/end + `timezone` (original zone for editing/display labels).
- **Authoritative TZ math is Go-only.** The renderer must not convert zones with Luxon/Temporal/manual offsets. All display and write conversions go through sidecar APIs so layout, ICS, and CalDAV stay consistent.
- **Display API:** e.g. `GET /api/email/calendar/window?from=&to=&displayTimezone=` returns events already projected for the grid (`displayStart` / `displayEnd` in the display TZ, plus all-day flags and packing hints as needed).
- **Bounds API:** `GET /api/email/calendar/bounds?view=&anchor=&displayTimezone=&days=` returns `from`/`to` instants and civil `fromDate`/`toDate` in the display TZ so the UI never derives week/month edges from UTC.
- **Write API:** `POST/PATCH /api/email/calendar/events` accepts wall times + event TZ (or all-day dates); Go converts to stored UTC + `timezone` before save. Window payloads also include `editStartWall` / `editEndWall` in the **event** timezone for forms.
- **Conflicts:** packing of overlapping timed intervals may run in Go (preferred) or in the UI using only Go-provided display intervals — never re-derived TZ math in JS.
- **All-day:** sit in a dedicated band above the time grid; do not participate in timed column packing.
- **Create/edit:** timezone defaults to selected calendar’s TZ, overrideable; “All day” uses date fields only.

---

## 4. Views (custom React)

Shared chrome: toolbar + optional left calendar rail. Grid uses CSS variables from the design tokens; event chips use calendar `color` at ~soft fill + stronger left edge.

| View | Behavior |
| --- | --- |
| Day | 24h (or working hours optional later) time column; all-day row; timed blocks |
| Multi-day | N columns (2–7); same all-day + timed structure |
| Month | Month matrix; events as color chips / +N overflow; click day → Day view |
| Year | 12 mini-months; density markers; click month → Month view |
| List | Chronological list (replacement for old Events tab), grouped by day; drafts first or tagged |

Interactions:

- Empty slot click → create modal with start/end prefilled
- Event click → detail/edit modal (approve/dismiss if draft)
- Visibility toggles filter all views without deleting data

---

## 5. Color palette (modern, on-scheme)

Fixed set of calendar colors (names → hex), chosen to sit on cream without neon/purple:

| Token | Hex | Role |
| --- | --- | --- |
| `pine` | `#0f6e56` | Default / Personal (matches `--accent`) |
| `sage` | `#5f8f74` | |
| `ink` | `#3d4f5f` | |
| `clay` | `#b46b4d` | |
| `ochre` | `#b0893c` | |
| `rose` | `#a35d6a` | |
| `slate` | `#6b645b` | Matches `--muted` |

UI never uses purple gradients or glow; chips are flat/soft.

---

## 6. ICS

- **Import file:** parse `.ics`, create or target a calendar (`source=ics` or merge into chosen calendar), upsert by `UID`.
- **Import URL:** fetch ICS (via Go sidecar to avoid renderer CORS), same upsert rules; optional refresh button.
- **Export:** download selected calendar (or visible calendars) as `.ics`.
- Library: Go-side parser/generator (e.g. `ics` / `icalendar` ecosystem) in sidecar HTTP APIs under `/api/email/calendar/...`.

---

## 7. CalDAV

- **Add account:** user enters server URL, username, password → discover principal → list calendars → user picks which to sync (each becomes a `calendars` row `source=caldav`).
- **Sync:** pull (report/sync-collection or ctag + query); push local creates/updates/deletes with etag/If-Match where possible.
- **Schedule:** share syncer-style interval or calendar-specific manual **Sync** + periodic tick (can reuse sync interval setting or a dedicated calendar sync loop).
- **Errors:** surface on calendar row / status strip; do not block UI rendering of cached events.
- Runs in Go sidecar (TLS, credentials never in renderer beyond IPC).

---

## 8. Analysis / drafts integration

- When analysis suggests `add_event`, upsert draft onto **default local calendar** (not CalDAV until approved, or allow approved push on next sync — prefer: drafts stay local calendar only; on Approve, remain on that calendar unless user moves).
- Create Event modal: calendar picker, all-day, timezone, start/end.
- Remove standalone `EventList` tab wiring; List mode reuses list UI patterns inside Calendar.

---

## 9. API surface (Go)

Illustrative routes (final names can match existing `/api/email/...` style):

- `GET/POST /api/email/calendar/settings` — display timezone (or fold into existing settings)
- `GET /api/email/calendar/window` — projected events + lanes for a range
- `GET /api/email/calendar/bounds` — view window edges in display TZ
- `POST/PATCH /api/email/calendar/events` — create/update with wall-time normalize
- `POST /api/email/calendar/ics/import` — multipart or URL
- `GET /api/email/calendar/ics/export?calendar=id`
- `POST /api/email/calendar/caldav/discover`
- `POST /api/email/calendar/caldav/sync` — one calendar or all
- Status optional: `GET /api/email/calendar/sync/status`

CRUD for calendars/events remains PocketBase collections for the React client; sync workers mutate the same collections.

---

## 10. Verification

- Day/multi-day/month/year/list render with mixed all-day + timed + multi-TZ events without overlap bugs.
- Display TZ change reflows timed events; event edit TZ preserved.
- Multiple calendars toggle colors/visibility.
- ICS round-trip UID stable; CalDAV sync pulls remote events and pushes a local create.
- Analysis draft appears on default calendar; Approve works from calendar detail.
- No OOM: load events by visible range (date window query), not unbounded getFullList for year view without bounds.

## Risks

- CalDAV server diversity (iCloud, Fastmail, Nextcloud) — support well-known + user-provided URL; document tested servers.
- All-day exclusive end dates confuse users — label end date carefully in UI (“ends on” = last inclusive day).
- Year view performance — query aggregated counts or clamp event fetch per month.
- Scope size — if CalDAV slips, UI + local + ICS must still ship; CalDAV is in-scope but implement behind the same schema so it can land in the same PR series.

## Open implementation notes (non-blocking)

- Renderer treats display strings/instants from Go as opaque for layout; formatting for labels may use `Intl` only on already-localized wall times returned by the API if needed — no zone conversion in JS.
- Go uses `time.LoadLocation` (IANA) for sync, window projection, and write normalization.
- Multi-day “week” starts on locale week start (configurable later; default locale) — week boundaries for fetches are computed in Go given `displayTimezone` + anchor date.
