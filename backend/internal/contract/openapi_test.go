package contract_test

import (
	"context"
	"github.com/getkin/kin-openapi/openapi3"
	"path/filepath"
	"runtime"
	"testing"
)

func TestS0OpenAPISurfacesValidate(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	files := []string{"api/admin/admin.openapi.yaml", "api/client/client-control.openapi.yaml", "api/internal/relay-control.openapi.yaml", "api/internal/usage-ingest.openapi.yaml"}
	for _, rel := range files {
		loader := openapi3.NewLoader()
		loader.IsExternalRefsAllowed = false
		doc, err := loader.LoadFromFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("load %s: %v", rel, err)
		}
		if err := doc.Validate(context.Background()); err != nil {
			t.Fatalf("validate %s: %v", rel, err)
		}
	}
}
