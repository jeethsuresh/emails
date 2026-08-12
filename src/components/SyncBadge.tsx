import type { SyncStatus } from "../../shared/types";

export function SyncBadge({ status }: { status: SyncStatus | null }) {
  if (!status) return <span className="badge">…</span>;
  const progress =
    status.state === "syncing" && status.phase === "backfill"
      ? ` · backfill · ${status.messagesSynced} msgs`
      : status.state === "syncing" && status.foldersTotal > 0
        ? ` · ${status.foldersSynced}/${status.foldersTotal} folders · ${status.messagesSynced} msgs`
        : status.lastSyncAt
          ? ` · ${new Date(status.lastSyncAt).toLocaleTimeString()}`
          : "";
  return (
    <span className={`badge state-${status.state}`} title={status.message}>
      {status.phase === "backfill" ? "backfill" : status.state}
      {status.phase && status.state === "syncing" && status.phase !== "backfill"
        ? `/${status.phase}`
        : ""}
      {progress}
    </span>
  );
}

export function SyncLivePanel({
  status,
  alwaysShow = false,
}: {
  status: SyncStatus | null;
  alwaysShow?: boolean;
}) {
  if (!status) return null;
  const show =
    alwaysShow ||
    status.state === "syncing" ||
    status.state === "error" ||
    status.state === "offline" ||
    (status.logs?.length ?? 0) > 0;
  if (!show) return null;

  return (
    <aside className="sync-live" aria-live="polite">
      <header>
        <strong>{status.message || status.state}</strong>
        {status.currentFolder ? <span>{status.currentFolder}</span> : null}
      </header>
      {status.state === "syncing" && status.foldersTotal > 0 && (
        <div className="sync-bar">
          <div
            className="sync-bar-fill"
            style={{ width: `${Math.min(100, (100 * status.foldersSynced) / status.foldersTotal)}%` }}
          />
        </div>
      )}
      <ol>
        {(status.logs ?? []).length === 0 ? (
          <li>No recent log lines.</li>
        ) : (
          (status.logs ?? []).slice(-12).map((line, i) => (
            <li key={`${i}-${line}`}>{line}</li>
          ))
        )}
      </ol>
    </aside>
  );
}
