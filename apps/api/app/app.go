// Package app composes the Cal API: store connection, background sync loops,
// and the HTTP handler. cmd/server serves it over TCP; apps/desktop mounts
// it inside a Wails window without a listener.
package app

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"cal/apps/api/internal/caldav"
	"cal/apps/api/internal/calendar"
	"cal/apps/api/internal/httpapi"
	"cal/apps/api/internal/store"
	"cal/apps/api/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// New connects to Postgres, applies pending migrations, starts the background
// loops, and returns the API handler. Call the returned close func on shutdown.
func New(ctx context.Context, databaseURL, dataDir string) (http.Handler, func(), error) {
	if err := migrate(databaseURL); err != nil {
		return nil, nil, err
	}
	db, err := store.Connect(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}
	s := store.New(db)
	if err := s.InitSecrets(ctx, secretKeyFile(dataDir)); err != nil {
		db.Close()
		return nil, nil, err
	}
	go httpapi.RefreshFeedsLoop(context.Background(), s, 30*time.Minute)
	go httpapi.PushLoop(context.Background(), s, time.Minute)
	go caldavLoop(context.Background(), s)
	go httpapi.GoogleSyncLoop(context.Background(), s, 15*time.Minute)
	go httpapi.GitHubSyncLoop(context.Background(), s, 15*time.Minute)
	go httpapi.BackupLoop(context.Background(), s, dataDir)

	router := httpapi.New(s, calendar.NewHolidayCache())
	return router, db.Close, nil
}

// secretKeyFile is where the credential encryption key lives: outside the
// database, so a DB dump alone decrypts nothing. CAL_SECRET_KEY_FILE
// overrides the default <dataDir>/secret.key (e.g. a mounted secret).
func secretKeyFile(dataDir string) string {
	if p := os.Getenv("CAL_SECRET_KEY_FILE"); p != "" {
		return p
	}
	return filepath.Join(dataDir, "secret.key")
}

// migrate applies pending migrations via the embedded FS — idempotent, so the
// Docker entrypoint's goose call stays harmless.
func migrate(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	// WithAllowMissing: fork-only migrations are numbered 10001+, so a later
	// upstream 000NN file sorts below versions already applied. Goose would
	// otherwise refuse to run it as "missing".
	return goose.Up(db, ".", goose.WithAllowMissing())
}

// caldavLoop syncs every connected account every 15 minutes.
func caldavLoop(ctx context.Context, s *store.Store) {
	syncer := caldav.NewSyncer(s)
	syncer.SyncAll(ctx)
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncer.SyncAll(ctx)
		}
	}
}
