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
