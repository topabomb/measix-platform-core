package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/internal/hub/testutil"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestHUBI5ListRequestsAppliesCombinationFilters(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }

	// user A + user B share one upstream.
	userA, upstreamID := seedUsageParents(t, store.Client, now)
	userB, _ := seedUsageParents(t, store.Client, now)

	modelRes := platformid.New(platformid.Model)
	ttsRes := platformid.New(platformid.TTS)
	ingest := func(user, res string, forwarded bool, status int) {
		e := validRequestUsageEvent(now, user, upstreamID)
		e.ResourceId = res
		e.Forwarded = forwarded
		e.HttpStatus = status
		if !forwarded {
			e.UpstreamHttpStatus = nil
		}
		if _, err := service.Ingest(context.Background(), usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{e}}); err != nil {
			t.Fatal(err)
		}
	}
	ingest(userA, modelRes, true, 200)  // success model
	ingest(userA, modelRes, false, 403) // blocked model (not forwarded)
	ingest(userB, ttsRes, true, 500)    // error tts (forwarded but 500)

	// Filter by user.
	onlyUserA, err := service.ListRequests(context.Background(), Filter{UserID: userA}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyUserA) != 2 {
		t.Fatalf("userA requests=%d want 2", len(onlyUserA))
	}

	// Filter by resource kind.
	onlyModels, err := service.ListRequests(context.Background(), Filter{ResourceKind: ResourceKindModel}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyModels) != 2 {
		t.Fatalf("model requests=%d want 2", len(onlyModels))
	}

	// Filter by status SUCCESS.
	onlySuccess, err := service.ListRequests(context.Background(), Filter{Status: RequestStatusSuccess}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlySuccess) != 1 || onlySuccess[0].HTTPStatus != 200 {
		t.Fatalf("success requests=%+v want 1 with 200", onlySuccess)
	}

	// Combination: user A + resource kind model + status success.
	combined, err := service.ListRequests(context.Background(), Filter{UserID: userA, ResourceKind: ResourceKindModel, Status: RequestStatusSuccess}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(combined) != 1 {
		t.Fatalf("combined requests=%d want 1", len(combined))
	}

	// Status ERROR includes forwarded requests with HTTP >= 400.
	onlyErrors, err := service.ListRequests(context.Background(), Filter{Status: RequestStatusError}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyErrors) != 1 || onlyErrors[0].HTTPStatus != 500 {
		t.Fatalf("error requests=%+v want 1 with 500", onlyErrors)
	}
}
