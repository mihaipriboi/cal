import type { ReactNode } from "react";
import { formatTime, fromIso, todayIso } from "./date";
import { safeHref } from "./markdown";

const ENTITIES: Record<string, string> = { amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " " };

function decodeEntities(text: string): string {
  return text.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (whole, code: string) => {
    if (code[0] !== "#") return ENTITIES[code.toLowerCase()] ?? whole;
    const n = code[1] === "x" || code[1] === "X" ? parseInt(code.slice(2), 16) : parseInt(code.slice(1), 10);
    return n > 0 && n <= 0x10ffff ? String.fromCodePoint(n) : whole;
  });
}

const isWebUrl = (s: string) => /^https?:\/\/\S+$/i.test(s);

// An <a> becomes its label, or the URL when the label is empty or just a
// shortened form of it; a label that says something else keeps the URL in
// brackets so it can still be linkified. Unsafe hrefs keep the label only.
// The result is still HTML text: entities are decoded once, later.
function anchorText(attrs: string, inner: string): string {
  const label = inner.replace(/<[^>]*>/g, "").trim();
  const match = /\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))/i.exec(attrs);
  const raw = (match?.[1] ?? match?.[2] ?? match?.[3] ?? "").trim();
  const href = decodeEntities(raw);
  if (!/^(https?:|mailto:)/i.test(href)) return label;
  const plainLabel = decodeEntities(label);
  if (isWebUrl(plainLabel)) return label;
  if (!plainLabel) return raw;
  const bare = plainLabel.replace(/^(https?:\/\/|mailto:)/i, "").replace(/\/$/, "");
  if (bare && href.includes(bare)) return raw;
  return `${label} (${raw})`;
}

