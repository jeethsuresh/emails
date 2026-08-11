import { useState } from "react";
import { SafeHtmlFrame } from "./SafeHtmlFrame";
import { decodeMIMEWords } from "../lib/mimeWords";
import { priorityLabel } from "../lib/analysis";
import type { MessageAnalysis } from "../../shared/types";

interface Message {
  id: string;
  subject: string;
  from_addr: string;
  date: string;
  body_text: string;
  body_html?: string;
  flagged: boolean;
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
    ? ACTION_LABELS[analysis.suggested_action]
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
                .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
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

export function MessageView({
  message,
  loadingBody = false,
  analysis,
  onApplyAnalysis,
}: {
  message: Message | null;
  loadingBody?: boolean;
  analysis?: MessageAnalysis;
  onApplyAnalysis?: (analysis: MessageAnalysis) => Promise<void>;
}) {
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

  return (
    <article className="reader">
      <header>
        <h1>{subject || "(no subject)"}</h1>
        <p>
          {message.from_addr}
          {message.flagged ? " · ★" : ""}
        </p>
        <time>{message.date ? new Date(message.date).toLocaleString() : ""}</time>
      </header>
      {analysis && onApplyAnalysis ? (
        <AnalysisPanel analysis={analysis} onApply={onApplyAnalysis} />
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
