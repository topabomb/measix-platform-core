package capability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/identity"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestHUBCAP001DraftOptimisticConcurrencyAndSaveDoesNotActivate(t *testing.T) {
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
	if up.ConfigRevision != 1 || up.ActiveConfigRevision != nil || up.Status != "INACTIVE" {
		t.Fatalf("candidate upstream unexpectedly active: %+v", up)
	}

	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, err := cap.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeState, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DraftRevision != draft.DraftRevision+1 {
		t.Fatalf("draft revision=%d want=%d", updated.DraftRevision, draft.DraftRevision+1)
	}
	if _, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content); !errors.Is(err, capability.ErrRevisionConflict) {
		t.Fatalf("stale draft write err=%v", err)
	}
	afterState, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if afterState.ActiveManagedGeneration != beforeState.ActiveManagedGeneration || afterState.ActiveReleaseID != beforeState.ActiveReleaseID {
		t.Fatalf("Save Draft changed active state: before=%+v after=%+v", beforeState, afterState)
	}

	result, err := cap.ValidateDraft(ctx, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("valid draft rejected: %+v", result)
	}
}

func TestHUBUPS003SecretVersionsAreAppendOnlyEncryptedAndReferencedPrecisely(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x24}, 32), 7)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	created, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "first-secret")
	if err != nil {
		t.Fatal(err)
	}
	if created.SecretVersion != 1 {
		t.Fatalf("version=%d", created.SecretVersion)
	}
	replaced, err := ups.ReplaceSecret(ctx, boot.AdminUserID, created.SecretID, 1, "second-secret")
	if err != nil {
		t.Fatal(err)
	}
	if replaced.SecretVersion != 2 || replaced.SecretID != created.SecretID {
		t.Fatalf("replace result=%+v", replaced)
	}
	if _, err := ups.ReplaceSecret(ctx, boot.AdminUserID, created.SecretID, 1, "stale-secret"); !errors.Is(err, upstream.ErrRevisionConflict) {
		t.Fatalf("stale secret replace err=%v", err)
	}
	versions, err := st.Client.SecretVersion.Query().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("secret versions=%d want=2", len(versions))
	}
	for _, version := range versions {
		if bytes.Contains(version.EncryptedPayload, []byte("first-secret")) || bytes.Contains(version.EncryptedPayload, []byte("second-secret")) {
			t.Fatal("plaintext secret persisted")
		}
		if version.KeyVersion != 7 {
			t.Fatalf("key version=%d", version.KeyVersion)
		}
	}
}

func TestHUBCAP006SnapshotDeterministicAndClientSafe(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x33}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	secret, err := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "do-not-leak")
	if err != nil {
		t.Fatal(err)
	}
	up, err := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	content := validDraft(up.UpstreamID)
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	releaseID := platformid.New(platformid.Release)
	input := capability.SnapshotInput{
		DeploymentID:      boot.DeploymentID,
		ReleaseID:         releaseID,
		ManagedGeneration: 1,
		Content:           content,
		PublishedAt:       now,
		PublishedByUserID: boot.AdminUserID,
	}
	first, firstHash, err := cap.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := cap.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if firstHash != secondHash || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("snapshot is not deterministic: %s != %s", firstHash, secondHash)
	}
	for _, forbidden := range []string{"do-not-leak", secret.SecretID, "https://adapter.example", content.Bindings[0].RuntimeRouteId} {
		if strings.Contains(string(firstJSON), forbidden) {
			t.Fatalf("client snapshot leaked %q: %s", forbidden, firstJSON)
		}
	}
	if first.Models[0].ModelId != content.Models[0].ModelId || first.Providers[0].ProviderId != content.Providers[0].ProviderId {
		t.Fatalf("candidate stable IDs were rewritten: snapshot=%+v draft=%+v", first, content)
	}
}

