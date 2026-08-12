import type PocketBase from "pocketbase";

export const CALENDAR_COLORS = [
  { id: "pine", hex: "#0f6e56" },
  { id: "sage", hex: "#5f8f74" },
  { id: "ink", hex: "#3d4f5f" },
  { id: "clay", hex: "#b46b4d" },
  { id: "ochre", hex: "#b0893c" },
  { id: "rose", hex: "#a35d6a" },
  { id: "slate", hex: "#6b645b" },
] as const;

export type CalendarViewMode = "day" | "multi" | "month" | "year" | "list";

export interface CalendarRecord {
  id: string;
  name: string;
  color: string;
  timezone: string;
  source: string;
  is_visible: boolean;
  is_default: boolean;
  ics_url?: string;
  caldav_url?: string;
  last_sync_at?: string;
  last_error?: string;
}

export interface WindowEvent {
  id: string;
  title: string;
  notes: string;
  sourceMessage: string;
  status: string;
  calendarId: string;
  calendarName: string;
  calendarColor: string;
  allDay: boolean;
  timezone: string;
  startsAt: string;
  endsAt: string;
  displayStart: string;
  displayEnd: string;
  displayDay: string;
  editStartWall: string;
  editEndWall: string;
  lane: number;
  laneCount: number;
  uid: string;
}

export interface WindowResponse {
  displayTimezone: string;
  from: string;
  to: string;
  events: WindowEvent[];
}

export interface BoundsResponse {
  displayTimezone: string;
  view: string;
  anchor: string;
  from: string;
  to: string;
  fromDate: string;
  toDate: string;
  days?: number;
}

export interface EventWriteInput {
  title: string;
  notes?: string;
  calendarId?: string;
  allDay?: boolean;
  timezone?: string;
  /** Wall clock in event timezone, or YYYY-MM-DD for all-day. */
  startWall: string;
  endWall: string;
  status?: string;
  sourceMessage?: string;
}

export async function fetchCalendarBounds(
  pb: PocketBase,
  opts: {
    view: CalendarViewMode;
    anchor: string;
    displayTimezone?: string;
    days?: number;
  },
): Promise<BoundsResponse> {
  const q = new URLSearchParams({
    view: opts.view,
    anchor: opts.anchor,
  });
  if (opts.displayTimezone) q.set("displayTimezone", opts.displayTimezone);
  if (opts.days) q.set("days", String(opts.days));
  return pb.send<BoundsResponse>(`/api/email/calendar/bounds?${q}`, { method: "GET" });
}

export async function fetchCalendarWindow(
  pb: PocketBase,
  opts: { from: string; to: string; displayTimezone?: string },
): Promise<WindowResponse> {
  const q = new URLSearchParams({ from: opts.from, to: opts.to });
  if (opts.displayTimezone) q.set("displayTimezone", opts.displayTimezone);
  return pb.send<WindowResponse>(`/api/email/calendar/window?${q}`, { method: "GET" });
}

export async function createCalendarEvent(pb: PocketBase, body: EventWriteInput) {
  return pb.send<{ ok: boolean; id: string }>("/api/email/calendar/events", {
    method: "POST",
    body,
  });
}

