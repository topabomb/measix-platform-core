package security

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

func LoadMasterKey(path string) ([]byte, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(value) != 32 {
		return nil, fmt.Errorf("master key file must contain exactly 32 bytes")
	}
	return append([]byte(nil), value...), nil
}

func LoadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch len(value) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(append([]byte(nil), value...)), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(value), nil
	}
	block, _ := pem.Decode(value)
	if block == nil {
		return nil, fmt.Errorf("Ed25519 private key must be raw seed/private key or PKCS#8 PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse Ed25519 PKCS#8 key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key is not Ed25519")
	}
	return append(ed25519.PrivateKey(nil), key...), nil
}
