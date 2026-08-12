import { FormEvent, useState } from "react";
import type PocketBase from "pocketbase";

function datetimeLocalToIso(value: string): string {
  const raw = value.trim();
  if (!raw) return "";
  const d = new Date(raw);
  if (Number.isNaN(d.getTime())) return "";
  return d.toISOString();
}

export function CreateTodoModal({
  pb,
  onClose,
  onSaved,
}: {
  pb: PocketBase;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [title, setTitle] = useState("");
  const [deadline, setDeadline] = useState("");
  const [notes, setNotes] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = title.trim();
    if (!trimmed) return;
    setBusy(true);
    setError(null);
    try {
      await pb.collection("todos").create({
        title: trimmed,
        notes: notes.trim(),
        source_message: "",
        created_at: new Date().toISOString(),
        deadline: datetimeLocalToIso(deadline),
        status: "approved",
      });
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <form
        className="modal"
        role="dialog"
        aria-label="New todo"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => void submit(e)}
      >
        <h2>New todo</h2>
        <label>
          Title
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="What needs doing?"
            required
            autoFocus
          />
        </label>
        <label>
          Deadline
          <input
            type="datetime-local"
            value={deadline}
            onChange={(e) => setDeadline(e.target.value)}
          />
        </label>
        <label>
          Notes
          <textarea
            rows={4}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Optional"
          />
        </label>
        {error ? <p className="error">{error}</p> : null}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={busy || !title.trim()}>
            {busy ? "Adding…" : "Add todo"}
          </button>
        </div>
      </form>
    </div>
  );
}
