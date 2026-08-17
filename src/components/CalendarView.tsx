import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type PocketBase from "pocketbase";
import { CreateEventModal } from "./CreateEventModal";
import { CalendarManagerModal } from "./CalendarManagerModal";
import {
  addCalendarDays,
  fetchCalendarBounds,
  fetchCalendarWindow,
  formatDisplayDayLabel,
  formatDisplayTime,
  getCalendarSettings,
  minutesFromDisplayWall,
  resolveCalendarColor,
  setCalendarSettings,
  todayAnchorLocal,
  type CalendarRecord,
  type CalendarViewMode,
  type EventWriteInput,
  type WindowEvent,
} from "../lib/calendarApi";
import { useViewport } from "../lib/viewport";

const HOUR_HEIGHT = 48;

export function CalendarView({ pb, active }: { pb: PocketBase; active: boolean }) {
  const viewport = useViewport();
  const [mode, setMode] = useState<CalendarViewMode>(() =>
    typeof window !== "undefined" && window.innerWidth < 640 ? "day" : "multi",
  );

  useEffect(() => {
    if (viewport === "phone" && mode === "multi") setMode("day");
  }, [viewport, mode]);
  const [multiDays, setMultiDays] = useState(7);
  const [anchor, setAnchor] = useState(todayAnchorLocal);
  const [displayTimezone, setDisplayTimezone] = useState("");
  const [events, setEvents] = useState<WindowEvent[]>([]);
  const [calendars, setCalendars] = useState<CalendarRecord[]>([]);
  const [boundsFromDate, setBoundsFromDate] = useState("");
  const [dayColumns, setDayColumns] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [createInitial, setCreateInitial] = useState<Partial<EventWriteInput> | undefined>();
  const [editEvent, setEditEvent] = useState<WindowEvent | null>(null);
  const [tzDraft, setTzDraft] = useState("");
  const [managerOpen, setManagerOpen] = useState(false);

  const refreshCalendars = useCallback(async () => {
    const rows = await pb.collection("calendars").getFullList<CalendarRecord>({ batch: 50 });
    setCalendars(rows);
  }, [pb]);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const settings = await getCalendarSettings(pb);
      const tz = settings.displayTimezone || displayTimezone;
      if (settings.displayTimezone && settings.displayTimezone !== displayTimezone) {
        setDisplayTimezone(settings.displayTimezone);
        setTzDraft(settings.displayTimezone);
      }
      const activeView: CalendarViewMode = mode;
      const bounds = await fetchCalendarBounds(pb, {
        view: activeView,
        anchor,
        displayTimezone: tz || undefined,
        days: multiDays,
      });
      setBoundsFromDate(bounds.fromDate || "");
      if (bounds.displayTimezone) {
        setDisplayTimezone(bounds.displayTimezone);
        setTzDraft((prev) => prev || bounds.displayTimezone);
      }

      const win = await fetchCalendarWindow(pb, {
        from: bounds.from,
        to: bounds.to,
        displayTimezone: bounds.displayTimezone || tz || undefined,
      });
      setEvents(win.events ?? []);

      if (activeView === "day") {
        setDayColumns([bounds.anchor || anchor]);
      } else if (activeView === "multi") {
        const fromDay = bounds.fromDate || bounds.anchor || anchor;
        const cols: string[] = [];
        for (let i = 0; i < (bounds.days || multiDays); i++) {
          cols.push(addCalendarDays(fromDay, i));
        }
        setDayColumns(cols);
      } else {
        setDayColumns([]);
      }

      await refreshCalendars();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [pb, mode, anchor, multiDays, displayTimezone, refreshCalendars]);

  useEffect(() => {
    if (!active) return;
    void refresh();
  }, [active, refresh]);

  const shiftAnchor = (dir: number) => {
    if (mode === "list") return;
    if (mode === "day") setAnchor((a) => addCalendarDays(a, dir));
    else if (mode === "multi") setAnchor((a) => addCalendarDays(a, dir * multiDays));
    else if (mode === "month") setAnchor((a) => addCalendarMonths(a, dir));
    else if (mode === "year") setAnchor((a) => addCalendarYears(a, dir));
  };

  const openCreate = (partial?: Partial<EventWriteInput>) => {
    setEditEvent(null);
    setCreateInitial(partial);
    setCreateOpen(true);
  };

  const allDayByDay = useMemo(() => {
    const map = new Map<string, WindowEvent[]>();
    for (const ev of events) {
      if (!ev.allDay) continue;
      for (const day of daysSpannedAllDay(ev)) {
        const list = map.get(day) ?? [];
        list.push(ev);
        map.set(day, list);
      }
    }
    return map;
  }, [events]);

  const timedByDay = useMemo(() => {
    const map = new Map<string, WindowEvent[]>();
    for (const ev of events) {
      if (ev.allDay) continue;
      const day = ev.displayDay;
      if (!day) continue;
      const list = map.get(day) ?? [];
      list.push(ev);
      map.set(day, list);
    }
    return map;
  }, [events]);

  const saveTz = async (value?: string) => {
    const next = (value ?? tzDraft).trim();
    await setCalendarSettings(pb, next);
    setTzDraft(next);
    setDisplayTimezone(next);
    await refresh();
  };

  const approve = async (ev: WindowEvent) => {
    if (!(ev.startsAt ?? "").trim() || !(ev.endsAt ?? "").trim() || !(ev.title ?? "").trim()) {
      setEditEvent(ev);
      return;
    }
    const baseId = ev.id.includes("#") ? ev.id.slice(0, ev.id.indexOf("#")) : ev.id;
    await pb.collection("events").update(baseId, { status: "approved" });
    await refresh();
  };

  const dismiss = async (ev: WindowEvent) => {
    const baseId = ev.id.includes("#") ? ev.id.slice(0, ev.id.indexOf("#")) : ev.id;
    await pb.collection("events").delete(baseId);
    await refresh();
  };

  return (
    <section className="calendar-shell" aria-label="Calendar">
      <header className="calendar-toolbar">
        <div className="calendar-toolbar-left">
          <h2>Calendar</h2>
          <div className="calendar-view-toggle" role="tablist" aria-label="View">
            {(
              [
                ["day", "Day"],
                ["multi", "Multi-day"],
                ["month", "Month"],
                ["year", "Year"],
                ["list", "List"],
              ] as const
            ).map(([id, label]) => (
              <button
                key={id}
                type="button"
                role="tab"
                className={mode === id ? "active" : undefined}
                aria-selected={mode === id}
                onClick={() => setMode(id)}
              >
                {label}
              </button>
            ))}
          </div>
          {mode === "multi" ? (
            <label className="calendar-days-ctrl">
              Days
              <input
                type="number"
                min={2}
                max={7}
                value={multiDays}
                onChange={(e) => setMultiDays(Math.min(7, Math.max(2, Number(e.target.value) || 7)))}
              />
            </label>
          ) : null}
        </div>
        <div className="calendar-toolbar-right">
          <button
            type="button"
            onClick={() => {
              const today = todayAnchorLocal();
              setAnchor(today);
              if (mode === "list") {
                const el = document.querySelector(`[data-cal-day="${today}"]`);
                el?.scrollIntoView({ block: "start", behavior: "smooth" });
              }
            }}
          >
            Today
          </button>
          {mode !== "list" ? (
            <>
              <button type="button" aria-label="Previous" onClick={() => shiftAnchor(-1)}>
                ‹
              </button>
              <button type="button" aria-label="Next" onClick={() => shiftAnchor(1)}>
                ›
              </button>
            </>
          ) : null}
          <span className="calendar-anchor">{formatDisplayDayLabel(anchor)}</span>
          <label className="calendar-tz">
            TZ
            <input
              value={tzDraft}
              onChange={(e) => setTzDraft(e.target.value)}
              onBlur={() => void saveTz()}
              placeholder={displayTimezone || "system"}
              list="calendar-tz-hints"
            />
            <datalist id="calendar-tz-hints">
              <option value="">system</option>
              <option value="UTC" />
              <option value="America/New_York" />
              <option value="America/Chicago" />
              <option value="America/Denver" />
              <option value="America/Los_Angeles" />
              <option value="Europe/London" />
              <option value="Europe/Paris" />
              <option value="Asia/Tokyo" />
            </datalist>
            <button type="button" className="linkish" onClick={() => void saveTz("")}>
              System
            </button>
          </label>
          <button type="button" onClick={() => setManagerOpen(true)}>
            Calendars
          </button>
          <button type="button" onClick={() => openCreate({ timezone: displayTimezone })}>
            New event
          </button>
          <button type="button" onClick={() => void refresh()} disabled={loading}>
            {loading ? "…" : "Refresh"}
          </button>
        </div>
      </header>

      <div className="calendar-body">
        <aside className="calendar-rail" aria-label="Calendars">
          <h3>Calendars</h3>
          <ul>
            {calendars.map((c) => (
              <li key={c.id}>
                <label>
                  <input
                    type="checkbox"
                    checked={c.is_visible}
                    onChange={async (e) => {
                      await pb.collection("calendars").update(c.id, { is_visible: e.target.checked });
                      await refresh();
                    }}
                  />
                  <span
                    className="cal-swatch"
                    style={{ background: resolveCalendarColor(c.color || "#0f6e56") }}
                  />
                  {c.name}
                </label>
                {c.last_error ? <p className="error cal-rail-error">{c.last_error}</p> : null}
              </li>
            ))}
          </ul>
          <button type="button" className="rail-manage" onClick={() => setManagerOpen(true)}>
            Manage…
          </button>
        </aside>

        <div className="calendar-main">
          {error ? <p className="error">{error}</p> : null}
          {mode === "list" ? (
            <CalendarList
              events={events}
              scrollToDay={todayAnchorLocal()}
              onEdit={setEditEvent}
              onApprove={(ev) => void approve(ev)}
              onDismiss={(ev) => void dismiss(ev)}
            />
          ) : null}
          {mode === "month" ? (
            <MonthGrid
              anchor={anchor}
              events={events}
              fromDate={boundsFromDate}
              onSelectDay={(day) => {
                setAnchor(day);
                setMode("day");
              }}
              onSelectEvent={setEditEvent}
            />
          ) : null}
          {mode === "year" ? (
            <YearGrid
              anchor={anchor}
              events={events}
              onSelectMonth={(day) => {
                setAnchor(day);
                setMode("month");
              }}
            />
          ) : null}
          {(mode === "day" || mode === "multi") && dayColumns.length > 0 ? (
            <TimeGrid
              days={dayColumns}
              allDayByDay={allDayByDay}
              timedByDay={timedByDay}
              displayTimezone={displayTimezone}
              onSlotClick={(day, hour) => {
                const start = `${day}T${String(hour).padStart(2, "0")}:00`;
                const end = `${day}T${String(hour + 1).padStart(2, "0")}:00`;
                openCreate({
                  startWall: start,
                  endWall: end,
                  timezone: displayTimezone || "UTC",
                  allDay: false,
                });
              }}
              onEventClick={setEditEvent}
            />
          ) : null}
        </div>
      </div>

      {createOpen || editEvent ? (
        <CreateEventModal
          key={editEvent?.id ?? "new-event"}
          pb={pb}
          calendars={calendars}
          defaultTimezone={displayTimezone || "UTC"}
          initial={createInitial}
          edit={editEvent}
          onClose={() => {
            setCreateOpen(false);
            setEditEvent(null);
            setCreateInitial(undefined);
          }}
          onSaved={() => void refresh()}
        />
      ) : null}
      {managerOpen ? (
        <CalendarManagerModal
          pb={pb}
          calendars={calendars}
          defaultTimezone={displayTimezone || "UTC"}
          onClose={() => setManagerOpen(false)}
          onChanged={() => void refresh()}
        />
      ) : null}
    </section>
  );
}

