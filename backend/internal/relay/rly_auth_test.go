package relay_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"measix/platform/internal/wire/relaycontrolapi"
	"measix/platform/internal/wire/relaystate"
	"measix/platform/pkg/platformid"
)

// RLY-AUTH-001: Valid EdDSA JWT is accepted.
// RLY-AUTH-002: Wrong alg/signature/kid is rejected.
// RLY-AUTH-003: Wrong issuer/deployment/audience is rejected.
// RLY-AUTH-004: Expired/not-yet-valid token is rejected.
// RLY-AUTH-005: Malformed usr_*/dev_*/ses_* is rejected.
// RLY-AUTH-006: Disabled user is rejected.
// RLY-AUTH-007: Revoked device/session is rejected.
// RLY-AUTH-008: Unknown kid does not call back to Hub.

func newAuthFixture(t *testing.T, principalState relaycontrolapi.PrincipalState) (*runtimeFixture, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision:         1,
		ActiveManagedGeneration: 1,
		DeploymentId:            platformid.New(platformid.Deployment),
		PrincipalState:          principalState,
		ResourceRoutes:          []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{
			RuntimeRouteId: routeID, UpstreamId: upstreamID,
			AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"},
			TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE,
			TimeoutPolicy:   relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000},
		}},
		Upstreams: []relaycontrolapi.RuntimeUpstreamSpec{{
			UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true,
			TransportCapabilities: []string{"HTTP_STREAMING_SSE"},
			Auth:                  relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}},
		}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	return newRuntimeFixture(t, state, privateKey), resourceID
}

func signToken(t *testing.T, signer *testAccessSigner, userID, deviceID, sessionID string) string {
	t.Helper()
	return signer.sign(t, userID, deviceID, sessionID)
}

// RLY-AUTH-001: Valid EdDSA JWT is accepted.
func TestRLYAUTH001ValidEdDSAJWTAccepted(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid JWT rejected: status=%d", resp.StatusCode)
	}
}

// RLY-AUTH-002: Wrong alg/signature/kid is rejected.
func TestRLYAUTH002WrongAlgSignatureKidRejected(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()

	t.Run("wrong alg (HS256)", func(t *testing.T) {
		claims := testAccessClaims{
			DeploymentID: fixture.signer.deploymentID,
			DeviceID:      fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
				Audience: jwt.ClaimStrings{"client", "runtime"},
				IssuedAt: jwt.NewNumericDate(fixture.signer.now), ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(10 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		token.Header["kid"] = fixture.signer.kid
		signed, err := token.SignedString([]byte("wrong-key"))
		if err != nil {
			t.Fatal(err)
		}
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong alg accepted: status=%d", resp.StatusCode)
		}
	})

	t.Run("wrong kid", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, testAccessClaims{
			DeploymentID: fixture.signer.deploymentID,
			DeviceID:      fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
				Audience: jwt.ClaimStrings{"client", "runtime"},
				IssuedAt: jwt.NewNumericDate(fixture.signer.now), ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(10 * time.Minute)),
			},
		})
		token.Header["kid"] = "unknown-kid"
		signed, err := token.SignedString(fixture.signer.privateKey)
		if err != nil {
			t.Fatal(err)
		}
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong kid accepted: status=%d", resp.StatusCode)
		}
	})
}

