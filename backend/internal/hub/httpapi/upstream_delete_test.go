package httpapi_test

import (
	"encoding/json"
	"testing"

	"measix/platform/ent/upstreamconfigrevision"
	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/hub/security"
	"measix/platform/internal/hub/upstream"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestUpstreamDeletionHTTP(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "inactive", true: "unused active"}[active], func(t *testing.T) {
			_, id, _, ctx, actor := setupFullHandler(t)
			box, _ := security.NewSecretBox(make([]byte, 32), 1)
			svc := upstream.NewService(id.Client, box)
			secret, err := svc.CreateSecret(ctx, actor, "Retained secret", "synthetic upstream credential")
			if err != nil {
				t.Fatal(err)
			}
			view, err := svc.CreateUpstream(ctx, actor, adminapi.UpstreamConfig{Name: "Unused", BaseUrl: "https://adapter.example", Auth: adminapi.UpstreamAuth{Type: adminapi.UpstreamAuthTypeBEARER, SecretRef: &adminapi.SecretRef{SecretId: secret.SecretID, SecretVersion: secret.SecretVersion}}, TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{adminapi.UpstreamConfigTransportCapabilitiesHTTPREQUESTRESPONSE}, CorrelationMode: adminapi.UpstreamConfigCorrelationModeHEADERECHO, UsageCapabilityLevel: adminapi.LEVEL0, TimeoutDefaults: adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}})
			if err != nil {
				t.Fatal(err)
			}
			if active {
				if _, err := id.Client.Upstream.UpdateOneID(view.UpstreamID).SetActiveConfigRevision(1).SetStatus("ACTIVE").Save(ctx); err != nil {
					t.Fatal(err)
				}
			}
			h := httpapi.NewFull(httpapi.Services{Identity: id, Upstream: svc})
			cookie, csrf := loginAdmin(t, h)
			headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
			path := "/api/admin/v1/upstreams/" + view.UpstreamID
			accountRequest(t, h, "DELETE", path, headers, map[string]int{"expectedConfigRevision": 2}, 409, "stale_upstream_config_revision")
			accountRequest(t, h, "DELETE", path, headers, map[string]int{"expectedConfigRevision": 1}, 204, "")
			accountRequest(t, h, "GET", path, headers, nil, 404, "")
			if count, _ := id.Client.UpstreamConfigRevision.Query().Where(upstreamconfigrevision.UpstreamIDEQ(view.UpstreamID)).Count(ctx); count != 0 {
				t.Fatal("deleted connection revisions remain")
			}
			if value, err := svc.ResolveSecret(ctx, secret.SecretID, secret.SecretVersion); err != nil || string(value) != "synthetic upstream credential" {
				t.Fatal("deletion changed shared credential")
			}
		})
	}
}

func TestCleanupReleasesUpstreamReferenceBeforeIndependentDeletion(t *testing.T) {
	_, id, _, ctx, actor := setupFullHandler(t)
	box, _ := security.NewSecretBox(make([]byte, 32), 1)
	svc := upstream.NewService(id.Client, box)
	view, err := svc.CreateUpstream(ctx, actor, adminapi.UpstreamConfig{Name: "History only", BaseUrl: "https://adapter.example", Auth: adminapi.UpstreamAuth{Type: adminapi.UpstreamAuthTypeNONE}, TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{adminapi.UpstreamConfigTransportCapabilitiesHTTPREQUESTRESPONSE}, CorrelationMode: adminapi.UpstreamConfigCorrelationModeHEADERECHO, UsageCapabilityLevel: adminapi.LEVEL0, TimeoutDefaults: adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}})
	if err != nil {
		t.Fatal(err)
	}
	releaseID := platformid.New(platformid.Release)
	raw, _ := json.Marshal(map[string]any{"bindings": []map[string]string{{"upstreamId": view.UpstreamID}}})
	id.Client.ManagedRelease.Create().SetID(releaseID).SetManagedGeneration(1).SetStatus("SUPERSEDED").SetReleaseContentJSON(raw).SetSnapshotJSON([]byte(`{"original":true}`)).SetSnapshotHash("original-hash").SetSourceDraftRevision(1).SetCreatedByUserID(actor).SetCreatedAt(id.Now()).SaveX(ctx)
	id.Client.ManagedState.UpdateOneID("current").SetRuntimeStatus("READY").SetLastAssignedGeneration(1).SaveX(ctx)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Upstream: svc, Capability: capability.NewService(id.Client)})
	cookie, csrf := loginAdmin(t, h)
	headers := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
	path := "/api/admin/v1/upstreams/" + view.UpstreamID
	refs := doJSON(t, h, "GET", path+"/references", headers, nil)
	var diag adminapi.UpstreamReferences
	decodeJSON(t, refs, &diag)
	if refs.Code != 200 || diag.DraftReferenced || len(diag.Releases) != 1 {
		t.Fatalf("diagnostic=%+v status=%d", diag, refs.Code)
	}
	accountRequest(t, h, "DELETE", path, headers, map[string]int{"expectedConfigRevision": 1}, 409, "upstream_in_use")
	sel := map[string]any{"releaseIds": []string{releaseID}}
	response := doJSON(t, h, "POST", "/api/admin/v1/releases/cleanup:preview", headers, sel)
	var p adminapi.ReleaseCleanupPreview
	decodeJSON(t, response, &p)
	if response.Code != 200 || len(p.Candidates) != 1 || len(p.ReleasedUpstreamIds) != 1 {
		t.Fatalf("preview=%+v response=%s", p, response.Body.String())
	}
	accountRequest(t, h, "POST", "/api/admin/v1/releases/cleanup", headers, map[string]any{"selection": sel, "previewHash": p.PreviewHash}, 200, "")
	if _, err = svc.GetUpstream(ctx, view.UpstreamID); err != nil {
		t.Fatal("cleanup unexpectedly deleted upstream")
	}
	accountRequest(t, h, "DELETE", path, headers, map[string]int{"expectedConfigRevision": 1}, 204, "")
}

