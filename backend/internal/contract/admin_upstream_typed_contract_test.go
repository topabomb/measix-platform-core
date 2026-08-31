package contract_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The S0 Control Protocol (§15) fixes the Upstream transport capability
// vocabulary. A typed executable contract must enumerate exactly these four
// values rather than accepting arbitrary strings.
func TestAdminUpstreamTransportCapabilitiesFrozenEnum(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "UpstreamConfig")
	prop := getProp(t, schema, "transportCapabilities")
	items := getItems(t, prop)
	if len(items.Enum) == 0 {
		t.Fatal("UpstreamConfig.transportCapabilities items is not a frozen enum")
	}
	values := enumStringValues(t, items.Enum)
	want := map[string]bool{
		"HTTP_REQUEST_RESPONSE": true,
		"HTTP_STREAMING_SSE":    true,
		"HTTP_BINARY_STREAM":    true,
		"HTTP_MULTIPART":        true,
	}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected transport capability %q, allowed: HTTP_REQUEST_RESPONSE, HTTP_STREAMING_SSE, HTTP_BINARY_STREAM, HTTP_MULTIPART", v)
		}
	}
	if len(values) != 4 {
		t.Fatalf("transportCapabilities enum=%v want exactly 4 values", values)
	}
}

// The Upstream Adapter Contract (§14) fixes the correlation mode vocabulary.
func TestAdminUpstreamCorrelationModeFrozenEnum(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "UpstreamConfig")
	prop := getProp(t, schema, "correlationMode")
	if len(prop.Enum) == 0 {
		t.Fatal("UpstreamConfig.correlationMode is not a frozen enum")
	}
	values := enumStringValues(t, prop.Enum)
	want := map[string]bool{
		"HEADER_ECHO":    true,
		"VIRTUAL_KEY":    true,
		"REQUEST_LOG_ID": true,
		"USAGE_API":      true,
		"WEBHOOK":        true,
		"NONE":           true,
	}
	for _, v := range values {
		if !want[v] {
			t.Fatalf("unexpected correlation mode %q, allowed: HEADER_ECHO, VIRTUAL_KEY, REQUEST_LOG_ID, USAGE_API, WEBHOOK, NONE", v)
		}
	}
	if len(values) != 6 {
		t.Fatalf("correlationMode enum=%v want exactly 6 values", values)
	}
}

// The S0 Control Protocol (§15) fixes the auth scheme as a typed discriminated
// union. The executable contract must not allow arbitrary additional properties
// and must expose explicit per-scheme fields for the editor to bind.
func TestAdminUpstreamAuthIsTypedClosed(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "UpstreamConfig")
	authRef := schema.Properties["auth"]
	if authRef == nil || authRef.Value == nil {
		t.Fatal("UpstreamConfig.auth is missing from schema")
	}
	auth := authRef.Value
	if auth.AdditionalProperties.Has == nil || *auth.AdditionalProperties.Has {
		t.Fatal("UpstreamConfig.auth must be a closed schema (additionalProperties:false)")
	}
	for _, field := range []string{"type", "secretRef", "headerName", "username", "passwordSecretRef"} {
		if _, ok := auth.Properties[field]; !ok {
			t.Fatalf("UpstreamConfig.auth is missing typed field %q", field)
		}
	}
}

// A SecretRef is an immutable {secretId, secretVersion} pair (Control Protocol §15).
func TestAdminSecretRefSchemaIsTyped(t *testing.T) {
	doc := loadAdminDoc(t)
	ref := getSchema(t, doc, "SecretRef")
	if ref.AdditionalProperties.Has == nil || *ref.AdditionalProperties.Has {
		t.Fatal("SecretRef must be a closed schema (additionalProperties:false)")
	}
	for _, field := range []string{"secretId", "secretVersion"} {
		if _, ok := ref.Properties[field]; !ok {
			t.Fatalf("SecretRef is missing field %q", field)
		}
	}
	required := map[string]bool{}
	for _, r := range ref.Required {
		required[r] = true
	}
	for _, field := range []string{"secretId", "secretVersion"} {
		if !required[field] {
			t.Fatalf("SecretRef.%s must be required", field)
		}
	}
}

// The auth per-scheme SecretRef fields must reference the typed SecretRef schema
// (not a free-form object), so generated Go/TS wire types stay canonical.
func TestAdminUpstreamAuthSecretRefUsesTypedRef(t *testing.T) {
	doc := loadAdminDoc(t)
	schema := getSchema(t, doc, "UpstreamConfig")
	auth := getProp(t, schema, "auth")
	for _, field := range []string{"secretRef", "passwordSecretRef"} {
		propRef, ok := auth.Properties[field]
		if !ok || propRef == nil {
			t.Fatalf("UpstreamConfig.auth.%s is not present", field)
		}
		if propRef.Ref == "" {
			t.Fatalf("UpstreamConfig.auth.%s must $ref the SecretRef schema", field)
		}
		if name := filepath.Base(propRef.Ref); name != "SecretRef" {
			t.Fatalf("UpstreamConfig.auth.%s ref=%q want SecretRef", field, name)
		}
	}
}

func loadAdminDoc(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(filepath.Join(root, "api/admin/admin.openapi.yaml"))
	if err != nil {
		t.Fatalf("load admin openapi: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate admin openapi: %v", err)
	}
	return doc
}
