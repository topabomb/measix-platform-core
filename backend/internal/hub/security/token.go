package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func RandomToken(bytes int) (string, error) {
	if bytes < 16 {
		return "", fmt.Errorf("credential entropy too small")
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read secure random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func DigestToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
