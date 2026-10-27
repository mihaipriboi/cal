import { afterEach, describe, expect, it, vi } from "vitest";
import {
  deviceTimeZone,
  formatDayShort,
  formatDayTitle,
  iso,
  minutesToTime,
  monthMatrix,
  rangeFor,
  setTimeZone,
  startOfWeek,
  timeToMinutes,
  todayIso,
  weekDays,
  zonedNow,
} from "./date";

describe("date helpers", () => {
  it("labels a missing date as No date", () => {
    expect(formatDayShort(undefined)).toBe("No date");
    expect(formatDayTitle(undefined)).toBe("No date");
    expect(formatDayShort("2026-05-11")).not.toBe("No date");
  });

  it("builds Monday-first weeks", () => {
    const days = weekDays(new Date(2026, 4, 12));
    expect(days).toHaveLength(7);
    expect(iso(days[0])).toBe("2026-05-11");
  });

  it("builds Sunday-first weeks", () => {
    const days = weekDays(new Date(2026, 4, 12), "sunday");
    expect(iso(days[0])).toBe("2026-05-10");
  });

  it("builds a 4-6 week month matrix", () => {
    expect(monthMatrix(new Date(2026, 4, 12)).length % 7).toBe(0);
    expect(monthMatrix(new Date(2026, 4, 12)).length).toBeGreaterThanOrEqual(28);
    expect(monthMatrix(new Date(2026, 4, 12)).length).toBeLessThanOrEqual(42);
  });

  it("covers the whole month exactly when aligned", () => {
    // Feb 2026 runs Sun 1st - Sat 28th: a perfect 4x7 on a Sunday-start calendar.
    const days = monthMatrix(new Date(2026, 1, 12), "sunday");
    expect(days).toHaveLength(28);
    expect(iso(days[0])).toBe("2026-02-01");
    expect(iso(days[27])).toBe("2026-02-28");
  });

  it("creates day range", () => {
    expect(rangeFor("day", new Date(2026, 4, 12))).toEqual({
      from: "2026-05-12",
      to: "2026-05-12",
    });
  });

  it("converts times to minutes and back", () => {
    expect(timeToMinutes("14:30")).toBe(870);
    expect(timeToMinutes(undefined)).toBeUndefined();
    expect(minutesToTime(870)).toBe("14:30");
    expect(minutesToTime(1440)).toBe("23:59");
  });

  it("startOfWeek respects preference", () => {
    const mon = startOfWeek(new Date(2026, 4, 13), "monday");
    const sun = startOfWeek(new Date(2026, 4, 13), "sunday");
    expect(iso(mon)).toBe("2026-05-11");
    expect(iso(sun)).toBe("2026-05-10");
  });
});

describe("Settings time zone", () => {
  afterEach(() => {
    setTimeZone(undefined);
    vi.useRealTimers();
  });

  it("dates today in the Settings zone, not the device's", () => {
    vi.useFakeTimers();
    // 00:30 on Oct 26 in Bucharest (UTC+2 after the fall-back), still Oct 25 in UTC.
    vi.setSystemTime(new Date("2026-10-25T22:30:00Z"));
    setTimeZone("Europe/Bucharest");
    expect(todayIso()).toBe("2026-10-26");
    expect(zonedNow().getHours()).toBe(0);
    expect(zonedNow().getMinutes()).toBe(30);
    setTimeZone("UTC");
    expect(todayIso()).toBe("2026-10-25");
    expect(zonedNow().getHours()).toBe(22);
    // 23:30 on Oct 26 in Bucharest is already Oct 27 at UTC+14.
    vi.setSystemTime(new Date("2026-10-26T21:30:00Z"));
    setTimeZone("Europe/Bucharest");
    expect(todayIso()).toBe("2026-10-26");
    expect(zonedNow().getHours()).toBe(23);
    setTimeZone("Pacific/Kiritimati");
    expect(todayIso()).toBe("2026-10-27");
  });

  it("falls back to the device's zone for an unknown name", () => {
    setTimeZone("Mars/Olympus");
    expect(todayIso()).toBe(iso(new Date()));
    expect(deviceTimeZone()).toMatch(/\w/);
  });
});
