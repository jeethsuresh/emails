import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AnalyzerStatus, MessageAnalysis, SyncStatus } from "../shared/types";
import { createPbClient } from "./lib/pb";
import {
  applyAnalysisAction,
  getAnalyzerStatus,
  loadAnalysesForMessages,
} from "./lib/analysis";
import {
  evictPagesOutside,
  fetchMessagePage,
  mergePageIntoSlots,
  pagesForRange,
  type ListMessage,
} from "./lib/messageCache";
import { AccountSetup } from "./components/AccountSetup";
import { FolderList } from "./components/FolderList";
import { MessageList } from "./components/MessageList";
import { MessageView } from "./components/MessageView";
import { SyncBadge } from "./components/SyncBadge";
import { ComposeModal } from "./components/ComposeModal";
import { SettingsScreen } from "./components/SettingsScreen";
import { TodoList } from "./components/TodoList";
import { EventList } from "./components/EventList";

const ANALYSIS_POLL_MS = 15_000;
const ANALYZER_STATUS_POLL_MS = 2_000;

type AppTab = "mail" | "todos" | "events";

interface Folder {
  id: string;
  name: string;
  role: string;
}

type Message = ListMessage;

function escapeFilterValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

export function App() {
  const pb = useMemo(() => createPbClient(), []);
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [slots, setSlots] = useState<Array<Message | null>>([]);
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
  const [analyzerStatus, setAnalyzerStatus] = useState<AnalyzerStatus | null>(null);
  const [activeTab, setActiveTab] = useState<AppTab>("mail");
  const [visibleMessageIds, setVisibleMessageIds] = useState<string[]>([]);

  const selectedFolderRef = useRef<string | null>(null);
  const queryRef = useRef("");
  const slotsRef = useRef<Array<Message | null>>([]);
  const visibleIdsRef = useRef<string[]>([]);
  const loadSeqRef = useRef(0);
  const loadedPagesRef = useRef<Set<number>>(new Set());
  const inflightPagesRef = useRef<Set<number>>(new Set());
  slotsRef.current = slots;
  visibleIdsRef.current = visibleMessageIds;

  useEffect(() => {
    selectedFolderRef.current = selectedFolder;
  }, [selectedFolder]);
  useEffect(() => {
    queryRef.current = query;
  }, [query]);

  const buildFilter = useCallback((folderId: string | null, search: string) => {
    const safe = escapeFilterValue(search.trim());
    if (safe) {
      return `(subject ~ "${safe}" || from_addr ~ "${safe}" || to_addrs ~ "${safe}" || snippet ~ "${safe}" || search_tokens ~ "${safe}")`;
    }
    return `folder = "${folderId}"`;
  }, []);

  const resetMessageList = useCallback(() => {
    loadSeqRef.current += 1;
    loadedPagesRef.current = new Set();
    inflightPagesRef.current = new Set();
    setSlots([]);
    setMessageTotal(0);
    setVisibleMessageIds([]);
    setAnalysisByMessage({});
  }, []);

  const ensurePages = useCallback(
    async (folderId: string | null, search: string, start: number, end: number) => {
      const q = search.trim();
      if (!folderId && !q) {
        resetMessageList();
        return;
      }
      const seq = loadSeqRef.current;
      const filter = buildFilter(folderId, q);
      const wanted = pagesForRange(start, end);
      const keep = new Set(wanted);
      setLoadingMessages(true);
      try {
        for (const page of wanted) {
          if (loadedPagesRef.current.has(page) || inflightPagesRef.current.has(page)) continue;
          inflightPagesRef.current.add(page);
          try {
            const { items, totalItems } = await fetchMessagePage(pb, filter, page);
            if (
              seq !== loadSeqRef.current ||
              selectedFolderRef.current !== folderId ||
              queryRef.current.trim() !== q
            ) {
              return;
            }
            loadedPagesRef.current.add(page);
            setMessageTotal(totalItems);
            setSlots((prev) => {
              const merged = mergePageIntoSlots(prev, totalItems, page, items);
              return evictPagesOutside(merged, keep);
            });
            // Drop loaded-page tracking for evicted pages.
            for (const p of [...loadedPagesRef.current]) {
              if (!keep.has(p)) loadedPagesRef.current.delete(p);
            }
          } finally {
            inflightPagesRef.current.delete(page);
          }
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        if (/autocancel|abort/i.test(msg)) {
          console.warn("ensurePages skipped", msg);
          return;
        }
        console.error("ensurePages failed", err);
      } finally {
        if (seq === loadSeqRef.current) {
          setLoadingMessages(false);
        }
      }
    },
    [pb, buildFilter, resetMessageList],
  );

  const reloadList = useCallback(
    async (folderId: string | null, search: string) => {
      resetMessageList();
      // Reserve seq after resetMessageList bumped it.
      await ensurePages(folderId, search, 0, 74);
    },
    [resetMessageList, ensurePages],
  );

  const refreshAnalyses = useCallback(
    async (ids: string[]) => {
      if (ids.length === 0) return;
      try {
        const map = await loadAnalysesForMessages(pb, ids);
        setAnalysisByMessage((prev) => ({ ...prev, ...map }));
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
        resetMessageList();
        return;
      }

      const folderRows = await pb.collection("folders").getFullList<Folder>({
        sort: "role,name",
      });
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
      await reloadList(folderId, queryRef.current);
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (/autocancel|abort/i.test(msg)) {
        console.warn("refreshFolders skipped", msg);
        return;
      }
      console.error("refreshFolders failed", err);
    } finally {
      setReady(true);
    }
  }, [pb, reloadList, resetMessageList]);

  const applyAnalysis = useCallback(
    async (analysis: MessageAnalysis) => {
      const subject =
        slotsRef.current.find((m) => m?.id === analysis.message)?.subject ??
        selectedMessage?.subject ??
        "";
      await applyAnalysisAction(pb, analysis, subject);
      if (analysis.suggested_action === "move_to_folder" || analysis.suggested_action === "move_to_spam") {
        await refreshFolders();
      } else {
        await refreshAnalyses(visibleIdsRef.current);
      }
    },
    [pb, refreshFolders, refreshAnalyses, selectedMessage?.subject],
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
        const becameIdle =
          prev?.state === "syncing" && next.state === "idle" && next.phase === "idle";
        if (recentDone || becameIdle) {
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

  useEffect(() => {
    if (!selectedMessage) return;
    const next = slots.find((m) => m?.id === selectedMessage.id);
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
  }, [slots, selectedMessage?.id]);

  useEffect(() => {
    const handle = window.setTimeout(() => {
      void reloadList(selectedFolderRef.current, query);
    }, query.trim() ? 250 : 0);
    return () => window.clearTimeout(handle);
  }, [query, reloadList]);

  useEffect(() => {
    void refreshAnalyses(visibleMessageIds);
  }, [visibleMessageIds, refreshAnalyses]);

  useEffect(() => {
    const t = setInterval(
      () => void refreshAnalyses(visibleIdsRef.current),
      ANALYSIS_POLL_MS,
    );
    return () => clearInterval(t);
  }, [refreshAnalyses]);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const next = await getAnalyzerStatus(pb);
        if (!cancelled) setAnalyzerStatus(next);
      } catch (err) {
        console.warn("analyzer status poll failed", err);
      }
    };
    void tick();
    const t = setInterval(() => void tick(), ANALYZER_STATUS_POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, [pb]);

  const selectFolder = (id: string) => {
    if (id === selectedFolder) return;
    selectedFolderRef.current = id;
    queryRef.current = "";
    setSelectedFolder(id);
    setSelectedMessage(null);
    setQuery("");
    void reloadList(id, "");
  };

  const selectMessage = async (m: Message) => {
    setSelectedMessage(m);
    setLoadingBody(true);
    try {
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

  const patchSlot = (id: string, patch: Partial<Message>) => {
    setSlots((prev) => {
      const idx = prev.findIndex((row) => row?.id === id);
      if (idx < 0 || !prev[idx]) return prev;
      const next = prev.slice();
      next[idx] = { ...prev[idx]!, ...patch };
      return next;
    });
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
        {!ready && <p className="hint">Connecting to local backend…</p>}
      </div>
    );
  }

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand">Email</div>
        <nav className="app-tabs" aria-label="Primary">
          <button
            type="button"
            className={activeTab === "mail" ? "tab active" : "tab"}
            onClick={() => setActiveTab("mail")}
          >
            Mail
          </button>
          <button
            type="button"
            className={activeTab === "todos" ? "tab active" : "tab"}
            onClick={() => setActiveTab("todos")}
          >
            Todos
          </button>
          <button
            type="button"
            className={activeTab === "events" ? "tab active" : "tab"}
            onClick={() => setActiveTab("events")}
          >
            Events
          </button>
        </nav>
        {activeTab === "mail" ? (
          <input
            className="search"
            placeholder="Search subject, from, or body…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search mail"
          />
        ) : (
          <div className="search-spacer" />
        )}
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

      {activeTab === "mail" ? (
        <div className="layout">
          <FolderList
            folders={folders}
            selected={selectedFolder}
            onSelect={selectFolder}
            syncStatus={status}
            analyzerStatus={analyzerStatus}
            downloadingBody={loadingBody}
          />
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
            onSelect={(m) => void selectMessage(m as Message)}
            onVisibleRange={(start, end, ids) => {
              setVisibleMessageIds(ids);
              void ensurePages(selectedFolderRef.current, queryRef.current, start, end);
            }}
            analysisByMessage={analysisByMessage}
            onToggleFlag={async (msg) => {
              await pb.collection("messages").update(msg.id, { flagged: !msg.flagged });
              patchSlot(msg.id, { flagged: !msg.flagged });
            }}
            onToggleSeen={async (msg) => {
              await pb.collection("messages").update(msg.id, { seen: !msg.seen });
              patchSlot(msg.id, { seen: !msg.seen });
            }}
          />
          <MessageView
            message={selectedMessage}
            loadingBody={loadingBody}
            analysis={selectedMessage ? analysisByMessage[selectedMessage.id] : undefined}
            onApplyAnalysis={applyAnalysis}
          />
        </div>
      ) : null}

      {activeTab === "todos" ? <TodoList pb={pb} active={activeTab === "todos"} /> : null}
      {activeTab === "events" ? <EventList pb={pb} active={activeTab === "events"} /> : null}

      {composeOpen && (
        <ComposeModal
          pb={pb}
          onClose={() => setComposeOpen(false)}
          onSaved={() => void reloadList(selectedFolderRef.current, queryRef.current)}
        />
      )}
      {settingsOpen && (
        <SettingsScreen
          pb={pb}
          status={status}
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
