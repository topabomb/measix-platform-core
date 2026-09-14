package control

import (
	"crypto/ed25519"
	"net/url"
	"time"

	"measix/platform/internal/wire/relaycontrolapi"
)

type Route struct {
	ID                  string
	UpstreamID          string
	AllowedMethods      map[string]struct{}
	AllowedPathPrefixes []string
	TransportPolicy     relaycontrolapi.RuntimeRouteSpecTransportPolicy
	TimeoutPolicy       relaycontrolapi.TimeoutPolicy
}

type UpstreamAuth struct {
	Type       relaycontrolapi.RuntimeUpstreamAuthType
	Token      string
	HeaderName string
	Value      string
	Username   string
	Password   string
}

type Upstream struct {
	ID                    string
	BaseURL               *url.URL
	Enabled               bool
	TransportCapabilities map[string]struct{}
	SecretRef             *relaycontrolapi.SecretRef
	Auth                  UpstreamAuth
}

type State struct {
	ControlRevision         int
	BundleHash              string
	ActiveManagedGeneration int
	DeploymentID            string
	AuthKeys                map[string]ed25519.PublicKey
	DisabledUsers           map[string]struct{}
	RevokedDevices          map[string]struct{}
	RevokedSessions         map[string]struct{}
	ResourceRoutes          map[string]string
	Routes                  map[string]Route
	Upstreams               map[string]Upstream
	OperationalLimits       relaycontrolapi.OperationalLimits
	AppliedAt               time.Time
}
