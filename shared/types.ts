export type SyncState = "idle" | "syncing" | "error" | "offline";

export type TlsMode = "tls" | "starttls" | "none";

export interface SyncStatus {
  state: SyncState;
  message: string;
  phase: string;
  currentFolder: string;
  lastSyncAt: string | null;
  foldersSynced: number;
  foldersTotal: number;
  messagesSynced: number;
  logs: string[];
}

export type AnalysisStatus = "pending" | "running" | "done" | "skipped";

export type AnalysisPriority = "high" | "medium" | "low";

export type SuggestedAction =
  | "move_to_folder"
  | "move_to_spam"
  | "add_event"
  | "add_todo";

export interface MessageAnalysis {
  id: string;
  message: string;
  status: AnalysisStatus;
  priority: AnalysisPriority | "";
  suggested_action: SuggestedAction | "";
  action_target: string;
  suggested_reply: string;
  model: string;
  error: string;
  fail_count: number;
  analyzed_at: string;
}

export interface AnalyzerSettings {
  model: string;
  baseUrl: string;
  syncIntervalMinutes: number; // 1–60, default 5
}

/** Combined app settings payload (analyzer + sync). Same shape as AnalyzerSettings. */
export type AppSettings = AnalyzerSettings;

export type AnalyzerWorkerState = "idle" | "running" | "paused";

export interface AnalyzerStatus {
  state: AnalyzerWorkerState;
  queueDepth: number;
  currentMessageId: string;
  message: string;
  model: string;
}

export type ItemStatus = "draft" | "approved";

export interface TodoItem {
  id: string;
  title: string;
  notes: string;
  source_message: string;
  created_at: string;
  deadline: string;
  status: ItemStatus;
}

export interface EventItem {
  id: string;
  title: string;
  notes: string;
  source_message: string;
  created_at: string;
  starts_at: string;
  ends_at: string;
  status: ItemStatus;
  calendar: string;
  all_day: boolean;
  timezone: string;
  uid: string;
}

export interface CalendarItem {
  id: string;
  name: string;
  color: string;
  timezone: string;
  source: string;
  is_visible: boolean;
  is_default: boolean;
}

export interface MailAccountInput {
  email: string;
  imapHost: string;
  imapPort: number;
  imapTLS?: boolean;
  imapSecurity?: TlsMode;
  smtpHost: string;
  smtpPort: number;
  smtpTLS?: boolean;
  smtpSecurity?: TlsMode;
  tlsInsecure?: boolean;
  username: string;
  password: string;
}
