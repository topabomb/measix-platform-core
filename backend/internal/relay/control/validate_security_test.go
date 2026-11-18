package control

import (
	"testing"

	"measix/platform/internal/wire/relaycontrolapi"
)

func TestRLYROUTE005ControlRejectsEncodedTraversalPrefixes(t *testing.T) {
	for _, path := range []string{"/v1/../admin", "/v1/%2e%2e/admin", "/v1/%252e%252e/admin", "/v1/%5c..%5cadmin"} {
		if safePath(path) {
			t.Fatalf("safePath(%q)=true, want false", path)
		}
	}
}

func TestRLYHDRCredentialHeaderCannotRestoreReservedHeaders(t *testing.T) {
	// RLY-HDR-001..008: configured credentials may add provider auth, but may not
	// reintroduce Relay-internal, routing, cookie or hop-by-hop headers after sanitization.
	for _, name := range []string{"Host", "Cookie", "Connection", "Transfer-Encoding", "X-Measix-Request-Id", "X-Forwarded-Host", "Bad Header"} {
		_, err := compileAuth(relaycontrolapi.RuntimeUpstreamAuth{
			Type:                 relaycontrolapi.STATICHEADER,
			AdditionalProperties: map[string]interface{}{"headerName": name, "value": "secret"},
		})
		if err == nil {
			t.Fatalf("STATIC_HEADER %q unexpectedly accepted", name)
		}
	}
	for _, name := range []string{"X-Api-Key", "Api-Key", "Authorization"} {
		_, err := compileAuth(relaycontrolapi.RuntimeUpstreamAuth{
			Type:                 relaycontrolapi.STATICHEADER,
			AdditionalProperties: map[string]interface{}{"headerName": name, "value": "secret"},
		})
		if err != nil {
			t.Fatalf("STATIC_HEADER %q rejected: %v", name, err)
		}
	}
	_, err := compileAuth(relaycontrolapi.RuntimeUpstreamAuth{
		Type:                 relaycontrolapi.STATICHEADER,
		AdditionalProperties: map[string]interface{}{"headerName": "X-Api-Key", "value": "secret\r\ninjected: true"},
	})
	if err == nil {
		t.Fatal("STATIC_HEADER value containing CRLF unexpectedly accepted")
	}
}
