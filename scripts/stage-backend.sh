#!/usr/bin/env bash
# Build and stage the Go backend binary electron-builder expects:
#   assets/email-backend      (unix)
#   assets/email-backend.exe  (windows)
#
# Usage:
#   ./scripts/stage-backend.sh <goos> <goarch>
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/assets"
GOOS="${1:?goos required}"
GOARCH="${2:?goarch required}"

mkdir -p "$OUT"
rm -f "$OUT/email-backend" "$OUT/email-backend.exe"

if [[ "$GOOS" == "windows" ]]; then
  NAME="email-backend.exe"
else
  NAME="email-backend"
fi

echo "building $NAME ($GOOS/$GOARCH)…"
(
  cd "$ROOT/backend"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags="-s -w" -o "$OUT/$NAME" ./cmd/native
)

ls -la "$OUT/$NAME"
