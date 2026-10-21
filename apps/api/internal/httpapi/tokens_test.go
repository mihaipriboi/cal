package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"cal/apps/api/internal/store"
	"cal/apps/api/migrations"
)

// Each capability token opens only its own endpoint.
func TestTokensAreScoped(t *testing.T) {
	srv, session := newTestServer(t)
	var st struct {
		APIToken, WidgetToken, IntakeToken, FeedToken string
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &st); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{"api": st.APIToken, "widget": st.WidgetToken, "intake": st.IntakeToken, "feed": st.FeedToken}
	seen := map[string]bool{}
	for kind, tok := range tokens {
		if tok == "" || seen[tok] {
			t.Fatalf("%s token empty or shared: %q", kind, tok)
		}
		seen[tok] = true
	}

	intake := func(tok string) int {
		code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/intake?token="+url.QueryEscape(tok), `{"subject":"from mail"}`, "")
		return code
	}
	get := func(path, tok string) int {
		code, _ := doJSONStatus(t, srv, http.MethodGet, path+"?token="+url.QueryEscape(tok), "", "")
		return code
	}
	mcp := func(tok string) int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}

	for kind, tok := range tokens {
		wantIntake, wantFeed, wantWidget, wantMCP := kind == "intake", kind == "feed", kind == "widget", kind == "api"
		if code := intake(tok); (code < 300) != wantIntake {
			t.Errorf("%s token on /api/intake: %d", kind, code)
		}
		if code := get("/api/feed.ics", tok); (code == http.StatusOK) != wantFeed {
			t.Errorf("%s token on /api/feed.ics: %d", kind, code)
		}
		if code := get("/api/widget/today", tok); (code == http.StatusOK) != wantWidget {
			t.Errorf("%s token on /api/widget/today: %d", kind, code)
		}
		if code := mcp(tok); (code == http.StatusOK) != wantMCP {
			t.Errorf("%s token on /api/mcp: %d", kind, code)
		}
	}

	// Intake also takes the token as a Bearer header; rotation retires the old one.
	if code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/intake", `{"subject":"via header"}`, st.IntakeToken); code >= 300 {
		t.Fatalf("intake with Bearer: %d", code)
	}
	var rotated struct {
		IntakeToken string `json:"intakeToken"`
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodPost, "/api/settings/intake-token", "", session)), &rotated); err != nil || rotated.IntakeToken == "" || rotated.IntakeToken == st.IntakeToken {
		t.Fatalf("rotate intake: %+v %v", rotated, err)
	}
	if code := intake(st.IntakeToken); code != http.StatusUnauthorized {
		t.Fatalf("old intake token still works: %d", code)
	}
	if code := intake(rotated.IntakeToken); code >= 300 {
		t.Fatalf("new intake token: %d", code)
	}
}

// Migration 10004 seeds the new tokens from the ones they replace, so
// intake and ICS URLs handed out before keep working.
func TestScopedTokenMigrationKeepsURLs(t *testing.T) {
	ctx := context.Background()
	dsn := newTestDBAt(t, 10003)
	pool, err := store.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	user, err := store.New(pool).CreateUser(ctx, "old@cal.local", "x")
	if err != nil {
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
	var api, widget, intake, feed string
	if err := pool.QueryRow(ctx, `SELECT api_token, widget_token, intake_token, feed_token FROM settings WHERE user_id = $1`, user.ID).Scan(&api, &widget, &intake, &feed); err != nil {
		t.Fatal(err)
	}
	if intake != api || feed != widget {
		t.Fatalf("intake %q (api %q), feed %q (widget %q)", intake, api, feed, widget)
	}
}
