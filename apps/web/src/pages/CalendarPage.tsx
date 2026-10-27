import { animate, AnimatePresence, motion, useIsPresent, useMotionValue, useReducedMotion, type MotionValue } from "framer-motion";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { MonthView } from "../components/MonthView";
import { TimeGridView } from "../components/TimeGridView";
import { TopBar } from "../components/TopBar";
import { useSwipeNavigation } from "../hooks/useSwipeNavigation";
import { rangeFor } from "../lib/date";
import type { Step } from "../lib/gestures";
import { moduleOn } from "../lib/modules";
import { personDatesInRange } from "../lib/people";
import { usePlanner } from "../stores/planner";
import { useUi } from "../stores/ui";

/** How the next period comes in: from the right (+1), the left (-1), or at once (0). */
interface Slide {
  dir: Step;
  area: RefObject<HTMLDivElement | null>;
}

const SLIDE_TRANSITION = { duration: 0.18, ease: [0.2, 0, 0, 1] as const };

// A period slides in by the calendar's width, and the old one slides out the
// other way. A view change, or reduced motion, swaps at once.
const SLIDE = {
  enter: ({ dir, area }: Slide) => ({ x: dir * (area.current?.clientWidth ?? window.innerWidth), opacity: 1 }),
  center: { x: 0, opacity: 1 },
  exit: ({ dir, area }: Slide) =>
    dir === 0 ? { opacity: 0, transition: { duration: 0 } } : { x: -dir * (area.current?.clientWidth ?? window.innerWidth) },
};

// SwipePane holds one period's view. The newest pane hands its x to the
// swipe gesture, which moves it while a finger drags the calendar.
function SwipePane({ slide, paneX, children }: { slide: Slide; paneX: { current?: MotionValue<number> }; children: ReactNode }) {
  const x = useMotionValue(0);
  const present = useIsPresent();
  useLayoutEffect(() => {
    if (present) paneX.current = x;
  }, [present, x, paneX]);
  return (
    <motion.div
      className="cal-pane"
      style={{ x }}
      custom={slide}
      variants={SLIDE}
      initial="enter"
      animate="center"
      exit="exit"
      transition={SLIDE_TRANSITION}
    >
      {children}
    </motion.div>
  );
}

// No swipes while something sits over the calendar.
function swipeEnabled(): boolean {
  const ui = useUi.getState();
  return ui.editor.mode === "closed" && !ui.paletteOpen && !document.querySelector(".day-modal, .feed-menu");
}

export function CalendarPage() {
  const { user, entries, feedEvents, holidays, people, settings, loadEntries, updateEntry, loadHolidays, loadFeeds, loadFeedEvents, loadPeople } =
    usePlanner();
  const { view, anchor, hiddenTypes, hiddenFeeds, shift, closeEventCard } = useUi();

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

  // Moving to a later period slides the new one in from the right, an
  // earlier one from the left: the ‹ › buttons, arrow keys, Today and swipes
  // alike.
  const [shown, setShown] = useState<{ view: string; from: string; dir: Step }>({ view, from: range.from, dir: 0 });
  if (shown.view !== view || shown.from !== range.from) {
    setShown({ view, from: range.from, dir: shown.view !== view ? 0 : range.from > shown.from ? 1 : -1 });
  }
  const reduceMotion = useReducedMotion();
  const area = useRef<HTMLDivElement>(null);
  const paneX = useRef<MotionValue<number>>(undefined);
  const slide: Slide = { dir: reduceMotion ? 0 : shown.dir, area };

  useSwipeNavigation(area, {
    onStep: (step) => {
      closeEventCard();
      shift(step);
    },
    enabled: swipeEnabled,
    follow: (dx) => paneX.current?.set(dx),
    settle: () => {
      if (paneX.current) void animate(paneX.current, 0, SLIDE_TRANSITION);
    },
  });

  // Chrome's touchpad back/forward swipe would fire on overscroll here.
  useEffect(() => {
    document.documentElement.classList.add("calendar-route");
    return () => document.documentElement.classList.remove("calendar-route");
  }, []);

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
      <div className="cal-swipe" ref={area}>
        <AnimatePresence initial={false} custom={slide}>
          <SwipePane key={`${view}:${range.from}`} slide={slide} paneX={paneX}>
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
          </SwipePane>
        </AnimatePresence>
      </div>
    </>
  );
}
