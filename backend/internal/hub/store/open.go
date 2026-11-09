package store

import (
	"database/sql"

	"measix/platform/internal/common/sqliteutil"
)

// Open opens hub.db using the S0 SQLite runtime policy. It does not auto-migrate.
func Open(path string) (*sql.DB, error) { return sqliteutil.Open(path) }
