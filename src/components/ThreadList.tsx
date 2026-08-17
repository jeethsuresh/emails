import { useCallback, useEffect, useRef, useState } from "react";
import type PocketBase from "pocketbase";
import { decodeMIMEWords } from "../lib/mimeWords";
import {
  getThread,
  listThreads,
  type MailThread,
  type ThreadMessage,
} from "../lib/mailApi";

export function ThreadList({
  pb,
  folder,
  receivedFor,
  selectedId,
  refreshKey,
  onOpenThread,
}: {
  pb: PocketBase;
  folder: string | null;
  receivedFor: string;
  selectedId: string | null;
  refreshKey: number;
  onOpenThread: (thread: MailThread, messages: ThreadMessage[]) => void;
}) {
  const [threads, setThreads] = useState<MailThread[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [openingId, setOpeningId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const loadSeq = useRef(0);
  const openSeq = useRef(0);

  const load = useCallback(
    async (nextPage: number, append: boolean) => {
      if (!folder) {
        setThreads([]);
        setTotal(0);
        return;
      }
      const seq = ++loadSeq.current;
      setLoading(true);
      setError(null);
      try {
        const result = await listThreads(pb, {
          folder,
          received_for: receivedFor || undefined,
          page: nextPage,
        });
        if (seq !== loadSeq.current) return;
        setThreads((current) => (append ? [...current, ...result.items] : result.items));
        setTotal(result.totalItems);
        setPage(result.page);
      } catch (err) {
        if (seq === loadSeq.current) {
          setError(err instanceof Error ? err.message : String(err));
        }
      } finally {
        if (seq === loadSeq.current) setLoading(false);
      }
    },
    [folder, pb, receivedFor],
  );

  useEffect(() => {
    void load(1, false);
  }, [load, refreshKey]);

  useEffect(() => {
    openSeq.current += 1;
    setOpeningId(null);
    return () => {
      openSeq.current += 1;
    };
  }, [folder, receivedFor, refreshKey]);

  const openThread = async (thread: MailThread) => {
    const seq = ++openSeq.current;
    setOpeningId(thread.id);
    setError(null);
    try {
      const result = await getThread(pb, thread.id, folder ?? undefined);
      if (seq === openSeq.current) onOpenThread(result.thread, result.messages);
    } catch (err) {
      if (seq === openSeq.current) {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (seq === openSeq.current) setOpeningId(null);
    }
  };

  return (
    <section className="messages thread-list" aria-busy={loading}>
      <h2>
        Threads
        {total > 0 ? <span className="count">{total}</span> : null}
        {loading ? <span className="count">Loading…</span> : null}
      </h2>
      {error ? <p className="error">{error}</p> : null}
      <div className="messages-scroll">
        {threads.length === 0 && !loading ? (
          <p className="empty">No threads in this folder</p>
        ) : (
          <ul>
            {threads.map((thread) => (
              <li
                key={thread.id}
                className={`${selectedId === thread.id ? "active " : ""}${thread.unread_count > 0 ? "unread" : "read"}`.trim()}
              >
                <button
                  type="button"
                  className={`row thread-row ${thread.unread_count > 0 ? "unread" : "read"}`}
                  disabled={openingId === thread.id}
                  onClick={() => void openThread(thread)}
                >
                  <div className="meta">
                    <strong className="clamp-2">
                      {thread.participants || thread.received_for || "(unknown)"}
                    </strong>
                    <time>{thread.last_date ? new Date(thread.last_date).toLocaleString() : ""}</time>
                  </div>
                  <div className="thread-subject-line">
                    <span className="subject clamp-2">
                      {decodeMIMEWords(thread.subject) || "(no subject)"}
                    </span>
                    {thread.message_count > 1 ? (
                      <span className="thread-count" aria-label={`${thread.message_count} messages`}>
                        {thread.message_count}
                      </span>
                    ) : null}
                    {thread.unread_count > 0 ? (
                      <span className="unread-dot" aria-label={`${thread.unread_count} unread`} />
                    ) : null}
                  </div>
                  <div className="snippet">{decodeMIMEWords(thread.snippet)}</div>
                  {openingId === thread.id ? <span className="thread-opening">Opening…</span> : null}
                </button>
              </li>
            ))}
          </ul>
        )}
        {threads.length < total ? (
          <button
            type="button"
            className="load-more"
            disabled={loading}
            onClick={() => void load(page + 1, true)}
          >
            {loading ? "Loading…" : "Load more"}
          </button>
        ) : null}
      </div>
    </section>
  );
}
