// Package migrations owns the ordered, embedded Control Hub database schema.
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

const (
	schemaTable           = "schema_migrations"
	legacyTable           = "devmigrate_revisions"
	legacyInitialName     = "202609120001_current.sql"
	legacyInitialChecksum = "c763eaa15f9f83245bf5031a3d419e5f9b906415b9835ec105cac35f9bdcff5f"
)

//go:embed *.sql
var Source embed.FS

type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

type ApplyResult struct {
	FromVersion   int
	ToVersion     int
	Applied       []int
	AdoptedLegacy bool
}

type Status struct {
	Version  int
	Identity string
}

func Names() []string {
	set := List()
	names := make([]string, len(set))
	for i, migration := range set {
		names[i] = migration.Name
	}
	return names
}

func List() []Migration {
	set, err := load(Source)
	if err != nil {
		panic(err)
	}
	return set
}

func CurrentVersion() int { return len(List()) }

func CurrentIdentity() string {
	h := sha256.New()
	for _, migration := range List() {
		fmt.Fprintf(h, "%06d\x00%s\x00%s\n", migration.Version, migration.Name, migration.Checksum)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// CurrentSQL is retained for diagnostics. Schema initialization and upgrades
// must use Apply so version/checksum records commit with each migration.
func CurrentSQL() string {
	var builder strings.Builder
	for i, migration := range List() {
		if i != 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(migration.SQL)
	}
	return builder.String()
}

func Apply(ctx context.Context, db *sql.DB) (ApplyResult, error) {
	return applySet(ctx, db, List())
}

func Verify(ctx context.Context, db *sql.DB) (Status, error) {
	if db == nil {
		return Status{}, fmt.Errorf("database is nil")
	}
	set := List()
	exists, err := tableExists(ctx, db, schemaTable)
	if err != nil {
		return Status{}, err
	}
	if !exists {
		return Status{}, fmt.Errorf("database has no schema migration history")
	}
	version, err := verifyRecorded(ctx, db, set)
	if err != nil {
		return Status{}, err
	}
	if version < len(set) {
		return Status{}, fmt.Errorf("database schema is behind: have version %d, need %d", version, len(set))
	}
	return Status{Version: version, Identity: CurrentIdentity()}, nil
}

func load(source fs.FS) ([]Migration, error) {
	names, err := fs.Glob(source, "*.sql")
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("missing embedded migrations")
	}
	sort.Strings(names)
	set := make([]Migration, 0, len(names))
	for index, name := range names {
		underscore := strings.IndexByte(name, '_')
		if underscore < 1 || !strings.HasSuffix(name, ".sql") {
			return nil, fmt.Errorf("invalid migration filename %q", name)
		}
		version, err := strconv.Atoi(name[:underscore])
		if err != nil || version != index+1 {
			return nil, fmt.Errorf("migration versions must be contiguous from 1: %q", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		set = append(set, Migration{Version: version, Name: name, SQL: string(data), Checksum: fmt.Sprintf("%x", sum)})
	}
	return set, nil
}

func applySet(ctx context.Context, db *sql.DB, set []Migration) (ApplyResult, error) {
	if db == nil {
		return ApplyResult{}, fmt.Errorf("database is nil")
	}
	if err := validateSet(set); err != nil {
		return ApplyResult{}, err
	}
	atlas, err := tableExists(ctx, db, "atlas_schema_revisions")
	if err != nil {
		return ApplyResult{}, err
	}
	if atlas {
		return ApplyResult{}, fmt.Errorf("Atlas-managed database is not supported by the Preview migrator")
	}

	hasSchema, err := tableExists(ctx, db, schemaTable)
	if err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{}
	if !hasSchema {
		hasLegacy, err := tableExists(ctx, db, legacyTable)
		if err != nil {
			return result, err
		}
		if hasLegacy {
			if err := adoptLegacy(ctx, db, set[0]); err != nil {
				return result, err
			}
			result.AdoptedLegacy = true
			hasSchema = true
		} else {
			count, err := applicationTableCount(ctx, db)
			if err != nil {
				return result, err
			}
			if count != 0 {
				return result, fmt.Errorf("unrecognized non-empty database; migration history is required")
			}
		}
	}

	current := 0
	if hasSchema {
		current, err = verifyRecorded(ctx, db, set)
		if err != nil {
			return result, err
		}
	}
	result.FromVersion = current
	for _, migration := range set[current:] {
		if err := applyOne(ctx, db, migration); err != nil {
			return result, fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}
		result.Applied = append(result.Applied, migration.Version)
		current = migration.Version
	}
	result.ToVersion = current
	return result, nil
}

func validateSet(set []Migration) error {
	if len(set) == 0 {
		return fmt.Errorf("migration set is empty")
	}
	for index, migration := range set {
		if migration.Version != index+1 || migration.Name == "" || migration.SQL == "" || migration.Checksum == "" {
			return fmt.Errorf("invalid migration set at index %d", index)
		}
		sum := sha256.Sum256([]byte(migration.SQL))
		if migration.Checksum != fmt.Sprintf("%x", sum) {
			return fmt.Errorf("migration %s checksum does not match content", migration.Name)
		}
	}
	return nil
}

func applyOne(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(
version INTEGER PRIMARY KEY,
name TEXT NOT NULL UNIQUE,
checksum TEXT NOT NULL,
applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,name,checksum) VALUES (?,?,?)", migration.Version, migration.Name, migration.Checksum); err != nil {
		return err
	}
	return tx.Commit()
}

func adoptLegacy(ctx context.Context, db *sql.DB, initial Migration) error {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM devmigrate_revisions").Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("legacy development schema history is not current")
	}
	var name, checksum string
	if err := db.QueryRowContext(ctx, "SELECT filename,checksum FROM devmigrate_revisions").Scan(&name, &checksum); err != nil {
		return err
	}
	if name != legacyInitialName || checksum != legacyInitialChecksum {
		return fmt.Errorf("legacy development schema checksum is not current")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE schema_migrations(
version INTEGER PRIMARY KEY,
name TEXT NOT NULL UNIQUE,
checksum TEXT NOT NULL,
applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,name,checksum) VALUES (1,?,?)", initial.Name, initial.Checksum); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DROP TABLE devmigrate_revisions"); err != nil {
		return err
	}
	return tx.Commit()
}

func verifyRecorded(ctx context.Context, db *sql.DB, set []Migration) (int, error) {
	rows, err := db.QueryContext(ctx, "SELECT version,name,checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	version := 0
	for rows.Next() {
		var recordedVersion int
		var name, checksum string
		if err := rows.Scan(&recordedVersion, &name, &checksum); err != nil {
			return 0, err
		}
		if recordedVersion > len(set) {
			return 0, fmt.Errorf("database schema is newer than this binary: version %d", recordedVersion)
		}
		if recordedVersion != version+1 {
			return 0, fmt.Errorf("database migration version gap at %d", recordedVersion)
		}
		expected := set[recordedVersion-1]
		if name != expected.Name {
			return 0, fmt.Errorf("database migration name conflict at version %d", recordedVersion)
		}
		if checksum != expected.Checksum {
			return 0, fmt.Errorf("database migration checksum conflict at version %d", recordedVersion)
		}
		version = recordedVersion
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return version, nil
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

func applicationTableCount(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT IN (?,?)", schemaTable, legacyTable).Scan(&count)
	return count, err
}
