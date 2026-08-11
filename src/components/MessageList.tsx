import { decodeMIMEWords } from "../lib/mimeWords";
import { priorityLabel } from "../lib/analysis";
import type { MessageAnalysis } from "../../shared/types";

interface Message {
  id: string;
  subject: string;
  from_addr: string;
  date: string;
  snippet: string;
  seen: boolean;
  flagged: boolean;
}

export function MessageList({
  messages,
  selectedId,
  totalCount,
  loading,
  hasMore,
  onSelect,
  onToggleFlag,
  onToggleSeen,
  onLoadMore,
  emptyLabel = "No messages in this folder",
  analysisByMessage = {},
}: {
  messages: Message[];
  selectedId: string | null;
  totalCount: number;
  loading?: boolean;
  hasMore?: boolean;
  onSelect: (m: Message) => void;
  onToggleFlag: (m: Message) => void;
  onToggleSeen: (m: Message) => void;
  onLoadMore?: () => void;
  emptyLabel?: string;
  analysisByMessage?: Record<string, MessageAnalysis>;
}) {
  return (
    <section className="messages">
      <h2>
        Messages
        {totalCount > 0 ? (
          <span className="count">
            {messages.length}
            {totalCount > messages.length ? ` of ${totalCount}` : ""}
          </span>
        ) : null}
      </h2>
      <ul>
        {messages.map((m) => {
          const analysis = analysisByMessage[m.id];
          const priority = analysis?.status === "done" ? analysis.priority : "";
          const label = priorityLabel(priority);
          return (
          <li key={m.id} className={selectedId === m.id ? "active" : ""}>
            <button type="button" className="row" onClick={() => onSelect(m)}>
              <div className="meta">
                <strong className={m.seen ? "" : "unread"}>{m.from_addr || "(unknown)"}</strong>
                <time>{m.date ? new Date(m.date).toLocaleString() : ""}</time>
              </div>
              <div className="subject">
                {label ? (
                  <span className={`priority-tag priority-${priority}`}>{label}</span>
                ) : null}
                {decodeMIMEWords(m.subject) || "(no subject)"}
              </div>
              <div className="snippet">{decodeMIMEWords(m.snippet)}</div>
            </button>
            <div className="actions">
              <button type="button" onClick={() => onToggleFlag(m)}>
                {m.flagged ? "★" : "☆"}
              </button>
              <button type="button" onClick={() => onToggleSeen(m)}>
                {m.seen ? "Read" : "Unread"}
              </button>
            </div>
          </li>
          );
        })}
        {messages.length === 0 && <li className="empty">{emptyLabel}</li>}
      </ul>
      {hasMore ? (
        <button
          type="button"
          className="load-more"
          disabled={loading}
          onClick={() => onLoadMore?.()}
        >
          {loading ? "Loading…" : `Load more (${totalCount - messages.length} remaining)`}
        </button>
      ) : null}
    </section>
  );
}
