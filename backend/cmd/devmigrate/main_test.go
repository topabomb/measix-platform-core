package main

import (
	"ariga.io/atlas/sql/migrate"
	"measix/platform/internal/common/sqliteutil"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingObjectDoesNotMarkMigrationApplied(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hub.db")
	db, err := sqliteutil.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE first(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.WriteFile(filepath.Join(dir, "001.sql"), []byte("CREATE TABLE first(id INTEGER); CREATE TABLE second(id INTEGER);"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(filepath.Join(dir, "001.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err = sqliteutil.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := applyMigrations(db, []migrate.File{migrate.NewLocalFile("001.sql", file)}); err == nil {
		t.Fatal("partial schema was silently marked applied")
	}
}

func TestDevelopmentMigrationAtomicityAndChecksum(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bad := migrate.NewLocalFile("001.sql", []byte("CREATE TABLE first(id INTEGER); INVALID SQL;"))
	if err := applyMigrations(db, []migrate.File{bad}); err == nil {
		t.Fatal("invalid SQL succeeded")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='first'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial migration committed: %d %v", count, err)
	}
	good := migrate.NewLocalFile("001.sql", []byte("CREATE TABLE first(id INTEGER);"))
	if err := applyMigrations(db, []migrate.File{good}); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(db, []migrate.File{good}); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if err := applyMigrations(db, []migrate.File{bad}); err == nil {
		t.Fatal("changed applied migration accepted")
	}
}
