package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrInvalid = errors.New("invalid entry")
var ErrRegistrationClosed = errors.New("registration is closed")

type Store struct {
	db *pgxpool.Pool
}

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	IsAdmin   bool      `json:"isAdmin"`
	CreatedAt time.Time `json:"createdAt"`
}

// entryCols is the canonical SELECT/RETURNING column list for entries.
// Times are rendered as HH:MM strings to keep the API surface simple.
const entryCols = `id::text, title, content, type, link_url, date::text,
	to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'),
	completed, color, tags, recur, remind, pinned,
	watched, link_image, link_desc, link_favicon, link_video_id,
	board_id::text, column_id::text, position, created_at,
	account_id::text, external_uid, external_href, external_etag, dirty,
	workspace_id::text, blocked_by::text, link_meta_at`

type Entry struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Content      string    `json:"content,omitempty"`
	Type         string    `json:"type"`
	LinkURL      string    `json:"linkUrl,omitempty"`
	Date         string    `json:"date"`
	StartTime    *string   `json:"startTime,omitempty"`
	EndTime      *string   `json:"endTime,omitempty"`
	Completed    bool      `json:"completed"`
	Pinned       bool      `json:"pinned"`
	Watched      bool      `json:"watched"`
	LinkImage    *string   `json:"linkImage,omitempty"`
	LinkDesc     *string   `json:"linkDesc,omitempty"`
	LinkFavicon  *string   `json:"linkFavicon,omitempty"`
	LinkVideoID  *string   `json:"linkVideoId,omitempty"`
	Color        string    `json:"color"`
	BoardID      *string   `json:"boardId,omitempty"`
	ColumnID     *string   `json:"columnId,omitempty"`
	Position     *float64  `json:"position,omitempty"`
	Tags         []string  `json:"tags"`
	Recur        string    `json:"recur"`
	Remind       *int      `json:"remind,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	OwnerID      string    `json:"-"`
	AccountID    *string   `json:"accountId,omitempty"`
	ExternalUID  *string   `json:"-"`
	ExternalHref *string   `json:"-"`
	ExternalETag *string   `json:"-"`
	Dirty        bool      `json:"-"`
	WorkspaceID  *string   `json:"workspaceId,omitempty"`
	BlockedBy    *string   `json:"blockedBy,omitempty"`
	// LinkMetaAt — set once enrichment (unfurl/oEmbed) has run; NULL = pending.
	LinkMetaAt *time.Time `json:"linkMetaAt,omitempty"`
}

func (e *Entry) scan(row interface{ Scan(...any) error }) error {
	return row.Scan(&e.ID, &e.Title, &e.Content, &e.Type, &e.LinkURL, &e.Date,
		&e.StartTime, &e.EndTime, &e.Completed, &e.Color, &e.Tags, &e.Recur, &e.Remind, &e.Pinned, &e.Watched, &e.LinkImage, &e.LinkDesc, &e.LinkFavicon, &e.LinkVideoID, &e.BoardID, &e.ColumnID, &e.Position, &e.CreatedAt,
		&e.AccountID, &e.ExternalUID, &e.ExternalHref, &e.ExternalETag, &e.Dirty, &e.WorkspaceID, &e.BlockedBy, &e.LinkMetaAt)
}

type Settings struct {
	Country      string   `json:"country"`
	ShowHolidays bool     `json:"showHolidays"`
	Theme        string   `json:"theme"`
	WeekStart    string   `json:"weekStart"`
	Accent       string   `json:"accent"`
	Timezone     string   `json:"timezone"`
	City         string   `json:"city"`
	QuotaMB      int      `json:"quotaMb"`
	DigestTime   string   `json:"digestTime"` // "HH:MM" or ""
	DefaultRate  *float64 `json:"defaultRate,omitempty"`
	GithubToken  string   `json:"githubToken"` // PAT; never logged
	WidgetToken  string   `json:"widgetToken"`
	ApiToken     string   `json:"apiToken"`
	// Modules gates feature areas; absent key = enabled.
	Modules         map[string]bool `json:"modules,omitempty"`
	ActiveWorkspace *string         `json:"activeWorkspace,omitempty"`
	// DefaultView is the calendar view fresh devices open on; a device's own
	// last-used view (localStorage) still wins once set.
	DefaultView string `json:"defaultView"`
	// NamedayCountry is the default two-letter country for nameday lookups
	// in the person editor ("" = none).
	NamedayCountry string `json:"namedayCountry"`
	// InvidiousURL is the base URL of the user's Invidious instance for
	// YouTube search ("" = feature off). Not a secret — a server address.
	InvidiousURL string `json:"invidiousUrl"`
	// PeopleShareToken exposes the public birthday/date list when set.
	PeopleShareToken string `json:"peopleShareToken"`
}

type Feed struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	URL       string     `json:"url"`
	Color     string     `json:"color"`
	Kind      string     `json:"kind"` // "calendar" (default) | "links" — items become link entries
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
}

type EntryInput struct {
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Type        string   `json:"type"`
	LinkURL     string   `json:"linkUrl"`
	Date        string   `json:"date"`
	StartTime   string   `json:"startTime"`
	EndTime     string   `json:"endTime"`
	Color       string   `json:"color"`
	Tags        []string `json:"tags"`
	Recur       string   `json:"recur"`
	Remind      *int     `json:"remind"`
	AccountID   *string  `json:"accountId"`
	BoardID     *string  `json:"boardId"`
	ColumnID    *string  `json:"columnId"`
	Position    *float64 `json:"-"`
	WorkspaceID *string  `json:"workspaceId"`
	BlockedBy   *string  `json:"blockedBy"`
}

type EntryPatch struct {
	Title      *string  `json:"title"`
	Content    *string  `json:"content"`
	Type       *string  `json:"type"`
	LinkURL    *string  `json:"linkUrl"`
	Date       *string  `json:"date"`
	StartTime  *string  `json:"startTime"`
	EndTime    *string  `json:"endTime"`
	Completed  *bool    `json:"completed"`
	Pinned     *bool    `json:"pinned"`
	Watched    *bool    `json:"watched"`
	BoardID    *string  `json:"boardId"`
	ColumnID   *string  `json:"columnId"`
	ClearBoard bool     `json:"-"`
	Color      *string  `json:"color"`
	Recur      *string  `json:"recur"`
	Remind     *int     `json:"remind"`
	Tags       []string `json:"tags"`
	HasTags    bool     `json:"-"`
	// WorkspaceID/BlockedBy: nil = leave; "" = clear; uuid = set.
	WorkspaceID *string `json:"workspaceId"`
	BlockedBy   *string `json:"blockedBy"`
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	return pgxpool.NewWithConfig(ctx, cfg)
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// registrationLock serializes sign-ups so two concurrent first registrations
// can't both become admin or slip past a just-closed policy.
const registrationLock = 7166001

// CreateUser registers an account. The first account is admin and always
// allowed; later ones need allow_registration = "true", re-checked under the
// lock so the policy can't race with a concurrent sign-up.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, registrationLock); err != nil {
		return User{}, err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return User{}, err
	}
	if count > 0 {
		var allow string
		err := tx.QueryRow(ctx, `SELECT value FROM server_config WHERE key = $1`, cfgAllowRegistration).Scan(&allow)
		if err != nil || allow != "true" {
			return User{}, ErrRegistrationClosed
		}
	}
	var u User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_admin)
		VALUES ($1, $2, $3)
		RETURNING id::text, email, is_admin, created_at
	`, NormalizeEmail(email), passwordHash, count == 0).Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO settings (user_id) VALUES ($1)`, u.ID); err != nil {
		return User{}, err
	}
	return u, tx.Commit(ctx)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, email, password_hash, is_admin, created_at FROM users WHERE email = $1
	`, NormalizeEmail(email)).Scan(&u.ID, &u.Email, &hash, &u.IsAdmin, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) UserBySession(ctx context.Context, sessionID string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT users.id::text, users.email, users.is_admin, users.created_at
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		WHERE sessions.id = $1 AND sessions.expires_at > now()
	`, sessionID).Scan(&u.ID, &u.Email, &u.IsAdmin, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateSession(ctx context.Context, userID, userAgent string) (string, error) {
	_, _ = s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, user_agent, last_seen_at)
		VALUES ($1, $2, now() + interval '30 days', nullif($3, ''), now())
	`, id, userID, userAgent)
	return id, err
}

// SessionInfo describes one live session for the security panel.
type SessionInfo struct {
	ID        string     `json:"id"`
	UserAgent *string    `json:"userAgent,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	LastSeen  *time.Time `json:"lastSeen,omitempty"`
	Current   bool       `json:"current"`
}

// ListSessions returns the user's live sessions; currentID marks "this one".
func (s *Store) ListSessions(ctx context.Context, userID, currentID string) ([]SessionInfo, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_agent, created_at, last_seen_at, id::text = $2
		FROM sessions
		WHERE user_id = $1 AND expires_at > now()
		ORDER BY last_seen_at DESC NULLS LAST
	`, userID, currentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		var si SessionInfo
		if err := rows.Scan(&si.ID, &si.UserAgent, &si.CreatedAt, &si.LastSeen, &si.Current); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

// TouchSession refreshes last_seen_at (cheap activity signal).
func (s *Store) TouchSession(ctx context.Context, sessionID string) {
	_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, sessionID)
}

