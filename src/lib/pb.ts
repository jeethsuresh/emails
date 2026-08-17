import PocketBase from "pocketbase";

function asHeaderPairs(headers: unknown): [string, string][] {
  if (!headers) return [];
  if (Array.isArray(headers)) {
    return headers.map((row) => {
      if (Array.isArray(row) && row.length >= 2) {
        return [String(row[0]), String(row[1])];
      }
      return [String(row), ""];
    });
  }
  if (typeof headers === "object") {
    return Object.entries(headers as Record<string, unknown>).map(([k, v]) => [
      k,
      String(v),
    ]);
  }
  return [];
}

function asArrayBuffer(body: unknown): ArrayBuffer | null {
  if (body == null) return null;
  if (body instanceof ArrayBuffer) return body;
  if (body instanceof Uint8Array) {
    return Uint8Array.from(body).buffer;
  }
  if (typeof body === "string") {
    return new TextEncoder().encode(body).buffer;
  }
  return new TextEncoder().encode(JSON.stringify(body)).buffer;
}

function prepareQueryParamValue(value: unknown): string | null {
  if (value == null) return null;
  if (value instanceof Date) {
    return encodeURIComponent(value.toISOString().replace("T", " "));
  }
  if (typeof value === "object") {
    return encodeURIComponent(JSON.stringify(value));
  }
  return encodeURIComponent(String(value));
}

/** Mirror PocketBase JS SDK query serialization (filter/sort/page/…). */
function serializeQueryParams(query: Record<string, unknown>): string {
  const parts: string[] = [];
  for (const key of Object.keys(query)) {
    const encodedKey = encodeURIComponent(key);
    const raw = query[key];
    const values = Array.isArray(raw) ? raw : [raw];
    for (const value of values) {
      const prepared = prepareQueryParamValue(value);
      if (prepared !== null) parts.push(`${encodedKey}=${prepared}`);
    }
  }
  return parts.join("&");
}

function getHeader(
  headers: HeadersInit | ReadonlyArray<readonly [string, string]> | undefined,
  name: string,
): string | null {
  if (!headers) return null;
  const lower = name.toLowerCase();
  if (headers instanceof Headers) return headers.get(name);
  if (Array.isArray(headers)) {
    for (const row of headers) {
      if (String(row[0]).toLowerCase() === lower) return String(row[1]);
    }
    return null;
  }
  for (const [k, v] of Object.entries(headers)) {
    if (k.toLowerCase() === lower) return String(v);
  }
  return null;
}

/** Same reserved keys as PocketBase's normalizeUnknownQueryParams — everything else is a query param. */
const SEND_OPTION_KEYS = new Set([
  "requestKey",
  "$cancelKey",
  "$autoCancel",
  "fetch",
  "headers",
  "body",
  "query",
  "params",
  "cache",
  "credentials",
  "integrity",
  "keepalive",
  "method",
  "mode",
  "redirect",
  "referrer",
  "referrerPolicy",
  "signal",
  "window",
]);

/**
 * PocketBase SDK moves unknown options (filter/sort/fields/…) into `query`.
 * Our IPC `send` override must do the same or every list request ships full bodies.
 */
function normalizeUnknownQueryParams(
  options: Record<string, unknown> & { query?: Record<string, unknown>; params?: Record<string, unknown> },
): void {
  options.query = { ...(options.params ?? {}), ...(options.query ?? {}) };
  for (const key of Object.keys(options)) {
    if (SEND_OPTION_KEYS.has(key)) continue;
    options.query[key] = options[key];
    delete options[key];
  }
}

/**
 * PocketBase JS client that routes HTTP through Electron IPC into the local backend.
 */
export function createPbClient() {
  const pb = new PocketBase("http://email.local");
  // Sync status polls / folder refresh would otherwise cancel in-flight list requests
  // and our error handler was wiping the message list.
  pb.autoCancellation(false);

  pb.send = async (pathName, options = {}) => {
    const opts = { ...options } as Record<string, unknown> & {
      method?: string;
      headers?: HeadersInit;
      body?: unknown;
      query?: Record<string, unknown>;
      params?: Record<string, unknown>;
    };

    normalizeUnknownQueryParams(opts);

    let url =
      typeof pb.buildURL === "function"
        ? pb.buildURL(pathName)
        : (pb as unknown as { buildUrl: (p: string) => string }).buildUrl(pathName);

    // PocketBase SDK normally appends options.query here before fetch — we must too.
    if (opts.query && typeof opts.query === "object") {
      const qs = serializeQueryParams(opts.query);
      if (qs) url += (url.includes("?") ? "&" : "?") + qs;
      delete opts.query;
    }

    const method = (opts.method ?? "GET").toUpperCase();
    const headers: [string, string][] = [];
    const h = opts.headers;
    if (h instanceof Headers) {
      h.forEach((v, k) => headers.push([k, v]));
    } else if (Array.isArray(h)) {
      for (const row of h) headers.push([String(row[0]), String(row[1])]);
    } else if (h) {
      for (const [k, v] of Object.entries(h)) headers.push([k, String(v)]);
    }

    let bodyRaw = opts.body ?? null;
    const isForm =
      typeof FormData !== "undefined" &&
      bodyRaw &&
      (bodyRaw instanceof FormData ||
        (typeof bodyRaw === "object" &&
          (bodyRaw as { constructor?: { name?: string } }).constructor?.name === "FormData"));

    // PocketBase's stock Client.send() adds Content-Type + JSON.stringify via
    // initSendOptions. Our IPC override must do the same or PATCH/POST bodies
    // arrive as opaque bytes with no Content-Type ("Unsupported Content-Type").
    if (bodyRaw != null && !isForm && method !== "GET" && method !== "HEAD") {
      if (!getHeader(headers, "Content-Type")) {
        headers.push(["Content-Type", "application/json"]);
      }
      if (
        getHeader(headers, "Content-Type")?.toLowerCase().includes("application/json") &&
        typeof bodyRaw !== "string" &&
        !(bodyRaw instanceof ArrayBuffer) &&
        !(bodyRaw instanceof Uint8Array)
      ) {
        bodyRaw = JSON.stringify(bodyRaw);
      }
    }
    const body = isForm
      ? null // FormData cannot cross IPC; callers must not send files through this path
      : asArrayBuffer(bodyRaw);

    const res = await window.email.pbFetch({ method, url, headers, body });
    const responseHeaders = new Headers();
    for (const [k, v] of asHeaderPairs(res.headers)) {
      responseHeaders.set(k, v);
    }

    const contentType = responseHeaders.get("content-type") ?? "";
    const raw =
      res.body instanceof ArrayBuffer
        ? new Uint8Array(res.body)
        : res.body
          ? new Uint8Array(res.body as ArrayBuffer)
          : new Uint8Array();

    let data: unknown = {};
    if (contentType.includes("application/json") || raw.byteLength > 0) {
      const text = new TextDecoder().decode(raw);
      if (contentType.includes("application/json") || text.startsWith("{") || text.startsWith("[")) {
        try {
          data = JSON.parse(text);
        } catch {
          data = text;
        }
      } else {
        data = text;
      }
    }

    if (res.status >= 400) {
      const detail =
        data && typeof data === "object"
          ? JSON.stringify(data).slice(0, 400)
          : String(data ?? "");
      throw Object.assign(
        new Error(`PocketBase request failed (${res.status}) ${url} ${detail}`),
        {
          status: res.status,
          response: { data },
        },
      );
    }
    return data as never;
  };

  return pb;
}
