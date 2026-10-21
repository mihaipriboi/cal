import type { Feed } from "@cal/api-client";
import { motion } from "framer-motion";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { FEED_COLORS } from "../lib/feeds";
import { usePlanner } from "../stores/planner";

export interface FeedMenuAnchor {
  feed: Feed;
  x: number;
  y: number;
}

// FeedMenu renames and recolours a subscribed calendar. A colour applies on
// click; the name saves on Enter or when the menu closes (Escape discards
// it). Portalled to <body>: the mobile sidebar is transformed, which would
// otherwise anchor position: fixed to the drawer instead of the viewport.
export function FeedMenu({ feed, x, y, onClose }: FeedMenuAnchor & { onClose: () => void }) {
  const updateFeed = usePlanner((state) => state.updateFeed);
  const current = usePlanner((state) => state.feeds.find((f) => f.id === feed.id)) ?? feed;
  const [name, setName] = useState(feed.name);
  const ref = useRef<HTMLDivElement>(null);
  const latest = useRef({ name, current, onClose });
  latest.current = { name, current, onClose };

  useEffect(() => {
    const save = () => {
      const { name: next, current: f } = latest.current;
      if (next.trim() && next.trim() !== f.name) void updateFeed(f.id, { name: next.trim() });
    };
    const outside = (event: MouseEvent | TouchEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        save();
        latest.current.onClose();
      }
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") latest.current.onClose();
      if (event.key === "Enter" && ref.current?.contains(document.activeElement)) {
        save();
        latest.current.onClose();
      }
    };
    window.addEventListener("mousedown", outside);
    window.addEventListener("touchstart", outside);
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("mousedown", outside);
      window.removeEventListener("touchstart", outside);
      window.removeEventListener("keydown", key);
    };
  }, [updateFeed]);

  return createPortal(
    <motion.div
      ref={ref}
      className="feed-menu"
      role="dialog"
      aria-label={`Edit ${current.name}`}
      style={{ left: Math.max(8, Math.min(x, window.innerWidth - 264)), top: Math.max(8, Math.min(y, window.innerHeight - 180)) }}
      initial={{ opacity: 0, scale: 0.96, y: -4 }}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      transition={{ duration: 0.12, ease: "easeOut" }}
    >
      <label className="field">
        <span>Name</span>
        <input className="input" value={name} maxLength={80} onChange={(e) => setName(e.target.value)} />
      </label>
      <div className="field">
        <span>Colour</span>
        <div className="dots">
          {FEED_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              className={`dot ${current.color === c ? "active" : ""}`}
              style={{ "--dot": `var(--c-${c})` } as React.CSSProperties}
              aria-label={`Colour ${c}`}
              aria-pressed={current.color === c}
              onClick={() => void updateFeed(feed.id, { color: c })}
            />
          ))}
        </div>
      </div>
    </motion.div>,
    document.body,
  );
}
