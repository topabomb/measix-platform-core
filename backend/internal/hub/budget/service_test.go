package budget

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"measix/platform/ent"
	"measix/platform/ent/budgetbucket"
	"measix/platform/ent/budgetrequest"
	"measix/platform/internal/hub/testutil"
	"measix/platform/pkg/platformid"
)

type budgetFixture struct {
	client       *ent.Client
	service      *Service
	now          time.Time
	adminID      string
	userID       string
	deploymentID string
	upstreamID   string
}

func newBudgetFixture(t *testing.T, timezone string) *budgetFixture {
	t.Helper()
	store := testutil.OpenStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 20, 3, 0, 0, 0, time.UTC)
	adminID, userID := platformid.New(platformid.User), platformid.New(platformid.User)
	for _, user := range []struct {
		id, username, role string
	}{{adminID, "budget-admin", "ADMIN"}, {userID, "budget-user", "USER"}} {
		if _, err := store.Client.User.Create().
			SetID(user.id).
			SetUsername(user.username).
			SetDisplayName(user.username).
			SetRole(user.role).
			SetStatus("ACTIVE").
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	deploymentID := platformid.New(platformid.Deployment)
	if _, err := store.Client.Deployment.Create().
		SetID(deploymentID).
		SetName("Budget test").
		SetStatus("ACTIVE").
		SetTimezone(timezone).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	upstreamID := platformid.New(platformid.Upstream)
	if _, err := store.Client.Upstream.Create().
		SetID(upstreamID).
		SetName("Budget upstream").
		SetConfigRevision(1).
		SetActiveConfigRevision(1).
		SetStatus("ACTIVE").
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store.Client, timezone)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &budgetFixture{
		client: store.Client, service: service, now: now, adminID: adminID, userID: userID,
		deploymentID: deploymentID, upstreamID: upstreamID,
	}
	service.Now = func() time.Time { return fixture.now }
	return fixture
}

func (f *budgetFixture) admit(requestID string, capability Capability, resourceID string, protocol ClientProtocol, known []MeterQuantity, supported ...Meter) AdmitInput {
	return AdmitInput{
		RequestID: requestID, RequestHash: testHash(requestID), DeploymentID: f.deploymentID,
		UserID: f.userID, Capability: capability, ResourceID: resourceID, ClientProtocol: protocol,
		UpstreamID: f.upstreamID, ManagedGeneration: 7, ControlRevision: 11,
		AdmittedAt: f.now, KnownQuantities: known, SupportedMeters: supported,
	}
}

