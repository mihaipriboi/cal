import type { FeedEvent } from "@cal/api-client";
import { formatTime } from "../lib/date";
import { useUi } from "../stores/ui";
import { cardAnchor, cardTrigger } from "./EventCard";

// FeedChip is a subscribed-calendar event in a day cell or the all-day row.
// Clicking it opens the event's read-only details card.
export function FeedChip({ event, showTime = true }: { event: FeedEvent; showTime?: boolean }) {
  const openEventCard = useUi((state) => state.openEventCard);
  return (
    <button
      type="button"
      className={`feed-chip color-${event.color}`}
      title={`${event.title} — ${event.feedName}${event.location ? ` · ${event.location}` : ""}`}
      {...cardTrigger((el) => openEventCard({ kind: "feed", item: event, rect: cardAnchor(el) }))}
    >
      {showTime && event.startTime && <span className="time">{formatTime(event.startTime)}</span>}
      <span className="title">{event.title}</span>
    </button>
  );
}
