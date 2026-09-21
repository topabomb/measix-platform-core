// Command devmigrate is a compatibility wrapper for local development.
// Production and upgrade automation should invoke `control-hub migrate`; both
// commands use the same embedded, ordered migration owner.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"measix/platform/internal/common/sqliteutil"
	"measix/platform/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("devmigrate", flag.ContinueOnError)
	dbPath := flags.String("db", "", "local SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return errors.New("--db is required")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o750); err != nil {
		return err
	}
	db, err := sqliteutil.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := migrations.Apply(context.Background(), db)
	if err != nil {
		return err
	}
	fmt.Printf("schema_from=%d schema_to=%d applied=%v adopted_legacy=%t\n", result.FromVersion, result.ToVersion, result.Applied, result.AdoptedLegacy)
	return nil
}
