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

type JournalAdmission struct {
	Admission      usageingestapi.BudgetAdmissionRequest `json:"admission"`
	RuntimeRouteID string                                `json:"runtimeRouteId"`
}

func NewRecorder(spool *Spool) *Recorder {
	return &Recorder{Spool: spool, Timeout: 2 * time.Second, Log: slog.Default()}
}

func (r *Recorder) PersistAdmission(admission usageingestapi.BudgetAdmissionRequest, runtimeRouteID string) (err error) {
	defer r.captureFailure(admission.RequestId, "budget admission journal write failed", &err)
	if err := r.valid(admission.RequestId); err != nil {
		return err
	}
	if platformid.Validate(platformid.Route, runtimeRouteID) != nil {
		return errors.New("invalid runtime route for admission journal")
	}
	payload, err := json.Marshal(JournalAdmission{Admission: admission, RuntimeRouteID: runtimeRouteID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	return r.Spool.SaveAdmission(ctx, admission.RequestId, payload, admission.AdmittedAt)
}

func (r *Recorder) MarkStarted(requestID string, startedAt time.Time) (err error) {
	defer r.captureFailure(requestID, "budget start journal write failed", &err)
	if err := r.valid(requestID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	return r.Spool.MarkStarted(ctx, requestID, startedAt)
}

func (r *Recorder) AbortAdmission(requestID string) (err error) {
	defer r.captureFailure(requestID, "budget admission journal cleanup failed", &err)
	if err := r.valid(requestID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	return r.Spool.AbortAdmission(ctx, requestID)
}

func (r *Recorder) Record(settlement usageingestapi.UsageSettlement) (err error) {
	defer r.captureFailure(settlement.RequestId, "usage settlement could not be persisted", &err)
	if err := r.valid(settlement.RequestId); err != nil || settlement.OccurredAt.IsZero() {
		if err != nil {
			return err
		}
		return errors.New("invalid usage settlement")
	}
	payload, err := json.Marshal(settlement)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	return r.Spool.AppendSettlement(ctx, settlement.RequestId, payload, settlement.OccurredAt)
}

func (r *Recorder) RecordDenied(admission usageingestapi.BudgetAdmissionRequest, runtimeRouteID string, settlement usageingestapi.UsageSettlement) (err error) {
	defer r.captureFailure(admission.RequestId, "denied request fact could not be persisted", &err)
	if err := r.valid(admission.RequestId); err != nil || admission.RequestId != settlement.RequestId || platformid.Validate(platformid.Route, runtimeRouteID) != nil {
		if err != nil {
			return err
		}
		return errors.New("invalid denied request fact")
	}
	journal, err := json.Marshal(JournalAdmission{Admission: admission, RuntimeRouteID: runtimeRouteID})
	if err != nil {
		return err
	}
	payload, err := json.Marshal(settlement)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	return r.Spool.SaveDenied(ctx, admission.RequestId, journal, payload, settlement.OccurredAt)
}

func (r *Recorder) valid(requestID string) error {
	if r == nil || r.Spool == nil || r.Timeout <= 0 || platformid.Validate(platformid.Request, requestID) != nil {
		return errors.New("invalid metering recorder configuration or request")
	}
	return nil
}

func (r *Recorder) captureFailure(requestID, message string, target *error) {
	if target == nil || *target == nil || r == nil {
		return
	}
	r.degraded.Store(true)
	if r.Log != nil {
		r.Log.Error(message, "event", "usage_spool_write_failed", "requestId", requestID, "error", *target)
	}
}

func (r *Recorder) State() string {
	if r != nil && r.degraded.Load() {
		return StateDegraded
	}
	return StateOK
}
