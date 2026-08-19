package app

import (
	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/common/health"
	"net/http"
)

type App struct {
	Public, Internal http.Handler
	Health           *health.State
}

func New() *App {
	h := &health.State{}
	pub := chi.NewRouter()
	pub.Get("/live", h.Live)
	pub.Get("/ready", h.Ready)
	internal := chi.NewRouter()
	internal.Get("/live", h.Live)
	return &App{Public: pub, Internal: internal, Health: h}
}
