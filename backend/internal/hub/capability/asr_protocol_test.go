package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestASRProtocolSettingsValidateAndSurviveProjection(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, fields string
		valid                  bool
	}{
		{"http", "OPENAI_AUDIO_TRANSCRIPTIONS", ``, true},
		{"dash-http", "DASHSCOPE_HTTP_ASR", ``, true},
		{"dash-http-wrong-transport", "DASHSCOPE_HTTP_ASR", ``, false},
		{"dash-http-realtime-fields", "DASHSCOPE_HTTP_ASR", `,"sampleRate":16000`, false},
		{"openai", "OPENAI_REALTIME_TRANSCRIPTION", `,"sampleRate":24000,"vadThreshold":0.5,"silenceDurationMs":500,"prefixPaddingMs":300,"prompt":"technical words"`, true},
		{"dashscope", "DASHSCOPE_REALTIME_ASR", `,"sampleRate":16000,"vadThreshold":0,"silenceDurationMs":400`, true},
		{"wrong-transport", "DASHSCOPE_REALTIME_ASR", `,"sampleRate":16000,"vadThreshold":0,"silenceDurationMs":400`, false},
		{"http-realtime-fields", "OPENAI_AUDIO_TRANSCRIPTIONS", `,"sampleRate":24000`, false},
		{"realtime-missing-settings", "OPENAI_REALTIME_TRANSCRIPTION", ``, false},
		{"openai-wrong-rate", "OPENAI_REALTIME_TRANSCRIPTION", `,"sampleRate":16000,"vadThreshold":0.5,"silenceDurationMs":500,"prefixPaddingMs":300`, false},
		{"dashscope-openai-field", "DASHSCOPE_REALTIME_ASR", `,"sampleRate":16000,"vadThreshold":0,"silenceDurationMs":400,"prefixPaddingMs":300`, false},
		{"invalid-vad", "DASHSCOPE_REALTIME_ASR", `,"sampleRate":16000,"vadThreshold":2,"silenceDurationMs":400`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, boot, now := bootstrapI2(t)
			box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
			if err != nil {
				t.Fatal(err)
			}
			ups := upstream.NewService(st.Client, box)
			secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "asr-test", "synthetic-asr-key")
			if err != nil {
				t.Fatal(err)
			}
			up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
			if err != nil {
				t.Fatal(err)
			}
			content := validDraft(up.UpstreamID)
			id := platformid.New(platformid.ASR)
			var definition adminapi.AsrDefinition
			if err := json.Unmarshal([]byte(`{"asrId":"`+id+`","displayName":"ASR test","enabled":true,"clientProtocol":"`+tc.protocol+`","upstreamModelKey":"asr-test","runtimePath":"/transcribe"`+tc.fields+`}`), &definition); err != nil {
				t.Fatal(err)
			}
			content.Asr = []adminapi.AsrDefinition{definition}
			content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{ResourceId: id, RuntimeRouteId: platformid.New(platformid.Route), UpstreamId: up.UpstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPMULTIPART})
			if tc.protocol == "DASHSCOPE_HTTP_ASR" && tc.name != "dash-http-wrong-transport" {
				content.Bindings[len(content.Bindings)-1].TransportPolicy = adminapi.RuntimeBindingDefinitionTransportPolicyHTTPREQUESTRESPONSE
			}
			if (tc.protocol == "OPENAI_REALTIME_TRANSCRIPTION" || tc.protocol == "DASHSCOPE_REALTIME_ASR") && tc.name != "wrong-transport" {
				binding := &content.Bindings[len(content.Bindings)-1]
				binding.TransportPolicy = adminapi.RuntimeBindingDefinitionTransportPolicyWEBSOCKET
				binding.AllowedMethods = []string{"GET"}
			}
			service := capability.NewService(st.Client)
			draft, err := service.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := service.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.ValidateDraft(ctx, saved.DraftRevision)
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid != tc.valid {
				t.Fatalf("valid=%v errors=%+v", result.Valid, result.Errors)
			}
			if !tc.valid {
				return
			}
			snapshot, _, err := service.CompileSnapshot(capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: saved.Content, PublishedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(definition)
			after, _ := json.Marshal(snapshot.Asr[0])
			if !bytes.Equal(before, after) {
				t.Fatalf("projection changed ASR fields: %s -> %s", before, after)
			}
		})
	}
}
