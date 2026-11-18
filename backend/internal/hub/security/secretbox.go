package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

type SecretBox struct {
	key        []byte
	keyVersion int
}

func NewSecretBox(key []byte, keyVersion int) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be exactly 32 bytes")
	}
	if keyVersion <= 0 {
		return nil, fmt.Errorf("key version must be positive")
	}
	return &SecretBox{key: append([]byte(nil), key...), keyVersion: keyVersion}, nil
}

func (b *SecretBox) KeyVersion() int { return b.keyVersion }

func (b *SecretBox) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	payload := make([]byte, 0, len(nonce)+len(sealed))
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)
	return payload, nil
}

func (b *SecretBox) Decrypt(payload []byte) ([]byte, error) {
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("encrypted payload is truncated")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}
