package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// doJSONStatus is doJSON with the status code — admin tests assert 403/400s.
func doJSONStatus(t *testing.T, srv *httptest.Server, method, path, body, session string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.Header.Set("Authorization", "Bearer "+session)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(res.Body)
	return res.StatusCode, buf.String()
}

func TestAdminBootstrapAndPolicy(t *testing.T) {
	srv, adminSession := newTestServer(t) // registers t@cal.local — first user

	// First user is admin.
	me := doJSON(t, srv, http.MethodGet, "/api/me", "", adminSession)
	var meUser struct {
		ID      string `json:"id"`
		IsAdmin bool   `json:"isAdmin"`
	}
	if err := json.Unmarshal([]byte(me), &meUser); err != nil || !meUser.IsAdmin {
		t.Fatalf("first user must be admin: %s", me)
	}

	// Once a user exists, registration is closed until an admin opens it.
	closedCfg := doJSON(t, srv, http.MethodGet, "/api/auth/config", "", "")
	if !strings.Contains(closedCfg, `"registrationOpen":false`) {
		t.Fatalf("registration must default to closed: %s", closedCfg)
	}
	if code, body := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"early@cal.local","password":"test-pass-123"}`, ""); code != http.StatusForbidden {
		t.Fatalf("default-closed register must 403, got %d %s", code, body)
	}
	openRegistration(t, srv, adminSession)

	// Second user is not admin.
	code, body := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"two@cal.local","password":"test-pass-123"}`, "")
	if code != http.StatusOK {
		t.Fatalf("second register: %d %s", code, body)
	}
	var reg2 struct {
		User struct {
			ID      string `json:"id"`
			IsAdmin bool   `json:"isAdmin"`
		} `json:"user"`
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(body), &reg2); err != nil || reg2.User.IsAdmin {
		t.Fatalf("second user must not be admin: %s", body)
	}

	// Public auth config reflects an occupied, open instance.
	cfg := doJSON(t, srv, http.MethodGet, "/api/auth/config", "", "")
	var ac struct {
		HasUsers         bool `json:"hasUsers"`
		RegistrationOpen bool `json:"registrationOpen"`
	}
	if err := json.Unmarshal([]byte(cfg), &ac); err != nil || !ac.HasUsers || !ac.RegistrationOpen {
		t.Fatalf("auth config: %s", cfg)
	}

	// Non-admin is refused.
	if code, _ := doJSONStatus(t, srv, http.MethodGet, "/api/admin/users", "", reg2.Session); code != http.StatusForbidden {
		t.Fatalf("non-admin must 403, got %d", code)
	}

	// Admin lists both users.
	users := doJSON(t, srv, http.MethodGet, "/api/admin/users", "", adminSession)
	if !strings.Contains(users, "t@cal.local") || !strings.Contains(users, "two@cal.local") {
		t.Fatalf("user list: %s", users)
	}

	// Closing registration blocks the next sign-up.
	if code, _ := doJSONStatus(t, srv, http.MethodPut, "/api/admin/config",
		`{"allowRegistration":false}`, adminSession); code != http.StatusOK {
		t.Fatalf("close registration: %d", code)
	}
	if code, body := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"three@cal.local","password":"test-pass-123"}`, ""); code != http.StatusForbidden {
		t.Fatalf("closed registration must 403, got %d %s", code, body)
	}

	// The sole admin can neither demote nor delete themselves out of adminhood.
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/admin/users/"+meUser.ID,
		`{"isAdmin":false}`, adminSession); code != http.StatusBadRequest {
		t.Fatalf("demoting last admin must 400, got %d", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodDelete, "/api/admin/users/"+meUser.ID, "", adminSession); code != http.StatusBadRequest {
		t.Fatalf("self-delete must 400, got %d", code)
	}

	// Promote the second user, then deleting them is allowed... but they're
	// an admin now, and there are two — deletion still works.
	if code, _ := doJSONStatus(t, srv, http.MethodPatch, "/api/admin/users/"+reg2.User.ID,
		`{"isAdmin":true}`, adminSession); code != http.StatusOK {
		t.Fatalf("promote: %d", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodDelete, "/api/admin/users/"+reg2.User.ID, "", adminSession); code != http.StatusNoContent {
		t.Fatalf("delete second admin (one remains) must 204, got %d", code)
	}
}

// Concurrent first sign-ups: exactly one wins (and is admin); the rest see a
// closed instance rather than racing past the bootstrap check.
func TestConcurrentFirstRegistration(t *testing.T) {
	srv := newEmptyTestServer(t)
	const n = 6 // under the 8/min auth rate limit
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
				fmt.Sprintf(`{"email":"u%d@cal.local","password":"test-pass-123"}`, i), "")
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	ok, forbidden := 0, 0
	for c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusForbidden:
			forbidden++
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 1 || forbidden != n-1 {
		t.Fatalf("want 1 success and %d refusals, got %d / %d", n-1, ok, forbidden)
	}
}

// bcrypt caps input at 72 bytes; longer passwords get a 400 with a reason,
// not a 500 from the hasher.
func TestRegisterRejectsOverlongPassword(t *testing.T) {
	srv := newEmptyTestServer(t)
	code, body := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"long@cal.local","password":"`+strings.Repeat("a", 73)+`"}`, "")
	if code != http.StatusBadRequest || !strings.Contains(body, "72 bytes") {
		t.Fatalf("73-byte password: got %d %q", code, body)
	}
	code, body = doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"long@cal.local","password":"`+strings.Repeat("a", 72)+`"}`, "")
	if code != http.StatusOK {
		t.Fatalf("72-byte password: got %d %q", code, body)
	}
}