// RevokeSession deletes one session owned by the user.
func (s *Store) RevokeSession(ctx context.Context, userID, sessionID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE id = $1 AND user_id = $2`, sessionID, userID)
	return err
}

// ChangePassword verifies the old password and sets a new hash.
func (s *Store) ChangePassword(ctx context.Context, userID, newHash string) error {
	_, err := s.db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, newHash, userID)
	return err
}

// RevokeOtherSessions deletes every session except the caller's.
func (s *Store) RevokeOtherSessions(ctx context.Context, userID, keepID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`, userID, keepID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	return err
}

// CreatePasswordReset stores a hashed one-time token for the user.
// The plaintext token only ever travels in the email link.
func (s *Store) CreatePasswordReset(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, _ = s.db.Exec(ctx, `DELETE FROM password_resets WHERE expires_at < now()`)
	_, err := s.db.Exec(ctx, `
		INSERT INTO password_resets (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at
	`, userID, tokenHash, expiresAt)
	return err
}

// ConsumePasswordReset deletes-and-returns a live reset token so each link
// is single-use. Returns the owning user ID or ErrNotFound.
func (s *Store) ConsumePasswordReset(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := s.db.QueryRow(ctx, `
		DELETE FROM password_resets WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id::text
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (s *Store) ListEntries(ctx context.Context, userID, from, to, q string) ([]Entry, error) {
	query := `
		SELECT ` + entryCols + `
		FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL
	`
	args := []any{userID}
	if from != "" {
		query += ` AND date >= $2`
		args = append(args, from)
	}
	if to != "" {
		query += fmt.Sprintf(" AND date <= $%d", len(args)+1)
		args = append(args, to)
	}
	if strings.TrimSpace(q) != "" {
		query += fmt.Sprintf(" AND (title ILIKE $%d OR content ILIKE $%d OR $%d = ANY(tags))",
			len(args)+1, len(args)+2, len(args)+3)
		like := "%" + strings.TrimSpace(q) + "%"
		args = append(args, like, like, strings.TrimSpace(q))
	}
	query += ` ORDER BY date ASC, created_at ASC`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) CreateEntry(ctx context.Context, userID string, input EntryInput) (Entry, error) {
	if input.Color == "" {
		input.Color = "slate"
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}
	var e Entry
	err := e.scan(s.db.QueryRow(ctx, `
		INSERT INTO entries (user_id, title, content, type, link_url, date, start_time, end_time, color, tags, recur, remind, account_id, board_id, column_id, position, dirty, workspace_id, blocked_by)
		VALUES ($1, $2, $3, $4, $5, $6, nullif($7, '')::time, nullif($8, '')::time, $9, $10, coalesce(nullif($11, ''), 'none'), $12, $13::uuid, $14::uuid, $15::uuid, $16, $13 IS NOT NULL, nullif($17, '')::uuid, nullif($18, '')::uuid)
		RETURNING `+entryCols+`
	`, userID, input.Title, input.Content, input.Type, input.LinkURL, input.Date, input.StartTime, input.EndTime, input.Color, input.Tags, input.Recur, input.Remind, input.AccountID, input.BoardID, input.ColumnID, input.Position, input.WorkspaceID, input.BlockedBy))
	return e, err
}

func (s *Store) UpdateEntry(ctx context.Context, userID, id string, patch EntryPatch) (Entry, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := s.entryTx(ctx, tx, userID, id)
	if err != nil {
		return Entry{}, err
	}
	if patch.Title != nil {
		current.Title = *patch.Title
	}
	if patch.Content != nil {
		current.Content = *patch.Content
	}
	if patch.Type != nil {
		current.Type = *patch.Type
	}
	if patch.LinkURL != nil {
		current.LinkURL = *patch.LinkURL
	}
	if patch.Date != nil {
		current.Date = *patch.Date
	}
	if patch.StartTime != nil {
		current.StartTime = patch.StartTime
	}
	if patch.EndTime != nil {
		current.EndTime = patch.EndTime
	}
	if patch.Recur != nil {
		current.Recur = *patch.Recur
	}
	if patch.Completed != nil {
		current.Completed = *patch.Completed
	}
	if patch.Pinned != nil {
		current.Pinned = *patch.Pinned
	}
	if patch.Watched != nil {
		current.Watched = *patch.Watched
	}
	if patch.ClearBoard {
		current.BoardID = nil
		current.ColumnID = nil
	} else {
		if patch.BoardID != nil {
			current.BoardID = patch.BoardID
		}
		if patch.ColumnID != nil {
			current.ColumnID = patch.ColumnID
		}
	}
	if patch.Color != nil {
		current.Color = *patch.Color
	}
	if patch.Remind != nil {
		current.Remind = patch.Remind
	}
	if patch.HasTags {
		current.Tags = patch.Tags
	}
	if patch.WorkspaceID != nil {
		current.WorkspaceID = nilIfEmpty(*patch.WorkspaceID)
	}
	if patch.BlockedBy != nil {
		current.BlockedBy = nilIfEmpty(*patch.BlockedBy)
	}

	// Blocked cards can't complete while the blocker is open.
	if patch.Completed != nil && *patch.Completed && current.BlockedBy != nil {
		if blocked, err := s.BlockerOpen(ctx, userID, id); err == nil && blocked {
			return Entry{}, ErrBlocked
		}
	}

	// Re-validate the merged row so a partial patch cannot violate the
	// time-ordering or recur-on-task rules the DB enforces.
	if strings.TrimSpace(current.Title) == "" || current.Type == "" {
		return Entry{}, ErrInvalid
	}
	if current.Type != "task" && current.Recur != "none" {
		return Entry{}, ErrInvalid
	}
	if current.StartTime == nil && current.EndTime != nil {
		return Entry{}, ErrInvalid
	}
	if current.StartTime != nil && current.EndTime != nil && *current.EndTime <= *current.StartTime {
		return Entry{}, ErrInvalid
	}

	// Snapshot the pre-update row so every change is recoverable.
	if _, err := tx.Exec(ctx, `
		INSERT INTO entry_revisions (entry_id, user_id, title, content, type, link_url, date, start_time, end_time, completed, color, tags, recur, remind)
		SELECT id, user_id, title, content, type, link_url, date, start_time, end_time, completed, color, tags, recur, remind
		FROM entries WHERE id = $1
	`, id); err != nil {
		return Entry{}, err
	}

	// Rescheduling clears the reminder so it can fire again.
	resetRemind := patch.Remind != nil || patch.StartTime != nil || patch.Date != nil

	var e Entry
	err = e.scan(tx.QueryRow(ctx, `
		UPDATE entries
		SET title = $1, content = $2, type = $3, link_url = $4, date = $5,
		    start_time = nullif($6, '')::time, end_time = nullif($7, '')::time,
		    completed = $8, color = $9, tags = $10, recur = $11, remind = $12,
		    pinned = $16, board_id = $17::uuid, column_id = $18::uuid, watched = $19,
		    workspace_id = $20::uuid, blocked_by = $21::uuid,
		    dirty = CASE WHEN account_id IS NOT NULL THEN true ELSE dirty END,
		    reminded_at = CASE WHEN $15 THEN NULL ELSE reminded_at END
		WHERE id = $13 AND user_id = $14
		RETURNING `+entryCols+`
	`, current.Title, current.Content, current.Type, current.LinkURL, current.Date,
		strOrEmpty(current.StartTime), strOrEmpty(current.EndTime),
		current.Completed, current.Color, current.Tags, current.Recur, current.Remind, id, userID, resetRemind, current.Pinned, current.BoardID, current.ColumnID, current.Watched, current.WorkspaceID, current.BlockedBy))
	if err != nil {
		return Entry{}, err
	}

	// Completing a recurring task spawns its next occurrence.
	if patch.Completed != nil && *patch.Completed && e.Type == "task" && e.Recur != "none" {
		next, ok := NextRecurDate(e.Date, e.Recur)
		if ok {
			_, err = tx.Exec(ctx, `
				INSERT INTO entries (user_id, title, content, type, link_url, date, start_time, end_time, color, tags, recur)
				VALUES ($1, $2, $3, $4, $5, $6, nullif($7, '')::time, nullif($8, '')::time, $9, $10, $11)
			`, userID, e.Title, e.Content, e.Type, e.LinkURL, next,
				strOrEmpty(e.StartTime), strOrEmpty(e.EndTime), e.Color, e.Tags, e.Recur)
			if err != nil {
				return Entry{}, err
			}
		}
	}

	return e, tx.Commit(ctx)
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// NextRecurDate returns the date of the next occurrence for a recurrence rule.
func NextRecurDate(date, recur string) (string, bool) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", false
	}
	var next time.Time
	switch recur {
	case "daily":
		next = d.AddDate(0, 0, 1)
	case "weekly":
		next = d.AddDate(0, 0, 7)
	case "monthly":
		next = addMonthsClamped(d, 1)
	case "yearly":
		next = addMonthsClamped(d, 12)
	default:
		return "", false
	}
	return next.Format(time.DateOnly), true
}

// addMonthsClamped adds months while clamping the day to the target month's
// length, so Jan 31 + 1 month lands on Feb 28/29 rather than Mar 3.
func addMonthsClamped(d time.Time, months int) time.Time {
	day := d.Day()
	next := time.Date(d.Year(), d.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := next.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(next.Year(), next.Month(), day, 0, 0, 0, 0, time.UTC)
}

func (s *Store) DeleteEntry(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `UPDATE entries SET deleted_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AllUserIDs returns every user id — used by the nightly backup.
func (s *Store) AllUserIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RestoreEntries re-inserts exported entries under the user, skipping IDs that
// already exist. Sync metadata is stripped — a restored entry is local.
func (s *Store) RestoreEntries(ctx context.Context, userID string, entries []Entry) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	imported := 0
	for _, e := range entries {
		if e.Title == "" || e.Type == "" || e.Date == "" {
			continue
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO entries (id, user_id, title, content, type, link_url, date, start_time, end_time, completed, color, tags, recur, remind)
			VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, nullif($8, '')::time, nullif($9, '')::time, $10, $11, coalesce($12::text[], '{}'::text[]), coalesce(nullif($13, ''), 'none'), $14)
			ON CONFLICT (id) DO NOTHING
		`, e.ID, userID, e.Title, e.Content, e.Type, e.LinkURL, e.Date, strOr(e.StartTime), strOr(e.EndTime), e.Completed, e.Color, e.Tags, e.Recur, e.Remind)
		if err != nil {
			return imported, err
		}
		imported += int(tag.RowsAffected())
	}
	return imported, tx.Commit(ctx)
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Revision is a point-in-time snapshot of an entry.
type Revision struct {
	ID        string    `json:"id"`
	EntryID   string    `json:"entryId"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`
	Date      string    `json:"date"`
	StartTime *string   `json:"startTime,omitempty"`
	EndTime   *string   `json:"endTime,omitempty"`
	Completed bool      `json:"completed"`
	Color     string    `json:"color"`
	Tags      []string  `json:"tags"`
	Recur     string    `json:"recur"`
	Remind    *int      `json:"remind,omitempty"`
	SavedAt   time.Time `json:"savedAt"`
}

// EntryRevisions lists snapshots for an entry, newest first (capped at 50).
func (s *Store) EntryRevisions(ctx context.Context, userID, entryID string) ([]Revision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, entry_id::text, title, coalesce(content, ''), type, date::text,
		       to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'),
		       completed, color, tags, recur, remind, saved_at
		FROM entry_revisions
		WHERE entry_id = $1 AND user_id = $2
		ORDER BY saved_at DESC
		LIMIT 50
	`, entryID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Revision{}
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.EntryID, &r.Title, &r.Content, &r.Type, &r.Date, &r.StartTime, &r.EndTime,
			&r.Completed, &r.Color, &r.Tags, &r.Recur, &r.Remind, &r.SavedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RestoreRevision copies a snapshot's fields back onto the live entry. The
// pre-restore state is snapshotted first so a restore is itself revertible.
func (s *Store) RestoreRevision(ctx context.Context, userID, entryID, revID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO entry_revisions (entry_id, user_id, title, content, type, link_url, date, start_time, end_time, completed, color, tags, recur, remind)
		SELECT id, user_id, title, content, type, link_url, date, start_time, end_time, completed, color, tags, recur, remind
		FROM entries WHERE id = $1
	`, entryID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE entries e
		SET title = r.title, content = r.content, type = r.type, link_url = r.link_url,
		    date = r.date, start_time = r.start_time, end_time = r.end_time,
		    completed = r.completed, color = r.color, tags = r.tags, recur = r.recur, remind = r.remind,
		    dirty = CASE WHEN e.account_id IS NOT NULL THEN true ELSE e.dirty END,
		    reminded_at = NULL
		FROM entry_revisions r
		WHERE e.id = r.entry_id AND r.id = $1 AND e.id = $2 AND e.user_id = $3
	`, revID, entryID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// Webhook is a user-registered outbound endpoint.
type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) Webhooks(ctx context.Context, userID string) ([]Webhook, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, url, secret, created_at FROM webhooks WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Webhook{}
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) CreateWebhook(ctx context.Context, userID, url, secret string) (Webhook, error) {
	var w Webhook
	err := s.db.QueryRow(ctx, `
		INSERT INTO webhooks (user_id, url, secret) VALUES ($1, $2, $3)
		RETURNING id::text, url, secret, created_at
	`, userID, url, secret).Scan(&w.ID, &w.URL, &w.Secret, &w.CreatedAt)
	return w, err
}

func (s *Store) DeleteWebhook(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// Entry returns one entry owned by the user.
func (s *Store) Entry(ctx context.Context, userID, id string) (Entry, error) {
	var e Entry
	err := e.scan(s.db.QueryRow(ctx, `
		SELECT `+entryCols+` FROM entries WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

func (s *Store) entryTx(ctx context.Context, tx pgx.Tx, userID, id string) (Entry, error) {
	var e Entry
	err := e.scan(tx.QueryRow(ctx, `
		SELECT `+entryCols+`
		FROM entries
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	return e, err
}

func (s *Store) Settings(ctx context.Context, userID string) (Settings, error) {
	var settings Settings
	err := s.db.QueryRow(ctx, `
		SELECT country, show_holidays, theme, week_start, accent, timezone, coalesce(city, ''), quota_mb,
		       coalesce(to_char(digest_time,'HH24:MI'), ''), widget_token, api_token, default_rate, coalesce(github_token,''),
		       modules, active_workspace::text, default_view, nameday_country, invidious_url,
		       coalesce(people_share_token, '') FROM settings WHERE user_id = $1
	`, userID).Scan(&settings.Country, &settings.ShowHolidays, &settings.Theme, &settings.WeekStart, &settings.Accent, &settings.Timezone, &settings.City, &settings.QuotaMB, &settings.DigestTime, &settings.WidgetToken, &settings.ApiToken, &settings.DefaultRate, &settings.GithubToken, &settings.Modules, &settings.ActiveWorkspace, &settings.DefaultView, &settings.NamedayCountry, &settings.InvidiousURL, &settings.PeopleShareToken)
	return settings, err
}

func (s *Store) UpdateSettings(ctx context.Context, userID string, settings Settings) (Settings, error) {
	var out Settings
	err := s.db.QueryRow(ctx, `
		INSERT INTO settings (user_id, country, show_holidays, theme, week_start, accent, timezone, city, digest_time, default_rate, github_token, modules, active_workspace, default_view, nameday_country, invidious_url)
		VALUES ($1, $2, $3, $4, $5, $6, coalesce(nullif($7, ''), 'UTC'), nullif($8, ''), nullif($9, '')::time, $10, nullif($11, ''), coalesce($12::jsonb, '{}'::jsonb), $13::uuid, coalesce(nullif($14, ''), 'month'), $15, $16)
		ON CONFLICT (user_id) DO UPDATE
		SET country = EXCLUDED.country,
		    show_holidays = EXCLUDED.show_holidays,
		    theme = EXCLUDED.theme,
		    week_start = EXCLUDED.week_start,
		    accent = EXCLUDED.accent,
		    timezone = EXCLUDED.timezone,
		    city = EXCLUDED.city,
		    digest_time = EXCLUDED.digest_time,
		    default_rate = EXCLUDED.default_rate,
		    github_token = EXCLUDED.github_token,
		    modules = EXCLUDED.modules,
		    active_workspace = EXCLUDED.active_workspace,
		    default_view = coalesce(EXCLUDED.default_view, 'month'),
		    nameday_country = EXCLUDED.nameday_country,
		    invidious_url = EXCLUDED.invidious_url
		RETURNING country, show_holidays, theme, week_start, accent, timezone, coalesce(city, ''), quota_mb,
		          coalesce(to_char(digest_time,'HH24:MI'), ''), widget_token, api_token, default_rate, coalesce(github_token,''),
		          modules, active_workspace::text, default_view, nameday_country, invidious_url
	`, userID, settings.Country, settings.ShowHolidays, settings.Theme, settings.WeekStart, settings.Accent, settings.Timezone, settings.City, settings.DigestTime, settings.DefaultRate, settings.GithubToken, settings.Modules, settings.ActiveWorkspace, settings.DefaultView, settings.NamedayCountry, settings.InvidiousURL).
		Scan(&out.Country, &out.ShowHolidays, &out.Theme, &out.WeekStart, &out.Accent, &out.Timezone, &out.City, &out.QuotaMB, &out.DigestTime, &out.WidgetToken, &out.ApiToken, &out.DefaultRate, &out.GithubToken, &out.Modules, &out.ActiveWorkspace, &out.DefaultView, &out.NamedayCountry, &out.InvidiousURL)
	return out, err
}

// RegenerateWidgetToken rotates the read-only widget share token.
func (s *Store) RegenerateWidgetToken(ctx context.Context, userID string) (string, error) {
	var token string
	err := s.db.QueryRow(ctx, `
		UPDATE settings SET widget_token = replace(gen_random_uuid()::text, '-', '')
		WHERE user_id = $1 RETURNING widget_token
	`, userID).Scan(&token)
	return token, err
}

// UserByWidgetToken resolves the owner of a widget share token (read-only use).
func (s *Store) UserByWidgetToken(ctx context.Context, token string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT users.id::text, users.email
		FROM settings JOIN users ON users.id = settings.user_id
		WHERE settings.widget_token = $1
	`, token).Scan(&u.ID, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// RegenerateApiToken rotates the MCP/API access token.
func (s *Store) RegenerateApiToken(ctx context.Context, userID string) (string, error) {
	var token string
	err := s.db.QueryRow(ctx, `
		UPDATE settings SET api_token = replace(gen_random_uuid()::text, '-', '')
		WHERE user_id = $1 RETURNING api_token
	`, userID).Scan(&token)
	return token, err
}

// UserByApiToken resolves the owner of an API token (used by the MCP endpoint).
func (s *Store) UserByApiToken(ctx context.Context, token string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT users.id::text, users.email
		FROM settings JOIN users ON users.id = settings.user_id
		WHERE settings.api_token = $1
	`, token).Scan(&u.ID, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// ---------- Push ----------

type PushSubscription struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
	Label    string `json:"label"`
}

func (s *Store) UpsertPushSub(ctx context.Context, userID, endpoint, p256dh, auth, label string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, label)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, user_id = EXCLUDED.user_id, label = EXCLUDED.label
	`, uuid.NewString(), userID, endpoint, p256dh, auth, label)
	return err
}

// DeletePushSubByID removes a subscription owned by the user.
func (s *Store) DeletePushSubByID(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) DeletePushSub(ctx context.Context, endpoint string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	return err
}

func (s *Store) PushSubs(ctx context.Context, userID string) ([]PushSubscription, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, endpoint, p256dh, auth, coalesce(label, '') FROM push_subscriptions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subs := []PushSubscription{}
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.ID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &sub.Label); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// DueReminders returns entries whose reminder moment has arrived but has not
// been pushed yet. Entry.OwnerID carries the owning user.
func (s *Store) DueReminders(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text, e.title, e.content, e.type, e.link_url, e.date::text,
		       to_char(e.start_time, 'HH24:MI'), to_char(e.end_time, 'HH24:MI'),
		       e.completed, e.color, e.tags, e.recur, e.remind, e.pinned,
		       e.watched, e.link_image, e.link_desc, e.link_favicon, e.link_video_id,
		       e.board_id::text, e.column_id::text, e.position, e.created_at,
		       e.account_id::text, e.external_uid, e.external_href, e.external_etag, e.dirty,
		       e.workspace_id::text, e.blocked_by::text, e.user_id::text
		FROM entries e JOIN settings st ON st.user_id = e.user_id
		WHERE e.deleted_at IS NULL AND e.remind IS NOT NULL AND e.reminded_at IS NULL AND e.start_time IS NOT NULL
		  AND ((e.date + e.start_time) AT TIME ZONE st.timezone - (e.remind || ' minutes')::interval) <= now()
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Title, &e.Content, &e.Type, &e.LinkURL, &e.Date,
			&e.StartTime, &e.EndTime, &e.Completed, &e.Color, &e.Tags, &e.Recur, &e.Remind, &e.Pinned, &e.Watched, &e.LinkImage, &e.LinkDesc, &e.LinkFavicon, &e.LinkVideoID, &e.BoardID, &e.ColumnID, &e.Position, &e.CreatedAt,
			&e.AccountID, &e.ExternalUID, &e.ExternalHref, &e.ExternalETag, &e.Dirty, &e.WorkspaceID, &e.BlockedBy, &e.OwnerID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) MarkReminded(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE entries SET reminded_at = now() WHERE id = $1`, id)
	return err
}

// ServerConfig reads/writes the singleton config map (VAPID keys, crypto key).
func (s *Store) ServerConfig(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRow(ctx, `SELECT value FROM server_config WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetServerConfig(ctx context.Context, key, value string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_config (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	return err
}

// ---------- CalDAV ----------

type CaldavAccount struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	Username    string     `json:"username"`
	PasswordEnc string     `json:"-"`
	Color       string     `json:"color"`
	LastSynced  *time.Time `json:"lastSynced,omitempty"`
}

func (s *Store) CreateCaldavAccount(ctx context.Context, userID, name, url, username, passwordEnc, color string) (CaldavAccount, error) {
	var a CaldavAccount
	err := s.db.QueryRow(ctx, `
		INSERT INTO caldav_accounts (id, user_id, name, url, username, password_enc, color)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, name, url, username, color
	`, uuid.NewString(), userID, name, url, username, passwordEnc, color).
		Scan(&a.ID, &a.Name, &a.URL, &a.Username, &a.Color)
	return a, err
}

func (s *Store) CaldavAccounts(ctx context.Context, userID string) ([]CaldavAccount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, url, username, color, last_synced FROM caldav_accounts WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CaldavAccount{}
	for rows.Next() {
		var a CaldavAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.URL, &a.Username, &a.Color, &a.LastSynced); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CaldavAccountWithSecret returns the account incl. the encrypted password.
func (s *Store) CaldavAccountWithSecret(ctx context.Context, id string) (CaldavAccount, string, error) {
	var a CaldavAccount
	var userID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, name, url, username, password_enc, color FROM caldav_accounts WHERE id = $1
	`, id).Scan(&a.ID, &userID, &a.Name, &a.URL, &a.Username, &a.PasswordEnc, &a.Color)
	return a, userID, err
}

// AccountOwner pairs an account with its user for the background sync loop.
type AccountOwner struct {
	Account CaldavAccount
	UserID  string
}

func (s *Store) AllCaldavAccounts(ctx context.Context) ([]AccountOwner, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, name, url, username, password_enc, color FROM caldav_accounts
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountOwner
	for rows.Next() {
		var o AccountOwner
		if err := rows.Scan(&o.Account.ID, &o.UserID, &o.Account.Name, &o.Account.URL,
			&o.Account.Username, &o.Account.PasswordEnc, &o.Account.Color); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) DeleteCaldavAccount(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM caldav_accounts WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) TouchCaldavSync(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE caldav_accounts SET last_synced = now() WHERE id = $1`, id)
	return err
}

// CarddavAccount is a connected addressbook (birthdays sync).
type CarddavAccount struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	Username    string     `json:"username"`
	PasswordEnc string     `json:"-"`
	LastSynced  *time.Time `json:"lastSynced,omitempty"`
}

func (s *Store) CreateCarddavAccount(ctx context.Context, userID, name, url, username, passwordEnc string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO carddav_accounts (id, user_id, name, url, username, password_enc)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.NewString(), userID, name, url, username, passwordEnc)
	return err
}

func (s *Store) CarddavAccounts(ctx context.Context, userID string) ([]CarddavAccount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, url, username, last_synced FROM carddav_accounts WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CarddavAccount{}
	for rows.Next() {
		var a CarddavAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.URL, &a.Username, &a.LastSynced); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CarddavAccountWithSecret(ctx context.Context, id string) (CarddavAccount, string, error) {
	var a CarddavAccount
	var userID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, name, url, username, password_enc FROM carddav_accounts WHERE id = $1
	`, id).Scan(&a.ID, &userID, &a.Name, &a.URL, &a.Username, &a.PasswordEnc)
	return a, userID, err
}

func (s *Store) DeleteCarddavAccount(ctx context.Context, userID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM carddav_accounts WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (s *Store) TouchCarddavSync(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE carddav_accounts SET last_synced = now() WHERE id = $1`, id)
	return err
}

// GoogleToken holds one user's OAuth state; the matching events live as a
// feed's cached ICS so the existing feed pipeline renders them.
type GoogleToken struct {
	UserID        string
	FeedID        string
	RefreshEnc    string
	AccessEnc     string
	AccessExpires time.Time
	CalendarID    string
}

func (s *Store) UpsertGoogleToken(ctx context.Context, userID, feedID, refreshEnc, accessEnc string, expires time.Time) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO google_tokens (user_id, feed_id, refresh_enc, access_enc, access_expires)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
		  feed_id = $2, refresh_enc = $3, access_enc = $4, access_expires = $5
	`, userID, feedID, refreshEnc, accessEnc, expires)
	return err
}

func (s *Store) GoogleToken(ctx context.Context, userID string) (GoogleToken, error) {
	var t GoogleToken
	err := s.db.QueryRow(ctx, `
		SELECT user_id::text, coalesce(feed_id::text, ''), refresh_enc, access_enc, access_expires, calendar_id
		FROM google_tokens WHERE user_id = $1
	`, userID).Scan(&t.UserID, &t.FeedID, &t.RefreshEnc, &t.AccessEnc, &t.AccessExpires, &t.CalendarID)
	return t, err
}

func (s *Store) UpdateGoogleAccess(ctx context.Context, userID, accessEnc string, expires time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE google_tokens SET access_enc = $2, access_expires = $3, last_synced = now() WHERE user_id = $1`,
		userID, accessEnc, expires)
	return err
}

func (s *Store) DeleteGoogleToken(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM google_tokens WHERE user_id = $1`, userID)
	return err
}

// AllGoogleTokens lists every connected account for the background sync loop.
func (s *Store) AllGoogleTokens(ctx context.Context) ([]GoogleToken, error) {
	rows, err := s.db.Query(ctx, `
		SELECT user_id::text, coalesce(feed_id::text, ''), refresh_enc, access_enc, access_expires, calendar_id
		FROM google_tokens
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GoogleToken
	for rows.Next() {
		var t GoogleToken
		if err := rows.Scan(&t.UserID, &t.FeedID, &t.RefreshEnc, &t.AccessEnc, &t.AccessExpires, &t.CalendarID); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AccountEntries returns all entries belonging to a CalDAV account.
func (s *Store) AccountEntries(ctx context.Context, userID, accountID string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+entryCols+` FROM entries WHERE user_id = $1 AND account_id = $2 AND deleted_at IS NULL
	`, userID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertSyncedEntry inserts or refreshes a pulled remote event. The caller
// supplies the full row; dirty stays false since the remote is authoritative.
func (s *Store) UpsertSyncedEntry(ctx context.Context, userID, accountID string, e Entry) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO entries (id, user_id, title, content, type, date, start_time, end_time, color, external_uid, external_href, external_etag, account_id)
		VALUES ($1, $2, $3, $4, 'event', $5, nullif($6, '')::time, nullif($7, '')::time, $8, $9, $10, $11, $12)
		ON CONFLICT (external_uid) WHERE external_uid IS NOT NULL DO UPDATE SET
			title = EXCLUDED.title, content = EXCLUDED.content, date = EXCLUDED.date,
			start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time,
			external_href = EXCLUDED.external_href, external_etag = EXCLUDED.external_etag,
			dirty = false
	`, e.ID, userID, e.Title, e.Content, e.Date, strOrEmpty(e.StartTime), strOrEmpty(e.EndTime), e.Color,
		e.ExternalUID, e.ExternalHref, e.ExternalETag, accountID)
	return err
}

// DeleteSyncedMissing removes synced entries whose UIDs disappeared remotely.
func (s *Store) DeleteSyncedMissing(ctx context.Context, userID, accountID string, keepUIDs []string) error {
	// Tombstone only entries that came from the server (not locally-created,
	// not-yet-pushed ones — those have an href but may just be pending).
	_, err := s.db.Exec(ctx, `
		DELETE FROM entries
		WHERE user_id = $1 AND account_id = $2 AND external_uid IS NOT NULL
		  AND external_uid != ALL($3) AND dirty = false
	`, userID, accountID, keepUIDs)
	return err
}

func (s *Store) ClearDirty(ctx context.Context, id, etag string) error {
	_, err := s.db.Exec(ctx, `UPDATE entries SET dirty = false, external_etag = coalesce(nullif($2, ''), external_etag) WHERE id = $1`, id, etag)
	return err
}

func (s *Store) SetExternalRef(ctx context.Context, id, uid, href, etag string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE entries SET external_uid = $2, external_href = $3, external_etag = $4, dirty = false WHERE id = $1
	`, id, uid, href, etag)
	return err
}

// Tombstone records a local delete that must propagate to the server.
func (s *Store) Tombstone(ctx context.Context, accountID, href string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sync_tombstones (account_id, href) VALUES ($1, $2) ON CONFLICT DO NOTHING
	`, accountID, href)
	return err
}

func (s *Store) Tombstones(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT href FROM sync_tombstones WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) ClearTombstone(ctx context.Context, accountID, href string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sync_tombstones WHERE account_id = $1 AND href = $2`, accountID, href)
	return err
}

// EntryExternalRef returns the href needed to tombstone a synced entry.
func (s *Store) EntryExternalRef(ctx context.Context, userID, id string) (accountID, href string, ok bool, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT account_id::text, external_href FROM entries WHERE id = $1 AND user_id = $2 AND external_href IS NOT NULL
	`, id, userID).Scan(&accountID, &href)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return accountID, href, err == nil, err
}

// ---------- Feeds ----------

func (s *Store) ListFeeds(ctx context.Context, userID string) ([]Feed, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, url, color, kind, fetched_at FROM feeds WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feeds := []Feed{}
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.Color, &f.Kind, &f.FetchedAt); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *Store) CreateFeed(ctx context.Context, userID, name, url, color, kind, ics string) (Feed, error) {
	var f Feed
	err := s.db.QueryRow(ctx, `
		INSERT INTO feeds (id, user_id, name, url, color, kind, ics_cache, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		RETURNING id::text, name, url, color, kind, fetched_at
	`, uuid.NewString(), userID, name, url, color, kind, ics).Scan(&f.ID, &f.Name, &f.URL, &f.Color, &f.Kind, &f.FetchedAt)
	return f, err
}

