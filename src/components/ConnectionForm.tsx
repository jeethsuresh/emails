import { useEffect, useState } from "react";
import {
  TLS_MODE_OPTIONS,
  defaultPorts,
  emptyConnection,
  toAccountPayload,
  type ConnectionSettings,
  type TlsMode,
} from "../lib/connection";

export function ConnectionForm({
  title,
  subtitle,
  initial,
  submitLabel,
  onSaved,
  onCancel,
}: {
  title: string;
  subtitle?: string;
  initial?: ConnectionSettings;
  submitLabel: string;
  onSaved: () => Promise<void> | void;
  onCancel?: () => void;
}) {
  const [form, setForm] = useState<ConnectionSettings>(initial ?? emptyConnection());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (initial) setForm((prev) => (prev === initial ? prev : initial));
  }, [initial]);

  const set = <K extends keyof ConnectionSettings>(key: K, value: ConnectionSettings[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const setImapSecurity = (mode: TlsMode) => {
    setForm((prev) => ({
      ...prev,
      imapSecurity: mode,
      imapPort: defaultPorts("imap", mode),
    }));
  };

  const setSmtpSecurity = (mode: TlsMode) => {
    setForm((prev) => ({
      ...prev,
      smtpSecurity: mode,
      smtpPort: defaultPorts("smtp", mode),
    }));
  };

  return (
    <form
      className="account-form"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        setError(null);
        void window.email
          .saveAccount(toAccountPayload(form))
          .then(() => onSaved())
          .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
          .finally(() => setBusy(false));
      }}
    >
      <h1>{title}</h1>
      {subtitle ? <p className="lede">{subtitle}</p> : null}

      <label>
        Email
        <input
          value={form.email}
          onChange={(e) => {
            const email = e.target.value;
            setForm((prev) => ({
              ...prev,
              email,
              username: prev.username === prev.email || !prev.username ? email : prev.username,
            }));
          }}
          required
          autoComplete="username"
        />
      </label>
      <label>
        Username
        <input
          value={form.username}
          onChange={(e) => set("username", e.target.value)}
          required
          autoComplete="username"
        />
      </label>
      <label>
        Password / app password
        <input
          type="password"
          value={form.password}
          onChange={(e) => set("password", e.target.value)}
          required
          autoComplete="current-password"
        />
      </label>

      <h2 className="form-section">IMAP</h2>
      <div className="grid">
        <label>
          Host
          <input
            value={form.imapHost}
            onChange={(e) => set("imapHost", e.target.value)}
            required
            placeholder="imap.example.com"
          />
        </label>
        <label>
          Port
          <input
            type="number"
            value={form.imapPort}
            onChange={(e) => set("imapPort", Number(e.target.value))}
            required
          />
        </label>
      </div>
      <fieldset className="tls-fieldset">
        <legend>IMAP security</legend>
        {TLS_MODE_OPTIONS.map((opt) => (
          <label key={opt.value} className="radio-row">
            <input
              type="radio"
              name="imapSecurity"
              checked={form.imapSecurity === opt.value}
              onChange={() => setImapSecurity(opt.value)}
            />
            <span>
              <strong>{opt.label}</strong>
              <em>{opt.hint}</em>
            </span>
          </label>
        ))}
      </fieldset>

      <h2 className="form-section">SMTP</h2>
      <div className="grid">
        <label>
          Host
          <input
            value={form.smtpHost}
            onChange={(e) => set("smtpHost", e.target.value)}
            required
            placeholder="smtp.example.com"
          />
        </label>
        <label>
          Port
          <input
            type="number"
            value={form.smtpPort}
            onChange={(e) => set("smtpPort", Number(e.target.value))}
            required
          />
        </label>
      </div>
      <fieldset className="tls-fieldset">
        <legend>SMTP security</legend>
        {TLS_MODE_OPTIONS.map((opt) => (
          <label key={opt.value} className="radio-row">
            <input
              type="radio"
              name="smtpSecurity"
              checked={form.smtpSecurity === opt.value}
              onChange={() => setSmtpSecurity(opt.value)}
            />
            <span>
              <strong>{opt.label}</strong>
              <em>{opt.hint}</em>
            </span>
          </label>
        ))}
      </fieldset>

      <label className="check-row">
        <input
          type="checkbox"
          checked={form.tlsInsecure}
          onChange={(e) => set("tlsInsecure", e.target.checked)}
        />
        <span>
          Allow insecure TLS
          <em>Skip certificate verification (self-signed / lab servers)</em>
        </span>
      </label>

      <p className="hint">OAuth is stubbed for later — use an app password for Gmail/Outlook today.</p>
      {error && <p className="error">{error}</p>}
      <div className="modal-actions">
        {onCancel ? (
          <button type="button" onClick={onCancel}>
            Cancel
          </button>
        ) : null}
        <button type="submit" disabled={busy}>
          {busy ? "Saving…" : submitLabel}
        </button>
      </div>
    </form>
  );
}
