package runtime

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"measix/platform/internal/relay/control"
	"measix/platform/internal/wire/relaycontrolapi"
)

func TestHTTP2ResponseHeaderTimeoutIsReportedAsTimeout(t *testing.T) {
	protocol := make(chan int, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol <- r.ProtoMajor
		<-r.Context().Done()
	}))
	upstream.EnableHTTP2 = true
	upstream.StartTLS()
	defer upstream.Close()

	base := upstream.Client().Transport.(*http.Transport).Clone()
	base.ForceAttemptHTTP2 = true
	handler := &Handler{baseTransport: base}
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	route := control.Route{
		TransportPolicy: relaycontrolapi.HTTPREQUESTRESPONSE,
		TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 50, IdleMs: 1000},
	}
	upstreamConfig := control.Upstream{BaseURL: target}
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"prompt":"test"}`))
	response := httptest.NewRecorder()
	result := &proxyResult{}
	started := time.Now()
	handler.serveProxy(&responseObserver{ResponseWriter: response}, request, route, upstreamConfig, "/v1/images/generations", "req_test", result, 1024)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("response header timeout took %v", elapsed)
	}
	select {
	case got := <-protocol:
		if got != 2 {
			t.Fatalf("upstream protocol = HTTP/%d, want HTTP/2", got)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not reach the HTTP/2 upstream")
	}
	if response.Code != http.StatusGatewayTimeout || result.ErrorClass != "UPSTREAM_TIMEOUT" {
		t.Fatalf("status=%d class=%q body=%s", response.Code, result.ErrorClass, response.Body.String())
	}
	if definitiveNoProviderConsumption(nil, result.ErrorClass) {
		t.Fatal("a request that timed out after forwarding must retain unknown provider consumption")
	}
}
