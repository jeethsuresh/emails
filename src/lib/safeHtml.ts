/**
 * Sanitize untrusted HTML email for display in a sandboxed iframe.
 * Does not attempt to be a full HTML sanitizer — relies on iframe sandbox + CSP.
 * Still strips the most dangerous tags/attrs so srcdoc is safer even if sandbox is mis-set.
 */

const DANGEROUS_TAGS =
  /<\/?(?:script|iframe|object|embed|applet|form|base|link|meta|frame|frameset|svg|math|video|audio|source|track|portal)\b[^>]*>/gi;

const EVENT_ATTRS = /\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi;

const DANGEROUS_URL_ATTRS =
  /\s+(?:href|src|xlink:href|action|formaction)\s*=\s*(?:(?:"\s*(?:javascript|vbscript|data\s*:\s*text\/html)[^"]*")|(?:'\s*(?:javascript|vbscript|data\s*:\s*text\/html)[^']*')|(?:(?:javascript|vbscript|data\s*:\s*text\/html)[^>\s]*))/gi;

export function sanitizeEmailHtml(html: string): string {
  let out = html;
  // Drop HTML comments (can hide IE conditional scripts).
  out = out.replace(/<!--[\s\S]*?-->/g, "");
  out = out.replace(DANGEROUS_TAGS, "");
  out = out.replace(EVENT_ATTRS, "");
  out = out.replace(DANGEROUS_URL_ATTRS, "");
  return out;
}

/** Build a full HTML document for srcdoc with a strict CSP. */
export function buildEmailSrcDoc(html: string): string {
  const body = sanitizeEmailHtml(html);
  const csp = [
    "default-src 'none'",
    "img-src https: http: data: cid: blob:",
    "style-src 'unsafe-inline' data:",
    "font-src https: http: data:",
    "media-src https: http: data:",
    "base-uri 'none'",
    "form-action 'none'",
    "frame-src 'none'",
    "object-src 'none'",
    "script-src 'none'",
    "worker-src 'none'",
    "connect-src 'none'",
    "child-src 'none'",
  ].join("; ");

  return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8"/>
<meta name="referrer" content="no-referrer"/>
<meta http-equiv="Content-Security-Policy" content="${csp}"/>
<base target="_blank" rel="noopener noreferrer"/>
<style>
  html, body { margin: 0; padding: 0; background: transparent; color: #1c1916; }
  body { font: 14px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; word-wrap: break-word; overflow-wrap: anywhere; }
  img { max-width: 100%; height: auto; }
  a { color: #0f6e56; }
  pre, code { white-space: pre-wrap; }
</style>
</head>
<body>${body}</body>
</html>`;
}
