import { useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import { sendMail, type ComposePrefill } from "../lib/mailApi";

export function ComposeModal({
  pb,
  onClose,
  onSaved,
  onSent,
  prefill,
  fromOptions = [],
}: {
  pb: PocketBase;
  onClose: () => void;
  onSaved: () => void;
  onSent?: (result?: { warning?: string }) => void;
  prefill?: ComposePrefill;
  /** Account addresses plus discovered aliases, in preference order. */
  fromOptions?: string[];
}) {
  const senderOptions = Array.from(
    new Set([prefill?.from ?? "", ...fromOptions].filter(Boolean)),
  );
  const firstFrom = senderOptions[0] ?? "";
  const [from, setFrom] = useState(prefill?.from ?? firstFrom);
  const [to, setTo] = useState(prefill?.to.join(", ") ?? "");
  const [cc, setCc] = useState(prefill?.cc.join(", ") ?? "");
  const [subject, setSubject] = useState(prefill?.subject ?? "");
  const [body, setBody] = useState(prefill?.bodyText ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!from && firstFrom) setFrom(firstFrom);
  }, [from, firstFrom]);

  const recipients = (value: string) =>
    value
      .split(",")
      .map((address) => address.trim())
      .filter(Boolean);

  const saveDraft = async () => {
    const accounts = await pb.collection("accounts").getList(1, 1);
    const account = accounts.items[0]?.id;
    if (!account) throw new Error("No account");
    await pb.collection("drafts").create({
      account,
      from_addr: from,
      to_addrs: to,
      cc_addrs: cc,
      subject,
      body_text: body,
      in_reply_to: prefill?.inReplyTo ?? "",
      references: prefill?.references ?? "",
      thread_id: prefill?.threadId ?? "",
      status: "draft",
    });
  };

  const runAction = (action: "save" | "send") => {
    setBusy(true);
    setError(null);
    void (async () => {
      try {
        if (action === "save") {
          await saveDraft();
          onSaved();
        } else {
          const result = await sendMail(pb, {
            from,
            to: recipients(to),
            cc: recipients(cc),
            subject,
            bodyText: body,
            inReplyTo: prefill?.inReplyTo ?? "",
            references: prefill?.references ?? "",
            threadId: prefill?.threadId ?? "",
          });
          onSent?.(result);
        }
        onClose();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setBusy(false);
      }
    })();
  };

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <form
        className="modal"
        role="dialog"
        aria-label="Compose"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          runAction("send");
        }}
      >
        <h2>Compose</h2>
        <label>
          From
          {senderOptions.length > 0 ? (
            <select value={from} onChange={(e) => setFrom(e.target.value)} required>
              {senderOptions.map((address) => (
                <option key={address} value={address}>
                  {address}
                </option>
              ))}
            </select>
          ) : (
            // Accounts may still be loading; let the user type rather than
            // dead-end on an empty picker.
            <input
              type="email"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              placeholder="you@example.com"
              required
            />
          )}
        </label>
        <label>
          To
          <input value={to} onChange={(e) => setTo(e.target.value)} required />
        </label>
        <label>
          Cc
          <input value={cc} onChange={(e) => setCc(e.target.value)} />
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
          <button type="button" disabled={busy || !from.trim()} onClick={() => runAction("save")}>
            Save draft
          </button>
          <button type="submit" disabled={busy}>
            {busy ? "Working…" : "Send"}
          </button>
        </div>
      </form>
    </div>
  );
}
