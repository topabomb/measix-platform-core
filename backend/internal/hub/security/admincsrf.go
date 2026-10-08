package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func CSRFToken(sessionSecret string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("measix-admin-csrf\x00"))
	_, _ = mac.Write([]byte(sessionSecret))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func VerifyCSRF(sessionSecret, token string, key []byte) bool {
	want := CSRFToken(sessionSecret, key)
	return hmac.Equal([]byte(want), []byte(token))
}
