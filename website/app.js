const ORDER = ["mac", "win", "deb", "rpm", "pacman", "appimage"];
const FALLBACK_LABELS = {
  mac: "macOS (Apple Silicon)",
  win: "Windows",
  deb: "Ubuntu / Debian",
  rpm: "Fedora / RHEL",
  pacman: "Arch / Manjaro",
  appimage: "Linux AppImage",
};

const versionLine = document.getElementById("version-line");
const downloadsEl = document.getElementById("downloads");
const releasesLink = document.getElementById("releases-link");

function card(key, info, releaseUrl) {
  const el = document.createElement("a");
  el.className = info ? "download" : "download missing";
  el.href = info ? info.url : releaseUrl;
  if (info) el.setAttribute("download", info.filename);
  const title = document.createElement("strong");
  title.textContent = (info && info.label) || FALLBACK_LABELS[key];
  const meta = document.createElement("span");
  meta.textContent = info ? info.filename : "Not in the latest release yet";
  el.append(title, meta);
  return el;
}

try {
  const res = await fetch("./latest.json", { cache: "no-store" });
  if (!res.ok) throw new Error(`latest.json ${res.status}`);
  const latest = await res.json();
  const releaseUrl = latest.releaseUrl || "https://github.com/jeethsuresh/emails/releases";
  releasesLink.href = releaseUrl;

  if (latest.version) {
    const when = latest.publishedAt
      ? new Date(latest.publishedAt).toLocaleDateString(undefined, {
          year: "numeric",
          month: "short",
          day: "numeric",
        })
      : "";
    versionLine.textContent = when
      ? `Latest release ${latest.version} · ${when}`
      : `Latest release ${latest.version}`;
  } else {
    versionLine.textContent = "No release published yet — check GitHub Releases.";
  }

  const downloads = latest.downloads || {};
  for (const key of ORDER) {
    downloadsEl.append(card(key, downloads[key], releaseUrl));
  }
} catch (err) {
  console.error(err);
  versionLine.textContent = "Could not load release metadata. Use GitHub Releases instead.";
  const releaseUrl = "https://github.com/jeethsuresh/emails/releases";
  for (const key of ORDER) {
    downloadsEl.append(card(key, null, releaseUrl));
  }
}
