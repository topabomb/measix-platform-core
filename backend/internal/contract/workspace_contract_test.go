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

func TestWorkspaceMCPCredentialFixtureIsAdminOnly(t *testing.T) {
	doc := loadContractDoc(t, "api/admin/admin.openapi.yaml")
	raw, err := os.ReadFile("../../../api/fixtures/workspace/mcp-connection.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["WorkspaceMCPConnection"].Value.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	client := loadContractDoc(t, "api/client/client-control.openapi.yaml")
	if client.Components.Schemas["WorkspaceMCPConnection"] != nil {
		t.Fatal("MCP credential leaked to client contract")
	}
	if doc.Paths.Find("/api/admin/v1/users/{userId}/workspace/mcp-connection").Get != nil {
		t.Fatal("credential reveal is not an explicit POST")
	}
}

func TestWorkspaceLinksAreAdminOnlyAndNeverContainCredentials(t *testing.T) {
	doc := loadContractDoc(t, "api/admin/admin.openapi.yaml")
	raw, err := os.ReadFile("../../../api/fixtures/workspace/links.json")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	schema := doc.Components.Schemas["WorkspaceLinks"].Value
	if err = schema.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	value["token"] = "must-not-be-exposed"
	if schema.VisitJSON(value) == nil {
		t.Fatal("links allow credentials")
	}
	if loadContractDoc(t, "api/client/client-control.openapi.yaml").Components.Schemas["WorkspaceLinks"] != nil {
		t.Fatal("raw links leaked to client contract")
	}
}

func TestDeploymentReplacementFixturesAreAdminOnly(t *testing.T) {
	doc := loadContractDoc(t, "api/admin/admin.openapi.yaml")
	for schema, name := range map[string]string{"WorkspaceServiceCheck": "replacement-check.json", "ReplaceWorkspaceDeploymentRequest": "replacement-request.json", "ApplyWorkspaceServiceRequest": "apply-request.json"} {
		raw, err := os.ReadFile(filepath.Join("../../../api/fixtures/workspace", name))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err = doc.Components.Schemas[schema].Value.VisitJSON(value); err != nil {
			t.Fatal(schema, err)
		}
	}
	if loadContractDoc(t, "api/client/client-control.openapi.yaml").Components.Schemas["ReplaceWorkspaceDeploymentRequest"] != nil {
		t.Fatal("deployment replacement exposed to clients")
	}
	path := doc.Paths.Find("/api/admin/v1/remote-workspace/services/{workspaceServiceId}/replace")
	if path == nil || path.Post == nil {
		t.Fatal("missing replacement endpoint")
	}
	if len(path.Post.Parameters) != 3 || path.Post.RequestBody == nil || !path.Post.RequestBody.Value.Required {
		t.Fatal("replacement must require CSRF, idempotency and explicit request")
	}
	apply := doc.Paths.Find("/api/admin/v1/remote-workspace/services/{workspaceServiceId}/apply")
	if apply == nil || apply.Post.RequestBody == nil || !apply.Post.RequestBody.Value.Required {
		t.Fatal("apply must require the administrator-confirmed revision")
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
