package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestManagedPolicySchemaMatchesAcrossAdminAndClient(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	admin := loadOpenAPIDocument(t, filepath.Join(root, "api/admin/admin.openapi.yaml"))
	client := loadOpenAPIDocument(t, filepath.Join(root, "api/client/client-control.openapi.yaml"))

	adminSchema, err := json.Marshal(admin.Components.Schemas["ManagedPolicy"])
	if err != nil {
		t.Fatalf("marshal Admin ManagedPolicy: %v", err)
	}
	clientSchema, err := json.Marshal(client.Components.Schemas["ManagedPolicy"])
	if err != nil {
		t.Fatalf("marshal Client ManagedPolicy: %v", err)
	}
	if !bytes.Equal(adminSchema, clientSchema) {
		t.Fatalf("Admin and Client ManagedPolicy schemas differ\nAdmin: %s\nClient: %s", adminSchema, clientSchema)
	}
}

func loadOpenAPIDocument(t *testing.T, path string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return doc
}
