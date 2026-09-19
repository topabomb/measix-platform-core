package usage

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/clientapi"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestUsageResourceNameComesFromRequestGeneration(t *testing.T) {
	st := testutil.OpenStore(t)
	ctx, now := context.Background(), time.Now().UTC()
	userID, upstreamID := seedUsageParents(t, st.Client, now)
	service := NewService(st.Client)
	fixture, err := os.ReadFile("../../../../api/fixtures/client-integration/snapshot-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot clientapi.ManagedSnapshot
	if err := json.Unmarshal(fixture, &snapshot); err != nil {
		t.Fatal(err)
	}
	modelID := snapshot.Models[0].ModelId
	for i, name := range []string{"Earlier model name", "Renamed model"} {
		generation := i + 1
		snapshot.ManagedGeneration, snapshot.ReleaseId = generation, platformid.New(platformid.Release)
		snapshot.Models[0].DisplayName = name
		body, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.Client.ManagedRelease.Create().SetID(snapshot.ReleaseId).SetManagedGeneration(int64(generation)).SetStatus("ACTIVE").SetReleaseContentJSON([]byte(`{}`)).SetSnapshotJSON(body).SetSnapshotHash(snapshot.SnapshotHash).SetSourceDraftRevision(int64(generation)).SetCreatedByUserID(userID).SetCreatedAt(now).Save(ctx); err != nil {
			t.Fatal(err)
		}
		event := validRequestUsageEvent(now, userID, upstreamID)
		event.ManagedGeneration, event.ResourceId = generation, modelID
		if _, err := service.Ingest(ctx, usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{event}}); err != nil {
			t.Fatal(err)
		}
		view, err := service.GetRequest(ctx, event.RequestId)
		if err != nil || view.ResourceDisplayName != name {
			t.Fatalf("detail name=%q want=%q err=%v", view.ResourceDisplayName, name, err)
		}
	}
	views, err := service.ListRequests(ctx, Filter{}, 50)
	if err != nil || len(views) != 2 {
		t.Fatalf("views=%v err=%v", views, err)
	}
	for _, view := range views {
		want := "Earlier model name"
		if view.ManagedGeneration == 2 {
			want = "Renamed model"
		}
		if view.ResourceDisplayName != want {
			t.Fatalf("generation=%d name=%q want=%q", view.ManagedGeneration, view.ResourceDisplayName, want)
		}
	}
}