func TestDefaultUnlimitedAndExactProtocolSet(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	view, err := f.service.Get(ctx, f.userID, CapabilityModel)
	if err != nil {
		t.Fatal(err)
	}
	if view.Mode != ModeUnlimited || view.Source != SourceDefault || view.Revision != 0 || view.ActivatedAt != nil {
		t.Fatalf("default projection = %+v", view)
	}
	protocols := map[ClientProtocol]Capability{
		ProtocolOpenAIChatCompletions: CapabilityModel, ProtocolOpenAIResponses: CapabilityModel,
		ProtocolOpenAIImagesGenerations:       CapabilityImageGeneration,
		ProtocolDashScopeMultimodalGeneration: CapabilityImageGeneration,
		ProtocolGoogleGenerateContent:         CapabilityModel, ProtocolAnthropicMessages: CapabilityModel,
		ProtocolOpenAIAudioSpeech: CapabilityTTS, ProtocolGeminiGenerateContentTTS: CapabilityTTS,
		ProtocolMiMoChatCompletionsTTS: CapabilityTTS, ProtocolOpenAIAudioTranscriptions: CapabilityASR,
		ProtocolDashScopeHTTPASR: CapabilityASR, ProtocolOpenAIRealtimeTranscription: CapabilityASR,
		ProtocolDashScopeRealtimeASR: CapabilityASR, ProtocolMCPStreamableHTTP: CapabilityMCP,
	}
	if len(protocols) != 14 {
		t.Fatalf("protocol count = %d", len(protocols))
	}
	for protocol, capability := range protocols {
		if !protocolMatchesCapability(protocol, capability) {
			t.Fatalf("%s does not match %s", protocol, capability)
		}
	}
	if protocolMatchesCapability("SYSTEM_TTS", CapabilityTTS) {
		t.Fatal("SYSTEM_TTS must not enter server budget admission")
	}
	for _, invalid := range []AdmitInput{
		f.admit(platformid.New(platformid.Request), CapabilityModel, platformid.New(platformid.Model), ProtocolOpenAIResponses,
			[]MeterQuantity{{Meter: MeterRequestedImages, Quantity: 1}}, MeterTotalTokens),
		f.admit(platformid.New(platformid.Request), CapabilityModel, platformid.New(platformid.Model), ProtocolOpenAIResponses,
			nil, MeterRequestedImages),
	} {
		if _, err := f.service.Admit(ctx, invalid); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("cross-capability admission meter error = %v", err)
		}
	}

	requestID := platformid.New(platformid.Request)
	input := f.admit(requestID, CapabilityModel, platformid.New(platformid.Model), ProtocolOpenAIResponses, nil, MeterTotalTokens)
	decision, err := f.service.Admit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Source != SourceDefault || decision.Revision != 0 || decision.InFlightRequests != 1 {
		t.Fatalf("default unlimited admission = %+v", decision)
	}
	duplicate, err := f.service.Admit(ctx, input)
	if err != nil || !reflect.DeepEqual(duplicate, decision) {
		t.Fatalf("idempotent admission = %+v, %v", duplicate, err)
	}
	input.KnownQuantities = []MeterQuantity{{Meter: MeterTotalTokens, Quantity: 1}}
	if _, err := f.service.Admit(ctx, input); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("changed request content error = %v", err)
	}

	start := LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now.Add(time.Second), EventHash: testHash("start")}
	if err := f.service.Start(ctx, start); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Start(ctx, start); err != nil {
		t.Fatalf("duplicate start: %v", err)
	}
	start.EventHash = testHash("different")
	if err := f.service.Start(ctx, start); !errors.Is(err, ErrLifecycleRevisionConflict) {
		t.Fatalf("same lifecycle revision with different hash = %v", err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterRequestedImages, Quantity: 1}},
	}); !errors.Is(err, ErrInvalidSettlement) {
		t.Fatalf("cross-capability settlement meter error = %v", err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 17}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLimitedUnlimitedLimitedKeepsUsageAndPeriodChangeCreatesScope(t *testing.T) {
	f := newBudgetFixture(t, "Asia/Shanghai")
	ctx := context.Background()
	created, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes:      []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterTotalTokens, Limit: 100}}}},
		ActorUserID: f.adminID, Reason: "initial token budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Source != SourceExplicit || created.Revision != 1 || len(created.Scopes) != 1 {
		t.Fatalf("created budget = %+v", created)
	}
	if _, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 0, Mode: ModeUnlimited,
		ActorUserID: f.adminID, Reason: "stale overwrite",
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	resourceID := platformid.New(platformid.Model)
	consume := func(label string, quantity int64) {
		t.Helper()
		requestID := platformid.New(platformid.Request)
		decision, err := f.service.Admit(ctx, f.admit(requestID, CapabilityModel, resourceID, ProtocolOpenAIResponses, nil, MeterTotalTokens))
		if err != nil || !decision.Allowed {
			t.Fatalf("%s admit = %+v, %v", label, decision, err)
		}
		if err := f.service.Start(ctx, LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now, EventHash: testHash(label + "-start")}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Settle(ctx, SettlementInput{
			RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
			Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: quantity}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	consume("limited", 60)
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 1, Mode: ModeUnlimited,
		ActorUserID: f.adminID, Reason: "temporarily unlimited",
	}); err != nil {
		t.Fatal(err)
	}
	consume("unlimited", 30)
	f.now = f.now.Add(time.Minute)
	reenabled, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 2, Mode: ModeLimited,
		ActorUserID: f.adminID, Reason: "restore retained token budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reenabled.Scopes[0].ScopeKey != created.Scopes[0].ScopeKey {
		t.Fatal("limited/unlimited switch reset the stable scope")
	}
	blocked, err := f.service.Admit(ctx, f.admit(platformid.New(platformid.Request), CapabilityModel, resourceID, ProtocolOpenAIResponses,
		[]MeterQuantity{{Meter: MeterTotalTokens, Quantity: 11}}, MeterTotalTokens))
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Allowed || blocked.Code != DecisionBudgetExhausted || len(blocked.BlockingLimits) != 1 || blocked.BlockingLimits[0].Used != 90 {
		t.Fatalf("retained usage decision = %+v", blocked)
	}
	state, err := f.service.State(ctx, f.userID, CapabilityModel)
	if err != nil || state.Status != StatusAvailable || len(state.Limits) != 1 || state.Limits[0].Used != 90 || state.Limits[0].Remaining != 10 {
		t.Fatalf("effective state = %+v, %v", state, err)
	}
	states, err := f.service.UserStates(ctx, f.userID)
	if err != nil || len(states) != 5 {
		t.Fatalf("all capability states = %+v, %v", states, err)
	}
	for _, other := range states {
		if !other.AsOf.Equal(states[0].AsOf) {
			t.Fatal("all-capability snapshot used mixed asOf values")
		}
	}

	f.now = f.now.Add(time.Minute)
	lowered, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 3, Mode: ModeLimited,
		Scopes:      []ScopeInput{{ScopeKey: created.Scopes[0].ScopeKey, Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterTotalTokens, Limit: 80}}}},
		ActorUserID: f.adminID, Reason: "lower limit without clearing usage",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lowered.Scopes[0].ScopeKey != created.Scopes[0].ScopeKey {
		t.Fatal("limit-only edit replaced scope")
	}
	blocked, err = f.service.Admit(ctx, f.admit(platformid.New(platformid.Request), CapabilityModel, resourceID, ProtocolOpenAIResponses, nil, MeterTotalTokens))
	if err != nil || blocked.Allowed || blocked.BlockingLimits[0].Used != 90 {
		t.Fatalf("lowered limit decision = %+v, %v", blocked, err)
	}

	f.now = f.now.Add(time.Minute)
	monthly, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 4, Mode: ModeLimited,
		Scopes:      []ScopeInput{{ScopeKey: created.Scopes[0].ScopeKey, Period: PeriodMonth, Limits: []LimitSpec{{Meter: MeterTotalTokens, Limit: 100}}}},
		ActorUserID: f.adminID, Reason: "change accounting period",
	})
	if err != nil {
		t.Fatal(err)
	}
	if monthly.Scopes[0].ScopeKey == created.Scopes[0].ScopeKey {
		t.Fatal("period change reused old scope")
	}
	if count, err := f.client.BudgetAudit.Query().Count(ctx); err != nil || count != 5 {
		t.Fatalf("audit count = %d, %v", count, err)
	}
	firstAuditPage, err := f.service.ListConfigAudit(ctx, AuditQuery{UserID: f.userID, PageSize: 2})
	if err != nil || len(firstAuditPage.Items) != 2 || firstAuditPage.NextCursor == "" {
		t.Fatalf("first audit page = %+v, %v", firstAuditPage, err)
	}
	secondAuditPage, err := f.service.ListConfigAudit(ctx, AuditQuery{UserID: f.userID, PageSize: 2, Cursor: firstAuditPage.NextCursor})
	if err != nil || len(secondAuditPage.Items) != 2 {
		t.Fatalf("second audit page = %+v, %v", secondAuditPage, err)
	}
}

