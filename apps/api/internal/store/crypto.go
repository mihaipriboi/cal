package store

// AES-256-GCM at-rest encryption for stored credentials: CalDAV, CardDAV and
// mail passwords, Google tokens and the GitHub token. The key is 32 random
// bytes in a file outside the database (default <DATA_DIR>/secret.key), so a
// database dump alone decrypts nothing. Earlier versions kept the key in
// server_config; InitSecrets moves it out on the first start.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNoSecretKey = errors.New("secret key not initialised")
	// ErrSecretKeyMismatch means the key file isn't the key the stored
	// credentials were encrypted with (lost, replaced or wrong path).
	ErrSecretKeyMismatch = errors.New("secret key does not match the stored credentials")
)

// secretsLock serialises InitSecrets across server instances.
const secretsLock = 0x63616c6b // "calk"

// encryptedColumns lists every column holding Encrypt output, with its row key.
var encryptedColumns = []struct{ table, key, column string }{
	{"caldav_accounts", "id", "password_enc"},
	{"carddav_accounts", "id", "password_enc"},
	{"mail_accounts", "id", "password_enc"},
	{"google_tokens", "user_id", "refresh_enc"},
	{"google_tokens", "user_id", "access_enc"},
	{"settings", "user_id", "github_token_enc"},
}

// InitSecrets loads the key from keyFile and prepares stored credentials:
//   - a missing file is created: with the legacy server_config key if there
//     is one (adopted as is), else with a new random key — but never when
//     credentials were already encrypted with a key that is now missing;
//   - a key file that differs from the legacy key gets every encrypted
//     column re-encrypted to it;
//   - plaintext GitHub tokens are encrypted;
//   - the legacy key is deleted and the key's fingerprint is recorded, so a
//     wrong key file later stops the server instead of failing every decrypt.
func (s *Store) InitSecrets(ctx context.Context, keyFile string) error {
	key, err := readKeyFile(keyFile)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, secretsLock); err != nil {
		return err
	}
	legacy, err := configValue(ctx, tx, "crypto_key")
	if err != nil {
		return err
	}
	var legacyKey []byte
	if legacy != "" {
		if legacyKey, err = base64.StdEncoding.DecodeString(legacy); err != nil || len(legacyKey) != 32 {
			return errors.New("server_config crypto_key is not a 32-byte base64 key")
		}
	}
	stored, err := configValue(ctx, tx, "secret_key_fingerprint")
	if err != nil {
		return err
	}

	if key == nil {
		switch {
		case legacyKey != nil:
			key = legacyKey
		case stored != "":
			return fmt.Errorf("%w: key file %s is missing (fingerprint %s); restore it or set CAL_SECRET_KEY_FILE", ErrSecretKeyMismatch, keyFile, stored)
		default:
			key = make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return err
			}
		}
		if err := writeKeyFile(keyFile, key); err != nil {
			return err
		}
	}
	fp := keyFingerprint(key)
	if stored != "" && stored != fp {
		return fmt.Errorf("%w: %s has fingerprint %s, credentials were encrypted with %s", ErrSecretKeyMismatch, keyFile, fp, stored)
	}

	if legacyKey != nil && string(legacyKey) != string(key) {
		for _, c := range encryptedColumns {
			if err := reencryptColumn(ctx, tx, c.table, c.key, c.column, legacyKey, key); err != nil {
				return fmt.Errorf("re-encrypt %s.%s: %w", c.table, c.column, err)
			}
		}
	}
	if err := encryptGitHubTokens(ctx, tx, key); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM server_config WHERE key = 'crypto_key'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO server_config (key, value) VALUES ('secret_key_fingerprint', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, fp); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.key = key
	return nil
}

func readKeyFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("secret key file %s must hold 32 bytes, base64-encoded", path)
	}
	return key, nil
}

func writeKeyFile(path string, key []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secret key dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create secret key: %w", err)
	}
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// keyFingerprint identifies a key without revealing it.
func keyFingerprint(key []byte) string {
	sum := sha256.Sum256(append([]byte("cal-secret-key-v1:"), key...))
	return hex.EncodeToString(sum[:8])
}

func configValue(ctx context.Context, tx pgx.Tx, key string) (string, error) {
	var v string
	err := tx.QueryRow(ctx, `SELECT value FROM server_config WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// reencryptColumn moves one column from the old key to the new one. Values
// the old key can't open were unreadable already and are left alone.
func reencryptColumn(ctx context.Context, tx pgx.Tx, table, keyCol, col string, from, to []byte) error {
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT %s::text, %s FROM %s WHERE %s IS NOT NULL`, keyCol, col, table, col))
	if err != nil {
		return err
	}
	type row struct{ id, val string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.val); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range all {
		plain, err := decryptWith(from, r.val)
		if err != nil {
			continue
		}
		enc, err := encryptWith(to, plain)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE %s::text = $2`, table, col, keyCol), enc, r.id); err != nil {
			return err
		}
	}
	return nil
}

func encryptGitHubTokens(ctx context.Context, tx pgx.Tx, key []byte) error {
	rows, err := tx.Query(ctx, `SELECT user_id::text, github_token FROM settings WHERE github_token IS NOT NULL AND github_token <> ''`)
	if err != nil {
		return err
	}
	type row struct{ id, token string }
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.token); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range all {
		enc, err := encryptWith(key, r.token)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE settings SET github_token_enc = $1, github_token = NULL WHERE user_id = $2`, enc, r.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Encrypt(_ context.Context, plaintext string) (string, error) {
	if s.key == nil {
		return "", ErrNoSecretKey
	}
	return encryptWith(s.key, plaintext)
}

func (s *Store) Decrypt(_ context.Context, encoded string) (string, error) {
	if s.key == nil {
		return "", ErrNoSecretKey
	}
	return decryptWith(s.key, encoded)
}

func encryptWith(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func decryptWith(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
