package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newZonedServer registers the first user with a browser time zone.
func newZonedServer(t *testing.T, tz string) (*httptest.Server, string) {
	t.Helper()
	srv := newEmptyTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/api/auth/register", `{"email":"z@cal.local","password":"test-pass-123","timezone":"`+tz+`"}`, "")
	var reg struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(resp), &reg); err != nil || reg.Session == "" {
		t.Fatalf("register: %s", resp)
	}
	return srv, reg.Session
}

func settingsOf(t *testing.T, srv *httptest.Server, session string) map[string]any {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

func setTimezone(t *testing.T, srv *httptest.Server, session, tz string) int {
	t.Helper()
	settings := settingsOf(t, srv, session)
	settings["timezone"] = tz
	body, _ := json.Marshal(settings)
	code, _ := doJSONStatus(t, srv, http.MethodPut, "/api/settings", string(body), session)
	return code
}

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestRegisterTimezone(t *testing.T) {
	srv, session := newZonedServer(t, "Europe/Bucharest")
	if tz := settingsOf(t, srv, session)["timezone"]; tz != "Europe/Bucharest" {
		t.Fatalf("timezone after register: %v", tz)
	}
	// "Local" would be the server's zone in Go and isn't a zone Postgres
	// knows, so it's refused as a setting.
	if code := setTimezone(t, srv, session, "Local"); code != http.StatusBadRequest {
		t.Errorf("Local: %d, want 400", code)
	}
	if code := setTimezone(t, srv, session, "Mars/Olympus"); code != http.StatusBadRequest {
		t.Errorf("unknown zone: %d, want 400", code)
	}
	if code := setTimezone(t, srv, session, "America/New_York"); code != http.StatusOK {
		t.Errorf("valid zone: %d", code)
	}

	// An unknown zone at sign-up is ignored: the account stays on UTC.
	openRegistration(t, srv, session)
	for _, tz := range []string{"Local", "Mars/Olympus"} {
		resp := doJSON(t, srv, http.MethodPost, "/api/auth/register", `{"email":"`+strings.ReplaceAll(tz, "/", "")+`@cal.local","password":"test-pass-123","timezone":"`+tz+`"}`, "")
		var reg struct {
			Session string `json:"session"`
		}
		if err := json.Unmarshal([]byte(resp), &reg); err != nil || reg.Session == "" {
			t.Fatalf("register %s: %s", tz, resp)
		}
		if got := settingsOf(t, srv, reg.Session)["timezone"]; got != "UTC" {
			t.Errorf("register with %q: timezone %v, want UTC", tz, got)
		}
	}
}

// Events at known instants, for the feed and import tests. The range tests
// rely on "Late Oct 31" (23:30 on Oct 31 in Bucharest) and "Early Nov 1"
// (01:30 on Nov 1 there), which are both Oct 31 in UTC.
const zonedFeedICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" +
	"BEGIN:VEVENT\r\nUID:z1\r\nSUMMARY:Exported\r\nDTSTART:20261022T070000Z\r\nDTEND:20261022T080000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z2\r\nSUMMARY:Outlook\r\nDTSTART;TZID=GTB Standard Time:20261022T100000\r\nDTEND;TZID=GTB Standard Time:20261022T110000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z3\r\nSUMMARY:Floating\r\nDTSTART:20261022T100000\r\nDTEND:20261022T110000\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z4\r\nSUMMARY:First 03:30\r\nDTSTART:20261025T003000Z\r\nDTEND:20261025T004500Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z5\r\nSUMMARY:Second 03:30\r\nDTSTART:20261025T013000Z\r\nDTEND:20261025T020000Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z6\r\nSUMMARY:Late Oct 31\r\nDTSTART:20261031T213000Z\r\nDTEND:20261031T214500Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z7\r\nSUMMARY:Early Nov 1\r\nDTSTART:20261031T233000Z\r\nDTEND:20261031T234500Z\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z8\r\nSUMMARY:Halloween\r\nDTSTART;VALUE=DATE:20261031\r\nDTEND;VALUE=DATE:20261101\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:z9\r\nSUMMARY:Skipped 03:30\r\nDTSTART:20270328T033000\r\nDTEND:20270328T050000\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

type zonedFeedEvent struct {
	Title     string
	Date      string
	StartTime *string
	EndTime   *string
}

func (e zonedFeedEvent) slot() string {
	s := e.Date
	if e.StartTime != nil {
		s += " " + *e.StartTime
	}
	if e.EndTime != nil {
		s += "-" + *e.EndTime
	}
	return s
}

func feedEventsIn(t *testing.T, srv *httptest.Server, session, from, to string) map[string]string {
	t.Helper()
	var events []zonedFeedEvent
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/feed-events?from="+from+"&to="+to, "", session)), &events); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range events {
		out[e.Title] = e.slot()
	}
	return out
}

