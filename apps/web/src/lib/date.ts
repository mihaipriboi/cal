export type CalendarView = "month" | "week" | "day";
export type WeekStartPref = "monday" | "sunday";

export function iso(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function fromIso(value: string): Date {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(year, month - 1, day);
}

export function todayIso(): string {
  return iso(new Date());
}

export function addDays(date: Date, days: number): Date {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
}

export function addMonths(date: Date, months: number): Date {
  const next = new Date(date);
  next.setMonth(next.getMonth() + months);
  return next;
}

/** weekOffset shifts Sunday-first JS weeks to the user's preference. */
function weekOffset(weekStart: WeekStartPref): number {
  return weekStart === "monday" ? 6 : 7;
}

export function startOfWeek(date: Date, weekStart: WeekStartPref = "monday"): Date {
  const next = new Date(date);
  const day = (next.getDay() + weekOffset(weekStart)) % 7;
  next.setDate(next.getDate() - day);
  next.setHours(0, 0, 0, 0);
  return next;
}

export function monthMatrix(anchor: Date, weekStart: WeekStartPref = "monday"): Date[] {
  const first = new Date(anchor.getFullYear(), anchor.getMonth(), 1);
  const start = startOfWeek(first, weekStart);
  const last = new Date(anchor.getFullYear(), anchor.getMonth() + 1, 0);
  const weeks = Math.ceil((Math.round((last.getTime() - start.getTime()) / 86400000) + 1) / 7);
  return Array.from({ length: weeks * 7 }, (_, index) => addDays(start, index));
}

export function weekDays(anchor: Date, weekStart: WeekStartPref = "monday"): Date[] {
  const start = startOfWeek(anchor, weekStart);
  return Array.from({ length: 7 }, (_, index) => addDays(start, index));
}

export function weekdayNames(weekStart: WeekStartPref = "monday", style: "short" | "narrow" = "short"): string[] {
  const base = weekDays(new Date(2026, 0, 5), weekStart); // 2026-01-05 is a Monday
  return base.map((day) => day.toLocaleDateString(undefined, { weekday: style }));
}

export function rangeFor(view: CalendarView, anchor: Date, weekStart: WeekStartPref = "monday"): { from: string; to: string } {
  if (view === "day") return { from: iso(anchor), to: iso(anchor) };
  const days = view === "week" ? weekDays(anchor, weekStart) : monthMatrix(anchor, weekStart);
  return { from: iso(days[0]), to: iso(days[days.length - 1]) };
}

export function formatHeading(view: CalendarView, anchor: Date, weekStart: WeekStartPref = "monday"): string {
  if (view === "day") {
    return anchor.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
  }
  if (view === "week") {
    const days = weekDays(anchor, weekStart);
    const sameMonth = days[0].getMonth() === days[6].getMonth();
    const start = days[0].toLocaleDateString(undefined, { month: "short", day: "numeric" });
    const end = days[6].toLocaleDateString(undefined, sameMonth ? { day: "numeric" } : { month: "short", day: "numeric" });
    return `${start} – ${end}`;
  }
  return anchor.toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

/** Undated tasks have no date: both formatters say "No date" for them. */
export function formatDayTitle(value: string | undefined): string {
  if (!value) return "No date";
  return fromIso(value).toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
}

export function formatDayShort(value: string | undefined): string {
  if (!value) return "No date";
  return fromIso(value).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
}

/** HH:MM -> minutes since midnight; undefined when absent. */
export function timeToMinutes(time?: string): number | undefined {
  if (!time) return undefined;
  const [hours, minutes] = time.split(":").map(Number);
  if (Number.isNaN(hours) || Number.isNaN(minutes)) return undefined;
  return hours * 60 + minutes;
}

export function minutesToTime(minutes: number): string {
  const clamped = Math.max(0, Math.min(1439, Math.round(minutes)));
  const hours = Math.floor(clamped / 60);
  const mins = clamped % 60;
  return `${String(hours).padStart(2, "0")}:${String(mins).padStart(2, "0")}`;
}

export function formatTime(time: string): string {
  const [hours, minutes] = time.split(":").map(Number);
  const date = new Date();
  date.setHours(hours, minutes, 0, 0);
  return date.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

export function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
