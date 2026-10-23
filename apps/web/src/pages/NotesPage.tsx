// Notes: two views — a card grid (spatial) and a stream (memos-style
// timeline grouped Today/Yesterday/date). Pinned notes float to the top in
// both. Inline #tags in content render as filter chips.

import { LayoutGrid, ListOrdered, Pin, Plus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { formatDayShort, todayIso } from "../lib/date";
import { usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

const STREAM_KEY = "cal.notes.view";

function dayLabel(date: string | undefined): string {
  const today = todayIso();
  if (date === today) return "Today";
  const y = new Date();
  y.setDate(y.getDate() - 1);
  if (date === y.toISOString().slice(0, 10)) return "Yesterday";
  return formatDayShort(date);
}

// Inline #tag tokens from note content — memo-style quick labels.
function contentTags(content: string): string[] {
  const found = content.match(/(?<!\w)#([a-z0-9_-]+)/gi) ?? [];
  return [...new Set(found.map((t) => t.slice(1).toLowerCase()))].slice(0, 5);
}

// Plain-text excerpt for cards — strips markdown sigils so previews read clean.
function excerpt(content: string, max: number): string {
  const text = content
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "") // images
    .replace(/\[\[([^\]]+)\]\]/g, "$1")   // wiki links
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1") // md links
    .replace(/[#*_`>~-]+/g, " ")           // heading/emphasis/list markers
    .replace(/\s+/g, " ")
    .trim();
  return text.length > max ? text.slice(0, max).trimEnd() + "…" : text;
}

export function NotesPage() {
  const entries = usePlanner((state) => state.entries);
  const loadEntries = usePlanner((state) => state.loadEntries);
  const openCreate = useUi((state) => state.openCreate);
  const openEdit = useUi((state) => state.openEdit);
  const [view, setView] = useState<"grid" | "stream">(() =>
    localStorage.getItem(STREAM_KEY) === "stream" ? "stream" : "grid",
  );
  const [tagFilter, setTagFilter] = useState("");

  useEffect(() => {
    void loadEntries({});
  }, [loadEntries]);

  useEffect(() => {
    localStorage.setItem(STREAM_KEY, view);
  }, [view]);

  const notes = useMemo(() => {
    const list = entries
      .filter((e) => e.type === "note")
      .filter((e) => !tagFilter || e.tags.includes(tagFilter) || contentTags(e.content ?? "").includes(tagFilter));
    // Pinned first, then newest.
    return list.sort(
      (a, b) =>
        Number(b.pinned) - Number(a.pinned) ||
        (b.date ?? "").localeCompare(a.date ?? "") ||
        b.createdAt.localeCompare(a.createdAt),
    );
  }, [entries, tagFilter]);

  const groups = useMemo(() => {
    const g: { label: string; notes: typeof notes }[] = [];
    for (const n of notes) {
      const label = dayLabel(n.date);
      const last = g[g.length - 1];
      if (last?.label === label) last.notes.push(n);
      else g.push({ label, notes: [n] });
    }
    return g;
  }, [notes]);

  const allTags = useMemo(() => {
    const set = new Set<string>();
    for (const n of entries.filter((e) => e.type === "note")) {
      n.tags.forEach((t) => set.add(t));
      contentTags(n.content ?? "").forEach((t) => set.add(t));
    }
    return [...set].sort();
  }, [entries]);

  return (
    <>
      <PageHeader title="Notes" sub={`${notes.length} note${notes.length === 1 ? "" : "s"}`}>
        <div className="seg">
          <button
            type="button"
            className={`seg-btn ${view === "grid" ? "on" : ""}`}
            onClick={() => setView("grid")}
            aria-label="Grid view"
          >
            <LayoutGrid size={14} />
          </button>
          <button
            type="button"
            className={`seg-btn ${view === "stream" ? "on" : ""}`}
            onClick={() => setView("stream")}
            aria-label="Stream view"
          >
            <ListOrdered size={14} />
          </button>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => openCreate(todayIso())}>
          <Plus size={14} strokeWidth={2.5} /> New note
        </button>
      </PageHeader>

      {allTags.length > 0 && (
        <div className="tag-filter-row">
          <button
            type="button"
            className={`tag-chip ${tagFilter === "" ? "on" : ""}`}
            onClick={() => setTagFilter("")}
          >
            All
          </button>
          {allTags.map((t) => (
            <button
              key={t}
              type="button"
              className={`tag-chip ${tagFilter === t ? "on" : ""}`}
              onClick={() => setTagFilter(tagFilter === t ? "" : t)}
            >
              #{t}
            </button>
          ))}
        </div>
      )}

      <div className="page-scroll">
        {notes.length === 0 ? (
          <div className="empty-hint">
            <strong>{tagFilter ? `Nothing tagged #${tagFilter}` : "No notes yet"}</strong>
            <span>Notes live on the calendar too — they pin context to a day.</span>
            <button type="button" className="btn btn-primary" onClick={() => openCreate(todayIso())}>
              <Plus size={14} /> Write one
            </button>
          </div>
        ) : view === "grid" ? (
          <div className="note-grid">
            {notes.map((note) => (
              <button
                key={note.id}
                type="button"
                className={`note-card color-${note.color}`}
                onClick={() => openEdit(note)}
              >
                {note.pinned && <Pin size={11} className="note-pin" aria-label="Pinned" />}
                <strong>{note.title}</strong>
                {note.content && <p>{excerpt(note.content, 160)}</p>}
                <span className="note-date">{formatDayShort(note.date)}</span>
              </button>
            ))}
          </div>
        ) : (
          <div className="note-stream">
            {groups.map((g) => (
              <div key={g.label} className="stream-group">
                <div className="stream-day">{g.label}</div>
                {g.notes.map((note) => (
                  <button key={note.id} type="button" className="stream-row" onClick={() => openEdit(note)}>
                    {note.pinned && <Pin size={11} className="note-pin" aria-label="Pinned" />}
                    <span className="stream-title">{note.title || "Untitled"}</span>
                    {note.content && <span className="stream-excerpt">{excerpt(note.content, 90)}</span>}
                    {[...note.tags, ...contentTags(note.content ?? "")].slice(0, 4).map((t) => (
                      <span key={t} className="stream-tag">
                        #{t}
                      </span>
                    ))}
                  </button>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
