package testutil

import (
	"path/filepath"
	"testing"

	"measix/platform/internal/common/sqliteutil"
	"measix/platform/internal/hub/store"
	"measix/platform/migrations"
)

func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqliteutil.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations.SQLAfter("")); err != nil {
		t.Fatalf("apply real test migrations: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := store.OpenEnt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
