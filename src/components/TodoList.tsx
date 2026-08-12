import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { ItemStatus, TodoItem } from "../../shared/types";

function formatDeadline(value: string | null | undefined): string {
  const raw = (value ?? "").trim();
  if (!raw) return "No deadline";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString();
}

function itemStatus(item: TodoItem): ItemStatus {
  return item.status === "draft" ? "draft" : "approved";
}

function sortTodos(items: TodoItem[]): TodoItem[] {
  return [...items].sort((a, b) => {
    const sa = itemStatus(a);
    const sb = itemStatus(b);
    if (sa !== sb) return sa === "draft" ? -1 : 1;
    const da = (a.deadline ?? "").trim();
    const db = (b.deadline ?? "").trim();
    if (!da && !db) return (b.created_at || "").localeCompare(a.created_at || "");
    if (!da) return 1;
    if (!db) return -1;
    return da.localeCompare(db);
  });
}

export function TodoList({ pb, active }: { pb: PocketBase; active: boolean }) {
  const [items, setItems] = useState<TodoItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const rows = await pb.collection("todos").getFullList<TodoItem>({
        batch: 200,
      });
      setItems(sortTodos(rows));
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

  const approve = async (item: TodoItem) => {
    await pb.collection("todos").update(item.id, { status: "approved" });
    await refresh();
  };

  const dismiss = async (item: TodoItem) => {
    await pb.collection("todos").delete(item.id);
    await refresh();
  };

  return (
    <section className="task-list" aria-label="Todos">
      <header className="task-list-header">
        <h2>
          Todos
          {items.length > 0 ? <span className="count">{items.length}</span> : null}
        </h2>
        <button type="button" onClick={() => void refresh()} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </button>
      </header>
      {error ? <p className="error">{error}</p> : null}
      {loading && items.length === 0 ? <p className="hint">Loading todos…</p> : null}
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
                <time dateTime={item.deadline || undefined}>{formatDeadline(item.deadline)}</time>
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
          <li className="empty">No todos yet — analysis drafts and Apply land here.</li>
        ) : null}
      </ul>
    </section>
  );
}
