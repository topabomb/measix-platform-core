package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

// Server version support is not per-client negotiation. Downloading a new
// release cannot acknowledge it, erase the prior Applied report or rewrite v4.
// The old Android decoder and its persisted cache are verified separately.
func TestOldClientReleaseVersionAndAppliedBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  int
		starters bool
	}{
		{"historical-v4", 4, true},
		{"new-v5-without-starters", 0, false},
		{"new-v5-with-starters", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, id, _, ctx, adminID := setupFullHandler(t)
			cap := capability.NewService(id.Client)
			h := httpapi.NewFull(httpapi.Services{Identity: id, Capability: cap})
			grant, err := id.CreateEnrollment(ctx, adminID, adminID, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			exchange := doJSON(t, h, http.MethodPost, "/api/client/v1/enrollments/exchange", nil, map[string]string{
				"code": grant.Code, "installationId": platformid.New(platformid.Installation),
				"deviceName": "Old Android compatibility", "appVersion": "0.0.20", "platform": "ANDROID",
			})
			if exchange.Code != http.StatusCreated {
				t.Fatalf("old enrollment shape: status %d", exchange.Code)
			}
			var session clientapi.EnrollmentExchangeResponse
			decodeJSON(t, exchange, &session)
			headers := map[string]string{"Authorization": "Bearer " + session.AccessToken}
			var discovery clientapi.Discovery
			decodeJSON(t, doJSON(t, h, http.MethodGet, "/.well-known/measix", nil, nil), &discovery)
			if !slices.Equal(discovery.SupportedSnapshotSchemaVersions, []int{4, 5}) {
				t.Fatal("unexpected discovery support")
			}

			raw, err := os.ReadFile("../../../../api/fixtures/draft/s02-client-profile.json")
			if err != nil {
				t.Fatal(err)
			}
			var content adminapi.ManagedDraftContent
			if err := json.Unmarshal(raw, &content); err != nil {
				t.Fatal(err)
			}
			storeRelease := func(generation, version int, content adminapi.ManagedDraftContent) (clientapi.ManagedSnapshot, []byte) {
				t.Helper()
				releaseID := platformid.New(platformid.Release)
				snapshot, hash, err := cap.CompileSnapshot(capability.SnapshotInput{
					SchemaVersion: version, DeploymentID: id.Signer.DeploymentID, ReleaseID: releaseID,
					ManagedGeneration: generation, Content: content, PublishedAt: id.Now(), PublishedByUserID: adminID,
				})
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.MarshalIndent(snapshot, "", " ")
				if err != nil {
					t.Fatal(err)
				}
				encoded = append(encoded, '\n')
				contentJSON, err := json.Marshal(content)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := id.Client.ManagedRelease.Create().SetID(releaseID).SetManagedGeneration(int64(generation)).SetStatus("ACTIVE").SetReleaseContentJSON(contentJSON).SetSnapshotJSON(encoded).SetSnapshotHash(hash).SetSourceDraftRevision(int64(generation)).SetCreatedByUserID(adminID).SetCreatedAt(id.Now()).Save(ctx); err != nil {
					t.Fatal(err)
				}
				if err := id.Client.ManagedState.UpdateOneID("current").SetActiveManagedGeneration(int64(generation)).SetRuntimeStatus("READY").Exec(ctx); err != nil {
					t.Fatal(err)
				}
				return snapshot, encoded
			}
			original, originalBytes := storeRelease(1, 4, content)
			response := doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 1, "snapshotHash": original.SnapshotHash})
			if response.Code != http.StatusNoContent {
				t.Fatalf("v4 Applied: %d", response.Code)
			}
			headers["X-Measix-Applied-Managed-Generation"] = "1"
			var state clientapi.ManagedState
			decodeJSON(t, doJSON(t, h, http.MethodGet, "/api/client/v1/managed/state", headers, nil), &state)
			if state.RuntimeBlocked || state.SyncRequired {
				t.Fatal("current v4 client was not ready")
			}
			if !tc.starters {
				content.Starters = []adminapi.AssistantStarterDefinition{}
			} else if tc.version == 0 {
				for index := range content.Starters {
					content.Starters[index].OpeningSnapshot = &adminapi.StarterOpeningSnapshot{Format: 1, SystemPrompt: "", InitialContexts: []adminapi.StarterInitialContext{}}
				}
			}
			if err := id.Client.ManagedRelease.UpdateOneID(string(original.ReleaseId)).SetStatus("SUPERSEDED").Exec(ctx); err != nil {
				t.Fatal(err)
			}
			next, nextBytes := storeRelease(2, tc.version, content)
			cookie, csrf := loginAdmin(t, h)
			adminHeaders := map[string]string{"Cookie": cookie, "X-CSRF-Token": csrf}
			for _, expected := range []clientapi.ManagedSnapshot{original, next} {
				var detail map[string]any
				decodeJSON(t, doJSON(t, h, http.MethodGet, "/api/admin/v1/releases/"+string(expected.ReleaseId), adminHeaders, nil), &detail)
				if detail["snapshotSchemaVersion"] != float64(expected.SchemaVersion) {
					t.Fatalf("Admin release version=%v want %d", detail["snapshotSchemaVersion"], expected.SchemaVersion)
				}
			}
			var list struct {
				Items []map[string]any `json:"items"`
			}
			decodeJSON(t, doJSON(t, h, http.MethodGet, "/api/admin/v1/releases", adminHeaders, nil), &list)
			if len(list.Items) != 2 || list.Items[0]["snapshotSchemaVersion"] != float64(next.SchemaVersion) || list.Items[1]["snapshotSchemaVersion"] != float64(original.SchemaVersion) {
				t.Fatalf("Admin release list lost actual versions: %+v", list.Items)
			}
			draft, err := cap.GetDraft(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var preview map[string]any
			decodeJSON(t, doJSON(t, h, http.MethodPost, "/api/admin/v1/draft:preview", adminHeaders, map[string]int{"expectedDraftRevision": draft.DraftRevision}), &preview)
			if preview["snapshotSchemaVersion"] != float64(5) {
				t.Fatalf("preview schema=%v want 5", preview["snapshotSchemaVersion"])
			}
			wantVersion := tc.version
			if wantVersion == 0 {
				wantVersion = 5
			}
			if int(next.SchemaVersion) != wantVersion {
				t.Fatalf("release version %d, want %d", next.SchemaVersion, wantVersion)
			}
			var boot clientapi.Bootstrap
			decodeJSON(t, doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", headers, nil), &boot)
			if !slices.Equal(boot.SupportedSnapshotSchemaVersions, []int{4, 5}) || boot.ManagedState.ActiveManagedGeneration != 2 {
				t.Fatal("bootstrap selected a different release for old appVersion")
			}
			state = clientapi.ManagedState{}
			decodeJSON(t, doJSON(t, h, http.MethodGet, "/api/client/v1/managed/state", headers, nil), &state)
			if !state.RuntimeBlocked || !state.SyncRequired || state.TargetManagedGeneration == nil || *state.TargetManagedGeneration != 2 {
				t.Fatal("prior generation remained ready after publication")
			}
			for _, saved := range []struct {
				generation int
				hash       string
				body       []byte
			}{{1, original.SnapshotHash, originalBytes}, {2, next.SnapshotHash, nextBytes}} {
				path := "/api/client/v1/managed/snapshots/" + strconv.Itoa(saved.generation)
				response := doJSON(t, h, http.MethodGet, path, headers, nil)
				etag := "\"" + saved.hash + "\""
				if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), saved.body) || response.Header().Get("ETag") != etag {
					t.Fatalf("download rewrote immutable generation %d", saved.generation)
				}
				headers["If-None-Match"] = etag
				response = doJSON(t, h, http.MethodGet, path, headers, nil)
				if response.Code != http.StatusNotModified || response.Body.Len() != 0 {
					t.Fatalf("generation %d lost conditional download", saved.generation)
				}
				delete(headers, "If-None-Match")
			}
			persisted, err := id.Client.Session.Get(ctx, string(session.SessionId))
			if err != nil {
				t.Fatal(err)
			}
			if persisted.AppliedManagedGeneration == nil || *persisted.AppliedManagedGeneration != 1 || persisted.AppliedSnapshotHash == nil || *persisted.AppliedSnapshotHash != original.SnapshotHash {
				t.Fatal("new release download changed prior Applied report")
			}
			devices, err := id.ListDeviceViews(ctx, adminID, 20, "")
			if err != nil || len(devices) != 1 || devices[0].ApplicationState != "PENDING" || devices[0].TargetManagedGeneration != 2 {
				t.Fatal("Admin did not retain the pending device state")
			}
		})
	}
}
