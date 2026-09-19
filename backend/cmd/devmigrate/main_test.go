package main

import (
	"ariga.io/atlas/sql/migrate"
	"measix/platform/internal/common/sqliteutil"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingObjectDoesNotMarkCurrentSchemaInitialized(t *testing.T) {
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
	if err := initializeSchema(db, []migrate.File{migrate.NewLocalFile("001.sql", file)}); err == nil {
		t.Fatal("partial schema was silently marked applied")
	}
}

func TestCurrentSchemaInitializationAtomicityAndChecksum(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bad := migrate.NewLocalFile("001.sql", []byte("CREATE TABLE first(id INTEGER); INVALID SQL;"))
	if err := initializeSchema(db, []migrate.File{bad}); err == nil {
		t.Fatal("invalid SQL succeeded")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='first'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial schema initialization committed: %d %v", count, err)
	}
	good := migrate.NewLocalFile("001.sql", []byte("CREATE TABLE first(id INTEGER);"))
	if err := initializeSchema(db, []migrate.File{good}); err != nil {
		t.Fatal(err)
	}
	if err := initializeSchema(db, []migrate.File{good}); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if err := initializeSchema(db, []migrate.File{bad}); err == nil {
		t.Fatal("changed current schema accepted")
	}
}

func TestInitializationRejectsIncrementalHistory(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []migrate.File{
		migrate.NewLocalFile("001.sql", []byte("CREATE TABLE first(id INTEGER);")),
		migrate.NewLocalFile("002.sql", []byte("ALTER TABLE first ADD COLUMN name TEXT;")),
	}
	if err := initializeSchema(db, files); err == nil {
		t.Fatal("incremental schema history accepted")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected initialization mutated database: %d %v", count, err)
	}
}

// The device preset and the make targets invoke this command against the
// repository's own migrations directory, so the path through run() is what must
// keep working. A previous change removed atlas.sum while leaving a directory
// checksum validation inside run(); because every test called initializeSchema
// directly, the breakage only surfaced when someone tried to initialize.
func TestRunInitializesFromRepositoryMigrations(t *testing.T) {
	migrations := filepath.Join("..", "..", "migrations")
	dbPath := filepath.Join(t.TempDir(), "hub.db")
	if err := run([]string{"--db", dbPath, "--migrations-dir", migrations}); err != nil {
		t.Fatalf("run against repository migrations: %v", err)
	}
	db, err := sqliteutil.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var recorded int
	if err := db.QueryRow("SELECT COUNT(*) FROM devmigrate_revisions").Scan(&recorded); err != nil || recorded != 1 {
		t.Fatalf("initialization record missing: %d %v", recorded, err)
	}
	// Re-running the current schema stays idempotent.
	if err := run([]string{"--db", dbPath, "--migrations-dir", migrations}); err != nil {
		t.Fatalf("replay against repository migrations: %v", err)
	}
}

// A directory holding no SQL is a configuration error, not a silent no-op.
func TestRunRejectsEmptyMigrationsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := run([]string{"--db", filepath.Join(dir, "hub.db"), "--migrations-dir", dir}); err == nil {
		t.Fatal("empty migrations directory accepted")
	}
}
