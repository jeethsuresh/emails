#!/usr/bin/env node
/**
 * Map a GitHub Release's assets into website/latest.json.
 *
 * Usage:
 *   node scripts/write-latest-json.mjs <tag> [outPath]
 *   node scripts/write-latest-json.mjs --from-json <release.json> [outPath]
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const LABELS = {
  mac: "macOS (Apple Silicon)",
  win: "Windows",
  deb: "Ubuntu / Debian",
  rpm: "Fedora / RHEL",
  pacman: "Arch / Manjaro",
  appimage: "Linux AppImage",
};

export function classifyAsset(name) {
  const lower = name.toLowerCase();
  if (
    lower.endsWith(".blockmap") ||
    lower.endsWith(".yml") ||
    lower.endsWith(".yaml") ||
    lower.endsWith(".zip")
  ) {
    return null;
  }
  if (lower.endsWith(".dmg")) return "mac";
  if (lower.endsWith(".exe")) return "win";
  if (lower.endsWith(".deb")) return "deb";
  if (lower.endsWith(".rpm")) return "rpm";
  if (lower.endsWith(".pacman") || lower.endsWith(".pkg.tar.zst")) return "pacman";
  if (lower.endsWith(".appimage")) return "appimage";
  return null;
}

export function latestFromRelease(release) {
  const downloads = {};
  for (const asset of release.assets ?? []) {
    const key = classifyAsset(asset.name);
    if (!key) continue;
    downloads[key] = {
      label: LABELS[key],
      url: asset.browser_download_url,
      filename: asset.name,
    };
  }
  return {
    version: release.tag_name ?? null,
    publishedAt: release.published_at ?? null,
    releaseUrl: release.html_url ?? "https://github.com/jeethsuresh/emails/releases",
    downloads,
  };
}

function isMain() {
  const self = fileURLToPath(import.meta.url);
  const invoked = process.argv[1] && path.resolve(process.argv[1]);
  return invoked === self;
}

async function fetchRelease(tag) {
  const repo = process.env.GITHUB_REPOSITORY || "jeethsuresh/emails";
  const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN || "";
  const url = `https://api.github.com/repos/${repo}/releases/tags/${encodeURIComponent(tag)}`;
  const headers = {
    Accept: "application/vnd.github+json",
    "User-Agent": "emails-release-ci",
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(url, { headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`GitHub API ${res.status} ${url}: ${body}`);
  }
  return res.json();
}

async function main() {
  const args = process.argv.slice(2);
  let release;
  let outArg;
  if (args[0] === "--from-json") {
    const src = args[1];
    if (!src) {
      console.error("usage: write-latest-json.mjs --from-json <release.json> [outPath]");
      process.exit(2);
    }
    release = JSON.parse(fs.readFileSync(src, "utf8"));
    outArg = args[2];
  } else {
    const tag = args[0];
    if (!tag) {
      console.error("usage: write-latest-json.mjs <tag> [outPath]");
      process.exit(2);
    }
    release = await fetchRelease(tag);
    outArg = args[1];
  }
  const out = path.resolve(outArg || path.join(root, "website/latest.json"));
  const latest = latestFromRelease(release);
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, JSON.stringify(latest, null, 2) + "\n");
  console.log("wrote", out, "version", latest.version);
}

if (isMain()) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
