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
	return &AccessSigner{
		PrivateKey:   privateKey,
		DeploymentID: deploymentID,
		KeyID:        keyID,
		TTL:          ttl,
		Now:          time.Now,
	}, nil
}

func (s *AccessSigner) Sign(userID, deviceID, sessionID string) (string, time.Time, error) {
	now := s.Now().UTC()
	expiresAt := now.Add(s.TTL)
	claims := AccessClaims{
		DeploymentID: s.DeploymentID,
		DeviceID:     deviceID,
		SessionID:    sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.DeploymentID,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"client", "runtime"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.KeyID
	token.Header["typ"] = "JWT"
	value, err := token.SignedString(s.PrivateKey)
	return value, expiresAt, err
}

func (s *AccessSigner) Verify(value string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(
		value,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodEdDSA {
				return nil, fmt.Errorf("unexpected signing method")
			}
			if kid, _ := token.Header["kid"].(string); kid != s.KeyID {
				return nil, fmt.Errorf("unexpected key id")
			}
			return s.PrivateKey.Public().(ed25519.PublicKey), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(s.DeploymentID),
		jwt.WithAudience("client"),
		jwt.WithTimeFunc(s.Now),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid access token")
	}
	if claims.DeploymentID != s.DeploymentID || claims.Subject == "" || claims.DeviceID == "" || claims.SessionID == "" {
		return nil, fmt.Errorf("invalid access claims")
	}
	return claims, nil
}

func (s *AccessSigner) PublicJWK() map[string]string {
	publicKey := s.PrivateKey.Public().(ed25519.PublicKey)
	return map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"alg": "EdDSA",
		"use": "sig",
		"kid": s.KeyID,
		"x":   base64.RawURLEncoding.EncodeToString(publicKey),
	}
}