func TestRemovingAndReaddingLimitKeepsHistoricalScopeUsage(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	created, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes:      []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterTotalTokens, Limit: 100}}}},
		ActorUserID: f.adminID, Reason: "initial daily token budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	requestID := platformid.New(platformid.Request)
	decision, err := f.service.Admit(ctx, f.admit(requestID, CapabilityModel, resourceID, ProtocolOpenAIResponses, nil, MeterTotalTokens))
	if err != nil || !decision.Allowed {
		t.Fatalf("admit = %+v, %v", decision, err)
	}
	if err := f.service.Start(ctx, LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now, EventHash: testHash("remove-readd-start")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 70}},
	}); err != nil {
		t.Fatal(err)
	}

	f.now = f.now.Add(time.Minute)
	if _, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 1, Mode: ModeLimited,
		Scopes:      []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 20}}}},
		ActorUserID: f.adminID, Reason: "temporarily remove token limit",
	}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	readded, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 2, Mode: ModeLimited,
		Scopes:      []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterTotalTokens, Limit: 100}}}},
		ActorUserID: f.adminID, Reason: "restore token limit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(readded.Scopes) != 1 || readded.Scopes[0].ScopeKey != created.Scopes[0].ScopeKey {
		t.Fatalf("re-added scope reset accounting identity: created=%+v readded=%+v", created.Scopes, readded.Scopes)
	}
	state, err := f.service.State(ctx, f.userID, CapabilityModel)
	if err != nil || len(state.Limits) != 1 || state.Limits[0].Used != 70 || state.Limits[0].Remaining != 30 {
		t.Fatalf("re-added limit lost usage: state=%+v err=%v", state, err)
	}
}

