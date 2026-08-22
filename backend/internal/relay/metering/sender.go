package metering

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"measix/platform/internal/wire/usageingestapi"
)

type Sender struct {
	Spool     *Spool
	HubURL    string
	Token     string
	Client    *http.Client
	BatchSize int
	Now       func() time.Time
	Jitter    func(time.Duration) time.Duration
}

func NewSender(spool *Spool, hubURL, token string) *Sender {
	return &Sender{
		Spool: spool, HubURL: strings.TrimSpace(hubURL), Token: token,
		Client: &http.Client{Timeout: 15 * time.Second}, BatchSize: 100, Now: time.Now,
		Jitter: func(base time.Duration) time.Duration {
			if base <= 0 {
				return 0
			}
			max := base / 4
			if max < time.Millisecond {
				max = time.Millisecond
			}
			return time.Duration(rand.Int64N(int64(max)))
		},
	}
}

func (s *Sender) FlushOnce(ctx context.Context) error {
	if s.Spool == nil || s.HubURL == "" || s.Token == "" || s.Client == nil || s.BatchSize < 1 || s.BatchSize > 200 || s.Now == nil || s.Jitter == nil {
		return errors.New("invalid usage sender configuration")
	}
	rows, err := s.Spool.Due(ctx, s.Now().UTC(), s.BatchSize)
	if err != nil || len(rows) == 0 {
		return err
	}
	return s.sendRows(ctx, rows)
}

func (s *Sender) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("invalid usage sender interval")
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			_ = s.FlushOnce(ctx)
			timer.Reset(interval)
		}
	}
}

func (s *Sender) sendRows(ctx context.Context, rows []Row) error {
	batch := usageingestapi.UsageBatch{Events: make([]usageingestapi.RequestUsageEvent, 0, len(rows))}
	for _, row := range rows {
		var event usageingestapi.RequestUsageEvent
		if err := json.Unmarshal(row.Payload, &event); err != nil || event.RequestId != row.RequestID {
			_ = s.failRows(ctx, []Row{row}, "poison_spool_payload", false)
			return fmt.Errorf("invalid persisted usage payload for %s", row.RequestID)
		}
		batch.Events = append(batch.Events, event)
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.HubURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+s.Token)
	response, err := s.Client.Do(request)
	if err != nil {
		_ = s.failRows(ctx, rows, "hub_unavailable", false)
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnprocessableEntity {
		if len(rows) == 1 {
			_ = s.failRows(ctx, rows, "poison_usage_event", false)
			return fmt.Errorf("Hub rejected usage event %s", rows[0].RequestID)
		}
		middle := len(rows) / 2
		leftErr := s.sendRows(ctx, rows[:middle])
		rightErr := s.sendRows(ctx, rows[middle:])
		if leftErr != nil || rightErr != nil {
			return errors.Join(leftErr, rightErr)
		}
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		authFailure := response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden
		code := fmt.Sprintf("hub_http_%d", response.StatusCode)
		_ = s.failRows(ctx, rows, code, authFailure)
		return fmt.Errorf("Hub usage ingest status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var ack usageingestapi.UsageBatchAck
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&ack); err != nil || ack.AcceptedCount+ack.DuplicateCount != len(rows) {
		_ = s.failRows(ctx, rows, "invalid_hub_ack", false)
		if err != nil {
			return err
		}
		return fmt.Errorf("Hub usage ACK count mismatch")
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].RequestID
	}
	return s.Spool.Ack(ctx, ids)
}

func (s *Sender) failRows(ctx context.Context, rows []Row, code string, authFailure bool) error {
	if len(rows) == 0 {
		return nil
	}
	maxAttempt := 0
	for _, row := range rows {
		if row.AttemptCount > maxAttempt {
			maxAttempt = row.AttemptCount
		}
	}
	backoff := exponentialBackoff(maxAttempt + 1)
	if authFailure && backoff < 30*time.Second {
		backoff = 30 * time.Second
	}
	backoff += s.Jitter(backoff)
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].RequestID
	}
	return s.Spool.MarkFailed(ctx, ids, s.Now().UTC().Add(backoff), code)
}

func exponentialBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 6 {
		shift = 6
	}
	backoff := time.Second * time.Duration(1<<shift)
	if backoff > 60*time.Second {
		return 60 * time.Second
	}
	return backoff
}