func (s *Store) DeleteFeed(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM feeds WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FeedCache returns the stored ICS body for every feed the user owns.
func (s *Store) FeedCaches(ctx context.Context, userID string) (map[Feed]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, url, color, kind, fetched_at, ics_cache FROM feeds WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[Feed]string{}
	for rows.Next() {
		var f Feed
		var cache *string
		if err := rows.Scan(&f.ID, &f.Name, &f.URL, &f.Color, &f.Kind, &f.FetchedAt, &cache); err != nil {
			return nil, err
		}
		if cache != nil {
			out[f] = *cache
		}
	}
	return out, rows.Err()
}

// AllFeeds returns every feed across users — used by the background sync loop.
func (s *Store) AllFeeds(ctx context.Context) (map[string]Feed, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, user_id::text, name, url, color, kind, fetched_at FROM feeds`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Feed{}
	var userID string
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &userID, &f.Name, &f.URL, &f.Color, &f.Kind, &f.FetchedAt); err != nil {
			return nil, err
		}
		out[userID+"/"+f.ID] = f
	}
	return out, rows.Err()
}

// RefreshFeedCache stores the latest ICS body for a feed.
func (s *Store) RefreshFeedCache(ctx context.Context, userID, feedID, ics string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE feeds SET ics_cache = $3, fetched_at = now() WHERE id = $1 AND user_id = $2
	`, feedID, userID, ics)
	return err
}

