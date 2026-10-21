package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"

	"cal/apps/api/internal/store"
	"cal/apps/api/migrations"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// The session list identifies sessions by the stored hash, never by the
// token, and revoking by that ID ends the session.
func TestSessionsListedByHash(t *testing.T) {
	srv, session := newTestServer(t)
	body := doJSON(t, srv, http.MethodGet, "/api/sessions", "", session)
	if strings.Contains(body, session) {
		t.Fatal("GET /api/sessions returns the session token")
	}
	var list []struct {
		ID      string `json:"id"`
		Current bool   `json:"current"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list) != 1 {
		t.Fatalf("sessions: %s", body)
	}
	if !list[0].Current || list[0].ID != sha256Hex(session) {
		t.Fatalf("session entry %+v, want current with id sha256(token)", list[0])
	}
	if code, _ := doJSONStatus(t, srv, http.MethodDelete, "/api/sessions/"+list[0].ID, "", session); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodGet, "/api/me", "", session); code != http.StatusUnauthorized {
		t.Fatalf("revoked session still works: %d", code)
	}
}

// Migration 10005 hashes existing session rows in place: logins made before
// the upgrade stay valid and the stored value is no longer the token.
func TestSessionHashMigrationKeepsLogins(t *testing.T) {
	ctx := context.Background()
	dsn := newTestDBAt(t, 10004)
	pool, err := store.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	st := store.New(pool)
	user, err := st.CreateUser(ctx, "old@cal.local", "x")
	if err != nil {
		t.Fatal(err)
	}
	token := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (id, user_id, expires_at) VALUES ($1, $2, now() + interval '1 day')`, token, user.ID); err != nil {
		t.Fatal(err)
	}
	mig, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer mig.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.Up(mig, "."); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT id FROM sessions WHERE user_id = $1`, user.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != sha256Hex(token) {
		t.Fatalf("stored %q, want sha256 of the token", stored)
	}
	if got, err := st.UserBySession(ctx, token); err != nil || got.ID != user.ID {
		t.Fatalf("pre-upgrade token no longer signs in: %v", err)
	}
}
