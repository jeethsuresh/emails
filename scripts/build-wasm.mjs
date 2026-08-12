#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const outDir = path.join(root, "assets");
fs.mkdirSync(outDir, { recursive: true });

const goBin = process.env.GO || "go";
const env = {
  ...process.env,
  PATH: `/usr/local/go/bin:${process.env.PATH || ""}`,
  GOOS: "js",
  GOARCH: "wasm",
};

const build = spawnSync(
  goBin,
  [
    "build",
    "-tags",
    "no_default_driver",
    "-o",
    path.join(outDir, "email.wasm"),
    "./cmd/wasm",
  ],
  {
    cwd: path.join(root, "backend"),
    env,
    encoding: "utf8",
  },
);

if (build.status !== 0) {
  console.error(build.stderr || build.stdout);
  process.exit(build.status ?? 1);
}

// Copy wasm_exec.js from Go toolchain
const goroot = spawnSync(goBin, ["env", "GOROOT"], {
  env: { ...process.env, PATH: env.PATH },
  encoding: "utf8",
});
const rootGo = (goroot.stdout || "").trim();
const candidates = [
  path.join(rootGo, "lib/wasm/wasm_exec.js"),
  path.join(rootGo, "misc/wasm/wasm_exec.js"),
];
const wasmExec = candidates.find((p) => fs.existsSync(p));
if (!wasmExec) {
  console.error("wasm_exec.js not found under", rootGo);
  process.exit(1);
}
fs.copyFileSync(wasmExec, path.join(outDir, "wasm_exec.js"));

const st = fs.statSync(path.join(outDir, "email.wasm"));
console.log("wrote assets/email.wasm", `(${st.size} bytes)`);
console.log("wrote assets/wasm_exec.js");
