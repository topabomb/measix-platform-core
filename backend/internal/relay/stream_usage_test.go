package relay_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relayruntime "measix/platform/internal/relay/runtime"
)

// RLY-TRN-004/005: a real interrupted stream still produces exactly one usage fact.
func TestInterruptedStreamRecordsUsage(t *testing.T) {
	for _, clientCancel := range []bool{false, true} {
		name, wantClass := "upstream_break", "UPSTREAM_UNAVAILABLE"
		if clientCancel {
			name, wantClass = "client_cancel", "CLIENT_CANCELLED"
		}
		t.Run(name, func(t *testing.T) {
			abort := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: first\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-abort:
					panic(http.ErrAbortHandler)
				case <-r.Context().Done():
				}
			}))
			defer upstream.Close()
			fixture, resourceID := singleRouteFixture(t, upstream.URL, "runtime-secret")
			fixture.server.Close()
			recorder := &captureUsageRecorder{}
			handler := relayruntime.NewHandlerWithRecorder(fixture.store, recorder)
			finished := make(chan struct{})
			fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				handler.ServeHTTP(w, r)
			}))
			defer fixture.close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			request := fixture.request(t, ctx, http.MethodPost, resourceID, "/v1/chat/completions", http.NoBody, "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if _, err := io.ReadFull(response.Body, make([]byte, len("data: first\n\n"))); err != nil {
				t.Fatal(err)
			}
			if clientCancel {
				cancel()
			} else {
				close(abort)
			}
			if _, err := io.ReadAll(response.Body); err == nil {
				t.Fatal("interrupted stream appeared complete")
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("relay did not finish")
			}
			events := recorder.snapshot()
			if len(events) != 1 {
				t.Fatalf("interrupted stream produced %d usage facts, want 1", len(events))
			}
			event := events[0]
			if !event.Forwarded || event.UpstreamHttpStatus == nil || *event.UpstreamHttpStatus != 200 || event.ResponseBytes == 0 || event.ManagedGeneration != 1 || event.ControlRevision != 1 {
				t.Fatalf("lost stream attribution: %+v", event)
			}
			if event.ErrorClass == nil || *event.ErrorClass != wantClass {
				t.Fatalf("stream error class = %v, want %s", event.ErrorClass, wantClass)
			}
		})
	}
}