export async function updateCalendarEvent(pb: PocketBase, id: string, body: Partial<EventWriteInput>) {
  return pb.send<{ ok: boolean; id: string }>(`/api/email/calendar/events/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body,
  });
}

export async function getCalendarSettings(pb: PocketBase) {
  return pb.send<{ displayTimezone: string }>("/api/email/calendar/settings", { method: "GET" });
}

export async function setCalendarSettings(pb: PocketBase, displayTimezone: string) {
  return pb.send<{ ok: boolean; displayTimezone: string }>("/api/email/calendar/settings", {
    method: "POST",
    body: { displayTimezone },
  });
}

export async function createLocalCalendar(
  pb: PocketBase,
  body: { name: string; color?: string; timezone?: string },
) {
  return pb.send<{ ok: boolean; id: string }>("/api/email/calendar/calendars/local", {
    method: "POST",
    body,
  });
}

export async function importICS(
  pb: PocketBase,
  body: { calendarId?: string; icsText?: string; url?: string },
) {
  return pb.send<{ ok: boolean; calendarId: string; imported: number }>(
    "/api/email/calendar/ics/import",
    { method: "POST", body },
  );
}

export async function refreshICS(pb: PocketBase) {
  return pb.send<{ ok: boolean; imported: number }>("/api/email/calendar/ics/refresh", {
    method: "POST",
  });
}

export function icsExportURL(calendarId: string): string {
  return `http://127.0.0.1:8090/api/email/calendar/ics/export?calendar=${encodeURIComponent(calendarId)}`;
}

export async function discoverCalDAV(
  pb: PocketBase,
  body: { url: string; username: string; password: string },
) {
  return pb.send<{ ok: boolean; calendars: { path: string; displayName: string }[] }>(
    "/api/email/calendar/caldav/discover",
    { method: "POST", body },
  );
}

export async function subscribeCalDAV(
  pb: PocketBase,
  body: {
    url: string;
    username: string;
    password: string;
    calendarPath: string;
    displayName?: string;
    color?: string;
    timezone?: string;
  },
) {
  return pb.send<{ ok: boolean; calendarId: string; imported: number; error?: string }>(
    "/api/email/calendar/caldav/subscribe",
    { method: "POST", body },
  );
}

export async function syncCalDAV(pb: PocketBase, calendarId?: string) {
  return pb.send<{ ok: boolean }>("/api/email/calendar/caldav/sync", {
    method: "POST",
    body: calendarId ? { calendarId } : { all: true },
  });
}

export async function syncAllCalendars(pb: PocketBase) {
  return pb.send<{ ok: boolean }>("/api/email/calendar/sync", { method: "POST" });
}

export function resolveCalendarColor(color: string): string {
  const hit = CALENDAR_COLORS.find((c) => c.id === color || c.hex === color);
  return hit?.hex ?? (color.startsWith("#") ? color : "#0f6e56");
}

/** Format Go display wall `YYYY-MM-DDTHH:mm:ss` without zone conversion. */
export function formatDisplayTime(wall: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(wall.trim());
  if (!m) return wall;
  const hour = Number(m[4]);
  const minute = m[5];
  const ampm = hour >= 12 ? "PM" : "AM";
  const h12 = hour % 12 === 0 ? 12 : hour % 12;
  return `${h12}:${minute} ${ampm}`;
}

export function formatDisplayDayLabel(day: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day.trim());
  if (!m) return day;
  const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  return `${months[Number(m[2]) - 1]} ${Number(m[3])}, ${m[1]}`;
}

/** Minutes from midnight for a Go display wall string (layout only). */
export function minutesFromDisplayWall(wall: string): number {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(wall.trim());
  if (!m) return 0;
  return Number(m[4]) * 60 + Number(m[5]);
}

export function addCalendarDays(day: string, delta: number): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day.trim());
  if (!m) return day;
  // Civil date arithmetic via UTC noon avoids DST zone math on the date itself.
  const t = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]) + delta, 12, 0, 0);
  const d = new Date(t);
  const y = d.getUTCFullYear();
  const mo = String(d.getUTCMonth() + 1).padStart(2, "0");
  const da = String(d.getUTCDate()).padStart(2, "0");
  return `${y}-${mo}-${da}`;
}

export function todayAnchorLocal(): string {
  // Anchor for navigation only — bounds/window still computed in Go with display TZ.
  const n = new Date();
  const y = n.getFullYear();
  const mo = String(n.getMonth() + 1).padStart(2, "0");
  const da = String(n.getDate()).padStart(2, "0");
  return `${y}-${mo}-${da}`;
}
