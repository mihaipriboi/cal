package store

// Workspaces, feature modules, time-entry editing/tags, file tags,
// saved filters, entry dependencies, mail accounts, dashboard stats.
//
// Workspace model: entries/files/boards/time_entries carry a nullable
// workspace_id; NULL is the implicit "Personal" space, so no backfill
// and no NOT NULL migration pain.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrBlocked = errors.New("entry is blocked")

// ---------- Workspaces ----------

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Icon      string    `json:"icon"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) ListWorkspaces(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, color, icon, position, created_at
		FROM workspaces WHERE user_id = $1 ORDER BY position, created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Color, &w.Icon, &w.Position, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) CreateWorkspace(ctx context.Context, userID, name, color, icon string) (Workspace, error) {
	if color == "" {
		color = "green"
	}
	if icon == "" {
		icon = "folder"
	}
	var w Workspace
	err := s.db.QueryRow(ctx, `
		INSERT INTO workspaces (user_id, name, color, icon, position)
		VALUES ($1, $2, $3, $4, coalesce((SELECT max(position)+1 FROM workspaces WHERE user_id = $1), 0))
		RETURNING id::text, name, color, icon, position, created_at`,
		userID, name, color, icon).
		Scan(&w.ID, &w.Name, &w.Color, &w.Icon, &w.Position, &w.CreatedAt)
	return w, err
}

func (s *Store) UpdateWorkspace(ctx context.Context, userID, id, name, color, icon string, position *int) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE workspaces SET
			name = coalesce(nullif($3, ''), name),
			color = coalesce(nullif($4, ''), color),
			icon = coalesce(nullif($5, ''), icon),
			position = coalesce($6, position)
		WHERE id = $1 AND user_id = $2`, id, userID, name, color, icon, position)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// DeleteWorkspace removes the space; contents fall back to Personal
// (workspace_id → NULL via ON DELETE SET NULL).
func (s *Store) DeleteWorkspace(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM workspaces WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// WorkspaceOwned verifies the workspace belongs to the user.
func (s *Store) WorkspaceOwned(ctx context.Context, userID, workspaceID string) bool {
	var ok bool
	_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1 AND user_id = $2)`, workspaceID, userID).Scan(&ok)
	return ok
}

// ---------- Scoped entries ----------

// ListEntriesScoped is ListEntries plus an optional workspace filter.
// workspace "" = all; "none" = Personal only (workspace_id IS NULL).
// A date range leaves out undated tasks unless undated is set.
func (s *Store) ListEntriesScoped(ctx context.Context, userID, from, to, q, workspace string, undated bool) ([]Entry, error) {
	query := `
		SELECT ` + entryCols + `
		FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL
	`
	args := []any{userID}
	if workspace == "none" {
		query += ` AND workspace_id IS NULL`
	} else if workspace != "" {
		query += fmt.Sprintf(" AND workspace_id = $%d::uuid", len(args)+1)
		args = append(args, workspace)
	}
	var dateRange []string
	if from != "" {
		dateRange = append(dateRange, fmt.Sprintf("date >= $%d", len(args)+1))
		args = append(args, from)
	}
	if to != "" {
		dateRange = append(dateRange, fmt.Sprintf("date <= $%d", len(args)+1))
		args = append(args, to)
	}
	if len(dateRange) > 0 {
		cond := strings.Join(dateRange, " AND ")
		if undated {
			cond = "date IS NULL OR (" + cond + ")"
		}
		query += " AND (" + cond + ")"
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

// EntryBlockedBy resolves the blocker's title/completion for display.
func (s *Store) BlockerOpen(ctx context.Context, userID, entryID string) (bool, error) {
	var blocked bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM entries b
			JOIN entries e ON e.blocked_by = b.id
			WHERE e.id = $1 AND e.user_id = $2 AND b.deleted_at IS NULL AND NOT b.completed
		)`, entryID, userID).Scan(&blocked)
	return blocked, err
}

// ---------- Time entries: manual create, edit, tags ----------

