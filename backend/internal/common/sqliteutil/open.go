package sqliteutil

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const BusyTimeoutMillis = 5000

// Open opens a production-style MEASIX SQLite database. Schema migration is
// intentionally external: production applies versioned Atlas migrations before
// the owning process starts.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_time_format=sqlite&_timezone=UTC")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		fmt.Sprintf("PRAGMA busy_timeout=%d", BusyTimeoutMillis),
		"PRAGMA synchronous=FULL",
	} {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite (%s): %w", statement, err)
		}
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}
