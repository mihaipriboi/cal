package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cal/apps/api/internal/store"
)

const testICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ev1\r\nSUMMARY:Lecture\r\n" +
	"DTSTART:20261022T090000Z\r\nDTEND:20261022T100000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

type feedJSON struct {
	ID, Name, Color string
}

// Feeds get distinct colours by default and can be renamed and recoloured;
// the new colour applies to their events, and nothing else can change.
func TestFeedRenameAndRecolour(t *testing.T) {
	t.Setenv("CAL_ALLOW_PRIVATE_FEEDS", "1")
	ics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, testICS) }))
	t.Cleanup(ics.Close)
	srv, session := newTestServer(t)

	create := func(body string) (int, feedJSON) {
		code, resp := doJSONStatus(t, srv, http.MethodPost, "/api/feeds", body, session)
		var f feedJSON
		_ = json.Unmarshal([]byte(resp), &f)
		return code, f
	}
	_, a := create(`{"name":"A","url":"` + ics.URL + `/a.ics"}`)
	_, b := create(`{"name":"B","url":"` + ics.URL + `/b.ics"}`)
	if a.Color == "" || a.Color == b.Color {
		t.Fatalf("new feeds should get distinct colours: %q, %q", a.Color, b.Color)
	}
	if code, _ := create(`{"name":"C","url":"` + ics.URL + `/c.ics","color":"chartreuse"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown colour on create: %d", code)
	}

	code, resp := doJSONStatus(t, srv, http.MethodPatch, "/api/feeds/"+a.ID, `{"name":"  Uni  ","color":"rose"}`, session)
	var got feedJSON
	if err := json.Unmarshal([]byte(resp), &got); code != http.StatusOK || err != nil || got.Name != "Uni" || got.Color != "rose" {
		t.Fatalf("patch: %d %s", code, resp)
	}
	var events []struct{ FeedID, Color string }
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/feed-events?from=2026-10-01&to=2026-10-31", "", session)), &events)
	found := false
	for _, e := range events {
		if e.FeedID == a.ID {
			found = true
			if e.Color != "rose" {
				t.Fatalf("event of recoloured feed has colour %q", e.Color)
			}
		}
	}
	if !found {
		t.Fatal("no events for the feed")
	}

	for _, bad := range []string{`{"name":"   "}`, `{"color":"chartreuse"}`, `{"url":"http://elsewhere.example/x.ics"}`} {
		if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/feeds/"+a.ID, bad, session); code != http.StatusBadRequest {
			t.Errorf("patch %s: %d, want 400", bad, code)
		}
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/feeds/00000000-0000-0000-0000-000000000000", `{"color":"red"}`, session); code != http.StatusNotFound {
		t.Errorf("unknown feed: %d", code)
	}
	other := registerSecond(t, srv, session, "other@cal.local")
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/feeds/"+a.ID, `{"color":"red"}`, other); code != http.StatusNotFound {
		t.Errorf("another user's feed: %d", code)
	}
}

func TestNextFeedColor(t *testing.T) {
	var feeds []store.Feed
	seen := map[string]bool{}
	for i := 0; i < len(paletteColors); i++ {
		c := nextFeedColor(feeds)
		if seen[c] || !validColor(c) {
			t.Fatalf("colour %d: %q repeated or invalid", i, c)
		}
		seen[c] = true
		feeds = append(feeds, store.Feed{Color: c})
	}
	if c := nextFeedColor(feeds); !validColor(c) {
		t.Fatalf("cycling colour %q invalid", c)
	}
}
