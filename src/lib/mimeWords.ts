/** Decode RFC 2047 encoded-words in email headers (e.g. =?utf-8?q?...?=). */
export function decodeMIMEWords(input: string): string {
  if (!input || !input.includes("=?")) return input;
  try {
    return input.replace(
      /=\?([^?]+)\?([bqBQ])\?([^?]*)\?=/g,
      (_full, charset: string, encoding: string, text: string) => {
        const cs = charset.toLowerCase();
        let bytes: Uint8Array;
        if (encoding.toLowerCase() === "b") {
          const bin = atob(text.replace(/\s+/g, ""));
          bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
        } else {
          // quoted-printable style inside encoded-word: _ = space
          const qp = text.replace(/_/g, " ").replace(/=\r?\n/g, "");
          const out: number[] = [];
          for (let i = 0; i < qp.length; i++) {
            if (qp[i] === "=" && i + 2 < qp.length) {
              const hex = qp.slice(i + 1, i + 3);
              if (/^[0-9a-fA-F]{2}$/.test(hex)) {
                out.push(parseInt(hex, 16));
                i += 2;
                continue;
              }
            }
            out.push(qp.charCodeAt(i));
          }
          bytes = Uint8Array.from(out);
        }
        try {
          return new TextDecoder(cs || "utf-8").decode(bytes);
        } catch {
          return new TextDecoder("utf-8").decode(bytes);
        }
      },
    );
  } catch {
    return input;
  }
}
