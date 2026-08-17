import { FormEvent, useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import {
  CALENDAR_COLORS,
  createCalendarEvent,
  type CalendarRecord,
  type EventWriteInput,
  type WindowEvent,
  updateCalendarEvent,
} from "../lib/calendarApi";
import { parseAttendeesField } from "../lib/analysis";

function attendeesToText(emails: string[] | undefined): string {
  return (emails ?? []).join(", ");
}

export function CreateEventModal({
  pb,
  onClose,
  onSaved,
  initial,
  edit,
  calendars,
  defaultTimezone,
  saveEvent,
}: {
  pb: PocketBase;
  onClose: () => void;
  onSaved: () => void;
  initial?: Partial<EventWriteInput>;
  edit?: WindowEvent | null;
  calendars: CalendarRecord[];
  defaultTimezone: string;
  /** When set, used instead of the default create/update path (e.g. AI Apply). */
  saveEvent?: (body: EventWriteInput) => Promise<void>;
}) {
  const defaultCal =
    calendars.find((c) => c.is_default)?.id ?? calendars[0]?.id ?? "";
  const [title, setTitle] = useState(edit?.title ?? initial?.title ?? "");
  const [notes, setNotes] = useState(edit?.notes ?? initial?.notes ?? "");
  const [calendarId, setCalendarId] = useState(
    edit?.calendarId ?? initial?.calendarId ?? defaultCal,
  );
  const [allDay, setAllDay] = useState(edit?.allDay ?? initial?.allDay ?? false);
  const [timezone, setTimezone] = useState(
    edit?.timezone ?? initial?.timezone ?? (defaultTimezone || "UTC"),
  );
  const [startWall, setStartWall] = useState(
    initial?.startWall ?? (edit ? edit.editStartWall : "") ?? "",
  );
  const [endWall, setEndWall] = useState(
    initial?.endWall ?? (edit ? edit.editEndWall : "") ?? "",
  );
  const [attendeesText, setAttendeesText] = useState(
    attendeesToText(initial?.attendees ?? edit?.attendees),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const cal = calendars.find((c) => c.id === calendarId);
    if (cal?.timezone && !edit) {
      setTimezone(cal.timezone);
    }
  }, [calendarId, calendars, edit]);

  const buildBody = (): EventWriteInput | null => {
    const trimmed = title.trim();
    const start = startWall.trim();
    const end = endWall.trim();
    if (!trimmed || !start || !end) {
      setError("Title, start, and end are required");
      return null;
    }
    return {
      title: trimmed,
      notes: notes.trim(),
      calendarId,
      allDay,
      timezone,
      startWall: start,
      endWall: end,
      attendees: parseAttendeesField(attendeesText),
      status: edit?.status === "draft" ? "draft" : (initial?.status ?? "approved"),
      sourceMessage: initial?.sourceMessage,
    };
  };

  const approveDraft = async () => {
    if (!edit?.id) return;
    const body = buildBody();
    if (!body) return;
    setBusy(true);
    setError(null);
    try {
      await updateCalendarEvent(pb, edit.id, { ...body, status: "approved" });
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const dismissDraft = async () => {
    if (!edit?.id) return;
    setBusy(true);
    setError(null);
    try {
      const baseId = edit.id.includes("#") ? edit.id.slice(0, edit.id.indexOf("#")) : edit.id;
      await pb.collection("events").delete(baseId);
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const deleteEvent = async () => {
    if (!edit?.id) return;
    if (!window.confirm("Delete this event?")) return;
    setBusy(true);
    setError(null);
    try {
      const baseId = edit.id.includes("#") ? edit.id.slice(0, edit.id.indexOf("#")) : edit.id;
      await pb.collection("events").delete(baseId);
      onSaved();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const body = buildBody();
    if (!body) return;
    setBusy(true);
    setError(null);
    try {
      if (saveEvent) {
        await saveEvent(body);
        onSaved();
      } else if (edit?.id) {
        await updateCalendarEvent(pb, edit.id, body);
        onSaved();
        onClose();
      } else {
        await createCalendarEvent(pb, body);
        onSaved();
        onClose();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const canSave = Boolean(title.trim() && startWall.trim() && endWall.trim());

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <form
        className="modal"
        role="dialog"
        aria-label={edit ? "Edit event" : "New event"}
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => void submit(e)}
      >
        <h2>{edit ? "Edit event" : "New event"}</h2>
        <label>
          Title
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Event name"
            required
            autoFocus
          />
        </label>
        {calendars.length > 0 ? (
          <label>
            Calendar
            <select value={calendarId} onChange={(e) => setCalendarId(e.target.value)}>
              {calendars.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>
        ) : null}
        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={allDay}
            onChange={(e) => setAllDay(e.target.checked)}
          />
          All day
        </label>
        {!allDay ? (
          <label>
            Timezone
            <input
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
              placeholder="America/New_York"
              list="iana-tz-hints"
            />
            <datalist id="iana-tz-hints">
              <option value="UTC" />
              <option value="America/New_York" />
              <option value="America/Chicago" />
              <option value="America/Denver" />
              <option value="America/Los_Angeles" />
              <option value="Europe/London" />
              <option value="Europe/Paris" />
              <option value="Asia/Tokyo" />
            </datalist>
          </label>
        ) : null}
        <label>
          Starts
          <input
            type={allDay ? "date" : "datetime-local"}
            value={startWall}
            onChange={(e) => setStartWall(e.target.value)}
            required
          />
        </label>
        <label>
          Ends{allDay ? " (inclusive last day)" : ""}
          <input
            type={allDay ? "date" : "datetime-local"}
            value={endWall}
            onChange={(e) => setEndWall(e.target.value)}
            required
          />
        </label>
        <label>
          Attendees
          <input
            value={attendeesText}
            onChange={(e) => setAttendeesText(e.target.value)}
            placeholder="Optional emails, comma-separated"
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
          {edit?.status === "draft" ? (
            <>
              <button type="button" className="danger" disabled={busy} onClick={() => void dismissDraft()}>
                Dismiss
              </button>
              <button type="button" disabled={busy || !canSave} onClick={() => void approveDraft()}>
                Approve
              </button>
            </>
          ) : null}
          {edit?.id && edit.status !== "draft" ? (
            <button type="button" className="danger" disabled={busy} onClick={() => void deleteEvent()}>
              Delete
            </button>
          ) : null}
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" disabled={busy || !canSave}>
            {busy ? "Saving…" : edit ? "Save" : "Add event"}
          </button>
        </div>
        <p className="hint modal-hint">
          Title, start, and end are required. Times are wall clocks in the event timezone.
        </p>
      </form>
    </div>
  );
}

export function CalendarColorSwatch({ color }: { color: string }) {
  const hex = CALENDAR_COLORS.find((c) => c.id === color)?.hex ?? color;
  return <span className="cal-swatch" style={{ background: hex }} aria-hidden />;
}