// RLY-AUTH-003: Wrong issuer/deployment/audience is rejected.
func TestRLYAUTH003WrongIssuerDeploymentAudienceRejected(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()

	t.Run("wrong issuer", func(t *testing.T) {
		claims := testAccessClaims{
			DeploymentID: "wrong-deployment",
			DeviceID:     fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "wrong-deployment", Subject: fixture.userID,
				Audience: jwt.ClaimStrings{"client", "runtime"},
				IssuedAt: jwt.NewNumericDate(fixture.signer.now), ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(10 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = fixture.signer.kid
		signed, _ := token.SignedString(fixture.signer.privateKey)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong issuer accepted: status=%d", resp.StatusCode)
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		claims := testAccessClaims{
			DeploymentID: fixture.signer.deploymentID,
			DeviceID:     fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
				Audience: jwt.ClaimStrings{"wrong-aud"},
				IssuedAt: jwt.NewNumericDate(fixture.signer.now), ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(10 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = fixture.signer.kid
		signed, _ := token.SignedString(fixture.signer.privateKey)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong audience accepted: status=%d", resp.StatusCode)
		}
	})
}

// RLY-AUTH-004: Expired/not-yet-valid token is rejected.
func TestRLYAUTH004ExpiredTokenRejected(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()

	t.Run("expired", func(t *testing.T) {
		claims := testAccessClaims{
			DeploymentID: fixture.signer.deploymentID,
			DeviceID:     fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
				Audience: jwt.ClaimStrings{"client", "runtime"},
				IssuedAt: jwt.NewNumericDate(fixture.signer.now.Add(-20 * time.Minute)),
				ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(-10 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = fixture.signer.kid
		signed, _ := token.SignedString(fixture.signer.privateKey)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expired token accepted: status=%d", resp.StatusCode)
		}
	})

	t.Run("not yet valid", func(t *testing.T) {
		claims := testAccessClaims{
			DeploymentID: fixture.signer.deploymentID,
			DeviceID:     fixture.deviceID, SessionID: fixture.sessionID,
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
				Audience:  jwt.ClaimStrings{"client", "runtime"},
				IssuedAt:  jwt.NewNumericDate(fixture.signer.now.Add(5 * time.Minute)),
				ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(20 * time.Minute)),
				NotBefore: jwt.NewNumericDate(fixture.signer.now.Add(5 * time.Minute)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		token.Header["kid"] = fixture.signer.kid
		signed, _ := token.SignedString(fixture.signer.privateKey)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+signed)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("not-yet-valid token accepted: status=%d", resp.StatusCode)
		}
	})
}

// RLY-AUTH-005: Malformed usr_*/dev_*/ses_* is rejected.
func TestRLYAUTH005MalformedPrincipalIDsRejected(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()

	for _, label := range []struct {
		name, userID, deviceID, sessionID string
	}{
		{"bad user", "not-a-user", "", ""},
		{"bad device", "", "not-a-device", ""},
		{"bad session", "", "", "not-a-session"},
	} {
		t.Run(label.name, func(t *testing.T) {
			uid := fixture.userID
			if label.userID != "" {
				uid = label.userID
			}
			did := fixture.deviceID
			if label.deviceID != "" {
				did = label.deviceID
			}
			sid := fixture.sessionID
			if label.sessionID != "" {
				sid = label.sessionID
			}
			token := fixture.signer.sign(t, uid, did, sid)
			req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("malformed principal accepted: status=%d", resp.StatusCode)
			}
		})
	}
}

// RLY-AUTH-006: Disabled user is rejected.
func TestRLYAUTH006DisabledUserRejected(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	userID := platformid.New(platformid.User)
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	deploymentID := platformid.New(platformid.Deployment)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	state := relaycontrolapi.RuntimeControlState{
		ControlRevision: 1, ActiveManagedGeneration: 1,
		DeploymentId:   deploymentID,
		PrincipalState:  relaycontrolapi.PrincipalState{DisabledUserIds: []string{userID}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{}},
		ResourceRoutes:  []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
		Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
		Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}}}},
		OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
	}
	fixture := newRuntimeFixture(t, state, privateKey)
	defer fixture.close()
	// Use fixture.signer (same kid and key as state AuthKeys) with a disabled user.
	token := fixture.signer.sign(t, userID, deviceID, sessionID)
	req, _ := http.NewRequest(http.MethodPost, fixture.server.URL+"/runtime/v1/resources/"+resourceID+"/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Measix-Managed-Generation", "1")
	req.Header.Set("X-Measix-Interaction-Id", platformid.New(platformid.Interaction))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled user not rejected: status=%d", resp.StatusCode)
	}
}

