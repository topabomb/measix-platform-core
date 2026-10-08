package main

import (
	"context"
	"path/filepath"
	"testing"

	"measix/platform/internal/common/sqliteutil"
	"measix/platform/migrations"
)

func TestRunAppliesEmbeddedMigrationsAndIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	if err := run([]string{"--db", dbPath}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--db", dbPath}); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	db, err := sqliteutil.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	status, err := migrations.Verify(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if status.Version != migrations.CurrentVersion() {
		t.Fatalf("schema version=%d want=%d", status.Version, migrations.CurrentVersion())
	}
}

func TestRunRejectsUnrecognizedNonEmptyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqliteutil.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE unknown(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--db", dbPath}); err == nil {
		t.Fatal("unrecognized database accepted")
	}
}

func TestRunRequiresDB(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("missing --db accepted")
	}
}
