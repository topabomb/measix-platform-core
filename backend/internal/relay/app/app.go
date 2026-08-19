package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/common/health"
	"github.com/topabomb/measix-platform-core/backend/internal/relay/control"
	relayruntime "github.com/topabomb/measix-platform-core/backend/internal/relay/runtime"
)

type App struct {
	Public, Internal http.Handler
	Health           *health.State
	Control          *control.Store
}

func New(serviceToken string) *App {
	h := &health.State{}
	store := control.NewStore(nil)

	pub := chi.NewRouter()
	pub.Get("/live", h.Live)
	pub.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		h.SetReady(store.Current() != nil)
		h.Ready(w, r)
	})
	pub.Mount("/runtime/v1/resources", relayruntime.NewHandler(store))

	internal := chi.NewRouter()
	internal.Get("/live", h.Live)
	internal.Mount("/", control.NewHandler(store, serviceToken))

	return &App{Public: pub, Internal: internal, Health: h, Control: store}
}