// RLY-AUTH-007: Revoked device/session is rejected.
func TestRLYAUTH007RevokedDeviceSessionRejected(t *testing.T) {
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	deploymentID := platformid.New(platformid.Deployment)
	resourceID := platformid.New(platformid.Model)
	routeID := platformid.New(platformid.Route)
	upstreamID := platformid.New(platformid.Upstream)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	userID := platformid.New(platformid.User)
	deviceID := platformid.New(platformid.Device)
	sessionID := platformid.New(platformid.Session)

	t.Run("revoked device", func(t *testing.T) {
		state := relaycontrolapi.RuntimeControlState{
			ControlRevision: 1, ActiveManagedGeneration: 1,
			DeploymentId:   deploymentID,
			PrincipalState:  relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{deviceID}, RevokedSessionIds: []string{}},
			ResourceRoutes:  []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
			Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
			Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}}}},
			OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
		}
		fixture := newRuntimeFixture(t, state, privateKey)
		defer fixture.close()
		token := fixture.signer.sign(t, userID, deviceID, sessionID)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("revoked device not rejected: status=%d", resp.StatusCode)
		}
	})

	t.Run("revoked session", func(t *testing.T) {
		state := relaycontrolapi.RuntimeControlState{
			ControlRevision: 1, ActiveManagedGeneration: 1,
			DeploymentId:   deploymentID,
			PrincipalState:  relaycontrolapi.PrincipalState{DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{sessionID}},
			ResourceRoutes:  []relaycontrolapi.ResourceRoute{{ResourceId: resourceID, RuntimeRouteId: routeID}},
			Routes: []relaycontrolapi.RuntimeRouteSpec{{RuntimeRouteId: routeID, UpstreamId: upstreamID, AllowedMethods: []string{"POST"}, AllowedPathPrefixes: []string{"/v1/chat/completions"}, TransportPolicy: relaycontrolapi.HTTPSTREAMINGSSE, TimeoutPolicy: relaycontrolapi.TimeoutPolicy{ConnectMs: 1000, ResponseHeaderMs: 5000, IdleMs: 30000}}},
			Upstreams:         []relaycontrolapi.RuntimeUpstreamSpec{{UpstreamId: upstreamID, BaseUrl: upstream.URL, Enabled: true, TransportCapabilities: []string{"HTTP_STREAMING_SSE"}, Auth: relaycontrolapi.RuntimeUpstreamAuth{Type: relaycontrolapi.NONE, AdditionalProperties: map[string]interface{}{}}}},
			OperationalLimits: relaycontrolapi.OperationalLimits{MaxRequestBytes: 1 << 20},
		}
		fixture := newRuntimeFixture(t, state, privateKey)
		defer fixture.close()
		token := fixture.signer.sign(t, userID, deviceID, sessionID)
		req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("revoked session not rejected: status=%d", resp.StatusCode)
		}
	})
}

// RLY-AUTH-008: Unknown kid does not call back to Hub — it returns 401 immediately.
func TestRLYAUTH008UnknownKidReturns401NoCallback(t *testing.T) {
	fixture, resourceID := newAuthFixture(t, relaycontrolapi.PrincipalState{
		DisabledUserIds: []string{}, RevokedDeviceIds: []string{}, RevokedSessionIds: []string{},
	})
	defer fixture.close()
	// Sign with the correct private key but use an unknown kid.
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, testAccessClaims{
		DeploymentID: fixture.signer.deploymentID,
		DeviceID:     fixture.deviceID, SessionID: fixture.sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: fixture.signer.deploymentID, Subject: fixture.userID,
			Audience:  jwt.ClaimStrings{"client", "runtime"},
			IssuedAt:  jwt.NewNumericDate(fixture.signer.now),
			ExpiresAt: jwt.NewNumericDate(fixture.signer.now.Add(10 * time.Minute)),
		},
	})
	token.Header["kid"] = "completely-unknown-kid"
	signed, _ := token.SignedString(fixture.signer.privateKey)
	req := fixture.request(t, nil, http.MethodPost, resourceID, "/v1/chat/completions", strings.NewReader(`{}`), "application/json")
	req.Header.Set("Authorization", "Bearer "+signed)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown kid not rejected: status=%d", resp.StatusCode)
	}
}

// Ensure imports are used.
var _ = base64.RawURLEncoding
var _ = relaystate.HashDescriptor
