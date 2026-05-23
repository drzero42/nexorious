package crypto_test

import (
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/crypto"
)

const testKey = "test-db-encryption-key-32-bytes!!"

func mustEncrypter(t *testing.T, key string) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter(key)
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}
	return enc
}

func TestEncrypter_RoundTrip(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	plain := []byte(`{"web_api_key":"abc","steam_id":"123"}`)
	ciphertext, err := enc.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ciphertext, "enc:v1:") {
		t.Fatalf("expected enc:v1: prefix, got %q", ciphertext)
	}
	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestEncrypter_UniqueNonces(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	plain := []byte("same plaintext")
	c1, _ := enc.Encrypt(plain)
	c2, _ := enc.Encrypt(plain)
	if c1 == c2 {
		t.Fatal("two encryptions of the same plaintext must produce different ciphertexts")
	}
}

func TestEncrypter_TamperDetection(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	ciphertext, _ := enc.Encrypt([]byte("sensitive"))
	// flip the last byte of the base64 payload
	b := []byte(ciphertext)
	b[len(b)-1] ^= 0xFF
	_, err := enc.Decrypt(string(b))
	if err == nil {
		t.Fatal("expected error on tampered ciphertext, got nil")
	}
}

func TestEncrypter_WrongKey(t *testing.T) {
	enc1 := mustEncrypter(t, testKey)
	enc2 := mustEncrypter(t, "different-db-encryption-key-32b!")
	ciphertext, _ := enc1.Encrypt([]byte("secret"))
	_, err := enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

func TestEncrypter_MissingPrefix(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	_, err := enc.Decrypt("plain json, no prefix")
	if err == nil {
		t.Fatal("expected error for missing enc:v1: prefix, got nil")
	}
}

func TestEncrypter_EmptyPlaintext(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	_, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
}

func TestNewEncrypter_ShortKey(t *testing.T) {
	_, err := crypto.NewEncrypter("tooshort")
	if err == nil {
		t.Fatal("expected error for key shorter than 32 bytes, got nil")
	}
}