function CalendarList({
  events,
  scrollToDay,
  onEdit,
  onApprove,
  onDismiss,
}: {
  events: WindowEvent[];
  scrollToDay: string;
  onEdit: (ev: WindowEvent) => void;
  onApprove: (ev: WindowEvent) => void;
  onDismiss: (ev: WindowEvent) => void;
}) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const groups = useMemo(() => {
    const map = new Map<string, WindowEvent[]>();
    for (const ev of events) {
      let day = ev.allDay ? ev.displayStart.slice(0, 10) : ev.displayDay;
      if (!day) day = "undated";
      const list = map.get(day) ?? [];
      list.push(ev);
      map.set(day, list);
    }
    return [...map.entries()].sort((a, b) => {
      if (a[0] === "undated") return 1;
      if (b[0] === "undated") return -1;
      return a[0].localeCompare(b[0]);
    });
  }, [events]);

  useEffect(() => {
    const root = scrollerRef.current;
    if (!root || !scrollToDay) return;
    const target =
      root.querySelector(`[data-cal-day="${scrollToDay}"]`) ??
      [...root.querySelectorAll<HTMLElement>("[data-cal-day]")].find((el) => {
        const day = el.dataset.calDay ?? "";
        return day !== "undated" && day >= scrollToDay;
      });
    if (!target) return;
    // Past days stay above the fold; pin today (or next upcoming) to the top.
    const top = target.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop;
    root.scrollTop = Math.max(0, top);
  }, [groups, scrollToDay]);

  return (
    <div className="calendar-list-grouped" ref={scrollerRef}>
      {groups.map(([day, items]) => (
        <section key={day} className="calendar-list-day" data-cal-day={day}>
          <h3>{day === "undated" ? "No date" : formatDisplayDayLabel(day)}</h3>
          <ul className="calendar-list">
            {items.map((ev) => {
              const draft = ev.status === "draft";
              return (
                <li key={ev.id} className={draft ? "task-row is-draft" : "task-row"}>
                  <button type="button" className="calendar-list-hit" onClick={() => onEdit(ev)}>
                    <span
                      className="cal-swatch"
                      style={{ background: resolveCalendarColor(ev.calendarColor) }}
                    />
                  <strong className="clamp-2">
                    {draft ? <span className="draft-tag">Draft</span> : null}
                    {ev.title || "(untitled)"}
                  </strong>
                    <span className="muted">
                      {day === "undated"
                        ? "No time set"
                        : ev.allDay
                          ? "All day"
                          : `${formatDisplayTime(ev.displayStart)} – ${formatDisplayTime(ev.displayEnd)}`}
                    </span>
                  </button>
                  {draft ? (
                    <div className="task-actions">
                      <button type="button" onClick={() => onApprove(ev)}>
                        Approve
                      </button>
                      <button type="button" className="danger" onClick={() => onDismiss(ev)}>
                        Dismiss
                      </button>
                    </div>
                  ) : null}
                </li>
              );
            })}
          </ul>
        </section>
      ))}
      {events.length === 0 ? <p className="empty">No events yet.</p> : null}
    </div>
  );
}

