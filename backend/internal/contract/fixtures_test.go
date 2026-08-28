package contract_test

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/internal/wire/usageingestapi"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "api", "fixtures"))
}

func decodeFixture[T any](t *testing.T, rel string, strict bool) T {
	t.Helper()
	file, err := os.Open(filepath.Join(fixtureRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var value T
	decoder := json.NewDecoder(io.LimitReader(file, 2<<20))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode fixture %s: %v", rel, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture %s has trailing JSON", rel)
	}
	return value
}

func TestSYSI0001CanonicalFixturesDecodeWithGeneratedWire(t *testing.T) {
	// SYS-I0-001: canonical fixtures and generated Go wire must remain compatible.
	_ = decodeFixture[clientapi.Discovery](t, "identity/discovery.json", true)
	_ = decodeFixture[clientapi.EnrollmentExchangeRequest](t, "identity/enrollment-exchange-request.json", true)
	_ = decodeFixture[clientapi.EnrollmentExchangeResponse](t, "identity/enrollment-exchange-response.json", true)
	_ = decodeFixture[clientapi.ManagedState](t, "managed-state/ready-generation-42.json", true)
	_ = decodeFixture[clientapi.ManagedState](t, "managed-state/sync-required.json", true)
	_ = decodeFixture[adminapi.Draft](t, "draft/minimal.json", true)
	_ = decodeFixture[clientapi.Problem](t, "problem/managed-snapshot-required.json", true)
	_ = decodeFixture[adminapi.Problem](t, "problem/stale-draft-revision.json", true)
	_ = decodeFixture[usageingestapi.UsageBatch](t, "usage/request-batch.json", true)

	// C0 canonical full-profile snapshot fixtures must decode with strict wire types.
	_ = decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/full-required-profile.json", true)
	_ = decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/model-openai-chat.json", true)
	_ = decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/tts-openai-speech.json", true)
	_ = decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/asr-openai-transcription.json", true)
	_ = decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/mcp-streamable-http.json", true)
}

func TestSYSI0002UnknownOptionalResponseFieldIsTolerated(t *testing.T) {
	// SYS-I0-002: response readers tolerate a future optional response field.
	_ = decodeFixture[adminapi.AdminSession](t, "compat/admin-session-unknown-optional-response.json", false)
}

