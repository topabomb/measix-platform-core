package runtime

import "testing"

func TestRLYROUTE005RejectsEncodedTraversalVariants(t *testing.T) {
	// RLY-ROUTE-005: dot segments, encoded traversal and double-encoding must fail closed.
	for _, path := range []string{
		"/v1/../admin",
		"/v1/%2e%2e/admin",
		"/v1/%252e%252e/admin",
		"/v1/%25252e%25252e/admin",
		"/v1/%2e/admin",
		"/v1/%5c..%5cadmin",
	} {
		if safeRuntimePath(path) {
			t.Fatalf("safeRuntimePath(%q)=true, want false", path)
		}
	}
	for _, path := range []string{"/v1/chat/completions", "/mcp", "/v1/files/name%20with%20space"} {
		if !safeRuntimePath(path) {
			t.Fatalf("safeRuntimePath(%q)=false, want true", path)
		}
	}
}
