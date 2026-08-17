import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AnalyzerStatus, MessageAnalysis, SyncStatus } from "../shared/types";
import { createPbClient } from "./lib/pb";
import {
  applyAnalysisAction,
  completeEventApply,
  getAnalyzerStatus,
  loadAnalysesForMessages,
  reanalyzeMessage,
  type ApplyAnalysisResult,
} from "./lib/analysis";
import type { CalendarRecord, EventWriteInput } from "./lib/calendarApi";
import { CreateEventModal } from "./components/CreateEventModal";
import {
  evictPagesOutside,
  fetchMessagePage,
  mergePageIntoSlots,
  pagesForRange,
  type ListMessage,
} from "./lib/messageCache";
import {
  composeReply,
  listAliases,
  moveMessage,
  type ComposeMode,
  type ComposePrefill,
  type MailThread,
  type ThreadMessage,
} from "./lib/mailApi";
import { AccountSetup } from "./components/AccountSetup";
import { AppChrome, type AppTab } from "./components/AppChrome";
import { MailShell, type MailPaneMeta } from "./components/MailShell";
import { SyncBadge } from "./components/SyncBadge";
import { ComposeModal } from "./components/ComposeModal";
import { SettingsScreen } from "./components/SettingsScreen";
import { TodoList } from "./components/TodoList";
import { CalendarView } from "./components/CalendarView";
import { useViewport } from "./lib/viewport";

const ANALYSIS_POLL_MS = 15_000;
const ANALYZER_STATUS_POLL_MS = 2_000;

interface Folder {
  id: string;
  name: string;
  role: string;
}

type Message = ListMessage;

async function fetchMissingMessageBody<T extends Message>(message: T): Promise<T> {
  if (message.body_text?.trim() || message.body_html?.trim()) return message;

  const res = await window.email.fetchMessageBody(message.id);
  const text = new TextDecoder().decode(new Uint8Array(res.body));
  const data = JSON.parse(text) as {
    body_text?: string;
    body_html?: string;
    snippet?: string;
  };
  return {
    ...message,
    body_text: data.body_text ?? "",
    body_html: data.body_html ?? "",
    snippet: data.snippet ?? message.snippet,
  };
}

function escapeFilterValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

