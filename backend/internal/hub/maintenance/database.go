package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var requiredTables = []string{
	"deployments", "users", "devices", "enrollments", "sessions",
	"managed_drafts", "managed_releases", "managed_states", "upstreams", "upstream_config_revisions",
	"secrets", "secret_versions", "activations", "idempotency_records", "request_usages", "semantic_usages", "pricing_rules",
}

type CheckResult struct {
	Integrity string
	Tables    int
}

type BackupMetadata struct {
	CreatedAt time.Time `json:"createdAt"`
	Build     string    `json:"build"`
	Schema    string    `json:"schema"`
}

func Check(ctx context.Context, db *sql.DB) (CheckResult, error) {
	if db == nil {
		return CheckResult{}, fmt.Errorf("database is nil")
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return CheckResult{}, err
	}
	if integrity != "ok" {
		return CheckResult{}, fmt.Errorf("sqlite integrity_check: %s", integrity)
	}
	var foreignKeyFailures int
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return CheckResult{}, err
	}
	for rows.Next() {
		foreignKeyFailures++
	}
	if err := rows.Close(); err != nil {
		return CheckResult{}, err
	}
	if foreignKeyFailures != 0 {
		return CheckResult{}, fmt.Errorf("sqlite foreign_key_check reported %d failure(s)", foreignKeyFailures)
	}
	for _, table := range requiredTables {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return CheckResult{}, err
		}
		if count != 1 {
			return CheckResult{}, fmt.Errorf("required table %q is missing", table)
		}
	}
	return CheckResult{Integrity: integrity, Tables: len(requiredTables)}, nil
}

func Backup(ctx context.Context, db *sql.DB, outputPath, build string, now time.Time) (string, error) {
	if db == nil || strings.TrimSpace(outputPath) == "" {
		return "", fmt.Errorf("invalid backup request")
	}
	outputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(outputPath); err == nil {
		return "", fmt.Errorf("backup output already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return "", err
	}
	quoted := strings.ReplaceAll(outputPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		return "", fmt.Errorf("vacuum backup: %w", err)
	}
	backupDB, err := sql.Open("sqlite", outputPath)
	if err == nil {
		_ = backupDB.Close()
	}
	metadata := BackupMetadata{CreatedAt: now.UTC(), Build: build, Schema: "s0-initial"}
	payload, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", err
	}
	metadataPath := outputPath + ".metadata.json"
	if err := os.WriteFile(metadataPath, append(payload, '\n'), 0o600); err != nil {
		_ = os.Remove(outputPath)
		return "", err
	}
	return metadataPath, nil
}
