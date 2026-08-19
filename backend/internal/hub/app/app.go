package app

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/common/health"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/config"
	"net/http"
)

type App struct {
	Router http.Handler
	Health *health.State
}

func New(cfg config.Config) *App {
	h := &health.State{}
	h.SetReady(true)
	r := chi.NewRouter()
	r.Get("/live", h.Live)
	r.Get("/ready", h.Ready)
	r.Get("/.well-known/measix", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"product": "MEASIX_AGENT_PLATFORM", "protocolVersion": "1", "deploymentName": "uninitialized", "clientApiBase": "/api/client/v1", "runtimeApiBase": "/runtime/v1", "supportedSnapshotSchemaVersions": []int{1}})
	})
	return &App{Router: r, Health: h}
}
