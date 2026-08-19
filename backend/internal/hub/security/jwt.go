package security

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessClaims struct {
	DeploymentID string `json:"deploymentId"`
	DeviceID     string `json:"deviceId"`
	SessionID    string `json:"sessionId"`
	jwt.RegisteredClaims
}

type AccessSigner struct {
	PrivateKey   ed25519.PrivateKey
	DeploymentID string
	KeyID        string
	TTL          time.Duration
	Now          func() time.Time
}

func NewAccessSigner(privateKey ed25519.PrivateKey, deploymentID, keyID string, ttl time.Duration) (*AccessSigner, error) {
	if len(privateKey) != ed25519.PrivateKeySize || deploymentID == "" || keyID == "" {
		return nil, fmt.Errorf("invalid access signer configuration")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, fmt.Errorf("access token TTL must be >0 and <=10m")
	}
	return &AccessSigner{PrivateKey: privateKey, DeploymentID: deploymentID, KeyID: keyID, TTL: ttl, Now: time.Now}, nil
}

func (s *AccessSigner) Sign(userID, deviceID, sessionID string) (string, time.Time, error) {
	now := s.Now().UTC()
	exp := now.Add(s.TTL)
	claims := AccessClaims{
		DeploymentID: s.DeploymentID,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: s.DeploymentID, Subject: userID, Audience: jwt.ClaimStrings{"client", "runtime"},
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.KeyID
	token.Header["typ"] = "JWT"
	value, err := token.SignedString(s.PrivateKey)
	return value, exp, err
}

func (s *AccessSigner) PublicJWK() map[string]string {
	pub := s.PrivateKey.Public().(ed25519.PublicKey)
	return map[string]string{
		"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": s.KeyID,
		"x": base64.RawURLEncoding.EncodeToString(pub),
	}
}
