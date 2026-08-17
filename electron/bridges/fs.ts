import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/** Stable on-disk root for mail/calendar local state (outside the repo / Electron cache). */
export function defaultDataRoot(): string {
  const fromEnv = process.env.EMAIL_HOME?.trim();
  if (fromEnv) return path.resolve(fromEnv);
  return path.join(os.homedir(), ".emails");
}

export function ensureDataDirs(root = defaultDataRoot()) {
  const resolved = path.resolve(root);
  const pbData = path.join(resolved, "pb_data");
  const attachments = path.join(resolved, "attachments");
  const index = path.join(resolved, "index");
  for (const dir of [resolved, pbData, attachments, index]) {
    fs.mkdirSync(dir, { recursive: true });
  }
  return { root: resolved, pbData, attachments, index };
}

/**
 * If ~/.emails is empty but a previous Electron userData tree exists, copy it once
 * so accounts/mail survive the path change.
 */
export function migrateLegacyDataDirs(legacyUserData: string, dest: DataDirs): void {
  const candidates = [
    path.join(legacyUserData, "email", "pb_data"),
    path.join(legacyUserData, "pb_data"),
    // Common Electron default for this app name on macOS.
    path.join(os.homedir(), "Library", "Application Support", "email", "email", "pb_data"),
    path.join(os.homedir(), "Library", "Application Support", "email", "pb_data"),
  ];

  const legacyPb = candidates.find((p) => fs.existsSync(path.join(p, "data.db")));
  if (!legacyPb) {
    console.log("no legacy email pb_data found to migrate");
    return;
  }

  const destDb = path.join(dest.pbData, "data.db");
  if (fs.existsSync(destDb)) {
    const destSize = fs.statSync(destDb).size;
    const legacySize = fs.statSync(path.join(legacyPb, "data.db")).size;
    // Keep dest if it already looks populated; otherwise replace tiny fresh DBs.
    if (destSize > 100_000 || destSize >= legacySize) {
      console.log("keeping existing ~/.emails data", { destSize, legacySize });
      return;
    }
  }

  const legacyRoot = path.dirname(legacyPb);
  console.log("migrating legacy email data", { from: legacyRoot, to: dest.root });
  // Replace pb_data/attachments/index from the legacy tree.
  for (const name of ["pb_data", "attachments", "index"] as const) {
    const from = path.join(legacyRoot, name);
    const to = path.join(dest.root, name);
    if (!fs.existsSync(from)) continue;
    fs.rmSync(to, { recursive: true, force: true });
    copyDirRecursive(from, to);
  }
}
function copyDirRecursive(src: string, dest: string) {
  fs.mkdirSync(dest, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const from = path.join(src, entry.name);
    const to = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDirRecursive(from, to);
    } else if (entry.isFile()) {
      fs.copyFileSync(from, to);
    }
  }
}

export type DataDirs = ReturnType<typeof ensureDataDirs>;
