import type { DashboardStats, Habit, TimeSummary, WeekReview } from "@cal/api-client";
import {
  Cake,
  Check,
  Crosshair,
  File,
  Flame,
  Heart,
  Link2,
  NotebookPen,
  Pencil,
  Play,
  Plus,
  Sparkles,
  StickyNote,
  Timer,
  UserRound,
  type LucideIcon,
} from "lucide-react";
import { Cloud, CloudDrizzle, CloudFog, CloudLightning, CloudRain, CloudSun, Snowflake, Sun } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { cardAnchor, cardTrigger } from "../components/EventCard";
import { PageHeader } from "../components/PageHeader";
import { addDays, formatTime, iso, timeToMinutes, todayIso } from "../lib/date";
import { groupTasks } from "../lib/entries";
import { moduleOn } from "../lib/modules";
import { daysLabel, upcomingDates } from "../lib/people";
import { fetchWeather, weatherIcon, type Weather } from "../lib/weather";
import { reportErr, usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

const addDaysIso = (date: string, n: number) => iso(addDays(new Date(`${date}T12:00:00`), n));

// Round-number ages that get milestone treatment on the upcoming strip.
const MILESTONES = new Set([1, 16, 18, 21, 25, 30, 40, 50, 60, 70, 80, 90, 100]);

const WEATHER_ICONS: Record<string, LucideIcon> = {
  sun: Sun,
  "cloud-sun": CloudSun,
  "cloud-fog": CloudFog,
  "cloud-drizzle": CloudDrizzle,
  "cloud-rain": CloudRain,
  "cloud-lightning": CloudLightning,
  snowflake: Snowflake,
  cloud: Cloud,
};

function greeting(now: Date): string {
  const h = now.getHours();
  if (h < 5) return "Still up";
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

const JOURNAL_TEMPLATE = `## Journal — {date}

**Morning.** What would make today good?


**Evening.** What happened; what did I learn?


**Grateful for.**`;

export function TodayPage() {
  const entries = usePlanner((state) => state.entries);
  const feedEvents = usePlanner((state) => state.feedEvents);
  const people = usePlanner((state) => state.people);
  const loadPeople = usePlanner((state) => state.loadPeople);
  const loadEntries = usePlanner((state) => state.loadEntries);
  const loadFeeds = usePlanner((state) => state.loadFeeds);
  const loadFeedEvents = usePlanner((state) => state.loadFeedEvents);
  const updateEntry = usePlanner((state) => state.updateEntry);
  const createEntry = usePlanner((state) => state.createEntry);
  const api = usePlanner((state) => state.api);
  const toast = usePlanner((state) => state.toast);
  const settings = usePlanner((state) => state.settings);
  const openCreate = useUi((state) => state.openCreate);
  const openEdit = useUi((state) => state.openEdit);
  const openEventCard = useUi((state) => state.openEventCard);
  const selectDate = useUi((state) => state.selectDate);
  const navigate = useNavigate();
  const [now, setNow] = useState(() => new Date());
  const [weather, setWeather] = useState<Weather | null>(null);
  const [habits, setHabits] = useState<Habit[]>([]);
  const [review, setReview] = useState<WeekReview | null>(null);
  const [showReview, setShowReview] = useState(false);
  const [focus, setFocus] = useState(false);
  const [activity, setActivity] = useState<Record<string, number>>({});
  const [time, setTime] = useState<TimeSummary | null>(null);
  const [dash, setDash] = useState<DashboardStats | null>(null);
  const [storage, setStorage] = useState<{ usedBytes: number; quotaBytes: number } | null>(null);
  const activeWorkspace = usePlanner((state) => state.settings.activeWorkspace);
  const today = todayIso();

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => selectDate(today), [today, selectDate]);

  const peopleOn = moduleOn(settings, "people");

  // Deep-link landing: ensure entries exist even if the calendar never mounted.
  useEffect(() => {
    void loadEntries({ from: addDaysIso(today, -14), to: addDaysIso(today, 14), undated: true });
    void loadFeeds();
    if (peopleOn) void loadPeople();
    void api.habits().then(setHabits).catch(reportErr("Could not load habits"));
    void api.activity().then(setActivity).catch(reportErr("Could not load activity"));
    void api.timeSummary().then(setTime).catch(reportErr("Could not load time totals"));
    void api.dashboard(activeWorkspace ?? "").then(setDash).catch(reportErr("Could not load dashboard"));
    void api.storage().then(setStorage).catch(reportErr("Could not load storage"));
  }, [today, loadEntries, loadFeeds, loadPeople, api, activeWorkspace, peopleOn]);

  useEffect(() => {
    if (settings.city) {
      void fetchWeather(settings.city).then(setWeather).catch(() => setWeather(null));
    } else {
      setWeather(null);
    }
  }, [settings.city]);

  const feeds = usePlanner((state) => state.feeds);
  useEffect(() => {
    if (feeds.length) void loadFeedEvents({ from: today, to: today });
  }, [feeds.length, today, loadFeedEvents]);

  const dayEntries = useMemo(
    () => entries.filter((e) => e.date === today).sort((a, b) => (timeToMinutes(a.startTime) ?? 9999) - (timeToMinutes(b.startTime) ?? 9999)),
    [entries, today],
  );
  const timed = dayEntries.filter((e) => timeToMinutes(e.startTime) !== undefined);
  const untimedTasks = dayEntries.filter((e) => e.type === "task" && timeToMinutes(e.startTime) === undefined);
  const notes = dayEntries.filter((e) => e.type === "note");
  const links = dayEntries.filter((e) => e.type === "link");
  const tasksTotal = dayEntries.filter((e) => e.type === "task").length;
  const tasksDone = dayEntries.filter((e) => e.type === "task" && e.completed).length;
  const nowMinutes = now.getHours() * 60 + now.getMinutes();
  const todaysFeeds = feedEvents.filter((e) => e.date === today);
  const nextUp = timed.find((e) => (timeToMinutes(e.startTime) ?? 0) >= nowMinutes || (timeToMinutes(e.endTime) ?? 0) > nowMinutes);

  const current = timed.find((e) => {
    const s = timeToMinutes(e.startTime) ?? 0;
    const en = timeToMinutes(e.endTime) ?? s + 60;
    return s <= nowMinutes && nowMinutes < en;
  });
  const openTasks = untimedTasks.filter((e) => !e.completed);
  const undatedTasks = useMemo(() => groupTasks(entries, today).noDate, [entries, today]);
  const [showUndated, setShowUndated] = useState(false);

  // Birthdays/anniversaries coming up — workspace-scoped like the entries.
  const upcomingPeople = useMemo(() => {
    if (!peopleOn) return [];
    const ws = activeWorkspace ?? "";
    const scoped =
      ws === "none" ? people.filter((p) => !p.workspaceId) : ws ? people.filter((p) => p.workspaceId === ws) : people;
    return upcomingDates(scoped, 14);
  }, [people, peopleOn, activeWorkspace]);

  // Recently added — newest profiles first, capped at a strip's worth.
  const recentPeople = useMemo(() => {
    if (!peopleOn) return [];
    const ws = activeWorkspace ?? "";
    const scoped =
      ws === "none" ? people.filter((p) => !p.workspaceId) : ws ? people.filter((p) => p.workspaceId === ws) : people;
    return [...scoped].sort((a, b) => b.createdAt.localeCompare(a.createdAt)).slice(0, 6);
  }, [people, peopleOn, activeWorkspace]);

  const WeatherIcon = weather ? WEATHER_ICONS[weatherIcon(weather.code).icon] : null;

  async function saveReviewAsNote() {
    if (!review) return;
    const lines = [
      `## Week of ${review.from}`,
      "",
      `- **${review.tasksDone}** tasks done, **${review.tasksSlipped}** slipped`,
      `- **${review.notesWritten}** notes written`,
      `- Completion streak: **${review.streak}** day${review.streak === 1 ? "" : "s"}`,
      review.busiestDay ? `- Busiest: ${review.busiestDay} (${review.busiestCount} done)` : "",
      "",
      "## Three focuses for next week",
      "",
      "1. ",
      "2. ",
      "3. ",
    ].join("\n");
    await createEntry({ title: `Weekly review ${review.from}`, type: "note", date: today, content: lines });
  }

  // Track — start a timer linked to a scheduled entry, from the focus pane.
  function track(entryId: string) {
    void api
      .startTimer({ entryId })
      .then(() => {
        toast("Timer started");
        void api.timeSummary().then(setTime).catch(reportErr("Could not load time totals"));
      })
      .catch((e) => toast(e instanceof Error ? e.message : "Could not start timer"));
  }

  async function openJournal() {
    const entry = await createEntry({
      title: "Journal",
      type: "note",
      date: today,
      content: JOURNAL_TEMPLATE.replace("{date}", today),
    });
    if (entry) openEdit(entry);
  }

  return (
    <>
      <PageHeader
        title={now.toLocaleDateString(undefined, { weekday: "long" })}
        sub={now.toLocaleDateString(undefined, { month: "long", day: "numeric", year: "numeric" })}
      >
        {weather && WeatherIcon && (
          <span className="weather-pill" title={`${settings.city}: ${weatherIcon(weather.code).label}, high ${weather.high}° low ${weather.low}°`}>
            <WeatherIcon size={13} /> {weather.temp}° · {weather.low}–{weather.high}°
          </span>
        )}
        <button type="button" className="btn btn-secondary hide-sm" onClick={() => void openJournal()}>
          <NotebookPen size={14} /> Journal
        </button>
        <button
          type="button"
          className={`btn hide-sm ${focus ? "btn-primary" : "btn-secondary"}`}
          aria-pressed={focus}
          onClick={() => setFocus((v) => !v)}
        >
          <Crosshair size={14} /> Focus
        </button>
        <button type="button" className="btn btn-primary" onClick={() => openCreate(today)}>
          <Plus size={14} strokeWidth={2.5} /> Add
        </button>
      </PageHeader>

      <div className="page-scroll">
        {!focus && (
          <div className="today-hero">
            <p className="greeting">{greeting(now)}.</p>
            <p className="today-line">
              {tasksTotal === 0 && timed.length === 0
                ? "Nothing planned. A quiet page is also a plan."
                : `${tasksDone} of ${tasksTotal} task${tasksTotal === 1 ? "" : "s"} done`}
              {nextUp ? ` — next up at ${formatTime(nextUp.startTime!)}` : ""}
            </p>
            {tasksTotal > 0 && (
              <div className="meter" aria-hidden>
                <i style={{ width: `${(tasksDone / tasksTotal) * 100}%` }} />
              </div>
            )}
            <div className="habit-row">
              <button
                type="button"
                className="habit-chip"
                title="Start a focus timer"
                onClick={() => void api.startTimer({}).then(() => api.timeSummary().then(setTime)).catch((e) => toast(e instanceof Error ? e.message : "Could not start timer"))}
              >
                <Timer size={12} /> Focus timer
              </button>
            </div>
            {time && time.todayMinutes > 0 && (
              <div className="habit-row">
                <span className="habit-chip on" title="Focused today">
                  <Timer size={12} /> {time.todayMinutes}m today · {time.weekMinutes}m this week
                </span>
                {time.perEntry.slice(0, 3).map((p) => (
                  <span key={p.entryId} className="habit-chip" title={p.title}>
                    {p.title} <b>{p.minutes}m</b>
                  </span>
                ))}
              </div>
            )}
            {habits.length > 0 && (
              <div className="habit-row">
                {habits.map((h) => (
                  <span key={h.id} className={`habit-chip ${h.streak > 0 ? "on" : ""}`} title={`${h.title} — ${h.recur}, streak ${h.streak}`}>
                    <Flame size={12} /> {h.title} <b>{h.streak}</b>
                  </span>
                ))}
              </div>
            )}
            {upcomingPeople.length > 0 && (
              <div className="habit-row">
                {upcomingPeople.map((o) => {
                  const milestone = o.label === "birthday" && o.turning !== undefined && MILESTONES.has(o.turning);
                  return (
                    <button
                      key={o.id}
                      type="button"
                      className={`habit-chip ${o.daysUntil <= 1 || milestone ? "on" : ""}`}
                      title={`${o.name} · ${o.label}${o.turning ? ` — turns ${o.turning}` : ""}`}
                      onClick={() => navigate(`/people/${o.personId}`)}
                    >
                      {milestone ? <Sparkles size={12} /> : o.label === "birthday" ? <Cake size={12} /> : <Heart size={12} />}
                      {o.name} <b>{milestone ? `turns ${o.turning} ${daysLabel(o.daysUntil)}` : daysLabel(o.daysUntil)}</b>
                    </button>
                  );
                })}
              </div>
            )}
            {recentPeople.length > 0 && (
              <div className="habit-row today-recent">
                <span className="today-recent-label">New</span>
                {recentPeople.map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    className="habit-chip person-chip-mini"
                    title={`${p.name} — added ${new Date(p.createdAt).toLocaleDateString()}`}
                    onClick={() => navigate(`/people/${p.id}`)}
                  >
                    <UserRound size={12} /> {p.name}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}

        {focus ? (
          <div className="focus-pane">
            {current ? (
              <div className="focus-now">
                <span className="focus-label">Now</span>
                <h2 className="focus-title">{current.title}</h2>
                <p className="focus-when">
                  {formatTime(current.startTime!)} – {formatTime(current.endTime ?? "")}
                  {" · "}
                  {Math.max(0, (timeToMinutes(current.endTime) ?? 0) - nowMinutes)} min left
                </p>
                <p className="focus-when">
                  <button type="button" className="btn btn-secondary btn-xs" onClick={() => track(current.id)}>
                    <Play size={11} /> Track this
                  </button>
                </p>
              </div>
            ) : nextUp ? (
              <div className="focus-now">
                <span className="focus-label">Next</span>
                <h2 className="focus-title">{nextUp.title}</h2>
                <p className="focus-when">starts {formatTime(nextUp.startTime!)}</p>
                <p className="focus-when">
                  <button type="button" className="btn btn-secondary btn-xs" onClick={() => track(nextUp.id)}>
                    <Play size={11} /> Track this
                  </button>
                </p>
              </div>
            ) : (
              <div className="focus-now">
                <span className="focus-label">Clear</span>
                <h2 className="focus-title">Nothing scheduled</h2>
                <p className="focus-when">{openTasks.length} task{openTasks.length === 1 ? "" : "s"} left today</p>
              </div>
            )}
            {openTasks.length > 0 && (
              <ul className="check-list focus-tasks">
                {openTasks.map((e) => (
                  <li key={e.id}>
                    <button
                      type="button"
                      className="tickbox"
                      aria-label="Complete"
                      onClick={() => void updateEntry(e.id, { completed: true })}
                    />
                    <button type="button" className="row-title" onClick={() => openEdit(e)}>
                      {e.title}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : (
          <div className="today-cols">
            <section className="panel">
              <h3>Schedule</h3>
              {timed.length === 0 && todaysFeeds.length === 0 && <p className="panel-empty">No timed entries today.</p>}
              <ol className="agenda">
                {todaysFeeds.map((e) => {
                  const start = timeToMinutes(e.startTime);
                  const end = timeToMinutes(e.endTime) ?? (start !== undefined ? start + 60 : undefined);
                  const live = start !== undefined && end !== undefined && start <= nowMinutes && nowMinutes < end;
                  return (
                    <li
                      key={e.id}
                      className={`agenda-item feed-item color-${e.color} ${live ? "live" : ""}`}
                      title={`${e.title} — ${e.feedName}${e.location ? ` · ${e.location}` : ""}`}
                      role="button"
                      tabIndex={0}
                      {...cardTrigger((el) => openEventCard({ kind: "feed", item: e, rect: cardAnchor(el) }))}
                    >
                      <span className="agenda-time">
                        {e.startTime ? formatTime(e.startTime) : "all day"}
                        <i>{e.endTime ? formatTime(e.endTime) : e.feedName}</i>
                      </span>
                      {e.image && <img className="agenda-thumb" src={e.image} alt="" loading="lazy" onError={(ev) => { ev.currentTarget.style.display = "none"; }} />}
                      <span className="agenda-title">{e.title}</span>
                      {live && <span className="live-pill">Now</span>}
                    </li>
                  );
                })}
                {timed.map((e) => {
                  const start = timeToMinutes(e.startTime)!;
                  const end = timeToMinutes(e.endTime) ?? start + 60;
                  const live = start <= nowMinutes && nowMinutes < end;
                  const past = end <= nowMinutes;
                  return (
                    <li
                      key={e.id}
                      className={`agenda-item color-${e.color} ${live ? "live" : ""} ${past ? "past" : ""} ${e.completed ? "done" : ""}`}
                      onClick={() => openEdit(e)}
                      role="button"
                      tabIndex={0}
                      onKeyDown={(ev) => ev.key === "Enter" && openEdit(e)}
                    >
                      <span className="agenda-time">
                        {formatTime(e.startTime!)}
                        <i>{formatTime(e.endTime ?? "")}</i>
                      </span>
                      <span className="agenda-title">{e.title}</span>
                      {live && <span className="live-pill">Now</span>}
                    </li>
                  );
                })}
              </ol>
            </section>

            <section className="panel">
              <h3>Tasks</h3>
              {untimedTasks.length === 0 && <p className="panel-empty">No untimed tasks today.</p>}
              <ul className="check-list">
                {untimedTasks.map((e) => (
                  <li key={e.id} className={e.completed ? "done" : ""}>
                    <button
                      type="button"
                      className="tickbox"
                      aria-label={e.completed ? "Reopen" : "Complete"}
                      onClick={() => void updateEntry(e.id, { completed: !e.completed })}
                    >
                      {e.completed && <Check size={12} strokeWidth={3} />}
                    </button>
                    <button type="button" className="row-title" onClick={() => openEdit(e)}>
                      {e.title}
                    </button>
                    {e.tags.includes("habit") && <Flame size={12} className="row-ic" style={{ color: "var(--accent)" }} />}
                    {e.recur !== "none" && <span className="meta-chip">{e.recur}</span>}
                  </li>
                ))}
              </ul>
              {undatedTasks.length > 0 && (
                <>
                  <button
                    type="button"
                    className="group-toggle"
                    style={{ marginTop: 12 }}
                    aria-expanded={showUndated}
                    onClick={() => setShowUndated((v) => !v)}
                  >
                    No date · {undatedTasks.length}
                  </button>
                  {showUndated && (
                    <ul className="check-list">
                      {undatedTasks.map((e) => (
                        <li key={e.id}>
                          <button
                            type="button"
                            className="tickbox"
                            aria-label="Complete"
                            onClick={() => void updateEntry(e.id, { completed: true })}
                          />
                          <button type="button" className="row-title" onClick={() => openEdit(e)}>
                            {e.title}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </>
              )}

              {(notes.length > 0 || links.length > 0) && <h3 style={{ marginTop: 18 }}>Notes & links</h3>}
              <ul className="check-list">
                {notes.map((e) => (
                  <li key={e.id}>
                    <StickyNote size={13} className="row-ic" />
                    <button type="button" className="row-title" onClick={() => openEdit(e)}>
                      {e.title}
                    </button>
                  </li>
                ))}
                {links.map((e) => (
                  <li key={e.id}>
                    <Link2 size={13} className="row-ic" />
                    <button
                      type="button"
                      className="row-title"
                      title={e.linkUrl}
                      onClick={() => (e.linkUrl ? window.open(e.linkUrl, "_blank", "noopener") : openEdit(e))}
                    >
                      {e.title}
                    </button>
                    <button type="button" className="icon-btn row-edit" aria-label={`Edit ${e.title}`} onClick={() => openEdit(e)}>
                      <Pencil size={11} />
                    </button>
                  </li>
                ))}
              </ul>
            </section>

            {dash && dash.deadlines.filter((d) => !dayEntries.some((e) => e.id === d.id)).length > 0 && (
              <section className="panel">
                <h3>Upcoming deadlines</h3>
                <ul className="check-list">
                  {dash.deadlines
                    .filter((d) => !dayEntries.some((e) => e.id === d.id))
                    .map((e) => {
                      const daysLeft = Math.round((new Date(`${e.date}T12:00:00`).getTime() - new Date(`${today}T12:00:00`).getTime()) / 86400000);
                      return (
                        <li key={e.id}>
                          <button
                            type="button"
                            className="tickbox"
                            aria-label="Complete"
                            onClick={() => void updateEntry(e.id, { completed: true })}
                          />
                          <button type="button" className="row-title" onClick={() => openEdit(e)}>
                            {e.title}
                          </button>
                          <span className={`meta-chip ${daysLeft <= 0 ? "overdue" : daysLeft <= 2 ? "soon" : ""}`}>
                            {daysLeft < 0 ? "overdue" : daysLeft === 0 ? "today" : daysLeft === 1 ? "tomorrow" : `${daysLeft}d`}
                          </span>
                        </li>
                      );
                    })}
                </ul>
              </section>
            )}

            <section className="panel">
              <h3>
                This week
                <button
                  type="button"
                  className="btn btn-secondary btn-xs"
                  style={{ float: "right" }}
                  onClick={() => {
                    const next = !showReview;
                    setShowReview(next);
                    if (next && !review) void api.weeklyReview().then(setReview).catch(reportErr("Could not load review"));
                  }}
                >
                  {showReview ? "Hide" : "Review"}
                </button>
              </h3>
              {!showReview && <p className="panel-empty">Open the weekly review for a digest of the last 7 days.</p>}
              {showReview && !review && <p className="panel-empty">Loading…</p>}
              {showReview && review && (
                <div className="review-body">
                  <div className="review-grid">
                    <div className="review-stat"><b>{review.tasksDone}</b><span>done</span></div>
                    <div className="review-stat"><b>{review.tasksSlipped}</b><span>slipped</span></div>
                    <div className="review-stat"><b>{review.notesWritten}</b><span>notes</span></div>
                    <div className="review-stat"><b>{review.streak}</b><span>day streak</span></div>
                  </div>
                  {review.busiestDay && (
                    <p className="panel-note">Busiest: {review.busiestDay} ({review.busiestCount} completed).</p>
                  )}
                  <button type="button" className="btn btn-secondary" onClick={() => void saveReviewAsNote()}>
                    <NotebookPen size={14} /> Save as note
                  </button>
                </div>
              )}
            </section>

            {dash && dash.feed.length > 0 && (
              <section className="panel">
                <h3>Recent activity</h3>
                <ul className="feed-list">
                  {dash.feed.slice(0, 10).map((f, i) => (
                    <li key={i} className="feed-row">
                      <span className={`feed-ic kind-${f.kind}`}>
                        {f.kind === "file" ? <File size={11} /> : f.kind === "card" ? <Check size={11} /> : <StickyNote size={11} />}
                      </span>
                      <span className="feed-title" title={f.title}>
                        <b>{f.action}</b> {f.title || "untitled"}
                      </span>
                      <time>{relTime(f.at)}</time>
                    </li>
                  ))}
                </ul>
              </section>
            )}

            {storage && storage.quotaBytes > 0 && (
              <section className="panel">
                <h3>Storage</h3>
                <div className="storage-row" title={`${fmtBytes(storage.usedBytes)} of ${fmtBytes(storage.quotaBytes)} used`}>
                  <div className="meter" aria-hidden>
                    <i style={{ width: `${Math.min(100, (storage.usedBytes / storage.quotaBytes) * 100)}%` }} />
                  </div>
                  <span className="panel-note">{fmtBytes(storage.usedBytes)} / {fmtBytes(storage.quotaBytes)}</span>
                </div>
              </section>
            )}

            <section className="panel">
              <h3>Activity</h3>
              <ActivityGrid counts={activity} today={today} />
            </section>
          </div>
        )}
      </div>
    </>
  );
}

function relTime(isoStr: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(isoStr).getTime()) / 60000));
  if (mins < 1) return "now";
  if (mins < 60) return `${mins}m`;
  const h = Math.floor(mins / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${Math.round(n / 1024)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

// ActivityGrid — contributions-style grid, last ~17 weeks, oldest → newest.
function ActivityGrid({ counts, today }: { counts: Record<string, number>; today: string }) {
  const cells = useMemo(() => {
    const end = new Date(today + "T00:00:00");
    // Align to the week's start so columns are full weeks.
    const start = new Date(end);
    start.setDate(start.getDate() - ((start.getDay() + 6) % 7) - 16 * 7); // Monday-aligned, 17 weeks back
    const out: { date: string; n: number }[] = [];
    for (let d = new Date(start); d <= end; d.setDate(d.getDate() + 1)) {
      const iso = d.toISOString().slice(0, 10);
      out.push({ date: iso, n: counts[iso] ?? 0 });
    }
    return out;
  }, [counts, today]);

  const max = Math.max(1, ...cells.map((c) => c.n));
  const level = (n: number) => (n === 0 ? 0 : n <= max * 0.25 ? 1 : n <= max * 0.5 ? 2 : n <= max * 0.8 ? 3 : 4);

  return (
    <div className="activity-grid" role="img" aria-label="Entry activity for the last 17 weeks">
      {cells.map((c) => (
        <span
          key={c.date}
          className={`activity-cell lv${level(c.n)}`}
          title={`${c.date} — ${c.n} entr${c.n === 1 ? "y" : "ies"}`}
        />
      ))}
    </div>
  );
}
