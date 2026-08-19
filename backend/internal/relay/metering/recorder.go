package metering

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/usageingestapi"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

type Recorder struct {
	Spool    *Spool
	Timeout  time.Duration
	degraded atomic.Bool
}

func NewRecorder(spool *Spool) *Recorder {
	return &Recorder{Spool: spool, Timeout: 2 * time.Second}
}

func (r *Recorder) Record(event usageingestapi.RequestUsageEvent) error {
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
