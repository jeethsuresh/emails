import { app, BrowserWindow, ipcMain } from "electron";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { ensureDataDirs } from "./bridges/fs";
import { BackendHost } from "./backend-host";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.join(__dirname, "..");

// Inbox can be thousands of light rows; give the renderer headroom past Chromium defaults.
app.commandLine.appendSwitch("js-flags", "--max-old-space-size=4096");

let mainWindow: BrowserWindow | null = null;
let backend: BackendHost | null = null;

function sendStatus(status: unknown) {
  const wc = mainWindow?.webContents;
  if (!wc || wc.isDestroyed()) return;
  wc.send("sync:status", status);
}

async function createWindow() {
  const dataDir = ensureDataDirs(app.getPath("userData"));

  backend = new BackendHost({
    dataDir,
    assetsDir: path.join(repoRoot, "assets"),
    repoRoot,
  });
  await backend.start();

  const preloadPath = path.join(__dirname, "preload.cjs");
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 840,
    minWidth: 900,
    minHeight: 600,
    title: "Email",
    backgroundColor: "#f3efe6",
    webPreferences: {
      preload: preloadPath,
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });

  mainWindow.webContents.on("console-message", (_e, level, message, line, sourceId) => {
    console.log(`[renderer:${level}] ${message} (${sourceId}:${line})`);
  });
  mainWindow.webContents.on("did-fail-load", (_e, code, desc, url) => {
    console.error("did-fail-load", { code, desc, url });
  });
  mainWindow.webContents.on("did-finish-load", () => {
    void mainWindow?.webContents
      .executeJavaScript(
        `({
          href: location.href,
          email: typeof window.email,
          root: document.getElementById('root')?.innerHTML?.slice(0, 300) ?? null
        })`,
      )
      .then((info) => console.log("renderer probe", info))
      .catch((err) => console.error("renderer probe failed", err));
  });

  backend.onStatus((status) => {
    // Don't push giant log buffers into the mail shell on every tick.
    const { logs: _logs, ...lite } = status;
    sendStatus({ ...lite, logs: [] });
  });

  const devUrl = process.env.VITE_DEV_SERVER_URL ?? "http://localhost:5173/";
  console.log("loading UI", { devUrl, preloadPath });
  await mainWindow.loadURL(devUrl);
  mainWindow.webContents.openDevTools({ mode: "detach" });
}

function registerIpc() {
  ipcMain.handle("pb:fetch", async (_event, input: {
    method: string;
    url: string;
    headers: [string, string][];
    body?: ArrayBuffer | null;
  }) => {
    if (!backend) throw new Error("backend not ready");
    return backend.fetch(input);
  });

  ipcMain.handle("sync:trigger", async () => {
    if (!backend) throw new Error("backend not ready");
    return backend.triggerSync();
  });

  ipcMain.handle("mail:wipe", async () => {
    if (!backend) throw new Error("backend not ready");
    return backend.wipeMail();
  });

  ipcMain.handle("mail:fetchBody", async (_event, messageId: string) => {
    if (!backend) throw new Error("backend not ready");
    return backend.fetchMessageBody(messageId);
  });

  ipcMain.handle("sync:getStatus", async () => {
    if (!backend) throw new Error("backend not ready");
    return backend.getStatus();
  });

  ipcMain.handle("account:save", async (_event, account: Record<string, unknown>) => {
    if (!backend) throw new Error("backend not ready");
    try {
      return await backend.saveAccount(account);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(msg);
    }
  });
}

app.whenReady().then(async () => {
  registerIpc();
  await createWindow();

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) void createWindow();
  });
});

app.on("window-all-closed", () => {
  backend?.stop();
  if (process.platform !== "darwin") app.quit();
});
