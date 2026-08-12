import { FormEvent, useState } from "react";
import type PocketBase from "pocketbase";
import {
  CALENDAR_COLORS,
  createLocalCalendar,
  discoverCalDAV,
  icsExportURL,
  importICS,
  refreshICS,
  resolveCalendarColor,
  subscribeCalDAV,
  syncAllCalendars,
  syncCalDAV,
  type CalendarRecord,
} from "../lib/calendarApi";

type ManagerTab = "local" | "ics" | "caldav";

export function CalendarManagerModal({
  pb,
  calendars,
  defaultTimezone,
  onClose,
  onChanged,
}: {
  pb: PocketBase;
  calendars: CalendarRecord[];
  defaultTimezone: string;
  onClose: () => void;
  onChanged: () => void;
}) {
  const [tab, setTab] = useState<ManagerTab>("local");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const [localName, setLocalName] = useState("");
  const [localColor, setLocalColor] = useState<string>(CALENDAR_COLORS[0].hex);
  const [localTz, setLocalTz] = useState(defaultTimezone || "UTC");

  const [icsTarget, setIcsTarget] = useState("");
  const [icsURL, setIcsURL] = useState("");
  const [icsText, setIcsText] = useState("");

  const [davURL, setDavURL] = useState("");
  const [davUser, setDavUser] = useState("");
  const [davPass, setDavPass] = useState("");
  const [davList, setDavList] = useState<{ path: string; displayName: string }[]>([]);
  const [davColor, setDavColor] = useState<string>(CALENDAR_COLORS[2].hex);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      await fn();
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const addLocal = async (e: FormEvent) => {
    e.preventDefault();
    await run(async () => {
      await createLocalCalendar(pb, {
        name: localName.trim(),
        color: localColor,
        timezone: localTz.trim() || "UTC",
      });
      setLocalName("");
      setMessage("Local calendar created.");
    });
  };

  const doICSImport = async (e: FormEvent) => {
    e.preventDefault();
    await run(async () => {
      const res = await importICS(pb, {
        calendarId: icsTarget || undefined,
        url: icsURL.trim() || undefined,
        icsText: icsText.trim() || undefined,
      });
      setMessage(`Imported ${res.imported} events.`);
      setIcsText("");
    });
  };

  const discover = async (e: FormEvent) => {
    e.preventDefault();
    await run(async () => {
      const res = await discoverCalDAV(pb, {
        url: davURL.trim(),
        username: davUser.trim(),
        password: davPass,
      });
      setDavList(res.calendars ?? []);
      setMessage(`Found ${(res.calendars ?? []).length} calendars.`);
    });
  };

  const subscribe = async (path: string, displayName: string) => {
    await run(async () => {
      const res = await subscribeCalDAV(pb, {
        url: davURL.trim(),
        username: davUser.trim(),
        password: davPass,
        calendarPath: path,
        displayName,
        color: davColor,
        timezone: defaultTimezone || "UTC",
      });
      setMessage(
        res.error
          ? `Subscribed with sync warning: ${res.error}`
          : `Subscribed (${res.imported} events).`,
      );
    });
  };

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div
        className="modal calendar-manager"
        role="dialog"
        aria-label="Manage calendars"
        onClick={(e) => e.stopPropagation()}
      >
        <h2>Manage calendars</h2>
        <div className="calendar-view-toggle" role="tablist">
          {(
            [
              ["local", "Local"],
              ["ics", "ICS"],
              ["caldav", "CalDAV"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              role="tab"
              className={tab === id ? "active" : undefined}
              onClick={() => setTab(id)}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="calendar-manager-existing">
          <h3>Existing</h3>
          <ul>
            {calendars.map((c) => (
              <li key={c.id}>
                <span className="cal-swatch" style={{ background: resolveCalendarColor(c.color) }} />
                <strong>{c.name}</strong>
                <span className="muted">{c.source}</span>
                {c.last_error ? <span className="error">{c.last_error}</span> : null}
                <span className="calendar-manager-row-actions">
                  {c.source === "caldav" ? (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        void run(async () => {
                          await syncCalDAV(pb, c.id);
                          setMessage(`Synced ${c.name}.`);
                        })
                      }
                    >
                      Sync
                    </button>
                  ) : null}
                  {c.source === "ics" && c.ics_url ? (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() =>
                        void run(async () => {
                          await refreshICS(pb);
                          setMessage("ICS subscriptions refreshed.");
                        })
                      }
                    >
                      Refresh
                    </button>
                  ) : null}
                  <a href={icsExportURL(c.id)} download={`${c.name || "calendar"}.ics`}>
                    Export
                  </a>
                  <button
                    type="button"
                    disabled={busy || c.is_default}
                    onClick={() =>
                      void run(async () => {
                        if (c.is_default) return;
                        await pb.collection("calendars").delete(c.id);
                        setMessage(`Removed ${c.name}.`);
                      })
                    }
                  >
                    Remove
                  </button>
                </span>
              </li>
            ))}
          </ul>
          <button
            type="button"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await syncAllCalendars(pb);
                setMessage("Remote calendars synced.");
              })
            }
          >
            Sync all remote
          </button>
        </div>

        {tab === "local" ? (
          <form onSubmit={(e) => void addLocal(e)}>
            <h3>Add local calendar</h3>
            <label>
              Name
              <input value={localName} onChange={(e) => setLocalName(e.target.value)} required />
            </label>
            <label>
              Color
              <select value={localColor} onChange={(e) => setLocalColor(e.target.value)}>
                {CALENDAR_COLORS.map((c) => (
                  <option key={c.id} value={c.hex}>
                    {c.id}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Default timezone
              <input value={localTz} onChange={(e) => setLocalTz(e.target.value)} />
            </label>
            <div className="modal-actions">
              <button type="submit" disabled={busy || !localName.trim()}>
                Create
              </button>
            </div>
          </form>
        ) : null}

        {tab === "ics" ? (
          <form onSubmit={(e) => void doICSImport(e)}>
            <h3>Import ICS</h3>
            <label>
              Target calendar (optional)
              <select value={icsTarget} onChange={(e) => setIcsTarget(e.target.value)}>
                <option value="">Create new ICS calendar</option>
                {calendars.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Subscription URL
              <input
                value={icsURL}
                onChange={(e) => setIcsURL(e.target.value)}
                placeholder="https://…"
              />
            </label>
            <label>
              Or paste .ics text
              <textarea rows={6} value={icsText} onChange={(e) => setIcsText(e.target.value)} />
            </label>
            <label className="file-row">
              Or choose file
              <input
                type="file"
                accept=".ics,text/calendar"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (!file) return;
                  void file.text().then(setIcsText);
                }}
              />
            </label>
            <div className="modal-actions">
              <button type="submit" disabled={busy || (!icsURL.trim() && !icsText.trim())}>
                Import
              </button>
            </div>
          </form>
        ) : null}

        {tab === "caldav" ? (
          <div>
            <form onSubmit={(e) => void discover(e)}>
              <h3>Add CalDAV</h3>
              <label>
                Server URL
                <input
                  value={davURL}
                  onChange={(e) => setDavURL(e.target.value)}
                  placeholder="https://caldav.example.com"
                  required
                />
              </label>
              <label>
                Username
                <input value={davUser} onChange={(e) => setDavUser(e.target.value)} required />
              </label>
              <label>
                Password / app token
                <input
                  type="password"
                  value={davPass}
                  onChange={(e) => setDavPass(e.target.value)}
                  required
                />
              </label>
              <label>
                Color
                <select value={davColor} onChange={(e) => setDavColor(e.target.value)}>
                  {CALENDAR_COLORS.map((c) => (
                    <option key={c.id} value={c.hex}>
                      {c.id}
                    </option>
                  ))}
                </select>
              </label>
              <div className="modal-actions">
                <button type="submit" disabled={busy}>
                  Discover
                </button>
              </div>
            </form>
            {davList.length > 0 ? (
              <ul className="caldav-discover-list">
                {davList.map((c) => (
                  <li key={c.path}>
                    <span>{c.displayName}</span>
                    <button type="button" disabled={busy} onClick={() => void subscribe(c.path, c.displayName)}>
                      Subscribe
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        ) : null}

        {error ? <p className="error">{error}</p> : null}
        {message ? <p className="hint">{message}</p> : null}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
