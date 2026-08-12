# Email — Design Spec (2026-08-11)

## Goal

Desktop mail client: Electron UI + PocketBase (Go→WASM) as offline source of truth + Go IMAP/SMTP sync + C hot paths (parse/search/crypto/index).

## Stack

- Electron main loads Go WASM (PocketBase + sync) and C WASM (`email_core`)
- Renderer: React + TypeScript + Vite; talks to PB via IPC→in-process HTTP handler
- Auth v1: IMAP/SMTP password (OAuth stubbed)
- Path: `~/projects/email`

## Architecture

```
Renderer (React) --IPC--> Main
                          ├─ Go WASM: PocketBase + IMAP sync loop
                          ├─ C WASM: MIME parse, FTS stubs, crypto helpers
                          ├─ net bridge: real TCP for IMAP/SMTP
                          └─ fs: userData/pb_data + attachments
```

UI reads/writes only through PocketBase. Sync is best-effort; status: `idle | syncing | error | offline`.

## v1 (scope B)

One account, folders, attachments, search, star/unread, drafts, password login, sync status.

## Later (scaffold TODOs)

Multi-account, contacts graph UI, offline compose queue, OAuth.

## Data (PB collections)

`accounts`, `folders`, `messages`, `attachments`, `drafts`, `sync_meta`

## PocketBase-on-WASM

Use custom `DBConnect` with `ncruces/go-sqlite3` + replace `modernc.org/sqlite` with empty stub (proven pattern). SQLite runs on the `memdb` VFS (OS disk I/O is unreliable under `GOOS=js`); Electron host loads/saves DB snapshots under `userData`. Serve via in-process handler (no listen); Electron IPC fetch shim. Net: custom `net.Conn` over main-process TCP.

## Error / offline

Local PB always works. Sync failures set status `error` with message; no UI blocking. Missing network → `offline`, resume on reconnect.
