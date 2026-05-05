package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
)

// Claims represents the JWT payload for both access and refresh tokens.
// Differentiated by the Type field ("access" or "refresh").
type Claims struct {
	Type string `json:"type"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a short-lived JWT with type="access".
func GenerateAccessToken(secretKey string, userID string, expireMinutes int) (string, error) {
	if secretKey == "" {
		return "", fmt.Errorf("secretKey must not be empty")
	}
	if expireMinutes <= 0 {
		return "", fmt.Errorf("expireMinutes must be positive, got %d", expireMinutes)
	}
	now := time.Now()
	claims := Claims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// GenerateRefreshToken creates a long-lived JWT with type="refresh".
func GenerateRefreshToken(secretKey string, userID string, expireDays int) (string, error) {
	if secretKey == "" {
		return "", fmt.Errorf("secretKey must not be empty")
	}
	if expireDays <= 0 {
		return "", fmt.Errorf("expireDays must be positive, got %d", expireDays)
	}
	now := time.Now()
	claims := Claims{
		Type: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireDays) * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secretKey))
}

// ParseToken validates a JWT string, checks the type claim matches expectedType,
// and returns the claims. Returns an error if malformed, expired, wrong key, or wrong type.
func ParseToken(secretKey string, tokenString string, expectedType string) (*Claims, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("secretKey must not be empty")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.Type != expectedType {
		return nil, fmt.Errorf("token type %q does not match expected %q", claims.Type, expectedType)
	}
	return claims, nil
}

// HashToken returns the SHA-256 hex digest of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// UserIDFromContext extracts user_id set by JWTMiddleware. Returns "" if unset.
func UserIDFromContext(c *echo.Context) string {
	v := c.Get("user_id")
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// IsAdminFromContext extracts is_admin set by JWTMiddleware. Returns false if unset.
func IsAdminFromContext(c *echo.Context) bool {
	v := c.Get("is_admin")
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// AdminMiddleware returns middleware that requires is_admin=true on the context.
// Must be applied after JWTMiddleware.
func AdminMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !IsAdminFromContext(c) {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "admin access required",
				})
			}
			return next(c)
		}
	}
}
