import type PocketBase from "pocketbase";
import type {
  AnalysisPriority,
  AnalyzerSettings,
  AnalyzerStatus,
  MessageAnalysis,
} from "../../shared/types";

export function escapeFilterValue(value: string): string {
  return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

/** Compact list/reader label for a done analysis's priority. */
export function priorityLabel(priority: AnalysisPriority | ""): string | null {
  switch (priority) {
    case "high":
      return "High";
    case "medium":
      return "Med";
    case "low":
      return "Low";
    case "":
      return null;
    default: {
      const _exhaustive: never = priority;
      return _exhaustive;
    }
  }
}

/** Builds a `message = "id1" || message = "id2" || …` filter for message_analysis lookups. */
export function buildMessageAnalysisFilter(ids: string[]): string {
  return ids.map((id) => `message = "${escapeFilterValue(id)}"`).join(" || ");
}

const ANALYSIS_LIST_FIELDS =
  "id,message,status,priority,suggested_action,action_target,model,analyzed_at,fail_count,error";

/** Keep OR-filters small — thousands of ids in one filter OOMs the renderer/PB. */
const ANALYSIS_ID_CHUNK = 75;

export async function loadAnalysesForMessages(
  pb: PocketBase,
  ids: string[],
): Promise<Record<string, MessageAnalysis>> {
  if (ids.length === 0) return {};
  const map: Record<string, MessageAnalysis> = {};
  for (let i = 0; i < ids.length; i += ANALYSIS_ID_CHUNK) {
    const chunk = ids.slice(i, i + ANALYSIS_ID_CHUNK);
    const rows = await pb.collection("message_analysis").getFullList<MessageAnalysis>({
      filter: buildMessageAnalysisFilter(chunk),
      fields: ANALYSIS_LIST_FIELDS,
      batch: 100,
    });
    for (const row of rows) {
      map[row.message] = {
        ...row,
        suggested_reply: row.suggested_reply ?? "",
      };
    }
  }
  return map;
}

export async function getAnalyzerStatus(pb: PocketBase): Promise<AnalyzerStatus> {
  return pb.send<AnalyzerStatus>("/api/email/analyzer/status", { method: "GET" });
}

export async function getAnalyzerSettings(pb: PocketBase): Promise<AnalyzerSettings> {
  return pb.send<AnalyzerSettings>("/api/email/analyzer/settings", { method: "GET" });
}

export async function saveAnalyzerSettings(
  pb: PocketBase,
  settings: AnalyzerSettings,
): Promise<AnalyzerSettings> {
  return pb.send<AnalyzerSettings>("/api/email/analyzer/settings", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: {
      model: settings.model,
      baseUrl: settings.baseUrl,
      syncIntervalMinutes: settings.syncIntervalMinutes,
    },
  });
}

async function approveOrCreateItem(
  pb: PocketBase,
  collection: "todos" | "events",
  sourceMessage: string,
  title: string,
  extras: Record<string, string>,
): Promise<void> {
  const existing = await pb.collection(collection).getFullList<{ id: string; status?: string }>({
    filter: `source_message = "${escapeFilterValue(sourceMessage)}"`,
    fields: "id,status",
    batch: 50,
  });
  const draft = existing.find((row) => (row.status ?? "") === "draft" || (row.status ?? "") === "");
  if (draft) {
    await pb.collection(collection).update(draft.id, { status: "approved", title });
    return;
  }
  if (existing.some((row) => row.status === "approved")) {
    return;
  }
  await pb.collection(collection).create({
    title,
    notes: "",
    source_message: sourceMessage,
    created_at: new Date().toISOString(),
    status: "approved",
    ...extras,
  });
}

/**
 * Carries out an analysis's suggested_action. Moves are delegated to the syncer's
 * move endpoint; add_event/add_todo approve an existing draft or create approved.
 */
export async function applyAnalysisAction(
  pb: PocketBase,
  analysis: MessageAnalysis,
  subject: string,
): Promise<void> {
  const title = analysis.action_target || subject || "(no subject)";
  switch (analysis.suggested_action) {
    case "move_to_folder":
      await pb.send(`/api/email/messages/${encodeURIComponent(analysis.message)}/move`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: { folderName: analysis.action_target },
      });
      return;
    case "move_to_spam":
      await pb.send(`/api/email/messages/${encodeURIComponent(analysis.message)}/move`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: { toSpam: true },
      });
      return;
    case "add_event":
      await approveOrCreateItem(pb, "events", analysis.message, title, {
        starts_at: "",
        ends_at: "",
      });
      return;
    case "add_todo":
      await approveOrCreateItem(pb, "todos", analysis.message, title, {
        deadline: "",
      });
      return;
    case "":
      return;
    default: {
      const _exhaustive: never = analysis.suggested_action;
      return _exhaustive;
    }
  }
}
