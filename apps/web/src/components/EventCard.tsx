import type { Feed, FeedEvent, Holiday } from "@cal/api-client";
import { AnimatePresence, motion, useDragControls } from "framer-motion";
import { AlignLeft, CalendarDays, Cake, Copy, CopyPlus, ExternalLink, EyeOff, Heart, Info, MapPin, Palette, X } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import { swallowNextClick } from "../lib/clicks";
import { countryName, descriptionToText, eventWhen, linkify, placeCard } from "../lib/eventText";
import type { PersonOccurrence } from "../lib/people";
import { usePlanner } from "../stores/planner";
import { useUi, type CardAnchor, type EventCardState } from "../stores/ui";
import { FeedMenu, type FeedMenuAnchor } from "./FeedMenu";

const NARROW = "(max-width: 639px)";

export function cardAnchor(el: Element): CardAnchor {
  const r = el.getBoundingClientRect();
  return { top: r.top, bottom: r.bottom, left: r.left, right: r.right };
}

/** Props for an item that opens the card. Its click and Enter stay off the
 *  day cell or grid column underneath, which would select the day or start
 *  a new entry; a non-button item also opens on Enter and Space. */
export function cardTrigger(open: (el: Element) => void) {
  return {
    onClick: (event: React.MouseEvent) => {
      event.stopPropagation();
      open(event.currentTarget);
    },
    onDoubleClick: (event: React.MouseEvent) => event.stopPropagation(),
    onKeyDown: (event: React.KeyboardEvent) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.stopPropagation();
      if (!(event.currentTarget instanceof HTMLButtonElement)) {
        event.preventDefault();
        open(event.currentTarget);
      }
    },
  };
}

function cardKey(card: EventCardState): string {
  if (card.kind === "holiday") return `holiday:${card.item[0]?.date}`;
  return `${card.kind}:${card.item.id}`;
}

// EventCard shows a read-only card for a calendar item that isn't one of the
// user's entries: a subscribed-calendar event, a holiday or a person date.
// A popover beside the item on desktop, a bottom sheet on narrow screens.
// Portalled to <body> like FeedMenu, since the sidebar and modals use
// transforms.
export function EventCard() {
  const card = useUi((state) => state.eventCard);
  const close = useUi((state) => state.closeEventCard);
  return createPortal(
    <AnimatePresence>{card && <CardView key={cardKey(card)} card={card} onClose={close} />}</AnimatePresence>,
    document.body,
  );
}

