import type { AnalyzerStatus, SyncStatus } from "../../shared/types";

function clampPct(n: number): number {
  if (!Number.isFinite(n) || n < 0) return 0;
  if (n > 100) return 100;
  return n;
}

function syncProgress(status: SyncStatus | null): {
  label: string;
  detail: string;
  pct: number | null;
  active: boolean;
} {
  if (!status) {
    return { label: "Sync", detail: "Waiting…", pct: null, active: false };
  }
  const syncing = status.state === "syncing";
  const folderPct =
    status.foldersTotal > 0
      ? clampPct((status.foldersSynced / status.foldersTotal) * 100)
      : null;
  if (syncing) {
    const folder =
      status.currentFolder ||
      (status.phase === "recent" || status.phase === "backfill" ? status.phase : "");
    const detail =
      status.message ||
      (folder
        ? `${folder}${status.messagesSynced ? ` · ${status.messagesSynced} msgs` : ""}`
        : "Syncing…");
    return {
      label: "Sync",
      detail,
      pct: folderPct,
      active: true,
    };
  }
  if (status.state === "error") {
    return { label: "Sync", detail: status.message || "Error", pct: null, active: false };
  }
  if (status.state === "offline") {
    return { label: "Sync", detail: status.message || "Offline", pct: null, active: false };
  }
  return {
    label: "Sync",
    detail: status.message || "Up to date",
    pct: status.foldersTotal > 0 ? 100 : null,
    active: false,
  };
}

function analyzerProgress(status: AnalyzerStatus | null): {
  label: string;
  detail: string;
  pct: number | null;
  active: boolean;
  indeterminate: boolean;
} {
  if (!status) {
    return {
      label: "AI",
      detail: "Waiting…",
      pct: null,
      active: false,
      indeterminate: false,
    };
  }
  switch (status.state) {
    case "running":
      return {
        label: "AI",
        detail:
          status.queueDepth > 0
            ? `${status.message || "Analyzing"} · ${status.queueDepth.toLocaleString()} queued`
            : status.message || "Analyzing…",
        pct: null,
        active: true,
        indeterminate: true,
      };
    case "paused":
      return {
        label: "AI",
        detail: status.message || "Paused — LM Studio unreachable",
        pct: null,
        active: true,
        indeterminate: true,
      };
    case "idle":
      return {
        label: "AI",
        detail: status.queueDepth > 0 ? `${status.queueDepth.toLocaleString()} queued` : "Idle",
        pct: status.queueDepth > 0 ? null : 100,
        active: status.queueDepth > 0,
        indeterminate: status.queueDepth > 0,
      };
    default: {
      const _exhaustive: never = status.state;
      return _exhaustive;
    }
  }
}

export function SidebarProgress({
  syncStatus,
  analyzerStatus,
  downloadingBody,
}: {
  syncStatus: SyncStatus | null;
  analyzerStatus: AnalyzerStatus | null;
  downloadingBody: boolean;
}) {
  const sync = syncProgress(syncStatus);
  const ai = analyzerProgress(analyzerStatus);

  return (
    <div className="sidebar-progress" aria-live="polite">
      {downloadingBody ? (
        <div className="sidebar-progress-row">
          <div className="sidebar-progress-meta">
            <span>Mail</span>
            <em>Downloading body…</em>
          </div>
          <div className="sidebar-progress-track">
            <div className="sidebar-progress-fill is-indeterminate" />
          </div>
        </div>
      ) : null}

      <div className="sidebar-progress-row">
        <div className="sidebar-progress-meta">
          <span>{sync.label}</span>
          <em title={sync.detail}>{sync.detail}</em>
        </div>
        <div className="sidebar-progress-track">
          <div
            className={`sidebar-progress-fill${sync.active && sync.pct == null ? " is-indeterminate" : ""}${sync.active ? " is-active" : ""}`}
            style={sync.pct != null ? { width: `${sync.pct}%` } : undefined}
          />
        </div>
      </div>

      <div className="sidebar-progress-row">
        <div className="sidebar-progress-meta">
          <span>{ai.label}</span>
          <em title={ai.detail}>{ai.detail}</em>
        </div>
        <div className="sidebar-progress-track">
          <div
            className={`sidebar-progress-fill${ai.indeterminate ? " is-indeterminate" : ""}${ai.active ? " is-active" : ""}`}
            style={!ai.indeterminate && ai.pct != null ? { width: `${ai.pct}%` } : undefined}
          />
        </div>
      </div>
    </div>
  );
}