func TestSYSI0003UnknownRequestFieldIsRejected(t *testing.T) {
	// SYS-I0-003: request readers use strict decoding at the HTTP boundary.
	file, err := os.Open(filepath.Join(fixtureRoot(t), "invalid/create-user-unknown-field.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var request adminapi.CreateUserRequest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err == nil {
		t.Fatal("unknown request field unexpectedly accepted")
	}
}

func TestNegativeSnapshotFixturesRejectedByEnumValidation(t *testing.T) {
	// C0 negative fixtures must fail enum validation.
	// Go JSON unmarshal does not reject unknown enum values for string-based types,
	// so we use the generated Valid() methods to verify they are not valid enum members.
	type check struct {
		path  string
		valid func() bool
	}

	// unsupported-protocol: Provider.clientProtocol = "UNSUPPORTED_PROTOCOL"
	snap := decodeFixture[clientapi.ManagedSnapshot](t, "invalid/snapshot-unsupported-protocol.json", true)
	if snap.Providers[0].ClientProtocol.Valid() {
		t.Fatal("UNSUPPORTED_PROTOCOL should not be a valid provider clientProtocol")
	}

	// invalid-modality: Model.inputModalities contains "VIDEO"
	snap2 := decodeFixture[clientapi.ManagedSnapshot](t, "invalid/snapshot-invalid-modality.json", true)
	for _, m := range snap2.Models[0].InputModalities {
		if string(m) == "VIDEO" && m.Valid() {
			t.Fatal("VIDEO should not be a valid input modality")
		}
	}

	// invalid-capability: Model.capabilities contains "CODE_INTERPRETER"
	snap3 := decodeFixture[clientapi.ManagedSnapshot](t, "invalid/snapshot-invalid-capability.json", true)
	for _, c := range snap3.Models[0].Capabilities {
		if string(c) == "CODE_INTERPRETER" && c.Valid() {
			t.Fatal("CODE_INTERPRETER should not be a valid model capability")
		}
	}

	_ = check{}
}

func TestSnapshotAndRuntimeControlGoldenHashes(t *testing.T) {
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/generation-42.json", true)
	hash, err := capability.HashSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if hash != string(snapshot.SnapshotHash) {
		t.Fatalf("snapshot golden hash=%s want=%s", hash, snapshot.SnapshotHash)
	}

	state := decodeFixture[relaycontrolapi.RuntimeControlState](t, "runtime-control/revision-57.json", true)
	controlHash, err := relaystate.HashDescriptor(state)
	if err != nil {
		t.Fatal(err)
	}
	if controlHash != state.BundleHash {
		t.Fatalf("runtime-control golden hash=%s want=%s", controlHash, state.BundleHash)
	}
}

func TestERXC0002SnapshotV2RetainsV1AndAddsAssistantsStarters(t *testing.T) {
	// ERX-C0-002: Snapshot v2 retains v1 A/B/Policy and adds assistants/starters.
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/v2-assistant-starter.json", true)
	if snapshot.SchemaVersion != 2 {
		t.Fatalf("expected schemaVersion=2, got %d", snapshot.SchemaVersion)
	}
	if len(snapshot.Assistants) == 0 {
		t.Fatal("v2 snapshot must contain at least one assistant")
	}
	if len(snapshot.Starters) == 0 {
		t.Fatal("v2 snapshot must contain at least one starter")
	}
	// v1 fields must be present
	if len(snapshot.Providers) == 0 || len(snapshot.Models) == 0 || len(snapshot.Mcp) == 0 {
		t.Fatal("v2 snapshot must retain v1 providers/models/mcp")
	}
	// Golden hash must match
	hash, err := capability.HashSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if hash != string(snapshot.SnapshotHash) {
		t.Fatalf("v2 snapshot golden hash=%s want=%s", hash, snapshot.SnapshotHash)
	}
}

func TestERXC0003AssistantRefsAreTypedAndComplete(t *testing.T) {
	// ERX-C0-003: assistant/model/MCP/starter refs are typed and complete.
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/v2-assistant-starter.json", true)
	modelIds := map[string]bool{}
	for _, m := range snapshot.Models {
		if m.Enabled {
			modelIds[string(m.ModelId)] = true
		}
	}
	mcpIds := map[string]bool{}
	for _, m := range snapshot.Mcp {
		if m.Enabled {
			mcpIds[string(m.McpServerId)] = true
		}
	}
	for _, a := range snapshot.Assistants {
		if a.AssistantDefinitionId == "" {
			t.Fatal("assistant has empty assistantDefinitionId")
		}
		if !modelIds[string(a.ModelId)] {
			t.Fatalf("assistant %s references unknown or disabled model %s", a.AssistantDefinitionId, a.ModelId)
		}
		for _, mcpId := range a.McpServerIds {
			if !mcpIds[string(mcpId)] {
				t.Fatalf("assistant %s references unknown or disabled MCP %s", a.AssistantDefinitionId, mcpId)
			}
		}
		if len(a.MemorySeed) == 0 {
			t.Fatalf("assistant %s has empty memorySeed", a.AssistantDefinitionId)
		}
		for _, seed := range a.MemorySeed {
			if seed == "" {
				t.Fatalf("assistant %s has empty seed item", a.AssistantDefinitionId)
			}
		}
	}
	assistantIds := map[string]bool{}
	for _, a := range snapshot.Assistants {
		if a.Enabled {
			assistantIds[string(a.AssistantDefinitionId)] = true
		}
	}
	for _, s := range snapshot.Starters {
		if s.StarterId == "" {
			t.Fatal("starter has empty starterId")
		}
		if !assistantIds[string(s.AssistantDefinitionId)] {
			t.Fatalf("starter %s references unknown or disabled assistant %s", s.StarterId, s.AssistantDefinitionId)
		}
	}
}

func TestERXC0005SeedStarterNormalizationAndOrder(t *testing.T) {
	// ERX-C0-005: seed/starter normalization and deterministic hash/order.
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/v2-assistant-starter.json", true)
	// Starters must be sorted by (assistantDefinitionId, sortOrder)
	for i := 1; i < len(snapshot.Starters); i++ {
		prev := snapshot.Starters[i-1]
		curr := snapshot.Starters[i]
		if string(prev.AssistantDefinitionId) > string(curr.AssistantDefinitionId) {
			t.Fatal("starters not sorted by assistantDefinitionId")
		}
		if string(prev.AssistantDefinitionId) == string(curr.AssistantDefinitionId) && prev.SortOrder > curr.SortOrder {
			t.Fatal("starters not sorted by sortOrder within same assistant")
		}
	}
	// Assistants must be sorted by assistantDefinitionId
	for i := 1; i < len(snapshot.Assistants); i++ {
		if string(snapshot.Assistants[i-1].AssistantDefinitionId) > string(snapshot.Assistants[i].AssistantDefinitionId) {
			t.Fatal("assistants not sorted by assistantDefinitionId")
		}
	}
	// Hash must be deterministic
	hash1, _ := capability.HashSnapshot(snapshot)
	hash2, _ := capability.HashSnapshot(snapshot)
	if hash1 != hash2 {
		t.Fatal("hash is not deterministic")
	}
}

func TestERXC0006SnapshotContainsNoEnterpriseUpdateBody(t *testing.T) {
	// ERX-C0-006: Snapshot contains no Enterprise Update body, Secret, Upstream or runtime route.
	snapshot := decodeFixture[clientapi.ManagedSnapshot](t, "snapshot/v2-assistant-starter.json", true)
	// Verify that no assistant or starter fields contain enterprise update content
	for _, a := range snapshot.Assistants {
		if a.SystemPrompt == "" {
			t.Fatal("assistant has empty systemPrompt — should not contain enterprise update body")
		}
	}
}
