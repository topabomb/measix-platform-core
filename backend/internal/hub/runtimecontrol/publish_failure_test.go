package runtimecontrol_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"measix/platform/ent/managedrelease"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/runtimecontrol"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/pkg/platformid"
)

// HUB-ACT-004 / Publish failure branch: a deterministic Relay validation reject
// proves that the desired state was not applied. The staged release therefore
// becomes ACTIVATION_FAILED and Hub returns to READY on the previous active state;
// it must not be left UNKNOWN waiting for reconciliation.
func TestPublishRelayValidationRejectIsTerminalAndKeepsPreviousRuntimeReady(t *testing.T) {
	ctx := context.Background()
	st := testutil.OpenStoreHandle(t)
	now := time.Date(2026, 8, 19, 18, 45, 0, 0, time.UTC)
	identityService := testutil.NewIdentityService(t, st, now)
	boot, err := identityService.Bootstrap(ctx, "Example Corp", "admin", "Admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	box, err := security.NewSecretBox(bytes.Repeat([]byte{0x61}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	upstreamService := upstream.NewService(st.Client, box)
	upstreamService.Now = func() time.Time { return now }
	secret, err := upstreamService.CreateSecret(ctx, boot.AdminUserID, "runtime-token", "synthetic-secret")
	if err != nil {
		t.Fatal(err)
	}
	upstreamView, err := upstreamService.CreateUpstream(ctx, boot.AdminUserID, publishUpstreamConfig(secret.SecretID, secret.SecretVersion))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Client.Upstream.UpdateOneID(upstreamView.UpstreamID).
		SetActiveConfigRevision(1).SetStatus("ACTIVE").SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}

	capabilityService := capability.NewService(st.Client)
	capabilityService.Now = func() time.Time { return now }
	draft, err := capabilityService.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := capabilityService.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, publishDraft(upstreamView.UpstreamID))
	if err != nil {
		t.Fatal(err)
	}

	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/internal/v1/control/state" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(relaycontrolapi.Problem{
			Type: "about:blank", Title: "Invalid runtime control", Status: http.StatusUnprocessableEntity, Code: "invalid_runtime_control",
		})
	}))
	defer relay.Close()

	service := runtimecontrol.NewService(
		st.Client,
		capabilityService,
		upstreamService,
		identityService.Signer,
		runtimecontrol.NewHTTPRelayClient(relay.URL, "relay-service-token", relay.Client()),
	)
	service.Now = func() time.Time { return now }

	result, err := service.Publish(ctx, runtimecontrol.PublishRequest{
		AdminUserID: boot.AdminUserID, IdempotencyKey: platformid.New(platformid.Idempotency), ExpectedDraftRevision: updated.DraftRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "FAILED" || result.ErrorCode == nil || *result.ErrorCode != "invalid_runtime_control" {
		t.Fatalf("deterministic Relay reject result=%+v, want FAILED/invalid_runtime_control", result)
	}

	release, err := st.Client.ManagedRelease.Query().Where(managedrelease.ManagedGenerationEQ(1)).Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if release.Status != "ACTIVATION_FAILED" {
		t.Fatalf("release status=%s, want ACTIVATION_FAILED", release.Status)
	}
	managed, err := st.Client.ManagedState.Get(ctx, "current")
	if err != nil {
		t.Fatal(err)
	}
	if managed.RuntimeStatus != "READY" || managed.ActiveManagedGeneration != 0 || managed.ActiveReleaseID != nil {
		t.Fatalf("failed publish changed active runtime state: %+v", managed)
	}
}
