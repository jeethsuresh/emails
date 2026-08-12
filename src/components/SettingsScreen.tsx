import { useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import { ConnectionForm } from "./ConnectionForm";
import { emptyConnection, fromAccountRecord, type ConnectionSettings } from "../lib/connection";
import { getAnalyzerSettings, saveAnalyzerSettings } from "../lib/analysis";
import { SyncLivePanel } from "./SyncBadge";
import type { AnalyzerSettings, SyncStatus } from "../../shared/types";

function AnalyzerSettingsForm({ pb }: { pb: PocketBase }) {
  const [form, setForm] = useState<AnalyzerSettings | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    void getAnalyzerSettings(pb)
      .then((s) =>
        setForm({
          ...s,
          syncIntervalMinutes: s.syncIntervalMinutes || 5,
        }),
      )
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
  }, [pb]);

  if (!form) {
    return <p className="hint">Loading LLM settings…</p>;
  }

  return (
    <form
      className="account-form"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        setError(null);
        setSaved(false);
        void saveAnalyzerSettings(pb, form)
          .then((next) => {
            setForm(next);
            setSaved(true);
          })
          .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
          .finally(() => setBusy(false));
      }}
    >
      <h2 className="form-section">Local LLM analysis</h2>
      <label>
        Model
        <input
          value={form.model}
          onChange={(e) => setForm({ ...form, model: e.target.value })}
          placeholder="google/gemma-4-e4b"
        />
      </label>
      <label>
        Base URL
        <input
          value={form.baseUrl}
          onChange={(e) => setForm({ ...form, baseUrl: e.target.value })}
          placeholder="http://127.0.0.1:1234"
        />
      </label>
      <h2 className="form-section">Sync</h2>
      <label>
        Sync every (minutes)
        <input
          type="number"
          min={1}
          max={60}
          value={form.syncIntervalMinutes ?? 5}
          onChange={(e) =>
            setForm({
              ...form,
              syncIntervalMinutes: Number(e.target.value) || 5,
            })
          }
        />
      </label>
      {error && <p className="error">{error}</p>}
      <div className="modal-actions">
        <button type="submit" disabled={busy}>
          {busy ? "Saving…" : saved ? "Saved" : "Save settings"}
        </button>
      </div>
    </form>
  );
}

function SyncLogsSection({ status }: { status: SyncStatus | null }) {
  const [live, setLive] = useState<SyncStatus | null>(status);

  useEffect(() => {
    setLive(status);
  }, [status]);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const next = await window.email.getSyncStatus();
        if (!cancelled) setLive(next);
      } catch {
        /* ignore */
      }
    };
    void tick();
    const t = setInterval(() => void tick(), 1000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, []);

  return (
    <section className="settings-sync-logs">
      <h2 className="form-section">Sync logs</h2>
      {!live ? (
        <p className="hint">Waiting for sync status…</p>
      ) : (
        <SyncLivePanel status={live} alwaysShow />
      )}
    </section>
  );
}

export function SettingsScreen({
  pb,
  status,
  onClose,
  onSaved,
}: {
  pb: PocketBase;
  status: SyncStatus | null;
  onClose: () => void;
  onSaved: () => Promise<void> | void;
}) {
  const [initial, setInitial] = useState<ConnectionSettings | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        const list = await pb.collection("accounts").getList(1, 1);
        if (list.items[0]) {
          setInitial(fromAccountRecord(list.items[0] as unknown as Record<string, unknown>));
        } else {
          setInitial(emptyConnection());
        }
      } catch (err) {
        setLoadError(err instanceof Error ? err.message : String(err));
        setInitial(emptyConnection());
      }
    })();
  }, [pb]);

  return (
    <div className="modal-backdrop" role="presentation" onClick={onClose}>
      <div className="modal settings-modal" role="dialog" aria-label="Settings" onClick={(e) => e.stopPropagation()}>
        {loadError && <p className="error">{loadError}</p>}
        {!initial ? (
          <p>Loading settings…</p>
        ) : (
          <ConnectionForm
            title="Settings"
            subtitle="Update IMAP/SMTP connection and TLS mode."
            initial={initial}
            submitLabel="Save & sync"
            onCancel={onClose}
            onSaved={async () => {
              await onSaved();
              onClose();
            }}
          />
        )}
        <AnalyzerSettingsForm pb={pb} />
        <SyncLogsSection status={status} />
      </div>
    </div>
  );
}
