package httpapi

// Nightly JSON backup of every user's data to DATA_DIR/backups/, keeping the
// last 14 days. Same shape as /api/export so a backup file is a valid restore.

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"cal/apps/api/internal/store"
)

const backupKeep = 14

// BackupLoop writes one backup per user per day.
func BackupLoop(ctx context.Context, s *store.Store, dataDir string) {
	run := func() {
		if err := writeBackups(ctx, s, dataDir); err != nil {
			log.Printf("backup: %v", err)
		}
	}
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func writeBackups(ctx context.Context, s *store.Store, dataDir string) error {
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	userIDs, err := s.AllUserIDs(ctx)
	if err != nil {
		return err
	}
	day := time.Now().Format("2006-01-02")
	for _, userID := range userIDs {
		settings, _ := s.Settings(ctx, userID)
		entries, err := s.ListEntries(ctx, userID, "", "", "")
		if err != nil {
			continue
		}
		people, _ := s.ListPeople(ctx, userID)
		links, _ := s.AllPersonRelations(ctx, userID)
		timeline, _ := s.AllTimeline(ctx, userID)
		// File rows are listed for completeness; the binaries already live in
		// uploads/ and are not duplicated 14× into backups. A JSON-only restore
		// skips file rows (no binaries in the archive).
		files, _ := s.ListFiles(ctx, userID)
		payload, _ := json.MarshalIndent(map[string]any{
			"userId": userID, "settings": exportSettings(settings), "entries": entries, "exportedAt": time.Now(),
			"people": people, "personLinks": links, "personTimeline": timeline, "files": files,
		}, "", "  ")
		path := filepath.Join(dir, "cal-"+userID+"-"+day+".json")
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return err
		}
	}
	return pruneBackups(dir)
}

func pruneBackups(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "cal-*.json"))
	if err != nil || len(files) <= backupKeep {
		return err
	}
	sort.Strings(files) // names end in YYYY-MM-DD → lexical = chronological
	for _, f := range files[:len(files)-backupKeep] {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}