// LinkEntryExists — dedupe for channel feeds: a link with this URL was already saved.
func (s *Store) LinkEntryExists(ctx context.Context, userID, linkURL string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM entries WHERE user_id = $1 AND link_url = $2 AND deleted_at IS NULL)
	`, userID, linkURL).Scan(&ok)
	return ok, err
}

// --- Files ---

// fileCols is the canonical files column list (id last-but-two style kept stable).
const fileCols = `id::text, name, orig_name, size, mime, share_token, created_at, tags, workspace_id::text, person_id::text, sha256`

type File struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`     // random name on disk
	OrigName    string    `json:"origName"` // client filename for display
	Size        int64     `json:"size"`
	Mime        string    `json:"mime"`
	ShareToken  *string   `json:"shareToken,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Tags        []string  `json:"tags"`
	WorkspaceID *string   `json:"workspaceId,omitempty"`
	PersonID    *string   `json:"personId,omitempty"` // attached to a person profile
	Sha256      string    `json:"sha256,omitempty"`   // content hash — set for dedup on upload/restore
}

func (f *File) scan(row interface{ Scan(...any) error }) error {
	return row.Scan(&f.ID, &f.Name, &f.OrigName, &f.Size, &f.Mime, &f.ShareToken, &f.CreatedAt, &f.Tags, &f.WorkspaceID, &f.PersonID, &f.Sha256)
}

func (s *Store) CreateFile(ctx context.Context, userID, name, origName, mime string, size int64, sha256 string, tags []string, workspaceID, personID *string) (File, error) {
	var f File
	err := s.db.QueryRow(ctx, `
		INSERT INTO files (user_id, name, orig_name, size, mime, sha256, tags, workspace_id, person_id)
		VALUES ($1, $2, $3, $4, $5, nullif($6, ''), coalesce($7::text[], '{}'::text[]), $8::uuid, $9::uuid)
		RETURNING `+fileCols+`
	`, userID, name, origName, size, mime, sha256, tags, workspaceID, personID).
		Scan(&f.ID, &f.Name, &f.OrigName, &f.Size, &f.Mime, &f.ShareToken, &f.CreatedAt, &f.Tags, &f.WorkspaceID, &f.PersonID, &f.Sha256)
	return f, err
}

// FileByHash — identical binary already stored for this user (upload dedup).
func (s *Store) FileByHash(ctx context.Context, userID, sha256 string) (File, error) {
	var f File
	err := s.db.QueryRow(ctx, `
		SELECT `+fileCols+`
		FROM files WHERE user_id = $1 AND sha256 = $2 ORDER BY created_at LIMIT 1`, userID, sha256).
		Scan(&f.ID, &f.Name, &f.OrigName, &f.Size, &f.Mime, &f.ShareToken, &f.CreatedAt, &f.Tags, &f.WorkspaceID, &f.PersonID, &f.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

// AttachFileToPerson — fill person_id on a deduped upload when empty.
func (s *Store) AttachFileToPerson(ctx context.Context, userID, id, personID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE files SET person_id = $3::uuid
		WHERE id = $1 AND user_id = $2 AND person_id IS NULL`, id, userID, personID)
	return err
}