function CardView({ card, onClose }: { card: EventCardState; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const [narrow] = useState(() => window.matchMedia(NARROW).matches);
  const [pos, setPos] = useState<{ left: number; top: number }>();
  const [menu, setMenu] = useState<FeedMenuAnchor>();
  const opener = useRef<HTMLElement | null>(null);
  const drag = useDragControls();
  const latest = useRef({ onClose, menu });
  latest.current = { onClose, menu };

  // × and Escape hand focus back to the item that opened the card.
  const dismiss = () => {
    if (opener.current?.isConnected) opener.current.focus({ preventScroll: true });
    onClose();
  };
  const latestDismiss = useRef(dismiss);
  latestDismiss.current = dismiss;

  useEffect(() => {
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    ref.current?.focus({ preventScroll: true });
  }, []);

  useLayoutEffect(() => {
    const el = ref.current;
    if (narrow || !el) return;
    const place = () => setPos(placeCard(card.rect, el.offsetHeight, { width: window.innerWidth, height: window.innerHeight }));
    place();
    const observer = typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(place);
    observer?.observe(el);
    return () => observer?.disconnect();
  }, [narrow, card.rect]);

  useEffect(() => {
    const outside = (event: MouseEvent | TouchEvent) => {
      const target = event.target as Element;
      if (latest.current.menu || ref.current?.contains(target) || target.closest?.(".feed-menu")) return;
      latest.current.onClose();
      // A click that only dismisses the card shouldn't also reach the
      // calendar grid underneath, where it would select a day or open a new
      // entry.
      if (target.closest?.(".tg-col, .day-cell") && !target.closest("button, a, [role=button]:not(.day-cell)")) swallowNextClick();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !latest.current.menu) latestDismiss.current();
    };
    // The popover is placed against the item, so it closes when the window
    // is resized. (Not on scroll: clicking a partly hidden event scrolls it
    // into view.) The sheet doesn't depend on the item's position.
    const resized = () => latest.current.onClose();
    window.addEventListener("keydown", key);
    if (!narrow) {
      window.addEventListener("mousedown", outside);
      window.addEventListener("touchstart", outside);
      window.addEventListener("resize", resized);
    }
    return () => {
      window.removeEventListener("keydown", key);
      window.removeEventListener("mousedown", outside);
      window.removeEventListener("touchstart", outside);
      window.removeEventListener("resize", resized);
    };
  }, [narrow]);

  const label = card.kind === "holiday" ? card.item.map((h) => h.name).join(", ") : card.kind === "feed" ? card.item.title : card.item.name;
  const body = (
    <CardBody
      card={card}
      onDismiss={dismiss}
      onClose={onClose}
      onDragStart={narrow ? (event) => drag.start(event) : undefined}
      onOpenMenu={(feed) => {
        const r = ref.current?.getBoundingClientRect();
        setMenu({ feed, x: r ? r.left + 16 : 16, y: r ? r.top + 48 : 16 });
      }}
    />
  );
  const feedMenu = menu && <FeedMenu {...menu} onClose={() => setMenu(undefined)} />;

  if (narrow) {
    return (
      <motion.div
        className="card-scrim"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.15 }}
        onClick={(event) => event.target === event.currentTarget && !menu && onClose()}
      >
        <motion.div
          ref={ref}
          className="event-card sheet"
          role="dialog"
          aria-modal="true"
          aria-label={label}
          tabIndex={-1}
          initial={{ y: "100%" }}
          animate={{ y: 0 }}
          exit={{ y: "100%" }}
          transition={{ type: "tween", duration: 0.2, ease: "easeOut" }}
          drag="y"
          dragControls={drag}
          dragListener={false}
          dragConstraints={{ top: 0, bottom: 0 }}
          dragElastic={{ top: 0, bottom: 0.7 }}
          onDragEnd={(_, info) => {
            if (info.offset.y > 80 || info.velocity.y > 500) onClose();
          }}
        >
          {body}
        </motion.div>
        {feedMenu}
      </motion.div>
    );
  }
  return (
    <>
      <motion.div
        ref={ref}
        className="event-card floating"
        role="dialog"
        aria-label={label}
        tabIndex={-1}
        style={pos ? { left: pos.left, top: pos.top } : { left: 0, top: 0, visibility: "hidden" }}
        initial={{ opacity: 0, scale: 0.97 }}
        animate={{ opacity: 1, scale: 1 }}
        exit={{ opacity: 0, scale: 0.98 }}
        transition={{ duration: 0.12, ease: "easeOut" }}
      >
        {body}
      </motion.div>
      {feedMenu}
    </>
  );
}

interface BodyProps {
  card: EventCardState;
  onDismiss: () => void;
  onClose: () => void;
  onDragStart?: (event: React.PointerEvent) => void;
  onOpenMenu: (feed: Feed) => void;
}