// CreateTimeEntry logs a finished (or still-running) session without the
// timer — the solidtime "manual entry" path.
func (s *Store) CreateTimeEntry(ctx context.Context, userID string, entryID *string, note string, startAt, endAt *time.Time, planned int, billable bool, rate *float64, projectID *string, tags []string) (TimeEntry, error) {
	var t TimeEntry
	if tags == nil {
		tags = []string{}
	}
	err := s.db.QueryRow(ctx, `
		INSERT INTO time_entries (user_id, entry_id, note, start_at, end_at, planned_minutes, billable, hourly_rate, project_id, tags)
		VALUES ($1, $2::uuid, $3, coalesce($4, now()), $5, nullif($6, 0), $7, $8, $9::uuid, $10)
		RETURNING id::text, entry_id::text, start_at, end_at, tags`,
		userID, entryID, note, startAt, endAt, planned, billable, rate, projectID, tags).
		Scan(&t.ID, &t.EntryID, &t.StartAt, &t.EndAt, &t.Tags)
	t.Note = note
	t.Billable = billable
	t.Rate = rate
	t.ProjectID = projectID
	return t, err
}

// UpdateTimeEntry patches a logged session: times, note, tags, billing.
func (s *Store) UpdateTimeEntry(ctx context.Context, userID, id string, p TimeEntryPatch) (TimeEntry, error) {
	var t TimeEntry
	err := s.db.QueryRow(ctx, `
		UPDATE time_entries SET
			start_at = coalesce($3, start_at),
			end_at = CASE WHEN $4 THEN NULL ELSE coalesce($5, end_at) END,
			note = coalesce($6, note),
			tags = coalesce($7, tags),
			billable = coalesce($8, billable),
			hourly_rate = CASE WHEN $9 THEN NULL ELSE coalesce($10, hourly_rate) END,
			project_id = CASE WHEN $11 THEN NULL ELSE coalesce($12::uuid, project_id) END,
			entry_id = CASE WHEN $13 THEN NULL ELSE coalesce($14::uuid, entry_id) END,
			planned_minutes = CASE WHEN $15 THEN NULL ELSE coalesce($16, planned_minutes) END
		WHERE id = $1 AND user_id = $2
		RETURNING id::text, entry_id::text, start_at, end_at, note, planned_minutes, billable, hourly_rate, project_id::text, tags`,
		id, userID,
		p.StartAt, p.ClearEnd, p.EndAt,
		p.Note, p.Tags,
		p.Billable, p.ClearRate, p.Rate,
		p.ClearProject, p.ProjectID,
		p.ClearEntry, p.EntryID,
		p.ClearPlanned, p.Planned).
		Scan(&t.ID, &t.EntryID, &t.StartAt, &t.EndAt, &t.Note, &t.Planned, &t.Billable, &t.Rate, &t.ProjectID, &t.Tags)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

type TimeEntryPatch struct {
	StartAt      *time.Time `json:"startAt"`
	EndAt        *time.Time `json:"endAt"`
	ClearEnd     bool       `json:"clearEnd"`
	Note         *string    `json:"note"`
	Tags         []string   `json:"tags"`
	Billable     *bool      `json:"billable"`
	Rate         *float64   `json:"rate"`
	ClearRate    bool       `json:"clearRate"`
	ProjectID    *string    `json:"projectId"`
	ClearProject bool       `json:"clearProject"`
	EntryID      *string    `json:"entryId"`
	ClearEntry   bool       `json:"clearEntry"`
	Planned      *int       `json:"planned"`
	ClearPlanned bool       `json:"clearPlanned"`
}

// ---------- File tags + workspace ----------

func (s *Store) UpdateFile(ctx context.Context, userID, id string, tags []string, workspaceID *string) (File, error) {
	var f File
	err := s.db.QueryRow(ctx, `
		UPDATE files SET
			tags = coalesce($3, tags),
			workspace_id = CASE WHEN $4 THEN workspace_id WHEN $5::text = '' THEN NULL ELSE coalesce($5::uuid, workspace_id) END
		WHERE id = $1 AND user_id = $2
		RETURNING `+fileCols,
		id, userID, tags, workspaceID == nil, workspaceID).
		Scan(&f.ID, &f.Name, &f.OrigName, &f.Size, &f.Mime, &f.ShareToken, &f.CreatedAt, &f.Tags, &f.WorkspaceID, &f.PersonID, &f.Sha256)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

// ---------- Saved filters ----------

type SavedFilter struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Filter    map[string]any `json:"filter"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (s *Store) ListFilters(ctx context.Context, userID string) ([]SavedFilter, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, filter, created_at FROM saved_filters
		WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SavedFilter{}
	for rows.Next() {
		var f SavedFilter
		if err := rows.Scan(&f.ID, &f.Name, &f.Filter, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFilter(ctx context.Context, userID, name string, filter map[string]any) (SavedFilter, error) {
	var f SavedFilter
	err := s.db.QueryRow(ctx, `
		INSERT INTO saved_filters (user_id, name, filter) VALUES ($1, $2, $3)
		RETURNING id::text, name, filter, created_at`, userID, name, filter).
		Scan(&f.ID, &f.Name, &f.Filter, &f.CreatedAt)
	return f, err
}

func (s *Store) DeleteFilter(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM saved_filters WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---------- Mail accounts ----------

type MailAccount struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	IMAPHost  string    `json:"imapHost"`
	IMAPPort  int       `json:"imapPort"`
	SMTPHost  string    `json:"smtpHost"`
	SMTPPort  int       `json:"smtpPort"`
	Username  string    `json:"username"`
	Insecure  bool      `json:"insecure"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) ListMailAccounts(ctx context.Context, userID string) ([]MailAccount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, email, imap_host, imap_port, smtp_host, smtp_port, username, insecure_tls, created_at
		FROM mail_accounts WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MailAccount{}
	for rows.Next() {
		var a MailAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &a.Insecure, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateMailAccount(ctx context.Context, userID string, a MailAccount, password string) (MailAccount, error) {
	enc, err := s.Encrypt(ctx, password)
	if err != nil {
		return a, err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO mail_accounts (user_id, name, email, imap_host, imap_port, smtp_host, smtp_port, username, password_enc, insecure_tls)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id::text, created_at`,
		userID, a.Name, a.Email, a.IMAPHost, a.IMAPPort, a.SMTPHost, a.SMTPPort, a.Username, enc, a.Insecure).
		Scan(&a.ID, &a.CreatedAt)
	return a, err
}

func (s *Store) DeleteMailAccount(ctx context.Context, userID, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM mail_accounts WHERE id = $1 AND user_id = $2`, id, userID)
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// MailCredentials resolves host + decrypted password for an owned account.
func (s *Store) MailCredentials(ctx context.Context, userID, id string) (MailAccount, string, error) {
	var a MailAccount
	var enc string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, email, imap_host, imap_port, smtp_host, smtp_port, username, password_enc, insecure_tls
		FROM mail_accounts WHERE id = $1 AND user_id = $2`, id, userID).
		Scan(&a.ID, &a.Name, &a.Email, &a.IMAPHost, &a.IMAPPort, &a.SMTPHost, &a.SMTPPort, &a.Username, &enc, &a.Insecure)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, "", ErrNotFound
	}
	if err != nil {
		return a, "", err
	}
	password, err := s.Decrypt(ctx, enc)
	return a, password, err
}

// ---------- Dashboard ----------

// DashboardStats aggregates the Today-page widgets in one round trip:
// deadlines, completion ring, weekly activity, and a merged activity feed.
type DashboardStats struct {
	TasksTotal   int            `json:"tasksTotal"`
	TasksDone    int            `json:"tasksDone"`
	DoneThisWeek int            `json:"doneThisWeek"`
	WeekActivity map[string]int `json:"weekActivity"`
	Deadlines    []Entry        `json:"deadlines"`
	Feed         []FeedItem     `json:"feed"`
	TimeTodayMin int            `json:"timeTodayMin"`
	TimeWeekMin  int            `json:"timeWeekMin"`
	Running      *TimeEntry     `json:"running,omitempty"`
}

type FeedItem struct {
	Kind    string    `json:"kind"` // entry | file | card
	Action  string    `json:"action"`
	Title   string    `json:"title"`
	EntryID string    `json:"entryId,omitempty"`
	At      time.Time `json:"at"`
}

func (s *Store) Dashboard(ctx context.Context, userID, workspace string) (DashboardStats, error) {
	var d DashboardStats
	wsClause, wsArgs := "", []any{userID}
	if workspace == "none" {
		wsClause = " AND workspace_id IS NULL"
	} else if workspace != "" {
		wsClause = " AND workspace_id = $2::uuid"
		wsArgs = append(wsArgs, workspace)
	}

	err := s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE NOT completed), count(*) FILTER (WHERE completed)
		FROM entries WHERE user_id = $1 AND type = 'task' AND deleted_at IS NULL`+wsClause, wsArgs...).
		Scan(&d.TasksTotal, &d.TasksDone)
	if err != nil {
		return d, err
	}

	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FROM entry_revisions
		WHERE user_id = $1 AND completed AND saved_at >= `+userDayStartSQL(userTodaySQL+` - 6`), userID).Scan(&d.DoneThisWeek)

	weekRows, err := s.db.Query(ctx, `
		SELECT (created_at AT TIME ZONE `+userZoneSQL+`)::date::text, count(*) FROM entries
		WHERE user_id = $1 AND created_at >= `+userDayStartSQL(userTodaySQL+` - 6`)+wsClause+`
		GROUP BY 1`, wsArgs...)
	if err == nil {
		d.WeekActivity = map[string]int{}
		for weekRows.Next() {
			var day string
			var n int
			if weekRows.Scan(&day, &n) == nil {
				d.WeekActivity[day] = n
			}
		}
		weekRows.Close()
	}

	d.Deadlines, err = s.deadlines(ctx, userID, workspace)
	if err != nil {
		return d, err
	}

	d.Feed, err = s.activityFeed(ctx, userID)
	if err != nil {
		return d, err
	}

	sum, err := s.TimeSummary(ctx, userID)
	if err == nil {
		d.TimeTodayMin, _ = sum["todayMinutes"].(int)
		d.TimeWeekMin, _ = sum["weekMinutes"].(int)
	}
	d.Running, _ = s.CurrentTimer(ctx, userID)
	return d, nil
}

// deadlines: open tasks with the nearest dates first (today counts).
func (s *Store) deadlines(ctx context.Context, userID, workspace string) ([]Entry, error) {
	query := `
		SELECT ` + entryCols + ` FROM entries
		WHERE user_id = $1 AND type = 'task' AND NOT completed AND deleted_at IS NULL
		  AND date <= ` + userTodaySQL + ` + 14`
	args := []any{userID}
	if workspace == "none" {
		query += ` AND workspace_id IS NULL`
	} else if workspace != "" {
		query += ` AND workspace_id = $2::uuid`
		args = append(args, workspace)
	}
	query += ` ORDER BY date ASC, coalesce(start_time, '23:59'::time) ASC LIMIT 8`
	rows, err := s.db.Query(ctx, query, args...)
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

// activityFeed merges recent card activity with recent entries/files —
// newest 20 across the toolkit.
func (s *Store) activityFeed(ctx context.Context, userID string) ([]FeedItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT kind, action, title, entry_id, at FROM (
			SELECT 'card' AS kind, a.action, coalesce(e.title, a.detail) AS title,
			       a.entry_id::text, a.created_at AS at
			FROM card_activity a LEFT JOIN entries e ON e.id = a.entry_id
			WHERE a.user_id = $1
			UNION ALL
			SELECT 'entry', CASE WHEN completed THEN 'completed' ELSE 'created' END,
			       title, id::text, created_at
			FROM entries WHERE user_id = $1 AND deleted_at IS NULL AND board_id IS NULL
			UNION ALL
			SELECT 'file', 'uploaded', orig_name, '', created_at
			FROM files WHERE user_id = $1
		) feed ORDER BY at DESC LIMIT 20`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedItem{}
	for rows.Next() {
		var f FeedItem
		if err := rows.Scan(&f.Kind, &f.Action, &f.Title, &f.EntryID, &f.At); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MinutesByBoard — total logged minutes per project/board (tile chip).
func (s *Store) MinutesByBoard(ctx context.Context, userID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT project_id::text, sum(EXTRACT(EPOCH FROM coalesce(end_at, now()) - start_at))/60::int
		FROM time_entries WHERE user_id = $1 AND project_id IS NOT NULL GROUP BY project_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var mins int
		if err := rows.Scan(&id, &mins); err != nil {
			return nil, err
		}
		out[id] = mins
	}
	return out, rows.Err()
}
