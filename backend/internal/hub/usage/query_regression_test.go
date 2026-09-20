package usage

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func TestUsageFiltersCursorSummaryEnrichmentAndUnknownRegression(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	deploymentID, userA, upstreamID := seedUsageParents(t, store.Client, now)
	userB := platformid.New(platformid.User)
	if _, err := store.Client.User.Create().SetID(userB).SetUsername(userB).SetDisplayName("Second User").SetRole("MEMBER").SetStatus("ACTIVE").SetCreatedAt(now).SetUpdatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	deviceID := platformid.New(platformid.Device)
	if _, err := store.Client.Device.Create().SetID(deviceID).SetUserID(userA).SetName("Alice phone").SetStatus("ACTIVE").SetCreatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}
	modelID := platformid.New(platformid.Model)
	ttsID := platformid.New(platformid.TTS)
	releaseID := platformid.New(platformid.Release)
	snapshot, err := json.Marshal(map[string]any{
		"managedGeneration": 1, "releaseId": releaseID, "snapshotHash": "snapshot-hash",
		"models": []map[string]any{{"modelId": modelID, "displayName": "Managed Model"}},
		"tts":    []any{}, "asr": []any{}, "mcp": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Client.ManagedRelease.Create().SetID(releaseID).SetManagedGeneration(1).SetStatus("ACTIVE").
		SetReleaseContentJSON([]byte(`{}`)).SetSnapshotJSON(snapshot).SetSnapshotHash("snapshot-hash").
		SetSourceDraftRevision(1).SetCreatedByUserID(userA).SetCreatedAt(now).Save(ctx); err != nil {
		t.Fatal(err)
	}

	exactID := createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userA, DeviceID: &deviceID, ResourceID: modelID, ResourceKind: "MODEL",
		Protocol: "OPENAI_RESPONSES", UpstreamID: upstreamID, CompletedAt: now, Forwarded: true, HTTPStatus: 200,
		Completeness: "EXACT", RequestBytes: 10, ResponseBytes: 20,
	})
	blockedID := createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userA, ResourceID: modelID, ResourceKind: "MODEL",
		Protocol: "OPENAI_RESPONSES", UpstreamID: upstreamID, CompletedAt: now.Add(-time.Second), Forwarded: false, HTTPStatus: 429,
		Completeness: "EXACT", RequestBytes: 11, ResponseBytes: 0,
	})
	createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userB, ResourceID: ttsID, ResourceKind: "TTS",
		Protocol: "OPENAI_AUDIO_SPEECH", UpstreamID: upstreamID, CompletedAt: now.Add(-2 * time.Second), Forwarded: true, HTTPStatus: 500,
		Completeness: "PARTIAL", RequestBytes: 12, ResponseBytes: 30,
	})
	unknownID := createUsageRequestRow(t, store.Client, requestRowInput{
		DeploymentID: deploymentID, UserID: userA, ResourceID: modelID, ResourceKind: "MODEL",
		Protocol: "OPENAI_RESPONSES", UpstreamID: upstreamID, CompletedAt: now.Add(-3 * time.Second), Forwarded: true, HTTPStatus: 200,
		Completeness: "UNKNOWN", RequestBytes: 13, ResponseBytes: 5,
	})
	createSemanticUsageRow(t, store.Client, exactID, "INPUT_TOKENS", 10, "EXACT", now)
	createSemanticUsageRow(t, store.Client, unknownID, "INPUT_TOKENS", 5, "UNKNOWN", now.Add(-3*time.Second))

	service := NewService(store.Client)
	service.Now = func() time.Time { return now.Add(time.Second) }
	combined, err := service.ListRequests(ctx, Filter{
		UserID: userA, ResourceID: modelID, ResourceKind: ResourceKindModel, UpstreamID: upstreamID,
		Status: RequestStatusSuccess, Completeness: CompletenessComplete, ClientProtocol: "OPENAI_RESPONSES",
	}, 50)
	if err != nil || len(combined) != 1 || combined[0].RequestID != exactID {
		t.Fatalf("combined filter = %+v err=%v", combined, err)
	}
	if combined[0].UserDisplayName != "Usage User" || combined[0].DeviceName != "Alice phone" || combined[0].ResourceDisplayName != "Managed Model" || len(combined[0].SemanticMeters) != 1 {
		t.Fatalf("enriched request = %+v", combined[0])
	}
	blocked, err := service.ListRequests(ctx, Filter{Status: RequestStatusBlocked, Completeness: CompletenessComplete}, 50)
	if err != nil || len(blocked) != 1 || blocked[0].RequestID != blockedID {
		t.Fatalf("blocked filter = %+v err=%v", blocked, err)
	}
	errorRows, err := service.ListRequests(ctx, Filter{Status: RequestStatusError, ResourceKind: ResourceKindTTS}, 50)
	if err != nil || len(errorRows) != 1 || errorRows[0].HTTPStatus != 500 {
		t.Fatalf("error filter = %+v err=%v", errorRows, err)
	}

	firstPage, err := service.ListRequests(ctx, Filter{}, 3)
	if err != nil || len(firstPage) != 3 {
		t.Fatalf("first page = %+v err=%v", firstPage, err)
	}
	cursor := firstPage[2].CompletedAt.UTC().Format(time.RFC3339Nano) + "|" + firstPage[2].RequestID
	secondPage, err := service.ListRequests(ctx, Filter{After: cursor}, 3)
	if err != nil || len(secondPage) != 1 || secondPage[0].RequestID != unknownID {
		t.Fatalf("second page = %+v err=%v", secondPage, err)
	}
	if _, err := service.ListRequests(ctx, Filter{After: "not-a-cursor"}, 2); !errors.Is(err, ErrInvalidBatch) {
		t.Fatalf("invalid cursor error = %v", err)
	}

	summary, err := service.Summary(ctx, Filter{UserID: userA})
	if err != nil || summary.RequestCount != 3 || summary.ForwardedRequestCount != 2 || summary.RequestBytes != 34 || summary.ResponseBytes != 25 ||
		summary.RequestCompleteness.Exact != 2 || summary.RequestCompleteness.Unknown != 1 || len(summary.Meters) != 1 || summary.Meters[0].Quantity != "15" || summary.Meters[0].Confidence != CompletenessUnknown {
		t.Fatalf("user summary = %+v err=%v", summary, err)
	}
	unknown, err := service.UnknownRequestCount(ctx)
	if err != nil || unknown != 1 {
		t.Fatalf("unknown request count = %d err=%v", unknown, err)
	}
}

