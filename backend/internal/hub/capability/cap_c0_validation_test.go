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
	content.Tts = []adminapi.TtsDefinition{{
		TtsId: ttsID, DisplayName: "Managed TTS", ClientProtocol: adminapi.TtsDefinitionClientProtocolOPENAIAUDIOSPEECH,
		UpstreamModelKey: "tts-1", RuntimePath: "/v1/audio/speech", Enabled: true,
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
		AllowedMethods: []string{"POST", "GET", "DELETE"}, AllowedPathPrefixes: []string{"/mcp"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPSTREAMINGSSE,
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

// CAP-C0-007: enabled Managed TTS without upstreamModelKey must fail validation.
func TestCAPC0007TTSUpstreamModelKeyRequired(t *testing.T) {
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
	content.Tts = []adminapi.TtsDefinition{{
		TtsId: ttsID, DisplayName: "Managed TTS", ClientProtocol: adminapi.TtsDefinitionClientProtocolOPENAIAUDIOSPEECH,
		UpstreamModelKey: "", Voice: "alloy", RuntimePath: "/v1/audio/speech", Enabled: true,
		// upstreamModelKey intentionally empty — must be a validation error per CAP-C0-007
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
		t.Fatalf("validation should fail for TTS without upstreamModelKey, result=%+v", result)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Path, "tts") && strings.Contains(strings.ToLower(e.Code), "model_key") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an upstreamModelKey validation error for TTS, got %+v", result.Errors)
	}
}

// CAP-C0-008: enabled Managed ASR without upstreamModelKey must fail validation.
func TestCAPC0008ASRUpstreamModelKeyRequired(t *testing.T) {
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
	asrID := platformid.New(platformid.ASR)
	content.Asr = []adminapi.AsrDefinition{{
		AsrId: asrID, DisplayName: "Managed ASR", ClientProtocol: adminapi.AsrDefinitionClientProtocol("OPENAI_AUDIO_TRANSCRIPTIONS"),
		UpstreamModelKey: "", RuntimePath: "/v1/audio/transcriptions", Enabled: true,
		// upstreamModelKey intentionally empty — must be a validation error per CAP-C0-008
	}}
	content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{
		RuntimeRouteId: platformid.New(platformid.Route), ResourceId: asrID, UpstreamId: up.UpstreamID,
		AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/audio/transcriptions"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPMULTIPART,
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
		t.Fatalf("validation should fail for ASR without upstreamModelKey, result=%+v", result)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Path, "asr") && strings.Contains(strings.ToLower(e.Code), "model_key") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an upstreamModelKey validation error for ASR, got %+v", result.Errors)
	}
}

func TestCurrentResourceTextAndModalitiesAreStrictlyValidated(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft("ups_00000000-0000-4000-8000-000000000001")
	content.Models[0].DisplayName = "  "
	content.Models[0].UpstreamModelKey = ""
	content.Models[0].InputModalities = nil
	content.Models[0].OutputModalities = []adminapi.ModelDefinitionOutputModalities{}
	content.Tts = []adminapi.TtsDefinition{{TtsId: platformid.New(platformid.TTS), DisplayName: " ", ClientProtocol: adminapi.TtsDefinitionClientProtocolOPENAIAUDIOSPEECH, UpstreamModelKey: "tts", Voice: "alloy", RuntimePath: "/tts"}}
	language := " "
	content.Asr = []adminapi.AsrDefinition{{AsrId: platformid.New(platformid.ASR), DisplayName: " ", ClientProtocol: adminapi.AsrDefinitionClientProtocolOPENAIAUDIOTRANSCRIPTIONS, UpstreamModelKey: "asr", Language: &language, RuntimePath: "/asr"}}
	content.Mcp = []adminapi.McpDefinition{{McpServerId: platformid.New(platformid.MCP), DisplayName: " ", ClientProtocol: adminapi.McpDefinitionClientProtocolMCPSTREAMABLEHTTP, AuthOwnership: adminapi.McpDefinitionAuthOwnershipNONE, RuntimePath: "/mcp"}}
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"missing_display_name:MODEL":      false,
		"missing_model_key:MODEL":         false,
		"missing_input_modalities:MODEL":  false,
		"missing_output_modalities:MODEL": false,
		"missing_display_name:TTS":        false,
		"missing_display_name:ASR":        false,
		"empty_language:ASR":              false,
		"missing_display_name:MCP":        false,
	}
	for _, issue := range result.Errors {
		if issue.ResourceKind == nil || issue.Field == nil || issue.ResourceId == nil {
			continue
		}
		key := issue.Code + ":" + string(*issue.ResourceKind)
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing structured issue %s in %+v", key, result.Errors)
		}
	}
}

// CAP-C2-041: default resource reference must point to an enabled resource.
// A disabled model referenced by defaultModelId must fail validation.
func TestCAPC2041DefaultMustReferenceEnabled(t *testing.T) {
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
	// Make the model disabled but keep it as defaultModelId — must fail validation.
	content.Models[0].Enabled = false
	// Remove the binding requirement by also removing the binding for the disabled model.
	// Since the model is disabled, it doesn't need a binding, but the defaultModelId must still be valid.
	for i, b := range content.Bindings {
		if b.ResourceId == content.Models[0].ModelId {
			content.Bindings = append(content.Bindings[:i], content.Bindings[i+1:]...)
			break
		}
	}
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatalf("validation should fail for defaultModelId pointing to disabled model, result=%+v", result)
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Code, "invalid_default_model") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected invalid_default_model error, got %+v", result.Errors)
	}
}
