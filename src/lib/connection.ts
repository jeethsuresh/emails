export type TlsMode = "tls" | "starttls" | "none";

export interface ConnectionSettings {
  id?: string;
  email: string;
  username: string;
  password: string;
  imapHost: string;
  imapPort: number;
  imapSecurity: TlsMode;
  smtpHost: string;
  smtpPort: number;
  smtpSecurity: TlsMode;
  tlsInsecure: boolean;
}

export const TLS_MODE_OPTIONS: { value: TlsMode; label: string; hint: string }[] = [
  {
    value: "tls",
    label: "Implicit TLS",
    hint: "TLS from connect (IMAP 993 / SMTP 465)",
  },
  {
    value: "starttls",
    label: "STARTTLS",
    hint: "Plain then upgrade (IMAP 143/1143 · SMTP 587/1025 — Proton Bridge)",
  },
  {
    value: "none",
    label: "None (insecure)",
    hint: "No encryption — local/dev only",
  },
];

export function defaultPorts(kind: "imap" | "smtp", mode: TlsMode): number {
  if (kind === "imap") {
    switch (mode) {
      case "tls":
        return 993;
      case "starttls":
        return 143;
      case "none":
        return 143;
      default: {
        const _exhaustive: never = mode;
        return _exhaustive;
      }
    }
  }
  switch (mode) {
    case "tls":
      return 465;
    case "starttls":
      return 587;
    case "none":
      return 25;
    default: {
      const _exhaustive: never = mode;
      return _exhaustive;
    }
  }
}

/** Sensible empty form — prefer STARTTLS so local bridges aren't one click away. */
export function emptyConnection(): ConnectionSettings {
  return {
    email: "",
    username: "",
    password: "",
    imapHost: "127.0.0.1",
    imapPort: 1143,
    imapSecurity: "starttls",
    smtpHost: "127.0.0.1",
    smtpPort: 1025,
    smtpSecurity: "starttls",
    tlsInsecure: true,
  };
}

/** Normalize a PocketBase accounts record into ConnectionSettings. */
export function fromAccountRecord(rec: Record<string, unknown>): ConnectionSettings {
  const imapSec = normalizeTlsMode(
    rec.imap_security ?? rec.imapSecurity,
    rec.imap_tls ?? rec.imapTLS,
  );
  const smtpSec = normalizeTlsMode(
    rec.smtp_security ?? rec.smtpSecurity,
    rec.smtp_tls ?? rec.smtpTLS,
  );
  return {
    id: typeof rec.id === "string" ? rec.id : undefined,
    email: String(rec.email ?? ""),
    username: String(rec.username ?? rec.email ?? ""),
    password: String(rec.password ?? ""),
    imapHost: String(rec.imap_host ?? rec.imapHost ?? ""),
    imapPort: Number(rec.imap_port ?? rec.imapPort ?? defaultPorts("imap", imapSec)),
    imapSecurity: imapSec,
    smtpHost: String(rec.smtp_host ?? rec.smtpHost ?? ""),
    smtpPort: Number(rec.smtp_port ?? rec.smtpPort ?? defaultPorts("smtp", smtpSec)),
    smtpSecurity: smtpSec,
    tlsInsecure: Boolean(rec.tls_insecure ?? rec.tlsInsecure ?? false),
  };
}

function normalizeTlsMode(security: unknown, legacyBool: unknown): TlsMode {
  if (security === "tls" || security === "starttls" || security === "none") {
    return security;
  }
  if (legacyBool === false || legacyBool === "false" || legacyBool === 0) {
    return "none";
  }
  return "tls";
}

export function toAccountPayload(c: ConnectionSettings) {
  return {
    email: c.email,
    username: c.username || c.email,
    password: c.password,
    imapHost: c.imapHost,
    imapPort: c.imapPort,
    imapSecurity: c.imapSecurity,
    imapTLS: c.imapSecurity === "tls", // legacy mirror
    smtpHost: c.smtpHost,
    smtpPort: c.smtpPort,
    smtpSecurity: c.smtpSecurity,
    smtpTLS: c.smtpSecurity === "tls",
    tlsInsecure: c.tlsInsecure,
  };
}
