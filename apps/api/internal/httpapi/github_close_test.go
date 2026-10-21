package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Completing a card linked to a GitHub issue writes to GitHub only when the
// user turned githubCloseIssues on.
func TestGitHubCloseIssueIsOptIn(t *testing.T) {
	var patches atomic.Int32
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/repos/o/r/issues/7" {
			patches.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(gh.Close)
	old := ghAPI
	ghAPI = gh.URL
	t.Cleanup(func() { ghAPI = old })

	srv, session := newTestServer(t)
	if code, body := doJSONStatus(t, srv, http.MethodPut, "/api/settings/github-token", `{"token":"ghp_test"}`, session); code != http.StatusOK {
		t.Fatalf("set token: %d %s", code, body)
	}
	boardID, colID := newBoard(t, srv, session, "B")
	var card struct {
		ID string `json:"id"`
	}
	resp := doJSON(t, srv, http.MethodPost, "/api/entries",
		`{"title":"fix","type":"task","date":"2026-10-21","linkUrl":"https://github.com/o/r/issues/7","boardId":"`+boardID+`","columnId":"`+colID+`"}`, session)
	if err := json.Unmarshal([]byte(resp), &card); err != nil || card.ID == "" {
		t.Fatalf("create card: %s", resp)
	}
	complete := func(done bool) {
		t.Helper()
		body := `{"completed":false}`
		if done {
			body = `{"completed":true}`
		}
		if code, resp := doJSONStatus(t, srv, http.MethodPatch, "/api/entries/"+card.ID, body, session); code != http.StatusOK {
			t.Fatalf("patch: %d %s", code, resp)
		}
	}

	complete(true)
	time.Sleep(300 * time.Millisecond)
	if n := patches.Load(); n != 0 {
		t.Fatalf("issue closed %d times with the setting off", n)
	}

	var settings map[string]any
	_ = json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &settings)
	settings["githubCloseIssues"] = true
	body, _ := json.Marshal(settings)
	if code, resp := doJSONStatus(t, srv, http.MethodPut, "/api/settings", string(body), session); code != http.StatusOK {
		t.Fatalf("enable: %d %s", code, resp)
	}
	complete(false)
	complete(true)
	for i := 0; i < 40 && patches.Load() == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if n := patches.Load(); n != 1 {
		t.Fatalf("issue closed %d times with the setting on, want 1", n)
	}
}
