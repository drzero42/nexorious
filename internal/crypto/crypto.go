package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const prefix = "enc:v1:"

// Encrypter performs AES-256-GCM encryption and decryption.
type Encrypter struct {
	key [32]byte
}

// NewEncrypter validates that rawKey is at least 32 bytes (security floor on
// raw entropy), derives a 32-byte AES-256 key via SHA-256(rawKey), and returns
// the Encrypter.
func NewEncrypter(rawKey string) (*Encrypter, error) {
	if len(rawKey) < 32 {
		return nil, fmt.Errorf("crypto: DB_ENCRYPTION_KEY must be at least 32 bytes, got %d", len(rawKey))
	}
	key := sha256.Sum256([]byte(rawKey))
	return &Encrypter{key: key}, nil
}

// Encrypt returns "enc:v1:" + base64(nonce || ciphertext || GCM tag).
// Uses AES-256-GCM with a random 12-byte nonce per call.
func (e *Encrypter) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce.
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt strips the "enc:v1:" prefix, base64-decodes, splits out the 12-byte
// nonce, and decrypts. Returns error on wrong key, tampered data, or missing prefix.
func (e *Encrypter) Decrypt(ciphertext string) ([]byte, error) {
	if !strings.HasPrefix(ciphertext, prefix) {
		return nil, fmt.Errorf("crypto: missing enc:v1: prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plain, nil
}
