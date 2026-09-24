package httpapi_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/hub/httpapi"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/pkg/platformid"
)

// Shared examples are checked against real authentication, SQLite and HTTP handlers.
func TestClientIntegrationPendingSnapshotAndLogout(t *testing.T) {
	_, id, _, ctx, adminID := setupFullHandler(t)
	cap := capability.NewService(id.Client)
	h := httpapi.NewFull(httpapi.Services{Identity: id, Capability: cap})
	cookie, csrf := loginAdmin(t, h)
	session := enrollClientSession(t, h, cookie, csrf)
	headers := map[string]string{"Authorization": "Bearer " + session.AccessToken}
	fixture := func(name string, value any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("../../../../api/fixtures/client-integration", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, value); err != nil {
			t.Fatal(err)
		}
	}
	var pending clientapi.ManagedState
	fixture("managed-pending", &pending)
	bootstrap := doJSON(t, h, http.MethodGet, "/api/client/v1/bootstrap", headers, nil)
	if bootstrap.Code != 200 {
		t.Fatal(bootstrap.Code, bootstrap.Body.String())
	}
	var boot clientapi.Bootstrap
	decodeJSON(t, bootstrap, &boot)
	if boot.ManagedState != pending {
		t.Fatalf("pending bootstrap=%+v want=%+v", boot.ManagedState, pending)
	}
	if !boot.Session.ExpiresAt.Equal(boot.Session.SessionIdleExpiresAt) {
		t.Fatal("bootstrap session expiry must be idle expiry")
	}
	// Reporting generation zero must never admit runtime before a first release.
	headers["X-Measix-Applied-Managed-Generation"] = "0"
	state := doJSON(t, h, http.MethodGet, "/api/client/v1/managed/state", headers, nil)
	var current clientapi.ManagedState
	decodeJSON(t, state, &current)
	if !current.RuntimeBlocked {
		t.Fatal("pending configuration generation zero admits runtime")
	}

	raw, err := os.ReadFile("../../../../api/fixtures/draft/s02-client-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	var content adminapi.ManagedDraftContent
	if err = json.Unmarshal(raw, &content); err != nil {
		t.Fatal(err)
	}
	releaseID := platformid.New(platformid.Release)
	snap, _, err := cap.CompileSnapshot(capability.SnapshotInput{DeploymentID: id.Signer.DeploymentID, ReleaseID: releaseID, ManagedGeneration: 42, Content: content, PublishedAt: id.Now(), PublishedByUserID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	_, err = id.Client.ManagedRelease.Create().SetID(releaseID).SetManagedGeneration(42).SetStatus("ACTIVE").SetReleaseContentJSON(raw).SetSnapshotJSON(encoded).SetSnapshotHash(snap.SnapshotHash).SetSourceDraftRevision(1).SetCreatedByUserID(adminID).SetCreatedAt(id.Now()).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = id.Client.ManagedState.UpdateOneID("current").SetActiveManagedGeneration(42).SetManagedStateRevision(77).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	delete(headers, "X-Measix-Applied-Managed-Generation")
	state = doJSON(t, h, http.MethodGet, "/api/client/v1/managed/state", headers, nil)
	decodeJSON(t, state, &current)
	if !current.SyncRequired || !current.RuntimeBlocked || current.TargetManagedGeneration == nil || *current.TargetManagedGeneration != 42 {
		t.Fatalf("unapplied state=%+v", current)
	}
	headers["X-Measix-Applied-Managed-Generation"] = "42"
	state = doJSON(t, h, http.MethodGet, "/api/client/v1/managed/state", headers, nil)
	decodeJSON(t, state, &current)
	var ready clientapi.ManagedState
	fixture("managed-ready", &ready)
	// Reset optional field before decoding each independent response.
	current = clientapi.ManagedState{}
	decodeJSON(t, state, &current)
	if current != ready {
		t.Fatalf("applied state=%+v want=%+v", current, ready)
	}
	path := "/api/client/v1/managed/snapshots/42"
	response := doJSON(t, h, http.MethodGet, path, headers, nil)
	if response.Code != 200 || response.Body.String() != string(encoded) || response.Header().Get("ETag") != "\""+snap.SnapshotHash+"\"" {
		t.Fatalf("snapshot status=%d body=%s", response.Code, response.Body.String())
	}
	headers["If-None-Match"] = response.Header().Get("ETag")
	response = doJSON(t, h, http.MethodGet, path, headers, nil)
	if response.Code != 304 || response.Body.Len() != 0 {
		t.Fatal("conditional snapshot must have no body", response.Code)
	}
	response = doJSON(t, h, http.MethodGet, "/api/client/v1/managed/snapshots/99", headers, nil)
	if response.Code != 404 {
		t.Fatal("unpublished generation", response.Code)
	}
	principal, err := id.AuthenticateAccess(ctx, session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	beforeReport, err := id.Client.Session.Get(ctx, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeReport.AppliedManagedGeneration != nil || beforeReport.AppliedReportedAt != nil {
		t.Fatal("query/download must not acknowledge application")
	}
	for _, report := range []struct {
		generation int
		hash       string
		status     int
	}{
		{42, "wrong", 422}, {99, snap.SnapshotHash, 422}, {42, snap.SnapshotHash, 204}, {42, snap.SnapshotHash, 204},
	} {
		got := doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": report.generation, "snapshotHash": report.hash})
		if got.Code != report.status {
			t.Fatalf("application report: %d want %d", got.Code, report.status)
		}
	}
	afterReport, err := id.Client.Session.Get(ctx, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterReport.AppliedManagedGeneration == nil || *afterReport.AppliedManagedGeneration != 42 || afterReport.AppliedSnapshotHash == nil || *afterReport.AppliedSnapshotHash != snap.SnapshotHash || afterReport.AppliedReportedAt == nil {
		t.Fatal("application report not durably recorded")
	}
	if !afterReport.ExpiresAt.Equal(beforeReport.ExpiresAt) {
		t.Fatal("report extended session lifetime")
	}
	devices, err := id.ListDeviceViews(ctx, principal.UserID, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].ApplicationState != "APPLIED" || devices[0].AppliedReportedAt == nil {
		t.Fatalf("device acknowledgement missing: %+v", devices)
	}
	if err := id.Client.ManagedState.UpdateOneID("current").SetActiveManagedGeneration(43).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	devices, err = id.ListDeviceViews(ctx, principal.UserID, 20, "")
	if err != nil || devices[0].ApplicationState != "PENDING" || devices[0].TargetManagedGeneration != 43 || *devices[0].AppliedManagedGeneration != 42 {
		t.Fatal("new publication did not expose pending application")
	}
	nextReleaseID := platformid.New(platformid.Release)
	nextSnapshot, _, err := cap.CompileSnapshot(capability.SnapshotInput{DeploymentID: id.Signer.DeploymentID, ReleaseID: nextReleaseID, ManagedGeneration: 43, Content: content, PublishedAt: id.Now(), PublishedByUserID: adminID})
	if err != nil {
		t.Fatal(err)
	}
	nextEncoded, err := json.Marshal(nextSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := id.Client.ManagedRelease.UpdateOneID(releaseID).SetStatus("SUPERSEDED").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := id.Client.ManagedRelease.Create().SetID(nextReleaseID).SetManagedGeneration(43).SetStatus("ACTIVE").SetReleaseContentJSON(raw).SetSnapshotJSON(nextEncoded).SetSnapshotHash(nextSnapshot.SnapshotHash).SetSourceDraftRevision(2).SetCreatedByUserID(adminID).SetCreatedAt(id.Now()).Save(ctx); err != nil {
		t.Fatal(err)
	}
	response = doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 43, "snapshotHash": nextSnapshot.SnapshotHash})
	if response.Code != 204 {
		t.Fatal("new application report", response.Code)
	}
	response = doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 42, "snapshotHash": snap.SnapshotHash})
	if response.Code != 409 {
		t.Fatal("regressed report accepted", response.Code)
	}
	response = doJSON(t, h, http.MethodPost, "/api/client/v1/sessions/logout", nil, map[string]string{"refreshToken": session.RefreshToken})
	if response.Code != 204 {
		t.Fatal("logout", response.Code)
	}
	response = doJSON(t, h, http.MethodGet, path, headers, nil)
	if response.Code != 403 {
		t.Fatal("ETag bypassed revoked session", response.Code)
	}
	response = doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 42, "snapshotHash": snap.SnapshotHash})
	if response.Code != 403 {
		t.Fatal("revoked session reported application", response.Code)
	}
	devices, err = id.ListDeviceViews(ctx, principal.UserID, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if devices[0].ApplicationState != "UNKNOWN" || devices[0].AppliedManagedGeneration != nil {
		t.Fatal("revoked session acknowledgement leaked into current device status")
	}
	device, err := id.Client.Device.Get(ctx, principal.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := id.CreateEnrollment(ctx, principal.UserID, adminID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rejoined, err := id.ExchangeEnrollment(ctx, grant.Code, *device.InstallationID, device.Name, *device.AppVersion)
	if err != nil {
		t.Fatal(err)
	}
	if rejoined.DeviceID != principal.DeviceID || rejoined.SessionID == principal.SessionID {
		t.Fatal("re-enrollment identity incorrect")
	}
	devices, err = id.ListDeviceViews(ctx, principal.UserID, 20, "")
	if err != nil || devices[0].ApplicationState != "UNKNOWN" || devices[0].AppliedReportedAt != nil {
		t.Fatal("new session inherited old report")
	}
	headers["Authorization"] = "Bearer " + rejoined.AccessToken
	response = doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 43, "snapshotHash": nextSnapshot.SnapshotHash})
	if response.Code != 204 {
		t.Fatal("new session cannot report its applied state", response.Code)
	}
	if err := id.Client.Session.UpdateOneID(rejoined.SessionID).SetExpiresAt(id.Now().Add(-time.Second)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	response = doJSON(t, h, http.MethodPut, "/api/client/v1/managed/applied", headers, map[string]any{"managedGeneration": 43, "snapshotHash": nextSnapshot.SnapshotHash})
	if response.Code != 401 {
		t.Fatal("expired session report accepted", response.Code)
	}
	devices, err = id.ListDeviceViews(ctx, principal.UserID, 20, "")
	if err != nil || devices[0].ApplicationState != "UNKNOWN" || devices[0].AppliedReportedAt != nil {
		t.Fatal("expired session remained applied")
	}
}
