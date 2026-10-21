import type { Feed, FeedEvent } from "@cal/api-client";

// Colour tokens a feed can take (styled as --c-* in styles.css; the server
// accepts the same list).
export const FEED_COLORS = ["slate", "mint", "sky", "violet", "amber", "orange", "rose", "red"] as const;

export interface FeedPatch {
  name?: string;
  color?: string;
}

// applyFeedPatch renames/recolours one feed and its already-loaded events,
// so the sidebar and calendar update before the server answers.
export function applyFeedPatch(feeds: Feed[], events: FeedEvent[], id: string, patch: FeedPatch) {
  return {
    feeds: feeds.map((f) => (f.id === id ? { ...f, ...patch } : f)),
    feedEvents: events.map((e) =>
      e.feedId !== id
        ? e
        : { ...e, ...(patch.color ? { color: patch.color } : {}), ...(patch.name ? { feedName: patch.name } : {}) },
    ),
  };
}
