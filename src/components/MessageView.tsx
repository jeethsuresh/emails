import { useState } from "react";
import { SafeHtmlFrame } from "./SafeHtmlFrame";
import { decodeMIMEWords } from "../lib/mimeWords";
import { priorityLabel } from "../lib/analysis";
import type { MessageAnalysis } from "../../shared/types";
import type { ComposeMode } from "../lib/mailApi";

interface Message {
  id: string;
  subject: string;
  from_addr: string;
  date: string;
  body_text: string;
  body_html?: string;
  flagged: boolean;
}

interface Folder {
  id: string;
  name: string;
  role: string;
}

const ACTION_LABELS: Record<string, string> = {
  move_to_folder: "Move to folder",
  move_to_spam: "Move to spam",
  add_event: "Add event",
  add_todo: "Add to-do",
};

function AnalysisPanel({
  analysis,
  onApply,
}: {
  analysis: MessageAnalysis;
  onApply: (analysis: MessageAnalysis) => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [applied, setApplied] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (analysis.status !== "done") return null;

  const label = priorityLabel(analysis.priority);
  const actionLabel = analysis.suggested_action
    ? analysis.suggested_action === "move_to_folder" && analysis.create_folder
      ? "Create folder and move"
      : ACTION_LABELS[analysis.suggested_action]
    : null;

  return (
    <section className="analysis-panel">
      {label ? (
        <span className={`priority-tag priority-${analysis.priority}`}>{label} priority</span>
      ) : null}
      {actionLabel ? (
        <p className="analysis-suggestion">
          Suggested: <strong>{actionLabel}</strong>
          {analysis.action_target ? ` — ${analysis.action_target}` : ""}
        </p>
      ) : null}
      {actionLabel ? (
        <div className="analysis-actions">
          <button
            type="button"
            disabled={busy || applied}
            onClick={() => {
              setBusy(true);
              setError(null);
              void onApply(analysis)
                .then(() => setApplied(true))
                .catch((err: unknown) => {
                  if (err instanceof Error && err.message === "cancelled") return;
                  setError(err instanceof Error ? err.message : String(err));
                })
                .finally(() => setBusy(false));
            }}
          >
            {busy ? "Applying…" : applied ? "Applied" : "Apply"}
          </button>
          {error && <span className="error">{error}</span>}
        </div>
      ) : null}
    </section>
  );
}

function MessageActionBar({
  folders,
  busy,
  error,
  onCompose,
  onMove,
  onArchive,
  onSpam,
  onDelete,
}: {
  folders: Folder[];
  busy: boolean;
  error: string | null;
  onCompose: (mode: ComposeMode) => void;
  onMove: (folderId: string) => void;
  onArchive: () => void;
  onSpam: () => void;
  onDelete: () => void;
}) {
  const moveFolders = folders.filter(
    (f) => !["trash", "spam", "junk"].includes(f.role.toLowerCase()),
  );

  return (
    <div className="message-toolbar" role="toolbar" aria-label="Message actions">
      <div className="message-toolbar-group">
        <button type="button" disabled={busy} onClick={() => onCompose("reply")}>
          Reply
        </button>
        <button type="button" disabled={busy} onClick={() => onCompose("reply_all")}>
          Reply all
        </button>
        <button type="button" disabled={busy} onClick={() => onCompose("forward")}>
          Forward
        </button>
      </div>
      <div className="message-toolbar-group">
        <label className="message-move">
          <span className="sr-only">Move to folder</span>
          <select
            disabled={busy || moveFolders.length === 0}
            defaultValue=""
            onChange={(e) => {
              const id = e.target.value;
              if (!id) return;
              onMove(id);
              e.target.value = "";
            }}
          >
            <option value="">Move…</option>
            {moveFolders.map((folder) => (
              <option key={folder.id} value={folder.id}>
                {folder.name}
              </option>
            ))}
          </select>
        </label>
        <button type="button" disabled={busy} onClick={onArchive}>
          Archive
        </button>
        <button type="button" disabled={busy} onClick={onSpam}>
          Spam
        </button>
        <button type="button" className="danger" disabled={busy} onClick={onDelete}>
          Delete
        </button>
      </div>
      {error ? <span className="error message-toolbar-error">{error}</span> : null}
    </div>
  );
}

export function MessageView({
  message,
  loadingBody = false,
  analysis,
  folders = [],
  onApplyAnalysis,
  onCompose,
  onMoveMessage,
  onArchiveMessage,
  onSpamMessage,
  onDeleteMessage,
}: {
  message: Message | null;
  loadingBody?: boolean;
  analysis?: MessageAnalysis;
  folders?: Folder[];
  onApplyAnalysis?: (analysis: MessageAnalysis) => Promise<void>;
  onCompose?: (mode: ComposeMode, useSuggestedReply?: boolean) => Promise<void>;
  onMoveMessage?: (folderId: string) => Promise<void>;
  onArchiveMessage?: () => Promise<void>;
  onSpamMessage?: () => Promise<void>;
  onDeleteMessage?: () => Promise<void>;
}) {
  const [actionBusy, setActionBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [composeBusy, setComposeBusy] = useState(false);
  const [composeError, setComposeError] = useState<string | null>(null);

  if (!message) {
    return (
      <article className="reader empty-reader">
        <p>Select a message</p>
      </article>
    );
  }

  const html = message.body_html?.trim() ?? "";
  const text = message.body_text?.trim() ?? "";
  const hasHtml = html.length > 0;
  const subject = decodeMIMEWords(message.subject || "");
  const suggestedReply = analysis?.suggested_reply.trim() ?? "";

  const runAction = async (action: () => Promise<void>) => {
    setActionBusy(true);
    setActionError(null);
    try {
      await action();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setActionBusy(false);
    }
  };

  const compose = (mode: ComposeMode, useSuggestedReply = false) => {
    if (!onCompose) return;
    setComposeBusy(true);
    setComposeError(null);
    void onCompose(mode, useSuggestedReply)
      .catch((err: unknown) =>
        setComposeError(err instanceof Error ? err.message : String(err)),
      )
      .finally(() => setComposeBusy(false));
  };

  const busy = actionBusy || composeBusy;

  return (
    <article className="reader">
      <header>
        <h1>{subject || "(no subject)"}</h1>
        <p className="reader-meta">
          <span>
            {message.from_addr}
            {message.flagged ? " · ★" : ""}
          </span>
          <time>{message.date ? new Date(message.date).toLocaleString() : ""}</time>
        </p>
        {onCompose || onMoveMessage || onArchiveMessage || onSpamMessage || onDeleteMessage ? (
          <MessageActionBar
            folders={folders}
            busy={busy}
            error={actionError || composeError}
            onCompose={(mode) => compose(mode)}
            onMove={(folderId) => {
              if (onMoveMessage) void runAction(() => onMoveMessage(folderId));
            }}
            onArchive={() => {
              if (onArchiveMessage) void runAction(onArchiveMessage);
            }}
            onSpam={() => {
              if (onSpamMessage) void runAction(onSpamMessage);
            }}
            onDelete={() => {
              if (onDeleteMessage) void runAction(onDeleteMessage);
            }}
          />
        ) : null}
      </header>
      {onCompose && suggestedReply ? (
        <section className="analysis-panel">
          <p className="analysis-suggestion">
            Suggested reply: <span>{suggestedReply}</span>
          </p>
          <div className="analysis-actions">
            <button
              type="button"
              disabled={busy}
              onClick={() => compose("reply", true)}
            >
              Use reply
            </button>
          </div>
        </section>
      ) : null}
      {analysis && onApplyAnalysis ? (
        <AnalysisPanel
          key={analysis.message}
          analysis={analysis}
          onApply={onApplyAnalysis}
        />
      ) : null}
      {loadingBody ? (
        <p className="hint">Loading body…</p>
      ) : hasHtml ? (
        <SafeHtmlFrame html={html} title={subject || "Email body"} />
      ) : (
        <pre className="body">{text || "(empty body)"}</pre>
      )}
    </article>
  );
}
