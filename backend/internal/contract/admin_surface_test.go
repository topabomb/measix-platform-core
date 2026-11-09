package contract_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The authoritative S0 Control Protocol requires GET/POST /api/admin/v1/upstreams.
// Without the list operation the Admin Console cannot manage server-generated upstream IDs.
func TestAdminContractExposesUpstreamList(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(root, "api/admin/admin.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	item := doc.Paths.Find("/api/admin/v1/upstreams")
	if item == nil || item.Get == nil {
		t.Fatal("GET /api/admin/v1/upstreams is required by the S0 Admin/Control contract")
	}
	if item.Get.Responses.Value("200") == nil {
		t.Fatal("upstream list GET must define a 200 response")
	}
}