func TestFeedEventsInUserZone(t *testing.T) {
	t.Setenv("CAL_ALLOW_PRIVATE_FEEDS", "1")
	ics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, zonedFeedICS) }))
	t.Cleanup(ics.Close)
	srv, session := newZonedServer(t, "Europe/Bucharest")
	if code, resp := doJSONStatus(t, srv, http.MethodPost, "/api/feeds", `{"name":"Google","url":"`+ics.URL+`/g.ics"}`, session); code >= 300 {
		t.Fatalf("create feed: %d %s", code, resp)
	}

	got := feedEventsIn(t, srv, session, "2026-10-01", "2026-11-30")
	want := map[string]string{
		"Exported":     "2026-10-22 10:00-11:00",
		"Outlook":      "2026-10-22 10:00-11:00",
		"Floating":     "2026-10-22 10:00-11:00",
		"First 03:30":  "2026-10-25 03:30-03:45",
		"Second 03:30": "2026-10-25 03:30-04:00",
		"Late Oct 31":  "2026-10-31 23:30-23:45",
		"Early Nov 1":  "2026-11-01 01:30-01:45",
		"Halloween":    "2026-10-31",
	}
	for title, w := range want {
		if got[title] != w {
			t.Errorf("%s: %q, want %q", title, got[title], w)
		}
	}
	if _, ok := got["Skipped 03:30"]; ok {
		t.Errorf("a March event is outside the range")
	}
	if got := feedEventsIn(t, srv, session, "2027-03-28", "2027-03-28")["Skipped 03:30"]; got != "2027-03-28 04:30-05:00" {
		t.Errorf("Skipped 03:30: %q, want 2027-03-28 04:30-05:00", got)
	}

	// The range is local days: Nov 1 starts at local midnight (22:00 UTC on
	// Oct 31), so it has the 01:30 event but not the 23:30 one before it.
	nov1 := feedEventsIn(t, srv, session, "2026-11-01", "2026-11-01")
	if _, ok := nov1["Early Nov 1"]; !ok {
		t.Errorf("Nov 1 is missing the 01:30 event: %v", nov1)
	}
	for _, title := range []string{"Late Oct 31", "Halloween"} {
		if _, ok := nov1[title]; ok {
			t.Errorf("Nov 1 has %s", title)
		}
	}

	// A UTC user sees the same instants on the UTC clock; floating times
	// are then UTC wall clock.
	if code := setTimezone(t, srv, session, "UTC"); code != http.StatusOK {
		t.Fatalf("set UTC: %d", code)
	}
	utc := feedEventsIn(t, srv, session, "2026-10-01", "2026-11-30")
	for title, w := range map[string]string{
		"Exported":    "2026-10-22 07:00-08:00",
		"Outlook":     "2026-10-22 07:00-08:00",
		"Floating":    "2026-10-22 10:00-11:00",
		"Early Nov 1": "2026-10-31 23:30-23:45",
		"Halloween":   "2026-10-31",
	} {
		if utc[title] != w {
			t.Errorf("UTC user, %s: %q, want %q", title, utc[title], w)
		}
	}
}