type requestRowInput struct {
	DeploymentID, UserID, ResourceID, ResourceKind, Protocol, UpstreamID string
	DeviceID                                                             *string
	CompletedAt                                                          time.Time
	Forwarded                                                            bool
	HTTPStatus                                                           int
	Completeness                                                         string
	RequestBytes, ResponseBytes                                          int64
}

func createUsageRequestRow(t *testing.T, client *ent.Client, input requestRowInput) string {
	t.Helper()
	requestID := platformid.New(platformid.Request)
	if _, err := client.RequestUsage.Create().SetRequestID(requestID).SetDeploymentID(input.DeploymentID).SetUserID(input.UserID).
		SetNillableDeviceID(input.DeviceID).SetResourceID(input.ResourceID).SetResourceKind(input.ResourceKind).SetClientProtocol(input.Protocol).
		SetRuntimeRouteID(platformid.New(platformid.Route)).SetUpstreamID(input.UpstreamID).SetManagedGeneration(1).SetControlRevision(1).
		SetStartedAt(input.CompletedAt.Add(-time.Second)).SetCompletedAt(input.CompletedAt).SetForwarded(input.Forwarded).SetHTTPStatus(input.HTTPStatus).
		SetRequestBytes(input.RequestBytes).SetResponseBytes(input.ResponseBytes).SetDurationMs(1000).SetRequestCompleteness(input.Completeness).
		SetSettlementState("SETTLED").SetSettlementRevision(1).SetBudgetRevision(0).SetIngestedAt(input.CompletedAt).Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	return requestID
}

func createSemanticUsageRow(t *testing.T, client *ent.Client, requestID, meter string, quantity int64, completeness string, occurredAt time.Time) {
	t.Helper()
	if _, err := client.SemanticUsage.Create().SetID(requestID + ":" + meter).SetRequestID(requestID).SetSettlementRevision(1).
		SetSourceEventID(requestID + ":1").SetMeter(meter).SetQuantityUnits(quantity).SetQuantityDecimal(strconv.FormatInt(quantity, 10)).
		SetCompleteness(completeness).SetSource("test").SetOccurredAt(occurredAt).Save(context.Background()); err != nil {
		t.Fatal(err)
	}
}
