import type PocketBase from "pocketbase";

export interface ComposePrefill {
  from: string;
  to: string[];
  cc: string[];
  subject: string;
  bodyText: string;
  inReplyTo: string;
  references: string;
  threadId: string;
}

export interface SendBody extends ComposePrefill {
  draftId?: string;
}

export interface MailThread {
  id: string;
  subject: string;
  normalized_subject: string;
  snippet: string;
  last_date: string;
  message_count: number;
  participants: string;
  received_for: string;
  folder: string;
  unread_count: number;
  updated_at: string;
}

export interface ThreadMessage {
  id: string;
  account: string;
  folder: string;
  uid: number;
  message_id: string;
  subject: string;
  from_addr: string;
  to_addrs: string;
  date: string;
  snippet: string;
  body_text: string;
  body_html: string;
  seen: boolean;
  flagged: boolean;
  in_reply_to: string;
  references: string;
  thread_id: string;
  received_for: string;
  normalized_subject: string;
}

export interface Contact {
  id: string;
  email: string;
  display_name: string;
  graph_json: string;
  last_message_at: string;
  message_count: number;
  updated_at: string;
}

export interface Page<T> {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export async function listAliases(
  pb: PocketBase,
): Promise<Array<{ email: string; count: number }>> {
  return pb.send<Array<{ email: string; count: number }>>("/api/email/aliases", {
    method: "GET",
  });
}

export async function listThreads(
  pb: PocketBase,
  opts: { folder?: string; received_for?: string; page?: number } = {},
): Promise<Page<MailThread>> {
  const query = new URLSearchParams();
  if (opts.folder) query.set("folder", opts.folder);
  if (opts.received_for) query.set("received_for", opts.received_for);
  if (opts.page) query.set("page", String(opts.page));
  const suffix = query.size > 0 ? `?${query}` : "";
  return pb.send<Page<MailThread>>(`/api/email/threads${suffix}`, { method: "GET" });
}

export async function getThread(
  pb: PocketBase,
  id: string,
  folder?: string,
): Promise<{ thread: MailThread; messages: ThreadMessage[] }> {
  const query = folder ? `?folder=${encodeURIComponent(folder)}` : "";
  return pb.send<{ thread: MailThread; messages: ThreadMessage[] }>(
    `/api/email/threads/${encodeURIComponent(id)}${query}`,
    { method: "GET" },
  );
}

export async function listContacts(
  pb: PocketBase,
  q?: string,
  page?: number,
): Promise<Page<Contact>> {
  const query = new URLSearchParams();
  if (q) query.set("q", q);
  if (page) query.set("page", String(page));
  const suffix = query.size > 0 ? `?${query}` : "";
  return pb.send<Page<Contact>>(`/api/email/contacts${suffix}`, { method: "GET" });
}

export async function contactMessages(
  pb: PocketBase,
  email: string,
  page?: number,
): Promise<Page<ThreadMessage>> {
  const query = new URLSearchParams();
  if (page) query.set("page", String(page));
  const suffix = query.size > 0 ? `?${query}` : "";
  return pb.send<Page<ThreadMessage>>(
    `/api/email/contacts/${encodeURIComponent(email)}/messages${suffix}`,
    { method: "GET" },
  );
}

export type ComposeMode = "reply" | "reply_all" | "forward";

export async function composeReply(
  pb: PocketBase,
  messageId: string,
  useSuggestedReply = false,
  mode: ComposeMode = "reply",
): Promise<ComposePrefill> {
  return pb.send<ComposePrefill>("/api/email/compose/reply", {
    method: "POST",
    body: { messageId, useSuggestedReply, mode },
  });
}

export async function moveMessage(
  pb: PocketBase,
  messageId: string,
  body: { folderId?: string; folderName?: string; toSpam?: boolean; createFolder?: boolean },
): Promise<void> {
  await pb.send(`/api/email/messages/${encodeURIComponent(messageId)}/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
  });
}

export async function sendMail(
  pb: PocketBase,
  body: SendBody,
): Promise<{ messageId: string; threadId: string; warning?: string }> {
  return pb.send<{ messageId: string; threadId: string; warning?: string }>("/api/email/send", {
    method: "POST",
    body,
  });
}
