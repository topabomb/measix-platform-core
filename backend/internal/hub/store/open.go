package store

import (
	"database/sql"

	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
)

// Open opens hub.db using the S0 SQLite runtime policy. It does not auto-migrate.
func Open(path string) (*sql.DB, error) { return sqliteutil.Open(path) }
