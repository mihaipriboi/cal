package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"cal/apps/api/internal/store"
	"cal/apps/api/migrations"
)

type undatedEntry struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Date      *string `json:"date"`
	StartTime *string `json:"startTime"`
}

func createEntryStatus(t *testing.T, srv *httptest.Server, body, session string) (int, undatedEntry) {
	t.Helper()
	code, raw := doJSONStatus(t, srv, http.MethodPost, "/api/entries", body, session)
	var e undatedEntry
	_ = json.Unmarshal([]byte(raw), &e)
	return code, e
}

func findEntry(t *testing.T, srv *httptest.Server, session, id string) undatedEntry {
	t.Helper()
	var list []undatedEntry
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/entries", "", session)), &list)
	for _, e := range list {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("entry %s not listed", id)
	return undatedEntry{}
}

func listTitles(t *testing.T, raw string) map[string]bool {
	t.Helper()
	var list []undatedEntry
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("list json: %v — %s", err, raw)
	}
	out := map[string]bool{}
	for _, e := range list {
		out[e.Title] = true
	}
	return out
}

// A task may go without a date; anything that needs a day to anchor to
// (events, times, repeats, reminders) still needs one.
func TestUndatedTaskRules(t *testing.T) {
	srv, session := newTestServer(t)

	code, inbox := createEntryStatus(t, srv, `{"title":"Someday","type":"task"}`, session)
	if code != http.StatusCreated || inbox.Date != nil {
		t.Fatalf("undated task: %d %+v", code, inbox)
	}
	for _, body := range []string{
		`{"title":"x","type":"event"}`,
		`{"title":"x","type":"note"}`,
		`{"title":"x","type":"task","startTime":"09:00"}`,
		`{"title":"x","type":"task","recur":"weekly"}`,
		`{"title":"x","type":"task","remind":10}`,
		`{"title":"x","type":"task","date":"not-a-date"}`,
	} {
		if code, _ := createEntryStatus(t, srv, body, session); code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", body, code)
		}
	}

	// Ranged views leave undated tasks out unless asked; no range includes them.
	createEntryStatus(t, srv, `{"title":"Dated","type":"task","date":"2026-11-02"}`, session)
	ranged := listTitles(t, doJSON(t, srv, http.MethodGet, "/api/entries?from=2026-11-01&to=2026-11-30", "", session))
	if ranged["Someday"] || !ranged["Dated"] {
		t.Fatalf("ranged list: %v", ranged)
	}
	withUndated := listTitles(t, doJSON(t, srv, http.MethodGet, "/api/entries?from=2026-11-01&to=2026-11-30&undated=1", "", session))
	if !withUndated["Someday"] || !withUndated["Dated"] {
		t.Fatalf("ranged list with undated=1: %v", withUndated)
	}
	if all := listTitles(t, doJSON(t, srv, http.MethodGet, "/api/entries", "", session)); !all["Someday"] {
		t.Fatalf("unranged list misses the undated task: %v", all)
	}

	// The two task settings round-trip, and a save without them resets to the defaults.
	var st map[string]any
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &st)
	if st["quickAddToday"] != false || st["undatedFirst"] != false {
		t.Fatalf("defaults: %v %v", st["quickAddToday"], st["undatedFirst"])
	}
	st["quickAddToday"], st["undatedFirst"] = true, true
	body, _ := json.Marshal(st)
	if res := doJSON(t, srv, http.MethodPut, "/api/settings", string(body), session); !strings.Contains(res, `"quickAddToday":true`) || !strings.Contains(res, `"undatedFirst":true`) {
		t.Fatalf("save settings: %s", res)
	}
	if all := listTitles(t, doJSON(t, srv, http.MethodGet, "/api/entries", "", session)); !all["Someday"] {
		t.Fatalf("unranged list misses the undated task: %v", all)
	}
	if res := doJSON(t, srv, http.MethodGet, "/api/search?q=Someday", "", session); !strings.Contains(res, "Someday") {
		t.Fatalf("search: %s", res)
	}
}

