#!/usr/bin/env node
import { classifyAsset, latestFromRelease } from "./write-latest-json.mjs";

const keys = [
  "Email-0.1.0-mac-arm64.dmg",
  "Email-0.1.0-win-x64.exe",
  "Email-0.1.0-linux-x64.deb",
  "Email-0.1.0-linux-x64.rpm",
  "Email-0.1.0-linux-x64.pacman",
  "Email-0.1.0-linux-x64.AppImage",
  "Email-0.1.0-win-x64.exe.blockmap",
];
const got = keys.map(classifyAsset);
const expected = ["mac", "win", "deb", "rpm", "pacman", "appimage", null];
if (JSON.stringify(got) !== JSON.stringify(expected)) {
  throw new Error(`classifyAsset mismatch: ${JSON.stringify(got)}`);
}
const latest = latestFromRelease({
  tag_name: "v0.1.0",
  published_at: "2026-08-20T00:00:00Z",
  html_url: "https://github.com/jeethsuresh/emails/releases/tag/v0.1.0",
  assets: keys.map((name) => ({ name, browser_download_url: `https://example/${name}` })),
});
if (!latest.downloads.mac || !latest.downloads.appimage || latest.downloads.win == null) {
  throw new Error("latest.json mapping failed");
}
if (latest.downloads.mac.filename !== "Email-0.1.0-mac-arm64.dmg") {
  throw new Error("mac filename mismatch");
}
console.log("write-latest-json ok");
