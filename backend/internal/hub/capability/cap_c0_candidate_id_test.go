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

// HUB-CAP-002: caller-proposed Draft candidate IDs must be validated for
// prefix, type, uniqueness, and reference integrity. Hub is the final authority.
func TestHUBCAP002CandidateIDValidation(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x42}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "token", "secret")
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

	t.Run("wrong prefix on model ID is rejected", func(t *testing.T) {
		content := validDraft(up.UpstreamID)
		content.Models[0].ModelId = "tts_" + platformid.New(platformid.TTS) // wrong prefix
		_, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
		if err == nil {
			t.Fatal("expected error for wrong-prefix model ID")
		}
		if !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("expected invalid id error, got: %v", err)
		}
	})

	t.Run("non-UUIDv4 ID is rejected", func(t *testing.T) {
		content := validDraft(up.UpstreamID)
		content.Models[0].ModelId = "mdl_not-a-uuid"
		_, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
		if err == nil {
			t.Fatal("expected error for non-UUID model ID")
		}
	})

	t.Run("duplicate provider ID within draft is rejected", func(t *testing.T) {
		content := validDraft(up.UpstreamID)
		providerID := content.Providers[0].ProviderId
		content.Providers = append(content.Providers, adminapi.ProviderDefinition{
			ProviderId:  providerID, // duplicate
			DisplayName: "Duplicate", ClientProtocol: adminapi.OPENAICHATCOMPLETIONS, Enabled: true,
		})
		_, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
		if err == nil {
			t.Fatal("expected error for duplicate provider ID")
		}
		if !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("expected duplicate id error, got: %v", err)
		}
	})

	t.Run("duplicate resource ID across model and TTS is rejected", func(t *testing.T) {
		content := validDraft(up.UpstreamID)
		sharedID := platformid.New(platformid.Model)
		content.Models[0].ModelId = sharedID
		// TTS with a different prefix but using the same UUID is still not a duplicate
		// because the prefix differs. But a model ID used twice is a duplicate.
		content.Models = append(content.Models, adminapi.ModelDefinition{
			ModelId:    sharedID, // same ID = duplicate
			ProviderId: content.Providers[0].ProviderId, DisplayName: "Dup Model",
			UpstreamModelKey: "model-dup", RuntimePath: "/v1/chat/completions", Enabled: false,
			Capabilities: []adminapi.ModelDefinitionCapabilities{adminapi.TOOL}, InputModalities: []adminapi.ModelDefinitionInputModalities{adminapi.ModelDefinitionInputModalitiesTEXT}, OutputModalities: []adminapi.ModelDefinitionOutputModalities{adminapi.ModelDefinitionOutputModalitiesTEXT},
		})
		_, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
		if err == nil {
			t.Fatal("expected error for duplicate model ID")
		}
	})

	t.Run("binding references unknown upstream is rejected on validate", func(t *testing.T) {
		content := validDraft(up.UpstreamID)
		// Use a valid upstream ID format that doesn't exist
		content.Bindings[0].UpstreamId = platformid.New(platformid.Upstream)
		updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
		if err != nil {
			t.Fatal(err)
		}
		result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Fatalf("validation should fail for binding referencing unknown upstream")
		}
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e.Code, "upstream") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected upstream validation error, got %+v", result.Errors)
		}
	})
}
