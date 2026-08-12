import fs from "node:fs";
import path from "node:path";

export function ensureDataDirs(userData: string) {
  const root = path.join(userData, "email");
  const pbData = path.join(root, "pb_data");
  const attachments = path.join(root, "attachments");
  const index = path.join(root, "index");
  for (const dir of [root, pbData, attachments, index]) {
    fs.mkdirSync(dir, { recursive: true });
  }
  return { root, pbData, attachments, index };
}

export type DataDirs = ReturnType<typeof ensureDataDirs>;
