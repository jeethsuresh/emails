import type PocketBase from "pocketbase";
import type {
  AnalysisPriority,
  AnalyzerSettings,
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

export async function loadAnalysesForMessages(
  pb: PocketBase,
  ids: string[],
): Promise<Record<string, MessageAnalysis>> {
  if (ids.length === 0) return {};
  const rows = await pb.collection("message_analysis").getFullList<MessageAnalysis>({
    filter: buildMessageAnalysisFilter(ids),
    batch: 200,
  });
  const map: Record<string, MessageAnalysis> = {};
  for (const row of rows) map[row.message] = row;
  return map;
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
    body: { model: settings.model, baseUrl: settings.baseUrl },
  });
}

/**
 * Carries out an analysis's suggested_action. Moves are delegated to the syncer's
 * move endpoint; add_event/add_todo create scaffold PocketBase records the user
 * can flesh out later.
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
      await pb.collection("events").create({
        title,
        notes: "",
        source_message: analysis.message,
        created_at: new Date().toISOString(),
      });
      return;
    case "add_todo":
      await pb.collection("todos").create({
        title,
        notes: "",
        source_message: analysis.message,
        created_at: new Date().toISOString(),
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
