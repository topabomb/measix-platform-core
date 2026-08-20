package runtimecontrol_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/ent/activation"
	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestI3PublishPersistsIntentBeforeRelayAndFinalizesAfterAck(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 7, 30, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x51}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	upstreamService.Now = func() time.Time { return now }
	secret, err := upstreamService.CreateSecret(ctx, boot.AdminUserID, "runtime-token", "plaintext-must-never-persist-in-activation")
	if err != nil {
		t.Fatal(err)
	}
	upstreamView, err := upstreamService.CreateUpstream(ctx, boot.AdminUserID, publishUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Client.Upstream.UpdateOneID(upstreamView.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}

	capabilityService := capability.NewService(st.Client)
	capabilityService.Now = func() time.Time { return now }
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content := publishDraft(upstreamView.UpstreamID)
	updated, err := capabilityService.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, content)
	if err != nil {
		t.Fatal(err)
	}

	relayStore := control.NewStore(func() time.Time { return now })
	relayHandler := control.NewHandler(relayStore, "relay-service-token")
	var applyCalls atomic.Int32
	guardedRelay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/internal/v1/control/state" {
			applyCalls.Add(1)
			pending, err := st.Client.Activation.Query().Where(activation.StateEQ("APPLYING")).Only(r.Context())
			if err != nil {
				t.Errorf("Relay apply started before persisted Activation: %v", err)
			} else if bytes.Contains(pending.TargetDescriptorJSON, []byte("plaintext-must-never-persist-in-activation")) {
				t.Error("Activation descriptor persisted plaintext credential")
			}
			managed, err := st.Client.ManagedState.Get(r.Context(), "current")
			if err != nil {
				t.Errorf("read managed state during apply: %v", err)
			} else {
				if managed.RuntimeStatus != "ACTIVATING" || managed.DesiredControlRevision != 1 || managed.DesiredBundleHash == nil || *managed.DesiredBundleHash == "" {
					t.Errorf("desired state not persisted before Relay apply: %+v", managed)
				}
			}
			activeCount, err := st.Client.ManagedRelease.Query().Where(managedrelease.StatusEQ("ACTIVE")).Count(r.Context())
			if err != nil {
				t.Errorf("count active releases: %v", err)
			} else if activeCount != 0 {
				t.Errorf("Release became ACTIVE before Relay ACK: count=%d", activeCount)
			}
		}
		relayHandler.ServeHTTP(w, r)
	}))
	defer guardedRelay.Close()

	relayClient := runtimecontrol.NewHTTPRelayClient(guardedRelay.URL, "relay-service-token", guardedRelay.Client())
	service := runtimecontrol.NewService(st.Client, capabilityService, upstreamService, identityService.Signer, relayClient)
	service.Now = func() time.Time { return now }
	key := platformid.New(platformid.Idempotency)
	result, err := service.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           boot.AdminUserID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: updated.DraftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "COMPLETED" || result.Kind != "PUBLISH" || applyCalls.Load() != 1 {
		t.Fatalf("unexpected publish result=%+v applyCalls=%d", result, applyCalls.Load())
	}

	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 1 || managed.ActiveReleaseID == nil || managed.DesiredControlRevision != 1 {
		t.Fatalf("publish did not finalize authoritative state: %+v", managed)
	}
	release, err := st.Client.ManagedRelease.Get(ctx, *managed.ActiveReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if release.Status != "ACTIVE" || release.ManagedGeneration != 1 {
		t.Fatalf("release not active after ACK: %+v", release)
	}
	relayStatus := relayStore.Status()
	if !relayStatus.Ready || relayStatus.AppliedControlRevision != 1 || relayStatus.ActiveManagedGeneration != 1 || relayStatus.BundleHash != *managed.DesiredBundleHash {
		t.Fatalf("Hub/Relay state diverged: hub=%+v relay=%+v", managed, relayStatus)
	}
	current := relayStore.Current()
	if current == nil || current.Upstreams[upstreamView.UpstreamID].Auth.Token != "plaintext-must-never-persist-in-activation" {
		t.Fatal("Relay did not receive resolved runtime credential")
	}

	replay, err := service.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           boot.AdminUserID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: updated.DraftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replay.ActivationID != result.ActivationID || applyCalls.Load() != 1 {
		t.Fatalf("idempotent replay created side effect: first=%+v replay=%+v calls=%d", result, replay, applyCalls.Load())
	}
	releaseCount, err := st.Client.ManagedRelease.Query().Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if releaseCount != 1 {
		t.Fatalf("idempotent replay created release: count=%d", releaseCount)
	}

	_, err = service.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID:           boot.AdminUserID,
		IdempotencyKey:        key,
		ExpectedDraftRevision: updated.DraftRevision + 1,
	})
	if !runtimecontrol.IsIdempotencyConflict(err) {
		t.Fatalf("same key with different request err=%v", err)
	}
}

func publishUpstreamConfig(secretID string, secretVersion int) adminapi.UpstreamConfig {
	return adminapi.UpstreamConfig{
		Name:    "Adapter",
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

func publishDraft(upstreamID string) adminapi.ManagedDraftContent {
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

func TestI3ActivationDescriptorDoesNotContainCredentialMaterial(t *testing.T) {
	for _, forbidden := range []string{"token", "password", "secret-value"} {
		if strings.Contains(runtimecontrol.DescriptorPolicy(), forbidden+"=") {
			t.Fatalf("descriptor policy unexpectedly contains credential assignment %q", forbidden)
		}
	}
}
