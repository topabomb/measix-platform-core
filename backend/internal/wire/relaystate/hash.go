package relaystate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/topabomb/measix-platform-core/backend/internal/wire/relaycontrolapi"
)

type authDescriptor struct {
	Type       relaycontrolapi.RuntimeUpstreamAuthType `json:"type"`
	HeaderName string                                  `json:"headerName,omitempty"`
	Username   string                                  `json:"username,omitempty"`
}

type upstreamDescriptor struct {
	UpstreamID            string                     `json:"upstreamId"`
	BaseURL               string                     `json:"baseUrl"`
	TransportCapabilities []string                   `json:"transportCapabilities"`
	Enabled               bool                       `json:"enabled"`
	SecretRef             *relaycontrolapi.SecretRef `json:"secretRef,omitempty"`
	Auth                  authDescriptor             `json:"auth"`
}

type descriptor struct {
	ControlRevision         int                                `json:"controlRevision"`
	ActiveManagedGeneration int                                `json:"activeManagedGeneration"`
	DeploymentID            string                             `json:"deploymentId"`
	AuthKeys                []relaycontrolapi.PublicJwk        `json:"authKeys"`
	PrincipalState          relaycontrolapi.PrincipalState     `json:"principalState"`
	ResourceRoutes          []relaycontrolapi.ResourceRoute    `json:"resourceRoutes"`
	Routes                  []relaycontrolapi.RuntimeRouteSpec `json:"routes"`
	Upstreams               []upstreamDescriptor               `json:"upstreams"`
	OperationalLimits       relaycontrolapi.OperationalLimits  `json:"operationalLimits"`
}

func DescriptorJSON(input relaycontrolapi.RuntimeControlState) ([]byte, error) {
	authKeys := append([]relaycontrolapi.PublicJwk(nil), input.AuthKeys...)
	sort.Slice(authKeys, func(i, j int) bool { return authKeys[i].Kid < authKeys[j].Kid })

	principal := input.PrincipalState
	principal.DisabledUserIds = append([]string(nil), principal.DisabledUserIds...)
	principal.RevokedDeviceIds = append([]string(nil), principal.RevokedDeviceIds...)
	principal.RevokedSessionIds = append([]string(nil), principal.RevokedSessionIds...)
	sort.Strings(principal.DisabledUserIds)
	sort.Strings(principal.RevokedDeviceIds)
	sort.Strings(principal.RevokedSessionIds)

	resourceRoutes := append([]relaycontrolapi.ResourceRoute(nil), input.ResourceRoutes...)
	sort.Slice(resourceRoutes, func(i, j int) bool {
		if resourceRoutes[i].ResourceId == resourceRoutes[j].ResourceId {
			return resourceRoutes[i].RuntimeRouteId < resourceRoutes[j].RuntimeRouteId
		}
		return resourceRoutes[i].ResourceId < resourceRoutes[j].ResourceId
	})

	routes := append([]relaycontrolapi.RuntimeRouteSpec(nil), input.Routes...)
	for i := range routes {
		routes[i].AllowedMethods = append([]string(nil), routes[i].AllowedMethods...)
		routes[i].AllowedPathPrefixes = append([]string(nil), routes[i].AllowedPathPrefixes...)
		sort.Strings(routes[i].AllowedMethods)
		sort.Strings(routes[i].AllowedPathPrefixes)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].RuntimeRouteId < routes[j].RuntimeRouteId })

	upstreams := make([]upstreamDescriptor, 0, len(input.Upstreams))
	for _, value := range input.Upstreams {
		caps := append([]string(nil), value.TransportCapabilities...)
		sort.Strings(caps)
		auth := authDescriptor{Type: value.Auth.Type}
		if header, ok := stringProperty(value.Auth, "headerName"); ok {
			auth.HeaderName = header
		}
		if username, ok := stringProperty(value.Auth, "username"); ok {
			auth.Username = username
		}
		upstreams = append(upstreams, upstreamDescriptor{
			UpstreamID: value.UpstreamId, BaseURL: value.BaseUrl, TransportCapabilities: caps,
			Enabled: value.Enabled, SecretRef: value.SecretRef, Auth: auth,
		})
	}
	sort.Slice(upstreams, func(i, j int) bool { return upstreams[i].UpstreamID < upstreams[j].UpstreamID })

	return json.Marshal(descriptor{
		ControlRevision: input.ControlRevision, ActiveManagedGeneration: input.ActiveManagedGeneration,
		DeploymentID: input.DeploymentId, AuthKeys: authKeys, PrincipalState: principal,
		ResourceRoutes: resourceRoutes, Routes: routes, Upstreams: upstreams, OperationalLimits: input.OperationalLimits,
	})
}

func HashDescriptor(input relaycontrolapi.RuntimeControlState) (relaycontrolapi.Sha256Hash, error) {
	payload, err := DescriptorJSON(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return relaycontrolapi.Sha256Hash("sha256:" + hex.EncodeToString(sum[:])), nil
}

func stringProperty(input relaycontrolapi.RuntimeUpstreamAuth, name string) (string, bool) {
	value, ok := input.AdditionalProperties[name]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}
