import { useCallback, useEffect, useRef, useState } from "react";
import type { AnalyzerStatus, MessageAnalysis, SyncStatus } from "../../shared/types";
import type { ListMessage } from "../lib/messageCache";
import { BREAKPOINTS, type Viewport, useContentWidth } from "../lib/viewport";
import { FolderList } from "./FolderList";
import { MessageList } from "./MessageList";
import { MessageView } from "./MessageView";

export type MailPane = "folders" | "list" | "reading";

interface Folder {
  id: string;
  name: string;
  role: string;
}

export interface MailPaneMeta {
  canBack: boolean;
  title: string;
  onBack: () => void;
}

export function MailShell({
  viewport,
  folders,
  selectedFolder,
  onSelectFolder,
  slots,
  messageTotal,
  loadingMessages,
  query,
  selectedMessage,
  onSelectMessage,
  clearSelectedMessage,
  loadingBody,
  syncStatus,
  analyzerStatus,
  downloadingBody,
  analysisByMessage,
  onVisibleRange,
  onToggleFlag,
  onToggleSeen,
  onApplyAnalysis,
  onComposeReply,
  onPaneMeta,
}: {
  viewport: Viewport;
  folders: Folder[];
  selectedFolder: string | null;
  onSelectFolder: (id: string) => void;
  slots: Array<ListMessage | null>;
  messageTotal: number;
  loadingMessages: boolean;
  query: string;
  selectedMessage: ListMessage | null;
  onSelectMessage: (m: ListMessage) => void;
  clearSelectedMessage: () => void;
  loadingBody: boolean;
  syncStatus: SyncStatus | null;
  analyzerStatus: AnalyzerStatus | null;
  downloadingBody: boolean;
  analysisByMessage: Record<string, MessageAnalysis>;
  onVisibleRange: (start: number, end: number, ids: string[]) => void;
  onToggleFlag: (m: ListMessage) => void;
  onToggleSeen: (m: ListMessage) => void;
  onApplyAnalysis: (a: MessageAnalysis) => void | Promise<void>;
  onComposeReply: (messageId: string, useSuggestedReply: boolean) => Promise<void>;
  onPaneMeta?: (meta: MailPaneMeta) => void;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const contentWidth = useContentWidth(rootRef);
  const [foldersOpen, setFoldersOpen] = useState(false);
  const [stackPane, setStackPane] = useState<MailPane>("folders");

  const stacked = viewport === "phone" || (viewport === "tablet" && contentWidth < BREAKPOINTS.tabletSplitMin);
  const tabletSplit = viewport === "tablet" && !stacked;
  const desktop = viewport === "desktop";

  useEffect(() => {
    if (!stacked) return;
    if (selectedMessage) setStackPane("reading");
    else if (selectedFolder) setStackPane("list");
    else setStackPane("folders");
  }, [stacked, selectedFolder, selectedMessage]);

  const selectFolder = (id: string) => {
    onSelectFolder(id);
    clearSelectedMessage();
    setFoldersOpen(false);
    if (stacked) setStackPane("list");
  };

  const selectMessage = (m: ListMessage) => {
    onSelectMessage(m);
    if (stacked) setStackPane("reading");
  };

  const goBack = useCallback(() => {
    if (stackPane === "reading") {
      clearSelectedMessage();
      setStackPane("list");
      return;
    }
    if (stackPane === "list") {
      setStackPane("folders");
    }
  }, [stackPane, clearSelectedMessage]);

  const title =
    stacked && stackPane === "reading"
      ? selectedMessage?.subject || "Message"
      : stacked && stackPane === "list"
        ? folders.find((f) => f.id === selectedFolder)?.name || "Mail"
        : "Folders";

  const canBack = stacked && stackPane !== "folders";

  useEffect(() => {
    onPaneMeta?.({ canBack, title, onBack: goBack });
  }, [canBack, title, goBack, onPaneMeta]);

  const mode = desktop ? "desktop" : tabletSplit ? "tablet-split" : "stacked";
  const pane = stacked ? stackPane : "list";

  return (
    <div className={`mail-shell ${mode}`} ref={rootRef} data-pane={pane}>
      {tabletSplit ? (
        <div className="mail-folders-toggle">
          <button type="button" onClick={() => setFoldersOpen((o) => !o)}>
            {foldersOpen ? "Hide folders" : "Folders"}
          </button>
        </div>
      ) : null}

      <div
        className={
          tabletSplit
            ? `mail-pane folders-pane drawer${foldersOpen ? " open" : ""}`
            : "mail-pane folders-pane"
        }
        data-active={desktop || (stacked && stackPane === "folders") || (tabletSplit && foldersOpen) ? "1" : "0"}
      >
        <FolderList
          folders={folders}
          selected={selectedFolder}
          onSelect={selectFolder}
          syncStatus={syncStatus}
          analyzerStatus={analyzerStatus}
          downloadingBody={downloadingBody}
        />
      </div>

      <div
        className="mail-pane list-pane"
        data-active={desktop || tabletSplit || (stacked && stackPane === "list") ? "1" : "0"}
      >
        <MessageList
          slots={slots}
          selectedId={selectedMessage?.id ?? null}
          totalCount={messageTotal}
          loading={loadingMessages}
          listKey={`${selectedFolder ?? ""}:${query}`}
          emptyLabel={
            loadingMessages && messageTotal === 0
              ? "Loading…"
              : query.trim()
                ? "No matching emails"
                : "No messages in this folder"
          }
          onSelect={selectMessage}
          onVisibleRange={onVisibleRange}
          analysisByMessage={analysisByMessage}
          onToggleFlag={onToggleFlag}
          onToggleSeen={onToggleSeen}
        />
      </div>

      <div
        className="mail-pane reader-pane"
        data-active={desktop || tabletSplit || (stacked && stackPane === "reading") ? "1" : "0"}
      >
        <MessageView
          message={selectedMessage}
          loadingBody={loadingBody}
          analysis={selectedMessage ? analysisByMessage[selectedMessage.id] : undefined}
          onApplyAnalysis={async (a): Promise<void> => {
            await Promise.resolve(onApplyAnalysis(a));
          }}
          onComposeReply={
            selectedMessage
              ? (useSuggestedReply) =>
                  onComposeReply(selectedMessage.id, useSuggestedReply)
              : undefined
          }
        />
      </div>

      {tabletSplit && foldersOpen ? (
        <button
          type="button"
          className="mail-drawer-scrim"
          aria-label="Close folders"
          onClick={() => setFoldersOpen(false)}
        />
      ) : null}
    </div>
  );
}
