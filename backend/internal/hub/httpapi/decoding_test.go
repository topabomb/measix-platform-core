package httpapi

import (
	"measix/platform/internal/hub/identity"
	"net/http/httptest"
	"strings"
	"testing"
)

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