function CardBody({ card, onDismiss, onClose, onDragStart, onOpenMenu }: BodyProps) {
  const feedId = card.kind === "feed" ? card.item.feedId : undefined;
  const feed = usePlanner((state) => (feedId ? state.feeds.find((f) => f.id === feedId) : undefined));
  const toggleFeed = useUi((state) => state.toggleFeed);
  const openEdit = useUi((state) => state.openEdit);
  const createEntry = usePlanner((state) => state.createEntry);
  const toast = usePlanner((state) => state.toast);

  const actions: ReactNode[] = [];
  if (card.kind === "feed") {
    const event = card.item;
    const name = feed?.name ?? event.feedName;
    actions.push(
      <button
        key="hide"
        type="button"
        className="icon-btn"
        aria-label="Hide this calendar"
        title="Hide this calendar"
        onClick={() => {
          toggleFeed(event.feedId);
          onClose();
          toast(`${name} hidden`, { label: "Undo", run: () => toggleFeed(event.feedId) });
        }}
      >
        <EyeOff size={16} />
      </button>,
    );
    if (feed) {
      actions.push(
        <button key="edit" type="button" className="icon-btn" aria-label="Rename or recolour" title="Rename / colour…" onClick={() => onOpenMenu(feed)}>
          <Palette size={16} />
        </button>,
      );
    }
    actions.push(
      <button
        key="copy"
        type="button"
        className="icon-btn"
        aria-label="Copy to my calendar"
        title="Copy to my calendar"
        onClick={() => {
          void createEntry({
            title: event.title,
            type: "event",
            date: event.date,
            startTime: event.startTime,
            // An own entry has one day, so a multi-day copy keeps only its start.
            endTime: event.endDate ? undefined : event.endTime,
            color: feed?.color ?? event.color,
            content: [descriptionToText(event.details), event.location && `Location: ${event.location}`, event.url]
              .filter(Boolean)
              .join("\n\n"),
          }).then((entry) => {
            if (!entry) return;
            toast("Copied to your calendar");
            openEdit(entry);
          });
        }}
      >
        <CopyPlus size={16} />
      </button>,
    );
  }

  return (
    <>
      <div
        className="card-top"
        onPointerDown={(event) => {
          if (onDragStart && !(event.target as Element).closest("button")) onDragStart(event);
        }}
      >
        {onDragStart && <span className="sheet-handle" aria-hidden="true" />}
        <div className="card-actions">
          {actions}
          <button type="button" className="icon-btn" aria-label="Close" onClick={onDismiss}>
            <X size={16} />
          </button>
        </div>
      </div>
      <div className="card-body">
        {card.kind === "feed" && <FeedDetails event={card.item} color={feed?.color} name={feed?.name} />}
        {card.kind === "holiday" && <HolidayDetails holidays={card.item} onClose={onClose} />}
        {card.kind === "person" && <PersonDetails occasion={card.item} onClose={onClose} />}
      </div>
    </>
  );
}

function Heading({ color, title, children }: { color: string; title: string; children?: ReactNode }) {
  return (
    <div className="card-row card-head">
      <span className={`card-square color-${color}`} aria-hidden="true" />
      <div>
        <h2 className="card-title">{title}</h2>
        {children}
      </div>
    </div>
  );
}

function When({ when }: { when: { day: string; time?: string } }) {
  return (
    <>
      <p className="card-when">{when.day}</p>
      {when.time && <p className="card-when">{when.time}</p>}
    </>
  );
}

function Row({ icon, children, className = "" }: { icon: ReactNode; children: ReactNode; className?: string }) {
  return (
    <div className={`card-row ${className}`}>
      <span className="card-icon" aria-hidden="true">
        {icon}
      </span>
      <div className="card-cell">{children}</div>
    </div>
  );
}

function Description({ text }: { text: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [clamped, setClamped] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el && !open) setClamped(el.scrollHeight > el.clientHeight + 1);
  }, [text, open]);
  return (
    <>
      <div ref={ref} className={`card-desc ${open ? "" : "clamp"}`}>
        {linkify(text)}
      </div>
      {(clamped || open) && (
        <button type="button" className="card-more" onClick={() => setOpen(!open)}>
          {open ? "Show less" : "Show more"}
        </button>
      )}
    </>
  );
}

