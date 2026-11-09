package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"measix/platform/internal/hub/capability"
)

func TestStarterV5StrictWireAndV4Isolation(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(fixtureRoot(t), "..", "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "client-integration", "snapshot-v5.json"))
	if err != nil {
		t.Fatal(err)
	}
	fresh := func() map[string]any {
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	opening := func(value map[string]any) map[string]any {
		return value["starters"].([]any)[0].(map[string]any)["openingSnapshot"].(map[string]any)
	}
	cases := map[string]func(map[string]any){
		"missing opening": func(v map[string]any) { delete(v["starters"].([]any)[0].(map[string]any), "openingSnapshot") },
		"null opening":    func(v map[string]any) { v["starters"].([]any)[0].(map[string]any)["openingSnapshot"] = nil },
		"future format":   func(v map[string]any) { opening(v)["format"] = 2 },
		"v4 opening":      func(v map[string]any) { v["schemaVersion"] = 4 },
		"blank id":        func(v map[string]any) { opening(v)["initialContexts"].([]any)[0].(map[string]any)["id"] = "\n " },
	}
	for _, field := range []string{"format", "systemPrompt", "initialContexts"} {
		field := field
		cases["missing "+field] = func(v map[string]any) { delete(opening(v), field) }
		cases["null "+field] = func(v map[string]any) { opening(v)[field] = nil }
	}
	for _, field := range []string{"id", "content"} {
		field := field
		cases["block missing "+field] = func(v map[string]any) { delete(opening(v)["initialContexts"].([]any)[0].(map[string]any), field) }
		cases["block null "+field] = func(v map[string]any) { opening(v)["initialContexts"].([]any)[0].(map[string]any)[field] = nil }
	}
	if doc.Components.Schemas["StarterOpeningSnapshot"].Value.Properties["initialContexts"].Value.Extensions["x-uniqueProperty"] != "id" {
		t.Fatal("Android generation requires unique-property contract")
	}
	schema := doc.Components.Schemas["ManagedSnapshot"].Value
	for name, change := range map[string]func(map[string]any){
		"ignored obsolete description": func(v map[string]any) { v["starters"].([]any)[0].(map[string]any)["description"] = "obsolete" },
		"ignored null description":     func(v map[string]any) { v["starters"].([]any)[0].(map[string]any)["description"] = nil },
		"ignored opening extension":    func(v map[string]any) { opening(v)["futureDisplayHint"] = true },
		"ignored obsolete title":       func(v map[string]any) { opening(v)["initialContexts"].([]any)[0].(map[string]any)["title"] = "  " },
		"ignored block extension": func(v map[string]any) {
			opening(v)["initialContexts"].([]any)[0].(map[string]any)["futureDisplayHint"] = "advisory"
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := fresh()
			change(value)
			if err := schema.VisitJSON(value); err != nil {
				t.Fatal(err)
			}
			hash, err := capability.HashSnapshot(value)
			if err != nil || hash != value["snapshotHash"] {
				t.Fatalf("ignored field changed projection: %s %v", hash, err)
			}
		})
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			value := fresh()
			change(value)
			if err := schema.VisitJSON(value); err == nil {
				t.Fatal("invalid v5 wire accepted")
			}
		})
	}
	value := fresh()
	opening(value)["systemPrompt"] = ""
	opening(value)["initialContexts"] = []any{}
	if err := schema.VisitJSON(value); err != nil {
		t.Fatal(err)
	}
	endpoint := doc.Paths.Value("/api/client/v1/managed/snapshots/{generation}").Get.Responses.Status(200).Value.Content["application/json"].Schema.Value
	if len(endpoint.OneOf) != 2 {
		t.Fatal("download must describe the two actual persisted versions")
	}
	for _, version := range []string{"v4", "v5"} {
		raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "client-integration", "snapshot-"+version+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var value any
		json.Unmarshal(raw, &value)
		if err := endpoint.VisitJSON(value); err != nil {
			t.Fatalf("%s: %v", version, err)
		}
	}
	// Admin and Client both use title-free background blocks.
	admin, err := openapi3.NewLoader().LoadFromFile(filepath.Join(fixtureRoot(t), "..", "admin", "admin.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	block := map[string]any{"id": "pending", "content": ""}
	if err := admin.Components.Schemas["StarterInitialContext"].Value.VisitJSON(block); err != nil {
		t.Fatal(err)
	}
	starter := fresh()["starters"].([]any)[0].(map[string]any)
	adminStarter := admin.Components.Schemas["AssistantStarterDefinition"].Value
	if err := adminStarter.VisitJSON(starter); err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []any{"obsolete", nil} {
		starter["description"] = obsolete
		if err := adminStarter.VisitJSON(starter); err == nil {
			t.Fatal("Admin must reject obsolete Starter description")
		}
	}
}
