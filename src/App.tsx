import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { MessageAnalysis, SyncStatus } from "../shared/types";
import { createPbClient } from "./lib/pb";
import { applyAnalysisAction, loadAnalysesForMessages } from "./lib/analysis";
import { AccountSetup } from "./components/AccountSetup";
import { FolderList } from "./components/FolderList";
import { MessageList } from "./components/MessageList";
import { MessageView } from "./components/MessageView";
import { SyncBadge, SyncLivePanel } from "./components/SyncBadge";
import { ComposeModal } from "./components/ComposeModal";
import { SettingsScreen } from "./components/SettingsScreen";

const ANALYSIS_POLL_MS = 15_000;

interface Folder {
  id: string;
  name: string;
  role: string;
}

const LIST_FIELDS =
  "id,uid,subject,from_addr,to_addrs,date,snippet,seen,flagged,folder";

interface Message {
  id: string;
  uid?: number;
  subject: string;
  from_addr: string;
  date: string;
  snippet: string;
  body_text: string;
  body_html?: string;
  seen: boolean;
  flagged: boolean;
  folder: string;
}

function escapeFilterValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

/** Newest first: RFC3339 date descending, then IMAP uid descending. */
function sortMessagesNewestFirst(items: Message[]): Message[] {
  return [...items].sort((a, b) => {
    const da = a.date || "";
    const db = b.date || "";
    if (da !== db) return db.localeCompare(da);
    return (b.uid ?? 0) - (a.uid ?? 0);
  });
}

