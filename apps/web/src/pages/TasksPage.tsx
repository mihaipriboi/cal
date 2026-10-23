import type { SavedFilter } from "@cal/api-client";
import { Bookmark, CalendarPlus, Check, CheckSquare, Plus, Save, Square, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "../components/PageHeader";
import { allTags, groupTasks } from "../lib/entries";
import { parseQuickAdd } from "../lib/quickadd";
import { addDays, formatDayShort, fromIso, iso, startOfWeek, todayIso } from "../lib/date";
import { reportErr, usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";
import type { Entry } from "@cal/api-client";

function TaskRow({
  entry,
  muted,
  selecting,
  selected,
  onSelect,
}: {
  entry: Entry;
  muted?: boolean;
  selecting?: boolean;
  selected?: boolean;
  onSelect?: (id: string) => void;
}) {
  const openEdit = useUi((state) => state.openEdit);
  const updateEntry = usePlanner((state) => state.updateEntry);
  const weekStart = usePlanner((state) => state.settings.weekStart);
  const [scheduling, setScheduling] = useState(false);
  const canSchedule = !entry.date && !entry.completed && !selecting;

  function schedule(date: string) {
    setScheduling(false);
    if (date) void updateEntry(entry.id, { date });
  }

  const today = fromIso(todayIso());
  return (
    <li className={`task-row ${entry.completed ? "done" : ""} ${muted ? "muted" : ""} ${scheduling ? "scheduling" : ""}`}>
      {selecting ? (
        <button
          type="button"
          className="tickbox"
          aria-label={selected ? "Deselect" : "Select"}
          onClick={() => onSelect?.(entry.id)}
        >
          {selected ? <CheckSquare size={12} strokeWidth={3} /> : <Square size={12} />}
        </button>
      ) : (
        <button
          type="button"
          className="tickbox"
          aria-label={entry.completed ? "Reopen" : "Complete"}
          onClick={() => void updateEntry(entry.id, { completed: !entry.completed })}
        >
          {entry.completed && <Check size={12} strokeWidth={3} />}
        </button>
      )}
      <button type="button" className="row-title" onClick={() => (selecting ? onSelect?.(entry.id) : openEdit(entry))}>
        {entry.title}
      </button>
      {entry.tags.map((tag) => (
        <span key={tag} className="meta-chip">
          {tag}
        </span>
      ))}
      {entry.recur !== "none" && <span className="meta-chip">{entry.recur}</span>}
      {entry.date && <span className="meta-chip date">{formatDayShort(entry.date)}</span>}
      {canSchedule && (
        <button
          type="button"
          className={`icon-btn ${scheduling ? "on" : ""}`}
          aria-label="Schedule"
          aria-expanded={scheduling}
          title="Schedule"
          onClick={() => setScheduling((v) => !v)}
        >
          <CalendarPlus size={14} />
        </button>
      )}
      {scheduling && (
        <div className="schedule-chips">
          <button type="button" className="tag-chip" onClick={() => schedule(iso(today))}>
            Today
          </button>
          <button type="button" className="tag-chip" onClick={() => schedule(iso(addDays(today, 1)))}>
            Tomorrow
          </button>
          <button type="button" className="tag-chip" onClick={() => schedule(iso(startOfWeek(addDays(today, 7), weekStart)))}>
            Next week
          </button>
          <input className="input" type="date" aria-label="Pick a date" onChange={(e) => schedule(e.target.value)} />
        </div>
      )}
    </li>
  );
}

export function TasksPage() {
  const entries = usePlanner((state) => state.entries);
  const loadEntries = usePlanner((state) => state.loadEntries);
  const createEntry = usePlanner((state) => state.createEntry);
  const updateEntry = usePlanner((state) => state.updateEntry);
  const deleteEntry = usePlanner((state) => state.deleteEntry);
  const toast = usePlanner((state) => state.toast);
  const [tag, setTag] = useState<string>();
  const [quick, setQuick] = useState("");
  const [showDone, setShowDone] = useState(false);
  const [selecting, setSelecting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [savedFilters, setSavedFilters] = useState<SavedFilter[]>([]);
  const api = usePlanner((state) => state.api);
  const quickAddToday = usePlanner((state) => state.settings.quickAddToday ?? false);
  const undatedFirst = usePlanner((state) => state.settings.undatedFirst ?? false);
  const today = todayIso();

  // The task list needs every entry, not just the visible calendar range.
  useEffect(() => {
    void loadEntries({});
    void api.filters().then(setSavedFilters).catch(reportErr("Could not load filters"));
  }, [loadEntries, api]);

  function applyFilter(f: SavedFilter) {
    const flt = (f.filter ?? {}) as { tag?: string; done?: boolean };
    setTag(flt.tag);
    setShowDone(flt.done ?? false);
  }

  async function saveCurrent() {
    const name = window.prompt("Save this filter as", tag ?? "All tasks");
    if (!name?.trim()) return;
    const filter: Record<string, unknown> = { type: "task" };
    if (tag) filter.tag = tag;
    if (showDone) filter.done = true;
    try {
      const f = await api.createFilter(name.trim(), filter);
      setSavedFilters((l) => [...l, f]);
      toast("Filter saved");
    } catch {
      toast("Could not save filter");
    }
  }

  const tasks = useMemo(
    () => (tag ? entries.filter((e) => e.tags.includes(tag)) : entries),
    [entries, tag],
  );
  const groups = useMemo(() => groupTasks(tasks, today), [tasks, today]);
  const tags = useMemo(() => allTags(entries.filter((e) => e.type === "task")), [entries]);

  async function addQuick() {
    const parsed = parseQuickAdd(quick);
    if (!parsed) return;
    setQuick("");
    await createEntry({
      title: parsed.title,
      type: "task",
      // No day in the text: undated, unless the setting says today.
      date: parsed.date ?? (quickAddToday ? today : undefined),
      startTime: parsed.startTime,
      tags: [...parsed.tags, ...(tag ? [tag] : [])],
    });
  }

  function toggleSelect(id: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function bulkComplete() {
    for (const id of selected) await updateEntry(id, { completed: true });
    toast(`${selected.size} completed`);
    setSelected(new Set());
    setSelecting(false);
  }

  async function bulkDelete() {
    for (const id of selected) await deleteEntry(id);
    toast(`${selected.size} moved to trash`);
    setSelected(new Set());
    setSelecting(false);
  }

  const rowProps = { selecting, onSelect: toggleSelect };
  const noDateGroup = groups.noDate.length > 0 && (
    <section className="panel">
      <h3>
        No date <span className="group-count">{groups.noDate.length}</span>
      </h3>
      <ul className="task-list">
        {groups.noDate.map((e) => (
          <TaskRow key={e.id} entry={e} selected={selected.has(e.id)} {...rowProps} />
        ))}
      </ul>
    </section>
  );

  return (
    <>
      <PageHeader title="Tasks" sub={`${groups.overdue.length + groups.today.length + groups.upcoming.length + groups.noDate.length} open`}>
        <button
          type="button"
          className={`btn btn-secondary btn-xs ${selecting ? "on" : ""}`}
          onClick={() => {
            setSelecting((v) => !v);
            setSelected(new Set());
          }}
        >
          {selecting ? `Done selecting (${selected.size})` : "Select"}
        </button>
      </PageHeader>
      <div className="page-scroll">
        {selecting && selected.size > 0 && (
          <div className="bulk-bar">
            <span>{selected.size} selected</span>
            <button type="button" className="btn btn-secondary btn-xs" onClick={() => void bulkComplete()}>
              <Check size={12} /> Complete all
            </button>
            <button type="button" className="btn btn-secondary btn-xs" onClick={() => void bulkDelete()}>
              <Trash2 size={12} /> Delete all
            </button>
          </div>
        )}
        <div className="quick-add">
          <Plus size={15} />
          <input
            value={quick}
            onChange={(e) => setQuick(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void addQuick()}
            placeholder='Try "dentist fri 5pm #health" — Enter to add'
            aria-label="Quick add task"
          />
        </div>

        {savedFilters.length > 0 && (
          <div className="tag-row">
            {savedFilters.map((f) => (
              <button key={f.id} type="button" className="tag-chip" onClick={() => applyFilter(f)}
                onContextMenu={(e) => {
                  e.preventDefault();
                  void api.deleteFilter(f.id).then(() => setSavedFilters((l) => l.filter((x) => x.id !== f.id))).catch((e) => toast(e instanceof Error ? e.message : "Could not delete filter"));
                }}
                title="Click to apply · right-click to delete"
              >
                <Bookmark size={10} /> {f.name}
              </button>
            ))}
          </div>
        )}
        {(tags.length > 0 || tag) && (
          <div className="tag-row">
            <button type="button" className={`tag-chip ${!tag ? "active" : ""}`} onClick={() => setTag(undefined)}>
              All
            </button>
            {tags.map((t) => (
              <button
                key={t}
                type="button"
                className={`tag-chip ${tag === t ? "active" : ""}`}
                onClick={() => setTag(tag === t ? undefined : t)}
              >
                {t}
              </button>
            ))}
            {tag && (
              <button type="button" className="tag-chip" onClick={() => void saveCurrent()} title="Save as a filter">
                <Save size={10} /> save
              </button>
            )}
          </div>
        )}

        <div className="task-groups">
          {undatedFirst && noDateGroup}
          {groups.overdue.length > 0 && (
            <section className="panel">
              <h3 className="overdue">Overdue</h3>
              <ul className="task-list">
                {groups.overdue.map((e) => (
                  <TaskRow key={e.id} entry={e} selected={selected.has(e.id)} {...rowProps} />
                ))}
              </ul>
            </section>
          )}
          <section className="panel">
            <h3>Today</h3>
            {groups.today.length === 0 && <p className="panel-empty">Nothing due today.</p>}
            <ul className="task-list">
              {groups.today.map((e) => (
                <TaskRow key={e.id} entry={e} selected={selected.has(e.id)} {...rowProps} />
              ))}
            </ul>
          </section>
          {groups.upcoming.length > 0 && (
            <section className="panel">
              <h3>Upcoming</h3>
              <ul className="task-list">
                {groups.upcoming.map((e) => (
                  <TaskRow key={e.id} entry={e} selected={selected.has(e.id)} {...rowProps} />
                ))}
              </ul>
            </section>
          )}
          {!undatedFirst && noDateGroup}
          {groups.done.length > 0 && (
            <section className="panel">
              <button type="button" className="group-toggle" onClick={() => setShowDone(!showDone)}>
                Done · {groups.done.length}
              </button>
              {showDone && (
                <ul className="task-list">
                  {groups.done.map((e) => (
                    <TaskRow key={e.id} entry={e} muted selected={selected.has(e.id)} {...rowProps} />
                  ))}
                </ul>
              )}
            </section>
          )}
        </div>
      </div>
    </>
  );
}
