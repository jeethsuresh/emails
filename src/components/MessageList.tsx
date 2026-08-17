import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { decodeMIMEWords } from "../lib/mimeWords";
import { priorityLabel } from "../lib/analysis";
import type { MessageAnalysis } from "../../shared/types";
import type { ListMessage } from "../lib/messageCache";

/** Fixed row height keeps virtualization cheap and stable (allows ~2–3 line clamps). */
const ROW_HEIGHT = 72;
const OVERSCAN = 10;

function idsKey(ids: string[]): string {
  return ids.join(",");
}

export function MessageList({
  slots,
  selectedId,
  totalCount,
  loading,
  listKey,
  onSelect,
  onToggleFlag,
  onToggleSeen,
  onVisibleRange,
  emptyLabel = "No messages in this folder",
  analysisByMessage = {},
}: {
  /** Sparse list: null = not loaded yet. Length should match totalCount. */
  slots: Array<ListMessage | null>;
  selectedId: string | null;
  totalCount: number;
  loading?: boolean;
  listKey?: string;
  onSelect: (m: ListMessage) => void;
  onToggleFlag: (m: ListMessage) => void;
  onToggleSeen: (m: ListMessage) => void;
  onVisibleRange?: (start: number, end: number, ids: string[]) => void;
  emptyLabel?: string;
  analysisByMessage?: Record<string, MessageAnalysis>;
}) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportH, setViewportH] = useState(600);
  const lastRangeKey = useRef("");

  useLayoutEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    const measure = () => {
      const next = el.clientHeight || 600;
      setViewportH((prev) => (prev === next ? prev : next));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    el.scrollTop = 0;
    setScrollTop(0);
    lastRangeKey.current = "";
  }, [listKey]);

  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const visibleCount = Math.ceil(viewportH / ROW_HEIGHT) + OVERSCAN * 2;
  const end = Math.min(totalCount, start + visibleCount);

  useEffect(() => {
    if (!onVisibleRange || totalCount === 0) return;
    const ids: string[] = [];
    for (let i = start; i < end; i++) {
      const m = slots[i];
      if (m) ids.push(m.id);
    }
    const key = `${start}:${end}:${idsKey(ids)}`;
    if (key === lastRangeKey.current) return;
    lastRangeKey.current = key;
    onVisibleRange(start, Math.max(start, end - 1), ids);
  }, [start, end, slots, totalCount, onVisibleRange]);

  return (
    <section className="messages">
      <h2>
        Messages
        {totalCount > 0 ? <span className="count">{totalCount}</span> : null}
        {loading ? <span className="count">Loading…</span> : null}
      </h2>
      <div
        className="messages-scroll"
        ref={scrollerRef}
        onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      >
        {totalCount === 0 ? (
          <p className="empty">{emptyLabel}</p>
        ) : (
          <div className="messages-virtual" style={{ height: totalCount * ROW_HEIGHT }}>
            <ul
              className="messages-window"
              style={{ transform: `translateY(${start * ROW_HEIGHT}px)` }}
            >
              {Array.from({ length: Math.max(0, end - start) }, (_, offset) => {
                const index = start + offset;
                const m = slots[index];
                if (!m) {
                  return (
                    <li key={`placeholder-${index}`} className="message-placeholder" style={{ height: ROW_HEIGHT }}>
                      Loading…
                    </li>
                  );
                }
                const analysis = analysisByMessage[m.id];
                const priority = analysis?.status === "done" ? analysis.priority : "";
                const label = priorityLabel(priority);
                return (
                  <li
                    key={m.id}
                    className={`${selectedId === m.id ? "active " : ""}${m.seen ? "read" : "unread"}`.trim()}
                    style={{ height: ROW_HEIGHT }}
                  >
                    <button
                      type="button"
                      className={`row ${m.seen ? "read" : "unread"}`}
                      onClick={() => onSelect(m)}
                    >
                      <div className="meta">
                        <strong className="clamp-2">
                          {m.seen ? null : <span className="unread-dot" aria-hidden />}
                          {m.from_addr || "(unknown)"}
                        </strong>
                        <time>{m.date ? new Date(m.date).toLocaleString() : ""}</time>
                      </div>
                      <div className="subject clamp-2">
                        {label ? (
                          <span className={`priority-tag priority-${priority}`}>{label}</span>
                        ) : null}
                        {decodeMIMEWords(m.subject) || "(no subject)"}
                      </div>
                      <div className="snippet clamp-2">{decodeMIMEWords(m.snippet)}</div>
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
            </ul>
          </div>
        )}
      </div>
    </section>
  );
}
