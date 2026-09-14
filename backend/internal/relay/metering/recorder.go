package metering

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

type Recorder struct {
	Spool    *Spool
	Timeout  time.Duration
	Log      *slog.Logger
	degraded atomic.Bool
}

func NewRecorder(spool *Spool) *Recorder {
	return &Recorder{Spool: spool, Timeout: 2 * time.Second, Log: slog.Default()}
}

func (r *Recorder) Record(event usageingestapi.RequestUsageEvent) (err error) {
	defer func() {
		if err != nil && r != nil {
			r.degraded.Store(true)
			if r.Log != nil {
				r.Log.Error("request usage could not be persisted", "event", "usage_spool_append_failed", "requestId", event.RequestId)
			}
		}
	}()
	// Keep loss sticky: later successful writes cannot restore a lost fact.
	if r == nil || r.Spool == nil || r.Timeout <= 0 || platformid.Validate(platformid.Request, event.RequestId) != nil {
		return errors.New("invalid metering recorder configuration or event")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		r.degraded.Store(true)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	createdAt := event.CompletedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if err := r.Spool.Append(ctx, event.RequestId, payload, createdAt); err != nil {
		r.degraded.Store(true)
		return err
	}
	return nil
}

func (r *Recorder) State() string {
	if r != nil && r.degraded.Load() {
		return StateDegraded
	}
	return StateOK
}