// Scheduling an undated task and clearing the date again, through PATCH.
func TestUndatedTaskPatch(t *testing.T) {
	srv, session := newTestServer(t)
	_, e := createEntryStatus(t, srv, `{"title":"Inbox","type":"task"}`, session)

	patch := func(body string) (int, undatedEntry) {
		t.Helper()
		code, raw := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+e.ID, body, session)
		var out undatedEntry
		_ = json.Unmarshal([]byte(raw), &out)
		return code, out
	}
	if code, out := patch(`{"date":"2026-11-05","startTime":"09:00"}`); code != http.StatusOK || out.Date == nil || *out.Date != "2026-11-05" {
		t.Fatalf("schedule: %d %+v", code, out)
	}
	// A time needs a date: clearing only the date is refused.
	if code, _ := patch(`{"date":null}`); code != http.StatusBadRequest {
		t.Fatalf("clear date with a time set: %d, want 400", code)
	}
	if code, out := patch(`{"date":null,"startTime":""}`); code != http.StatusOK || out.Date != nil || out.StartTime != nil {
		t.Fatalf("clear date and time: %d %+v", code, out)
	}
	if code, out := patch(`{"date":"2026-11-06"}`); code != http.StatusOK || out.Date == nil {
		t.Fatalf("reschedule: %d %+v", code, out)
	}
	if code, out := patch(`{"date":""}`); code != http.StatusOK || out.Date != nil {
		t.Fatalf("clear with empty string: %d %+v", code, out)
	}
	// Events can't lose their date.
	_, ev := createEntryStatus(t, srv, `{"title":"Meet","type":"event","date":"2026-11-05"}`, session)
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+ev.ID, `{"date":null}`, session); code != http.StatusBadRequest {
		t.Fatalf("clear an event's date: %d, want 400", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+e.ID, `{"type":"event"}`, session); code != http.StatusBadRequest {
		t.Fatalf("turn an undated task into an event: %d, want 400", code)
	}

	// Revisions keep the missing date, and restoring one brings it back.
	var revs []struct {
		ID   string  `json:"id"`
		Date *string `json:"date"`
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/entries/"+e.ID+"/revisions", "", session)), &revs); err != nil || len(revs) == 0 {
		t.Fatalf("revisions: %v", err)
	}
	var undatedRev, datedRev string
	for _, r := range revs {
		if r.Date == nil && undatedRev == "" {
			undatedRev = r.ID
		}
		if r.Date != nil && datedRev == "" {
			datedRev = r.ID
		}
	}
	if undatedRev == "" || datedRev == "" {
		t.Fatalf("want both dated and undated revisions: %+v", revs)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/entries/"+e.ID+"/restore/"+datedRev, "", session); code != http.StatusNoContent {
		t.Fatalf("restore dated revision: %d", code)
	}
	if got := findEntry(t, srv, session, e.ID); got.Date == nil {
		t.Fatalf("after restoring the dated revision: %+v", got)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/entries/"+e.ID+"/restore/"+undatedRev, "", session); code != http.StatusNoContent {
		t.Fatalf("restore undated revision: %d", code)
	}
	if got := findEntry(t, srv, session, e.ID); got.Date != nil {
		t.Fatalf("after restoring the undated revision: %+v", got)
	}
}

// The ICS feed has no day to place an undated task on; backups keep them.
func TestUndatedTaskExports(t *testing.T) {
	srv, session := newTestServer(t)
	createEntryStatus(t, srv, `{"title":"No day yet","type":"task"}`, session)
	createEntryStatus(t, srv, `{"title":"Has a day","type":"task","date":"2026-11-05"}`, session)

	var st struct{ FeedToken string }
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &st)
	ics := doJSON(t, srv, http.MethodGet, "/api/feed.ics?token="+url.QueryEscape(st.FeedToken), "", "")
	if strings.Contains(ics, "No day yet") || !strings.Contains(ics, "Has a day") {
		t.Fatalf("feed.ics:\n%s", ics)
	}
	if md := doJSON(t, srv, http.MethodGet, "/api/agenda?days=60", "", session); !strings.Contains(md, "## No date") || !strings.Contains(md, "No day yet") {
		t.Fatalf("agenda markdown:\n%s", md)
	}

	export := doJSON(t, srv, http.MethodGet, "/api/export", "", session)
	// Entry IDs are global, so restore into a fresh server (its own database).
	t.Run("restore", func(t *testing.T) {
		other, otherSession := newTestServer(t)
		if p := decodePreview(t, doJSON(t, other, http.MethodPost, "/api/restore?dry=1", export, otherSession)); p.Entries.New != 2 {
			t.Fatalf("restore preview: %+v", p)
		}
		if res := doJSON(t, other, http.MethodPost, "/api/restore", export, otherSession); !strings.Contains(res, `"restored":2`) {
			t.Fatalf("restore: %s", res)
		}
		var list []undatedEntry
		_ = json.Unmarshal([]byte(doJSON(t, other, http.MethodGet, "/api/entries", "", otherSession)), &list)
		if len(list) != 2 {
			t.Fatalf("restored entries: %+v", list)
		}
		for _, e := range list {
			if e.Title == "No day yet" && e.Date != nil {
				t.Fatalf("restored undated task got a date: %+v", e)
			}
		}
	})
}

