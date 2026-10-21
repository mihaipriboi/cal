package httpapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cal/apps/api/internal/store"
)

// legacyEncrypt reproduces the stored format (base64(nonce|AES-GCM)) so tests
// can seed rows encrypted with the old in-database key.
func legacyEncrypt(t *testing.T, key []byte, plain string) string {
	t.Helper()
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))
}

func randomKey() []byte {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return k
}

func writeKey(t *testing.T, path string, key []byte) {
	t.Helper()
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// legacyDB is a migrated database whose credentials were written by an
// older version: key in server_config, a CalDAV password encrypted with it
// and a plaintext GitHub token.
func legacyDB(t *testing.T) (*pgxpool.Pool, *store.Store, string, []byte) {
	t.Helper()
	ctx := context.Background()
	pool, err := store.Connect(ctx, newTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	st := store.New(pool)
	user, err := st.CreateUser(ctx, "legacy@cal.local", "x")
	if err != nil {
		t.Fatal(err)
	}
	legacy := randomKey()
	if err := st.SetServerConfig(ctx, "crypto_key", base64.StdEncoding.EncodeToString(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCaldavAccount(ctx, user.ID, "cal", "https://dav.example", "me", legacyEncrypt(t, legacy, "dav-pass"), "sky"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settings SET github_token = 'ghp_plain' WHERE user_id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	return pool, st, user.ID, legacy
}

func checkMigrated(t *testing.T, pool *pgxpool.Pool, st *store.Store, userID string) {
	t.Helper()
	ctx := context.Background()
	accounts, err := st.CaldavAccounts(ctx, userID)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("accounts: %v %d", err, len(accounts))
	}
	account, _, err := st.CaldavAccountWithSecret(ctx, accounts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if pw, err := st.Decrypt(ctx, account.PasswordEnc); err != nil || pw != "dav-pass" {
		t.Fatalf("CalDAV password after migration: %q %v", pw, err)
	}
	if tok, err := st.GitHubToken(ctx, userID); err != nil || tok != "ghp_plain" {
		t.Fatalf("GitHub token after migration: %q %v", tok, err)
	}
	var plain *string
	var legacyLeft bool
	_ = pool.QueryRow(ctx, `SELECT github_token FROM settings WHERE user_id = $1`, userID).Scan(&plain)
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM server_config WHERE key = 'crypto_key')`).Scan(&legacyLeft)
	if plain != nil || legacyLeft {
		t.Fatalf("plaintext token %v or in-database key left behind (%v)", plain, legacyLeft)
	}
}

// Upgrade without a key file: the in-database key moves to the file as is.
func TestInitSecretsAdoptsLegacyKey(t *testing.T) {
	pool, st, userID, legacy := legacyDB(t)
	keyFile := filepath.Join(t.TempDir(), "secret.key")
	if err := st.InitSecrets(context.Background(), keyFile); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(keyFile)
	if err != nil || strings.TrimSpace(string(raw)) != base64.StdEncoding.EncodeToString(legacy) {
		t.Fatalf("key file should hold the legacy key: %v", err)
	}
	if info, _ := os.Stat(keyFile); info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode %v", info.Mode().Perm())
	}
	checkMigrated(t, pool, st, userID)
}

// Upgrade with an operator-provided key file: credentials are re-encrypted.
func TestInitSecretsReencryptsToProvidedKey(t *testing.T) {
	pool, st, userID, _ := legacyDB(t)
	keyFile := filepath.Join(t.TempDir(), "secret.key")
	writeKey(t, keyFile, randomKey())
	if err := st.InitSecrets(context.Background(), keyFile); err != nil {
		t.Fatal(err)
	}
	checkMigrated(t, pool, st, userID)
}

// Once credentials exist, a missing or different key file stops startup
// instead of silently making them undecryptable.
func TestInitSecretsRefusesWrongKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "secret.key")
	st := newRawTestStore(t)
	if err := st.InitSecrets(ctx, keyFile); err != nil {
		t.Fatal(err)
	}
	if err := st.InitSecrets(ctx, keyFile); err != nil {
		t.Fatalf("restart with the same key: %v", err)
	}
	other := filepath.Join(dir, "other.key")
	writeKey(t, other, randomKey())
	if err := st.InitSecrets(ctx, other); !errors.Is(err, store.ErrSecretKeyMismatch) {
		t.Fatalf("different key: %v", err)
	}
	if err := st.InitSecrets(ctx, filepath.Join(dir, "missing.key")); !errors.Is(err, store.ErrSecretKeyMismatch) {
		t.Fatalf("missing key file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.key")); !os.IsNotExist(err) {
		t.Fatal("a new key file was created although credentials exist")
	}
}

// The GitHub token is write-only over HTTP and stays out of exports.
func TestGitHubTokenWriteOnly(t *testing.T) {
	srv, session := newTestServer(t)
	settingsJSON := func() map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if code, body := doJSONStatus(t, srv, http.MethodPut, "/api/settings/github-token", `{"token":"ghp_secret123"}`, session); code != http.StatusOK {
		t.Fatalf("set token: %d %s", code, body)
	}
	got := settingsJSON()
	if got["githubConnected"] != true {
		t.Fatalf("githubConnected = %v", got["githubConnected"])
	}
	// A full settings save (the web app sends the whole object) can't touch it.
	got["githubToken"] = "ghp_overwrite"
	body, _ := json.Marshal(got)
	if code, resp := doJSONStatus(t, srv, http.MethodPut, "/api/settings", string(body), session); code != http.StatusOK {
		t.Fatalf("save settings: %d %s", code, resp)
	}
	for _, path := range []string{"/api/settings", "/api/export"} {
		if out := doJSON(t, srv, http.MethodGet, path, "", session); strings.Contains(out, "ghp_") {
			t.Fatalf("%s leaks the GitHub token", path)
		}
	}
	if export := doJSON(t, srv, http.MethodGet, "/api/export", "", session); strings.Contains(export, got["apiToken"].(string)) || strings.Contains(export, got["widgetToken"].(string)) {
		t.Fatal("export contains the API or widget token")
	}
	if code, _ := doJSONStatus(t, srv, http.MethodPut, "/api/settings/github-token", `{"token":"has space"}`, session); code != http.StatusBadRequest {
		t.Fatalf("invalid token accepted: %d", code)
	}
	if code, _ := doJSONStatus(t, srv, http.MethodDelete, "/api/settings/github-token", "", session); code != http.StatusOK {
		t.Fatalf("remove token: %d", code)
	}
	if settingsJSON()["githubConnected"] != false {
		t.Fatal("token still connected after DELETE")
	}
}
