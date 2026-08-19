package runtime

import (
	"crypto/ed25519"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"measix/platform/internal/relay/control"
	"measix/platform/pkg/platformid"
)

type accessClaims struct {
	DeploymentID string `json:"deploymentId"`
	DeviceID     string `json:"deviceId"`
	SessionID    string `json:"sessionId"`
	jwt.RegisteredClaims
}

func (h *Handler) authenticate(state *control.State, value string) (*accessClaims, error) {
	claims := &accessClaims{}
	token, err := jwt.ParseWithClaims(
		value,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodEdDSA {
				return nil, jwt.ErrSignatureInvalid
			}
			kid, _ := token.Header["kid"].(string)
			key, ok := state.AuthKeys[kid]
			if !ok || len(key) != ed25519.PublicKeySize {
				return nil, jwt.ErrTokenUnverifiable
			}
			return key, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(state.DeploymentID),
		jwt.WithAudience("runtime"),
		jwt.WithTimeFunc(h.store.Now),
	)
	if err != nil || !token.Valid || claims.DeploymentID != state.DeploymentID {
		return nil, jwt.ErrTokenInvalidClaims
	}
	if platformid.Validate(platformid.User, claims.Subject) != nil ||
		platformid.Validate(platformid.Device, claims.DeviceID) != nil ||
		platformid.Validate(platformid.Session, claims.SessionID) != nil {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

func bearer(r *http.Request) string {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}
