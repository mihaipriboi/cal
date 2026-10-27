import type { Entry, FeedEvent, Holiday } from "@cal/api-client";
import { create } from "zustand";
import { addDays, addMonths, fromIso, todayIso, type CalendarView } from "../lib/date";
import type { PersonOccurrence } from "../lib/people";

export interface EditorPrefill {
  title?: string;
  content?: string;
  linkUrl?: string;
  type?: string;
}

export type EditorState =
  | { mode: "closed" }
  | { mode: "create"; date: string; startTime?: string; prefill?: EditorPrefill }
  | { mode: "edit"; entry: Entry };

export interface ContextMenuState {
  x: number;
  y: number;
  entry: Entry;
}

/** Where the clicked item is on screen; the card opens beside it. */
export interface CardAnchor {
  top: number;
  bottom: number;
  left: number;
  right: number;
}

/** The read-only details card for a calendar item that isn't an entry. A
 *  day can have several holidays, which share one label and one card. */
export type EventCardState =
  | { kind: "feed"; item: FeedEvent; rect: CardAnchor }
  | { kind: "holiday"; item: Holiday[]; rect: CardAnchor }
  | { kind: "person"; item: PersonOccurrence; rect: CardAnchor };

interface UiState {
  view: CalendarView;
  anchor: Date;
  selectedDate: string;
  paletteOpen: boolean;
  editor: EditorState;
  contextMenu?: ContextMenuState;
  eventCard?: EventCardState;
  sidebarOpen: boolean;
  hiddenTypes: Set<string>;
  hiddenFeeds: Set<string>;
  setView: (view: CalendarView) => void;
  setAnchor: (date: Date) => void;
  shift: (direction: -1 | 1) => void;
  goToday: () => void;
  /** After a time-zone change: moves a selection that was on the old today to the new one. */
  followToday: (before: string, after: string) => void;
  selectDate: (date: string) => void;
  openPalette: () => void;
  closePalette: () => void;
  openCreate: (date?: string, startTime?: string, prefill?: EditorPrefill) => void;
  openEdit: (entry: Entry) => void;
  closeEditor: () => void;
  setContextMenu: (menu?: ContextMenuState) => void;
  openEventCard: (card: EventCardState) => void;
  closeEventCard: () => void;
  toggleSidebar: () => void;
  closeSidebar: () => void;
  toggleType: (type: string) => void;
  toggleFeed: (id: string) => void;
  applyDefaultView: (view: CalendarView | undefined) => void;
}

const persisted = (() => {
  try {
    return JSON.parse(localStorage.getItem("cal:ui") ?? "{}") as {
      view?: CalendarView;
      hiddenTypes?: string[];
      hiddenFeeds?: string[];
    };
  } catch {
    return {};
  }
})();

function persist(state: Pick<UiState, "view" | "hiddenTypes" | "hiddenFeeds">) {
  try {
    localStorage.setItem(
      "cal:ui",
      JSON.stringify({ view: state.view, hiddenTypes: [...state.hiddenTypes], hiddenFeeds: [...state.hiddenFeeds] }),
    );
  } catch {
    // no storage (tests, SSR)
  }
}

export const useUi = create<UiState>((set, get) => ({
  // Last-used view wins; fresh devices start on month (settings.defaultView
  // replaces it when the server settings land — see applyDefaultView).
  view:
    persisted.view === "month" || persisted.view === "week" || persisted.view === "day"
      ? persisted.view
      : "month",
  anchor: fromIso(todayIso()),
  selectedDate: todayIso(),
  paletteOpen: false,
  editor: { mode: "closed" },
  contextMenu: undefined,
  eventCard: undefined,
  sidebarOpen: false,
  hiddenTypes: new Set(persisted.hiddenTypes ?? []),
  hiddenFeeds: new Set(persisted.hiddenFeeds ?? []),

  setView(view) {
    set({ view });
    persist(get());
  },
  setAnchor(anchor) {
    set({ anchor });
  },
  shift(direction) {
    const { view, anchor } = get();
    if (view === "day") set({ anchor: addDays(anchor, direction) });
    else if (view === "week") set({ anchor: addDays(anchor, direction * 7) });
    else set({ anchor: addMonths(anchor, direction) });
  },
  goToday() {
    set({ anchor: fromIso(todayIso()), selectedDate: todayIso() });
  },
  followToday(before, after) {
    if (get().selectedDate === before) set({ anchor: fromIso(after), selectedDate: after });
  },
  selectDate(selectedDate) {
    set({ selectedDate, anchor: new Date(`${selectedDate}T12:00:00`) });
  },
  openPalette() {
    set({ paletteOpen: true, contextMenu: undefined, eventCard: undefined });
  },
  closePalette() {
    set({ paletteOpen: false });
  },
  openCreate(date, startTime, prefill) {
    set({ editor: { mode: "create", date: date ?? get().selectedDate, startTime, prefill }, contextMenu: undefined, eventCard: undefined });
  },
  openEdit(entry) {
    set({ editor: { mode: "edit", entry }, contextMenu: undefined, eventCard: undefined });
  },
  closeEditor() {
    set({ editor: { mode: "closed" } });
  },
  setContextMenu(contextMenu) {
    set({ contextMenu });
  },
  openEventCard(eventCard) {
    set({ eventCard, contextMenu: undefined });
  },
  closeEventCard() {
    set({ eventCard: undefined });
  },
  toggleSidebar() {
    set({ sidebarOpen: !get().sidebarOpen });
  },
  closeSidebar() {
    set({ sidebarOpen: false });
  },
  toggleType(type) {
    const hiddenTypes = new Set(get().hiddenTypes);
    if (hiddenTypes.has(type)) hiddenTypes.delete(type);
    else hiddenTypes.add(type);
    set({ hiddenTypes });
    persist(get());
  },
  toggleFeed(id) {
    const hiddenFeeds = new Set(get().hiddenFeeds);
    if (hiddenFeeds.has(id)) hiddenFeeds.delete(id);
    else hiddenFeeds.add(id);
    set({ hiddenFeeds });
    persist(get());
  },
  // Server default applies only when this device never chose a view itself.
  applyDefaultView(view) {
    if (!view || persisted.view) return;
    if (get().view !== view) get().setView(view);
  },
}));
