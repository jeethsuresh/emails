import type PocketBase from "pocketbase";
import type {
  AnalysisPriority,
  AnalyzerSettings,
  AnalyzerStatus,
  MessageAnalysis,
} from "../../shared/types";
import {
  createCalendarEvent,
  updateCalendarEvent,
  type EventWriteInput,
} from "./calendarApi";

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
  "id,message,status,priority,suggested_action,action_target,create_folder,event_starts_at,event_ends_at,event_attendees,model,analyzed_at,fail_count,error";
const ANALYSIS_READER_FIELDS = `${ANALYSIS_LIST_FIELDS},suggested_reply`;

/** Keep OR-filters small — thousands of ids in one filter OOMs the renderer/PB. */
const ANALYSIS_ID_CHUNK = 75;

export async function loadAnalysesForMessages(
  pb: PocketBase,
  ids: string[],
  opts: { includeReply?: boolean } = {},
): Promise<Record<string, MessageAnalysis>> {
  if (ids.length === 0) return {};
  const map: Record<string, MessageAnalysis> = {};
  const fields = opts.includeReply ? ANALYSIS_READER_FIELDS : ANALYSIS_LIST_FIELDS;
  for (let i = 0; i < ids.length; i += ANALYSIS_ID_CHUNK) {
    const chunk = ids.slice(i, i + ANALYSIS_ID_CHUNK);
    const rows = await pb.collection("message_analysis").getFullList<MessageAnalysis>({
      filter: buildMessageAnalysisFilter(chunk),
      fields,
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

/** Queue a message for a fresh AI analysis (clears prior result). */
export async function reanalyzeMessage(
  pb: PocketBase,
  messageId: string,
): Promise<{ ok: boolean; status: string }> {
  return pb.send<{ ok: boolean; status: string }>("/api/email/analyzer/reanalyze", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: { messageId },
  });
}

export function parseAttendeesField(raw: string | string[] | undefined | null): string[] {
  if (Array.isArray(raw)) {
    return normalizeAttendeeEmails(raw);
  }
  const text = (raw ?? "").trim();
  if (!text) return [];
  if (text.startsWith("[")) {
    try {
      const parsed = JSON.parse(text) as unknown;
      if (Array.isArray(parsed)) {
        return normalizeAttendeeEmails(parsed.map((v) => String(v)));
      }
    } catch {
      /* fall through */
    }
  }
  return normalizeAttendeeEmails(
    text.split(/[,;\n\r]+/).map((part) => part.trim()),
  );
}

function normalizeAttendeeEmails(emails: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const email of emails) {
    const normalized = email.trim().toLowerCase();
    if (!normalized.includes("@") || seen.has(normalized)) continue;
    seen.add(normalized);
    out.push(normalized);
  }
  return out;
}

/** Convert RFC3339 to datetime-local wall clock in the given IANA timezone. */
export function isoToWallClock(iso: string, timeZone: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: timeZone || "UTC",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).formatToParts(d);
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  const y = get("year");
  const m = get("month");
  const day = get("day");
  const h = get("hour");
  const min = get("minute");
  if (!y || !m || !day) return "";
  return `${y}-${m}-${day}T${h.padStart(2, "0")}:${min.padStart(2, "0")}`;
}

export type ApplyAnalysisResult =
  | { status: "done" }
  | {
      status: "needs_event_details";
      title: string;
      initial: Partial<EventWriteInput>;
      analysis: MessageAnalysis;
    };

async function approveOrCreateTodo(
  pb: PocketBase,
  sourceMessage: string,
  title: string,
): Promise<void> {
  const existing = await pb.collection("todos").getFullList<{ id: string; status?: string }>({
    filter: `source_message = "${escapeFilterValue(sourceMessage)}"`,
    fields: "id,status",
    batch: 50,
  });
  const draft = existing.find((row) => (row.status ?? "") === "draft" || (row.status ?? "") === "");
  if (draft) {
    await pb.collection("todos").update(draft.id, { status: "approved", title });
    return;
  }
  if (existing.some((row) => row.status === "approved" || row.status === "completed")) {
    return;
  }
  await pb.collection("todos").create({
    title,
    notes: "",
    source_message: sourceMessage,
    created_at: new Date().toISOString(),
    status: "approved",
    deadline: "",
  });
}

async function findEventDraftId(pb: PocketBase, sourceMessage: string): Promise<string | null> {
  const existing = await pb.collection("events").getFullList<{ id: string; status?: string }>({
    filter: `source_message = "${escapeFilterValue(sourceMessage)}"`,
    fields: "id,status",
    batch: 50,
  });
  const draft = existing.find((row) => (row.status ?? "") === "draft" || (row.status ?? "") === "");
  return draft?.id ?? null;
}

async function defaultCalendarId(pb: PocketBase): Promise<string> {
  try {
    const rows = await pb.collection("calendars").getFullList<{ id: string; is_default?: boolean }>({
      fields: "id,is_default",
      batch: 50,
    });
    return rows.find((row) => row.is_default)?.id ?? rows[0]?.id ?? "";
  } catch {
    return "";
  }
}

async function recordMailAction(
  pb: PocketBase,
  messageId: string,
  action: string,
  target: string,
): Promise<void> {
  try {
    const message = await pb.collection("messages").getOne<{
      from_addr?: string;
      received_for?: string;
    }>(messageId, { fields: "id,from_addr,received_for" });
    await pb.collection("mail_actions").create({
      message: messageId,
      from_addr: (message.from_addr ?? "").toLowerCase(),
      received_for: (message.received_for ?? "").toLowerCase(),
      action,
      target,
      created_at: new Date().toISOString(),
    });
  } catch (err) {
    console.warn("recordMailAction failed", err);
  }
}

export function eventDetailsFromAnalysis(
  analysis: MessageAnalysis,
  title: string,
  calendarId: string,
  timezone: string,
): Partial<EventWriteInput> {
  const starts = (analysis.event_starts_at ?? "").trim();
  let ends = (analysis.event_ends_at ?? "").trim();
  let startWall = starts ? isoToWallClock(starts, timezone) : "";
  let endWall = ends ? isoToWallClock(ends, timezone) : "";
  if (startWall && !endWall) {
    const startMs = Date.parse(starts);
    if (!Number.isNaN(startMs)) {
      ends = new Date(startMs + 60 * 60 * 1000).toISOString();
      endWall = isoToWallClock(ends, timezone);
    }
  }
  return {
    title,
    calendarId,
    allDay: false,
    timezone,
    startWall,
    endWall,
    attendees: parseAttendeesField(analysis.event_attendees),
    status: "approved",
    sourceMessage: analysis.message,
  };
}

/** Persist an approved event from Apply (with required title/start/end). */
export async function completeEventApply(
  pb: PocketBase,
  analysis: MessageAnalysis,
  body: EventWriteInput,
): Promise<void> {
  const title = body.title.trim();
  if (!title || !body.startWall.trim() || !body.endWall.trim()) {
    throw new Error("Event title, start, and end are required");
  }
  const draftId = await findEventDraftId(pb, analysis.message);
  const payload: EventWriteInput = {
    ...body,
    title,
    status: "approved",
    sourceMessage: analysis.message,
    attendees: body.attendees ?? [],
  };
  if (draftId) {
    await updateCalendarEvent(pb, draftId, payload);
  } else {
    await createCalendarEvent(pb, payload);
  }
  await recordMailAction(pb, analysis.message, "add_event", title);
}

/**
 * Carries out an analysis's suggested_action. Moves are delegated to the syncer's
 * move endpoint; add_event may return needs_event_details when times are missing.
 */
export async function applyAnalysisAction(
  pb: PocketBase,
  analysis: MessageAnalysis,
  subject: string,
): Promise<ApplyAnalysisResult> {
  const title = analysis.action_target || subject || "(no subject)";
  switch (analysis.suggested_action) {
    case "move_to_folder":
      await pb.send(`/api/email/messages/${encodeURIComponent(analysis.message)}/move`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: {
          folderName: analysis.action_target,
          createFolder: true,
        },
      });
      return { status: "done" };
    case "move_to_spam":
      await pb.send(`/api/email/messages/${encodeURIComponent(analysis.message)}/move`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: { toSpam: true },
      });
      return { status: "done" };
    case "add_event": {
      const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
      const calendarId = await defaultCalendarId(pb);
      const starts = (analysis.event_starts_at ?? "").trim();
      const initial = eventDetailsFromAnalysis(analysis, title, calendarId, timezone);
      // Start alone is enough; end defaults to start+1h. Modal only if start missing.
      if (!starts || !initial.startWall) {
        return {
          status: "needs_event_details",
          title,
          initial,
          analysis,
        };
      }
      await completeEventApply(pb, analysis, {
        title,
        notes: "",
        calendarId,
        allDay: false,
        timezone,
        startWall: initial.startWall,
        endWall: initial.endWall || initial.startWall,
        attendees: initial.attendees ?? [],
        status: "approved",
        sourceMessage: analysis.message,
      });
      return { status: "done" };
    }
    case "add_todo":
      await approveOrCreateTodo(pb, analysis.message, title);
      await recordMailAction(pb, analysis.message, "add_todo", title);
      return { status: "done" };
    case "":
      return { status: "done" };
    default: {
      const _exhaustive: never = analysis.suggested_action;
      return _exhaustive;
    }
  }
}
