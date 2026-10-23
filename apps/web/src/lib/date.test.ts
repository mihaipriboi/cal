import { describe, expect, it } from "vitest";
import { formatDayShort, formatDayTitle, iso, minutesToTime, monthMatrix, rangeFor, startOfWeek, timeToMinutes, weekDays } from "./date";

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
