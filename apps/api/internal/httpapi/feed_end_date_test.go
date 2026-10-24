package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cal/apps/api/internal/ical"
)

const multiDayICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:trip\r\nSUMMARY:Trip\r\nDTSTART;VALUE=DATE:20261030\r\nDTEND;VALUE=DATE:20261103\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:party\r\nSUMMARY:Party\r\nDTSTART:20261024T220000Z\r\nDTEND:20261025T020000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:holiday\r\nSUMMARY:Holiday\r\nDTSTART;VALUE=DATE:20261031\r\nDTEND;VALUE=DATE:20261101\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:late\r\nSUMMARY:Late\r\nDTSTART:20261026T220000Z\r\nDTEND:20261027T000000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:lecture\r\nSUMMARY:Lecture\r\nDTSTART:20261022T090000Z\r\nDTEND:20261022T100000Z\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// An event that ends on a later day carries its last day; single-day events,
// including an all-day one (exclusive DTEND) and one ending at midnight,
// carry none.
var wantEndDate = map[string]string{
	"Trip":    "2026-11-02",
	"Party":   "2026-10-25",
	"Holiday": "",
	"Late":    "",
	"Lecture": "",
}

func TestFeedEndDate(t *testing.T) {
	events, err := ical.Parse(multiDayICS)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if got := feedEndDate(e); got != wantEndDate[e.Summary] {
			t.Errorf("%s: endDate %q, want %q", e.Summary, got, wantEndDate[e.Summary])
		}
	}
	// The end is read in the start's zone.
	bucharest, err := time.LoadLocation("Europe/Bucharest")
	if err != nil {
		t.Fatal(err)
	}
	mixed := ical.Event{
		Start: time.Date(2026, 10, 24, 20, 0, 0, 0, bucharest),
		End:   time.Date(2026, 10, 24, 21, 30, 0, 0, time.UTC), // 00:30 the next day in Bucharest
	}
	if got := feedEndDate(mixed); got != "2026-10-25" {
		t.Errorf("mixed zones: endDate %q", got)
	}
}

func TestFeedEventsEndDate(t *testing.T) {
	t.Setenv("CAL_ALLOW_PRIVATE_FEEDS", "1")
	ics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, multiDayICS) }))
	t.Cleanup(ics.Close)
	srv, session := newTestServer(t)
	if code, resp := doJSONStatus(t, srv, http.MethodPost, "/api/feeds", `{"name":"Trips","url":"`+ics.URL+`/t.ics"}`, session); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create feed: %d %s", code, resp)
	}
	var events []struct {
		Title   string
		Date    string
		EndDate *string
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/feed-events?from=2026-10-01&to=2026-11-30", "", session)), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != len(wantEndDate) {
		t.Fatalf("got %d events, want %d", len(events), len(wantEndDate))
	}
	for _, e := range events {
		want := wantEndDate[e.Title]
		switch {
		case want == "" && e.EndDate != nil:
			t.Errorf("%s: endDate %q should be omitted", e.Title, *e.EndDate)
		case want != "" && (e.EndDate == nil || *e.EndDate != want):
			t.Errorf("%s: endDate %v, want %q", e.Title, e.EndDate, want)
		}
	}
}
