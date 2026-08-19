package control

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaystate"
	"github.com/topabomb/measix-platform-core/backend/pkg/platformid"
)

func HashDescriptor(input relaycontrolapi.RuntimeControlState) (relaycontrolapi.Sha256Hash, error) {
	return relaystate.HashDescriptor(input)
}

func build(input relaycontrolapi.RuntimeControlState, appliedAt time.Time) (*State, error) {
	if input.ControlRevision < 1 || input.ActiveManagedGeneration < 0 || input.OperationalLimits.MaxRequestBytes < 1 {
		return nil, ErrInvalidControl
	}
	if err := platformid.Validate(platformid.Deployment, input.DeploymentId); err != nil {
		return nil, ErrInvalidControl
	}
	hash, err := relaystate.HashDescriptor(input)
	if err != nil || string(hash) != string(input.BundleHash) {
		return nil, ErrInvalidControl
	}

	state := &State{
		ControlRevision: input.ControlRevision, BundleHash: string(input.BundleHash),
		ActiveManagedGeneration: input.ActiveManagedGeneration, DeploymentID: input.DeploymentId,
		AuthKeys: make(map[string]ed25519.PublicKey), DisabledUsers: make(map[string]struct{}),
		RevokedDevices: make(map[string]struct{}), RevokedSessions: make(map[string]struct{}),
		ResourceRoutes: make(map[string]string), Routes: make(map[string]Route), Upstreams: make(map[string]Upstream),
		OperationalLimits: input.OperationalLimits, AppliedAt: appliedAt,
	}

	for _, key := range input.AuthKeys {
		if key.Kty != relaycontrolapi.OKP || key.Crv != relaycontrolapi.Ed25519 || key.Alg != relaycontrolapi.EdDSA || key.Use != relaycontrolapi.Sig || key.Kid == "" {
			return nil, ErrInvalidControl
		}
		decoded, err := base64.RawURLEncoding.DecodeString(key.X)
		if err != nil || len(decoded) != ed25519.PublicKeySize {
			return nil, ErrInvalidControl
		}
		if _, exists := state.AuthKeys[key.Kid]; exists {
			return nil, ErrInvalidControl
		}
		state.AuthKeys[key.Kid] = ed25519.PublicKey(append([]byte(nil), decoded...))
	}
	if err := addIDs(state.DisabledUsers, platformid.User, input.PrincipalState.DisabledUserIds); err != nil {
		return nil, err
	}
	if err := addIDs(state.RevokedDevices, platformid.Device, input.PrincipalState.RevokedDeviceIds); err != nil {
		return nil, err
	}
	if err := addIDs(state.RevokedSessions, platformid.Session, input.PrincipalState.RevokedSessionIds); err != nil {
		return nil, err
	}

	for _, value := range input.Upstreams {
		if err := platformid.Validate(platformid.Upstream, value.UpstreamId); err != nil || !value.Auth.Type.Valid() {
			return nil, ErrInvalidControl
		}
		if _, exists := state.Upstreams[value.UpstreamId]; exists {
			return nil, ErrInvalidControl
		}
		parsed, err := url.Parse(value.BaseUrl)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			return nil, ErrInvalidControl
		}
		auth, err := compileAuth(value.Auth)
		if err != nil {
			return nil, err
		}
		caps := make(map[string]struct{}, len(value.TransportCapabilities))
		for _, capability := range value.TransportCapabilities {
			if strings.TrimSpace(capability) == "" {
				return nil, ErrInvalidControl
			}
			caps[capability] = struct{}{}
		}
		var secretRef *relaycontrolapi.SecretRef
		if value.SecretRef != nil {
			if err := platformid.Validate(platformid.Secret, value.SecretRef.SecretId); err != nil || value.SecretRef.SecretVersion < 1 {
				return nil, ErrInvalidControl
			}
			copyRef := *value.SecretRef
			secretRef = &copyRef
		}
		state.Upstreams[value.UpstreamId] = Upstream{
			ID: value.UpstreamId, BaseURL: parsed, Enabled: value.Enabled,
			TransportCapabilities: caps, SecretRef: secretRef, Auth: auth,
		}
	}

	for _, value := range input.Routes {
		if err := platformid.Validate(platformid.Route, value.RuntimeRouteId); err != nil || platformid.Validate(platformid.Upstream, value.UpstreamId) != nil {
			return nil, ErrInvalidControl
		}
		if _, exists := state.Routes[value.RuntimeRouteId]; exists {
			return nil, ErrInvalidControl
		}
		if _, exists := state.Upstreams[value.UpstreamId]; !exists || !value.TransportPolicy.Valid() || len(value.AllowedMethods) == 0 || len(value.AllowedPathPrefixes) == 0 {
			return nil, ErrInvalidControl
		}
		methods := make(map[string]struct{}, len(value.AllowedMethods))
		for _, method := range value.AllowedMethods {
			method = strings.ToUpper(strings.TrimSpace(method))
			if !validMethod(method) {
				return nil, ErrInvalidControl
			}
			methods[method] = struct{}{}
		}
		prefixes := append([]string(nil), value.AllowedPathPrefixes...)
		for _, prefix := range prefixes {
			if !safePath(prefix) {
				return nil, ErrInvalidControl
			}
		}
		sort.Strings(prefixes)
		if value.TimeoutPolicy.ConnectMs < 1 || value.TimeoutPolicy.ResponseHeaderMs < 1 || value.TimeoutPolicy.IdleMs < 1 {
			return nil, ErrInvalidControl
		}
		if value.TimeoutPolicy.OverallMs != nil && *value.TimeoutPolicy.OverallMs < 1 {
			return nil, ErrInvalidControl
		}
		state.Routes[value.RuntimeRouteId] = Route{
			ID: value.RuntimeRouteId, UpstreamID: value.UpstreamId, AllowedMethods: methods,
			AllowedPathPrefixes: prefixes, TransportPolicy: value.TransportPolicy, TimeoutPolicy: value.TimeoutPolicy,
		}
	}

	for _, value := range input.ResourceRoutes {
		if !runtimeResourceID(value.ResourceId) || platformid.Validate(platformid.Route, value.RuntimeRouteId) != nil {
			return nil, ErrInvalidControl
		}
		if _, exists := state.ResourceRoutes[value.ResourceId]; exists {
			return nil, ErrInvalidControl
		}
		if _, exists := state.Routes[value.RuntimeRouteId]; !exists {
			return nil, ErrInvalidControl
		}
		state.ResourceRoutes[value.ResourceId] = value.RuntimeRouteId
	}
	return state, nil
}

