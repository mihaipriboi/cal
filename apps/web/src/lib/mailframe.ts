// HTML mail is untrusted markup. It renders inside <iframe sandbox srcdoc>
// (no allow-scripts, no allow-same-origin), so it gets an opaque origin and
// can't run script or touch the app. The CSP meta below further limits what
// the message may load; remote images stay off until the reader opts in, which
// keeps tracking pixels from firing on open.

/** Sandbox flags for the mail frame: links may open a new tab, nothing else. */
export const MAIL_FRAME_SANDBOX = "allow-popups allow-popups-to-escape-sandbox";

export function mailFrameDoc(html: string, allowRemote: boolean): string {
  const img = allowRemote ? "data: cid: https:" : "data: cid:";
  const csp = `default-src 'none'; img-src ${img}; style-src 'unsafe-inline'; font-src data:`;
  return (
    `<!doctype html><html><head><meta charset="utf-8">` +
    `<meta http-equiv="Content-Security-Policy" content="${csp}">` +
    `<meta name="referrer" content="no-referrer">` +
    `<base target="_blank">` +
    `<style>html,body{margin:0;background:#fff;color:#111}` +
    `body{padding:12px;font:14px/1.5 system-ui,sans-serif;word-break:break-word}` +
    `img{max-width:100%;height:auto}</style>` +
    `</head><body>${html}</body></html>`
  );
}

/** Whether the message references images or backgrounds that load remotely. */
export function hasRemoteContent(html: string): boolean {
  return /(?:src|background)\s*=\s*["']?\s*(?:https?:)?\/\/|url\(\s*["']?\s*(?:https?:)?\/\//i.test(html);
}
