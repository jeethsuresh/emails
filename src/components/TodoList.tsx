import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { TodoItem } from "../../shared/types";

function formatDeadline(value: string): string {
  if (!value.trim()) return "No deadline";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function sortTodos(items: TodoItem[]): TodoItem[] {
  return [...items].sort((a, b) => {
    const da = a.deadline?.trim() ?? "";
    const db = b.deadline?.trim() ?? "";
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
      <ul>
        {items.map((item) => (
          <li key={item.id} className="task-row">
            <div className="task-main">
              <strong>{item.title || "(untitled)"}</strong>
              <time dateTime={item.deadline || undefined}>{formatDeadline(item.deadline)}</time>
            </div>
            {item.notes?.trim() ? <p className="task-notes">{item.notes}</p> : null}
            {item.source_message ? (
              <p className="task-meta">From message {item.source_message}</p>
            ) : null}
          </li>
        ))}
        {!loading && items.length === 0 ? (
          <li className="empty">No todos yet — apply an analysis suggestion or add one later.</li>
        ) : null}
      </ul>
    </section>
  );
}
