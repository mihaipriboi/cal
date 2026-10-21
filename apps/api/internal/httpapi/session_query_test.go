package httpapi

import (
	"net/http"
	"net/url"
	"testing"
)

// ?session= exists for <img src>/file links (GET); it must not authorize writes.
func TestQuerySessionOnlyForReads(t *testing.T) {
	srv, session := newTestServer(t)
	q := "?session=" + url.QueryEscape(session)

	if code, body := doJSONStatus(t, srv, http.MethodGet, "/api/settings"+q, "", ""); code != http.StatusOK {
		t.Fatalf("GET with ?session=: %d %s", code, body)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPut, "/api/settings"+q, `{"timezone":"UTC"}`, ""); code != http.StatusUnauthorized {
		t.Fatalf("PUT with ?session= must be 401, got %d", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/entries"+q, `{"title":"x","type":"task","date":"2026-10-21"}`, ""); code != http.StatusUnauthorized {
		t.Fatalf("POST with ?session= must be 401, got %d", code)
	}
	if code, body := doJSONStatus(t, srv, http.MethodPost, "/api/entries", `{"title":"x","type":"task","date":"2026-10-21"}`, session); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("POST with Bearer: %d %s", code, body)
	}
}
