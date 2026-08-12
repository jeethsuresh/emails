/// <reference types="vite/client" />

import type { SyncStatus } from "../shared/types";

export {};

declare global {
  interface Window {
    email: {
      pbFetch: (input: {
        method: string;
        url: string;
        headers: [string, string][];
        body?: ArrayBuffer | null;
      }) => Promise<{
        status: number;
        statusText: string;
        headers: [string, string][];
        body: ArrayBuffer;
      }>;
      triggerSync: () => Promise<{ ok: boolean }>;
      wipeMail: () => Promise<{ ok: boolean }>;
      fetchMessageBody: (messageId: string) => Promise<{
        status: number;
        statusText: string;
        headers: [string, string][];
        body: ArrayBuffer;
      }>;
      getSyncStatus: () => Promise<SyncStatus>;
      saveAccount: (account: Record<string, unknown>) => Promise<unknown>;
      onSyncStatus: (cb: (status: SyncStatus) => void) => () => void;
    };
  }
}
