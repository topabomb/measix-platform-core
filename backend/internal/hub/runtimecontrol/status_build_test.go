package runtimecontrol

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRelayStatusRequiresCurrentBuildIdentity(t *testing.T) {
	for _, version := range []string{"", `,"buildVersion":""`, `,"buildVersion":"relay-build"`} {
		t.Run(version, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"ready":false,"appliedControlRevision":0,"bundleHash":"","activeManagedGeneration":0,"startedAt":"2026-09-18T00:00:00Z"%s}`, version)
			}))
			defer server.Close()
			status, err := NewHTTPRelayClient(server.URL, "token", server.Client()).Status(context.Background())
			if version == `,"buildVersion":"relay-build"` {
				if err != nil || status.BuildVersion != "relay-build" {
					t.Fatalf("status=%+v err=%v", status, err)
				}
			} else if err == nil {
				t.Fatal("accepted missing or empty current build identity")
			}
		})
	}
}
