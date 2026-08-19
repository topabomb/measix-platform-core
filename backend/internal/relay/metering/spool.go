package metering

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/common/sqliteutil"
)

//go:embed migrations/001_spool.sql
var spoolSchema string

const (
	StateOK       = "OK"
	StateDegraded = "METERING_DEGRADED"
)

type Spool struct{ db *sql.DB }

type Row struct {
	Seq          int64
	RequestID    string
	Payload      json.RawMessage
	AttemptCount int
}

type Stats struct {
	State            string
	PendingCount     int
	OldestPendingAge time.Duration
}

func OpenSpool(path string) (*Spool, error) {
	db, err := sqliteutil.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(spoolSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize relay spool: %w", err)
	}
	return &Spool{db: db}, nil
}

func (s *Spool) Close() error { return s.db.Close() }

func (s *Spool) Append(ctx context.Context, requestID string, payload json.RawMessage, createdAt time.Time) error {
	if requestID == "" || !json.Valid(payload) || createdAt.IsZero() {
		return fmt.Errorf("invalid spool event")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO request_usage_spool(request_id,payload_json,created_at) VALUES(?,?,?)`,
		requestID, string(payload), createdAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("append spool event: %w", err)
	}
	return nil
}

func (s *Spool) Pending(ctx context.Context, limit int) ([]Row, error) {
	return s.queryRows(ctx, `SELECT seq,request_id,payload_json,attempt_count FROM request_usage_spool ORDER BY seq LIMIT ?`, limit)
}

func (s *Spool) Due(ctx context.Context, now time.Time, limit int) ([]Row, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid spool limit")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq,request_id,payload_json,attempt_count FROM request_usage_spool WHERE next_attempt_at IS NULL OR next_attempt_at<=? ORDER BY seq LIMIT ?`,
		now.UTC().Format(time.RFC3339Nano), limit,
	)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func (s *Spool) queryRows(ctx context.Context, query string, limit int) ([]Row, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid spool limit")
	}
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]Row, error) {
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var payload string
		if err := rows.Scan(&r.Seq, &r.RequestID, &payload, &r.AttemptCount); err != nil {
			return nil, err
		}
		r.Payload = json.RawMessage(payload)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Spool) Ack(ctx context.Context, requestIDs []string) error {
	if len(requestIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range requestIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM request_usage_spool WHERE request_id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Spool) MarkFailed(ctx context.Context, requestIDs []string, nextAttempt time.Time, code string) error {
	if len(requestIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range requestIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE request_usage_spool SET attempt_count=attempt_count+1,next_attempt_at=?,last_error_code=? WHERE request_id=?`,
			nextAttempt.UTC().Format(time.RFC3339Nano), code, id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Spool) Stats(ctx context.Context, now time.Time) (Stats, error) {
	var count int
	var oldest sql.NullString
	var failed int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),MIN(created_at),COALESCE(SUM(CASE WHEN last_error_code IS NOT NULL THEN 1 ELSE 0 END),0) FROM request_usage_spool`,
	).Scan(&count, &oldest, &failed); err != nil {
		return Stats{}, err
	}
	stats := Stats{State: StateOK, PendingCount: count}
	if failed > 0 {
		stats.State = StateDegraded
	}
	if oldest.Valid {
		createdAt, err := time.Parse(time.RFC3339Nano, oldest.String)
		if err != nil {
			return Stats{}, err
		}
		if age := now.UTC().Sub(createdAt); age > 0 {
			stats.OldestPendingAge = age
		}
	}
	return stats, nil
}
