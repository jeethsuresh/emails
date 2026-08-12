# Email

Offline-first desktop mail client.

- **Electron** shell (React + TypeScript + Vite)
- **PocketBase** (Go → WASM) as local source of truth
- **Go IMAP sync** over an Electron TCP bridge
- **C → WASM** hot paths (`native/email_core.c`) for MIME/search/hash/contacts

## Prerequisites

- Go 1.24+
- Node 22+ (`~/.local/node` or system)
- [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) clang for C→WASM (default path `~/.local/wasi-sdk-25.0-arm64-macos`, or set `WASI_SDK_PATH`)

## Setup

```bash
export PATH="$HOME/.local/node/bin:/usr/local/go/bin:$PATH"
cd ~/projects/email
npm install
npm run dev
```

`npm run dev` builds C WASM, Go WASM, then starts Vite + Electron.

## Layout

| Path | Role |
|------|------|
| `electron/` | Main process, IPC, TCP/fs bridges, WASM host |
| `backend/` | PocketBase + IMAP syncer (WASM) |
| `native/` | C hot-path library |
| `src/` | React UI |
| `docs/superpowers/specs/` | Design spec |

## Notes

- Dev uses a **native Go PocketBase sidecar** (`assets/email-backend`) — full PB-in-WASM hangs on nested SQLite/wazero under `GOOS=js`; the WASM build remains available via `npm run build:wasm`
- v1 auth is IMAP/SMTP password (OAuth stubbed — see `TODO.md`)
- Compose saves drafts locally; SMTP send + offline queue are phase-C
- The JS client talks to PocketBase through Electron IPC (proxied to `127.0.0.1:8090`)
- C hot paths still load as WASM (`email_core.wasm`)
