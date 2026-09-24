package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/enterpriseupdate"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/system"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/hub/usage"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
)

type Services struct {
	Identity         *identity.Service
	Capability       *capability.Service
	Upstream         *upstream.Service
	RuntimeControl   *runtimecontrol.Service
	Usage            *usage.Service
	Budget           *budget.Service
	System           *system.Service
	EnterpriseUpdate *enterpriseupdate.Service
	BuildVersion     string
}

type fullAdminHandler struct {
	*adminHandler
	services Services
}

func RegisterFull(router chi.Router, services Services) {
	admin := &fullAdminHandler{
		adminHandler: &adminHandler{identity: services.Identity},
		services:     services,
	}
	client := &fullClientHandler{
		clientHandler: &clientHandler{identity: services.Identity}, capability: services.Capability,
		enterpriseUpdate: services.EnterpriseUpdate, budget: services.Budget, usage: services.Usage,
	}
	adminapi.HandlerFromMux(admin, router)
	clientapi.HandlerWithOptions(client, clientapi.ChiServerOptions{BaseRouter: router, ErrorHandlerFunc: clientBindingError})
}

// Required security-header binding failures retain the Portal protocol's 403,
// including requests rejected by generated code before the domain handler runs.
func clientBindingError(w http.ResponseWriter, r *http.Request, err error) {
	if r.Method == http.MethodDelete && r.URL.Path == "/api/portal/v1/session" {
		w.Header().Set("Cache-Control", "no-store")
		writeProblem(w, http.StatusForbidden, "forbidden", "Portal security headers required")
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func NewFull(services Services) http.Handler {
	router := chi.NewRouter()
	RegisterFull(router, services)
	return router
}
