package metering

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"measix/platform/internal/common/sqliteutil"
)

//go:embed migrations/001_spool.sql
var spoolSchema string

const (
	StateOK       = "OK"
	StateDegraded = "METERING_DEGRADED"

	RequestAdmitted          = "ADMITTED"
	RequestStarted           = "STARTED"
	RequestSettlementPending = "SETTLEMENT_PENDING"
)

var ErrSpoolConflict = errors.New("runtime request spool conflict")

type Spool struct{ db *sql.DB }

type Row struct {
	Seq          int64
	RequestID    string
	Payload      json.RawMessage
	AttemptCount int
}

type RecoverableRow struct {
	RequestID string
	State     string
	Admission json.RawMessage
	StartedAt *time.Time
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

func (s *Spool) SaveAdmission(ctx context.Context, requestID string, payload json.RawMessage, createdAt time.Time) error {
	if requestID == "" || !json.Valid(payload) || createdAt.IsZero() {
		return fmt.Errorf("invalid admission journal record")
	}
	stamp := createdAt.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `INSERT INTO runtime_request_spool(request_id,state,admission_json,created_at,updated_at)
		VALUES(?,?,?,?,?) ON CONFLICT(request_id) DO NOTHING`, requestID, RequestAdmitted, string(payload), stamp, stamp)
	if err != nil {
		return fmt.Errorf("save admission journal record: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var existing string
	if err := s.db.QueryRowContext(ctx, `SELECT admission_json FROM runtime_request_spool WHERE request_id=?`, requestID).Scan(&existing); err != nil {
		return err
	}
	if existing != string(payload) {
		return ErrSpoolConflict
	}
	return nil
}

func (s *Spool) MarkStarted(ctx context.Context, requestID string, startedAt time.Time) error {
	if requestID == "" || startedAt.IsZero() {
		return fmt.Errorf("invalid started journal record")
	}
	stamp := startedAt.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE runtime_request_spool SET state=?,started_at=?,updated_at=?
		WHERE request_id=? AND state=?`, RequestStarted, stamp, stamp, requestID, RequestAdmitted)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var state string
	var existing sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT state,started_at FROM runtime_request_spool WHERE request_id=?`, requestID).Scan(&state, &existing); err != nil {
		return err
	}
	if (state == RequestStarted || state == RequestSettlementPending) && existing.Valid && existing.String == stamp {
		return nil
	}
	return ErrSpoolConflict
}

func (s *Spool) AbortAdmission(ctx context.Context, requestID string) error {
	if requestID == "" {
		return fmt.Errorf("invalid admission journal cleanup")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM runtime_request_spool WHERE request_id=? AND state IN (?,?)`, requestID, RequestAdmitted, RequestStarted)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_request_spool WHERE request_id=?`, requestID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return ErrSpoolConflict
}

func (s *Spool) AppendSettlement(ctx context.Context, requestID string, payload json.RawMessage, createdAt time.Time) error {
	if requestID == "" || !json.Valid(payload) || createdAt.IsZero() {
		return fmt.Errorf("invalid settlement spool event")
	}
	stamp := createdAt.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE runtime_request_spool SET state=?,settlement_json=?,updated_at=?,attempt_count=0,next_attempt_at=NULL,last_error_code=NULL
		WHERE request_id=? AND state=?`, RequestSettlementPending, string(payload), stamp, requestID, RequestStarted)
	if err != nil {
		return fmt.Errorf("append settlement spool event: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var state string
	var existing sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT state,settlement_json FROM runtime_request_spool WHERE request_id=?`, requestID).Scan(&state, &existing); err != nil {
		return err
	}
	if state == RequestSettlementPending && existing.Valid && existing.String == string(payload) {
		return nil
	}
	return ErrSpoolConflict
}

func (s *Spool) SaveDenied(ctx context.Context, requestID string, admission, settlement json.RawMessage, occurredAt time.Time) error {
	if requestID == "" || !json.Valid(admission) || !json.Valid(settlement) || occurredAt.IsZero() {
		return fmt.Errorf("invalid denied request journal record")
	}
	stamp := occurredAt.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `INSERT INTO runtime_request_spool(request_id,state,admission_json,started_at,settlement_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(request_id) DO NOTHING`, requestID, RequestSettlementPending, string(admission), stamp, string(settlement), stamp, stamp)
	if err != nil {
		return fmt.Errorf("save denied request journal record: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 1 {
		return nil
	}
	var existingAdmission, existingSettlement string
	if err := s.db.QueryRowContext(ctx, `SELECT admission_json,settlement_json FROM runtime_request_spool WHERE request_id=? AND state=?`, requestID, RequestSettlementPending).
		Scan(&existingAdmission, &existingSettlement); err != nil {
		return err
	}
	if existingAdmission != string(admission) || existingSettlement != string(settlement) {
		return ErrSpoolConflict
	}
	return nil
}

func (s *Spool) Pending(ctx context.Context, limit int) ([]Row, error) {
	return s.queryRows(ctx, `SELECT seq,request_id,settlement_json,attempt_count FROM runtime_request_spool WHERE state='SETTLEMENT_PENDING' ORDER BY seq LIMIT ?`, limit)
}

func (s *Spool) Due(ctx context.Context, now time.Time, limit int) ([]Row, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid spool limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq,request_id,settlement_json,attempt_count FROM runtime_request_spool
		WHERE state='SETTLEMENT_PENDING' AND (next_attempt_at IS NULL OR julianday(next_attempt_at)<=julianday(?)) ORDER BY seq LIMIT ?`,
		now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	return scanRows(rows)
}

func (s *Spool) Recoverable(ctx context.Context, limit int) ([]RecoverableRow, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("invalid recovery limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT request_id,state,admission_json,started_at FROM runtime_request_spool
		WHERE state IN ('ADMITTED','STARTED') ORDER BY seq LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RecoverableRow{}
	for rows.Next() {
		var row RecoverableRow
		var admission string
		var started sql.NullString
		if err := rows.Scan(&row.RequestID, &row.State, &admission, &started); err != nil {
			return nil, err
		}
		row.Admission = json.RawMessage(admission)
		if started.Valid {
			value, err := time.Parse(time.RFC3339Nano, started.String)
			if err != nil {
				return nil, err
			}
			row.StartedAt = &value
		}
		result = append(result, row)
	}
	return result, rows.Err()
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
		var row Row
		var payload string
		if err := rows.Scan(&row.Seq, &row.RequestID, &payload, &row.AttemptCount); err != nil {
			return nil, err
		}
		row.Payload = json.RawMessage(payload)
		out = append(out, row)
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
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_request_spool WHERE request_id=? AND state='SETTLEMENT_PENDING'`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PurgeUsers removes every durable request fact owned by a deleted principal.
// The admission journal is the local attribution authority, so this does not
// depend on settlement state or on a successful previous delivery.
func (s *Spool) PurgeUsers(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	deleted := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" {
			return fmt.Errorf("invalid deleted principal")
		}
		deleted[userID] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT request_id,admission_json FROM runtime_request_spool`)
	if err != nil {
		return err
	}
	var requestIDs []string
	for rows.Next() {
		var requestID, payload string
		if err := rows.Scan(&requestID, &payload); err != nil {
			rows.Close()
			return err
		}
		var admission struct {
			UserID string `json:"userId"`
		}
		if err := json.Unmarshal([]byte(payload), &admission); err != nil || admission.UserID == "" {
			rows.Close()
			return fmt.Errorf("decode admission ownership for %s", requestID)
		}
		if _, found := deleted[admission.UserID]; found {
			requestIDs = append(requestIDs, requestID)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(requestIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, requestID := range requestIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_request_spool WHERE request_id=?`, requestID); err != nil {
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
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range requestIDs {
		if _, err := tx.ExecContext(ctx, `UPDATE runtime_request_spool SET attempt_count=attempt_count+1,next_attempt_at=?,last_error_code=?,updated_at=?
			WHERE request_id=? AND state='SETTLEMENT_PENDING'`, nextAttempt.UTC().Format(time.RFC3339Nano), code, stamp, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Spool) Stats(ctx context.Context, now time.Time) (Stats, error) {
	var count int
	var oldest sql.NullString
	var failed int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),(SELECT created_at FROM runtime_request_spool ORDER BY julianday(created_at),seq LIMIT 1),
		COALESCE(SUM(CASE WHEN last_error_code IS NOT NULL THEN 1 ELSE 0 END),0) FROM runtime_request_spool`).Scan(&count, &oldest, &failed); err != nil {
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
