package agentspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestResourcesValidateIdentityAndPreservePartialObservations(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/workspace/resources.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]any
	json.Unmarshal(raw, &fixture)
	for _, key := range []string{"runtime", "disk"} {
		fixture[key].(map[string]any)["observedAt"] = time.Now().UnixMilli()
	}
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-management" {
			t.Error("unexpected request")
		}
		json.NewEncoder(w).Encode(map[string]any{"username": "alice", "agentSpaceId": fixture["agentSpaceId"], "mcpUrl": origin + "/u/alice/mcp", "resources": fixture})
	}))
	defer server.Close()
	origin = server.URL
	c, _ := New(origin, origin, "", "test-management")
	space := fixture["agentSpaceId"].(string)
	got, err := c.Resources(context.Background(), "alice", space)
	if err != nil || got.Disk.Value == nil || got.Disk.Value.UsedBytes != 0 || got.Memory.Status != "unavailable" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = c.Resources(context.Background(), "alice", "spc_550e8400-e29b-41d4-a716-446655440001"); err == nil {
		t.Fatal("accepted wrong space")
	}
	fixture["memory"].(map[string]any)["status"] = "current"
	got, err = c.Resources(context.Background(), "alice", space)
	if err != nil || got.Memory.Status != "error" || got.Disk.Status != "current" {
		t.Fatalf("invalid metric should not hide valid disk: %+v %v", got, err)
	}
	fixture["runtime"].(map[string]any)["reason"] = "Bearer sensitive-remote-detail"
	got, err = c.Resources(context.Background(), "alice", space)
	if err != nil || got.Runtime.Reason != nil && *got.Runtime.Reason == "Bearer sensitive-remote-detail" {
		t.Fatal("unsafe reason")
	}
	for _, field := range []string{"value", "observedAt", "reason"} {
		t.Run("malformed memory "+field, func(t *testing.T) {
			original := fixture["memory"]
			defer func() { fixture["memory"] = original }()
			fixture["memory"] = map[string]any{"status": "current", "value": 1024, "observedAt": time.Now().UnixMilli(), "reason": nil}
			fixture["memory"].(map[string]any)[field] = []string{"wrong type"}
			got, err := c.Resources(context.Background(), "alice", space)
			if err != nil || got.Memory.Status != "error" || got.Memory.Value != nil || got.Disk.Status != "current" || got.Runtime.Status != "current" {
				t.Fatalf("one malformed metric must not discard valid observations: %+v %v", got, err)
			}
		})
	}
	fixture["allocation"].(map[string]any)["cpuCores"].(map[string]any)["value"] = "wrong type"
	got, err = c.Resources(context.Background(), "alice", space)
	if err != nil || got.Allocation.CpuCores.Source != "unknown" || got.Allocation.CpuCores.Value != nil || got.Allocation.MemoryLimitBytes.Value == nil || got.Disk.Status != "current" {
		t.Fatalf("one malformed allocation must not discard other values: %+v %v", got, err)
	}
}