func TestConcurrentMultiMeterAdmissionIsAtomicAndReleaseRequiresNoForward(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	_, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityTTS, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes: []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{
			{Meter: MeterRequests, Limit: 1}, {Meter: MeterCharacters, Limit: 4},
		}}}, ActorUserID: f.adminID, Reason: "atomic TTS budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.TTS)
	type result struct {
		requestID string
		decision  AdmissionDecision
		err       error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requestID := platformid.New(platformid.Request)
			decision, err := f.service.Admit(ctx, f.admit(requestID, CapabilityTTS, resourceID, ProtocolOpenAIAudioSpeech,
				[]MeterQuantity{{Meter: MeterCharacters, Quantity: 3}}, MeterCharacters))
			results <- result{requestID: requestID, decision: decision, err: err}
		}()
	}
	wg.Wait()
	close(results)
	var allowed, denied result
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.decision.Allowed {
			allowed = got
		} else {
			denied = got
		}
	}
	if allowed.requestID == "" || denied.requestID == "" || len(denied.decision.BlockingLimits) != 2 {
		t.Fatalf("allowed=%+v denied=%+v", allowed, denied)
	}
	for meter, want := range map[Meter]int64{MeterRequests: 1, MeterCharacters: 3} {
		bucket, err := f.client.BudgetBucket.Query().Where(budgetbucket.MeterEQ(budgetbucket.Meter(meter))).Only(ctx)
		if err != nil || bucket.ReservedQuantity != want || bucket.SettledQuantity != 0 {
			t.Fatalf("%s bucket = %+v, %v", meter, bucket, err)
		}
	}
	release := ReleaseInput{LifecycleInput: LifecycleInput{
		RequestID: allowed.requestID, Revision: 1, OccurredAt: f.now.Add(time.Second), EventHash: testHash("release"),
	}, Reason: "body validation proved no upstream attempt"}
	if err := f.service.Release(ctx, release); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Release(ctx, release); err != nil {
		t.Fatalf("duplicate release: %v", err)
	}
	release.EventHash = testHash("conflicting-release")
	if err := f.service.Release(ctx, release); !errors.Is(err, ErrLifecycleRevisionConflict) {
		t.Fatalf("conflicting release revision = %v", err)
	}
	for _, meter := range []Meter{MeterRequests, MeterCharacters} {
		bucket, err := f.client.BudgetBucket.Query().Where(budgetbucket.MeterEQ(budgetbucket.Meter(meter))).Only(ctx)
		if err != nil || bucket.ReservedQuantity != 0 {
			t.Fatalf("released %s bucket = %+v, %v", meter, bucket, err)
		}
	}

	startedID := platformid.New(platformid.Request)
	decision, err := f.service.Admit(ctx, f.admit(startedID, CapabilityTTS, resourceID, ProtocolOpenAIAudioSpeech,
		[]MeterQuantity{{Meter: MeterCharacters, Quantity: 3}}, MeterCharacters))
	if err != nil || !decision.Allowed {
		t.Fatalf("second admission = %+v, %v", decision, err)
	}
	if err := f.service.Start(ctx, LifecycleInput{RequestID: startedID, Revision: 1, OccurredAt: f.now.Add(time.Second), EventHash: testHash("start-forward")}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Release(ctx, ReleaseInput{LifecycleInput: LifecycleInput{
		RequestID: startedID, Revision: 2, OccurredAt: f.now.Add(2 * time.Second), EventHash: testHash("unsafe-release"),
	}, Reason: "timeout alone"}); !errors.Is(err, ErrAlreadyForwarded) {
		t.Fatalf("release after start = %v", err)
	}
}