export function App() {
  const pb = useMemo(() => createPbClient(), []);
  const viewport = useViewport();
  const [status, setStatus] = useState<SyncStatus | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [slots, setSlots] = useState<Array<Message | null>>([]);
  const [messageTotal, setMessageTotal] = useState(0);
  const [selectedFolder, setSelectedFolder] = useState<string | null>(null);
  const [selectedMessage, setSelectedMessage] = useState<Message | null>(null);
  const [selectedAlias, setSelectedAlias] = useState("");
  const [selectedThread, setSelectedThread] = useState<MailThread | null>(null);
  const [threadMessages, setThreadMessages] = useState<ThreadMessage[]>([]);
  const [threadRefreshKey, setThreadRefreshKey] = useState(0);
  const [loadingBody, setLoadingBody] = useState(false);
  const [query, setQuery] = useState("");
  const [hasAccount, setHasAccount] = useState(false);
  const [composeOpen, setComposeOpen] = useState(false);
  const [composePrefill, setComposePrefill] = useState<ComposePrefill>();
  const [aliases, setAliases] = useState<Array<{ email: string; count: number }>>([]);
  const [accountEmails, setAccountEmails] = useState<string[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [eventApplyGate, setEventApplyGate] = useState<{
    analysis: MessageAnalysis;
    initial: Partial<EventWriteInput>;
    resolve: () => void;
    reject: (err: Error) => void;
  } | null>(null);
  const [applyCalendars, setApplyCalendars] = useState<CalendarRecord[]>([]);
  const [ready, setReady] = useState(false);
  const [loadingMessages, setLoadingMessages] = useState(false);
  const [analysisByMessage, setAnalysisByMessage] = useState<Record<string, MessageAnalysis>>({});
  const [analyzerStatus, setAnalyzerStatus] = useState<AnalyzerStatus | null>(null);
  const [activeTab, setActiveTab] = useState<AppTab>("mail");
  const [visibleMessageIds, setVisibleMessageIds] = useState<string[]>([]);
  const [mailMeta, setMailMeta] = useState<MailPaneMeta | null>(null);
  const handlePaneMeta = useCallback((meta: MailPaneMeta) => {
    setMailMeta((prev) => {
      if (prev && prev.canBack === meta.canBack && prev.title === meta.title) {
        return prev;
      }
      return meta;
    });
  }, []);
  const selectedFolderRef = useRef<string | null>(null);
  const queryRef = useRef("");
  const slotsRef = useRef<Array<Message | null>>([]);
  const visibleIdsRef = useRef<string[]>([]);
  const loadSeqRef = useRef(0);
  const selectionSeqRef = useRef(0);
  const loadedPagesRef = useRef<Set<number>>(new Set());
  const inflightPagesRef = useRef<Set<number>>(new Set());
  const readerIdsRef = useRef<string[]>([]);
  const readerMessageIds = useMemo(() => {
    const ids: string[] = [];
    for (const message of threadMessages) {
      if (message.id) ids.push(message.id);
    }
    if (selectedMessage?.id && !ids.includes(selectedMessage.id)) {
      ids.push(selectedMessage.id);
    }
    return ids;
  }, [threadMessages, selectedMessage?.id]);
  slotsRef.current = slots;
  visibleIdsRef.current = visibleMessageIds;
  readerIdsRef.current = readerMessageIds;

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 6000);
    return () => window.clearTimeout(timer);
  }, [notice]);

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
    async (ids: string[], opts: { includeReply?: boolean } = {}) => {
      if (ids.length === 0) return;
      try {
        const map = await loadAnalysesForMessages(pb, ids, opts);
        setAnalysisByMessage((prev) => {
          const next = { ...prev };
          for (const [id, row] of Object.entries(map)) {
            next[id] = {
              ...prev[id],
              ...row,
              suggested_reply: row.suggested_reply || prev[id]?.suggested_reply || "",
            };
          }
          return next;
        });
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
      const accounts = await pb
        .collection("accounts")
        .getList<{ id: string; email: string }>(1, 50, { fields: "id,email" });
      setHasAccount(accounts.totalItems > 0);
      setAccountEmails(accounts.items.map((account) => account.email).filter(Boolean));
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
      setThreadRefreshKey((key) => key + 1);

      let folderId = selectedFolderRef.current;
      if (!folderId || !folderRows.some((f) => f.id === folderId)) {
        folderId = folderRows.find((f) => f.role === "inbox")?.id ?? folderRows[0]?.id ?? null;
        selectedFolderRef.current = folderId;
        setSelectedFolder(folderId);
      }
      if (queryRef.current.trim()) {
        await reloadList(folderId, queryRef.current);
      } else {
        resetMessageList();
      }
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
      const result: ApplyAnalysisResult = await applyAnalysisAction(pb, analysis, subject);
      if (result.status === "needs_event_details") {
        const rows = await pb.collection("calendars").getFullList<CalendarRecord>({ batch: 50 });
        setApplyCalendars(rows);
        await new Promise<void>((resolve, reject) => {
          setEventApplyGate({
            analysis: result.analysis,
            initial: result.initial,
            resolve,
            reject,
          });
        });
      }
      if (analysis.suggested_action === "move_to_folder" || analysis.suggested_action === "move_to_spam") {
        await refreshFolders();
      } else {
        await refreshAnalyses(visibleIdsRef.current);
        await refreshAnalyses(readerIdsRef.current, { includeReply: true });
      }
    },
    [pb, refreshFolders, refreshAnalyses, selectedMessage?.subject],
  );

  const reanalyze = useCallback(
    async (messageId: string) => {
      await reanalyzeMessage(pb, messageId);
      setAnalysisByMessage((prev) => ({
        ...prev,
        [messageId]: {
          ...(prev[messageId] ?? {
            id: "",
            message: messageId,
            priority: "",
            suggested_action: "",
            action_target: "",
            suggested_reply: "",
            model: "",
            error: "",
            fail_count: 0,
            analyzed_at: "",
          }),
          status: "pending",
          priority: "",
          suggested_action: "",
          action_target: "",
          suggested_reply: "",
          event_starts_at: "",
          event_ends_at: "",
          event_attendees: "",
          error: "",
        },
      }));
      await refreshAnalyses([messageId], { includeReply: true });
    },
    [pb, refreshAnalyses],
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
    if (!hasAccount) {
      setAliases([]);
      return;
    }
    let cancelled = false;
    void listAliases(pb)
      .then((rows) => {
        if (!cancelled) setAliases(rows);
      })
      .catch((err: unknown) => console.error("listAliases failed", err));
    return () => {
      cancelled = true;
    };
  }, [hasAccount, pb]);

  // Account addresses are always valid senders; aliases only appear once mail
  // has arrived for them, so From must not depend on the alias list.
  const fromOptions = useMemo(
    () => [...new Set([...accountEmails, ...aliases.map((row) => row.email)].filter(Boolean))],
    [accountEmails, aliases],
  );

  useEffect(() => {
    if (!selectedMessage) return;
    const next = slots.find((m) => m?.id === selectedMessage.id);
    if (!next) return;
    setSelectedMessage((prev) => {
      if (!prev || prev.id !== next.id) return prev;
      if (
        prev.subject === next.subject &&
        prev.from_addr === next.from_addr &&
        prev.date === next.date &&
        prev.snippet === next.snippet &&
        prev.seen === next.seen &&
        prev.flagged === next.flagged
      ) {
        return prev;
      }
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
      selectionSeqRef.current += 1;
      setSelectedThread(null);
      setThreadMessages([]);
      setSelectedMessage(null);
      setLoadingBody(false);
      if (query.trim()) {
        void reloadList(selectedFolderRef.current, query);
      } else {
        resetMessageList();
      }
    }, query.trim() ? 250 : 0);
    return () => window.clearTimeout(handle);
  }, [query, reloadList, resetMessageList]);

  useEffect(() => {
    void refreshAnalyses(visibleMessageIds);
  }, [visibleMessageIds, refreshAnalyses]);

  useEffect(() => {
    void refreshAnalyses(readerMessageIds, { includeReply: true });
  }, [readerMessageIds, refreshAnalyses]);

  useEffect(() => {
    const t = setInterval(() => {
      void refreshAnalyses(visibleIdsRef.current);
      void refreshAnalyses(readerIdsRef.current, { includeReply: true });
    }, ANALYSIS_POLL_MS);
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
    clearMailSelection();
    setQuery("");
    resetMessageList();
  };

  const selectMessage = async (m: Message) => {
    const seq = ++selectionSeqRef.current;
    setSelectedThread(null);
    setThreadMessages([]);
    setSelectedMessage(m);
    setLoadingBody(true);
    try {
      const full = await pb.collection("messages").getOne<Message>(m.id);
      const next = await fetchMissingMessageBody(full);
      if (seq === selectionSeqRef.current) setSelectedMessage(next);
    } catch (err) {
      if (seq === selectionSeqRef.current) console.error("selectMessage failed", err);
    } finally {
      if (seq === selectionSeqRef.current) setLoadingBody(false);
    }
  };

  const selectLoadedMessage = async (message: ThreadMessage) => {
    const seq = ++selectionSeqRef.current;
    setSelectedMessage(message);
    setLoadingBody(true);
    try {
      const next = await fetchMissingMessageBody(message);
      if (seq !== selectionSeqRef.current) return;
      setSelectedMessage(next);
      setThreadMessages((current) =>
        current.map((item) => (item.id === next.id ? { ...item, ...next } : item)),
      );
    } catch (err) {
      if (seq === selectionSeqRef.current) console.error("selectLoadedMessage failed", err);
    } finally {
      if (seq === selectionSeqRef.current) setLoadingBody(false);
    }
  };

  const openThread = (thread: MailThread, messages: ThreadMessage[]) => {
    setSelectedThread(thread);
    setThreadMessages(messages);
    const newest = messages.at(-1);
    if (newest) {
      void selectLoadedMessage(newest);
    } else {
      selectionSeqRef.current += 1;
      setSelectedMessage(null);
      setLoadingBody(false);
    }
  };

  const clearMailSelection = useCallback(() => {
    selectionSeqRef.current += 1;
    setSelectedMessage(null);
    setSelectedThread(null);
    setThreadMessages([]);
    setLoadingBody(false);
  }, []);

  const refreshCurrentList = () => {
    setThreadRefreshKey((key) => key + 1);
    if (queryRef.current.trim()) {
      void reloadList(selectedFolderRef.current, queryRef.current);
    }
    if (hasAccount) {
      void listAliases(pb)
        .then(setAliases)
        .catch((err: unknown) => console.error("listAliases failed", err));
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

  const openCompose = async (
    messageId: string,
    mode: ComposeMode = "reply",
    useSuggestedReply = false,
  ) => {
    const nextPrefill = await composeReply(pb, messageId, useSuggestedReply, mode);
    setComposePrefill(nextPrefill);
    setComposeOpen(true);
  };

  const findFolderByRole = (roles: string[], nameHints: string[] = []) => {
    const roleSet = new Set(roles.map((r) => r.toLowerCase()));
    const byRole = folders.find((f) => roleSet.has(f.role.toLowerCase()));
    if (byRole) return byRole;
    return folders.find((f) =>
      nameHints.some((hint) => f.name.toLowerCase().includes(hint.toLowerCase())),
    );
  };

  const afterMessageAction = async () => {
    clearMailSelection();
    await refreshFolders();
    refreshCurrentList();
  };

  const moveSelectedMessage = async (messageId: string, folderId: string) => {
    await moveMessage(pb, messageId, { folderId });
    await afterMessageAction();
  };

  const archiveMessage = async (messageId: string) => {
    const folder = findFolderByRole(["archive"], ["archive", "all mail"]);
    if (!folder) throw new Error("No archive folder found");
    await moveMessage(pb, messageId, { folderId: folder.id });
    await afterMessageAction();
  };

  const spamMessage = async (messageId: string) => {
    await moveMessage(pb, messageId, { toSpam: true });
    await afterMessageAction();
  };

  const deleteMessage = async (messageId: string) => {
    const folder = findFolderByRole(["trash"], ["trash", "deleted"]);
    if (!folder) throw new Error("No trash folder found");
    await moveMessage(pb, messageId, { folderId: folder.id });
    await afterMessageAction();
  };

  if (!ready) {
    return (
      <div className="setup-screen">
        <p className="hint">Connecting to local backend…</p>
      </div>
    );
  }

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
      </div>
    );
  }

  return (
    <AppChrome
      viewport={viewport}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      title={activeTab === "mail" ? mailMeta?.title : undefined}
      showBack={activeTab === "mail" && Boolean(mailMeta?.canBack)}
      onBack={mailMeta?.onBack}
      search={query}
      onSearchChange={setQuery}
      showSearch={activeTab === "mail"}
      onCompose={() => {
        setComposePrefill(undefined);
        setComposeOpen(true);
      }}
      onSettings={() => setSettingsOpen(true)}
      onSync={() => void window.email.triggerSync()}
      status={status}
    >
      {notice ? (
        <p className="hint app-notice" role="status">
          {notice}
          <button type="button" onClick={() => setNotice(null)}>
            Dismiss
          </button>
        </p>
      ) : null}
      {activeTab === "mail" ? (
        <MailShell
          pb={pb}
          viewport={viewport}
          folders={folders}
          selectedFolder={selectedFolder}
          onSelectFolder={selectFolder}
          selectedAlias={selectedAlias}
          onAliasChange={(email) => {
            setSelectedAlias(email);
            clearMailSelection();
          }}
          aliases={aliases}
          selectedThread={selectedThread}
          threadMessages={threadMessages}
          onOpenThread={openThread}
          onSelectLoadedMessage={selectLoadedMessage}
          threadRefreshKey={threadRefreshKey}
          slots={slots}
          messageTotal={messageTotal}
          loadingMessages={loadingMessages}
          query={query}
          selectedMessage={selectedMessage}
          onSelectMessage={(m) => void selectMessage(m)}
          clearSelectedMessage={clearMailSelection}
          loadingBody={loadingBody}
          syncStatus={status}
          analyzerStatus={analyzerStatus}
          downloadingBody={loadingBody}
          analysisByMessage={analysisByMessage}
          onVisibleRange={(start, end, ids) => {
            setVisibleMessageIds((prev) =>
              prev.length === ids.length && prev.every((id, i) => id === ids[i]) ? prev : ids,
            );
            void ensurePages(selectedFolderRef.current, queryRef.current, start, end);
          }}
          onToggleFlag={async (msg) => {
            await pb.collection("messages").update(msg.id, { flagged: !msg.flagged });
            patchSlot(msg.id, { flagged: !msg.flagged });
          }}
          onToggleSeen={async (msg) => {
            await pb.collection("messages").update(msg.id, { seen: !msg.seen });
            patchSlot(msg.id, { seen: !msg.seen });
          }}
          onApplyAnalysis={applyAnalysis}
          onReanalyze={reanalyze}
          onCompose={openCompose}
          onMoveMessage={moveSelectedMessage}
          onArchiveMessage={archiveMessage}
          onSpamMessage={spamMessage}
          onDeleteMessage={deleteMessage}
          onPaneMeta={handlePaneMeta}
        />
      ) : null}

      {activeTab === "todos" ? <TodoList pb={pb} active={activeTab === "todos"} /> : null}
      {activeTab === "calendar" ? <CalendarView pb={pb} active={activeTab === "calendar"} /> : null}

      {composeOpen && (
        <ComposeModal
          pb={pb}
          prefill={composePrefill}
          fromOptions={fromOptions}
          onClose={() => {
            setComposeOpen(false);
            setComposePrefill(undefined);
          }}
          onSaved={() => {
            refreshCurrentList();
            setNotice("Draft saved");
          }}
          onSent={(result) => {
            refreshCurrentList();
            setNotice(result?.warning ? `Sent (${result.warning})` : "Message sent");
          }}
        />
      )}
      {eventApplyGate ? (
        <CreateEventModal
          pb={pb}
          calendars={applyCalendars}
          defaultTimezone={
            eventApplyGate.initial.timezone ||
            Intl.DateTimeFormat().resolvedOptions().timeZone ||
            "UTC"
          }
          initial={eventApplyGate.initial}
          saveEvent={async (body) => {
            await completeEventApply(pb, eventApplyGate.analysis, body);
          }}
          onClose={() => {
            setEventApplyGate((prev) => {
              if (prev) prev.reject(new Error("cancelled"));
              return null;
            });
          }}
          onSaved={() => {
            setEventApplyGate((prev) => {
              prev?.resolve();
              return null;
            });
          }}
        />
      ) : null}
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
    </AppChrome>
  );
}
