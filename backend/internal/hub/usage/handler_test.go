package usage

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/hub/testutil"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/usageingestapi"
)

func TestHUBI5UsageIngestHTTPContract(t *testing.T) {
	store := testutil.OpenStore(t)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	userID, upstreamID := seedUsageParents(t, store.Client, now)
	service := NewService(store.Client)
	service.Now = func() time.Time { return now }
	handler := usageingestapi.Handler(NewHandler(service, "relay-service-token"))

	body, _ := json.Marshal(usageingestapi.UsageBatch{Events: []usageingestapi.RequestUsageEvent{validRequestUsageEvent(now, userID, upstreamID)}})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/usage/request-events:batch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer relay-service-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", response.Code, response.Body.String())
	}
	var ack usageingestapi.UsageBatchAck
	if err := json.Unmarshal(response.Body.Bytes(), &ack); err != nil {
		t.Fatal(err)
	}
	if ack.AcceptedCount != 1 || ack.DuplicateCount != 0 {
		t.Fatalf("unexpected ack: %+v", ack)
	}
}

func TestHUBI5UsageIngestRequiresServiceAuthAndStrictJSON(t *testing.T) {
	store := testutil.OpenStore(t)
	service := NewService(store.Client)
	handler := usageingestapi.Handler(NewHandler(service, "relay-service-token"))

	unauthorized := httptest.NewRequest(http.MethodPost, "/internal/v1/usage/request-events:batch", bytes.NewBufferString(`{"events":[]}`))
	unauthorized.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, unauthorized)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing service credential status=%d", response.Code)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/internal/v1/usage/request-events:batch", bytes.NewBufferString(`{"events":[],"unknown":true}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalid.Header.Set("Authorization", "Bearer relay-service-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, invalid)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status=%d body=%s", response.Code, response.Body.String())
	}
}
