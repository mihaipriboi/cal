package app

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pressly/goose/v3"
)

// testDB creates a scratch database on CAL_TEST_DATABASE_URL (a DSN ending in
// /postgres?...). Skips when unset.
func testDB(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CAL_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("CAL_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := fmt.Sprintf("cal_test_app_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if a, err := sql.Open("pgx", base); err == nil {
			_, _ = a.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
			_ = a.Close()
		}
	})
	return strings.Replace(base, "/postgres?", "/"+name+"?", 1)
}

// The real migration set applies and is idempotent.
func TestMigrateIdempotent(t *testing.T) {
	dsn := testDB(t)
	if err := migrate(dsn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := migrate(dsn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// A fork migration (10001) applied before a later upstream one (00002) must
// not block it: that is the case WithAllowMissing exists for.
func TestUpstreamMigrationAfterForkMigration(t *testing.T) {
	db, err := sql.Open("pgx", testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sqlFile := func(stmt string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("-- +goose Up\n" + stmt + "\n-- +goose Down\nSELECT 1;\n")}
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { goose.SetBaseFS(nil) })

	goose.SetBaseFS(fstest.MapFS{
		"00001_up.sql":   sqlFile("CREATE TABLE a (id int);"),
		"10001_fork.sql": sqlFile("CREATE TABLE f (id int);"),
	})
	if err := goose.Up(db, ".", goose.WithAllowMissing()); err != nil {
		t.Fatal(err)
	}

	goose.SetBaseFS(fstest.MapFS{
		"00001_up.sql":   sqlFile("CREATE TABLE a (id int);"),
		"00002_up.sql":   sqlFile("CREATE TABLE b (id int);"),
		"10001_fork.sql": sqlFile("CREATE TABLE f (id int);"),
	})
	if err := goose.Up(db, "."); err == nil {
		t.Fatal("without WithAllowMissing goose should refuse the out-of-order migration")
	}
	if err := goose.Up(db, ".", goose.WithAllowMissing()); err != nil {
		t.Fatalf("with WithAllowMissing: %v", err)
	}
	if _, err := db.Exec("SELECT * FROM b"); err != nil {
		t.Fatalf("upstream migration not applied: %v", err)
	}
}
