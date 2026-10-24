import type { Entry, FeedEvent, Holiday } from "@cal/api-client";
import { Plus, X } from "lucide-react";
import { useMemo, useState } from "react";
import { draggedEntryId, dropTargetProps } from "../lib/dnd";
import { formatDayTitle, iso, monthMatrix, timeToMinutes, todayIso, weekdayNames, type WeekStartPref } from "../lib/date";
import type { PersonOccurrence } from "../lib/people";
import { useUi } from "../stores/ui";
import { EntryChip } from "./EntryChip";
import { cardAnchor, cardTrigger } from "./EventCard";
import { FeedChip } from "./FeedChip";
import { PersonChip } from "./PersonChip";

interface Props {
  anchor: Date;
  entries: Entry[];
  feedEvents: FeedEvent[];
  holidays: Holiday[];
  personDates?: PersonOccurrence[];
  weekStart: WeekStartPref;
  onMoveEntry: (id: string, date: string) => void;
}

const MAX_VISIBLE = 4;

export function MonthView({ anchor, entries, feedEvents, holidays, personDates = [], weekStart, onMoveEntry }: Props) {
  const selectDate = useUi((state) => state.selectDate);
  const selectedDate = useUi((state) => state.selectedDate);
  const openCreate = useUi((state) => state.openCreate);
  const openEventCard = useUi((state) => state.openEventCard);
  const [dayModal, setDayModal] = useState<string | null>(null);
  const today = todayIso();

  const days = useMemo(() => monthMatrix(anchor, weekStart), [anchor, weekStart]);
  const names = useMemo(() => weekdayNames(weekStart), [weekStart]);

  const byDate = useMemo(() => {
    const map = new Map<string, Entry[]>();
    for (const entry of entries) {
      if (!entry.date) continue; // undated tasks have no cell
      const list = map.get(entry.date) ?? [];
      list.push(entry);
      map.set(entry.date, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => (timeToMinutes(a.startTime) ?? 9999) - (timeToMinutes(b.startTime) ?? 9999) || a.createdAt.localeCompare(b.createdAt));
    }
    return map;
  }, [entries]);

  const holidayByDate = useMemo(() => {
    const map = new Map<string, Holiday[]>();
    for (const h of holidays) map.set(h.date, [...(map.get(h.date) ?? []), h]);
    return map;
  }, [holidays]);

  const feedByDate = useMemo(() => {
    const map = new Map<string, FeedEvent[]>();
    for (const event of feedEvents) {
      const list = map.get(event.date) ?? [];
      list.push(event);
      map.set(event.date, list);
    }
    for (const list of map.values()) {
      list.sort((a, b) => (timeToMinutes(a.startTime) ?? 9999) - (timeToMinutes(b.startTime) ?? 9999));
    }
    return map;
  }, [feedEvents]);

  const personByDate = useMemo(() => {
    const map = new Map<string, PersonOccurrence[]>();
    for (const p of personDates) {
      const list = map.get(p.date) ?? [];
      list.push(p);
      map.set(p.date, list);
    }
    return map;
  }, [personDates]);

  const modalDate = dayModal;
  const modalEntries = modalDate ? byDate.get(modalDate) ?? [] : [];
  const modalFeeds = modalDate ? feedByDate.get(modalDate) ?? [] : [];
  const modalPeople = modalDate ? personByDate.get(modalDate) ?? [] : [];

  return (
    <div className="month-wrap">
      <div className="dow-row">
        {names.map((name) => (
          <span key={name}>{name}</span>
        ))}
      </div>
      <div className="month-grid">
        {days.map((day) => {
          const date = iso(day);
          const dayEntries = byDate.get(date) ?? [];
          const dayFeeds = feedByDate.get(date) ?? [];
          const dayPeople = personByDate.get(date) ?? [];
          const dayHolidays = holidayByDate.get(date);
          const isOutside = day.getMonth() !== anchor.getMonth();
          const visible = dayEntries.slice(0, MAX_VISIBLE);
          const visibleFeeds = dayFeeds.slice(0, Math.max(0, MAX_VISIBLE - visible.length));
          const visiblePeople = dayPeople.slice(0, Math.max(0, MAX_VISIBLE - visible.length - visibleFeeds.length));
          const hidden = dayEntries.length + dayFeeds.length + dayPeople.length - visible.length - visibleFeeds.length - visiblePeople.length;
          return (
            <div
              key={date}
              className={`day-cell ${date === today ? "today" : ""} ${date === selectedDate ? "selected" : ""} ${isOutside ? "outside" : ""}`}
              role="button"
              tabIndex={0}
              aria-label={date}
              onClick={() => selectDate(date)}
              onKeyDown={(event) => {
                if (event.key === "Enter") openCreate(date);
              }}
              onDoubleClick={() => openCreate(date)}
              {...dropTargetProps((event) => {
                const id = draggedEntryId(event);
                if (id) onMoveEntry(id, date);
              })}
            >
              <div className="day-head">
                <span className="day-num">{day.getDate()}</span>
                {dayHolidays && (
                  <button
                    type="button"
                    className="holiday-label"
                    {...cardTrigger((el) => openEventCard({ kind: "holiday", item: dayHolidays, rect: cardAnchor(el) }))}
                  >
                    {dayHolidays.map((h) => h.name).join(", ")}
                  </button>
                )}
                <button
                  type="button"
                  className="icon-btn day-add"
                  aria-label={`Add entry on ${date}`}
                  onClick={(event) => {
                    event.stopPropagation();
                    openCreate(date);
                  }}
                >
                  <Plus size={13} />
                </button>
              </div>
              {visible.map((entry) => (
                <EntryChip key={entry.id} entry={entry} />
              ))}
              {visibleFeeds.map((event) => (
                <FeedChip key={event.id} event={event} />
              ))}
              {visiblePeople.map((p) => (
                <PersonChip key={p.id} occasion={p} />
              ))}
              {hidden > 0 && (
                <button
                  type="button"
                  className="more-link"
                  onClick={(event) => {
                    event.stopPropagation();
                    selectDate(date);
                    setDayModal(date);
                  }}
                >
                  {`+${hidden} more`}
                </button>
              )}
            </div>
          );
        })}
      </div>
      {modalDate && (
        <div className="scrim" onMouseDown={(e) => e.target === e.currentTarget && setDayModal(null)}>
          <div className="palette day-modal" role="dialog" aria-label={`Entries on ${modalDate}`}>
            <div className="day-modal-head">
              <h3>{formatDayTitle(modalDate)}</h3>
              <button
                type="button"
                className="btn btn-primary btn-xs"
                onClick={() => {
                  openCreate(modalDate);
                  setDayModal(null);
                }}
              >
                <Plus size={12} /> Add
              </button>
              <button type="button" className="icon-btn" aria-label="Close" onClick={() => setDayModal(null)}>
                <X size={15} />
              </button>
            </div>
            <div className="day-modal-list">
              {modalEntries.map((entry) => (
                <EntryChip key={entry.id} entry={entry} />
              ))}
              {modalFeeds.map((event) => (
                <FeedChip key={event.id} event={event} />
              ))}
              {modalPeople.map((p) => (
                <PersonChip key={p.id} occasion={p} />
              ))}
              {modalEntries.length === 0 && modalFeeds.length === 0 && modalPeople.length === 0 && (
                <p className="panel-empty">Nothing on this day.</p>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
