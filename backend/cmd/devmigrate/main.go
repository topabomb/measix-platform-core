// Command devmigrate initializes the single current schema for local development.
// SQL and its checksum record commit atomically; Atlas-managed databases are excluded.
package main

import (
	"ariga.io/atlas/sql/migrate"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"measix/platform/internal/common/sqliteutil"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("devmigrate", flag.ContinueOnError)
	dbPath := flags.String("db", "", "local SQLite database path")
	directory := flags.String("migrations-dir", "migrations", "checksummed Atlas migration directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return errors.New("--db is required")
	}
	dir, err := migrate.NewLocalDir(*directory)
	if err != nil {
		return err
	}
	if err := migrate.Validate(dir); err != nil {
		return fmt.Errorf("invalid migration directory checksum: %w", err)
	}
	files, err := dir.Files()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no migration SQL files")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0750); err != nil {
		return err
	}
	db, err := sqliteutil.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return initializeSchema(db, files)
}

// initializeSchema initializes or verifies one current schema; it never upgrades data.
func initializeSchema(db *sql.DB, files []migrate.File) error {
	if len(files) != 1 {
		return errors.New("exactly one current initialization SQL file is required")
	}
	var atlasOwned int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='atlas_schema_revisions'").Scan(&atlasOwned); err != nil {
		return err
	}
	if atlasOwned != 0 {
		return errors.New("Atlas-managed database: use Atlas, not devmigrate")
	}
	var initialized int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='devmigrate_revisions'").Scan(&initialized); err != nil {
		return err
	}
	if initialized != 0 {
		var count int
		var name, checksum string
		if err := db.QueryRow("SELECT COUNT(*) FROM devmigrate_revisions").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("non-current database; delete the obsolete development database and initialize again")
		}
		if err := db.QueryRow("SELECT filename, checksum FROM devmigrate_revisions").Scan(&name, &checksum); err != nil {
			return fmt.Errorf("non-current initialization record; recreate development database: %w", err)
		}
		if name != files[0].Name() || checksum != fmt.Sprintf("%x", sha256.Sum256(files[0].Bytes())) {
			return errors.New("non-current schema/checksum; delete the obsolete development database and initialize again")
		}
		return nil
	}
	var tables int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("unrecognized non-empty database; recreate the obsolete development database")
	}
	return applyOne(db, files[0])
}

func applyOne(db *sql.DB, file migrate.File) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("CREATE TABLE devmigrate_revisions(filename TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return err
	}
	if _, err := tx.Exec(string(file.Bytes())); err != nil {
		return fmt.Errorf("apply %s (rolled back): %w", file.Name(), err)
	}
	if _, err := tx.Exec("INSERT INTO devmigrate_revisions(filename,checksum) VALUES (?,?)", file.Name(), fmt.Sprintf("%x", sha256.Sum256(file.Bytes()))); err != nil {
		return err
	}
	return tx.Commit()
}
