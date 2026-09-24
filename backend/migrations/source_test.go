package migrations

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"measix/platform/internal/common/sqliteutil"
)

func TestApplyInitializesAndVerifiesCurrentSchema(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	result, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if result.FromVersion != 0 || result.ToVersion != CurrentVersion() || len(result.Applied) != CurrentVersion() {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if _, err := Verify(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	second, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if second.FromVersion != CurrentVersion() || second.ToVersion != CurrentVersion() || len(second.Applied) != 0 {
		t.Fatalf("non-idempotent replay: %+v", second)
	}
}

func TestApplySetCommitsCompletedMigrationAndRollsBackFailedMigration(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	set := []Migration{
		migrationForTest(1, "000001_first.sql", "CREATE TABLE first(id INTEGER PRIMARY KEY);"),
		migrationForTest(2, "000002_broken.sql", "CREATE TABLE second(id INTEGER); INVALID SQL;"),
	}
	if _, err := applySet(context.Background(), db, set); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var first, second, recorded int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='first'").Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='second'").Scan(&second); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if first != 1 || second != 0 || recorded != 1 {
		t.Fatalf("unexpected atomicity result first=%d second=%d recorded=%d", first, second, recorded)
	}
}

func TestVerifyRejectsChecksumConflictAndFutureVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(t *testing.T, dbPath string)
		wantErr string
	}{
		{
			name: "checksum",
			mutate: func(t *testing.T, dbPath string) {
				db, err := sqliteutil.Open(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("UPDATE schema_migrations SET checksum='changed' WHERE version=1"); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "checksum",
		},
		{
			name: "future",
			mutate: func(t *testing.T, dbPath string) {
				db, err := sqliteutil.Open(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("INSERT INTO schema_migrations(version,name,checksum,applied_at) VALUES (999,'future.sql','future',CURRENT_TIMESTAMP)"); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "newer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "hub.db")
			db, err := sqliteutil.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Apply(context.Background(), db); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, dbPath)
			db, err = sqliteutil.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := Verify(context.Background(), db); err == nil || !contains(err.Error(), tc.wantErr) {
				t.Fatalf("Verify error=%v want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestApplyAdoptsMatchingLegacyDevelopmentRevision(t *testing.T) {
	db, err := sqliteutil.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	set := List()
	if len(set) != 1 {
		t.Fatalf("initial Preview migration count=%d", len(set))
	}
	if _, err := db.Exec(set[0].SQL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE devmigrate_revisions(filename TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO devmigrate_revisions(filename,checksum) VALUES (?,?)", legacyInitialName, legacyInitialChecksum); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if !result.AdoptedLegacy || result.ToVersion != 1 || len(result.Applied) != 0 {
		t.Fatalf("unexpected adoption: %+v", result)
	}
	var legacy int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='devmigrate_revisions'").Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy != 0 {
		t.Fatal("legacy revision table retained")
	}
}

func migrationForTest(version int, name, sql string) Migration {
	sum := sha256.Sum256([]byte(sql))
	return Migration{Version: version, Name: name, SQL: sql, Checksum: fmt.Sprintf("%x", sum)}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
