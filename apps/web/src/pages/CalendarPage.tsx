import { useEffect, useMemo } from "react";
import { MonthView } from "../components/MonthView";
import { TimeGridView } from "../components/TimeGridView";
import { TopBar } from "../components/TopBar";
import { rangeFor } from "../lib/date";
import { moduleOn } from "../lib/modules";
import { personDatesInRange } from "../lib/people";
import { usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

export function CalendarPage() {
  const { user, entries, feedEvents, holidays, people, settings, loadEntries, updateEntry, loadHolidays, loadFeeds, loadFeedEvents, loadPeople } =
    usePlanner();
  const { view, anchor, hiddenTypes, hiddenFeeds } = useUi();

  const range = useMemo(() => rangeFor(view, anchor, settings.weekStart), [view, anchor, settings.weekStart]);
  const peopleOn = moduleOn(settings, "people");

  useEffect(() => {
    if (!user) return;
    void loadEntries(range);
  }, [user, range.from, range.to, loadEntries]);

  useEffect(() => {
    if (!user || !settings.showHolidays) return;
    void loadHolidays(settings.country, anchor.getFullYear());
  }, [user, settings.country, settings.showHolidays, anchor, loadHolidays]);

  useEffect(() => {
    if (!user) return;
    void loadFeeds();
  }, [user, loadFeeds]);

  useEffect(() => {
    if (!user || !peopleOn) return;
    void loadPeople();
  }, [user, peopleOn, loadPeople]);

  const feedsKey = usePlanner((s) => s.feeds).length;
  useEffect(() => {
    if (!user) return;
    // The server reads feed times in the Settings zone, so a change refetches.
    void loadFeedEvents({ from: range.from, to: range.to });
  }, [user, range.from, range.to, feedsKey, settings.timezone, loadFeedEvents]);

  const visible = entries.filter((entry) => !hiddenTypes.has(entry.type));
  const visibleFeeds = feedEvents.filter((event) => !hiddenFeeds.has(event.feedId));
  const shownHolidays = settings.showHolidays ? holidays : [];

  // Birthdays/anniversaries expand client-side — they recur yearly, so any
  // range works once people are loaded. Workspace-scoped like the entries.
  const personDates = useMemo(() => {
    if (!peopleOn) return [];
    const ws = settings.activeWorkspace ?? "";
    const scoped = ws === "none"
      ? people.filter((p) => !p.workspaceId)
      : ws
        ? people.filter((p) => p.workspaceId === ws)
        : people;
    return personDatesInRange(scoped, range.from, range.to);
  }, [people, peopleOn, settings.activeWorkspace, range.from, range.to]);

  return (
    <>
      <TopBar weekStart={settings.weekStart} />
      {view === "month" ? (
        <MonthView
          anchor={anchor}
          entries={visible}
          feedEvents={visibleFeeds}
          holidays={shownHolidays}
          personDates={personDates}
          weekStart={settings.weekStart}
          onMoveEntry={(id, date) => void updateEntry(id, { date })}
        />
      ) : (
        <TimeGridView
          view={view}
          anchor={anchor}
          entries={visible}
          feedEvents={visibleFeeds}
          holidays={shownHolidays}
          personDates={personDates}
          weekStart={settings.weekStart}
          onMoveEntry={(id, patch) => void updateEntry(id, patch)}
        />
      )}
    </>
  );
}
