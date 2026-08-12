import { spawn, execSync, type ChildProcessWithoutNullStreams } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import type { DataDirs } from "./bridges/fs";
import type { SyncStatus } from "../shared/types";
import { loadEmailCore } from "./bridges/email-core";

export interface BackendHostOptions {
  dataDir: DataDirs;
  assetsDir: string;
  repoRoot: string;
}

const emptyStatus = (): SyncStatus => ({
  state: "idle",
  message: "Starting backend…",
  phase: "boot",
  currentFolder: "",
  lastSyncAt: null,
  foldersSynced: 0,
  foldersTotal: 0,
  messagesSynced: 0,
  logs: [],
});

/**
 * Hosts the native Go PocketBase+IMAP backend as a child process.
 * Live sync progress arrives via EMAIL_STATUS JSON lines on stdout (+ optional poll).
 */
export class BackendHost {
  private opts: BackendHostOptions;
  private child: ChildProcessWithoutNullStreams | null = null;
  private baseURL = "http://127.0.0.1:8090";
  private status: SyncStatus = emptyStatus();
  private statusListeners = new Set<(s: SyncStatus) => void>();
  private pollTimer: ReturnType<typeof setInterval> | null = null;
  private stdoutBuf = "";
  private starting: Promise<void> | null = null;

  constructor(opts: BackendHostOptions) {
    this.opts = opts;
  }

  onStatus(cb: (s: SyncStatus) => void) {
    this.statusListeners.add(cb);
    cb(this.status);
    return () => this.statusListeners.delete(cb);
  }

  private setStatus(partial: Partial<SyncStatus>) {
    this.status = {
      ...this.status,
      ...partial,
      logs: partial.logs ?? this.status.logs,
    };
    this.queueStatusNotify();
  }

  /** Coalesce rapid EMAIL_STATUS + poll updates so the renderer is not flooded. */
  private pendingNotify: SyncStatus | null = null;
  private notifyTimer: ReturnType<typeof setTimeout> | null = null;

  private queueStatusNotify() {
    this.pendingNotify = this.status;
    if (this.notifyTimer) return;
    this.notifyTimer = setTimeout(() => {
      this.notifyTimer = null;
      const snap = this.pendingNotify;
      this.pendingNotify = null;
      if (!snap) return;
      for (const cb of this.statusListeners) cb(snap);
    }, 200);
  }

  private ingestStatusLine(line: string) {
    const marker = "EMAIL_STATUS:";
    const idx = line.indexOf(marker);
    if (idx < 0) return;
    try {
      const parsed = JSON.parse(line.slice(idx + marker.length)) as SyncStatus;
      this.setStatus({
        state: parsed.state ?? this.status.state,
        message: parsed.message ?? this.status.message,
        phase: parsed.phase ?? this.status.phase,
        currentFolder: parsed.currentFolder ?? "",
        lastSyncAt: parsed.lastSyncAt ?? this.status.lastSyncAt,
        foldersSynced: parsed.foldersSynced ?? this.status.foldersSynced,
        foldersTotal: parsed.foldersTotal ?? this.status.foldersTotal,
        messagesSynced: parsed.messagesSynced ?? this.status.messagesSynced,
        logs: Array.isArray(parsed.logs) ? parsed.logs : this.status.logs,
      });
    } catch (err) {
      console.warn("bad EMAIL_STATUS line", err);
    }
  }

  async start() {
    if (this.starting) return this.starting;
    this.starting = this.startInner().finally(() => {
      this.starting = null;
    });
    return this.starting;
  }

  private async startInner() {
    await loadEmailCore(path.join(this.opts.assetsDir, "email_core.wasm"));

    const bin = path.join(this.opts.assetsDir, "email-backend");
    if (!fs.existsSync(bin)) {
      throw new Error(`missing ${bin} — run npm run build:backend`);
    }

    // Vite/Electron reloads can orphan prior sidecars still bound to 8090.
    freeListenPort(8090);

    this.child = spawn(bin, [], {
      env: {
        ...process.env,
        EMAIL_DATA_DIR: this.opts.dataDir.pbData,
        EMAIL_ADDR: "127.0.0.1:8090",
      },
      cwd: this.opts.repoRoot,
    });

    this.child.stdout.on("data", (buf: Buffer) => {
      const chunk = buf.toString();
      process.stdout.write(`[backend] ${chunk}`);
      this.stdoutBuf += chunk;
      let nl: number;
      while ((nl = this.stdoutBuf.indexOf("\n")) >= 0) {
        const line = this.stdoutBuf.slice(0, nl);
        this.stdoutBuf = this.stdoutBuf.slice(nl + 1);
        this.ingestStatusLine(line);
        if (line.includes("listening")) {
          this.setStatus({ state: "idle", message: "Backend ready", phase: "idle" });
        }
      }
    });
    this.child.stderr.on("data", (buf: Buffer) => {
      process.stderr.write(`[backend] ${buf.toString()}`);
    });
    this.child.on("exit", (code) => {
      this.child = null;
      this.setStatus({ state: "error", message: `Backend exited (${code})`, phase: "error" });
      if (this.pollTimer) clearInterval(this.pollTimer);
      this.pollTimer = null;
    });

    await waitForHTTP(`${this.baseURL}/api/collections/accounts/records`, 60_000);
    this.setStatus({ state: "idle", message: "Backend ready", phase: "idle" });

    // Fallback poll so UI stays live even if a status line is missed.
    if (this.pollTimer) clearInterval(this.pollTimer);
    this.pollTimer = setInterval(() => {
      void this.pollStatus();
    }, 750);
  }

