package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"measix/platform/internal/common/sqliteutil"
	"measix/platform/internal/hub/store"
)

func OpenStore(t *testing.T) *store.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hub.db")
	db, err := sqliteutil.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../migrations"))
	migrationFiles := []string{
		filepath.Join(migrationsDir, "202608190001_initial.sql"),
		filepath.Join(migrationsDir, "202608280001_enterprise_updates.sql"),
	}
	for _, migrationPath := range migrationFiles {
		sqlText, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(sqlText)); err != nil {
			t.Fatalf("apply test migration %s: %v", migrationPath, err)
		}
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