func TestHUBCAP008StagedReleaseIsImmutable(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	box, _ := security.NewSecretBox(bytes.Repeat([]byte{0x11}, 32), 1)
	ups := upstream.NewService(st.Client, box)
	ups.Now = func() time.Time { return now }
	secret, _ := ups.CreateSecret(ctx, boot.AdminUserID, "provider-token", "secret")
	up, _ := ups.CreateUpstream(ctx, boot.AdminUserID, testUpstreamConfig(secret.SecretID, secret.SecretVersion))
	cap := capability.NewService(st.Client)
	cap.Now = func() time.Time { return now }
	draft, _ := cap.GetDraft(ctx)
	content := validDraft(up.UpstreamID)
	updated, err := cap.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}
	release, err := cap.StageRelease(ctx, boot.AdminUserID, updated.DraftRevision)
	if err != nil {
		t.Fatal(err)
	}
	storedBefore, err := st.Client.ManagedRelease.Get(ctx, release.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	mutated := content
	mutated.Models = append([]adminapi.ModelDefinition(nil), content.Models...)
	mutated.Models[0].DisplayName = "Changed after staging"
	if _, err := cap.PutDraft(ctx, boot.AdminUserID, updated.DraftRevision, mutated); err != nil {
		t.Fatal(err)
	}
	storedAfter, err := st.Client.ManagedRelease.Get(ctx, release.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(storedBefore.ReleaseContentJSON, storedAfter.ReleaseContentJSON) || !bytes.Equal(storedBefore.SnapshotJSON, storedAfter.SnapshotJSON) {
		t.Fatal("staged release mutated after Draft edit")
	}
	if release.ManagedGeneration != 1 || release.Status != "STAGED" {
		t.Fatalf("unexpected staged release: %+v", release)
	}
}

func bootstrapI2(t *testing.T) (*testutil.StoreHandle, identity.BootstrapResult, time.Time) {
	t.Helper()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 1, 2, 3, 0, time.UTC)
	service := testutil.NewIdentityService(t, st, now)
	boot, err := service.Bootstrap(context.Background(), "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	return st, boot, now
}

func testUpstreamConfig(secretID string, secretVersion int) adminapi.UpstreamConfig {
	return adminapi.UpstreamConfig{
		Name: "Adapter",
		BaseUrl: "https://adapter.example",
		TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{
			adminapi.UpstreamConfigTransportCapabilitiesHTTPREQUESTRESPONSE,
			adminapi.UpstreamConfigTransportCapabilitiesHTTPSTREAMINGSSE,
		},
		Auth: adminapi.UpstreamAuth{
			Type:      adminapi.UpstreamAuthTypeBEARER,
			SecretRef: &adminapi.SecretRef{SecretId: secretID, SecretVersion: secretVersion},
		},
		CorrelationMode:      adminapi.UpstreamConfigCorrelationModeHEADERECHO,
		UsageCapabilityLevel: adminapi.LEVEL0,
		TimeoutDefaults:      adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
	}
}

func validDraft(upstreamID string) adminapi.ManagedDraftContent {
	providerID := platformid.New(platformid.Provider)
	modelID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	policyID := platformid.New(platformid.Policy)
	return adminapi.ManagedDraftContent{
		Providers: []adminapi.ProviderDefinition{{ProviderId: providerID, DisplayName: "Managed AI", ClientProtocol: adminapi.OPENAICHATCOMPLETIONS, Enabled: true}},
		Models: []adminapi.ModelDefinition{{
			ModelId: modelID, ProviderId: providerID, DisplayName: "Managed Model", UpstreamModelKey: "model-x", RuntimePath: "/v1/chat/completions", Enabled: true,
			Capabilities: []adminapi.ModelDefinitionCapabilities{adminapi.TOOL}, InputModalities: []adminapi.ModelDefinitionInputModalities{adminapi.ModelDefinitionInputModalitiesTEXT}, OutputModalities: []adminapi.ModelDefinitionOutputModalities{adminapi.ModelDefinitionOutputModalitiesTEXT},
		}},
		Tts: []adminapi.TtsDefinition{}, Asr: []adminapi.AsrDefinition{}, Mcp: []adminapi.McpDefinition{},
		Bindings: []adminapi.RuntimeBindingDefinition{{
			RuntimeRouteId: routeID, ResourceId: modelID, UpstreamId: upstreamID,
			AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: adminapi.RuntimeBindingDefinitionTransportPolicyHTTPSTREAMINGSSE,
		}},
		Policy: adminapi.ManagedPolicy{PolicyId: policyID, AllowLocalProviders: true, AllowLocalTts: true, AllowLocalAsr: true, AllowLocalMcp: true, DefaultModelId: &modelID},
	}
}