// FilesForPerson — attachments shown on a person profile, newest first.
func (s *Store) FilesForPerson(ctx context.Context, userID, personID string) ([]File, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+fileCols+`
		FROM files WHERE user_id = $1 AND person_id = $2 ORDER BY created_at DESC`, userID, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		if err := f.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) ListFiles(ctx context.Context, userID string) ([]File, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+fileCols+`
		FROM files WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		if err := f.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) FileByName(ctx context.Context, userID, name string) (File, error) {
	var f File
	err := f.scan(s.db.QueryRow(ctx, `
		SELECT `+fileCols+`
		FROM files WHERE user_id = $1 AND name = $2`, userID, name))
	return f, err
}

// FileIDExists reports whether a file id is taken (any owner) — the restore
// path uses it to skip binaries whose row insert would conflict anyway.
func (s *Store) FileIDExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM files WHERE id = $1::uuid)`, id).Scan(&ok)
	return ok, err
}

// FileByShareToken resolves a public share link — no ownership check.
func (s *Store) FileByShareToken(ctx context.Context, token string) (string, File, error) {
	var f File
	var userID string
	err := s.db.QueryRow(ctx, `
		SELECT user_id::text, `+fileCols+`
		FROM files WHERE share_token = $1`, token).
		Scan(&userID, &f.ID, &f.Name, &f.OrigName, &f.Size, &f.Mime, &f.ShareToken, &f.CreatedAt, &f.Tags, &f.WorkspaceID, &f.PersonID, &f.Sha256)
	return userID, f, err
}

