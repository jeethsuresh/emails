import type PocketBase from "pocketbase";

export const MESSAGE_PAGE_SIZE = 75;
/** Keep current page ± this many pages loaded. */
export const MESSAGE_PAGE_RADIUS = 1;

export interface ListMessage {
  id: string;
  uid?: number;
  subject: string;
  from_addr: string;
  date: string;
  snippet: string;
  seen: boolean;
  flagged: boolean;
  folder: string;
  body_text: string;
  body_html?: string;
}

const LIST_FIELDS =
  "id,uid,subject,from_addr,to_addrs,date,snippet,seen,flagged,folder";

export function pageForIndex(index: number): number {
  return Math.floor(index / MESSAGE_PAGE_SIZE) + 1;
}

export function pageStartIndex(page: number): number {
  return (page - 1) * MESSAGE_PAGE_SIZE;
}

export function pagesForRange(start: number, end: number): number[] {
  if (end < start) return [];
  const first = Math.max(1, pageForIndex(start) - MESSAGE_PAGE_RADIUS);
  const last = pageForIndex(Math.max(0, end)) + MESSAGE_PAGE_RADIUS;
  const pages: number[] = [];
  for (let p = first; p <= last; p++) pages.push(p);
  return pages;
}

export function toListMessage(m: Partial<ListMessage> & { id: string }): ListMessage {
  return {
    id: m.id,
    uid: m.uid,
    subject: m.subject ?? "",
    from_addr: m.from_addr ?? "",
    date: m.date ?? "",
    snippet: m.snippet ?? "",
    seen: Boolean(m.seen),
    flagged: Boolean(m.flagged),
    folder: m.folder ?? "",
    body_text: "",
    body_html: "",
  };
}

export async function fetchMessagePage(
  pb: PocketBase,
  filter: string,
  page: number,
): Promise<{ items: ListMessage[]; totalItems: number; totalPages: number }> {
  const result = await pb.collection("messages").getList<ListMessage>(page, MESSAGE_PAGE_SIZE, {
    filter,
    sort: "-date,-uid",
    fields: LIST_FIELDS,
  });
  return {
    items: result.items.map((row) => toListMessage(row)),
    totalItems: result.totalItems,
    totalPages: result.totalPages,
  };
}

/** Merge fetched page into a sparse slot array; returns a new array reference. */
export function mergePageIntoSlots(
  prev: Array<ListMessage | null>,
  totalItems: number,
  page: number,
  items: ListMessage[],
): Array<ListMessage | null> {
  const next =
    prev.length === totalItems ? prev.slice() : Array.from({ length: totalItems }, (_, i) => prev[i] ?? null);
  const start = pageStartIndex(page);
  for (let i = 0; i < items.length; i++) {
    next[start + i] = items[i]!;
  }
  // Clear trailing slots if this was a short last page after a shrink.
  if (page * MESSAGE_PAGE_SIZE >= totalItems) {
    for (let i = start + items.length; i < next.length; i++) next[i] = null;
  }
  return next;
}

/** Drop pages outside keep set to free message object memory. */
export function evictPagesOutside(
  slots: Array<ListMessage | null>,
  keepPages: Set<number>,
): Array<ListMessage | null> {
  let changed = false;
  const next = slots.slice();
  for (let i = 0; i < next.length; i++) {
    if (next[i] == null) continue;
    const page = pageForIndex(i);
    if (!keepPages.has(page)) {
      next[i] = null;
      changed = true;
    }
  }
  return changed ? next : slots;
}
