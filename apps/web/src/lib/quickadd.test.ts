import { describe, expect, it } from "vitest";
import { parseQuickAdd } from "./quickadd";

const thursday = new Date(2026, 8, 10); // Thu Sep 10 2026

describe("parseQuickAdd", () => {
  it("parses title only, leaving the date to the caller", () => {
    expect(parseQuickAdd("Buy milk #home", thursday)).toEqual({
      title: "Buy milk",
      date: undefined,
      startTime: undefined,
      tags: ["home"],
    });
  });

  it("a time without a day means today", () => {
    expect(parseQuickAdd("standup 9:30", thursday)).toMatchObject({ date: "2026-09-10", startTime: "09:30" });
  });

  it("parses weekday + time + tag", () => {
    const out = parseQuickAdd("dentist fri 5pm #health", thursday);
    expect(out).toEqual({ title: "dentist", date: "2026-09-11", startTime: "17:00", tags: ["health"] });
  });

  it("next weekday skips a week", () => {
    const out = parseQuickAdd("review next monday", thursday);
    expect(out?.date).toBe("2026-09-21");
    expect(parseQuickAdd("review monday", thursday)?.date).toBe("2026-09-14");
  });

  it("same weekday rolls forward", () => {
    const out = parseQuickAdd("gym thursday 7am", thursday);
    expect(out?.date).toBe("2026-09-17");
    expect(out?.startTime).toBe("07:00");
  });

  it("tomorrow", () => {
    expect(parseQuickAdd("call mom tomorrow", thursday)?.date).toBe("2026-09-11");
  });

  it("at-time and 24h time", () => {
    expect(parseQuickAdd("lunch at 12:30", thursday)?.startTime).toBe("12:30");
    expect(parseQuickAdd("deploy 17:45", thursday)?.startTime).toBe("17:45");
  });

  it("month-name dates pick the next occurrence", () => {
    expect(parseQuickAdd("anniversary oct 3", thursday)?.date).toBe("2026-10-03");
    expect(parseQuickAdd("past jan 5", thursday)?.date).toBe("2027-01-05");
  });

  it("m/d dates", () => {
    expect(parseQuickAdd("release 9/25", thursday)?.date).toBe("2026-09-25");
  });

  it("empty input returns null", () => {
    expect(parseQuickAdd("   ", thursday)).toBeNull();
    expect(parseQuickAdd("fri", thursday)).toBeNull();
  });
});
