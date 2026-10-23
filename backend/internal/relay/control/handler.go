package control

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/common/observability"
	"measix/platform/internal/wire/relaycontrolapi"
)

type SpoolStatus struct {
	State            relaycontrolapi.ControlStatusSpoolState
	PendingCount     int
	OldestAgeSeconds *int
}

type SpoolStatusProvider func(context.Context) (SpoolStatus, error)
type ApplyHook func(context.Context, relaycontrolapi.RuntimeControlState) error

type handler struct {
	relaycontrolapi.Unimplemented
	store        *Store
	serviceToken string
	buildVersion string
	spoolStatus  SpoolStatusProvider
	applyHook    ApplyHook
	telemetry    *observability.Recorder
}

func NewHandler(store *Store, serviceToken, buildVersion string, statusProvider SpoolStatusProvider, applyHooks ...ApplyHook) http.Handler {
	var applyHook ApplyHook
	if len(applyHooks) > 0 {
		applyHook = applyHooks[0]
	}
	return newHandler(store, serviceToken, buildVersion, statusProvider, applyHook, nil)
}

func NewHandlerWithTelemetry(store *Store, serviceToken, buildVersion string, statusProvider SpoolStatusProvider, applyHook ApplyHook, telemetry *observability.Recorder) http.Handler {
	return newHandler(store, serviceToken, buildVersion, statusProvider, applyHook, telemetry)
}

func newHandler(store *Store, serviceToken, buildVersion string, statusProvider SpoolStatusProvider, applyHook ApplyHook, telemetry *observability.Recorder) http.Handler {
	router := chi.NewRouter()
	relaycontrolapi.HandlerFromMux(&handler{store: store, serviceToken: serviceToken, buildVersion: buildVersion, spoolStatus: statusProvider, applyHook: applyHook, telemetry: telemetry}, router)
	return router
}

func (h *handler) ApplyControlState(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		slog.Warn("control apply rejected", "event", "control.apply_failed", "errorCode", "invalid_service_credential")
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return
	}
	var input relaycontrolapi.RuntimeControlState
	if err := decodeStrictJSON(w, r, &input); err != nil {
		slog.Warn("control apply rejected", "event", "control.apply_failed", "errorCode", "invalid_request")
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request")
		return
	}
	ack, err := h.store.Apply(input)
	if err != nil {
		slog.Error("control apply failed", "event", "control.apply_failed", "error", err)
		switch {
		case errors.Is(err, ErrStaleRevision):
			writeProblem(w, http.StatusConflict, "stale_control_revision", "Stale control revision")
		case errors.Is(err, ErrRevisionHashConflict):
			writeProblem(w, http.StatusConflict, "control_revision_hash_conflict", "Control revision hash conflict")
		default:
			writeProblem(w, http.StatusUnprocessableEntity, "invalid_runtime_control", "Invalid runtime control")
		}
		return
	}
	if h.applyHook != nil {
		if err := h.applyHook(r.Context(), input); err != nil {
			slog.Error("control cleanup failed", "event", "control.apply_failed", "error", err, "controlRevision", input.ControlRevision)
			writeProblem(w, http.StatusServiceUnavailable, "control_apply_incomplete", "Runtime control cleanup incomplete")
			return
		}
	}
	slog.Info("control state applied", "event", "control.apply_completed", "controlRevision", input.ControlRevision, "managedGeneration", input.ActiveManagedGeneration)
	writeJSON(w, http.StatusOK, ack)
}

func (h *handler) GetControlStatus(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeProblem(w, http.StatusUnauthorized, "invalid_service_credential", "Unauthorized")
		return
	}
	status := h.store.Status()
	status.BuildVersion = h.buildVersion
	if h.spoolStatus != nil {
		extra, err := h.spoolStatus(r.Context())
		if err != nil {
			degraded := relaycontrolapi.METERINGDEGRADED
			status.SpoolState = &degraded
		} else {
			status.SpoolState = &extra.State
			status.SpoolPendingCount = &extra.PendingCount
			status.OldestPendingAgeSeconds = extra.OldestAgeSeconds
		}
	}
	if h.telemetry != nil {
		value := relayTelemetryWire(h.telemetry.Snapshot(60 * time.Minute))
		status.Telemetry = &value
	}
	writeJSON(w, http.StatusOK, status)
}

func relayTelemetryWire(snapshot observability.Snapshot) relaycontrolapi.ProcessTelemetry {
	buckets := make([]relaycontrolapi.TelemetryBucket, 0, len(snapshot.Buckets))
	for _, bucket := range snapshot.Buckets {
		upstream, denied := bucket.UpstreamErrorCount, bucket.BudgetDeniedCount
		buckets = append(buckets, relaycontrolapi.TelemetryBucket{
			Minute: bucket.Minute, RequestCount: bucket.RequestCount, SuccessCount: bucket.SuccessCount,
			ClientErrorCount: bucket.ClientErrorCount, ServerErrorCount: bucket.ServerErrorCount,
			RejectedCount: bucket.RejectedCount, TimeoutCount: bucket.TimeoutCount, CancelledCount: bucket.CancelledCount,
			DurationP95Ms: bucket.DurationP95Ms, InFlight: bucket.InFlightPeak,
			UpstreamErrorCount: &upstream, BudgetDeniedCount: &denied,
		})
	}
	upstream, denied := snapshot.Summary.UpstreamErrorCount, snapshot.Summary.BudgetDeniedCount
	return relaycontrolapi.ProcessTelemetry{StartedAt: snapshot.StartedAt, Buckets: buckets, Summary: relaycontrolapi.TelemetryMetrics{
		RequestCount: snapshot.Summary.RequestCount, SuccessCount: snapshot.Summary.SuccessCount,
		ClientErrorCount: snapshot.Summary.ClientErrorCount, ServerErrorCount: snapshot.Summary.ServerErrorCount,
		RejectedCount: snapshot.Summary.RejectedCount, TimeoutCount: snapshot.Summary.TimeoutCount, CancelledCount: snapshot.Summary.CancelledCount,
		DurationP95Ms: snapshot.Summary.DurationP95Ms, InFlight: snapshot.InFlight,
		UpstreamErrorCount: &upstream, BudgetDeniedCount: &denied,
	}}
}

func (h *handler) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) || h.serviceToken == "" {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	return len(provided) == len(h.serviceToken) && subtle.ConstantTimeCompare([]byte(provided), []byte(h.serviceToken)) == 1
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(relaycontrolapi.Problem{Type: "about:blank", Title: title, Status: status, Code: code})
}