// The import reads each time in the user's zone. The events are a month
// from now, since the import only keeps the year around today.
func TestImportICSInUserZone(t *testing.T) {
	srv, session := newZonedServer(t, "Europe/Bucharest")
	loc := mustZone(t, "Europe/Bucharest")
	day := time.Now().AddDate(0, 1, 0)
	d := day.Format("20060102")
	date := day.Format(time.DateOnly)
	z := time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, time.UTC)
	zStart, zEnd := z.In(loc).Format("15:04"), z.Add(time.Hour).In(loc).Format("15:04")
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\nUID:i1\r\nSUMMARY:Exported\r\nDTSTART:" + d + "T080000Z\r\nDTEND:" + d + "T090000Z\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i2\r\nSUMMARY:Berlin\r\nDTSTART;TZID=Europe/Berlin:" + d + "T090000\r\nDTEND;TZID=Europe/Berlin:" + d + "T100000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i3\r\nSUMMARY:Outlook\r\nDTSTART;TZID=GTB Standard Time:" + d + "T100000\r\nDTEND;TZID=GTB Standard Time:" + d + "T110000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i4\r\nSUMMARY:Unknown\r\nDTSTART;TZID=Mars Standard Time:" + d + "T100000\r\nDTEND;TZID=Mars Standard Time:" + d + "T110000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i5\r\nSUMMARY:Floating\r\nDTSTART:" + d + "T100000\r\nDURATION:PT1H\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i6\r\nSUMMARY:All day\r\nDTSTART;VALUE=DATE:" + d + "\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:i7\r\nSUMMARY:Overnight\r\nDTSTART;TZID=Europe/Bucharest:" + d + "T230000\r\nDURATION:PT2H\r\nEND:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/import", strings.NewReader(ics))
	req.Header.Set("Authorization", "Bearer "+session)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var result struct{ Imported, ZoneGuessed int }
	_ = json.NewDecoder(res.Body).Decode(&result)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || result.Imported != 7 || result.ZoneGuessed != 1 {
		t.Fatalf("import: %d %+v, want 7 imported and 1 guessed zone", res.StatusCode, result)
	}

	var entries []struct {
		Title, Date        string
		StartTime, EndTime *string
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/entries?from="+date+"&to="+date, "", session)), &entries); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Title] = zonedFeedEvent{Title: e.Title, Date: e.Date, StartTime: e.StartTime, EndTime: e.EndTime}.slot()
	}
	want := map[string]string{
		"Exported": date + " " + zStart + "-" + zEnd,
		"Berlin":   date + " 10:00-11:00",
		"Outlook":  date + " 10:00-11:00",
		"Unknown":  date + " 10:00-11:00",
		"Floating": date + " 10:00-11:00",
		"All day":  date,
		// Ends on the next day: an entry covers one day, so only its start.
		"Overnight": date + " 23:00",
	}
	for title, w := range want {
		if got[title] != w {
			t.Errorf("%s: %q, want %q", title, got[title], w)
		}
	}
}

// "Today" on the server is the user's local date. Kiritimati (UTC+14) and
// Pago Pago (UTC-11) are 25 hours apart, so their dates always differ.
func TestTodayInUserZone(t *testing.T) {
	srv, session := newZonedServer(t, "Pacific/Kiritimati")
	settings := settingsOf(t, srv, session)
	intakeToken, _ := settings["intakeToken"].(string)
	apiToken, _ := settings["apiToken"].(string)
	east, west := mustZone(t, "Pacific/Kiritimati"), mustZone(t, "Pacific/Pago_Pago")

	intakeDate := func() string {
		t.Helper()
		resp := doJSON(t, srv, http.MethodPost, "/api/intake?token="+url.QueryEscape(intakeToken), `{"subject":"mail"}`, "")
		var e struct{ Date string }
		_ = json.Unmarshal([]byte(resp), &e)
		return e.Date
	}
	// A task due 14 days from today in Kiritimati is a deadline there, but
	// 15 or 16 days away in Pago Pago, past the dashboard's 14-day window.
	due := localDay(east).AddDate(0, 0, 14).Format(time.DateOnly)
	if code, resp := doJSONStatus(t, srv, http.MethodPost, "/api/entries", `{"title":"Due","type":"task","date":"`+due+`"}`, session); code >= 300 {
		t.Fatalf("create task: %d %s", code, resp)
	}
	hasDeadline := func() bool {
		t.Helper()
		var d struct{ Deadlines []struct{ Title string } }
		_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/dashboard", "", session)), &d)
		for _, e := range d.Deadlines {
			if e.Title == "Due" {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		zone string
		loc  *time.Location
		due  bool
	}{{"Pacific/Kiritimati", east, true}, {"Pacific/Pago_Pago", west, false}} {
		if code := setTimezone(t, srv, session, c.zone); code != http.StatusOK {
			t.Fatalf("set %s: %d", c.zone, code)
		}
		if got, want := intakeDate(), localDay(c.loc).Format(time.DateOnly); got != want {
			t.Errorf("%s: intake date %s, want %s", c.zone, got, want)
		}
		if got := hasDeadline(); got != c.due {
			t.Errorf("%s: task due %s is a deadline: %v, want %v", c.zone, due, got, c.due)
		}
		// MCP "today" lists the intake task just created for this zone's day.
		text, isErr := mcpCall(t, srv, apiToken, "today", `{}`)
		if isErr || !strings.Contains(text, `"date":"`+localDay(c.loc).Format(time.DateOnly)+`"`) {
			t.Errorf("%s: MCP today: %s", c.zone, text)
		}
	}
	if localDay(east).Equal(localDay(west)) {
		t.Fatal("test zones share a date")
	}
}
