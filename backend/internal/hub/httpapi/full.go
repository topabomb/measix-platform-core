package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
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

func RegisterFull(router chi.Router, services Services, options Options) {
	admin := &fullAdminHandler{
		adminHandler: &adminHandler{identity: services.Identity},
		services:     services,
	}
	client := &fullClientHandler{clientHandler: &clientHandler{identity: services.Identity, options: options}}
	adminapi.HandlerFromMux(admin, router)
	clientapi.HandlerFromMux(client, router)
}

func NewFull(services Services, options Options) http.Handler {
	router := chi.NewRouter()
	RegisterFull(router, services, options)
	return router
}
