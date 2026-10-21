// Minimal markdown → React renderer for note preview. No dependency, no
// dangerouslySetInnerHTML — every node is a real element, so it's safe by
// construction. Supports: headings, bold, italic, code (inline + fenced),
// links, lists (ul/ol/todos), blockquotes, hr, paragraphs.

import type { ReactNode } from "react";

let key = 0;
const k = () => `m${key++}`;

function inline(text: string, onWiki?: (title: string) => void): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(!\[[^\]]*\]\([^)]+\))|(\[\[[^\]]+\]\])|(\[[^\]]+\]\([^)]+\))|(?<!\w)(#[a-z0-9_-]+)/gi;
  let last = 0;
  for (const m of text.matchAll(re)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const tok = m[0];
    if (tok.startsWith("`")) {
      out.push(<code key={k()}>{tok.slice(1, -1)}</code>);
    } else if (tok.startsWith("#")) {
      out.push(
        <span key={k()} className="md-tag">
          {tok}
        </span>,
      );
    } else if (tok.startsWith("[[")) {
      const title = tok.slice(2, -2).trim();
      out.push(
        <a
          key={k()}
          href="#"
          className="md-wiki"
          onClick={(e) => {
            e.preventDefault();
            onWiki?.(title);
          }}
        >
          {title}
        </a>,
      );
    } else if (tok.startsWith("**")) {
      out.push(<strong key={k()}>{tok.slice(2, -2)}</strong>);
    } else if (tok.startsWith("![")) {
      const end = tok.indexOf("](");
      const alt = tok.slice(2, end);
      const src = tok.slice(end + 2, -1);
      // Only render same-origin uploads or http(s) — nothing else loads.
      if (isRelative(src) || /^https?:\/\//i.test(src)) {
        out.push(<img key={k()} src={resolveAsset(src)} alt={alt} className="md-img" />);
      } else {
        out.push(alt);
      }
    } else if (tok.startsWith("[")) {
      const end = tok.indexOf("](");
      const label = tok.slice(1, end);
      const href = tok.slice(end + 2, -1);
      out.push(
        safeHref(href) ? (
          <a key={k()} href={href} target="_blank" rel="noreferrer">
            {label}
          </a>
        ) : (
          label
        ),
      );
    } else {
      out.push(<em key={k()}>{tok.slice(1, -1)}</em>);
    }
    last = m.index + tok.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

// A single leading "/" is same-origin; "//host" and "/\\host" are another host.
function isRelative(url: string): boolean {
  return /^\/(?![/\\])/.test(url);
}

// Links may be http(s), mailto or same-origin; anything else (javascript:,
// data:, intent:, …) renders as plain text.
export function safeHref(href: string): boolean {
  return isRelative(href) || /^(https?:\/\/|mailto:)/i.test(href.trim());
}

// /api/… paths resolve to the stored remote server + session on native builds.
function resolveAsset(src: string): string {
  if (!src.startsWith("/api/")) return src;
  try {
    const server = localStorage.getItem("cal:server");
    const session = localStorage.getItem("cal:session");
    const base = server ? server + src : src;
    return session ? `${base}${src.includes("?") ? "&" : "?"}session=${encodeURIComponent(session)}` : base;
  } catch {
    return src;
  }
}

export function renderMarkdown(src: string, onWiki?: (title: string) => void): ReactNode[] {
  key = 0;
  const run = (t: string) => inline(t, onWiki);
  const blocks: ReactNode[] = [];
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === "") {
      i++;
      continue;
    }
    // fenced code
    if (line.trimStart().startsWith("```")) {
      const buf: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith("```")) buf.push(lines[i++]);
      i++;
      blocks.push(
        <pre key={k()}>
          <code>{buf.join("\n")}</code>
        </pre>,
      );
      continue;
    }
    // heading
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    if (h) {
      const level = h[1].length;
      const Tag = `h${Math.min(level, 4)}` as "h1" | "h2" | "h3" | "h4";
      blocks.push(<Tag key={k()}>{run(h[2])}</Tag>);
      i++;
      continue;
    }
    // hr
    if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) {
      blocks.push(<hr key={k()} />);
      i++;
      continue;
    }
    // blockquote
    if (line.trimStart().startsWith(">")) {
      const buf: string[] = [];
      while (i < lines.length && lines[i].trimStart().startsWith(">")) {
        buf.push(lines[i].trimStart().slice(1).trimStart());
        i++;
      }
      blocks.push(<blockquote key={k()}>{run(buf.join(" "))}</blockquote>);
      continue;
    }
    // lists (incl. - [ ] todos)
    if (/^\s*([-*+]|\d+\.)\s+/.test(line)) {
      const items: { text: string; todo?: boolean; done?: boolean }[] = [];
      const ordered = /^\s*\d+\./.test(line);
      while (i < lines.length && /^\s*([-*+]|\d+\.)\s+/.test(lines[i])) {
        const raw = lines[i].replace(/^\s*([-*+]|\d+\.)\s+/, "");
        const todo = /^\[( |x)\]\s+/i.exec(raw);
        items.push(todo ? { text: raw.slice(todo[0].length), todo: true, done: todo[1] === "x" } : { text: raw });
        i++;
      }
      const items_ = items.map((it, j) =>
        it.todo ? (
          <li key={j} className={`todo ${it.done ? "done" : ""}`}>
            <span className="box" aria-hidden />
            <span>{run(it.text)}</span>
          </li>
        ) : (
          <li key={j}>{run(it.text)}</li>
        ),
      );
      blocks.push(ordered ? <ol key={k()}>{items_}</ol> : <ul key={k()}>{items_}</ul>);
      continue;
    }
    // paragraph — merge consecutive plain lines
    const buf = [line];
    i++;
    while (i < lines.length && lines[i].trim() !== "" && !/^\s*(#{1,4}|>|[-*+]|\d+\.|```|-{3,})/.test(lines[i])) {
      buf.push(lines[i]);
      i++;
    }
    blocks.push(<p key={k()}>{run(buf.join(" "))}</p>);
  }
  return blocks;
}
