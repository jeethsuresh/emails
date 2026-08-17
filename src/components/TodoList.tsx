import { useCallback, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import type { ItemStatus, TodoItem } from "../../shared/types";
import { escapeFilterValue } from "../lib/analysis";
import { CreateTodoModal } from "./CreateTodoModal";

type TodoRow = TodoItem & { received_at: string };

const MESSAGE_DATE_CHUNK = 75;

function formatDate(value: string | null | undefined, empty: string): string {
  const raw = (value ?? "").trim();
  if (!raw) return empty;
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return raw;
  return d.toLocaleString();
}

function itemStatus(item: TodoItem): ItemStatus {
  switch (item.status) {
    case "draft":
      return "draft";
    case "completed":
      return "completed";
    case "approved":
      return "approved";
    default:
      return "approved";
  }
}

function statusRank(status: ItemStatus): number {
  switch (status) {
    case "draft":
      return 0;
    case "approved":
      return 1;
    case "completed":
      return 2;
    default: {
      const _exhaustive: never = status;
      return _exhaustive;
    }
  }
}

function receivedDate(item: TodoRow): string {
  return (item.received_at || item.created_at || "").trim();
}

function sortTodos(items: TodoRow[]): TodoRow[] {
  return [...items].sort((a, b) => {
    const sa = itemStatus(a);
    const sb = itemStatus(b);
    if (sa !== sb) return statusRank(sa) - statusRank(sb);
    const da = (a.deadline ?? "").trim();
    const db = (b.deadline ?? "").trim();
    if (da && db) return da.localeCompare(db);
    if (da && !db) return -1;
    if (!da && db) return 1;
    return receivedDate(b).localeCompare(receivedDate(a));
  });
}

async function loadMessageDates(
  pb: PocketBase,
  ids: string[],
): Promise<Record<string, string>> {
  const unique = [...new Set(ids.map((id) => id.trim()).filter(Boolean))];
  const dates: Record<string, string> = {};
  for (let i = 0; i < unique.length; i += MESSAGE_DATE_CHUNK) {
    const chunk = unique.slice(i, i + MESSAGE_DATE_CHUNK);
    const filter = chunk.map((id) => `id = "${escapeFilterValue(id)}"`).join(" || ");
    const rows = await pb.collection("messages").getFullList<{ id: string; date: string }>({
      filter,
      fields: "id,date",
      batch: 100,
    });
    for (const row of rows) {
      dates[row.id] = row.date ?? "";
    }
  }
  return dates;
}

export function TodoList({ pb, active }: { pb: PocketBase; active: boolean }) {
  const [items, setItems] = useState<TodoRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const rows = await pb.collection("todos").getFullList<TodoItem>({
        batch: 200,
      });
      const needsDate = rows
        .filter((row) => !(row.deadline ?? "").trim() && (row.source_message ?? "").trim())
        .map((row) => row.source_message);
      const dates = await loadMessageDates(pb, needsDate);
      setItems(
        sortTodos(
          rows.map((row) => ({
            ...row,
            received_at: dates[row.source_message] ?? "",
          })),
        ),
      );
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

  const approve = async (item: TodoRow) => {
    await pb.collection("todos").update(item.id, { status: "approved" });
    await refresh();
  };

  const complete = async (item: TodoRow) => {
    await pb.collection("todos").update(item.id, { status: "completed" });
    await refresh();
  };

  const reopen = async (item: TodoRow) => {
    await pb.collection("todos").update(item.id, { status: "approved" });
    await refresh();
  };

  const dismiss = async (item: TodoRow) => {
    await pb.collection("todos").delete(item.id);
    await refresh();
  };

  const drafts = items.filter((item) => itemStatus(item) === "draft");

  const approveAll = async () => {
    if (drafts.length === 0 || bulkBusy) return;
    setBulkBusy(true);
    setError(null);
    try {
      for (const item of drafts) {
        await pb.collection("todos").update(item.id, { status: "approved" });
      }
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBulkBusy(false);
    }
  };

  const dismissAll = async () => {
    if (drafts.length === 0 || bulkBusy) return;
    if (!window.confirm(`Dismiss ${drafts.length} draft todo${drafts.length === 1 ? "" : "s"}?`)) {
      return;
    }
    setBulkBusy(true);
    setError(null);
    try {
      for (const item of drafts) {
        await pb.collection("todos").delete(item.id);
      }
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBulkBusy(false);
    }
  };

  return (
    <section className="task-list" aria-label="Todos">
      <header className="task-list-header">
        <h2>
          Todos
          {items.length > 0 ? <span className="count">{items.length}</span> : null}
        </h2>
        <div className="task-list-actions">
          {drafts.length > 0 ? (
            <>
              <button type="button" disabled={bulkBusy} onClick={() => void approveAll()}>
                {bulkBusy ? "Working…" : `Approve all (${drafts.length})`}
              </button>
              <button
                type="button"
                className="danger"
                disabled={bulkBusy}
                onClick={() => void dismissAll()}
              >
                Dismiss all
              </button>
            </>
          ) : null}
          <button type="button" onClick={() => setCreateOpen(true)}>
            New todo
          </button>
          <button type="button" onClick={() => void refresh()} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        </div>
      </header>
      {error ? <p className="error">{error}</p> : null}
      {loading && items.length === 0 ? <p className="hint">Loading todos…</p> : null}
      <ul>
        {items.map((item) => {
          const status = itemStatus(item);
          const deadline = (item.deadline ?? "").trim();
          const shownDate = deadline || item.received_at || item.created_at;
          const rowClass =
            status === "draft"
              ? "task-row is-draft"
              : status === "completed"
                ? "task-row is-completed"
                : "task-row";
          return (
            <li key={item.id} className={rowClass}>
              <div className="task-main">
                <strong className="clamp-2">
                  {status === "draft" ? <span className="draft-tag">Draft</span> : null}
                  {status === "completed" ? <span className="done-tag">Done</span> : null}
                  {item.title || "(untitled)"}
                </strong>
                <time dateTime={shownDate || undefined}>
                  {deadline
                    ? formatDate(deadline, "No deadline")
                    : formatDate(shownDate, "No date")}
                </time>
              </div>
              {(item.notes ?? "").trim() ? <p className="task-notes clamp-3">{item.notes}</p> : null}
              {item.source_message ? (
                <p className="task-meta">From message {item.source_message}</p>
              ) : null}
              {status === "draft" ? (
                <div className="task-actions">
                  <button type="button" disabled={bulkBusy} onClick={() => void approve(item)}>
                    Approve
                  </button>
                  <button type="button" className="danger" disabled={bulkBusy} onClick={() => void dismiss(item)}>
                    Dismiss
                  </button>
                </div>
              ) : null}
              {status === "approved" ? (
                <div className="task-actions">
                  <button type="button" disabled={bulkBusy} onClick={() => void complete(item)}>
                    Complete
                  </button>
                  <button type="button" className="danger" disabled={bulkBusy} onClick={() => void dismiss(item)}>
                    Delete
                  </button>
                </div>
              ) : null}
              {status === "completed" ? (
                <div className="task-actions">
                  <button type="button" disabled={bulkBusy} onClick={() => void reopen(item)}>
                    Reopen
                  </button>
                  <button type="button" className="danger" disabled={bulkBusy} onClick={() => void dismiss(item)}>
                    Delete
                  </button>
                </div>
              ) : null}
            </li>
          );
        })}
        {!loading && items.length === 0 && !error ? (
          <li className="empty">No todos yet — create one or approve an analysis draft.</li>
        ) : null}
      </ul>
      {createOpen ? (
        <CreateTodoModal
          pb={pb}
          onClose={() => setCreateOpen(false)}
          onSaved={() => void refresh()}
        />
      ) : null}
    </section>
  );
}
