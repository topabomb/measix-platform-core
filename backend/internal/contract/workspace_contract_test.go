package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
