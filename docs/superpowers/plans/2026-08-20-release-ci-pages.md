# Release CI, installers, and GitHub Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Electron mail client packagable, publish signed-off unsigned installers from GitHub Actions for macOS arm64, Windows x64, and Linux (deb/rpm/pacman/AppImage), and deploy a Pages site whose download links CI rewrites after each release.

**Architecture:** electron-builder extraResources ship the Go sidecar and C WASM. A matrix workflow builds on macos-14 / windows-latest / ubuntu-22.04, then a publish job creates a GitHub Release and a pages job writes `website/latest.json` from release assets and deploys.

**Tech Stack:** Electron 37, electron-builder 26, GitHub Actions, static HTML/CSS/JS Pages site, Go 1.24, wasi-sdk 25.

## Global Constraints

- Linux package name is `emails` (no spaces).
- Artifact names: `Email-${version}-${os}-${arch}.${ext}`.
- Apple Silicon only (no Intel Mac). Unsigned builds; `CSC_IDENTITY_AUTO_DISCOVERY=false`.
- Ubuntu CI uses apt package `rpm`, not `rpm-build`.
- Pages source is GitHub Actions; `latest.json` keys are `mac`, `win`, `deb`, `rpm`, `pacman`, `appimage`.
- Packaged app does not include the Go-WASM PocketBase build.

---

### Task 1: Production Electron paths

**Files:**
- Modify: `electron/main.ts`
- Modify: `electron/backend-host.ts`
- Modify: `vite.config.ts`
- Modify: `scripts/build-backend.mjs`
- Modify: `scripts/build-c.mjs`

**Interfaces:**
- Consumes: `app.isPackaged`, `process.resourcesPath`
- Produces: packaged `loadFile(dist/index.html)`, sidecar `email-backend.exe` on Windows, assets from `process.resourcesPath/assets`

- [ ] Point main at `loadFile` when packaged; resolve assets from `process.resourcesPath`.
- [ ] Windows sidecar name + skip `lsof`; spawn `cwd` = data dir.
- [ ] Vite production `base: './'`.
- [ ] `build-backend.mjs` honors `GOOS`/`GOARCH` and writes `.exe` on Windows.
- [ ] `build-c.mjs` finds wasi-sdk clang on linux/windows including `clang.exe`.

### Task 2: electron-builder + stage scripts

**Files:**
- Create: `electron-builder.yml`
- Create: `scripts/stage-backend.sh`
- Create: `scripts/install-wasi-sdk.sh`
- Modify: `package.json`
- Modify: `.gitignore`

- [ ] Add electron-builder 26 config matching the spec artifact names.
- [ ] Stage script writes `assets/email-backend` or `.exe`.
- [ ] WASI SDK installer for darwin/linux/windows 25.0.
- [ ] `npm run package` = native + backend + ui + electron-builder.
- [ ] Ignore `release/`.

### Task 3: CI workflows + latest.json writer

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`
- Create: `scripts/write-latest-json.mjs`

- [ ] PR/main CI: typecheck + native WASM + Go sidecar on Ubuntu.
- [ ] Release matrix + publish + pages deploy.
- [ ] `write-latest-json.mjs` maps release assets into `latest.json`.

### Task 4: GitHub Pages site

**Files:**
- Create: `website/index.html`
- Create: `website/styles.css`
- Create: `website/app.js`
- Create: `website/latest.json`
- Modify: `README.md`

- [ ] Static site: what / how / downloads from `latest.json`.
- [ ] README: tag `v*`, Pages URL, unsigned install notes.

### Task 5: Verify, merge, tag, watch CI

- [ ] `npm run typecheck` and a local `electron-builder --dir` smoke if feasible.
- [ ] Commit, push branch, open PR, merge to main.
- [ ] Tag `v0.1.0`, watch release workflow until green, confirm Pages deploy.