// HTML has a closing tag, a <br>/<p>-like tag or an entity; "a<b and c>d"
// in plain text has none of these.
const LOOKS_HTML = /<\/[a-z][a-z0-9]*\s*>|<(br|hr|img|p|div|li|script|style|iframe)\b[^>]*>|&(#\d+|#x[0-9a-f]+|[a-z]+);/i;

// Block boundaries become markers first, so that adjacent boundaries
// (</div><div>) give one break, as they do in a browser: \u0001 is a line
// break, \u0002 a paragraph break.
const LINE = "\u0001";
const PARA = "\u0002";

/** descriptionToText turns an ICS DESCRIPTION — often HTML in Google
 *  exports — into plain text: line breaks for <br> and block tags, every
 *  other tag stripped, entities decoded. Script and style contents go.
 *  Text that doesn't look like HTML is kept as it is. */
export function descriptionToText(input: string | undefined): string {
  if (!input) return "";
  let s = input.replace(/\r\n?/g, "\n").replace(/[\u0001\u0002]/g, "");
  if (LOOKS_HTML.test(s)) {
    s = s.replace(/<!--[\s\S]*?(-->|$)/g, "");
    s = s.replace(/<(script|style|head|title|template)\b[\s\S]*?(<\/\1\s*>|$)/gi, "");
    s = s.replace(/<a\b([^>]*)>([\s\S]*?)<\/a\s*>/gi, (_, attrs: string, inner: string) => anchorText(attrs, inner));
    s = s.replace(/<br\s*\/?>/gi, "\n");
    s = s.replace(/<li\b[^>]*>/gi, `${LINE}• `);
    s = s.replace(/<\/?(p|h[1-6]|blockquote|pre|ul|ol|table)\b[^>]*>/gi, PARA);
    s = s.replace(/<\/?(div|tr|section|article|header|footer|hr|dl|dt|dd)\b[^>]*>/gi, LINE);
    s = s.replace(/<\/?[a-z][^>]*>/gi, "");
    s = s.replace(/[ \t]*[\u0001\u0002](?:\s*[\u0001\u0002])*[ \t]*/g, (run) => (run.includes(PARA) ? "\n\n" : "\n"));
    s = decodeEntities(s);
  }
  s = s
    .split("\n")
    .map((line) => line.replace(/[ \t]+$/, ""))
    .join("\n");
  return s.replace(/\n{3,}/g, "\n\n").trim();
}

const LINK = /(https?:\/\/[^\s<>"']+|mailto:[^\s<>"']+|[\w.+-]+@[\w-]+(?:\.[\w-]+)*\.[a-z]{2,})/gi;

// trimLink drops trailing punctuation that ends a sentence rather than the
// URL, and a closing bracket that has no opening one inside the URL.
function trimLink(raw: string): string {
  let url = raw;
  for (;;) {
    const last = url.slice(-1);
    if (/[.,;:!?'"]/.test(last)) url = url.slice(0, -1);
    else if (last === ")" && (url.match(/\(/g)?.length ?? 0) < (url.match(/\)/g)?.length ?? 0)) url = url.slice(0, -1);
    else if (last === "]" && (url.match(/\[/g)?.length ?? 0) < (url.match(/]/g)?.length ?? 0)) url = url.slice(0, -1);
    else return url;
  }
}

/** linkify renders text as React text nodes, with http(s) URLs, mailto:
 *  links and bare email addresses as links (all through safeHref). */
export function linkify(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const m of text.matchAll(LINK)) {
    const url = trimLink(m[0]);
    const start = m.index;
    const href = /^(https?:|mailto:)/i.test(url) ? url : `mailto:${url}`;
    if (!url || !safeHref(href)) continue;
    if (start > last) out.push(text.slice(last, start));
    out.push(
      <a key={start} href={href} target="_blank" rel="noreferrer">
        {url}
      </a>,
    );
    last = start + url.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export interface WhenInput {
  date: string;
  endDate?: string;
  startTime?: string;
  endTime?: string;
}

/** eventWhen is the card's date line ("Saturday, October 31") and time line
 *  ("09:00 – 10:30", "All day"); the year shows when it isn't this year's.
 *  Multi-day events get one range line: "Oct 30 – Nov 2". */
export function eventWhen(event: WhenInput, today = todayIso()): { day: string; time?: string } {
  const thisYear = today.slice(0, 4);
  const withYear = event.date.slice(0, 4) !== thisYear || (event.endDate ?? event.date).slice(0, 4) !== thisYear;
  const year = withYear ? ({ year: "numeric" } as const) : {};
  const range = (start: string, end?: string) => (end && end !== start ? `${start} – ${end}` : start);
  if (event.endDate && event.endDate > event.date) {
    const short = (d: string) => fromIso(d).toLocaleDateString(undefined, { month: "short", day: "numeric", ...year });
    if (!event.startTime) return { day: `${short(event.date)} – ${short(event.endDate)}`, time: "All day" };
    const end = event.endTime ? `, ${formatTime(event.endTime)}` : "";
    return { day: `${short(event.date)}, ${formatTime(event.startTime)} – ${short(event.endDate)}${end}` };
  }
  const day = fromIso(event.date).toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", ...year });
  if (!event.startTime) return { day, time: "All day" };
  return { day, time: range(formatTime(event.startTime), event.endTime ? formatTime(event.endTime) : undefined) };
}

export const CARD_WIDTH = 320;

/** placeCard puts the desktop card beside the clicked item — on its right
 *  when there's room, otherwise on its left — and keeps it on screen. */
export function placeCard(
  rect: { top: number; left: number; right: number },
  height: number,
  viewport: { width: number; height: number },
): { left: number; top: number } {
  const gap = 8;
  let left = rect.right + gap;
  if (left + CARD_WIDTH > viewport.width - gap) left = rect.left - gap - CARD_WIDTH;
  left = Math.max(gap, Math.min(left, viewport.width - CARD_WIDTH - gap));
  const top = Math.max(gap, Math.min(rect.top, viewport.height - height - gap));
  return { left, top };
}

/** countryName is "Romania" for "RO", or the code when the browser can't
 *  name it. */
export function countryName(code: string): string {
  try {
    return new Intl.DisplayNames(undefined, { type: "region" }).of(code.toUpperCase()) ?? code;
  } catch {
    return code;
  }
}
