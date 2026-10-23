import type { YtResult } from "@cal/api-client";
import { Check, ExternalLink, Link2, Play, Plus, Search, SquarePlay } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { linkDomain } from "../lib/entries";
import { formatDayShort, todayIso } from "../lib/date";
import { usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

type Filter = "all" | "videos" | "articles";

export function LinksPage() {
  const entries = usePlanner((state) => state.entries);
  const loadEntries = usePlanner((state) => state.loadEntries);
  const updateEntry = usePlanner((state) => state.updateEntry);
  const createEntry = usePlanner((state) => state.createEntry);
  const settings = usePlanner((state) => state.settings);
  const api = usePlanner((state) => state.api);
  const toast = usePlanner((state) => state.toast);
  const openCreate = useUi((state) => state.openCreate);
  const openEdit = useUi((state) => state.openEdit);
  const [grid, setGrid] = useState(true);
  const [filter, setFilter] = useState<Filter>("all");
  const [showSearch, setShowSearch] = useState(false);
  const [ytQuery, setYtQuery] = useState("");
  const [ytResults, setYtResults] = useState<YtResult[] | null>(null);
  const [ytBusy, setYtBusy] = useState(false);
  const [ytSaved, setYtSaved] = useState<Set<string>>(new Set());

  useEffect(() => {
    void loadEntries({});
  }, [loadEntries]);

  const links = useMemo(
    () =>
      entries
        .filter((e) => e.type === "link")
        .sort((a, b) => (b.date ?? "").localeCompare(a.date ?? "") || b.createdAt.localeCompare(a.createdAt)),
    [entries],
  );

  const visible = useMemo(() => {
    if (filter === "videos") return links.filter((l) => l.linkVideoId);
    if (filter === "articles") return links.filter((l) => !l.linkVideoId);
    return links;
  }, [links, filter]);

  const videoCount = links.filter((l) => l.linkVideoId).length;

  async function runSearch() {
    const q = ytQuery.trim();
    if (!q || ytBusy) return;
    setYtBusy(true);
    try {
      setYtResults(await api.youtubeSearch(q));
    } catch (error) {
      setYtResults(null);
      toast(error instanceof Error ? error.message : "Search failed");
    } finally {
      setYtBusy(false);
    }
  }

  async function saveVideo(v: YtResult) {
    try {
      await createEntry({
        title: v.title,
        type: "link",
        linkUrl: v.url,
        date: todayIso(),
        tags: ["video"],
      });
      setYtSaved((prev) => new Set(prev).add(v.videoId));
      toast("Saved to links");
    } catch {
      // createEntry already reports via toast
    }
  }

  return (
    <>
      <PageHeader title="Links" sub={`${links.length} saved · ${videoCount} video${videoCount === 1 ? "" : "s"}`}>
        {(["all", "videos", "articles"] as Filter[]).map((f) => (
          <button
            key={f}
            type="button"
            className={`btn btn-secondary btn-xs ${filter === f ? "on" : ""}`}
            onClick={() => setFilter(f)}
          >
            {f === "videos" ? "Videos" : f === "articles" ? "Articles" : "All"}
          </button>
        ))}
        <button type="button" className="btn btn-secondary btn-xs" onClick={() => setGrid((v) => !v)}>
          {grid ? "List" : "Cards"}
        </button>
        <button
          type="button"
          className={`btn btn-secondary btn-xs ${showSearch ? "on" : ""}`}
          aria-pressed={showSearch}
          onClick={() => setShowSearch((v) => !v)}
        >
          <SquarePlay size={13} /> YouTube
        </button>
        <button type="button" className="btn btn-primary" onClick={() => openCreate(todayIso())}>
          <Plus size={14} strokeWidth={2.5} /> Save link
        </button>
      </PageHeader>
      {showSearch && (
        <div className="yt-search">
          <form
            className="yt-search-bar"
            onSubmit={(e) => {
              e.preventDefault();
              void runSearch();
            }}
          >
            <Search size={14} />
            <input
              className="input"
              value={ytQuery}
              onChange={(e) => setYtQuery(e.target.value)}
              placeholder="Search YouTube…"
              aria-label="Search YouTube"
              autoFocus
            />
            <button type="submit" className="btn btn-secondary btn-xs" disabled={ytBusy || !ytQuery.trim()}>
              {ytBusy ? "Searching…" : "Search"}
            </button>
          </form>
          {!settings.invidiousUrl ? (
            <p className="panel-note" style={{ marginTop: 8 }}>
              YouTube search runs through your own Invidious instance — set its URL in{" "}
              <a href="/settings">Settings</a> first.
            </p>
          ) : ytResults === null ? null : ytResults.length === 0 ? (
            <p className="panel-note" style={{ marginTop: 8 }}>No videos found.</p>
          ) : (
            <div className="link-grid" style={{ marginTop: 10 }}>
              {ytResults.map((v) => (
                <article key={v.videoId} className="link-card">
                  <a href={v.url} target="_blank" rel="noopener noreferrer" className="link-thumb">
                    <img
                      src={v.thumbnail}
                      alt=""
                      loading="lazy"
                      style={{ position: "absolute", inset: 0 }}
                      onError={(e) => { e.currentTarget.style.display = "none"; }}
                    />
                    <span className="link-play">
                      <span className="link-play-btn"><Play size={18} fill="currentColor" /></span>
                    </span>
                  </a>
                  <div className="link-card-body">
                    <a href={v.url} target="_blank" rel="noopener noreferrer" className="link-card-title">
                      {v.title}
                    </a>
                    <div className="link-card-meta">
                      <span className="link-domain">{v.author}</span>
                      {v.seconds > 0 && (
                        <span className="meta-chip">
                          {Math.floor(v.seconds / 60)}:{String(v.seconds % 60).padStart(2, "0")}
                        </span>
                      )}
                    </div>
                    <button
                      type="button"
                      className="btn btn-secondary btn-xs"
                      disabled={ytSaved.has(v.videoId)}
                      onClick={() => void saveVideo(v)}
                    >
                      {ytSaved.has(v.videoId) ? <Check size={12} /> : <Plus size={12} />}
                      {ytSaved.has(v.videoId) ? "Saved" : "Save"}
                    </button>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      )}
      <div className="page-scroll">
        {visible.length === 0 ? (
          <div className="empty-hint">
            <strong>{filter === "all" ? "No links saved" : `No ${filter}`}</strong>
            <span>Save a link — previews and YouTube thumbnails fetch themselves.</span>
            <button type="button" className="btn btn-primary" onClick={() => openCreate(todayIso())}>
              <Plus size={14} /> Save one
            </button>
          </div>
        ) : grid ? (
          <div className="link-grid">
            {visible.map((link) => {
              const domain = linkDomain(link.linkUrl);
              const isVideo = !!link.linkVideoId;
              return (
                <article key={link.id} className={`link-card ${link.watched ? "watched" : ""}`}>
                  <a href={link.linkUrl} target="_blank" rel="noopener noreferrer" className="link-thumb">
                    {/* Fallback sits under the image — a broken og:image hides, the mark shows through. */}
                    <span className="link-thumb-plain">
                      {/* Letter/icon is the base layer; the favicon sits on top
                          and hides on error, so a dead favicon URL degrades to
                          the letter instead of a broken-image glyph. */}
                      {domain ? (
                        <span className="link-thumb-letter">{domain.replace(/^www\./, "")[0].toUpperCase()}</span>
                      ) : (
                        <Link2 size={24} />
                      )}
                      {link.linkFavicon && (
                        <img
                          src={link.linkFavicon}
                          alt=""
                          className="link-favicon link-favicon-lg"
                          onError={(e) => { e.currentTarget.style.display = "none"; }}
                        />
                      )}
                    </span>
                    {link.linkImage && (
                      <img
                        src={link.linkImage}
                        alt=""
                        loading="lazy"
                        style={{ position: "absolute", inset: 0 }}
                        onLoad={(e) => { if (e.currentTarget.naturalWidth === 0) e.currentTarget.style.display = "none"; }}
                        onError={(e) => { e.currentTarget.style.display = "none"; }}
                      />
                    )}
                    {isVideo && (
                      <span className="link-play">
                        <span className="link-play-btn"><Play size={18} fill="currentColor" /></span>
                      </span>
                    )}
                    {isVideo && (
                      <button
                        type="button"
                        className={`link-watched ${link.watched ? "on" : ""}`}
                        aria-label={link.watched ? "Mark unwatched" : "Mark watched"}
                        onClick={(e) => {
                          e.preventDefault();
                          void updateEntry(link.id, { watched: !link.watched });
                        }}
                      >
                        <Check size={13} />
                        {link.watched && <span>Watched</span>}
                      </button>
                    )}
                  </a>
                  <div className="link-card-body">
                    <button type="button" className="link-card-title" onClick={() => openEdit(link)}>
                      {link.title}
                    </button>
                    {link.linkDesc && (
                      <p className="link-desc">
                        {isVideo && <SquarePlay size={12} className="link-yt" />}
                        {link.linkDesc}
                      </p>
                    )}
                    <div className="link-card-meta">
                      <span className="link-domain">{domain}</span>
                      {!link.linkMetaAt && <span className="meta-chip enriching">Fetching…</span>}
                      <span className="meta-chip date">{formatDayShort(link.date)}</span>
                      {link.tags?.slice(0, 2).map((t) => (
                        <span key={t} className="meta-chip">#{t}</span>
                      ))}
                    </div>
                  </div>
                </article>
              );
            })}
          </div>
        ) : (
          <ul className="link-list">
            {visible.map((link) => {
              const domain = linkDomain(link.linkUrl);
              return (
                <li key={link.id} className={`link-row color-${link.color} ${link.watched ? "watched" : ""}`}>
                  {/* Mark under the favicon, same overlay trick as the cards —
                      a 404 favicon falls back to the domain letter. */}
                  <span className="link-mark">
                    {domain ? domain[0].toUpperCase() : <Link2 size={14} />}
                    {link.linkFavicon && (
                      <img
                        src={link.linkFavicon}
                        alt=""
                        className="link-favicon"
                        onError={(e) => { e.currentTarget.style.display = "none"; }}
                      />
                    )}
                  </span>
                  <button type="button" className="row-title" onClick={() => openEdit(link)}>
                    {link.title}
                    {domain && <span className="link-domain">{domain}</span>}
                  </button>
                  {link.linkVideoId && <SquarePlay size={13} style={{ color: "var(--text-3)" }} />}
                  {!link.linkMetaAt && <span className="meta-chip enriching">Fetching…</span>}
                  <span className="meta-chip date">{formatDayShort(link.date)}</span>
                  {link.linkUrl && (
                    <a
                      className="icon-btn"
                      href={link.linkUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label="Open link"
                    >
                      <ExternalLink size={15} />
                    </a>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </>
  );
}
