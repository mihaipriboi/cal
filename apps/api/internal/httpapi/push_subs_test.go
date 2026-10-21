package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// registerSecond opens registration and returns a second user's session.
func registerSecond(t *testing.T, srv *httptest.Server, adminSession, email string) string {
	t.Helper()
	openRegistration(t, srv, adminSession)
	code, body := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register", `{"email":"`+email+`","password":"test-pass-123"}`, "")
	var reg struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(body), &reg); code != http.StatusOK || err != nil || reg.Session == "" {
		t.Fatalf("register %s: %d %s", email, code, body)
	}
	return reg.Session
}

func pushSubCount(t *testing.T, srv *httptest.Server, session string) int {
	t.Helper()
	var subs []json.RawMessage
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/push/subscriptions", "", session)), &subs); err != nil {
		t.Fatal(err)
	}
	return len(subs)
}

// A push endpoint can't be taken over or deleted by another user who merely
// knows it; the same browser (same keys) switching accounts still moves it.
func TestPushSubscriptionOwnership(t *testing.T) {
	srv, alice := newTestServer(t)
	bob := registerSecond(t, srv, alice, "bob@cal.local")
	sub := func(p256dh, auth string) string {
		return `{"endpoint":"https://push.example/ep1","keys":{"p256dh":"` + p256dh + `","auth":"` + auth + `"}}`
	}
	post := func(path, body, session string) {
		t.Helper()
		if code, resp := doJSONStatus(t, srv, http.MethodPost, path, body, session); code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, code, strings.TrimSpace(resp))
		}
	}

	post("/api/push/subscribe", sub("k1", "a1"), alice)

	// Bob knows the endpoint but not the keys: no takeover, no delete.
	post("/api/push/subscribe", sub("k2", "a2"), bob)
	post("/api/push/unsubscribe", `{"endpoint":"https://push.example/ep1"}`, bob)
	if a, b := pushSubCount(t, srv, alice), pushSubCount(t, srv, bob); a != 1 || b != 0 {
		t.Fatalf("after foreign subscribe/unsubscribe: alice %d, bob %d; want 1, 0", a, b)
	}

	// Same browser, now logged in as Bob: identical keys move it.
	post("/api/push/subscribe", sub("k1", "a1"), bob)
	if a, b := pushSubCount(t, srv, alice), pushSubCount(t, srv, bob); a != 0 || b != 1 {
		t.Fatalf("after same-browser switch: alice %d, bob %d; want 0, 1", a, b)
	}

	// The owner can unsubscribe.
	post("/api/push/unsubscribe", `{"endpoint":"https://push.example/ep1"}`, bob)
	if b := pushSubCount(t, srv, bob); b != 0 {
		t.Fatalf("owner unsubscribe left %d", b)
	}
}
