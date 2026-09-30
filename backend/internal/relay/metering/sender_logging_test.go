package metering

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/common/observability"
)

func TestSenderRunLogsFailureAndConfirmedRecoveryTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	spool, err := OpenSpool(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	now := time.Now().UTC()
	requestID := "req_logging_transition"
	payload := json.RawMessage(`{"requestId":"req_logging_transition","revision":1}`)
	if err := spool.SaveAdmission(ctx, requestID, json.RawMessage(`{"admission":true}`), now); err != nil {
		t.Fatal(err)
	}
	if err := spool.MarkStarted(ctx, requestID, now); err != nil {
		t.Fatal(err)
	}
	if err := spool.AppendSettlement(ctx, requestID, payload, now); err != nil {
		t.Fatal(err)
	}

	attempts := 0
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"acceptedCount":1,"duplicateCount":0,"discardedCount":0}`))
		time.AfterFunc(10*time.Millisecond, cancel)
	}))
	defer hub.Close()
	var output bytes.Buffer
	sender := NewSender(spool, hub.URL, "service-token")
	sender.Now = func() time.Time { return now }
	sender.Jitter = func(base time.Duration) time.Duration { return -base }
	sender.Log = observability.NewLogger(&output, "relay", "preview")
	if err := sender.Run(ctx, time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("run error=%v", err)
	}
	logs := output.String()
	if !strings.Contains(logs, `"event":"spool.flush_failed"`) || !strings.Contains(logs, `"event":"spool.flush_recovered"`) {
		t.Fatalf("missing state transition logs: %s", logs)
	}
}