func (s *Store) SetFileShare(ctx context.Context, userID, id string, token *string) error {
	tag, err := s.db.Exec(ctx, `UPDATE files SET share_token = $3 WHERE id = $1 AND user_id = $2`, id, userID, token)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteFile removes the row; the caller unlinks the disk file.
func (s *Store) DeleteFile(ctx context.Context, userID, id string) (string, error) {
	var name string
	err := s.db.QueryRow(ctx, `DELETE FROM files WHERE id = $1 AND user_id = $2 RETURNING name`, id, userID).Scan(&name)
	return name, err
}

// RestoreFiles re-inserts file metadata rows additively — same contract as
// RestoreEntries: existing ids and already-taken disk names are skipped.
// person_id/workspace_id are kept only when the target exists locally;
// share_token is kept only when still free (global unique index).
func (s *Store) RestoreFiles(ctx context.Context, userID string, files []File) (int, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	imported := 0
	for _, f := range files {
		if f.ID == "" || f.Name == "" {
			continue
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO files (id, user_id, name, orig_name, size, mime, sha256, share_token, created_at, tags, workspace_id, person_id)
			SELECT $1::uuid, $2, $3, $4, $5, $6, nullif($7, ''),
				CASE WHEN EXISTS (SELECT 1 FROM files WHERE share_token = $8) THEN NULL ELSE $8 END,
				coalesce($9, now()), coalesce($10::text[], '{}'::text[]),
				CASE WHEN EXISTS (SELECT 1 FROM workspaces WHERE id = $11::uuid AND user_id = $2) THEN $11::uuid ELSE NULL END,
				CASE WHEN EXISTS (SELECT 1 FROM people WHERE id = $12::uuid AND user_id = $2) THEN $12::uuid ELSE NULL END
			WHERE NOT EXISTS (SELECT 1 FROM files WHERE user_id = $2 AND name = $3)
			ON CONFLICT (id) DO NOTHING`,
			f.ID, userID, f.Name, f.OrigName, f.Size, f.Mime, f.Sha256, f.ShareToken,
			orNow(f.CreatedAt), f.Tags, f.WorkspaceID, f.PersonID)
		if err != nil {
			return imported, err
		}
		imported += int(tag.RowsAffected())
	}
	return imported, tx.Commit(ctx)
}

// Activity returns date → entry count over the last N days.
func (s *Store) Activity(ctx context.Context, userID string, days int) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT date::text, count(*) FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL AND date >= current_date - $2::int
		GROUP BY date`, userID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		out[d] = n
	}
	return out, rows.Err()
}

// --- Boards ---

type Board struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	TargetDate  *string   `json:"targetDate,omitempty"`
	ShareToken  *string   `json:"shareToken,omitempty"`
	Total       int       `json:"total"`
	Done        int       `json:"done"`
	Minutes     int       `json:"minutes"` // time tracked against this board
	CreatedAt   time.Time `json:"createdAt"`
}

type BoardColumn struct {
	ID       string `json:"id"`
	BoardID  string `json:"boardId"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	WipLimit *int   `json:"wipLimit,omitempty"`
}

func (s *Store) ListBoards(ctx context.Context, userID string) ([]Board, error) {
	rows, err := s.db.Query(ctx, `
		SELECT b.id::text, b.name, b.color, b.description, b.target_date::text, b.share_token, b.created_at,
		       count(e.id) FILTER (WHERE e.deleted_at IS NULL) AS total,
		       count(e.id) FILTER (WHERE e.deleted_at IS NULL AND e.completed) AS done,
		       coalesce((SELECT sum(EXTRACT(EPOCH FROM coalesce(t.end_at, now()) - t.start_at))/60
		                 FROM time_entries t WHERE t.project_id = b.id), 0)::int AS minutes
		FROM boards b LEFT JOIN entries e ON e.board_id = b.id
		WHERE b.user_id = $1 GROUP BY b.id ORDER BY b.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Board{}
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Name, &b.Color, &b.Description, &b.TargetDate, &b.ShareToken, &b.CreatedAt, &b.Total, &b.Done, &b.Minutes); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) CreateBoard(ctx context.Context, userID, name, color string) (Board, error) {
	var b Board
	err := s.db.QueryRow(ctx, `
		INSERT INTO boards (user_id, name, color) VALUES ($1, $2, $3)
		RETURNING id::text, name, color, description, target_date::text, share_token, created_at`, userID, name, color).
		Scan(&b.ID, &b.Name, &b.Color, &b.Description, &b.TargetDate, &b.ShareToken, &b.CreatedAt)
	return b, err
}

