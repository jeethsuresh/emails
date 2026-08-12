import { useState } from "react";
import type PocketBase from "pocketbase";

export function ComposeModal({
  pb,
  onClose,
  onSaved,
}: {
  pb: PocketBase;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [to, setTo] = useState("");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <form
        className="modal"
        role="dialog"
        aria-label="Compose"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          setBusy(true);
          setError(null);
          void (async () => {
            try {
              const accounts = await pb.collection("accounts").getList(1, 1);
              const account = accounts.items[0]?.id;
              if (!account) throw new Error("No account");
              await pb.collection("drafts").create({
                account,
                to_addrs: to,
                subject,
                body_text: body,
              });
              // TODO(phase-C): offline compose queue + SMTP send worker
              onSaved();
              onClose();
            } catch (err) {
              setError(err instanceof Error ? err.message : String(err));
            } finally {
              setBusy(false);
            }
          })();
        }}
      >
        <h2>Compose</h2>
        <label>
          To
          <input value={to} onChange={(e) => setTo(e.target.value)} required />
        </label>
        <label>
          Subject
          <input value={subject} onChange={(e) => setSubject(e.target.value)} />
        </label>
        <label>
          Body
          <textarea rows={12} value={body} onChange={(e) => setBody(e.target.value)} />
        </label>
        {error && <p className="error">{error}</p>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save draft"}
          </button>
        </div>
      </form>
    </div>
  );
}
