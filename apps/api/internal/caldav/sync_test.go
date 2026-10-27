package caldav

import (
	"strings"
	"testing"
	"time"

	"cal/apps/api/internal/ical"
	"cal/apps/api/internal/store"
)

// Pushed entries carry the user's zone as TZID; pulled events land on the
// user's local clock.
func TestEntryZones(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Bucharest")
	if err != nil {
		t.Fatal(err)
	}
	start, end := "10:00", "11:00"
	ics := encodeEntry(store.Entry{ID: "1", Title: "Standup", Date: "2026-10-22", StartTime: &start, EndTime: &end}, "cal-1", loc)
	for _, want := range []string{"DTSTART;TZID=Europe/Bucharest:20261022T100000", "DTEND;TZID=Europe/Bucharest:20261022T110000"} {
		if !strings.Contains(ics, want) {
			t.Errorf("push is missing %q:\n%s", want, ics)
		}
	}
	allDay := encodeEntry(store.Entry{ID: "2", Title: "Trip", Date: "2026-10-31"}, "cal-2", loc)
	if !strings.Contains(allDay, "DTSTART;VALUE=DATE:20261031") || !strings.Contains(allDay, "DTEND;VALUE=DATE:20261101") {
		t.Errorf("all-day push:\n%s", allDay)
	}

	remote := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:r\r\nSUMMARY:Call\r\nDTSTART:20261022T070000Z\r\nDTEND:20261022T080000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	evs, err := ical.ParseIn(remote, loc)
	if err != nil || len(evs) != 1 {
		t.Fatalf("parse: %v", err)
	}
	e := entryFrom(evs[0], loc)
	if e.Date != "2026-10-22" || e.StartTime == nil || *e.StartTime != "10:00" || e.EndTime == nil || *e.EndTime != "11:00" {
		t.Errorf("pull: %s %v %v", e.Date, e.StartTime, e.EndTime)
	}
	// Round trip: what was pushed reads back the same.
	back, _ := ical.ParseIn(ics, loc)
	if r := entryFrom(back[0], loc); r.Date != "2026-10-22" || *r.StartTime != "10:00" || *r.EndTime != "11:00" {
		t.Errorf("round trip: %s %v %v", r.Date, *r.StartTime, *r.EndTime)
	}
}
