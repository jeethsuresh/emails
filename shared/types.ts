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