// Agents and email intake can create undated tasks too.
func TestUndatedTaskMCPAndIntake(t *testing.T) {
	srv, session := newTestServer(t)
	var st struct{ APIToken, IntakeToken string }
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &st)

	text, isErr := mcpCall(t, srv, st.APIToken, "create_entry", `{"title":"Agent inbox","type":"task"}`)
	var e undatedEntry
	if isErr || json.Unmarshal([]byte(text), &e) != nil || e.Date != nil {
		t.Fatalf("MCP undated task: %s", text)
	}
	if text, isErr := mcpCall(t, srv, st.APIToken, "create_entry", `{"title":"x","type":"event"}`); !isErr {
		t.Fatalf("MCP undated event accepted: %s", text)
	}
	if text, isErr := mcpCall(t, srv, st.APIToken, "update_entry", `{"id":"`+e.ID+`","date":"2026-11-05"}`); isErr || !strings.Contains(text, "2026-11-05") {
		t.Fatalf("MCP schedule: %s", text)
	}
	if text, isErr := mcpCall(t, srv, st.APIToken, "update_entry", `{"id":"`+e.ID+`","date":null}`); isErr || strings.Contains(text, `"date"`) {
		t.Fatalf("MCP unschedule: %s", text)
	}
	if text, _ := mcpCall(t, srv, st.APIToken, "list_entries", `{}`); !strings.Contains(text, "Agent inbox") {
		t.Fatalf("MCP list without range: %s", text)
	}
	if text, _ := mcpCall(t, srv, st.APIToken, "list_entries", `{"from":"2026-01-01","to":"2026-12-31"}`); strings.Contains(text, "Agent inbox") {
		t.Fatalf("MCP ranged list includes undated: %s", text)
	}
	if text, _ := mcpCall(t, srv, st.APIToken, "list_entries", `{"from":"2026-01-01","to":"2026-12-31","undated":true}`); !strings.Contains(text, "Agent inbox") {
		t.Fatalf("MCP ranged list with undated: %s", text)
	}

	intake := func(body string) undatedEntry {
		t.Helper()
		code, raw := doJSONStatus(t, srv, http.MethodPost, "/api/intake", body, st.IntakeToken)
		var out undatedEntry
		if code != http.StatusCreated || json.Unmarshal([]byte(raw), &out) != nil {
			t.Fatalf("intake %s: %d %s", body, code, raw)
		}
		return out
	}
	if out := intake(`{"subject":"mail, no date"}`); out.Date == nil || *out.Date != time.Now().Format(time.DateOnly) {
		t.Fatalf("intake without date should be today: %+v", out)
	}
	if out := intake(`{"subject":"mail, null date","date":null}`); out.Date != nil {
		t.Fatalf("intake with null date should be undated: %+v", out)
	}
	if out := intake(`{"subject":"mail, dated","date":"2026-12-24"}`); out.Date == nil || *out.Date != "2026-12-24" {
		t.Fatalf("intake with a date: %+v", out)
	}
}

// The database refuses an undated row that isn't a plain task, whatever the
// API checks; rolling the migration back dates undated tasks to their
// creation day.
func TestUndatedMigration(t *testing.T) {
	ctx := context.Background()
	dsn := newTestDBAt(t, 10006)
	pool, err := store.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	user, err := store.New(pool).CreateUser(ctx, "undated@cal.local", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO entries (user_id, title, type, date) VALUES ($1, 'ok', 'task', NULL)`, user.ID); err != nil {
		t.Fatalf("plain undated task: %v", err)
	}
	for _, q := range []string{
		`INSERT INTO entries (user_id, title, type, date) VALUES ($1, 'x', 'event', NULL)`,
		`INSERT INTO entries (user_id, title, type, date, start_time) VALUES ($1, 'x', 'task', NULL, '09:00')`,
		`INSERT INTO entries (user_id, title, type, date, recur) VALUES ($1, 'x', 'task', NULL, 'daily')`,
		`INSERT INTO entries (user_id, title, type, date, remind) VALUES ($1, 'x', 'task', NULL, 5)`,
	} {
		if _, err := pool.Exec(ctx, q, user.ID); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}

	mig, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer mig.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.DownTo(mig, ".", 10005); err != nil {
		t.Fatalf("down: %v", err)
	}
	var dated bool
	if err := pool.QueryRow(ctx, `SELECT date = created_at::date FROM entries WHERE title = 'ok'`).Scan(&dated); err != nil || !dated {
		t.Fatalf("down should date undated tasks by creation day: %v %v", dated, err)
	}
	if err := goose.Up(mig, "."); err != nil {
		t.Fatalf("up again: %v", err)
	}
}