func (s *Store) DeleteBoard(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM boards WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// BoardColumns returns a board's columns ordered by position.
func (s *Store) BoardColumns(ctx context.Context, userID, boardID string) ([]BoardColumn, error) {
	rows, err := s.db.Query(ctx, `
		SELECT c.id::text, c.board_id::text, c.name, c.position, c.wip_limit
		FROM board_columns c JOIN boards b ON b.id = c.board_id
		WHERE c.board_id = $1 AND b.user_id = $2
		ORDER BY c.position`, boardID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BoardColumn{}
	for rows.Next() {
		var col BoardColumn
		if err := rows.Scan(&col.ID, &col.BoardID, &col.Name, &col.Position, &col.WipLimit); err != nil {
			return nil, err
		}
		out = append(out, col)
	}
	return out, rows.Err()
}

func (s *Store) CreateColumn(ctx context.Context, userID, boardID, name string, position int) (BoardColumn, error) {
	var col BoardColumn
	err := s.db.QueryRow(ctx, `
		INSERT INTO board_columns (board_id, name, position)
		SELECT $1, $2, $3 FROM boards WHERE id = $1 AND user_id = $4
		RETURNING id::text, board_id::text, name, position, wip_limit`, boardID, name, position, userID).
		Scan(&col.ID, &col.BoardID, &col.Name, &col.Position, &col.WipLimit)
	return col, err
}

// UpdateColumn renames and/or sets the WIP limit (0 clears).
func (s *Store) UpdateColumn(ctx context.Context, userID, id string, name *string, wip *int) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE board_columns c SET
			name = coalesce($3, c.name),
			wip_limit = CASE WHEN $4 THEN NULL ELSE coalesce($5, c.wip_limit) END
		FROM boards b
		WHERE c.id = $1 AND c.board_id = b.id AND b.user_id = $2`, id, userID, name, wip != nil && *wip == 0, wip)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) RenameColumn(ctx context.Context, userID, id, name string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE board_columns c SET name = $3 FROM boards b
		WHERE c.id = $1 AND c.board_id = b.id AND b.user_id = $2`, id, userID, name)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteColumn(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM board_columns c USING boards b
		WHERE c.id = $1 AND c.board_id = b.id AND b.user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// BoardCards returns task entries on a board, ordered within columns.
func (s *Store) BoardCards(ctx context.Context, userID, boardID string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+entryCols+` FROM entries
		WHERE user_id = $1 AND board_id = $2 AND deleted_at IS NULL
		ORDER BY column_id NULLS LAST, position NULLS LAST, created_at`, userID, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BoardExists verifies the user owns the board (for card assignment).
func (s *Store) BoardExists(ctx context.Context, userID, boardID string) bool {
	var ok bool
	_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM boards WHERE id = $1 AND user_id = $2)`, boardID, userID).Scan(&ok)
	return ok
}

// ColumnInBoard verifies the column belongs to a board the user owns.
func (s *Store) ColumnInBoard(ctx context.Context, userID, boardID, columnID string) bool {
	var ok bool
	_ = s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM board_columns c JOIN boards b ON b.id = c.board_id
			WHERE c.id = $1 AND c.board_id = $2 AND b.user_id = $3)`, columnID, boardID, userID).Scan(&ok)
	return ok
}

// MoveCard sets column + position (fractional between neighbours).
func (s *Store) MoveCard(ctx context.Context, userID, entryID, boardID string, columnID *string, position float64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE entries SET board_id = $3, column_id = $4, position = $5
		WHERE id = $1 AND user_id = $2`, entryID, userID, boardID, columnID, position)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// --- Trash ---

func (s *Store) ListTrash(ctx context.Context, userID string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+entryCols+` FROM entries
		WHERE user_id = $1 AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) RestoreEntry(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `UPDATE entries SET deleted_at = NULL WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) PurgeEntry(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM entries WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// --- Time tracking (solidtime-inspired: one running timer per user) ---

type TimeEntry struct {
	ID        string     `json:"id"`
	EntryID   *string    `json:"entryId,omitempty"`
	Title     string     `json:"title"` // joined from entries when present
	StartAt   time.Time  `json:"startAt"`
	EndAt     *time.Time `json:"endAt,omitempty"`
	Note      string     `json:"note"`
	Planned   *int       `json:"planned,omitempty"`
	Billable  bool       `json:"billable"`
	Rate      *float64   `json:"rate,omitempty"`
	ProjectID *string    `json:"projectId,omitempty"`
	Project   string     `json:"project,omitempty"` // board name, joined
	Tags      []string   `json:"tags"`
}

// StartTimer opens a running time entry; only one runs per user.
func (s *Store) StartTimer(ctx context.Context, userID string, entryID *string, note string, planned int, billable bool, rate *float64, projectID *string, tags []string) (TimeEntry, error) {
	var t TimeEntry
	err := s.db.QueryRow(ctx, `
		INSERT INTO time_entries (user_id, entry_id, note, planned_minutes, billable, hourly_rate, project_id, tags)
		VALUES ($1, $2::uuid, $3, nullif($4, 0), $5, $6, $7::uuid, coalesce($8::text[], '{}'))
		ON CONFLICT (user_id) WHERE end_at IS NULL DO NOTHING
		RETURNING id::text, entry_id::text, start_at`, userID, entryID, note, planned, billable, rate, projectID, tags).
		Scan(&t.ID, &t.EntryID, &t.StartAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, fmt.Errorf("timer already running")
	}
	return t, err
}

func (s *Store) StopTimer(ctx context.Context, userID string) (TimeEntry, error) {
	var t TimeEntry
	err := s.db.QueryRow(ctx, `
		UPDATE time_entries SET end_at = now()
		WHERE user_id = $1 AND end_at IS NULL
		RETURNING id::text, entry_id::text, start_at, end_at`, userID).
		Scan(&t.ID, &t.EntryID, &t.StartAt, &t.EndAt)
	return t, err
}

func (s *Store) CurrentTimer(ctx context.Context, userID string) (*TimeEntry, error) {
	var t TimeEntry
	err := s.db.QueryRow(ctx, `
		SELECT t.id::text, t.entry_id::text, coalesce(e.title,''), t.start_at, t.note, t.planned_minutes, t.billable, t.hourly_rate, t.project_id::text, t.tags
		FROM time_entries t LEFT JOIN entries e ON e.id = t.entry_id
		WHERE t.user_id = $1 AND t.end_at IS NULL`, userID).
		Scan(&t.ID, &t.EntryID, &t.Title, &t.StartAt, &t.Note, &t.Planned, &t.Billable, &t.Rate, &t.ProjectID, &t.Tags)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

// TimeSummary returns minutes logged per entry today + today total + week total.
func (s *Store) TimeSummary(ctx context.Context, userID string) (map[string]any, error) {
	var today, week int
	var billableAmount float64
	err := s.db.QueryRow(ctx, `
		SELECT coalesce(sum(EXTRACT(EPOCH FROM coalesce(end_at, now()) - start_at))/60,0)::int,
		       coalesce(sum(CASE WHEN billable AND hourly_rate IS NOT NULL
		            THEN EXTRACT(EPOCH FROM coalesce(end_at, now()) - start_at)/3600 * hourly_rate END),0)
		FROM time_entries WHERE user_id = $1 AND start_at::date = current_date`, userID).Scan(&today, &billableAmount)
	if err != nil {
		return nil, err
	}
	_ = s.db.QueryRow(ctx, `
		SELECT coalesce(sum(EXTRACT(EPOCH FROM coalesce(end_at, now()) - start_at))/60,0)::int
		FROM time_entries WHERE user_id = $1 AND start_at >= current_date - 6`, userID).Scan(&week)
	rows, err := s.db.Query(ctx, `
		SELECT t.entry_id::text, coalesce(e.title,''),
		       (sum(EXTRACT(EPOCH FROM coalesce(t.end_at, now()) - t.start_at))/60)::int AS mins
		FROM time_entries t LEFT JOIN entries e ON e.id = t.entry_id
		WHERE t.user_id = $1 AND t.start_at::date = current_date AND t.entry_id IS NOT NULL
		GROUP BY t.entry_id, e.title ORDER BY mins DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	per := []map[string]any{}
	for rows.Next() {
		var id, title string
		var mins int
		if err := rows.Scan(&id, &title, &mins); err != nil {
			return nil, err
		}
		per = append(per, map[string]any{"entryId": id, "title": title, "minutes": mins})
	}
	return map[string]any{"todayMinutes": today, "weekMinutes": week, "billableAmount": billableAmount, "perEntry": per}, nil
}

// MinutesByEntry — for the card "time spent" chip.
func (s *Store) MinutesByEntry(ctx context.Context, userID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT entry_id::text, (sum(EXTRACT(EPOCH FROM coalesce(end_at, now()) - start_at))/60)::int
		FROM time_entries WHERE user_id = $1 AND entry_id IS NOT NULL GROUP BY entry_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var m int
		if err := rows.Scan(&id, &m); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// --- Card activity ---

type Activity struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) LogActivity(ctx context.Context, userID, entryID, action, detail string) {
	_, _ = s.db.Exec(ctx, `
		INSERT INTO card_activity (entry_id, user_id, action, detail) VALUES ($1, $2, $3, $4)`,
		entryID, userID, action, detail)
}

func (s *Store) EntryActivity(ctx context.Context, userID, entryID string) ([]Activity, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.action, a.detail, a.created_at
		FROM card_activity a WHERE a.entry_id = $1 AND a.user_id = $2
		ORDER BY a.created_at DESC LIMIT 50`, entryID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Action, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Board meta + public share ---

func (s *Store) SetBoardMeta(ctx context.Context, userID, id string, desc *string, target *string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE boards SET description = coalesce($3, description),
		target_date = CASE WHEN $4::text = '' THEN NULL ELSE coalesce($4::date, target_date) END
		WHERE id = $1 AND user_id = $2`, id, userID, desc, target)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetBoardShare(ctx context.Context, userID, id string, on, edit bool) (string, error) {
	var token string
	if on {
		token = newToken(20)
	}
	tag, err := s.db.Exec(ctx, `UPDATE boards SET share_token = $3, share_edit = $4 WHERE id = $1 AND user_id = $2`, id, userID, nilIfEmpty(token), edit && on)
	if tag.RowsAffected() == 0 {
		return "", ErrNotFound
	}
	return token, err
}

// SharedBoardOwner resolves a share token → owner + board for write-tier ops.
func (s *Store) SharedBoardOwner(ctx context.Context, token string) (string, struct {
	ID       string
	Editable bool
}, error) {
	var out struct {
		ID       string
		Editable bool
	}
	var userID string
	err := s.db.QueryRow(ctx,
		`SELECT user_id::text, id::text, share_edit FROM boards WHERE share_token = $1`, token).
		Scan(&userID, &out.ID, &out.Editable)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", out, ErrNotFound
	}
	return userID, out, err
}

