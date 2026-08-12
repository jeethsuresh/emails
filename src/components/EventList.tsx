import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { EventItem } from "../../shared/types";

function formatWhen(value: string): string {
  if (!value.trim()) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatRange(startsAt: string, endsAt: string): string {
  const start = formatWhen(startsAt);
  const end = formatWhen(endsAt);
  if (!start && !end) return "No time";
  if (start && end) return `${start} – ${end}`;
  if (start) return `Starts ${start}`;
  return `Ends ${end}`;
}

function sortEvents(items: EventItem[]): EventItem[] {
  return [...items].sort((a, b) => {
    const sa = a.starts_at?.trim() ?? "";
    const sb = b.starts_at?.trim() ?? "";
    if (!sa && !sb) return (b.created_at || "").localeCompare(a.created_at || "");
    if (!sa) return 1;
    if (!sb) return -1;
    return sa.localeCompare(sb);
  });
}

export function EventList({ pb, active }: { pb: PocketBase; active: boolean }) {
  const [items, setItems] = useState<EventItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const rows = await pb.collection("events").getFullList<EventItem>({
        batch: 200,
      });
      setItems(sortEvents(rows));
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
        <h2>
          Events
          {items.length > 0 ? <span className="count">{items.length}</span> : null}
        </h2>
        <button type="button" onClick={() => void refresh()} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </button>
      </header>
      {error ? <p className="error">{error}</p> : null}
      <ul>
        {items.map((item) => (
          <li key={item.id} className="task-row">
            <div className="task-main">
              <strong>{item.title || "(untitled)"}</strong>
              <time dateTime={item.starts_at || item.ends_at || undefined}>
                {formatRange(item.starts_at, item.ends_at)}
              </time>
            </div>
            {item.notes?.trim() ? <p className="task-notes">{item.notes}</p> : null}
            {item.source_message ? (
              <p className="task-meta">From message {item.source_message}</p>
            ) : null}
          </li>
        ))}
        {!loading && items.length === 0 ? (
          <li className="empty">No events yet — apply an analysis suggestion or add one later.</li>
        ) : null}
      </ul>
    </section>
  );
}
