package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
)

func TestSnapshotWireVersionRequirements(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(fixtureRoot(t), "..", "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	schema := doc.Components.Schemas["ManagedSnapshot"].Value
	t.Run("compiler empty experience", func(t *testing.T) {
		draft := decodeFixture[adminapi.Draft](t, "draft/minimal.json", true)
		allow := false
		draft.Content.Policy.AllowLocalAssistants = allow
		draft.Content.Assistants = []adminapi.ManagedAssistantDefinition{}
		draft.Content.Starters = []adminapi.AssistantStarterDefinition{}
		snapshot, _, err := capability.NewService(nil).CompileSnapshot(capability.SnapshotInput{
			DeploymentID: "dep_550e8400-e29b-41d4-a716-446655440000", ReleaseID: "rel_550e8400-e29b-41d4-a716-446655440000",
			ManagedGeneration: 1, PublishedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Content: draft.Content,
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.VisitJSON(value); err != nil {
			t.Fatal(err)
		}
	})
	read := func(name string) map[string]any {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "snapshot", name))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	for _, name := range []string{"v4-user-configuration-policy.json"} {
		t.Run(name, func(t *testing.T) {
			if err := schema.VisitJSON(read(name)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, flag := range []string{"allowLocalProviders", "allowLocalTts", "allowLocalAsr", "allowLocalMcp", "allowLocalAssistants"} {
		for _, mutation := range []string{"missing", "null", "string"} {
			t.Run(flag+"/"+mutation, func(t *testing.T) {
				value := read("v4-user-configuration-policy.json")
				policy := value["policy"].(map[string]any)
				switch mutation {
				case "missing":
					delete(policy, flag)
				case "null":
					policy[flag] = nil
				case "string":
					policy[flag] = "false"
				}
				if err := schema.VisitJSON(value); err == nil {
					t.Fatal("invalid v4 policy accepted")
				}
			})
		}
	}
	for _, field := range []string{"assistants", "starters"} {
		t.Run("missing/"+field, func(t *testing.T) {
			value := read("v4-user-configuration-policy.json")
			delete(value, field)
			if err := schema.VisitJSON(value); err == nil {
				t.Fatal("v4 collection missing")
			}
		})
	}
	for _, version := range []int{0, 1, 2, 3, 5, 99} {
		t.Run("version/"+strconv.Itoa(version), func(t *testing.T) {
			value := read("v4-user-configuration-policy.json")
			value["schemaVersion"] = version
			if err := schema.VisitJSON(value); err == nil {
				t.Fatal("unsupported profile accepted")
			}
		})
	}
}
