import { contextBridge, ipcRenderer } from "electron";
import type { SyncStatus } from "../shared/types";

contextBridge.exposeInMainWorld("email", {
  pbFetch: (input: {
    method: string;
    url: string;
    headers: [string, string][];
    body?: ArrayBuffer | null;
  }) => ipcRenderer.invoke("pb:fetch", input),

  triggerSync: () => ipcRenderer.invoke("sync:trigger"),
  wipeMail: () => ipcRenderer.invoke("mail:wipe"),
  fetchMessageBody: (messageId: string) => ipcRenderer.invoke("mail:fetchBody", messageId),
  getSyncStatus: () => ipcRenderer.invoke("sync:getStatus") as Promise<SyncStatus>,
  saveAccount: (account: Record<string, unknown>) =>
    ipcRenderer.invoke("account:save", account),

  onSyncStatus: (cb: (status: SyncStatus) => void) => {
    const listener = (_: Electron.IpcRendererEvent, status: SyncStatus) => cb(status);
    ipcRenderer.on("sync:status", listener);
    return () => ipcRenderer.removeListener("sync:status", listener);
  },
});
