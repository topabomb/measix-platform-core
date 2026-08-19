package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/store"
)

func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqliteutil.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../migrations/202608190001_initial.sql"))
	sqlText, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlText)); err != nil {
		t.Fatalf("apply test migration: %v", err)
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
