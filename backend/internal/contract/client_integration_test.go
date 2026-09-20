package contract_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/clientapi"
)

func TestClientIntegrationSharedWireCases(t *testing.T) {
	root := filepath.Join(fixtureRoot(t), "client-integration")
	raw, err := os.ReadFile(filepath.Join(root, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Schema string
		Valid  bool
		Value  any
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 {
		t.Fatal("incomplete integration cases")
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(fixtureRoot(t), "..", "client", "client-control.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			schema := doc.Components.Schemas[c.Schema]
			if schema == nil {
				t.Fatal("missing schema", c.Schema)
			}
			err := schema.Value.VisitJSON(c.Value)
			if (err == nil) != c.Valid {
				t.Fatalf("expected valid=%v: %v", c.Valid, err)
			}
		})
	}
	for _, file := range []string{"snapshot-v4.json", "snapshot-v4-denied.json"} {
		raw, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		var s clientapi.ManagedSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		hash, err := capability.HashSnapshot(s)
		if err != nil || hash != s.SnapshotHash {
			t.Fatalf("compiler hash mismatch: %s %v", file, err)
		}
		if len(s.Models) == 0 || len(s.Tts) == 0 || len(s.Asr) == 0 || len(s.Mcp) == 0 || len(s.Assistants) < 2 || len(s.Starters) < 3 {
			t.Fatal("incomplete v4 profile")
		}
	}
}

// These vectors specify receiver expectations for Android; checking their
// consistency here is not evidence that an Android receiver already exists.
func TestSharedSnapshotReceptionAndRuntimeExamples(t *testing.T) {
	read := func(name string, target any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(fixtureRoot(t), "client-integration", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	var cases []struct {
		Name    string
		Valid   bool
		Context struct {
			DeploymentID string
			Generation   int
			Etag         string
		}
		Snapshot clientapi.ManagedSnapshot
	}
	read("snapshot-reception-cases.json", &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			hash := c.Snapshot.SnapshotHash
			valid := c.Context.DeploymentID == c.Snapshot.DeploymentId && c.Context.Generation == c.Snapshot.ManagedGeneration && c.Context.Etag == "\""+hash+"\""
			if valid != c.Valid {
				t.Fatal("reception outcome mismatch")
			}
		})
	}
	var snapshot clientapi.ManagedSnapshot
	read("snapshot-v4.json", &snapshot)
	var asrSnapshot clientapi.ManagedSnapshot
	read("snapshot-v4-asr.json", &asrSnapshot)
	var examples []struct {
		ResourceID, Protocol, Method, URL, ContentType, ResponseKind string
		Headers                                                      map[string]string
		Body, Fields                                                 map[string]any
	}
	read("runtime-examples.json", &examples)
	resources := map[string]string{}
	resources[snapshot.Models[0].ModelId] = snapshot.Models[0].RuntimePath
	resources[(*snapshot.ImageGenerators)[0].ImageId] = (*snapshot.ImageGenerators)[0].RuntimePath
	resources[snapshot.Tts[0].TtsId] = snapshot.Tts[0].RuntimePath
	resources[snapshot.Asr[0].AsrId] = snapshot.Asr[0].RuntimePath
	resources[asrSnapshot.Asr[1].AsrId] = asrSnapshot.Asr[1].RuntimePath
	resources[snapshot.Mcp[0].McpServerId] = snapshot.Mcp[0].RuntimePath
	if len(examples) != 6 {
		t.Fatal("missing Runtime profile")
	}
	for _, example := range examples {
		parsed, err := url.Parse(example.URL)
		if err != nil {
			t.Fatal(err)
		}
		path, ok := resources[example.ResourceID]
		if !ok || parsed.Scheme != "https" || parsed.Host != "platform.example.invalid" || parsed.Path != "/runtime/v1/resources/"+example.ResourceID+path || example.Method != "POST" {
			t.Fatalf("invalid public runtime request: %+v", example)
		}
		if example.Headers["X-Measix-Managed-Generation"] != fmt.Sprint(snapshot.ManagedGeneration) || example.Headers["Authorization"] != "Bearer synthetic.access.token" || example.Headers["X-Measix-Interaction-Id"] == "" {
			t.Fatal("invalid Runtime auth/generation")
		}
		if example.ResourceID == snapshot.Models[0].ModelId && example.Body["model"] != snapshot.Models[0].UpstreamModelKey {
			t.Fatal("wire ID used as model key")
		}
		if example.ResourceID == (*snapshot.ImageGenerators)[0].ImageId && example.Body["model"] != (*snapshot.ImageGenerators)[0].UpstreamModelKey {
			t.Fatal("image wire ID used as upstream model key")
		}
		delete(resources, example.ResourceID)
	}
	if len(resources) != 0 {
		t.Fatal("Runtime profile coverage incomplete")
	}
	// Load the copied schema directly: all refs must resolve in the export.
	path := filepath.Join(fixtureRoot(t), "../generated/android/integration/measix-platform-core/api/generated/android/client-control.openapi.yaml")
	doc, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Components.Schemas["ManagedSnapshot"] == nil {
		t.Fatal("missing exported snapshot schema")
	}
}