function FeedDetails({ event, color, name }: { event: FeedEvent; color?: string; name?: string }) {
  const toast = usePlanner((state) => state.toast);
  const description = descriptionToText(event.details);
  const location = event.location?.trim();
  const locationIsLink = Boolean(location && /^https?:\/\/\S+$/i.test(location));
  const link = event.url && /^https?:\/\//i.test(event.url) ? event.url : undefined;
  const image = event.image && /^https:\/\//i.test(event.image) ? event.image : undefined;
  const tone = color ?? event.color;
  return (
    <>
      <Heading color={tone} title={event.title || "(No title)"}>
        <When when={eventWhen(event)} />
      </Heading>
      {location && (
        <Row icon={<MapPin size={16} />} className="card-location">
          {locationIsLink ? (
            <a href={location} target="_blank" rel="noreferrer">
              {location}
            </a>
          ) : (
            <span>{location}</span>
          )}
          <button
            type="button"
            className="icon-btn card-copy"
            aria-label="Copy location"
            onClick={() =>
              void navigator.clipboard
                .writeText(location)
                .then(() => toast("Location copied"))
                .catch(() => toast("Copy failed — clipboard unavailable"))
            }
          >
            <Copy size={14} />
          </button>
        </Row>
      )}
      {description && (
        <Row icon={<AlignLeft size={16} />}>
          <Description text={description} />
        </Row>
      )}
      {link && (
        <Row icon={<ExternalLink size={16} />}>
          <a href={link} target="_blank" rel="noreferrer">
            Open link ↗
          </a>
        </Row>
      )}
      {image && (
        <Row icon={null}>
          <img
            className="card-image"
            src={image}
            alt=""
            loading="lazy"
            referrerPolicy="no-referrer"
            onError={(e) => {
              e.currentTarget.style.display = "none";
            }}
          />
        </Row>
      )}
      <Row icon={<CalendarDays size={16} />} className="card-source">
        <span className="card-feed">
          <span className={`card-dot color-${tone}`} aria-hidden="true" />
          {name ?? event.feedName}
        </span>
        <span className="card-muted">Subscribed calendar · read-only</span>
      </Row>
    </>
  );
}

function HolidayDetails({ holidays, onClose }: { holidays: Holiday[]; onClose: () => void }) {
  const navigate = useNavigate();
  const first = holidays[0];
  if (!first) return null;
  return (
    <>
      <Heading color="slate" title={holidays.map((h) => h.name).join(", ")}>
        <When when={{ day: eventWhen({ date: first.date }).day }} />
      </Heading>
      <Row icon={<CalendarDays size={16} />} className="card-source">
        <span>Holiday · {countryName(first.country)}</span>
      </Row>
      <Row icon={<Info size={16} />} className="card-source">
        <span className="card-muted">
          To hide holidays:{" "}
          <button
            type="button"
            className="card-link"
            onClick={() => {
              onClose();
              navigate("/settings");
            }}
          >
            Settings → Holidays
          </button>
        </span>
      </Row>
    </>
  );
}

function PersonDetails({ occasion, onClose }: { occasion: PersonOccurrence; onClose: () => void }) {
  const navigate = useNavigate();
  const birthday = occasion.label === "birthday";
  const label = birthday ? "Birthday" : occasion.label.charAt(0).toUpperCase() + occasion.label.slice(1);
  return (
    <>
      <Heading color={occasion.color} title={occasion.name}>
        <When when={{ day: eventWhen({ date: occasion.date }).day }} />
      </Heading>
      <Row icon={birthday ? <Cake size={16} /> : <Heart size={16} />} className="card-source">
        <span>
          {label}
          {occasion.turning ? ` · ${birthday ? `turns ${occasion.turning}` : `${occasion.turning} years`}` : ""}
        </span>
        <button
          type="button"
          className="card-link"
          onClick={() => {
            onClose();
            navigate(`/people/${occasion.personId}`);
          }}
        >
          Open profile →
        </button>
      </Row>
    </>
  );
}
