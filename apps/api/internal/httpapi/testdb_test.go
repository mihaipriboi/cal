package httpapi

// DB-backed handler tests: a real Postgres (embedded, or
// CAL_TEST_DATABASE_URL when set) + real migrations + the real router.
// Each test gets its own database so order never matters.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"cal/apps/api/internal/calendar"
	"cal/apps/api/internal/store"
	"cal/apps/api/migrations"
)

var (
	testPGOnce sync.Once
	testPGURL  string
	testPGErr  error
)

// testDatabaseURL boots one embedded Postgres per test run on a free port.
// Skips (not fails) when binaries can't be fetched — e.g. offline dev box.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	if url := strings.TrimSpace(env("CAL_TEST_DATABASE_URL", "")); url != "" {
		return url
	}
	testPGOnce.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			testPGErr = err
			return
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(uint32(port)))
		if err := pg.Start(); err != nil {
			testPGErr = err
			return
		}
		testPGURL = fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%d/postgres?sslmode=disable", port)
	})
	if testPGErr != nil {
		t.Skipf("embedded postgres unavailable: %v", testPGErr)
	}
	return testPGURL
}

// newTestStore creates a fresh, migrated database on the shared instance,
// with a credential key in a temp file.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st := newRawTestStore(t)
	if err := st.InitSecrets(context.Background(), filepath.Join(t.TempDir(), "secret.key")); err != nil {
		t.Fatalf("init secrets: %v", err)
	}
	return st
}

// newRawTestStore is newTestStore without InitSecrets.
func newRawTestStore(t *testing.T) *store.Store {
	t.Helper()
	pool, err := store.Connect(context.Background(), newTestDB(t))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return store.New(pool)
}

// newTestDB creates a fresh, migrated database and returns its DSN.
func newTestDB(t *testing.T) string {
	t.Helper()
	return newTestDBAt(t, 0)
}

// newTestDBAt migrates only up to version (0 = all), for migration tests.
func newTestDBAt(t *testing.T, version int64) string {
	t.Helper()
	base := testDatabaseURL(t)

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()
	dbName := "cal_test_" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))
	dbName = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, dbName)
	if len(dbName) > 55 {
		dbName = dbName[:55]
	}
	if _, err := admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbName)); err != nil {
		t.Fatalf("drop test db: %v", err)
	}
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	dsn := strings.Replace(base, "/postgres?", "/"+dbName+"?", 1)
	if !strings.Contains(dsn, "/"+dbName) {
		t.Fatalf("test DSN rewrite failed for %q", base)
	}
	mig, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	if version > 0 {
		err = goose.UpTo(mig, ".", version)
	} else {
		err = goose.Up(mig, ".")
	}
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = mig.Close()
	return dsn
}

// newEmptyTestServer serves the real router over httptest on a fresh
// database. No users exist.
func newEmptyTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := newTestStore(t)
	t.Setenv("DATA_DIR", t.TempDir())
	router := New(st, calendar.NewHolidayCache())
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

// newTestServer is newEmptyTestServer plus a registered first user (admin).
// Returns the server and that user's session token.
func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := newEmptyTestServer(t)
	// Register a user; the session id doubles as a bearer token.
	resp := doJSON(t, srv, http.MethodPost, "/api/auth/register", `{"email":"t@cal.local","password":"test-pass-123"}`, "")
	var reg struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal([]byte(resp), &reg); err != nil || reg.Session == "" {
		t.Fatalf("register: %s", resp)
	}
	return srv, reg.Session
}

// openRegistration lets further sign-ups in; it's closed once a user exists.
func openRegistration(t *testing.T, srv *httptest.Server, adminSession string) {
	t.Helper()
	doJSON(t, srv, http.MethodPut, "/api/admin/config", `{"allowRegistration":true}`, adminSession)
}

// doJSON sends a request; session "" = anonymous.
func doJSON(t *testing.T, srv *httptest.Server, method, path, body, session string) string {
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
	return buf.String()
}
