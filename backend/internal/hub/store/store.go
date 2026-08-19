package store

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/topabomb/measix-platform-core/backend/ent"
	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
)

type Store struct {
	DB     *sql.DB
	Client *ent.Client
}

func OpenEnt(path string) (*Store, error) {
	db, err := sqliteutil.Open(path)
	if err != nil {
		return nil, err
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	return &Store{DB: db, Client: ent.NewClient(ent.Driver(drv))}, nil
}

func (s *Store) Close() error { return s.Client.Close() }
