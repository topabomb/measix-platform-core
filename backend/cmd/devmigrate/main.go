// Command devmigrate applies the published Atlas migration SQL to a local SQLite
// database for development. This is NOT a replacement for `atlas migrate apply`
// in CI/production — it exists only because the Atlas CLI binary cannot be
// installed alongside Go 1.26 toolchains locally. The migration SQL itself is
// the immutable published artifact; this program merely executes it verbatim.
//
// By default devmigrate discovers and applies ALL .sql files in the migrations
// directory (sorted by filename), skipping already-applied migrations tracked
// in the atlas_schema_revisions table. Use --migration to apply a single file
// without the skip logic.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "SQLite database path (required)")
	migrationFile := flag.String("migration", "", "single migration SQL file (optional; defaults to all files in migrations/)")
	migrationsDir := flag.String("migrations-dir", "migrations", "directory containing migration SQL files (applied in filename order)")
	flag.Parse()

	if *dbPath == "" {
		log.Fatal("--db is required")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}

	// Collect migration files to apply.
	var files []string
	singleFile := *migrationFile != ""
	if singleFile {
		files = []string{*migrationFile}
	} else {
		entries, err := os.ReadDir(*migrationsDir)
		if err != nil {
			log.Fatalf("read migrations dir %s: %v", *migrationsDir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".sql") {
				continue
			}
			files = append(files, filepath.Join(*migrationsDir, name))
		}
		sort.Strings(files)
	}

	if len(files) == 0 {
		log.Fatalf("no migration SQL files found in %s", *migrationsDir)
	}

	// Use plain path (not file:URI) to support Windows paths with spaces.
	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			log.Fatalf("configure sqlite (%s): %v", pragma, err)
		}
	}

	// In multi-file mode, track applied migrations to skip on re-run.
	applied := make(map[string]bool)
	if !singleFile {
		ensureRevisionsTable(db)
		applied = loadApplied(db)
	}

	for _, mf := range files {
		base := filepath.Base(mf)
		if applied[base] {
			fmt.Printf("migration already applied, skipping: %s\n", base)
			continue
		}
		sqlBytes, err := os.ReadFile(mf)
		if err != nil {
			log.Fatalf("read migration %s: %v", mf, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			// If the error is "already exists" for a previously-applied
			// migration, treat it as a skip rather than a hard failure.
			if isAlreadyExistsErr(err) {
				fmt.Printf("migration already applied (objects exist), skipping: %s\n", base)
				if !singleFile {
					recordApplied(db, base)
				}
				continue
			}
			log.Fatalf("apply migration %s: %v", base, err)
		}
		if !singleFile {
			recordApplied(db, base)
		}
		fmt.Printf("migration applied: %s -> %s\n", base, *dbPath)
	}
}

// ensureRevisionsTable creates the tracking table used by devmigrate.
// This is a local convention — it is NOT the same as atlas_schema_revisions
// (which uses a different schema). It is intentionally minimal.
func ensureRevisionsTable(db *sql.DB) {
	const create = `CREATE TABLE IF NOT EXISTS devmigrate_revisions (filename TEXT PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`
	if _, err := db.Exec(create); err != nil {
		log.Fatalf("create devmigrate_revisions table: %v", err)
	}
}

func loadApplied(db *sql.DB) map[string]bool {
	rows, err := db.Query(`SELECT filename FROM devmigrate_revisions`)
	if err != nil {
		log.Fatalf("query applied migrations: %v", err)
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Fatalf("scan migration filename: %v", err)
		}
		result[name] = true
	}
	return result
}

func recordApplied(db *sql.DB, filename string) {
	if _, err := db.Exec(`INSERT OR IGNORE INTO devmigrate_revisions (filename) VALUES (?)`, filename); err != nil {
		log.Fatalf("record applied migration %s: %v", filename, err)
	}
}

// isAlreadyExistsErr returns true for SQLite "table X already exists" errors.
func isAlreadyExistsErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "already exists")
}
