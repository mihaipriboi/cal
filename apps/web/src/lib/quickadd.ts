// Minimal natural-language quick-add: "dentist fri 5pm #health".
// Understands weekday names, today/tomorrow, M/D dates, month names,
// HH:MM / h(am|pm) times, and #tags. The remainder becomes the title.

import { iso } from "./date";

export interface QuickAdd {
  title: string;
  /** Absent when the text names no day and no time; the caller picks the default. */
  date?: string;
  startTime?: string;
  tags: string[];
}

const WEEKDAYS = ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"];
const DAY_ALIAS: Record<string, number> = { sun: 0, mon: 1, tue: 2, tues: 2, wed: 3, thu: 4, thur: 4, thurs: 4, fri: 5, sat: 6 };
const MONTHS = ["jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"];

function toIso(d: Date): string {
  return iso(d);
}

export function parseQuickAdd(input: string, today: Date = new Date()): QuickAdd | null {
  const tags: string[] = [];
  const words = input.trim().split(/\s+/).filter(Boolean);
  const kept: string[] = [];
  let date: string | undefined;
  let startTime: string | undefined;
  let nextWeek = false;

  for (let i = 0; i < words.length; i++) {
    const raw = words[i];
    const word = raw.toLowerCase();

    if (raw.startsWith("#") && raw.length > 1) {
      tags.push(raw.slice(1));
      continue;
    }
    const nextWord = i + 1 < words.length ? words[i + 1].toLowerCase() : "";
    const nextIsDay = WEEKDAYS.includes(nextWord) || nextWord in DAY_ALIAS;
    if (word === "next" && nextIsDay) {
      nextWeek = true;
      continue;
    }
    if (word === "today") {
      date = iso(today);
      continue;
    }
    if (word === "tomorrow" || word === "tmr" || word === "tmrw") {
      const d = new Date(today);
      d.setDate(d.getDate() + 1);
      date = toIso(d);
      continue;
    }
    if (WEEKDAYS.includes(word) || word in DAY_ALIAS) {
      const d = new Date(today);
      const target = WEEKDAYS.includes(word) ? WEEKDAYS.indexOf(word) : DAY_ALIAS[word];
      let delta = (target - d.getDay() + 7) % 7;
      if (delta === 0) delta = 7;
      if (nextWeek) delta += 7; // "next X" = the occurrence after the upcoming one
      d.setDate(d.getDate() + delta);
      date = toIso(d);
      nextWeek = false;
      continue;
    }
    // HH:MM or h(am|pm)
    const timeMatch = /^(\d{1,2})(?::(\d{2}))?(am|pm)?$/i.exec(word);
    if (timeMatch && (timeMatch[2] !== undefined || timeMatch[3] !== undefined || (i > 0 && words[i - 1].toLowerCase() === "at"))) {
      let h = Number(timeMatch[1]);
      const min = Number(timeMatch[2] ?? 0);
      const meridiem = timeMatch[3]?.toLowerCase();
      if (meridiem === "pm" && h < 12) h += 12;
      if (meridiem === "am" && h === 12) h = 0;
      if (h < 24 && min < 60) {
        startTime = `${String(h).padStart(2, "0")}:${String(min).padStart(2, "0")}`;
        continue;
      }
    }
    if (word === "at") continue; // consumed by the time branch via look-behind
    // ISO date or M/D
    const isoMatch = /^(\d{4})-(\d{1,2})-(\d{1,2})$/.exec(word);
    const mdMatch = /^(\d{1,2})\/(\d{1,2})(?:\/(\d{2,4}))?$/.exec(word);
    const monthMatch = /^([a-z]{3})\.?\s*$/.exec(word);
    if (isoMatch) {
      date = `${isoMatch[1]}-${isoMatch[2].padStart(2, "0")}-${isoMatch[3].padStart(2, "0")}`;
      continue;
    }
    if (mdMatch) {
      const y = mdMatch[3] ? (mdMatch[3].length === 2 ? `20${mdMatch[3]}` : mdMatch[3]) : String(today.getFullYear());
      date = `${y}-${mdMatch[1].padStart(2, "0")}-${mdMatch[2].padStart(2, "0")}`;
      continue;
    }
    // "sep 15" / "sep 15 2027" — month name followed by a day number.
    if (monthMatch && MONTHS.includes(monthMatch[1]) && i + 1 < words.length && /^\d{1,2}$/.test(words[i + 1])) {
      const month = MONTHS.indexOf(monthMatch[1]) + 1;
      const day = Number(words[i + 1]);
      let year = today.getFullYear();
      const candidate = new Date(year, month - 1, day);
      if (candidate < new Date(today.getFullYear(), today.getMonth(), today.getDate())) year++;
      date = `${year}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
      i++;
      if (i + 1 < words.length && /^\d{4}$/.test(words[i + 1])) i++; // swallow a trailing year
      continue;
    }
    kept.push(raw);
  }

  const title = kept.join(" ").trim();
  if (!title) return null;
  // A time alone means today: a time needs a day.
  return { title, date: date ?? (startTime ? iso(today) : undefined), startTime, tags };
}
