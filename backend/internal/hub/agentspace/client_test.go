package agentspace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUsernameKeepsFullIdentity(t *testing.T) {
	got, err := Username("usr_550e8400-e29b-41d4-a716-446655440000")
	if err != nil || got != "measix_550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"alice", "usr_../alice", "usr_550E8400-e29b-41d4-a716-446655440000"} {
		if _, err := Username(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestManagementNeverFollowsRedirectOrLeaksCredential(t *testing.T) {
	reached := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-test" {
			t.Error("missing management auth")
		}
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	c, err := New(server.URL, server.URL, "", "admin-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Check(context.Background()); err == nil || reached {
		t.Fatalf("redirect followed: %v %v", reached, err)
	}
	if strings.Contains(err.Error(), "admin-test") {
		t.Fatal("secret in error")
	}
}

func TestDAVRequiresExactSubmittedCredentialAndIdentity(t *testing.T) {
	const spc = "spc_550e8400-e29b-41d4-a716-446655440000"
	token := strings.Repeat("x", 43)
	for _, mismatch := range []string{"", "token", "space", "origin"} {
		t.Run(mismatch, func(t *testing.T) {
			var origin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["token"] != token || body["agentSpaceId"] != spc {
					t.Error("wrong explicit candidate")
				}
				out := map[string]string{"username": "alice", "agentSpaceId": spc, "token": token, "davUrl": origin + "/u/alice/dav/"}
				if mismatch == "token" {
					out["token"] = "unexpected-random-value"
				}
				if mismatch == "space" {
					out["agentSpaceId"] = "spc_other"
				}
				if mismatch == "origin" {
					out["davUrl"] = "https://evil.invalid/u/alice/dav/"
				}
				_ = json.NewEncoder(w).Encode(out)
			}))
			defer server.Close()
			origin = server.URL
			c, err := New(origin, origin, origin, "admin-test")
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.SetDAV(context.Background(), "alice", spc, token)
			if (err != nil) != (mismatch != "") {
				t.Fatalf("mismatch=%s error=%v", mismatch, err)
			}
		})
	}
}

func TestPathRejectsTraversalAndProtectsRoot(t *testing.T) {
	for _, p := range []string{"../a", "a/../b", "/a", "a//b", "a\\b", "%2e%2e/a", "a/%252f/b", ".agent-space/a", "a\x00b"} {
		if _, err := RelativePath(p, false); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
	for _, p := range []string{"", ".", "/"} {
		if _, err := RelativePath(p, true); err == nil {
			t.Errorf("mutable root %q", p)
		}
	}
	if got, err := RelativePath("中文 空格/#%.txt", false); err != nil || got != "中文 空格/#%.txt" {
		t.Fatalf("%q %v", got, err)
	}
}
