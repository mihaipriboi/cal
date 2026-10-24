import { Cake, Heart } from "lucide-react";
import type { PersonOccurrence } from "../lib/people";
import { useUi } from "../stores/ui";
import { cardAnchor, cardTrigger } from "./EventCard";

// PersonChip — a birthday/anniversary marker on the calendar. Clicking opens
// its details card, which links to the person's profile.
export function PersonChip({ occasion }: { occasion: PersonOccurrence }) {
  const openEventCard = useUi((state) => state.openEventCard);
  const turned = occasion.turning ? ` · turns ${occasion.turning}` : "";
  return (
    <button
      type="button"
      className={`person-chip color-${occasion.color}`}
      title={`${occasion.name} · ${occasion.label}${turned}`}
      {...cardTrigger((el) => openEventCard({ kind: "person", item: occasion, rect: cardAnchor(el) }))}
    >
      {occasion.label === "birthday" ? <Cake size={11} /> : <Heart size={11} />}
      <span className="title">{occasion.name}</span>
    </button>
  );
}
