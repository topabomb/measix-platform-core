package security_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/topabomb/measix-platform-core/backend/internal/hub/security"
)

func TestPasswordArgon2idRoundTrip(t *testing.T) {
	hash, err := security.HashPassword("correct horse battery staple")
	if err != nil { t.Fatal(err) }
	if !security.VerifyPassword(hash, "correct horse battery staple") || security.VerifyPassword(hash, "wrong password") { t.Fatal("password verification invariant failed") }
}

func TestAccessJWTClaimsAndTTL(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	signer, err := security.NewAccessSigner(privateKey, "dep_550e8400-e29b-41d4-a716-446655440000", "s0-test", 10*time.Minute)
	if err != nil { t.Fatal(err) }
	now := time.Unix(1_800_000_000, 0).UTC()
	signer.Now = func() time.Time { return now }
	tokenText, expires, err := signer.Sign("usr_550e8400-e29b-41d4-a716-446655440000", "dev_550e8400-e29b-41d4-a716-446655440000", "ses_550e8400-e29b-41d4-a716-446655440000")
	if err != nil { t.Fatal(err) }
	if !expires.Equal(now.Add(10*time.Minute)) { t.Fatalf("expires=%v", expires) }
	claims := &security.AccessClaims{}
	tok, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) { return privateKey.Public(), nil }, jwt.WithAudience("runtime"), jwt.WithIssuer(signer.DeploymentID), jwt.WithTimeFunc(func() time.Time { return now.Add(time.Second) }))
	if err != nil || !tok.Valid { t.Fatalf("parse JWT: %v", err) }
	if claims.Subject == "" || claims.DeviceID == "" || claims.SessionID == "" { t.Fatalf("missing claims: %+v", claims) }
}

func TestCSRFIsBoundToOpaqueSessionSecret(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	token := security.CSRFToken("opaque-cookie", key)
	if !security.VerifyCSRF("opaque-cookie", token, key) || security.VerifyCSRF("other-cookie", token, key) { t.Fatal("csrf binding failed") }
}
