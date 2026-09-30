package relay_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relaybudget "measix/platform/internal/relay/budget"
	relayruntime "measix/platform/internal/relay/runtime"
	"measix/platform/internal/wire/usageingestapi"
)

type unavailableBudgetClient struct{}

func (*unavailableBudgetClient) Admit(context.Context, usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	return usageingestapi.BudgetAdmissionDecision{}, nil, relaybudget.ErrUnavailable
}
func (*unavailableBudgetClient) Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error {
	return relaybudget.ErrUnavailable
}
func (*unavailableBudgetClient) Release(context.Context, string, usageingestapi.BudgetReleaseRequest) error {
	return relaybudget.ErrUnavailable
}

type legacyInFlightLimitBudgetClient struct{}

func (*legacyInFlightLimitBudgetClient) Admit(_ context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	return usageingestapi.BudgetAdmissionDecision{
		RequestId: input.RequestId, Allowed: false, Code: usageingestapi.INFLIGHTLIMIT,
		Mode: usageingestapi.UNLIMITED, Source: usageingestapi.DEFAULT, AsOf: input.AdmittedAt,
	}, nil, nil
}
func (*legacyInFlightLimitBudgetClient) Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error {
	return nil
}
func (*legacyInFlightLimitBudgetClient) Release(context.Context, string, usageingestapi.BudgetReleaseRequest) error {
	return nil
}

type failingAdmissionRecorder struct{}

func (*failingAdmissionRecorder) PersistAdmission(usageingestapi.BudgetAdmissionRequest, string) error {
	return errors.New("metering spool unavailable")
}
func (*failingAdmissionRecorder) MarkStarted(string, time.Time) error { return nil }
func (*failingAdmissionRecorder) AbortAdmission(string) error         { return nil }
func (*failingAdmissionRecorder) Record(usageingestapi.UsageSettlement) error {
	return nil
}
func (*failingAdmissionRecorder) RecordDenied(usageingestapi.BudgetAdmissionRequest, string, usageingestapi.UsageSettlement) error {
	return nil
}

type releasingBudgetClient struct {
	releases atomic.Int64
}

func (*releasingBudgetClient) Admit(_ context.Context, input usageingestapi.BudgetAdmissionRequest) (usageingestapi.BudgetAdmissionDecision, *usageingestapi.Problem, error) {
	return usageingestapi.BudgetAdmissionDecision{
		RequestId: input.RequestId, Allowed: true, Code: usageingestapi.ALLOWED,
		Mode: usageingestapi.UNLIMITED, Source: usageingestapi.DEFAULT, AsOf: input.AdmittedAt,
	}, nil, nil
}
func (*releasingBudgetClient) Start(context.Context, string, usageingestapi.BudgetLifecycleEvent) error {
	return nil
}
func (c *releasingBudgetClient) Release(context.Context, string, usageingestapi.BudgetReleaseRequest) error {
	c.releases.Add(1)
	return nil
}

func TestBudgetServiceFailureDoesNotBlockRuntime(t *testing.T) {
	var forwarded atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "secret")
	defer fixture.close()
	fixture.server.Close()
	fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, fixture.recorder, &unavailableBudgetClient{}))

	response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || forwarded.Load() != 1 {
		t.Fatalf("budget outage blocked runtime: status=%d forwarded=%d", response.StatusCode, forwarded.Load())
	}
}

func TestMeteringJournalFailureDoesNotBlockRuntime(t *testing.T) {
	var forwarded atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "secret")
	defer fixture.close()
	fixture.server.Close()
	budgetClient := &releasingBudgetClient{}
	fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, &failingAdmissionRecorder{}, budgetClient))

	response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || forwarded.Load() != 1 || budgetClient.releases.Load() != 1 {
		t.Fatalf("journal outage blocked runtime: status=%d forwarded=%d releases=%d", response.StatusCode, forwarded.Load(), budgetClient.releases.Load())
	}
}

func TestLegacyMeteringInFlightLimitDoesNotBlockRuntime(t *testing.T) {
	var forwarded atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()
	fixture, resourceID := singleRouteFixture(t, upstream.URL, "secret")
	defer fixture.close()
	fixture.server.Close()
	fixture.server = httptest.NewServer(relayruntime.NewHandler(fixture.store, fixture.recorder, &legacyInFlightLimitBudgetClient{}))

	response, err := http.DefaultClient.Do(fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`), "application/json"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || forwarded.Load() != 1 {
		t.Fatalf("legacy in-flight metering limit blocked runtime: status=%d forwarded=%d", response.StatusCode, forwarded.Load())
	}
}
