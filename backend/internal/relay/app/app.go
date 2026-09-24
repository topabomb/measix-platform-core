package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/common/health"
	"measix/platform/internal/common/observability"
	"measix/platform/internal/relay/budget"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/relay/metering"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/relaycontrolapi"
)

type App struct {
	Public, Internal http.Handler
	Health           *health.State
	Control          *control.Store
	Recorder         *metering.Recorder
	Spool            *metering.Spool
	Telemetry        *observability.Recorder
}

func New(serviceToken, buildVersion string, spool *metering.Spool, recorder *metering.Recorder, budgetClient budget.Client) *App {
	h := &health.State{}
	store := control.NewStore(nil)
	telemetry := observability.NewRecorder(nil)
	log := observability.Logger{Log: slog.Default()}

	pub := chi.NewRouter()
	pub.Use(observability.HTTPMiddleware(telemetry, log))
	pub.Get("/live", h.Live)
	pub.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		h.SetReady(store.Current() != nil)
		h.Ready(w, r)
	})
	pub.Handle("/runtime/v1/resources/*", relayruntime.NewHandler(store, recorder, budgetClient))

	internal := chi.NewRouter()
	internal.Use(observability.HTTPMiddleware(telemetry, log))
	internal.Get("/live", h.Live)
	var statusProvider control.SpoolStatusProvider
	if spool != nil {
		statusProvider = func(ctx context.Context) (control.SpoolStatus, error) {
			stats, err := spool.Stats(ctx, store.Now())
			if err != nil {
				return control.SpoolStatus{}, err
			}
			state := relaycontrolapi.ControlStatusSpoolState(stats.State)
			if recorder != nil && recorder.State() == metering.StateDegraded {
				state = relaycontrolapi.METERINGDEGRADED
			}
			var oldest *int
			if stats.PendingCount > 0 {
				value := int(stats.OldestPendingAge.Seconds())
				oldest = &value
			}
			return control.SpoolStatus{State: state, PendingCount: stats.PendingCount, OldestAgeSeconds: oldest}, nil
		}
	}
	var applyHook control.ApplyHook
	if spool != nil {
		applyHook = func(ctx context.Context, state relaycontrolapi.RuntimeControlState) error {
			return spool.PurgeUsers(ctx, state.PrincipalState.DeletedUserIds)
		}
	}
	internal.Mount("/", control.NewHandlerWithTelemetry(store, serviceToken, buildVersion, statusProvider, applyHook, telemetry))

	return &App{
		Public:   pub,
		Internal: internal,
		Health:   h, Control: store, Recorder: recorder, Spool: spool, Telemetry: telemetry,
	}
}