  /** Restart the sidecar if it crashed or never came up. */
  async ensureAlive() {
    if (this.starting) {
      await this.starting;
      return;
    }
    if (this.child && this.child.exitCode == null) {
      try {
        await waitForHTTP(`${this.baseURL}/api/collections/accounts/records`, 1500);
        return;
      } catch {
        /* fall through and restart */
      }
    }
    this.stop();
    await this.start();
  }

  private async pollStatus() {
    try {
      const res = await fetch(`${this.baseURL}/api/email/sync/status`);
      if (!res.ok) return;
      const parsed = (await res.json()) as SyncStatus;
      this.setStatus({
        state: parsed.state,
        message: parsed.message,
        phase: parsed.phase,
        currentFolder: parsed.currentFolder ?? "",
        lastSyncAt: parsed.lastSyncAt,
        foldersSynced: parsed.foldersSynced,
        foldersTotal: parsed.foldersTotal,
        messagesSynced: parsed.messagesSynced,
        logs: parsed.logs ?? [],
      });
    } catch {
      /* backend still booting */
    }
  }

  stop() {
    if (this.notifyTimer) clearTimeout(this.notifyTimer);
    this.notifyTimer = null;
    this.pendingNotify = null;
    if (this.pollTimer) clearInterval(this.pollTimer);
    this.pollTimer = null;
    if (this.child) {
      this.child.removeAllListeners("exit");
      this.child.kill();
      this.child = null;
    }
  }

  async fetch(input: {
    method: string;
    url: string;
    headers: [string, string][];
    body?: ArrayBuffer | null;
  }) {
    await this.ensureAlive();
    const target = rewriteURL(input.url, this.baseURL);
    const body = input.body ? Buffer.from(new Uint8Array(input.body)) : undefined;
    try {
      const res = await fetch(target, {
        method: input.method,
        headers: Object.fromEntries(input.headers),
        body: input.method === "GET" || input.method === "HEAD" ? undefined : body,
      });
      const ab = await res.arrayBuffer();
      const headers: [string, string][] = [];
      res.headers.forEach((v, k) => headers.push([k, v]));
      return {
        status: res.status,
        statusText: res.statusText,
        headers,
        body: ab,
      };
    } catch (err) {
      const detail = err instanceof Error ? err.message : String(err);
      // One recovery attempt if the sidecar died mid-request.
      await this.ensureAlive();
      try {
        const res = await fetch(target, {
          method: input.method,
          headers: Object.fromEntries(input.headers),
          body: input.method === "GET" || input.method === "HEAD" ? undefined : body,
        });
        const ab = await res.arrayBuffer();
        const headers: [string, string][] = [];
        res.headers.forEach((v, k) => headers.push([k, v]));
        return {
          status: res.status,
          statusText: res.statusText,
          headers,
          body: ab,
        };
      } catch {
        throw new Error(
          `Local mail backend unreachable at ${this.baseURL} (${detail}). Restart the app if this persists.`,
        );
      }
    }
  }

  getStatus() {
    return this.status;
  }

  async triggerSync() {
    this.setStatus({ state: "syncing", message: "Sync requested…", phase: "start" });
    await this.fetch({
      method: "POST",
      url: "/api/email/sync",
      headers: [["Content-Type", "application/json"]],
      body: new TextEncoder().encode("{}").buffer,
    });
    return { ok: true };
  }

  async wipeMail() {
    this.setStatus({ state: "syncing", message: "Wiping local mail…", phase: "start" });
    await this.fetch({
      method: "POST",
      url: "/api/email/wipe",
      headers: [["Content-Type", "application/json"]],
      body: new TextEncoder().encode("{}").buffer,
    });
    return { ok: true };
  }

  async fetchMessageBody(messageId: string) {
    return this.fetch({
      method: "POST",
      url: `/api/email/messages/${encodeURIComponent(messageId)}/fetch-body`,
      headers: [["Content-Type", "application/json"]],
      body: new TextEncoder().encode("{}").buffer,
    });
  }

  async saveAccount(account: Record<string, unknown>) {
    this.setStatus({ state: "syncing", message: "Saving account…", phase: "start" });
    const body = new TextEncoder().encode(JSON.stringify(account));
    return this.fetch({
      method: "POST",
      url: "/api/email/account",
      headers: [["Content-Type", "application/json"]],
      body: body.buffer,
    });
  }
}

function rewriteURL(url: string, base: string) {
  if (url.startsWith("http://email.local")) {
    return url.replace("http://email.local", base);
  }
  if (url.startsWith("/")) return base + url;
  return url;
}

function freeListenPort(port: number) {
  try {
    const out = execSync(`lsof -tiTCP:${port} -sTCP:LISTEN`, {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (!out) return;
    for (const pid of out.split(/\s+/)) {
      try {
        process.kill(Number(pid), "SIGTERM");
      } catch {
        /* already gone */
      }
    }
    try {
      execSync("sleep 0.35", { stdio: "ignore" });
    } catch {
      /* ignore */
    }
  } catch {
    /* nothing listening */
  }
}

function waitForHTTP(url: string, timeoutMs: number) {
  const start = Date.now();
  return new Promise<void>((resolve, reject) => {
    const tick = () => {
      const req = http.get(url, (res) => {
        res.resume();
        resolve();
      });
      req.on("error", () => {
        if (Date.now() - start > timeoutMs) {
          reject(new Error(`backend not ready: ${url}`));
          return;
        }
        setTimeout(tick, 200);
      });
    };
    tick();
  });
}
