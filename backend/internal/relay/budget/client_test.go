package budget

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"measix/platform/internal/wire/usageingestapi"
)

func TestDefaultBudgetClientReusesConnectionsAcrossHundredRequestBursts(t *testing.T) {
	const concurrency = 100
	type barrier struct {
		entered atomic.Int64
		ready   chan struct{}
		release chan struct{}
	}
	newBarrier := func() *barrier { return &barrier{ready: make(chan struct{}), release: make(chan struct{})} }
	first, second := newBarrier(), newBarrier()
	var phase atomic.Int64
	phase.Store(1)
	var connections atomic.Int64

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := first
		if phase.Load() == 2 {
			current = second
		}
		if current.entered.Add(1) == concurrency {
			close(current.ready)
		}
		<-current.release
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"requestId":"req_00000000-0000-4000-8000-000000000000","allowed":true,"code":"ALLOWED","mode":"UNLIMITED","source":"DEFAULT","revision":0,"inFlightRequests":1,"blockingLimits":[],"asOf":"2026-09-20T00:00:00Z"}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "service-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	runWave := func(current *barrier) {
		t.Helper()
		errors := make(chan error, concurrency)
		var group sync.WaitGroup
		group.Add(concurrency)
		for range concurrency {
			go func() {
				defer group.Done()
				_, problem, err := client.Admit(context.Background(), usageingestapi.BudgetAdmissionRequest{})
				if problem != nil {
					err = fmt.Errorf("unexpected problem: %+v", *problem)
				}
				errors <- err
			}()
		}
		select {
		case <-current.ready:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d/%d requests reached Hub", current.entered.Load(), concurrency)
		}
		close(current.release)
		group.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	runWave(first)
	phase.Store(2)
	runWave(second)
	if got := connections.Load(); got > concurrency+5 {
		t.Fatalf("two 100-request bursts opened %d Hub connections; want reuse after the first burst", got)
	}
}
