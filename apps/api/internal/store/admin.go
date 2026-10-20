package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Instance access policy lives in server_config (key/value TEXT) — reused for
// the registration switch rather than a new table.
const cfgAllowRegistration = "allow_registration"

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_admin`).Scan(&n)
	return n, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, email, is_admin, created_at FROM users ORDER BY created_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) SetUserAdmin(ctx context.Context, id string, admin bool) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET is_admin = $2 WHERE id = $1`, id, admin)
	return err
}

// DeleteUser cascades sessions, settings, entries and every other per-user
// row via the foreign keys declared in the migrations.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

// AllowRegistration is closed unless an admin explicitly opened it; a missing
// key or a DB error reads as closed. The first account is admitted separately
// (CreateUser), so a fresh instance still bootstraps.
func (s *Store) AllowRegistration(ctx context.Context) bool {
	v, err := s.ServerConfig(ctx, cfgAllowRegistration)
	return err == nil && v == "true"
}

func (s *Store) SetAllowRegistration(ctx context.Context, allow bool) error {
	v := "true"
	if !allow {
		v = "false"
	}
	return s.SetServerConfig(ctx, cfgAllowRegistration, v)
}

// UserExists is used by admin handlers to distinguish "not found" from a
// failed update.
func (s *Store) UserExists(ctx context.Context, id string) bool {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	return err == nil && exists
}

func (s *Store) UserIsAdmin(ctx context.Context, id string) (bool, error) {
	var admin bool
	err := s.db.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1`, id).Scan(&admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return admin, err
}
