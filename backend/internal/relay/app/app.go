package app

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/common/health"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/metering"
	relayruntime "github.com/topabomb/measix-platform-core/backend/internal/relay/runtime"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
)

type App struct {
	Public, Internal http.Handler
	Health           *health.State
	Control          *control.Store
	Recorder         *metering.Recorder
	Spool            *metering.Spool
}

func New(serviceToken string) *App {
	return NewWithMetering(serviceToken, nil, nil)
}

func NewWithMetering(serviceToken string, spool *metering.Spool, recorder *metering.Recorder) *App {
	h := &health.State{}
	store := control.NewStore(nil)

	pub := chi.NewRouter()
	pub.Get("/live", h.Live)
	pub.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		h.SetReady(store.Current() != nil)
		h.Ready(w, r)
	})
	pub.Handle("/runtime/v1/resources/*", relayruntime.NewHandlerWithRecorder(store, recorder))

	internal := chi.NewRouter()
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
	internal.Mount("/", control.NewHandlerWithSpoolStatus(store, serviceToken, statusProvider))

	return &App{Public: pub, Internal: internal, Health: h, Control: store, Recorder: recorder, Spool: spool}
}
