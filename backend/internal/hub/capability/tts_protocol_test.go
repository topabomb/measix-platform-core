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

func TestTTSProtocolsValidateAndProjectTheirOwnFields(t *testing.T) {
	cases := []struct {
		name, fields string
		local, valid bool
	}{
		{"openai", `"clientProtocol":"OPENAI_AUDIO_SPEECH","upstreamModelKey":"tts-test","voice":"alloy","runtimePath":"/v1/audio/speech"`, false, true},
		{"gemini", `"clientProtocol":"GEMINI_GENERATE_CONTENT_TTS","upstreamModelKey":"gemini-test","voice":"Kore","runtimePath":"/v1beta/models/gemini-test:generateContent"`, false, true},
		{"mimo", `"clientProtocol":"MIMO_CHAT_COMPLETIONS_TTS","upstreamModelKey":"mimo-v2.5-tts","voice":"mimo_default","voiceDesignPrompt":"Warm voice","runtimePath":"/v1/chat/completions"`, false, true},
		{"mimo-design", `"clientProtocol":"MIMO_CHAT_COMPLETIONS_TTS","upstreamModelKey":"mimo-v2.5-tts-voicedesign","voiceDesignPrompt":"Warm voice","runtimePath":"/v1/chat/completions"`, false, true},
		{"system", `"clientProtocol":"SYSTEM_TTS","speechRate":1.2,"pitch":0.9`, true, true},
		{"system-with-binding", `"clientProtocol":"SYSTEM_TTS","speechRate":1,"pitch":1`, false, false},
		{"openai-design-prompt", `"clientProtocol":"OPENAI_AUDIO_SPEECH","upstreamModelKey":"tts-test","voice":"alloy","runtimePath":"/v1/audio/speech","voiceDesignPrompt":"Warm"`, false, false},
		{"system-cloud-fields", `"clientProtocol":"SYSTEM_TTS","speechRate":1,"pitch":1,"voice":"alloy","runtimePath":"/v1/audio/speech","upstreamModelKey":"fake"`, true, false},
		{"system-missing-settings", `"clientProtocol":"SYSTEM_TTS"`, true, false},
		{"system-invalid-rate", `"clientProtocol":"SYSTEM_TTS","speechRate":0,"pitch":1`, true, false},
		{"cloud-system-settings", `"clientProtocol":"OPENAI_AUDIO_SPEECH","upstreamModelKey":"tts-test","voice":"alloy","runtimePath":"/v1/audio/speech","pitch":1`, false, false},
		{"design-missing-prompt", `"clientProtocol":"MIMO_CHAT_COMPLETIONS_TTS","upstreamModelKey":"mimo-v2.5-tts-voicedesign","runtimePath":"/v1/chat/completions"`, false, false},
		{"design-with-voice", `"clientProtocol":"MIMO_CHAT_COMPLETIONS_TTS","upstreamModelKey":"mimo-v2.5-tts-voicedesign","voiceDesignPrompt":"Warm","voice":"mimo_default","runtimePath":"/v1/chat/completions"`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			st, boot, now := bootstrapI2(t)
			box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
			if err != nil {
				t.Fatal(err)
			}
			ups := upstream.NewService(st.Client, box)
			secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "tts-test", "synthetic-tts-key")
			if err != nil {
				t.Fatal(err)
			}
			up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
			if err != nil {
				t.Fatal(err)
			}
			content := validDraft(up.UpstreamID)
			id := platformid.New(platformid.TTS)
			var definition adminapi.TtsDefinition
			if err := json.Unmarshal([]byte(`{"ttsId":"`+id+`","displayName":"Test voice","enabled":true,`+tc.fields+`}`), &definition); err != nil {
				t.Fatal(err)
			}
			content.Tts = []adminapi.TtsDefinition{definition}
			content.Policy.DefaultTtsId = &id
			if !tc.local {
				content.Bindings = append(content.Bindings, adminapi.RuntimeBindingDefinition{RuntimeRouteId: platformid.New(platformid.Route), ResourceId: id, UpstreamId: up.UpstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPSTREAMINGSSE})
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
				t.Fatalf("valid=%v want=%v errors=%+v", result.Valid, tc.valid, result.Errors)
			}
			if !tc.valid {
				return
			}
			snapshot, _, err := service.CompileSnapshot(capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: saved.Content, PublishedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(definition)
			after, _ := json.Marshal(snapshot.Tts[0])
			if !bytes.Equal(before, after) {
				t.Fatalf("TTS projection lost fields: %s -> %s", before, after)
			}
			if tc.local && bytes.Contains(after, []byte("runtimePath")) {
				t.Fatal("system TTS carries a fake cloud endpoint")
			}
		})
	}
}
