package contract_test

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func findQueryParam(t *testing.T, op *openapi3.Operation, name string) *openapi3.ParameterRef {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Value != nil && p.Value.In == "query" && p.Value.Name == name {
			return p
		}
	}
	return nil
}

// Product requirements (§14.2) require combination filters on the usage read
// model. Both /usage/summary and /usage/requests must expose them so the Usage
// page can filter client-side without guessing.
func TestAdminUsageEndpointsAcceptCombinationFilters(t *testing.T) {
	doc := loadAdminDoc(t)

	required := []string{"from", "to", "userId", "resourceId", "resourceKind", "upstreamId", "status"}
	for _, path := range []string{"/api/admin/v1/usage/summary", "/api/admin/v1/usage/requests"} {
		op := doc.Paths.Find(path).Get
		if op == nil {
			t.Fatalf("%s GET operation not found", path)
		}
		names := map[string]bool{}
		for _, p := range op.Parameters {
			if p.Value != nil && p.Value.In == "query" && p.Value.Name != "" {
				names[p.Value.Name] = true
			}
		}
		for _, want := range required {
			if !names[want] {
				t.Fatalf("%s GET is missing query filter %q", path, want)
			}
		}
	}
}

// The resourceKind filter must be a closed enum of the managed resource kinds.
func TestAdminUsageResourceKindFilterIsFrozenEnum(t *testing.T) {
	doc := loadAdminDoc(t)
	op := doc.Paths.Find("/api/admin/v1/usage/requests").Get
	kindParam := findQueryParam(t, op, "resourceKind")
	if kindParam == nil || kindParam.Value.Schema == nil || kindParam.Value.Schema.Value == nil || len(kindParam.Value.Schema.Value.Enum) == 0 {
		t.Fatal("usage resourceKind filter must be a frozen enum")
	}
	values := enumStringValues(t, kindParam.Value.Schema.Value.Enum)
	want := map[string]bool{
		"PROVIDER": true,
		"MODEL":    true,
		"TTS":      true,
		"ASR":      true,
		"MCP":      true,
	}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected resourceKind %q, allowed: PROVIDER/MODEL/TTS/ASR/MCP", v)
		}
	}
}

// The status filter must be a closed enum (success / error / blocked).
func TestAdminUsageStatusFilterIsFrozenEnum(t *testing.T) {
	doc := loadAdminDoc(t)
	op := doc.Paths.Find("/api/admin/v1/usage/summary").Get
	statusParam := findQueryParam(t, op, "status")
	if statusParam == nil || statusParam.Value.Schema == nil || statusParam.Value.Schema.Value == nil || len(statusParam.Value.Schema.Value.Enum) == 0 {
		t.Fatal("usage status filter must be a frozen enum")
	}
	values := enumStringValues(t, statusParam.Value.Schema.Value.Enum)
	want := map[string]bool{"SUCCESS": true, "ERROR": true, "BLOCKED": true}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected status %q, allowed: SUCCESS/ERROR/BLOCKED", v)
		}
	}
}

// /usage/requests must keep a cursor for pagination alongside the filters.
func TestAdminUsageRequestsKeepsCursor(t *testing.T) {
	doc := loadAdminDoc(t)
	op := doc.Paths.Find("/api/admin/v1/usage/requests").Get
	if findQueryParam(t, op, "cursor") == nil {
		t.Fatal("usage/requests GET must keep cursor pagination")
	}
}
