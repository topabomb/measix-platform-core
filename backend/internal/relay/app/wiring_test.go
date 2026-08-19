package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/topabomb/measix-platform-core/backend/internal/relay/app"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func TestRelayAppFailsClosedUntilControlApply(t *testing.T) {
	a := app.New("relay-service-token")

	ready := httptest.NewRecorder()
	a.Public.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready before control=%d want=503", ready.Code)
	}

	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 0,
		DeploymentId:            platformid.New(platformid.Deployment),
		AuthKeys:                []relaycontrolapi.PublicJwk{},
		PrincipalState: relaycontrolapi.PrincipalState{
			DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
		},
		ResourceRoutes:    []relaycontrolapi.ResourceRoute{},
		Routes:            []relaycontrolapi.RuntimeRouteSpec{},
		Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1024},
	}
	hash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	state.BundleHash = hash
	payload, _ := json.Marshal(state)
	request := httptest.NewRequest(http.MethodPut, "/internal/v1/control/state", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer relay-service-token")
	request.Header.Set("Content-Type", "application/json")
	applied := httptest.NewRecorder()
	a.Internal.ServeHTTP(applied, request)
	if applied.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", applied.Code, applied.Body.String())
	}

	ready = httptest.NewRecorder()
	a.Public.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("ready after control=%d want=200", ready.Code)
	}

	resourceID := platformid.New(platformid.Model)
	runtime := httptest.NewRecorder()
	a.Public.ServeHTTP(runtime, httptest.NewRequest(http.MethodPost, "/runtime/v1/resources/"+resourceID+"/v1/chat/completions", nil))
	if runtime.Code != http.StatusUnauthorized {
		t.Fatalf("runtime was not mounted on public listener: status=%d body=%s", runtime.Code, runtime.Body.String())
	}
}

func TestRelayInternalControlRequiresServiceCredential(t *testing.T) {
	a := app.New("relay-service-token")
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/control/status", nil)
	response := httptest.NewRecorder()
	a.Internal.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("internal status without credential=%d want=401", response.Code)
	}
}
