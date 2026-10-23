package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The entry editor sends remind with every save: a number sets the reminder,
// null removes it. Out-of-range values are refused.
func TestPatchRemind(t *testing.T) {
	srv, session := newTestServer(t)
	var e struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodPost, "/api/entries", `{"title":"Call","type":"event","date":"2026-11-05","startTime":"09:00"}`, session)), &e)

	remindOf := func(body string) (int, *int) {
		t.Helper()
		code, raw := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+e.ID, body, session)
		var out struct {
			Remind *int `json:"remind"`
		}
		_ = json.Unmarshal([]byte(raw), &out)
		return code, out.Remind
	}
	if code, r := remindOf(`{"title":"Call mum","remind":15}`); code != http.StatusOK || r == nil || *r != 15 {
		t.Fatalf("set remind: %d %v", code, r)
	}
	if code, r := remindOf(`{"title":"Call mum","remind":null}`); code != http.StatusOK || r != nil {
		t.Fatalf("clear remind: %d %v", code, r)
	}
	for _, body := range []string{`{"remind":-1}`, `{"remind":10081}`, `{"remind":"soon"}`} {
		if code, _ := remindOf(body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, code)
		}
	}
}

// The editor sends startTime and endTime as "" for an untimed entry; empty
// means unset, so the end-after-start check must not trip on "" <= "".
func TestPatchEmptyTimes(t *testing.T) {
	srv, session := newTestServer(t)
	var e struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodPost, "/api/entries", `{"title":"Errand","type":"task","date":"2026-11-05"}`, session)), &e)
	patch := func(body string) int {
		t.Helper()
		code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+e.ID, body, session)
		return code
	}
	if code := patch(`{"title":"Errand","startTime":"","endTime":"","remind":null}`); code != http.StatusOK {
		t.Fatalf("editor-style save of an untimed entry: %d", code)
	}
	if code := patch(`{"startTime":"10:00","endTime":"11:00"}`); code != http.StatusOK {
		t.Fatalf("set times: %d", code)
	}
	if code := patch(`{"startTime":"","endTime":""}`); code != http.StatusOK {
		t.Fatalf("clear both times: %d", code)
	}
	for _, body := range []string{`{"startTime":"","endTime":"11:00"}`, `{"startTime":"11:00","endTime":"10:00"}`} {
		if code := patch(body); code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, code)
		}
	}
}
