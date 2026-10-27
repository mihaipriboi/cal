package ical

import (
	"testing"
	"time"
)

func bucharest(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Bucharest")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// Every event below is 10:00–11:00 in Bucharest on 2026-10-22 (UTC+3), or
// all day on 2026-10-31.
const zonesICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:utc\r\nSUMMARY:UTC\r\nDTSTART:20261022T070000Z\r\nDTEND:20261022T080000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:iana\r\nSUMMARY:IANA\r\nDTSTART;TZID=Europe/Berlin:20261022T090000\r\nDTEND;TZID=Europe/Berlin:20261022T100000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:windows\r\nSUMMARY:Windows\r\nDTSTART;TZID=GTB Standard Time:20261022T100000\r\nDTEND;TZID=GTB Standard Time:20261022T110000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:prefix\r\nSUMMARY:Prefix\r\nDTSTART;TZID=/mozilla.org/20050126_1/Europe/Bucharest:20261022T100000\r\nDURATION:PT1H\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:unknown\r\nSUMMARY:Unknown\r\nDTSTART;TZID=\"Mars Standard Time\":20261022T100000\r\nDTEND;TZID=\"Mars Standard Time\":20261022T110000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:floating\r\nSUMMARY:Floating\r\nDURATION:PT1H\r\nDTSTART:20261022T100000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:date\r\nSUMMARY:Date\r\nDTSTART;VALUE=DATE:20261031\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func TestParseInZones(t *testing.T) {
	loc := bucharest(t)
	events, err := ParseIn(zonesICS, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 7 {
		t.Fatalf("got %d events", len(events))
	}
	for _, e := range events {
		date, start, end := LocalSlot(e, loc)
		got := date + " " + start + "-" + end
		want := "2026-10-22 10:00-11:00"
		if e.Summary == "Date" {
			want = "2026-10-31 -"
		}
		if got != want {
			t.Errorf("%s: %q, want %q", e.Summary, got, want)
		}
		if unknown := e.Summary == "Unknown"; (e.UnknownTZID != "") != unknown {
			t.Errorf("%s: UnknownTZID %q", e.Summary, e.UnknownTZID)
		}
	}
	// The same file read by a UTC reader: instants stay put, and floating
	// and unknown-zone times are 10:00 UTC.
	utc, _ := ParseIn(zonesICS, time.UTC)
	for _, e := range utc {
		_, start, _ := LocalSlot(e, time.UTC)
		want := "07:00"
		switch e.Summary {
		case "Unknown", "Floating":
			want = "10:00"
		case "Date":
			want = ""
		}
		if start != want {
			t.Errorf("UTC reader, %s: start %q, want %q", e.Summary, start, want)
		}
	}
}

func TestWallTimeDST(t *testing.T) {
	loc := bucharest(t)
	cases := []struct{ wall, want string }{
		{"2026-10-25 02:30", "2026-10-24T23:30:00Z"}, // before the fall-back hour
		{"2026-10-25 03:30", "2026-10-25T00:30:00Z"}, // happens twice: the first one (EEST)
		{"2026-10-25 04:30", "2026-10-25T02:30:00Z"}, // after it (EET)
		{"2027-03-28 02:30", "2027-03-28T00:30:00Z"}, // before the gap (EET)
		{"2027-03-28 03:30", "2027-03-28T01:30:00Z"}, // skipped: moves forward to 04:30 EEST
		{"2027-03-28 04:30", "2027-03-28T01:30:00Z"},
		{"2026-07-01 12:00", "2026-07-01T09:00:00Z"},
	}
	for _, c := range cases {
		got, err := LocalTime(c.wall[:10], c.wall[11:], loc)
		if err != nil {
			t.Fatal(err)
		}
		if s := got.UTC().Format(time.RFC3339); s != c.want {
			t.Errorf("%s: %s, want %s", c.wall, s, c.want)
		}
	}
	// A zone without DST and UTC itself are plain offsets.
	if got, _ := LocalTime("2026-10-25", "03:30", time.UTC); got.Format(time.RFC3339) != "2026-10-25T03:30:00Z" {
		t.Errorf("UTC: %v", got)
	}
}

func TestParseInDSTDays(t *testing.T) {
	loc := bucharest(t)
	ics := "BEGIN:VCALENDAR\r\n" +
		// Both read 03:30 in Bucharest on the fall-back day.
		"BEGIN:VEVENT\r\nUID:a\r\nSUMMARY:First 03:30\r\nDTSTART:20261025T003000Z\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:b\r\nSUMMARY:Second 03:30\r\nDTSTART:20261025T013000Z\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:c\r\nSUMMARY:Floating twice\r\nDTSTART:20261025T033000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:d\r\nSUMMARY:Floating skipped\r\nDTSTART:20270328T033000\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	events, err := ParseIn(ics, loc)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"First 03:30":      "2026-10-25 03:30 2026-10-25T00:30:00Z",
		"Second 03:30":     "2026-10-25 03:30 2026-10-25T01:30:00Z",
		"Floating twice":   "2026-10-25 03:30 2026-10-25T00:30:00Z",
		"Floating skipped": "2027-03-28 04:30 2027-03-28T01:30:00Z",
	}
	for _, e := range events {
		date, start, _ := LocalSlot(e, loc)
		if got := date + " " + start + " " + e.Start.UTC().Format(time.RFC3339); got != want[e.Summary] {
			t.Errorf("%s: %q, want %q", e.Summary, got, want[e.Summary])
		}
	}
}

