import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { EventItem, ItemStatus } from "../../shared/types";
import type { CalendarRecord } from "../lib/calendarApi";
import { CreateEventModal } from "./CreateEventModal";

function itemStatus(item: EventItem): ItemStatus {
  return item.status === "draft" ? "draft" : "approved";
}

/** Legacy list helper — prefer CalendarView list mode. */
export function EventList({ pb, active }: { pb: PocketBase; active: boolean }) {
  const [items, setItems] = useState<EventItem[]>([]);
  const [calendars, setCalendars] = useState<CalendarRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [rows, cals] = await Promise.all([
        pb.collection("events").getFullList<EventItem>({ batch: 200 }),
        pb.collection("calendars").getFullList<CalendarRecord>({ batch: 50 }),
      ]);
      setItems(rows);
      setCalendars(cals);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [pb]);

  useEffect(() => {
    if (!active) return;
    void refresh();
  }, [active, refresh]);

  return (
    <section className="task-list" aria-label="Events">
      <header className="task-list-header">
        <h2>Events</h2>
        <div className="task-list-actions">
          <button type="button" onClick={() => setCreateOpen(true)}>
            New event
          </button>
          <button type="button" onClick={() => void refresh()} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        </div>
      </header>
      {error ? <p className="error">{error}</p> : null}
      <ul>
        {items.map((item) => (
          <li key={item.id} className={itemStatus(item) === "draft" ? "task-row is-draft" : "task-row"}>
            <strong>{item.title || "(untitled)"}</strong>
          </li>
        ))}
      </ul>
      {createOpen ? (
        <CreateEventModal
          pb={pb}
          calendars={calendars}
          defaultTimezone="UTC"
          onClose={() => setCreateOpen(false)}
          onSaved={() => void refresh()}
        />
      ) : null}
    </section>
  );
}