export function App() {
  const pb = useMemo(() => createPbClient(), []);
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [messages, setMessages] = useState<Message[]>([]);
  const [messageTotal, setMessageTotal] = useState(0);
  const [selectedFolder, setSelectedFolder] = useState<string | null>(null);
  const [selectedMessage, setSelectedMessage] = useState<Message | null>(null);
  const [loadingBody, setLoadingBody] = useState(false);
  const [query, setQuery] = useState("");
  const [hasAccount, setHasAccount] = useState(false);
  const [composeOpen, setComposeOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [ready, setReady] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [analysisByMessage, setAnalysisByMessage] = useState<Record<string, MessageAnalysis>>({});

  const selectedFolderRef = useRef<string | null>(null);
  const queryRef = useRef("");
  const messagesRef = useRef<Message[]>([]);
  selectedFolderRef.current = selectedFolder;
  queryRef.current = query;
  messagesRef.current = messages;

  const buildFilter = useCallback((folderId: string | null, search: string) => {
    const safe = escapeFilterValue(search.trim());
    if (safe) {
      return `(subject ~ "${safe}" || from_addr ~ "${safe}" || to_addrs ~ "${safe}" || snippet ~ "${safe}" || search_tokens ~ "${safe}")`;
    }
    return `folder = "${folderId}"`;
  }, []);

  const loadMessages = useCallback(
    async (folderId: string | null, search: string, _page = 1, _append = false) => {
      const q = search.trim();
      if (!folderId && !q) {
        setMessages([]);
        setMessageTotal(0);
        return;
      }
      setLoadingMessages(true);
      try {
        const filter = buildFilter(folderId, q);
        // Full folder metadata (no bodies). Pagination was racing with sync polls and
        // auto-cancelled requests were wiping the list — so load everything light.
        const items = await pb.collection("messages").getFullList<Message>({
          filter,
          sort: "-date,-uid",
          fields: LIST_FIELDS,
          batch: 200,
        });
        if (
          selectedFolderRef.current !== folderId ||
          queryRef.current.trim() !== q
        ) {
          return;
        }
        const sorted = sortMessagesNewestFirst(items);
        setMessageTotal(sorted.length);
        setMessages(sorted);
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        // Never clear an existing list on transient/cancel errors.
        if (/autocancel|abort/i.test(msg)) {
          console.warn("loadMessages skipped", msg);
          return;
        }
        console.error("loadMessages failed", err);
      } finally {
        if (
          selectedFolderRef.current === folderId &&
          queryRef.current.trim() === q
        ) {
          setLoadingMessages(false);
        }
      }
    },
    [pb, buildFilter],
  );

  const refreshAnalyses = useCallback(
    async (ids: string[]) => {
      try {
        const map = await loadAnalysesForMessages(pb, ids);
        setAnalysisByMessage(map);
      } catch (err) {
        console.error("refreshAnalyses failed", err);
      }
    },
    [pb],
  );

  const refreshFolders = useCallback(async () => {
    if (!window.email) {
      setHasAccount(false);
      setReady(true);
      return;
    }
    try {
      const accounts = await pb.collection("accounts").getList(1, 1);
      setHasAccount(accounts.totalItems > 0);
      if (accounts.totalItems === 0) {
        setFolders([]);
        setMessages([]);
        return;
      }

      const folderRows = await pb.collection("folders").getFullList<Folder>({
        sort: "role,name",
      });
      // Prefer inbox first in the sidebar.
      folderRows.sort((a, b) => {
        const rank = (r: string) =>
          r === "inbox" ? 0 : r === "sent" ? 1 : r === "drafts" ? 2 : r === "trash" ? 3 : 9;
        return rank(a.role) - rank(b.role) || a.name.localeCompare(b.name);
      });
      setFolders(folderRows);

      let folderId = selectedFolderRef.current;
      if (!folderId || !folderRows.some((f) => f.id === folderId)) {
        folderId = folderRows.find((f) => f.role === "inbox")?.id ?? folderRows[0]?.id ?? null;
        selectedFolderRef.current = folderId;
        setSelectedFolder(folderId);
      }
      await loadMessages(folderId, queryRef.current);
    } catch (err) {
      console.error("refreshFolders failed", err);
      setHasAccount(false);
    } finally {
      setReady(true);
    }
  }, [pb, loadMessages]);

  const applyAnalysis = useCallback(
    async (analysis: MessageAnalysis) => {
      const subject = messagesRef.current.find((m) => m.id === analysis.message)?.subject ?? "";
      await applyAnalysisAction(pb, analysis, subject);
      if (analysis.suggested_action === "move_to_folder" || analysis.suggested_action === "move_to_spam") {
        await refreshFolders();
      } else {
        await refreshAnalyses(messagesRef.current.map((m) => m.id));
      }
    },
    [pb, refreshFolders, refreshAnalyses],
  );

  useEffect(() => {
    if (!window.email) {
      console.error("window.email missing — preload failed");
      setReady(true);
      return;
    }
    void window.email.getSyncStatus().then(setStatus).catch(console.error);
    return window.email.onSyncStatus((next) => {
      setStatus((prev) => {
        const recentDone =
          prev?.phase === "recent" &&
          (next.phase === "backfill" || (next.state === "idle" && next.phase === "idle"));
        // Full list is expensive (~4k rows) — only reload when a sync phase settles.
        if (recentDone || (next.state === "idle" && next.phase === "idle")) {
          void refreshFolders();
        }
        return next;
      });
    });
  }, [refreshFolders]);

  useEffect(() => {
    void refreshFolders();
    const t = setInterval(() => void refreshFolders(), 30_000);
    return () => clearInterval(t);
  }, [refreshFolders]);

  // Keep the open reader metadata in sync; do not clobber loaded bodies from list rows.
  useEffect(() => {
    if (!selectedMessage) return;
    const next = messages.find((m) => m.id === selectedMessage.id);
    if (!next) return;
    setSelectedMessage((prev) => {
      if (!prev || prev.id !== next.id) return prev;
      return {
        ...prev,
        subject: next.subject,
        from_addr: next.from_addr,
        date: next.date,
        snippet: next.snippet,
        seen: next.seen,
        flagged: next.flagged,
      };
    });
  }, [messages, selectedMessage?.id]);

  // Debounce search so we don't hammer PocketBase on every keystroke.
  useEffect(() => {
    const handle = window.setTimeout(() => {
      void loadMessages(selectedFolder, query);
    }, query.trim() ? 250 : 0);
    return () => window.clearTimeout(handle);
  }, [selectedFolder, query, loadMessages]);

  // Load analyses for the visible list, and re-poll periodically so
  // pending/running rows pick up "done" status without a full folder refresh.
  useEffect(() => {
    void refreshAnalyses(messages.map((m) => m.id));
  }, [messages, refreshAnalyses]);

  useEffect(() => {
    const t = setInterval(
      () => void refreshAnalyses(messagesRef.current.map((m) => m.id)),
      ANALYSIS_POLL_MS,
    );
    return () => clearInterval(t);
  }, [refreshAnalyses]);

  const selectFolder = (id: string) => {
    if (id === selectedFolderRef.current) return;
    selectedFolderRef.current = id;
    setSelectedFolder(id);
    setSelectedMessage(null);
    setQuery("");
    queryRef.current = "";
    setMessages([]);
    setMessageTotal(0);
  };

  const selectMessage = async (m: Message) => {
    setSelectedMessage(m);
    setLoadingBody(true);
    try {
      // List rows omit bodies — fetch the full record (and backfill from IMAP if empty).
      const full = await pb.collection("messages").getOne<Message>(m.id);
      let next = full;
      if (!full.body_text?.trim() && !full.body_html?.trim()) {
        const res = await window.email.fetchMessageBody(m.id);
        const text = new TextDecoder().decode(new Uint8Array(res.body));
        const data = JSON.parse(text) as {
          body_text?: string;
          body_html?: string;
          snippet?: string;
        };
        next = {
          ...full,
          body_text: data.body_text ?? "",
          body_html: data.body_html ?? "",
          snippet: data.snippet ?? full.snippet,
        };
      }
      setSelectedMessage(next);
    } catch (err) {
      console.error("selectMessage failed", err);
    } finally {
      setLoadingBody(false);
    }
  };

  const loadMoreMessages = () => {
    // Full list is loaded up front; keep handler for MessageList API compat.
  };

  if (!hasAccount) {
    return (
      <div className="setup-screen">
        <AccountSetup
          onSaved={async () => {
            setHasAccount(true);
            await refreshFolders();
          }}
        />
        <SyncBadge status={status} />
        <SyncLivePanel status={status} />
        {!ready && <p className="hint">Connecting to local backend…</p>}
      </div>
    );
  }

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand">Email</div>
        <input
          className="search"
          placeholder="Search subject, from, or body…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-label="Search mail"
        />
        <button type="button" onClick={() => setComposeOpen(true)}>
          Compose
        </button>
        <button type="button" onClick={() => setSettingsOpen(true)}>
          Settings
        </button>
        <button type="button" onClick={() => void window.email.triggerSync()}>
          Sync
        </button>
        <SyncBadge status={status} />
      </header>

      <SyncLivePanel status={status} />

      <div className="layout">
        <FolderList folders={folders} selected={selectedFolder} onSelect={selectFolder} />
        <MessageList
          messages={messages}
          selectedId={selectedMessage?.id ?? null}
          totalCount={messageTotal}
          loading={loadingMessages}
          hasMore={false}
          emptyLabel={
            loadingMessages && messages.length === 0
              ? "Loading…"
              : query.trim()
                ? "No matching emails"
                : "No messages in this folder"
          }
          onSelect={(m) => void selectMessage(m as Message)}
          onLoadMore={loadMoreMessages}
          analysisByMessage={analysisByMessage}
          onToggleFlag={async (msg) => {
            await pb.collection("messages").update(msg.id, { flagged: !msg.flagged });
            setMessages((prev) =>
              prev.map((row) => (row.id === msg.id ? { ...row, flagged: !msg.flagged } : row)),
            );
          }}
          onToggleSeen={async (msg) => {
            await pb.collection("messages").update(msg.id, { seen: !msg.seen });
            setMessages((prev) =>
              prev.map((row) => (row.id === msg.id ? { ...row, seen: !msg.seen } : row)),
            );
          }}
        />
        <MessageView
          message={selectedMessage}
          loadingBody={loadingBody}
          analysis={selectedMessage ? analysisByMessage[selectedMessage.id] : undefined}
          onApplyAnalysis={applyAnalysis}
        />
      </div>

      {composeOpen && (
        <ComposeModal
          pb={pb}
          onClose={() => setComposeOpen(false)}
          onSaved={() => void loadMessages(selectedFolderRef.current, queryRef.current)}
        />
      )}
      {settingsOpen && (
        <SettingsScreen
          pb={pb}
          onClose={() => setSettingsOpen(false)}
          onSaved={async () => {
            await refreshFolders();
            void window.email.triggerSync();
          }}
        />
      )}
    </div>
  );
}
