package metering_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	relaybudget "measix/platform/internal/relay/budget"
	"measix/platform/internal/relay/metering"
	"measix/platform/internal/wire/usageingestapi"
	"measix/platform/pkg/platformid"
)

func TestRecoverLifecycleReleasesUnstartedAndReconcilesStarted(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	now := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)
	unstarted := recoveryAdmission(now)
	started := recoveryAdmission(now.Add(time.Second))
	if err := recorder.PersistAdmission(unstarted, platformid.New(platformid.Route)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.PersistAdmission(started, platformid.New(platformid.Route)); err != nil {
		t.Fatal(err)
	}
	startedAt := now.Add(2 * time.Second)
	if err := recorder.MarkStarted(started.RequestId, startedAt); err != nil {
		t.Fatal(err)
	}

	client := &recoveryBudgetClient{}
	if err := metering.RecoverLifecycle(ctx, spool, client, recorder); err != nil {
		t.Fatal(err)
	}
	if len(client.releases) != 1 || client.releases[0].id != unstarted.RequestId || client.releases[0].event.Revision != 1 {
		t.Fatalf("unexpected release calls: %+v", client.releases)
	}
	if len(client.starts) != 1 || client.starts[0].id != started.RequestId || !client.starts[0].event.OccurredAt.Equal(startedAt) {
		t.Fatalf("unexpected start replays: %+v", client.starts)
	}
	pending, err := spool.Pending(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].RequestID != started.RequestId {
		t.Fatalf("unexpected pending settlements: %+v err=%v", pending, err)
	}
	if err := metering.RecoverLifecycle(ctx, spool, client, recorder); err != nil {
		t.Fatal(err)
	}
	if len(client.releases) != 1 || len(client.starts) != 1 {
		t.Fatalf("completed recovery replayed lifecycle: releases=%d starts=%d", len(client.releases), len(client.starts))
	}
}

func TestRecoverLifecycleDrainsMoreThanOneRecoveryBatch(t *testing.T) {
	ctx := context.Background()
	spool, err := metering.OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	recorder := metering.NewRecorder(spool)
	now := time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)
	for index := 0; index < 1001; index++ {
		admission := recoveryAdmission(now.Add(time.Duration(index) * time.Nanosecond))
		if err := recorder.PersistAdmission(admission, platformid.New(platformid.Route)); err != nil {
			t.Fatal(err)
		}
	}
	client := &recoveryBudgetClient{}
	if err := metering.RecoverLifecycle(ctx, spool, client, recorder); err != nil {
		t.Fatal(err)
	}
	if len(client.releases) != 1001 {
		t.Fatalf("released %d admissions, want 1001", len(client.releases))
	}
	remaining, err := spool.Recoverable(ctx, 1000)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("recovery left %d rows: %v", len(remaining), err)
	}
}

func recoveryAdmission(now time.Time) usageingestapi.BudgetAdmissionRequest {
	return usageingestapi.BudgetAdmissionRequest{
		RequestId: platformid.New(platformid.Request), RequestHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DeploymentId: platformid.New(platformid.Deployment), UserId: platformid.New(platformid.User),
		DeviceId: ptr(platformid.New(platformid.Device)), InteractionId: ptr(platformid.New(platformid.Interaction)),
		ResourceId: platformid.New(platformid.Model), ResourceKind: usageingestapi.ResourceKindMODEL,
		ClientProtocol: usageingestapi.OPENAIRESPONSES, UpstreamId: platformid.New(platformid.Upstream),
		ManagedGeneration: 1, ControlRevision: 1, AdmittedAt: now,
		SupportedMeters: []usageingestapi.UsageMeter{usageingestapi.REQUESTS, usageingestapi.INPUTTOKENS},
	}
}

type recoveryBudgetClient struct {
	starts []struct {
		id    string
		event usageingestapi.BudgetLifecycleEvent
	}
	releases []struct {
		id    string
		event usageingestapi.BudgetReleaseRequest
	}
}

func (c *recoveryBudgetClient) Admit(context.Context, usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	return usageingestapi.BudgetAdmissionDecision{}, nil, relaybudget.ErrUnavailable
}

func (c *recoveryBudgetClient) Start(_ context.Context, id string, event usageingestapi.BudgetLifecycleEvent) error {
	c.starts = append(c.starts, struct {
		id    string
		event usageingestapi.BudgetLifecycleEvent
	}{id, event})
	return nil
}

func (c *recoveryBudgetClient) Release(_ context.Context, id string, event usageingestapi.BudgetReleaseRequest) error {
	c.releases = append(c.releases, struct {
		id    string
		event usageingestapi.BudgetReleaseRequest
	}{id, event})
	return nil
}

func ptr(value string) *string { return &value }
