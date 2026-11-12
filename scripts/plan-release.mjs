import { appendFileSync, readFileSync } from "node:fs";

const { version } = JSON.parse(readFileSync(new URL("../package.json", import.meta.url)));
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) {
  throw new Error("Email release version must be stable X.Y.Z");
}
const tag = "v" + version;
let publish = false;
if (process.env.GITHUB_REF === "refs/heads/main" &&
    ["push", "workflow_dispatch"].includes(process.env.GITHUB_EVENT_NAME)) {
  const api = "https://api.github.com/repos/jeethsuresh/emails";
  const github = (path) => fetch(api + path, {
    headers: { Authorization: `Bearer ${process.env.GH_TOKEN}` },
    signal: AbortSignal.timeout(20000),
  });
  const response = await github("/releases/tags/" + tag);
  if (response.status === 404) {
    const existingTag = await github("/git/ref/tags/" + tag);
    if (existingTag.status !== 404) throw new Error(`Cannot publish over an existing tag or unverifiable tag: ${tag}`);
    publish = true;
  }
  else if (!response.ok) throw new Error(`Unable to check ${tag}: GitHub HTTP ${response.status}`);
  else if ((await response.json()).draft) throw new Error(`Inspect the existing draft ${tag} before retrying`);
}
appendFileSync(process.env.GITHUB_OUTPUT, `tag=${tag}\npublish=${publish}\n`);
