package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/drzero42/nexorious-go/internal/auth"
)

func TestGenerateAndParseAccessToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	token, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ParseToken(secret, token, "access")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID)
	}
	if claims.Type != "access" {
		t.Errorf("Type = %q, want %q", claims.Type, "access")
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}
	expectedExpiry := time.Now().Add(15 * time.Minute)
	diff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt off by %v", diff)
	}
}

func TestGenerateAndParseRefreshToken(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	token, err := auth.GenerateRefreshToken(secret, userID, 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := auth.ParseToken(secret, token, "refresh")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("Subject = %q, want %q", claims.Subject, userID)
	}
	if claims.Type != "refresh" {
		t.Errorf("Type = %q, want %q", claims.Type, "refresh")
	}
	expectedExpiry := time.Now().Add(30 * 24 * time.Hour)
	diff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt off by %v", diff)
	}
}

func TestParseToken_WrongType(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	accessToken, err := auth.GenerateAccessToken(secret, userID, 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	_, err = auth.ParseToken(secret, accessToken, "refresh")
	if err == nil {
		t.Fatal("expected error when parsing access token as refresh")
	}

	refreshToken, err := auth.GenerateRefreshToken(secret, userID, 30)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	_, err = auth.ParseToken(secret, refreshToken, "access")
	if err == nil {
		t.Fatal("expected error when parsing refresh token as access")
	}
}

func TestParseToken_Expired(t *testing.T) {
	secret := "test-secret-key-at-least-32-bytes!"
	userID := "550e8400-e29b-41d4-a716-446655440000"

	claims := auth.Claims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-16 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = auth.ParseToken(secret, signed, "access")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseToken_WrongKey(t *testing.T) {
	token, err := auth.GenerateAccessToken("key-A-at-least-32-bytes-long!!!!", "user1", 15)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	_, err = auth.ParseToken("key-B-at-least-32-bytes-long!!!!", token, "access")
	if err == nil {
		t.Fatal("expected error when verifying with wrong key")
	}
}

func TestParseToken_Malformed(t *testing.T) {
	_, err := auth.ParseToken("secret-key-at-least-32-bytes!!!!!", "not.a.jwt.token", "access")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestHashToken(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiJ9.test"
	hash1 := auth.HashToken(token)
	hash2 := auth.HashToken(token)

	if hash1 != hash2 {
		t.Errorf("HashToken not deterministic: %q != %q", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got length %d", len(hash1))
	}

	hash3 := auth.HashToken("different-token")
	if hash1 == hash3 {
		t.Error("different inputs produced the same hash")
	}
}
