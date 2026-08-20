#!/usr/bin/env bash
# Install wasi-sdk 25.0 into ~/.local/wasi-sdk-25.0-<slug> and print WASI_SDK_PATH.
set -euo pipefail

VERSION="${WASI_SDK_VERSION:-25.0}"
TAG="wasi-sdk-${VERSION%%.*}"
PREFIX="${WASI_SDK_PREFIX:-$HOME/.local}"

uname_s="$(uname -s)"
uname_m="$(uname -m)"
case "$uname_s" in
  Darwin)
    case "$uname_m" in
      arm64) SLUG="arm64-macos" ;;
      *) SLUG="x86_64-macos" ;;
    esac
    ;;
  Linux)
    case "$uname_m" in
      aarch64 | arm64) SLUG="arm64-linux" ;;
      *) SLUG="x86_64-linux" ;;
    esac
    ;;
  MINGW* | MSYS* | CYGWIN*)
    SLUG="x86_64-windows"
    ;;
  *)
    echo "unsupported OS: $uname_s" >&2
    exit 1
    ;;
esac

NAME="wasi-sdk-${VERSION}-${SLUG}"
DEST="$PREFIX/$NAME"
URL="https://github.com/WebAssembly/wasi-sdk/releases/download/${TAG}/${NAME}.tar.gz"

clang_bin() {
  if [[ -x "$DEST/bin/clang" ]]; then
    echo "$DEST/bin/clang"
  elif [[ -x "$DEST/bin/clang.exe" ]]; then
    echo "$DEST/bin/clang.exe"
  else
    return 1
  fi
}

if clang_bin >/dev/null 2>&1; then
  echo "wasi-sdk already present at $DEST"
else
  mkdir -p "$PREFIX"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "downloading $URL"
  curl -fsSL "$URL" | tar -xz -C "$tmp"
  found="$(find "$tmp" \( -name clang -o -name clang.exe \) | head -n 1)"
  if [[ -z "$found" ]]; then
    echo "clang not found in wasi-sdk tarball" >&2
    exit 1
  fi
  sdk_root="$(cd "$(dirname "$found")/.." && pwd)"
  rm -rf "$DEST"
  mv "$sdk_root" "$DEST"
fi

echo "WASI_SDK_PATH=$DEST"
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "WASI_SDK_PATH=$DEST" >> "$GITHUB_ENV"
fi
