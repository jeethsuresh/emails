import { useEffect, useMemo, useRef } from "react";
import { buildEmailSrcDoc } from "../lib/safeHtml";

/**
 * Renders untrusted HTML mail in a locked-down iframe.
 *
 * Isolations:
 * - sandbox WITHOUT allow-scripts / allow-same-origin / allow-forms / allow-top-navigation
 * - allow-popups so links can open externally; no script can run to abuse that
 * - CSP inside srcdoc blocks scripts, XHR, frames, form posts
 * - referrerpolicy=no-referrer
 */
export function SafeHtmlFrame({ html, title }: { html: string; title?: string }) {
  const frameRef = useRef<HTMLIFrameElement>(null);
  const srcDoc = useMemo(() => buildEmailSrcDoc(html), [html]);

  useEffect(() => {
    const frame = frameRef.current;
    if (!frame) return;
    frame.setAttribute(
      "csp",
      "default-src 'none'; img-src https: http: data: cid: blob:; style-src 'unsafe-inline' data:; script-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'",
    );
  }, [srcDoc]);

  return (
    <iframe
      ref={frameRef}
      className="html-frame"
      title={title ?? "Email body"}
      srcDoc={srcDoc}
      // Intentionally omit allow-scripts and allow-same-origin.
      sandbox="allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
    />
  );
}
