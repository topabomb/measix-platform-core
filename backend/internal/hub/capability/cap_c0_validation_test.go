package capability_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

// CAP-C0-004: enabled Managed TTS without voice must fail validation.
func TestCAPC0004TTSVoiceRequired(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "super-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	ttsID := platformid.New(platformid.TTS)
	ttsModelKey := "tts-1"
	content.Tts = []adminapi.TtsDefinition{{
		TtsId: ttsID, DisplayName: "Managed TTS", ClientProtocol: adminapi.OPENAIAUDIOSPEECH,
		UpstreamModelKey: &ttsModelKey, RuntimePath: "/v1/audio/speech", Enabled: true,
		// voice intentionally empty — must be a validation error per CAP-C0-004
	}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: ttsID, UpstreamId: up.UpstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/speech"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPREQUESTRESPONSE,
	})
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("validation should fail for enabled TTS without voice, result=%+v", result)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Path, "tts") && strings.Contains(strings.ToLower(e.Code), "voice") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a voice-related validation error for TTS, got %+v", result.Errors)
	}
}

// CAP-C0-006: MCP auth ownership must be ENTERPRISE_MANAGED or NONE.
// A MCP definition without authOwnership (empty string) must fail validation.
func TestCAPC0006MCPAuthOwnershipValidation(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "super-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }

	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	mcpID := platformid.New(platformid.MCP)
	content.Mcp = []adminapi.McpDefinition{{
		McpServerId: mcpID, DisplayName: "Managed MCP", ClientProtocol: adminapi.McpDefinitionClientProtocol("MCP_STREAMABLE_HTTP"),
		RuntimePath: "/mcp", Enabled: true,
		// authOwnership intentionally empty — must be a validation error per CAP-C0-006
	}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: mcpID, UpstreamId: up.UpstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPSTREAMINGSSE,
	})
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("validation should fail for MCP without authOwnership, result=%+v", result)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Path, "mcp") && strings.Contains(strings.ToLower(e.Code), "auth") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an authOwnership validation error for MCP, got %+v", result.Errors)
	}
}
