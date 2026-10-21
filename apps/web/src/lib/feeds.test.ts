import { describe, expect, it } from "vitest";
import type { Feed, FeedEvent } from "@cal/api-client";
import { applyFeedPatch } from "./feeds";

const feeds: Feed[] = [
  { id: "a", name: "Uni", url: "https://x/a.ics", color: "sky", kind: "calendar" },
  { id: "b", name: "Gym", url: "https://x/b.ics", color: "mint", kind: "calendar" },
];
const events = [
  { id: "1", feedId: "a", feedName: "Uni", title: "Lecture", date: "2026-10-22", color: "sky" },
  { id: "2", feedId: "b", feedName: "Gym", title: "Legs", date: "2026-10-22", color: "mint" },
] as FeedEvent[];

describe("applyFeedPatch", () => {
  it("recolours the feed and only its events", () => {
    const out = applyFeedPatch(feeds, events, "a", { color: "rose" });
    expect(out.feeds.map((f) => f.color)).toEqual(["rose", "mint"]);
    expect(out.feedEvents.map((e) => e.color)).toEqual(["rose", "mint"]);
  });

  it("renames the feed and its events' feed name, leaving colours", () => {
    const out = applyFeedPatch(feeds, events, "b", { name: "Gym ⚡" });
    expect(out.feeds[1]).toMatchObject({ name: "Gym ⚡", color: "mint" });
    expect(out.feedEvents[1]).toMatchObject({ feedName: "Gym ⚡", color: "mint" });
    expect(out.feedEvents[0]).toBe(events[0]);
  });
});
