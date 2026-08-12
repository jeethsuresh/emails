#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const outDir = path.join(root, "assets");
fs.mkdirSync(outDir, { recursive: true });

const src = path.join(root, "native/email_core.c");
const out = path.join(outDir, "email_core.wasm");

function findClang() {
  const home = os.homedir();
  const candidates = [
    process.env.WASI_SDK_PATH && path.join(process.env.WASI_SDK_PATH, "bin/clang"),
    path.join(home, ".local/wasi-sdk-25.0-arm64-macos/bin/clang"),
    path.join(home, ".local/wasi-sdk-25.0-x86_64-macos/bin/clang"),
    "clang",
  ].filter(Boolean);
  for (const c of candidates) {
    if (c === "clang") return c;
    if (fs.existsSync(c)) return c;
  }
  return "clang";
}

const clang = findClang();
const r = spawnSync(
  clang,
  [
    "--target=wasm32",
    "-O2",
    "-nostdlib",
    "-Wl,--no-entry",
    "-Wl,--export-memory",
    "-Wl,--export=alloc",
    "-Wl,--export=dealloc",
    "-Wl,--export=mime_header_get",
    "-Wl,--export=hash_blake_like",
    "-Wl,--export=index_tokenize",
    "-Wl,--export=contact_normalize",
    "-o",
    out,
    src,
  ],
  { encoding: "utf8" },
);

if (r.status !== 0) {
  console.error(r.stderr || r.stdout);
  console.error(
    "\nHint: install wasi-sdk and set WASI_SDK_PATH, or place it under ~/.local/wasi-sdk-25.0-*-macos",
  );
  process.exit(r.status ?? 1);
}

console.log("wrote", out, `(${fs.statSync(out).size} bytes) via ${clang}`);
