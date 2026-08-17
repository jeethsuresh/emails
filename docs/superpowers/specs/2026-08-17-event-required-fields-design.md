# Event required fields & AI Apply design

**Date:** 2026-08-17  
**Status:** Approved

## Summary

Events always require a non-empty title, start time, and end time. Attendees are optional (email list). AI may extract times when present; if missing, Apply opens the event form so the user must enter them. The analyzer can list existing events/todos (including drafts) to avoid duplicates.

## Requirements

1. **Create/update (all paths):** title + start + end required; end after start; attendees optional emails.
2. **AI `add_event`:** optional `event_starts_at` / `event_ends_at` (RFC3339) and `attendees`; never invent times.
3. **Apply:** if times present → save approved event; if missing → open CreateEventModal prefilled; save completes Apply.
4. **Tool:** `list_events_and_todos` for dedup against drafts and approved items.
5. **Storage:** `events.attendees` (JSON text); analysis fields `event_starts_at`, `event_ends_at`, `event_attendees`.

## Non-goals

CalDAV invite delivery, RSVP, free-text attendee names (emails only).
