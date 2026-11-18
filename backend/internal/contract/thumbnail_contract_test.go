package contract_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWorkspaceThumbnailContractAndSharedFixtures(t *testing.T) {
	for _, contract := range []struct{ file, endpoint string }{
		{"api/admin/admin.openapi.yaml", "/api/admin/v1/users/{userId}/workspace/thumbnail"},
		{"api/client/client-control.openapi.yaml", "/api/client/v1/workspace/thumbnail"},
	} {
		doc := loadContractDoc(t, contract.file)
		resource := doc.Paths.Find(contract.endpoint)
		if resource == nil || resource.Options == nil || resource.Get == nil || resource.Head == nil || resource.Put != nil || resource.Post != nil {
			t.Fatalf("missing independent read-only thumbnail resource: %s", contract.file)
		}
		for _, parameter := range resource.Options.Parameters {
			if parameter.Value.In == "query" && parameter.Value.Name != "agentSpaceId" {
				t.Fatal("capability discovery depends on a file query")
			}
		}
		if resource.Get.Responses.Status(200).Value.Content["image/webp"] == nil {
			t.Fatal("thumbnail response can be mistaken for original content")
		}
		for status, response := range resource.Head.Responses.Map() {
			if len(response.Value.Content) != 0 {
				t.Fatalf("HEAD %s declares a body", status)
			}
		}
		schema := doc.Components.Schemas["WorkspaceThumbnailCapabilities"].Value
		for _, fixture := range []string{"supported", "unsupported"} {
			raw, err := os.ReadFile("../../../api/fixtures/workspace/thumbnail-capabilities-" + fixture + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			value["futureObservation"] = map[string]any{"enabled": true}
			if err := schema.VisitJSON(value); err != nil {
				t.Fatalf("valid additive capability fixture rejected: %v", err)
			}
			value["supported"] = nil
			if schema.VisitJSON(value) == nil {
				t.Fatal("known capability field accepts null")
			}
			delete(value, "supported")
			if schema.VisitJSON(value) == nil {
				t.Fatal("capability omits required support state")
			}
		}
	}
}
