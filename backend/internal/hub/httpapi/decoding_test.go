package httpapi

import (
	"encoding/json"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/wire/adminapi"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDraftWriteRequiresExplicitBooleanPermissions(t *testing.T) {
	fields := []string{"allowLocalProviders", "allowLocalTts", "allowLocalAsr", "allowLocalMcp", "allowLocalAssistants"}
	for _, field := range fields {
		for _, invalid := range []string{"missing", "null"} {
			t.Run(field+"/"+invalid, func(t *testing.T) {
				policy := map[string]any{}
				for _, name := range fields {
					policy[name] = false
				}
				if invalid == "missing" {
					delete(policy, field)
				} else {
					policy[field] = nil
				}
				payload, err := json.Marshal(map[string]any{"content": map[string]any{"policy": policy}})
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest("PUT", "/api/admin/v1/draft", strings.NewReader(string(payload)))
				var body adminapi.PutDraftRequest
				if err := decodeStrictJSON(r, &body); err == nil {
					t.Fatal("missing/null permission silently treated as false")
				}
			})
		}
	}
}

func TestDraftWriteRequiresAllCollections(t *testing.T) {
	for _, field := range []string{"providers", "models", "tts", "asr", "mcp", "bindings", "assistants", "starters"} {
		for _, invalid := range []string{"missing", "null"} {
			t.Run(field+"/"+invalid, func(t *testing.T) {
				content := map[string]any{
					"providers": []any{}, "models": []any{}, "tts": []any{}, "asr": []any{},
					"mcp": []any{}, "bindings": []any{}, "assistants": []any{}, "starters": []any{},
					"policy": map[string]any{"policyId": "pol_00000000-0000-4000-8000-000000000001", "allowLocalProviders": false,
						"allowLocalTts": false, "allowLocalAsr": false, "allowLocalMcp": false, "allowLocalAssistants": false},
				}
				if invalid == "missing" {
					delete(content, field)
				} else {
					content[field] = nil
				}
				payload, err := json.Marshal(map[string]any{"expectedDraftRevision": 1, "content": content})
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest("PUT", "/api/admin/v1/draft", strings.NewReader(string(payload)))
				var body adminapi.PutDraftRequest
				if err := decodeStrictJSON(r, &body); err == nil {
					t.Fatal("missing/null draft collection accepted")
				}
			})
		}
	}
}

func TestStrictBodyRejectsOversizeWhitespace(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader("{}"+strings.Repeat(" ", 1<<20)))
	var body map[string]any
	if err := decodeStrictJSON(r, &body); err == nil {
		t.Fatal("oversized body accepted")
	}
}
func TestManagedStateHasNoZeroTargetGeneration(t *testing.T) {
	wire := managedStateWire(identity.ManagedStateView{ActiveManagedGeneration: 0}, nil)
	if wire.TargetManagedGeneration != nil {
		t.Fatal("zero is not a valid target generation")
	}
}
