package security_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"github.com/golang-jwt/jwt/v5"
	"measix/platform/internal/hub/security"
	"measix/platform/pkg/platformid"
	"testing"
	"time"
)

func TestJWTRejectsMissingExpiry(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := security.NewAccessSigner(key, platformid.New(platformid.Deployment), "test", time.Minute)
	claims := security.AccessClaims{DeploymentID: s.DeploymentID, DeviceID: platformid.New(platformid.Device), SessionID: platformid.New(platformid.Session), RegisteredClaims: jwt.RegisteredClaims{Issuer: s.DeploymentID, Subject: platformid.New(platformid.User), Audience: jwt.ClaimStrings{"client", "runtime"}, IssuedAt: jwt.NewNumericDate(time.Now())}}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = "test"
	encoded, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(encoded); err == nil {
		t.Fatal("non-expiring access token accepted")
	}
}
