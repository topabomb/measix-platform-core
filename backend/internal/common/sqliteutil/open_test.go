package sqliteutil_test

import (
	"path/filepath"
	"testing"

	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
)

func TestOpenAppliesProductionPragmas(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	checks := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"synchronous": "2",
	}
	for pragma, want := range checks {
		var got string
		if err := db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatalf("query %s: %v", pragma, err)
		}
		if got != want {
			t.Fatalf("PRAGMA %s=%q, want %q", pragma, got, want)
		}
	}
	if stats := db.Stats(); stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections=%d, want 1", stats.MaxOpenConnections)
	}
}
