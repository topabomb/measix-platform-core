package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestDAVStatisticsAreIdempotentWithoutBudgetOrSemanticEffects(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	dep, user, _ := seedUsageParents(t, store.Client, now)
	statistics := true
	method := "PUT"
	version := usageingestapi.RequestUsageFactTargetVersion(3)
	event := usageingestapi.UsageSettlement{RequestId: platformid.New(platformid.Request), Revision: 1, EventHash: testHash("DAV"), SourceEventId: "dav-event", OccurredAt: now, Completeness: "EXACT", State: "SETTLED", Meters: []usageingestapi.MeterValue{}, StatisticsOnly: &statistics, Request: usageingestapi.RequestUsageFact{DeploymentId: dep, UserId: user, ResourceId: platformid.New(platformid.WebDAV), ResourceKind: "WEBDAV", ClientProtocol: "WEBDAV", RuntimeRouteId: platformid.New(platformid.Route), ManagedGeneration: 1, ControlRevision: 1, StartedAt: now, CompletedAt: now, Forwarded: true, HttpStatus: 201, HttpMethod: &method, RequestBytes: (4 << 30) + 1, TargetVersion: &version, WorkspaceTarget: &usageingestapi.WorkspaceTarget{WorkspaceServiceId: platformid.New(platformid.WorkspaceService), AgentSpaceId: platformid.New(platformid.AgentSpace), RemoteUsername: "alice", BindingRevision: 1}}}
	svc := NewService(store.Client)
	batch := usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{event}}
	ack, err := svc.IngestSettlements(ctx, batch)
	if err != nil || ack.AcceptedCount != 1 {
		t.Fatal("DAV stats require budget", ack, err)
	}
	ack, err = svc.IngestSettlements(ctx, batch)
	if err != nil || ack.DuplicateCount != 1 {
		t.Fatal("DAV duplicate", ack, err)
	}
	if store.Client.BudgetRequest.Query().CountX(ctx) != 0 || store.Client.SemanticUsage.Query().CountX(ctx) != 0 {
		t.Fatal("DAV created budget/semantic meters")
	}
	view, err := svc.GetRequest(ctx, event.RequestId)
	if err != nil || view.RequestBytes != (4<<30)+1 || view.Budget != nil {
		t.Fatal("DAV request detail", view, err)
	}
	store.Client.DeletedPrincipal.Create().SetID(user).SetDeletedAt(now).SaveX(ctx)
	before := store.Client.RequestUsage.Query().CountX(ctx)
	event.RequestId = platformid.New(platformid.Request)
	event.SourceEventId = "late-dav"
	ack, err = svc.IngestSettlements(ctx, usageingestapi.UsageSettlementBatch{Events: []usageingestapi.UsageSettlement{event}})
	if err != nil || ack.DiscardedCount != 1 || store.Client.RequestUsage.Query().CountX(ctx) != before {
		t.Fatal("late deleted principal was resurrected", ack, err)
	}
	statistics = false
	if validateSettlement(event) == nil {
		t.Fatal("DAV accepted without statistics-only marker")
	}
	statistics = true
	event.Request.ResourceKind = "MCP"
	event.Request.ClientProtocol = "MCP_STREAMABLE_HTTP"
	event.Request.ResourceId = platformid.New(platformid.MCP)
	if validateSettlement(event) == nil {
		t.Fatal("MCP escaped budget through statistics-only marker")
	}
}
