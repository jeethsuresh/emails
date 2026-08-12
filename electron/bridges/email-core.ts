import fs from "node:fs";

export interface EmailCore {
  mimeHeaderGet(raw: string, name: string): string;
  hashBlakeLike(input: string): string;
  indexTokenize(text: string): string[];
  contactNormalize(email: string): string;
}

/**
 * Loads C-compiled WASM hot-path module (parse / search / crypto helpers).
 */
export async function loadEmailCore(wasmPath: string): Promise<EmailCore> {
  if (!fs.existsSync(wasmPath)) {
    // Dev fallback so Electron can boot before first native build.
    return {
      mimeHeaderGet: (raw, name) => fallbackMimeHeader(raw, name),
      hashBlakeLike: (input) => simpleHash(input),
      indexTokenize: (text) =>
        text
          .toLowerCase()
          .split(/[^a-z0-9@._+-]+/)
          .filter(Boolean),
      contactNormalize: (email) => email.trim().toLowerCase(),
    };
  }

  const bytes = fs.readFileSync(wasmPath);
  const { instance } = await WebAssembly.instantiate(bytes, {
    env: {
      // abort stub for clang wasm
      abort: () => {
        throw new Error("email_core abort");
      },
    },
  });

  const exports = instance.exports as Record<string, WebAssembly.ExportValue>;
  const memory = exports.memory as WebAssembly.Memory;
  const alloc = exports.alloc as (n: number) => number;
  const dealloc = exports.dealloc as (p: number, n: number) => void;

  const readCString = (ptr: number) => {
    const view = new Uint8Array(memory.buffer);
    let end = ptr;
    while (view[end] !== 0) end++;
    return new TextDecoder().decode(view.subarray(ptr, end));
  };

  const writeString = (s: string) => {
    const bytes = new TextEncoder().encode(s + "\0");
    const ptr = alloc(bytes.length);
    new Uint8Array(memory.buffer).set(bytes, ptr);
    return { ptr, len: bytes.length };
  };

  const callStringFn = (fnName: string, ...args: string[]) => {
    const fn = exports[fnName] as (...ptrs: number[]) => number;
    const allocated = args.map(writeString);
    try {
      const outPtr = fn(...allocated.map((a) => a.ptr));
      return readCString(outPtr);
    } finally {
      for (const a of allocated) dealloc(a.ptr, a.len);
    }
  };

  return {
    mimeHeaderGet: (raw, name) => callStringFn("mime_header_get", raw, name),
    hashBlakeLike: (input) => callStringFn("hash_blake_like", input),
    indexTokenize: (text) =>
      callStringFn("index_tokenize", text)
        .split("\n")
        .filter(Boolean),
    contactNormalize: (email) => callStringFn("contact_normalize", email),
  };
}

function fallbackMimeHeader(raw: string, name: string): string {
  const re = new RegExp(`^${name}:\\s*(.*)$`, "im");
  const m = raw.match(re);
  return m?.[1]?.trim() ?? "";
}

function simpleHash(input: string): string {
  let h = 2166136261;
  for (let i = 0; i < input.length; i++) {
    h ^= input.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return (h >>> 0).toString(16).padStart(8, "0");
}