function TimeGrid({
  days,
  allDayByDay,
  timedByDay,
  displayTimezone,
  onSlotClick,
  onEventClick,
}: {
  days: string[];
  allDayByDay: Map<string, WindowEvent[]>;
  timedByDay: Map<string, WindowEvent[]>;
  displayTimezone: string;
  onSlotClick: (day: string, hour: number) => void;
  onEventClick: (ev: WindowEvent) => void;
}) {
  const hours = Array.from({ length: 24 }, (_, i) => i);
  return (
    <div className="time-grid" style={{ ["--day-count" as string]: days.length }}>
      <div className="time-grid-allday">
        <div className="time-grid-gutter muted">All day</div>
        {days.map((day) => (
          <div key={day} className="time-grid-allday-col">
            <div className="time-grid-dayhead">{formatDisplayDayLabel(day)}</div>
            {(allDayByDay.get(day) ?? []).map((ev) => (
              <button
                key={ev.id}
                type="button"
                className={ev.status === "draft" ? "cal-chip draft clamp-2" : "cal-chip clamp-2"}
                style={{ borderLeftColor: resolveCalendarColor(ev.calendarColor), background: soft(resolveCalendarColor(ev.calendarColor)) }}
                onClick={() => onEventClick(ev)}
              >
                {ev.title || "(untitled)"}
              </button>
            ))}
          </div>
        ))}
      </div>
      <div className="time-grid-scroll">
        <div className="time-grid-gutter">
          {hours.map((h) => (
            <div key={h} className="time-grid-hourlabel" style={{ height: HOUR_HEIGHT }}>
              {formatHour(h)}
            </div>
          ))}
        </div>
        {days.map((day) => (
          <div key={day} className="time-grid-daycol" style={{ height: 24 * HOUR_HEIGHT }}>
            {hours.map((h) => (
              <button
                key={h}
                type="button"
                className="time-grid-slot"
                style={{ height: HOUR_HEIGHT }}
                aria-label={`Create at ${day} ${formatHour(h)} ${displayTimezone}`}
                onClick={() => onSlotClick(day, h)}
              />
            ))}
            {(timedByDay.get(day) ?? []).map((ev) => {
              const startM = minutesFromDisplayWall(ev.displayStart);
              const endM = Math.max(startM + 30, minutesFromDisplayWall(ev.displayEnd));
              const top = (startM / 60) * HOUR_HEIGHT;
              const height = ((endM - startM) / 60) * HOUR_HEIGHT;
              const lanes = Math.max(1, ev.laneCount);
              const width = 100 / lanes;
              const left = ev.lane * width;
              const color = resolveCalendarColor(ev.calendarColor);
              return (
                <button
                  key={ev.id}
                  type="button"
                  className={ev.status === "draft" ? "cal-block draft" : "cal-block"}
                  style={{
                    top,
                    height,
                    left: `${left}%`,
                    width: `${width}%`,
                    borderLeftColor: color,
                    background: soft(color),
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    onEventClick(ev);
                  }}
                >
                  <strong className="clamp-2">{ev.title || "(untitled)"}</strong>
                  <span>
                    {formatDisplayTime(ev.displayStart)}–{formatDisplayTime(ev.displayEnd)}
                  </span>
                </button>
              );
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

function MonthGrid({
  anchor,
  events,
  fromDate,
  onSelectDay,
  onSelectEvent,
}: {
  anchor: string;
  events: WindowEvent[];
  fromDate: string;
  onSelectDay: (day: string) => void;
  onSelectEvent: (ev: WindowEvent) => void;
}) {
  const startDay = fromDate || monthStart(anchor);
  const cells = Array.from({ length: 42 }, (_, i) => addCalendarDays(startDay, i));
  const byDay = new Map<string, WindowEvent[]>();
  for (const ev of events) {
    const days = ev.allDay ? daysSpannedAllDay(ev) : ev.displayDay ? [ev.displayDay] : [];
    for (const d of days) {
      const list = byDay.get(d) ?? [];
      list.push(ev);
      byDay.set(d, list);
    }
  }
  const monthPrefix = anchor.slice(0, 7);
  return (
    <div className="month-grid">
      {["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"].map((d) => (
        <div key={d} className="month-dow">
          {d}
        </div>
      ))}
      {cells.map((day) => {
        const inMonth = day.startsWith(monthPrefix);
        const list = byDay.get(day) ?? [];
        return (
          <button
            key={day}
            type="button"
            className={inMonth ? "month-cell" : "month-cell muted"}
            onClick={() => onSelectDay(day)}
          >
            <span className="month-cell-num">{Number(day.slice(8))}</span>
            <div className="month-cell-events">
              {list.slice(0, 3).map((ev) => (
                <span
                  key={ev.id}
                  className="cal-chip tight clamp-2"
                  style={{
                    borderLeftColor: resolveCalendarColor(ev.calendarColor),
                    background: soft(resolveCalendarColor(ev.calendarColor)),
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    onSelectEvent(ev);
                  }}
                >
                  {ev.title || "(untitled)"}
                </span>
              ))}
              {list.length > 3 ? <span className="muted">+{list.length - 3}</span> : null}
            </div>
          </button>
        );
      })}
    </div>
  );
}

function YearGrid({
  anchor,
  events,
  onSelectMonth,
}: {
  anchor: string;
  events: WindowEvent[];
  onSelectMonth: (day: string) => void;
}) {
  const year = Number(anchor.slice(0, 4)) || new Date().getFullYear();
  const counts = new Array(12).fill(0);
  for (const ev of events) {
    const day = ev.allDay ? ev.displayStart : ev.displayDay;
    if (day && day.startsWith(String(year))) {
      const m = Number(day.slice(5, 7)) - 1;
      if (m >= 0 && m < 12) counts[m]++;
    }
  }
  const names = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  return (
    <div className="year-grid">
      {names.map((name, i) => (
        <button
          key={name}
          type="button"
          className="year-month"
          onClick={() => onSelectMonth(`${year}-${String(i + 1).padStart(2, "0")}-01`)}
        >
          <strong>{name}</strong>
          <span className="muted">{counts[i]} events</span>
        </button>
      ))}
    </div>
  );
}

function soft(hex: string): string {
  const h = hex.replace("#", "");
  if (h.length !== 6) return "rgba(15,110,86,0.12)";
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return `rgba(${r},${g},${b},0.16)`;
}

function formatHour(h: number): string {
  const ampm = h >= 12 ? "PM" : "AM";
  const h12 = h % 12 === 0 ? 12 : h % 12;
  return `${h12} ${ampm}`;
}

function daysSpannedAllDay(ev: WindowEvent): string[] {
  const start = ev.displayStart.slice(0, 10);
  const endEx = ev.displayEnd.slice(0, 10);
  if (!start) return [];
  const out: string[] = [];
  let cur = start;
  for (let i = 0; i < 370; i++) {
    if (cur >= endEx) break;
    out.push(cur);
    cur = addCalendarDays(cur, 1);
  }
  return out.length ? out : [start];
}

function monthStart(anchor: string): string {
  return `${anchor.slice(0, 7)}-01`;
}

function addCalendarMonths(day: string, delta: number): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!m) return day;
  const t = Date.UTC(Number(m[1]), Number(m[2]) - 1 + delta, Number(m[3]), 12, 0, 0);
  const d = new Date(t);
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}-${String(d.getUTCDate()).padStart(2, "0")}`;
}

function addCalendarYears(day: string, delta: number): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!m) return day;
  return `${Number(m[1]) + delta}-${m[2]}-${m[3]}`;
}
