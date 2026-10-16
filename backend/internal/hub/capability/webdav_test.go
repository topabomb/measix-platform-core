package capability_test

import (
	"context"
	"encoding/json"
	"testing"

	"measix/platform/internal/hub/capability"
	"measix/platform/internal/wire/adminapi"
	"measix/platform/pkg/platformid"
)

func TestDAVSnapshotIncludesUniqueExchangeDefinitionAndHashesIt(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, err := svc.GetDraft(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id := platformid.New(platformid.WebDAV)
	definitions := []adminapi.WebDAVDefinition{{ResourceId: id, DisplayName: "Files", Enabled: true, ClientProtocol: "WEBDAV", AuthOwnership: "ENTERPRISE_MANAGED", Purpose: "WORKSPACE_FILE_EXCHANGE", RootPathTemplate: "/runtime/v1/resources/" + id + "/spaces/{agentSpaceId}/dav/"}}
	draft.Content.Webdav = &definitions
	input := capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: draft.Content, PublishedAt: now}
	snapshot, hash, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Webdav == nil || len(*snapshot.Webdav) != 1 || (*snapshot.Webdav)[0].Purpose != "WORKSPACE_FILE_EXCHANGE" {
		t.Fatal("snapshot omitted designated DAV")
	}
	actual, err := capability.HashSnapshot(snapshot)
	if err != nil || actual != hash {
		t.Fatal("DAV hash cannot round trip", err)
	}
	definitions[0].Enabled = false
	_, changed, err := svc.CompileSnapshot(input)
	if err != nil || changed == hash {
		t.Fatal("DAV disable not covered by snapshot hash", err)
	}
	input.SchemaVersion = 4
	legacy, _, err := svc.CompileSnapshot(input)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["webdav"]; present {
		t.Fatal("DAV reinterpreted retained v4")
	}
}

func TestDAVCompilerRejectsMultipleExchangeResourcesAndUnsafeTemplate(t *testing.T) {
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, err := svc.GetDraft(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id := platformid.New(platformid.WebDAV)
	def := adminapi.WebDAVDefinition{ResourceId: id, DisplayName: "Files", Enabled: true, ClientProtocol: "WEBDAV", AuthOwnership: "ENTERPRISE_MANAGED", Purpose: "WORKSPACE_FILE_EXCHANGE", RootPathTemplate: "https://outside.example/{agentSpaceId}"}
	defs := []adminapi.WebDAVDefinition{def}
	draft.Content.Webdav = &defs
	input := capability.SnapshotInput{DeploymentID: boot.DeploymentID, ReleaseID: platformid.New(platformid.Release), ManagedGeneration: 1, Content: draft.Content, PublishedAt: now}
	if _, _, err = svc.CompileSnapshot(input); err == nil {
		t.Fatal("accepted external DAV template")
	}
	defs[0].RootPathTemplate = "/runtime/v1/resources/" + id + "/spaces/{agentSpaceId}/dav/"
	second := defs[0]
	second.ResourceId = platformid.New(platformid.WebDAV)
	second.RootPathTemplate = "/runtime/v1/resources/" + second.ResourceId + "/spaces/{agentSpaceId}/dav/"
	defs = append(defs, second)
	if _, _, err = svc.CompileSnapshot(input); err == nil {
		t.Fatal("accepted ambiguous exchange resources")
	}
}

func TestDAVPublishRequiresMatchingWorkspaceBinding(t *testing.T) {
	ctx := context.Background()
	st, boot, now := bootstrapI2(t)
	svc := capability.NewService(st.Client)
	draft, err := svc.GetDraft(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, route, serviceID := platformid.New(platformid.WebDAV), platformid.New(platformid.Route), platformid.New(platformid.WorkspaceService)
	st.Client.WorkspaceService.Create().SetID(serviceID).SetName("Space").SetConfigRevision(1).SetActiveConfigRevision(1).SetEnabled(true).SetState("ACTIVE").SetMcpServerID(platformid.New(platformid.MCP)).SetRuntimeRouteID(platformid.New(platformid.Route)).SetDavResourceID(id).SetDavRuntimeRouteID(route).SetCreatedAt(now).SetUpdatedAt(now).SaveX(ctx)
	defs := []adminapi.WebDAVDefinition{{ResourceId: id, DisplayName: "Files", Enabled: true, ClientProtocol: "WEBDAV", AuthOwnership: "ENTERPRISE_MANAGED", Purpose: "WORKSPACE_FILE_EXCHANGE", RootPathTemplate: capability.DAVRootPathTemplate(id)}}
	draft.Content.Webdav = &defs
	kind := adminapi.RuntimeBindingDefinitionTargetKind("REMOTE_WORKSPACE")
	draft.Content.Bindings = append(draft.Content.Bindings, adminapi.RuntimeBindingDefinition{ResourceId: id, RuntimeRouteId: route, TargetKind: &kind, WorkspaceServiceId: &serviceID, TransportPolicy: "HTTP_REQUEST_RESPONSE", AllowedMethods: []string{"OPTIONS", "GET", "HEAD", "PROPFIND", "PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK"}, AllowedPathPrefixes: []string{"/dav"}})
	draft, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ValidateDraft(ctx, draft.DraftRevision)
	if err != nil || !result.Valid {
		t.Fatalf("matching DAV workspace rejected: %+v %v", result, err)
	}
	preview, err := svc.PreviewDraft(ctx, draft.DraftRevision)
	if err != nil || preview.DiffSummary.Added < 1 {
		t.Fatal("DAV preview unavailable", err)
	}
	draft.Content.Bindings[len(draft.Content.Bindings)-1].RuntimeRouteId = platformid.New(platformid.Route)
	draft, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content)
	if err != nil {
		t.Fatal(err)
	}
	result, err = svc.ValidateDraft(ctx, draft.DraftRevision)
	if err != nil || result.Valid {
		t.Fatal("mismatched DAV route was accepted", err)
	}
	st.Client.WorkspaceService.UpdateOneID(serviceID).SetDavStaged(true).ExecX(ctx)
	draft.Content.Webdav = nil
	if _, err = svc.PutDraft(ctx, boot.AdminUserID, draft.DraftRevision, draft.Content); err == nil {
		t.Fatal("derived DAV could be removed by an older client omitting the field")
	}
}