// SharedBoard resolves a public board token → name + columns + cards.
func (s *Store) SharedBoard(ctx context.Context, token string) (map[string]any, error) {
	var boardID, name, desc string
	var target *string
	var edit bool
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, description, target_date::text, share_edit FROM boards WHERE share_token = $1`, token).
		Scan(&boardID, &name, &desc, &target, &edit)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	colRows, err := s.db.Query(ctx, `SELECT id::text, name, position FROM board_columns WHERE board_id = $1 ORDER BY position`, boardID)
	if err != nil {
		return nil, err
	}
	cols := []map[string]any{}
	for colRows.Next() {
		var id, cname string
		var pos int
		if err := colRows.Scan(&id, &cname, &pos); err != nil {
			colRows.Close()
			return nil, err
		}
		cols = append(cols, map[string]any{"id": id, "name": cname, "position": pos})
	}
	colRows.Close()
	cardRows, err := s.db.Query(ctx, `
		SELECT id::text, column_id::text, title, completed, date::text, tags FROM entries
		WHERE board_id = $1 AND deleted_at IS NULL ORDER BY position NULLS LAST`, boardID)
	if err != nil {
		return nil, err
	}
	cards := []map[string]any{}
	for cardRows.Next() {
		var id string
		var colID *string
		var title, date string
		var done bool
		var tags []string
		if err := cardRows.Scan(&id, &colID, &title, &done, &date, &tags); err != nil {
			cardRows.Close()
			return nil, err
		}
		cards = append(cards, map[string]any{"id": id, "columnId": colID, "title": title, "completed": done, "date": date, "tags": tags})
	}
	cardRows.Close()
	return map[string]any{"name": name, "description": desc, "targetDate": target, "edit": edit, "columns": cols, "cards": cards}, nil
}

// TagCounts — all tags with entry counts for the /tags page.
func (s *Store) TagCounts(ctx context.Context, userID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT tag, sum(n) FROM (
			SELECT unnest(e.tags) AS tag, count(*) AS n FROM entries e
			WHERE e.user_id = $1 AND e.deleted_at IS NULL GROUP BY tag
			UNION ALL
			SELECT unnest(f.tags) AS tag, count(*) FROM files f WHERE f.user_id = $1 GROUP BY tag
			UNION ALL
			SELECT unnest(t.tags) AS tag, count(*) FROM time_entries t WHERE t.user_id = $1 GROUP BY tag
			UNION ALL
			SELECT unnest(p.tags) AS tag, count(*) FROM people p WHERE p.user_id = $1 GROUP BY tag
		) counts GROUP BY tag ORDER BY sum(n) DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var tag string
		var n int
		if err := rows.Scan(&tag, &n); err != nil {
			return nil, err
		}
		out[tag] = n
	}
	return out, rows.Err()
}

// newToken returns n random hex chars — share tokens, etc.
func newToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Time log ---

// TimeLog lists finished sessions in a range (timesheet view).
func (s *Store) TimeLog(ctx context.Context, userID, from, to string) ([]TimeEntry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.entry_id::text, coalesce(e.title, t.note), t.start_at, t.end_at, t.note, t.planned_minutes,
		       t.billable, t.hourly_rate, t.project_id::text, coalesce(b.name, ''), t.tags
		FROM time_entries t
		LEFT JOIN entries e ON e.id = t.entry_id
		LEFT JOIN boards b ON b.id = t.project_id
		WHERE t.user_id = $1 AND t.start_at::date >= $2::date AND t.start_at::date <= $3::date
		ORDER BY t.start_at DESC`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimeEntry{}
	for rows.Next() {
		var t TimeEntry
		if err := rows.Scan(&t.ID, &t.EntryID, &t.Title, &t.StartAt, &t.EndAt, &t.Note, &t.Planned, &t.Billable, &t.Rate, &t.ProjectID, &t.Project, &t.Tags); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTimeEntry(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM time_entries WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DigestDue — users whose digest_time passed today and haven't been sent.
func (s *Store) DigestDue(ctx context.Context) ([]struct {
	UserID   string
	Timezone string
	Payload  string
}, error) {
	rows, err := s.db.Query(ctx, `
		SELECT st.user_id::text, st.timezone,
			(SELECT count(*) FROM entries WHERE user_id = st.user_id AND deleted_at IS NULL
			 AND date = (now() AT TIME ZONE st.timezone)::date AND type = 'task' AND NOT completed)::text || ' open tasks, ' ||
			(SELECT count(*) FROM entries WHERE user_id = st.user_id AND deleted_at IS NULL
			 AND date = (now() AT TIME ZONE st.timezone)::date AND type = 'event')::text || ' events'
		FROM settings st
		WHERE st.digest_time IS NOT NULL
		  AND (st.digest_last IS NULL OR st.digest_last < (now() AT TIME ZONE st.timezone)::date)
		  AND (now() AT TIME ZONE st.timezone)::time >= st.digest_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		UserID   string
		Timezone string
		Payload  string
	}
	for rows.Next() {
		var r struct {
			UserID   string
			Timezone string
			Payload  string
		}
		if err := rows.Scan(&r.UserID, &r.Timezone, &r.Payload); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) MarkDigestSent(ctx context.Context, userID string) {
	_, _ = s.db.Exec(ctx, `UPDATE settings SET digest_last = (now() AT TIME ZONE timezone)::date WHERE user_id = $1`, userID)
}

// SetLinkMeta stores the unfurl/enrich results on a link entry. A fetched
// title replaces a placeholder title (empty, or the raw URL).
func (s *Store) SetLinkMeta(ctx context.Context, userID, id string, desc, image, favicon, videoID, title string) {
	_, _ = s.db.Exec(ctx, `
		UPDATE entries SET link_desc = nullif($3,''), link_image = nullif($4,''),
		  link_favicon = nullif($5,''), link_video_id = nullif($6,''),
		  link_meta_at = now(),
		  title = CASE WHEN $7 <> '' AND (title = '' OR title = link_url) THEN $7 ELSE title END
		WHERE id = $1 AND user_id = $2`, id, userID, desc, image, favicon, videoID, title)
}

// GlobalSearch — one query across entries (title+content), file names,
// link URLs. Returns capped rows for the palette.
func (s *Store) GlobalSearch(ctx context.Context, userID, q string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+entryCols+` FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL
		  AND (title ILIKE $2 OR content ILIKE $2 OR link_url ILIKE $2 OR $3 = ANY(tags))
		ORDER BY date DESC LIMIT 60`, userID, "%"+q+"%", q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SearchFiles — file-name match for the global search.
func (s *Store) SearchFiles(ctx context.Context, userID, q string) ([]File, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+fileCols+`
		FROM files WHERE user_id = $1 AND (orig_name ILIKE $2 OR $3 = ANY(tags)) ORDER BY created_at DESC LIMIT 20`,
		userID, "%"+q+"%", q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []File{}
	for rows.Next() {
		var f File
		if err := f.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GitHubLinkedCards — open task entries whose linkUrl is a github issue/PR.
func (s *Store) GitHubLinkedCards(ctx context.Context, userID string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+entryCols+` FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL AND link_url LIKE '%github.com%/%/issues/%'
		   OR user_id = $1 AND deleted_at IS NULL AND link_url LIKE '%github.com%/%/pull/%'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := e.scan(rows); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GitHubUsers — everyone with a token set (the sync loop iterates them).
func (s *Store) GitHubUsers(ctx context.Context) ([]struct{ UserID, Token string }, error) {
	rows, err := s.db.Query(ctx, `SELECT user_id::text, github_token FROM settings WHERE github_token IS NOT NULL AND github_token <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ UserID, Token string }
	for rows.Next() {
		var r struct{ UserID, Token string }
		if err := rows.Scan(&r.UserID, &r.Token); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchBoards — board-name match for the global search.
func (s *Store) SearchBoards(ctx context.Context, userID, q string) ([]Board, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, color, coalesce(description,''), target_date::text, share_token, created_at, 0, 0
		FROM boards WHERE user_id = $1 AND name ILIKE $2 ORDER BY created_at DESC LIMIT 10`,
		userID, "%"+q+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Board{}
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.Name, &b.Color, &b.Description, &b.TargetDate, &b.ShareToken, &b.CreatedAt, &b.Total, &b.Done); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
