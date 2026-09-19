package capability_test

import (
	"bytes"
	"context"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestModelProtocolsSurviveDraftValidationAndSnapshot(t *testing.T) {
	for _, protocol := range []string{"OPENAI_CHAT_COMPLETIONS", "OPENAI_RESPONSES", "GOOGLE_GENERATE_CONTENT", "ANTHROPIC_MESSAGES"} {
		t.Run(protocol, func(t *testing.T) {
			ctx := context.Background()
			st, boot, now := bootstrapI2(t)
			box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
			if err != nil {
				t.Fatal(err)
			}
			ups := upstream.NewService(st.Client, box)
			secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "test", "synthetic-model-key")
			if err != nil {
				t.Fatal(err)
			}
			up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
			if err != nil {
				t.Fatal(err)
			}
			content := validDraft(up.UpstreamID)
			content.Providers[0].ClientProtocol = adminapi.ProviderDefinitionClientProtocol(protocol)
			path := map[string]string{"OPENAI_CHAT_COMPLETIONS": "/v1/chat/completions", "OPENAI_RESPONSES": "/v1/responses", "GOOGLE_GENERATE_CONTENT": "/v1beta/models/gemini-test:streamGenerateContent", "ANTHROPIC_MESSAGES": "/v1/messages"}[protocol]
			content.Models[0].RuntimePath = path
			content.Bindings[0].AllowedPathPrefixes = []string{path}
			service := capability.NewService(st.Client)
			draft, err := service.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := service.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
			if err != nil {
				t.Fatal(err)
			}
			validation, err := service.ValidateDraft(ctx, saved.DraftRevision)
			if err != nil || !validation.Valid {
				t.Fatalf("protocol rejected: %+v err=%v", validation, err)
			}
			snapshot, _, err := service.CompileSnapshot(capability.SnapshotInput{DeploymentID: platformid.New(platformid.Deployment), ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: saved.Content, PublishedAt: now})
			if err != nil {
				t.Fatal(err)
			}
			if string(snapshot.Providers[0].ClientProtocol) != protocol || snapshot.Models[0].RuntimePath != content.Models[0].RuntimePath {
				t.Fatal("protocol or endpoint changed during projection")
			}
			if !snapshot.Providers[0].ClientProtocol.Valid() {
				t.Fatal("client contract cannot consume selected protocol")
			}
		})
	}
}
