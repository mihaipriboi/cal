package store

import (
	"context"
	"time"
)

// Location is the zone a user's dates and times are read in: their Settings
// time zone, or UTC when it's empty or unknown. Never the server's own zone.
func Location(name string) *time.Location {
	if name == "" || name == "Local" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ValidTimezone reports whether name can be a Settings time zone: an IANA
// name both Go and Postgres know. "Local" (the server's zone in Go) isn't one.
func ValidTimezone(name string) bool {
	if name == "Local" {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// UserLocation loads the user's Settings time zone (see Location).
func (s *Store) UserLocation(ctx context.Context, userID string) *time.Location {
	var name string
	_ = s.db.QueryRow(ctx, `SELECT timezone FROM settings WHERE user_id = $1`, userID).Scan(&name)
	return Location(name)
}

// SetTimezone sets the user's Settings time zone.
func (s *Store) SetTimezone(ctx context.Context, userID, name string) error {
	_, err := s.db.Exec(ctx, `UPDATE settings SET timezone = $2 WHERE user_id = $1`, userID, name)
	return err
}

// Today is the current date in loc.
func Today(loc *time.Location) string {
	return time.Now().In(loc).Format(time.DateOnly)
}

// userZoneSQL is the user's Settings time zone in SQL, and userTodaySQL
// today's date there. Both expect the user's id as $1.
const (
	userZoneSQL  = `coalesce((SELECT nullif(timezone, '') FROM settings WHERE user_id = $1), 'UTC')`
	userTodaySQL = `(now() AT TIME ZONE ` + userZoneSQL + `)::date`
)

// userDayStartSQL is the instant the user's local day `day` (a SQL date
// expression) starts, for comparing with timestamptz columns.
func userDayStartSQL(day string) string {
	return `((` + day + `)::timestamp AT TIME ZONE ` + userZoneSQL + `)`
}
