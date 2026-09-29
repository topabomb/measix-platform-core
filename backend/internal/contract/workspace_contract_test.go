package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceResourceFixtureIsAdminOnly(t *testing.T) {
	doc := loadContractDoc(t, "api/admin/admin.openapi.yaml")
	schema := doc.Components.Schemas["WorkspaceResources"].Value
	raw, err := os.ReadFile("../../../api/fixtures/workspace/resources.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	json.Unmarshal(raw, &value)
	if err = schema.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	disk := value["disk"].(map[string]any)
	disk["status"] = "historical"
	disk["reason"] = "stopped"
	if err = schema.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	disk["status"] = "error"
	disk["value"] = nil
	disk["observedAt"] = nil
	if err = schema.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	if loadContractDoc(t, "api/client/client-control.openapi.yaml").Components.Schemas["WorkspaceResources"] != nil {
		t.Fatal("admin observations leaked to client contract")
	}
}

func TestWorkspaceClientProjectionFixtures(t *testing.T) {
	for _, contract := range []string{"api/client/client-control.openapi.yaml", "api/admin/admin.openapi.yaml"} {
		doc := loadContractDoc(t, contract)
		schema := doc.Components.Schemas["WorkspaceProjection"].Value
		for _, name := range []string{"projection-unprovisioned.json", "projection-files-only.json"} {
			raw, err := os.ReadFile(filepath.Join("../../../api/fixtures/workspace", name))
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err = json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			if err = schema.VisitJSON(value); err != nil {
				t.Fatalf("%s/%s: %v", contract, name, err)
			}
			for _, state := range []string{"NOT_CONFIGURED", "DISABLED", "ENABLED"} {
				value["serviceState"] = state
				if err = schema.VisitJSON(value); err != nil {
					t.Fatal(err)
				}
			}
			value["serviceState"] = "UNKNOWN"
			if schema.VisitJSON(value) == nil {
				t.Fatal("accepted unknown service state")
			}
			delete(value, "serviceState")
			if schema.VisitJSON(value) == nil {
				t.Fatal("accepted missing service state")
			}
		}
	}
}

func TestWorkspaceTargetFixturesAreExclusiveAndMCPOnly(t *testing.T) {
	doc := loadContractDoc(t, "api/internal/usage-ingest.openapi.yaml")
	for schema, file := range map[string]string{"BudgetAdmissionRequest": "admission-v2.json", "RequestUsageFact": "usage-target-v2.json"} {
		t.Run(schema, func(t *testing.T) {
			raw, e := os.ReadFile(filepath.Join("../../../api/fixtures/workspace", file))
			if e != nil {
				t.Fatal(e)
			}
			var fixture map[string]any
			if e = json.Unmarshal(raw, &fixture); e != nil {
				t.Fatal(e)
			}
			cases := []struct {
				name  string
				valid bool
				edit  func(map[string]any)
			}{
				{"integration", true, func(map[string]any) {}},
				{"legacy", true, func(v map[string]any) {
					delete(v, "targetVersion")
					delete(v, "workspaceTarget")
					v["upstreamId"] = "ups_550e8400-e29b-41d4-a716-446655440000"
				}},
				{"both", false, func(v map[string]any) { v["upstreamId"] = "ups_550e8400-e29b-41d4-a716-446655440000" }},
				{"neither", false, func(v map[string]any) { delete(v, "targetVersion"); delete(v, "workspaceTarget") }},
				{"unversioned", false, func(v map[string]any) { delete(v, "targetVersion") }},
				{"wrong-kind", false, func(v map[string]any) { v["resourceKind"] = "MODEL"; v["clientProtocol"] = "OPENAI_CHAT_COMPLETIONS" }},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var value map[string]any
					json.Unmarshal(raw, &value)
					tc.edit(value)
					e = doc.Components.Schemas[schema].Value.VisitJSON(value)
					if (e == nil) != tc.valid {
						t.Fatalf("valid=%v: %v", tc.valid, e)
					}
				})
			}
		})
	}
}
