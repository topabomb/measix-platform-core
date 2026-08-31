// Command devmigrate replays checksummed Atlas SQL for local development only.
// It owns a separate dev ledger, never adopts an Atlas-managed production DB,
// never ignores SQL errors, and commits each migration with its checksum.
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
	return applyMigrations(db, files)
}

func applyMigrations(db *sql.DB, files []migrate.File) error {
	var atlasOwned int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='atlas_schema_revisions'").Scan(&atlasOwned); err != nil {
		return err
	}
	if atlasOwned != 0 {
		return errors.New("Atlas-managed database: use Atlas, not devmigrate")
	}
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS devmigrate_revisions(filename TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return err
	}
	// A legacy no-checksum ledger is not proof that SQL completed. Never adopt it
	// implicitly; preserve the database for an explicit verified history repair.
	rows, err := db.Query("SELECT filename, checksum FROM devmigrate_revisions ORDER BY filename")
	if err != nil {
		return fmt.Errorf("legacy/unreadable development history; back up and verify history before adoption: %w", err)
	}
	applied := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return err
		}
		applied[name] = checksum
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	missing := false
	for _, file := range files {
		known[file.Name()] = true
		if previous, ok := applied[file.Name()]; ok {
			if missing {
				return errors.New("non-contiguous development migration history")
			}
			if previous != fmt.Sprintf("%x", sha256.Sum256(file.Bytes())) {
				return fmt.Errorf("applied migration checksum changed: %s", file.Name())
			}
		} else {
			missing = true
		}
	}
	for name := range applied {
		if !known[name] {
			return fmt.Errorf("applied migration absent from source: %s", name)
		}
	}
	for _, file := range files {
		if _, ok := applied[file.Name()]; ok {
			continue
		}
		if err := applyOne(db, file); err != nil {
			return err
		}
	}
	return nil
}

func applyOne(db *sql.DB, file migrate.File) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(string(file.Bytes())); err != nil {
		return fmt.Errorf("apply %s (rolled back): %w", file.Name(), err)
	}
	if _, err := tx.Exec("INSERT INTO devmigrate_revisions(filename,checksum) VALUES (?,?)", file.Name(), fmt.Sprintf("%x", sha256.Sum256(file.Bytes()))); err != nil {
		return err
	}
	return tx.Commit()
}
