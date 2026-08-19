package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/maintenance"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
)

func TestHUBDBIntegrityAndBackupRestore(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	check, err := maintenance.Check(ctx, store.DB)
	if err != nil {
		t.Fatal(err)
	}
	if check.Integrity != "ok" || check.Tables < 10 {
		t.Fatalf("unexpected check: %+v", check)
	}
	output := filepath.Join(t.TempDir(), "hub-backup.db")
	metadata, err := maintenance.Backup(ctx, store.DB, output, "test-build", time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(metadata); err != nil {
		t.Fatalf("backup metadata missing: %v", err)
	}
	restored, err := sqliteutil.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := maintenance.Check(ctx, restored); err != nil {
		t.Fatalf("backup did not restore cleanly: %v", err)
	}
}

func TestHUBDBCheckRejectsEmptyDatabase(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := maintenance.Check(context.Background(), db); err == nil {
		t.Fatal("empty database passed schema check")
	}
}
