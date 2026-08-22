// Command devmigrate applies the published Atlas migration SQL to a local SQLite
// database for development. This is NOT a replacement for `atlas migrate apply`
// in CI/production — it exists only because the Atlas CLI binary cannot be
// installed alongside Go 1.26 toolchains locally. The migration SQL itself is
// the immutable published artifact; this program merely executes it verbatim.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "", "SQLite database path (required)")
	migrationFile := flag.String("migration", "migrations/202608190001_initial.sql", "migration SQL file")
	flag.Parse()

	if *dbPath == "" {
		log.Fatal("--db is required")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}
	sqlBytes, err := os.ReadFile(*migrationFile)
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}
	// Use plain path (not file: URI) to support Windows paths with spaces.
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
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		log.Fatalf("apply migration: %v", err)
	}
	fmt.Printf("migration applied: %s -> %s\n", filepath.Base(*migrationFile), *dbPath)
}
