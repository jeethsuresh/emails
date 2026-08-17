import { useCallback, useEffect, useRef, useState } from "react";
import type PocketBase from "pocketbase";
import type { AnalyzerStatus, MessageAnalysis, SyncStatus } from "../../shared/types";
import type { ListMessage } from "../lib/messageCache";
import type { ComposeMode, MailThread, ThreadMessage } from "../lib/mailApi";
import { type Viewport } from "../lib/viewport";
import { ContactsView } from "./ContactsView";
import { FolderList } from "./FolderList";
import { MessageList } from "./MessageList";
import { MessageView } from "./MessageView";
import { ThreadList } from "./ThreadList";

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
  pb,
  viewport,
  folders,
  selectedFolder,
  onSelectFolder,
  selectedAlias,
  onAliasChange,
  aliases,
  selectedThread,
  threadMessages,
  onOpenThread,
  onSelectLoadedMessage,
  threadRefreshKey,
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
  onReanalyze,
  onCompose,
  onMoveMessage,
  onArchiveMessage,
  onSpamMessage,
  onDeleteMessage,
  onPaneMeta,
}: {
  pb: PocketBase;
  viewport: Viewport;
  folders: Folder[];
  selectedFolder: string | null;
  onSelectFolder: (id: string) => void;
  selectedAlias: string;
  onAliasChange: (email: string) => void;
  aliases: Array<{ email: string; count: number }>;
  selectedThread: MailThread | null;
  threadMessages: ThreadMessage[];
  onOpenThread: (thread: MailThread, messages: ThreadMessage[]) => void;
  onSelectLoadedMessage: (message: ThreadMessage) => void;
  threadRefreshKey: number;
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
  onReanalyze: (messageId: string) => Promise<void>;
  onCompose: (messageId: string, mode: ComposeMode, useSuggestedReply?: boolean) => Promise<void>;
  onMoveMessage: (messageId: string, folderId: string) => Promise<void>;
  onArchiveMessage: (messageId: string) => Promise<void>;
  onSpamMessage: (messageId: string) => Promise<void>;
  onDeleteMessage: (messageId: string) => Promise<void>;
  onPaneMeta?: (meta: MailPaneMeta) => void;
}) {
  const [foldersOpen, setFoldersOpen] = useState(false);
  const [stackPane, setStackPane] = useState<MailPane>("folders");
  const [contactsOpen, setContactsOpen] = useState(false);

  const stacked = viewport === "phone";
  const tabletSplit = viewport === "tablet";
  const desktop = viewport === "desktop";

  useEffect(() => {
    if (!stacked) return;
    setStackPane((prev) => {
      const next: MailPane = selectedMessage ? "reading" : selectedFolder || contactsOpen ? "list" : "folders";
      return prev === next ? prev : next;
    });
  }, [stacked, selectedFolder, selectedMessage, contactsOpen]);

  const selectFolder = (id: string) => {
    setContactsOpen(false);
    onSelectFolder(id);
    clearSelectedMessage();
    setFoldersOpen(false);
    if (stacked) setStackPane("list");
  };

  const selectMessage = (m: ListMessage) => {
    onSelectMessage(m);
    if (stacked) setStackPane("reading");
  };

  const selectLoadedMessage = (message: ThreadMessage) => {
    clearSelectedMessage();
    onSelectLoadedMessage(message);
    if (stacked) setStackPane("reading");
  };

  const openThread = (thread: MailThread, messages: ThreadMessage[]) => {
    onOpenThread(thread, messages);
    if (stacked) setStackPane("reading");
  };

  const selectContacts = () => {
    setContactsOpen(true);
    clearSelectedMessage();
    setFoldersOpen(false);
    if (stacked) setStackPane("list");
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
  const goBackRef = useRef(goBack);
  goBackRef.current = goBack;

  const title =
    stacked && stackPane === "reading"
      ? selectedMessage?.subject || "Message"
      : stacked && stackPane === "list"
        ? contactsOpen
          ? "Contacts"
          : folders.find((f) => f.id === selectedFolder)?.name || "Mail"
        : "Folders";

  const canBack = stacked && stackPane !== "folders";

  const lastPaneMeta = useRef({ canBack: false, title: "" });
  useEffect(() => {
    if (lastPaneMeta.current.canBack === canBack && lastPaneMeta.current.title === title) {
      return;
    }
    lastPaneMeta.current = { canBack, title };
    onPaneMeta?.({
      canBack,
      title,
      onBack: () => goBackRef.current(),
    });
  }, [canBack, title, onPaneMeta]);

  const mode = desktop ? "desktop" : tabletSplit ? "tablet-split" : "stacked";
  const pane = stacked ? stackPane : "list";

  return (
    <div className={`mail-shell ${mode}`} data-pane={pane}>
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
          selected={contactsOpen ? null : selectedFolder}
          onSelect={selectFolder}
          contactsActive={contactsOpen}
          onSelectContacts={selectContacts}
          syncStatus={syncStatus}
          analyzerStatus={analyzerStatus}
          downloadingBody={downloadingBody}
          aliases={aliases}
          selectedAlias={selectedAlias}
          onAliasChange={onAliasChange}
        />
      </div>

      <div
        className="mail-pane list-pane"
        data-active={desktop || tabletSplit || (stacked && stackPane === "list") ? "1" : "0"}
      >
        {query.trim() ? (
          <MessageList
            slots={slots}
            selectedId={selectedMessage?.id ?? null}
            totalCount={messageTotal}
            loading={loadingMessages}
            listKey={`${selectedFolder ?? ""}:${query}`}
            emptyLabel={loadingMessages && messageTotal === 0 ? "Loading…" : "No matching emails"}
            onSelect={selectMessage}
            onVisibleRange={onVisibleRange}
            analysisByMessage={analysisByMessage}
            onToggleFlag={onToggleFlag}
            onToggleSeen={onToggleSeen}
          />
        ) : contactsOpen ? (
          <ContactsView
            pb={pb}
            selectedMessageId={selectedMessage?.id ?? null}
            onSelectMessage={selectLoadedMessage}
          />
        ) : (
          <ThreadList
            pb={pb}
            folder={selectedFolder}
            receivedFor={selectedAlias}
            selectedId={selectedThread?.id ?? null}
            refreshKey={threadRefreshKey}
            onOpenThread={openThread}
          />
        )}
      </div>

      <div
        className="mail-pane reader-pane"
        data-active={desktop || tabletSplit || (stacked && stackPane === "reading") ? "1" : "0"}
      >
        {threadMessages.length > 1 ? (
          <nav className="thread-message-stack" aria-label="Messages in thread">
            {threadMessages.map((message, index) => (
              <button
                type="button"
                key={message.id}
                className={selectedMessage?.id === message.id ? "active" : ""}
                onClick={() => onSelectLoadedMessage(message)}
              >
                <span>{index + 1}</span>
                <strong className="clamp-2">{message.from_addr || "(unknown)"}</strong>
                <time>{message.date ? new Date(message.date).toLocaleDateString() : ""}</time>
              </button>
            ))}
          </nav>
        ) : null}
        <MessageView
          message={selectedMessage}
          loadingBody={loadingBody}
          analysis={selectedMessage ? analysisByMessage[selectedMessage.id] : undefined}
          folders={folders}
          onApplyAnalysis={async (a): Promise<void> => {
            await Promise.resolve(onApplyAnalysis(a));
          }}
          onReanalyze={onReanalyze}
          onCompose={
            selectedMessage
              ? (mode, useSuggestedReply) =>
                  onCompose(selectedMessage.id, mode, useSuggestedReply)
              : undefined
          }
          onMoveMessage={
            selectedMessage
              ? (folderId) => onMoveMessage(selectedMessage.id, folderId)
              : undefined
          }
          onArchiveMessage={
            selectedMessage ? () => onArchiveMessage(selectedMessage.id) : undefined
          }
          onSpamMessage={selectedMessage ? () => onSpamMessage(selectedMessage.id) : undefined}
          onDeleteMessage={
            selectedMessage ? () => onDeleteMessage(selectedMessage.id) : undefined
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
