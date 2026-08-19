package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/capability"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/identity"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/runtimecontrol"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/upstream"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/usage"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/adminapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/clientapi"
)

type Services struct {
	Identity       *identity.Service
	Capability     *capability.Service
	Upstream       *upstream.Service
	RuntimeControl *runtimecontrol.Service
	Usage          *usage.Service
	BuildVersion   string
}

type fullAdminHandler struct {
	*adminHandler
	services Services
}

func NewFull(services Services, options Options) http.Handler {
	router := chi.NewRouter()
	admin := &fullAdminHandler{
		adminHandler: &adminHandler{identity: services.Identity},
		services:     services,
	}
	adminapi.HandlerFromMux(admin, router)
	clientapi.HandlerFromMux(&clientHandler{identity: services.Identity, options: options}, router)
	return router
}
