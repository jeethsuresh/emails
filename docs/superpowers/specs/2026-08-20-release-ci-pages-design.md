# Release CI, installers, and GitHub Pages — Design Spec (2026-08-20)

## Goal

Ship tagged versions of the Email desktop client as installers for macOS Apple Silicon, Windows x64, and Linux x64 (Ubuntu `.deb`, Fedora `.rpm`, Arch `.pacman`, plus AppImage). Host a GitHub Pages site that explains the app and links the latest artifacts. After each successful release, CI regenerates download metadata and redeploys the site.

## Production packaging

The app is currently dev-only (`loadURL` always hits Vite). Packaged builds must:

- Load `dist/index.html` when `app.isPackaged`; keep the Vite URL in development.
- Resolve the Go sidecar and `email_core.wasm` from `process.resourcesPath` when packaged.
- Name the Windows sidecar `email-backend.exe`.
- Skip `lsof` port cleanup on Windows.
- Use Vite `base: './'` in production so `file://` loads work.
- Spawn the sidecar with `cwd` set to a real directory (`~/.emails`), not an asar path.

`npm run package` builds C WASM + the native Go sidecar for the host OS + the Vite renderer/electron bundle, then runs electron-builder. Packaged apps do not need the Go-WASM PocketBase build.

## electron-builder

- `appId`: `com.jeeth.emails`
- `productName`: `Email`
- `executableName` / Linux package name: `emails` (no spaces)
- `artifactName`: `Email-${version}-${os}-${arch}.${ext}`
- extraResources: `assets/email-backend` (or `.exe`) and `assets/email_core.wasm`
- macOS: DMG, arm64 only, unsigned (`CSC_IDENTITY_AUTO_DISCOVERY=false`)
- Windows: NSIS x64, unsigned
- Linux: `deb`, `rpm`, `pacman`, `AppImage` on Ubuntu 22.04

Unsigned macOS: right-click → Open the first time. Windows: SmartScreen “Run anyway.”

## CI matrix

| Job | Runner | Artifacts |
|-----|--------|-----------|
| mac-arm64 | `macos-14` | `.dmg` |
| windows-x64 | `windows-latest` | NSIS `.exe` |
| linux-x64 | `ubuntu-22.04` | `.deb`, `.rpm`, `.pacman`, `.AppImage` |

Linux apt packages: `rpm` (not `rpm-build`), `libarchive-tools`, `libfuse2`, `fakeroot`. WASI SDK 25 is installed on every runner so `email_core.wasm` builds.

Triggers for `.github/workflows/release.yml`: push of `v*` tags, plus `workflow_dispatch` with an optional tag. `workflow_dispatch` without a tag uploads workflow artifacts only (no GitHub Release, no Pages update).

`.github/workflows/ci.yml` runs typecheck + native WASM + Go sidecar on pull requests and pushes to `main`.

## Pages site

Static files in `website/`. Visual language matches the app: warm paper background, Iowan/Palatino display, IBM Plex body, accent `#0f6e56`.

Content: what the app is, how local PocketBase + IMAP/SMTP sidecar works, install notes, latest-version download buttons from `latest.json`. Fallback link to GitHub Releases if a file is missing.

`latest.json` keys: `mac`, `win`, `deb`, `rpm`, `pacman`, `appimage`.

Repo Pages source must be **GitHub Actions**. Site URL: `https://jeethsuresh.github.io/emails/`.

## Out of scope

Code signing / notarization, auto-update, Intel Mac, Linux ARM, Snap/Flatpak, building on Fedora/Arch hosts.
