package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicHandlerKeepsDiscoveryAndRuntimeOnOneOrigin(t *testing.T) {
	hub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", "hub")
		w.WriteHeader(http.StatusOK)
	})
	relay := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", "relay")
		w.WriteHeader(http.StatusOK)
	})
	handler := publicHandler(hub, relay)
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/.well-known/measix", "hub"},
		{"/api/client/v1/bootstrap", "hub"},
		{"/runtime/v1/resources/mdl_test/v1/chat/completions", "relay"},
		{"/runtime/v10/resources/mdl_test", "hub"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if got := response.Header().Get("X-Handler"); got != tc.want {
			t.Fatalf("path %s dispatched to %q, want %q", tc.path, got, tc.want)
		}
	}
}
