import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { EventItem, ItemStatus } from "../../shared/types";

function formatWhen(value: string | null | undefined): string {
  const raw = (value ?? "").trim();
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString();
}

function formatRange(
  startsAt: string | null | undefined,
  endsAt: string | null | undefined,
): string {
  const start = formatWhen(startsAt);
  const end = formatWhen(endsAt);
  if (!start && !end) return "No time";
  if (start && end) return `${start} – ${end}`;
  if (start) return `Starts ${start}`;
  return `Ends ${end}`;
}

function itemStatus(item: EventItem): ItemStatus {
  return item.status === "draft" ? "draft" : "approved";
}

function sortEvents(items: EventItem[]): EventItem[] {
  return [...items].sort((a, b) => {
    const sa = itemStatus(a);
    const sb = itemStatus(b);
    if (sa !== sb) return sa === "draft" ? -1 : 1;
    const aStart = (a.starts_at ?? "").trim();
    const bStart = (b.starts_at ?? "").trim();
    if (!aStart && !bStart) return (b.created_at || "").localeCompare(a.created_at || "");
    if (!aStart) return 1;
    if (!bStart) return -1;
    return aStart.localeCompare(bStart);
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

  const approve = async (item: EventItem) => {
    await pb.collection("events").update(item.id, { status: "approved" });
    await refresh();
  };

  const dismiss = async (item: EventItem) => {
    await pb.collection("events").delete(item.id);
    await refresh();
  };

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
      {loading && items.length === 0 ? <p className="hint">Loading events…</p> : null}
      <ul>
        {items.map((item) => {
          const draft = itemStatus(item) === "draft";
          return (
            <li key={item.id} className={draft ? "task-row is-draft" : "task-row"}>
              <div className="task-main">
                <strong>
                  {draft ? <span className="draft-tag">Draft</span> : null}
                  {item.title || "(untitled)"}
                </strong>
                <time dateTime={(item.starts_at || item.ends_at || undefined) ?? undefined}>
                  {formatRange(item.starts_at, item.ends_at)}
                </time>
              </div>
              {(item.notes ?? "").trim() ? <p className="task-notes">{item.notes}</p> : null}
              {item.source_message ? (
                <p className="task-meta">From message {item.source_message}</p>
              ) : null}
              {draft ? (
                <div className="task-actions">
                  <button type="button" onClick={() => void approve(item)}>
                    Approve
                  </button>
                  <button type="button" className="danger" onClick={() => void dismiss(item)}>
                    Dismiss
                  </button>
                </div>
              ) : null}
            </li>
          );
        })}
        {!loading && items.length === 0 && !error ? (
          <li className="empty">No events yet — analysis drafts and Apply land here.</li>
        ) : null}
      </ul>
    </section>
  );
}
