import type { Entry } from "@cal/api-client";
import { timeToMinutes } from "./date";

export interface TaskGroups {
  overdue: Entry[];
  today: Entry[];
  upcoming: Entry[];
  /** Open tasks without a date. */
  noDate: Entry[];
  done: Entry[];
}

/** Orders by date, oldest first, with undated entries after every dated one. */
export function compareDates(a: string | undefined, b: string | undefined): number {
  if (!a || !b) return Number(!a) - Number(!b);
  return a.localeCompare(b);
}

/** Buckets tasks into overdue / today / upcoming / no date / done, each sorted for scanning. */
export function groupTasks(entries: Entry[], today: string): TaskGroups {
  const groups: TaskGroups = { overdue: [], today: [], upcoming: [], noDate: [], done: [] };
  for (const entry of entries) {
    if (entry.type !== "task") continue;
    if (entry.completed) groups.done.push(entry);
    else if (!entry.date) groups.noDate.push(entry);
    else if (entry.date < today) groups.overdue.push(entry);
    else if (entry.date === today) groups.today.push(entry);
    else groups.upcoming.push(entry);
  }
  const byStart = (a: Entry, b: Entry) =>
    (timeToMinutes(a.startTime) ?? 9999) - (timeToMinutes(b.startTime) ?? 9999) || a.createdAt.localeCompare(b.createdAt);
  groups.overdue.sort((a, b) => compareDates(a.date, b.date) || byStart(a, b));
  groups.today.sort(byStart);
  groups.upcoming.sort((a, b) => compareDates(a.date, b.date) || byStart(a, b));
  // Pinned first, then a set position, then newest first.
  groups.noDate.sort(
    (a, b) =>
      Number(b.pinned) - Number(a.pinned) ||
      (a.position ?? Infinity) - (b.position ?? Infinity) ||
      b.createdAt.localeCompare(a.createdAt),
  );
  groups.done.sort((a, b) => (b.date ?? "").localeCompare(a.date ?? "") || b.createdAt.localeCompare(a.createdAt));
  return groups;
}

/** All tags across entries, ordered by frequency then name. */
export function allTags(entries: Entry[]): string[] {
  const counts = new Map<string, number>();
  for (const entry of entries) for (const tag of entry.tags) counts.set(tag, (counts.get(tag) ?? 0) + 1);
  return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([tag]) => tag);
}

/** Host part of a link entry, e.g. "linear.app". */
export function linkDomain(url: string | undefined): string {
  if (!url) return "";
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return "";
  }
}