func TestUpstreamDeletionRetainsReferencedConnection(t *testing.T) {
	for _, owner := range []string{"draft", "retained release", "pending activation"} {
		t.Run(owner, func(t *testing.T) {
			_, id, _, ctx, actor := setupFullHandler(t)
			box, _ := security.NewSecretBox(make([]byte, 32), 1)
			svc := upstream.NewService(id.Client, box)
			view, err := svc.CreateUpstream(ctx, actor, adminapi.UpstreamConfig{Name: "Referenced", BaseUrl: "https://adapter.example", Auth: adminapi.UpstreamAuth{Type: adminapi.UpstreamAuthTypeNONE}, TransportCapabilities: []adminapi.UpstreamConfigTransportCapabilities{adminapi.UpstreamConfigTransportCapabilitiesHTTPREQUESTRESPONSE}, CorrelationMode: adminapi.UpstreamConfigCorrelationModeHEADERECHO, UsageCapabilityLevel: adminapi.LEVEL0, TimeoutDefaults: adminapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 1000, IdleMs: 1000}})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(map[string]any{"bindings": []map[string]string{{"upstreamId": view.UpstreamID}}})
			switch owner {
			case "draft":
				row, err := id.Client.ManagedDraft.Query().Only(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := id.Client.ManagedDraft.UpdateOneID(row.ID).SetContentJSON(raw).Save(ctx); err != nil {
					t.Fatal(err)
				}
			case "retained release":
				_, err := id.Client.ManagedRelease.Create().SetID(platformid.New(platformid.Release)).SetManagedGeneration(1).SetStatus("SUPERSEDED").SetReleaseContentJSON(raw).SetSnapshotJSON([]byte(`{"original":true}`)).SetSnapshotHash("original-hash").SetSourceDraftRevision(1).SetCreatedByUserID(actor).SetCreatedAt(id.Now()).Save(ctx)
				if err != nil {
					t.Fatal(err)
				}
			case "pending activation":
				_, err := id.Client.Activation.Create().SetID(platformid.New(platformid.Activation)).SetKind("RUNTIME_CONFIG").SetState("UNKNOWN").SetIdempotencyKey(platformid.New(platformid.Idempotency)).SetRequestHash("synthetic-hash").SetControlRevision(1).SetBundleHash("synthetic-hash").SetTargetDescriptorJSON([]byte(`{}`)).SetCreatedByUserID(actor).SetCreatedAt(id.Now()).Save(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			h := httpapi.NewFull(httpapi.Services{Identity: id, Upstream: svc})
			cookie, csrf := loginAdmin(t, h)
			code := "upstream_in_use"
			if owner == "pending activation" {
				code = "activation_in_progress"
			}
			accountRequest(t, h, "DELETE", "/api/admin/v1/upstreams/"+view.UpstreamID, map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}, map[string]int{"expectedConfigRevision": 1}, 409, code)
			if _, err := svc.GetUpstream(ctx, view.UpstreamID); err != nil {
				t.Fatal("rejected deletion removed connection")
			}
			if owner == "retained release" {
				row, err := id.Client.ManagedRelease.Query().Only(ctx)
				if err != nil || string(row.ReleaseContentJSON) != string(raw) || row.SnapshotHash != "original-hash" || string(row.SnapshotJSON) != `{"original":true}` {
					t.Fatal("immutable release changed")
				}
			}
		})
	}
}