func TestDayWeekMonthAndLifetimeBucketsAccumulateAtTheirOwnBoundaries(t *testing.T) {
	f := newBudgetFixture(t, "Asia/Shanghai")
	f.now = time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC) // Wednesday 11:00 local.
	ctx := context.Background()
	scopes := make([]ScopeInput, 0, 4)
	for _, period := range []Period{PeriodDay, PeriodWeek, PeriodMonth, PeriodLifetime} {
		scopes = append(scopes, ScopeInput{Period: period, Limits: []LimitSpec{{Meter: MeterRequests, Limit: 10}}})
	}
	if _, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityMCP, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes: scopes, ActorUserID: f.adminID, Reason: "all natural periods",
	}); err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.MCP)
	consume := func(label string) {
		t.Helper()
		requestID := platformid.New(platformid.Request)
		decision, err := f.service.Admit(ctx, f.admit(requestID, CapabilityMCP, resourceID, ProtocolMCPStreamableHTTP, nil))
		if err != nil || !decision.Allowed {
			t.Fatalf("%s admission = %+v, %v", label, decision, err)
		}
		if err := f.service.Start(ctx, LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now, EventHash: testHash(label + "-start")}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Settle(ctx, SettlementInput{
			RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
			Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	consume("wednesday")
	f.now = f.now.AddDate(0, 0, 1)
	consume("thursday")
	for period, wantBuckets := range map[Period]int{PeriodDay: 2, PeriodWeek: 1, PeriodMonth: 1, PeriodLifetime: 1} {
		rows, err := f.client.BudgetBucket.Query().Where(budgetbucket.PeriodEQ(budgetbucket.Period(period))).All(ctx)
		if err != nil || len(rows) != wantBuckets {
			t.Fatalf("%s bucket count = %d, want %d (%v)", period, len(rows), wantBuckets, err)
		}
		var total int64
		for _, row := range rows {
			total += row.SettledQuantity
		}
		if total != 2 {
			t.Fatalf("%s settled total = %d, want 2", period, total)
		}
	}
}

func TestSettlementRevisionCorrectionAndAuditedManualResolution(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	_, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityModel, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes: []ScopeInput{{Period: PeriodLifetime, Limits: []LimitSpec{
			{Meter: MeterRequests, Limit: 10}, {Meter: MeterTotalTokens, Limit: 100},
		}}}, ActorUserID: f.adminID, Reason: "lifetime correction test",
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	requestID := platformid.New(platformid.Request)
	decision, err := f.service.Admit(ctx, f.admit(requestID, CapabilityModel, resourceID, ProtocolAnthropicMessages, nil, MeterTotalTokens))
	if err != nil || !decision.Allowed {
		t.Fatalf("admission = %+v, %v", decision, err)
	}
	if err := f.service.Start(ctx, LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now, EventHash: testHash("start-correction")}); err != nil {
		t.Fatal(err)
	}
	partial := SettlementInput{
		RequestID: requestID, Revision: 1, Complete: false, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 70}},
	}
	first, err := f.service.Settle(ctx, partial)
	if err != nil || first.Outcome != OutcomeReconciliation {
		t.Fatalf("partial settlement = %+v, %v", first, err)
	}
	openPage, err := f.service.ListOpenReconciliations(ctx, ReconciliationQuery{UserID: &f.userID, PageSize: 10})
	if err != nil || len(openPage.Items) != 1 || len(openPage.Items[0].Allocations) != 2 || openPage.Items[0].RequestState != RequestReconciliation {
		t.Fatalf("open reconciliation page = %+v, %v", openPage, err)
	}
	duplicate, err := f.service.Settle(ctx, partial)
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("duplicate settlement = %+v, %v", duplicate, err)
	}
	partial.Meters[1].Quantity = 71
	if _, err := f.service.Settle(ctx, partial); !errors.Is(err, ErrSettlementRevisionConflict) {
		t.Fatalf("same revision different settlement = %v", err)
	}
	corrected, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 2, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 80}},
	})
	if err != nil || corrected.Outcome != OutcomeCorrected {
		t.Fatalf("reliable late settlement = %+v, %v", corrected, err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 3, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 50}},
	}); err != nil {
		t.Fatal(err)
	}
	tokenBucket, err := f.client.BudgetBucket.Query().Where(budgetbucket.MeterEQ(budgetbucket.Meter(MeterTotalTokens))).Only(ctx)
	if err != nil || tokenBucket.SettledQuantity != 50 {
		t.Fatalf("corrected token bucket = %+v, %v", tokenBucket, err)
	}

	uncertainID := platformid.New(platformid.Request)
	decision, err = f.service.Admit(ctx, f.admit(uncertainID, CapabilityModel, resourceID, ProtocolAnthropicMessages, nil, MeterTotalTokens))
	if err != nil || !decision.Allowed {
		t.Fatalf("uncertain admission = %+v, %v", decision, err)
	}
	if err := f.service.Start(ctx, LifecycleInput{RequestID: uncertainID, Revision: 1, OccurredAt: f.now, EventHash: testHash("uncertain-start")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: uncertainID, Revision: 1, Complete: false, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Resolve(ctx, ResolveInput{
		RequestID: uncertainID, Action: ResolutionReleaseUncertain,
		ActorUserID: f.adminID, Reason: "operator verified provider has no retrievable usage",
	}); err != nil {
		t.Fatal(err)
	}
	request, err := f.client.BudgetRequest.Query().Where(budgetrequest.IDEQ(uncertainID)).Only(ctx)
	if err != nil || RequestState(request.State) != RequestResolved {
		t.Fatalf("resolved request = %+v, %v", request, err)
	}
	if count, err := f.client.BudgetAudit.Query().Count(ctx); err != nil || count != 2 {
		t.Fatalf("configuration + reconciliation audit count = %d, %v", count, err)
	}
	// Reliable data arriving after a human release remains applicable as a
	// correction; it does not erase the human audit trail.
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: uncertainID, Revision: 2, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterTotalTokens, Quantity: 20}},
	}); err != nil {
		t.Fatal(err)
	}
	tokenBucket, err = f.client.BudgetBucket.Query().Where(budgetbucket.MeterEQ(budgetbucket.Meter(MeterTotalTokens))).Only(ctx)
	if err != nil || tokenBucket.SettledQuantity != 70 {
		t.Fatalf("late reliable correction bucket = %+v, %v", tokenBucket, err)
	}
}