// A weekly series in its own zone keeps its wall-clock time across a DST
// change; a UTC series keeps its UTC time, so its local time moves.
func TestExpandAcrossDST(t *testing.T) {
	loc := bucharest(t)
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\nUID:zoned\r\nSUMMARY:Zoned\r\nDTSTART;TZID=Europe/Bucharest:20261018T090000\r\nRRULE:FREQ=WEEKLY;COUNT=3\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:utc\r\nSUMMARY:UTC\r\nDTSTART:20261018T060000Z\r\nRRULE:FREQ=WEEKLY;COUNT=3\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	events, err := ParseIn(ics, loc)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, e := range Expand(events, time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 11, 30, 0, 0, 0, 0, loc)) {
		date, start, _ := LocalSlot(e, loc)
		got[e.Summary] = append(got[e.Summary], date+" "+start)
	}
	want := map[string]string{
		"Zoned": "[2026-10-18 09:00 2026-10-25 09:00 2026-11-01 09:00]",
		"UTC":   "[2026-10-18 09:00 2026-10-25 08:00 2026-11-01 08:00]",
	}
	for name, w := range want {
		if g := fmtList(got[name]); g != w {
			t.Errorf("%s: %s, want %s", name, g, w)
		}
	}
}

func fmtList(l []string) string {
	out := "["
	for i, s := range l {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out + "]"
}

// A DATE UNTIL includes that whole day, also for a timed series.
func TestUntilDate(t *testing.T) {
	loc := bucharest(t)
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:u\r\nSUMMARY:Class\r\n" +
		"RRULE:FREQ=DAILY;UNTIL=20261022\r\nDTSTART;TZID=Europe/Bucharest:20261020T180000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	events, err := ParseIn(ics, loc)
	if err != nil {
		t.Fatal(err)
	}
	n := len(Expand(events, time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 11, 1, 0, 0, 0, 0, loc)))
	if n != 3 {
		t.Errorf("got %d occurrences, want 3 (Oct 20, 21, 22)", n)
	}
}

func TestLocalSlot(t *testing.T) {
	loc := bucharest(t)
	cross := Event{Start: time.Date(2026, 10, 22, 20, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 22, 22, 0, 0, 0, time.UTC)}
	if d, s, e := LocalSlot(cross, loc); d != "2026-10-22" || s != "23:00" || e != "" {
		t.Errorf("ends on the next local day: %s %s %q", d, s, e)
	}
	if d, s, e := LocalSlot(cross, time.UTC); d != "2026-10-22" || s != "20:00" || e != "22:00" {
		t.Errorf("same day in UTC: %s %s %s", d, s, e)
	}
	late := Event{Start: time.Date(2026, 10, 22, 21, 30, 0, 0, time.UTC), End: time.Date(2026, 10, 22, 22, 0, 0, 0, time.UTC)}
	if d, s, _ := LocalSlot(late, loc); d != "2026-10-23" || s != "00:30" {
		t.Errorf("starts after local midnight: %s %s", d, s)
	}
}
