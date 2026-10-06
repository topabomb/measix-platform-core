package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"measix/platform/internal/hub/capability"
)

// Extension tolerance is recursive, but never replaces validation of known
// fields or version selection. The published descriptor stays unchanged.
func TestClientResponseExtensionsPreserveKnownProjection(t *testing.T) {
	doc := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	for _, c := range []struct{ file, schema string }{
		{"snapshot-v5.json", "ManagedSnapshot"},
		{"snapshot-v4.json", "ManagedSnapshotV4"},
		{"discovery.json", "Discovery"},
		{"bootstrap.json", "Bootstrap"},
		{"refresh-response.json", "RefreshResponse"},
	} {
		t.Run(c.schema, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "client-integration", c.file))
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			if err := doc.Components.Schemas[c.schema].Value.VisitJSON(value); err != nil {
				t.Fatal(err)
			}
			addResponseHints(value)
			if c.schema == "ManagedSnapshotV4" {
				// Even a field known to v5 is an ignored extension to the v4 DTO.
				value["starters"].([]any)[0].(map[string]any)["openingSnapshot"] = "not a v4 field"
				value["mcp"].([]any)[0].(map[string]any)["allowedTools"] = "not a v4 field"
			}
			if err := doc.Components.Schemas[c.schema].Value.VisitJSON(value); err != nil {
				t.Fatalf("ordinary response extension rejected: %v", err)
			}
			if c.schema == "ManagedSnapshot" || c.schema == "ManagedSnapshotV4" {
				hash, err := capability.HashSnapshot(value)
				if err != nil || hash != value["snapshotHash"] {
					t.Fatalf("extension changed the versioned descriptor: %s %v", hash, err)
				}
				value["policy"].(map[string]any)["allowLocalMcp"] = "true"
				if doc.Components.Schemas[c.schema].Value.VisitJSON(value) == nil {
					t.Fatal("known boolean accepted a string")
				}
			}
		})
	}
}

func addResponseHints(value any) {
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			addResponseHints(child)
		}
		v["futureDisplayHint"] = map[string]any{"label": "advisory", "revision": 1}
	case []any:
		for _, child := range v {
			addResponseHints(child)
		}
	}
}

func TestResponseToleranceDoesNotWidenCommands(t *testing.T) {
	doc := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	for _, c := range []struct {
		schema string
		value  map[string]any
	}{
		{"ManagedAppliedReport", map[string]any{"managedGeneration": 42, "snapshotHash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{"RefreshRequest", map[string]any{"refreshToken": "synthetic-refresh"}},
	} {
		t.Run(c.schema, func(t *testing.T) {
			schema := doc.Components.Schemas[c.schema].Value
			if err := schema.VisitJSON(c.value); err != nil {
				t.Fatal(err)
			}
			c.value["unhandledCommand"] = true
			if schema.VisitJSON(c.value) == nil {
				t.Fatal("unknown command field accepted")
			}
		})
	}
}

func TestClientContractDoesNotExportAdminAuthoringPrototypes(t *testing.T) {
	doc := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	for _, name := range []string{"ManagedDraftContent", "RuntimeBindingDefinition", "TimeoutPolicy", "ValidationIssue"} {
		if doc.Components.Schemas[name] != nil {
			t.Errorf("Client contract still exports unused Admin/internal prototype %s", name)
		}
	}
}
