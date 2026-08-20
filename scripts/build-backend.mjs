#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const outDir = path.join(root, "assets");
fs.mkdirSync(outDir, { recursive: true });

const goBin = process.env.GO || "go";
const goos =
  process.env.GOOS ||
  (process.platform === "win32" ? "windows" : process.platform === "darwin" ? "darwin" : "linux");
const name = goos === "windows" ? "email-backend.exe" : "email-backend";
const out = path.join(outDir, name);

const env = {
  ...process.env,
  PATH: `/usr/local/go/bin:${process.env.PATH || ""}`,
  CGO_ENABLED: "0",
  GOOS: goos,
};
if (process.env.GOARCH) {
  env.GOARCH = process.env.GOARCH;
}

for (const stale of ["email-backend", "email-backend.exe"]) {
  const p = path.join(outDir, stale);
  if (p !== out && fs.existsSync(p)) fs.unlinkSync(p);
}

const build = spawnSync(goBin, ["build", "-ldflags=-s -w", "-o", out, "./cmd/native"], {
  cwd: path.join(root, "backend"),
  env,
  encoding: "utf8",
});

if (build.status !== 0) {
  console.error(build.stderr || build.stdout);
  process.exit(build.status ?? 1);
}

console.log("wrote", out, `(${fs.statSync(out).size} bytes)`);