func TestUnsupportedLimitedMeterAndAudioUseIntegerMilliseconds(t *testing.T) {
	f := newBudgetFixture(t, "UTC")
	ctx := context.Background()
	_, err := f.service.Put(ctx, PutBudgetInput{
		UserID: f.userID, Capability: CapabilityASR, ExpectedRevision: 0, Mode: ModeLimited,
		Scopes:      []ScopeInput{{Period: PeriodDay, Limits: []LimitSpec{{Meter: MeterAudioMilliseconds, Limit: 1500}}}},
		ActorUserID: f.adminID, Reason: "ASR duration budget",
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.ASR)
	unsupported := f.admit(platformid.New(platformid.Request), CapabilityASR, resourceID, ProtocolOpenAIAudioTranscriptions, nil)
	decision, err := f.service.Admit(ctx, unsupported)
	if err != nil || decision.Allowed || decision.Code != DecisionMeterUnavailable || len(decision.Unavailable) != 1 || decision.Unavailable[0] != MeterAudioMilliseconds {
		t.Fatalf("unavailable meter decision = %+v, %v", decision, err)
	}

	requestID := platformid.New(platformid.Request)
	decision, err = f.service.Admit(ctx, f.admit(requestID, CapabilityASR, resourceID, ProtocolOpenAIAudioTranscriptions,
		[]MeterQuantity{{Meter: MeterAudioMilliseconds, Quantity: 1000}}, MeterAudioMilliseconds))
	if err != nil || !decision.Allowed {
		t.Fatalf("1000ms admission = %+v, %v", decision, err)
	}
	if err := f.service.Start(ctx, LifecycleInput{RequestID: requestID, Revision: 1, OccurredAt: f.now, EventHash: testHash("audio-start")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Settle(ctx, SettlementInput{
		RequestID: requestID, Revision: 1, Complete: true, ReportedBy: "runtime-relay",
		Meters: []MeterQuantity{{Meter: MeterRequests, Quantity: 1}, {Meter: MeterAudioMilliseconds, Quantity: 1000}},
	}); err != nil {
		t.Fatal(err)
	}
	blocked, err := f.service.Admit(ctx, f.admit(platformid.New(platformid.Request), CapabilityASR, resourceID, ProtocolOpenAIAudioTranscriptions,
		[]MeterQuantity{{Meter: MeterAudioMilliseconds, Quantity: 501}}, MeterAudioMilliseconds))
	if err != nil || blocked.Allowed || blocked.BlockingLimits[0].Used != 1000 || blocked.BlockingLimits[0].Limit != 1500 {
		t.Fatalf("millisecond budget decision = %+v, %v", blocked, err)
	}
}

func testHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
