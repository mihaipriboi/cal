// Tags — every tag across entries, files and people with counts; click
// filters a cross-type list.

import { Hash, UserRound } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { PageHeader } from "../components/PageHeader";
import { formatDayShort } from "../lib/date";
import { reportErr, usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

export function TagsPage() {
  const api = usePlanner((s) => s.api);
  const entries = usePlanner((s) => s.entries);
  const loadEntries = usePlanner((s) => s.loadEntries);
  const people = usePlanner((s) => s.people);
  const loadPeople = usePlanner((s) => s.loadPeople);
  const openEdit = useUi((s) => s.openEdit);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [sel, setSel] = useState("");

  useEffect(() => {
    void loadEntries({});
    void loadPeople();
    void api.tags().then(setCounts).catch(reportErr("Could not load tags"));
  }, [api, loadEntries, loadPeople]);

  const filtered = useMemo(
    () => entries.filter((e) => e.tags.includes(sel)).sort((a, b) => (b.date ?? "").localeCompare(a.date ?? "")),
    [entries, sel],
  );
  const taggedPeople = useMemo(() => people.filter((p) => (p.tags ?? []).includes(sel)), [people, sel]);

  return (
    <>
      <PageHeader title="Tags" sub={sel ? `#${sel}` : `${Object.keys(counts).length} tags in use`} />
      {Object.keys(counts).length > 0 && (
        <div className="tag-filter-row" style={{ paddingTop: 8 }}>
          <button type="button" className={`tag-chip ${sel === "" ? "on" : ""}`} onClick={() => setSel("")}>
            All
          </button>
          {Object.entries(counts).map(([t, n]) => (
            <button key={t} type="button" className={`tag-chip ${sel === t ? "on" : ""}`} onClick={() => setSel(sel === t ? "" : t)}>
              #{t} <span className="tag-n">{n}</span>
            </button>
          ))}
        </div>
      )}
      <div className="page-scroll">
        {sel === "" ? (
          <div className="empty-hint">
            <strong>Pick a tag</strong>
            <span>Tags span tasks, notes, links, board cards, files and people — one tap filters everything.</span>
          </div>
        ) : filtered.length === 0 && taggedPeople.length === 0 ? (
          <div className="empty-hint">
            <strong>Nothing tagged #{sel}</strong>
          </div>
        ) : (
          <>
            {taggedPeople.length > 0 && (
              <>
                <div className="side-label" style={{ margin: "4px 0 6px" }}>People</div>
                <ul className="trash-list" style={{ marginBottom: 14 }}>
                  {taggedPeople.map((p) => (
                    <li key={p.id} className="trash-row">
                      <span className="trash-type">person</span>
                      <Link className="trash-title tag-open" to={`/people/${p.id}`}>
                        <UserRound size={12} /> {p.name}
                      </Link>
                      <span className="trash-date">{p.relation}</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
            {filtered.length > 0 && (
              <>
                {taggedPeople.length > 0 && <div className="side-label" style={{ margin: "4px 0 6px" }}>Entries</div>}
                <ul className="trash-list">
                  {filtered.map((e) => (
                    <li key={e.id} className="trash-row">
                      <span className="trash-type">{e.type}</span>
                      <button type="button" className="trash-title tag-open" onClick={() => openEdit(e)}>
                        <Hash size={12} /> {e.title || "Untitled"}
                      </button>
                      <span className="trash-date">{formatDayShort(e.date)}</span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </>
        )}
      </div>
    </>
  );
}
