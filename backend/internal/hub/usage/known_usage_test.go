package usage

import (
	"context"
	"testing"
	"time"

	"measix/platform/ent/requestusage"
	"measix/platform/internal/hub/budget"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

func TestKnownUsageSurvivesUnknownAcrossReadModels(t *testing.T) {
	for _, tc := range []struct {
		kind, protocol, meter, quantity string
		prefix                          platformid.Kind
		units                           int64
	}{
		{"MODEL", "OPENAI_CHAT_COMPLETIONS", "TOTAL_TOKENS", "120", platformid.Model, 120},
		{"TTS", "OPENAI_AUDIO_SPEECH", "CHARACTERS", "120", platformid.TTS, 120},
		{"ASR", "OPENAI_AUDIO_TRANSCRIPTIONS", "AUDIO_SECONDS", "0.12", platformid.ASR, 120},
		{"IMAGE_GENERATION", "OPENAI_IMAGES_GENERATIONS", "REQUESTED_IMAGES", "120", platformid.ImageGeneration, 120},
		{"MCP", "MCP_STREAMABLE_HTTP", "REQUESTS", "120", platformid.MCP, 120},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			store := testutil.OpenStore(t)
			ctx := context.Background()
			now := time.Date(2026, 10, 14, 6, 0, 0, 0, time.UTC)
			deployment, user, upstream := seedUsageParents(t, store.Client, now)
			resource := platformid.New(tc.prefix)
			for _, state := range []string{"EXACT", "UNKNOWN"} {
				id := createUsageRequestRow(t, store.Client, requestRowInput{DeploymentID: deployment, UserID: user, UpstreamID: upstream, ResourceID: resource, ResourceKind: tc.kind, Protocol: tc.protocol, CompletedAt: now.Add(-time.Minute), Forwarded: true, HTTPStatus: 200, Completeness: state})
				units := tc.units
				if state == "UNKNOWN" {
					units = 999
				} // Unreliable stored numbers must not enter a known subtotal.
				createSemanticUsageRow(t, store.Client, id, tc.meter, units, state, now.Add(-time.Minute))
			}
			service := NewService(store.Client)
			service.Now = func() time.Time { return now }
			from, to := now.Add(-time.Hour), now
			filter := Filter{From: &from, To: &to}
			check := func(label string, meters []MeterSummary) {
				t.Helper()
				if len(meters) != 1 || meters[0].Quantity != tc.quantity || meters[0].Confidence != CompletenessPartial {
					t.Errorf("%s: %+v", label, meters)
				}
			}
			summary, err := service.Summary(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			check("summary", summary.Meters)
			trend, err := service.Trend(ctx, filter, time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			check("trend", trend.Points[0].Meters)
			distribution, err := service.Distribution(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			check("distribution", distribution.Items[0].Meters)
			budgets, err := budget.NewService(store.Client, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			users, err := service.ListUsers(ctx, filter, budgets, "", 50, "")
			if err != nil {
				t.Fatal(err)
			}
			check("user", users.Items[0].Meters)
			check("capability", users.Items[0].UsageMeters[budget.Capability(tc.kind)])
			unknown, err := service.Summary(ctx, Filter{From: &from, To: &to, Completeness: CompletenessUnknown})
			if err != nil {
				t.Fatal(err)
			}
			if len(unknown.Meters) != 1 || unknown.Meters[0].Confidence != CompletenessUnknown || unknown.Meters[0].Quantity != "0" {
				t.Errorf("all unknown: %+v", unknown.Meters)
			}
			requests, err := service.ListRequests(ctx, filter, 50)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range requests {
				if r.RequestCompleteness == CompletenessUnknown && (r.SemanticMeters[0].Confidence != CompletenessUnknown || r.SemanticMeters[0].Quantity != "999") {
					t.Fatal("individual unknown was rewritten")
				}
			}
		})
	}
}

func TestRequestStatusIncludesErrorsAfterHTTPHeaders(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 14, 6, 0, 0, 0, time.UTC)
	deployment, user, upstream := seedUsageParents(t, store.Client, now)
	for _, code := range []string{"", "CLIENT_CANCELLED", "UPSTREAM_TIMEOUT", "UPSTREAM_UNAVAILABLE"} {
		id := createUsageRequestRow(t, store.Client, requestRowInput{DeploymentID: deployment, UserID: user, UpstreamID: upstream, ResourceID: platformid.New(platformid.Model), ResourceKind: "MODEL", Protocol: "OPENAI_CHAT_COMPLETIONS", CompletedAt: now.Add(-time.Minute), Forwarded: true, HTTPStatus: 200, Completeness: "EXACT"})
		store.Client.RequestUsage.Update().Where(requestusage.RequestIDEQ(id)).SetErrorClass(code).ExecX(ctx)
	}
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	for status, want := range map[RequestStatus]int{RequestStatusSuccess: 1, RequestStatusError: 3} {
		rows, err := service.ListRequests(ctx, Filter{Status: status}, 50)
		if err != nil || len(rows) != want {
			t.Errorf("%s: count=%d err=%v", status, len(rows), err)
		}
	}
	budgets, err := budget.NewService(store.Client, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.ListUsers(ctx, Filter{}, budgets, "", 50, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ErrorRequestCount != 3 {
		t.Fatalf("user errors: %+v err=%v", page, err)
	}
}

func TestAggregateKeepsKnownZeroAndLatestPartialCorrection(t *testing.T) {
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 14, 6, 0, 0, 0, time.UTC)
	deployment, user, upstream := seedUsageParents(t, store.Client, now)
	resource := platformid.New(platformid.TTS)
	for _, state := range []string{"EXACT", "UNKNOWN"} {
		id := createUsageRequestRow(t, store.Client, requestRowInput{DeploymentID: deployment, UserID: user, UpstreamID: upstream, ResourceID: resource, ResourceKind: "TTS", Protocol: "OPENAI_AUDIO_SPEECH", CompletedAt: now.Add(-time.Minute), Forwarded: true, HTTPStatus: 200, Completeness: state})
		createSemanticUsageRow(t, store.Client, id, "CHARACTERS", 0, state, now.Add(-time.Minute))
	}
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	summary, err := service.Summary(ctx, Filter{})
	if err != nil || len(summary.Meters) != 1 || summary.Meters[0].Quantity != "0" || summary.Meters[0].Confidence != CompletenessPartial {
		t.Fatalf("known zero lost: %+v err=%v", summary.Meters, err)
	}
	// A reliable correction is read by its existing revision, not added again.
	corrected := createUsageRequestRow(t, store.Client, requestRowInput{DeploymentID: deployment, UserID: user, UpstreamID: upstream, ResourceID: resource, ResourceKind: "TTS", Protocol: "OPENAI_AUDIO_SPEECH", CompletedAt: now.Add(-time.Minute), Forwarded: true, HTTPStatus: 200, Completeness: "PARTIAL"})
	createSemanticUsageRow(t, store.Client, corrected, "CHARACTERS", 99, "EXACT", now.Add(-time.Minute))
	store.Client.SemanticUsage.Create().SetID(corrected + ":2:CHARACTERS").SetRequestID(corrected).SetSettlementRevision(2).
		SetSourceEventID(corrected + ":2").SetMeter("CHARACTERS").SetQuantityUnits(7).SetQuantityDecimal("7").
		SetCompleteness("PARTIAL").SetSource("test").SetOccurredAt(now.Add(-time.Minute)).SaveX(ctx)
	store.Client.RequestUsage.Update().Where(requestusage.RequestIDEQ(corrected)).SetSettlementRevision(2).ExecX(ctx)
	summary, err = service.Summary(ctx, Filter{})
	if err != nil || len(summary.Meters) != 1 || summary.Meters[0].Quantity != "7" || summary.Meters[0].Confidence != CompletenessPartial {
		t.Fatalf("latest partial correction: %+v err=%v", summary.Meters, err)
	}
}
