package contract_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestFinalS0ClientControlSurfaceMatchesArchitecture(t *testing.T) {
	doc := loadOpenAPI(t, "api/client/client-control.openapi.yaml")
	required := []string{
		"/.well-known/measix",
		"/api/v1/enrollments/exchange",
		"/api/v1/auth/refresh",
		"/api/v1/auth/logout",
		"/api/v1/bootstrap",
		"/api/v1/managed/state",
		"/api/v1/managed/snapshots/{generation}",
		"/api/v1/managed/events",
		"/api/v1/devices/heartbeat",
	}
	for _, path := range required {
		if doc.Paths.Find(path) == nil {
			t.Errorf("final S0 client control path missing: %s", path)
		}
	}
	if doc.Paths.Find("/api/client/v1/managed/state") != nil {
		t.Error("obsolete /api/client/v1 namespace is still present")
	}
}

func TestFinalS0GatewayControlSurfaceUsesActivationSaga(t *testing.T) {
	doc := loadOpenAPI(t, "api/internal/relay-control.openapi.yaml")
	required := []string{
		"/internal/v1/runtime-control/prepare",
		"/internal/v1/runtime-control/activations/{activationId}/barrier",
		"/internal/v1/runtime-control/activations/{activationId}/commit",
		"/internal/v1/runtime-control/activations/{activationId}/abort",
		"/internal/v1/runtime-control/apply-operational",
		"/internal/v1/runtime-control/status",
	}
	for _, path := range required {
		if doc.Paths.Find(path) == nil {
			t.Errorf("final S0 gateway control path missing: %s", path)
		}
	}
	if doc.Paths.Find("/internal/v1/control/state") != nil {
		t.Error("obsolete one-shot full-state apply endpoint is still present")
	}
}

func loadOpenAPI(t *testing.T, rel string) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("load %s: %v", rel, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate %s: %v", rel, err)
	}
	return doc
}