func compileAuth(input relaycontrolapi.RuntimeUpstreamAuth) (UpstreamAuth, error) {
	result := UpstreamAuth{Type: input.Type}
	switch input.Type {
	case relaycontrolapi.NONE:
		return result, nil
	case relaycontrolapi.BEARER:
		value, ok := stringProperty(input, "token")
		if !ok || value == "" {
			return UpstreamAuth{}, ErrInvalidControl
		}
		result.Token = value
	case relaycontrolapi.STATICHEADER:
		header, okHeader := stringProperty(input, "headerName")
		value, okValue := stringProperty(input, "value")
		if !okHeader || !okValue || strings.TrimSpace(header) == "" || value == "" || strings.ContainsAny(header, "\r\n:") {
			return UpstreamAuth{}, ErrInvalidControl
		}
		result.HeaderName, result.Value = header, value
	case relaycontrolapi.BASIC:
		username, okUser := stringProperty(input, "username")
		password, okPassword := stringProperty(input, "password")
		if !okUser || !okPassword || username == "" || password == "" {
			return UpstreamAuth{}, ErrInvalidControl
		}
		result.Username, result.Password = username, password
	default:
		return UpstreamAuth{}, ErrInvalidControl
	}
	return result, nil
}

func stringProperty(input relaycontrolapi.RuntimeUpstreamAuth, name string) (string, bool) {
	value, ok := input.AdditionalProperties[name]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func addIDs(target map[string]struct{}, kind platformid.Kind, values []string) error {
	for _, value := range values {
		if platformid.Validate(kind, value) != nil {
			return ErrInvalidControl
		}
		if _, exists := target[value]; exists {
			return ErrInvalidControl
		}
		target[value] = struct{}{}
	}
	return nil
}

func runtimeResourceID(value string) bool {
	kind, err := platformid.KindOf(value)
	if err != nil {
		return false
	}
	switch kind {
	case platformid.Model, platformid.TTS, platformid.ASR, platformid.MCP:
		return true
	default:
		return false
	}
}

func validMethod(value string) bool {
	switch value {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func safePath(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "//") || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." || segment == "." {
			return false
		}
	}
	return true
}
