package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
)

const sessionCookieName = "session_id"

// GenerateSessionID returns a cryptographically random 64-char hex string.
func GenerateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("GenerateSessionID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateAPIKey returns a cryptographically random API key prefixed with "nxr_".
func GenerateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("GenerateAPIKey: %w", err)
	}
	return "nxr_" + hex.EncodeToString(b), nil
}

// SetSessionCookie writes an HttpOnly SameSite=Strict session cookie.
func SetSessionCookie(c *echo.Context, sessionID string, expireDays int) {
	cookie := new(http.Cookie)
	cookie.Name = sessionCookieName
	cookie.Value = sessionID
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Secure = true
	cookie.Path = "/"
	cookie.MaxAge = expireDays * 86400
	c.SetCookie(cookie)
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(c *echo.Context) {
	cookie := new(http.Cookie)
	cookie.Name = sessionCookieName
	cookie.Value = ""
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode
	cookie.Secure = true
	cookie.Path = "/"
	cookie.MaxAge = 0
	c.SetCookie(cookie)
}

// AuthMiddleware tries cookie-based session auth first, then Bearer API key.
func AuthMiddleware(db *bun.DB, expireDays int) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			userID, sessionHash, cookieErr := trySessionCookie(c, db)
			if cookieErr != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "session expired or not found"})
			}
			if userID == "" {
				apiUserID, apiErr := tryAPIKey(c, db)
				if apiErr != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired api key"})
				}
				if apiUserID == "" {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing or invalid authorization"})
				}
				userID = apiUserID
			}

			var user AuthUser
			err := db.QueryRowContext(c.Request().Context(),
				"SELECT id, username, is_active, is_admin FROM users WHERE id = ?",
				userID,
			).Scan(&user.ID, &user.Username, &user.IsActive, &user.IsAdmin)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
				}
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
			if !user.IsActive {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user account is disabled"})
			}

			c.Set("user_id", user.ID)
			c.Set("is_admin", user.IsAdmin)
			c.Set("user", &user)
			c.Set("session_hash", sessionHash)

			if sessionHash != "" {
				go func() {
					if _, err := db.ExecContext(context.Background(),
						"UPDATE user_sessions SET last_used_at = now() WHERE session_id_hash = ?",
						sessionHash,
					); err != nil {
						slog.Error("auth: update session last_used_at", "err", err)
					}
				}()
			}

			return next(c)
		}
	}
}

// trySessionCookie checks for a session cookie. Returns ("", "", nil) when no
// cookie is present (fall through to API key). Returns an error when a cookie
// is present but the session is not found or expired.
func trySessionCookie(c *echo.Context, db *bun.DB) (userID, sessionHash string, err error) {
	cookie, cookieErr := c.Cookie(sessionCookieName)
	if cookieErr != nil {
		return "", "", nil
	}
	hash := HashToken(cookie.Value)
	var uid string
	err = db.QueryRowContext(c.Request().Context(),
		"SELECT user_id FROM user_sessions WHERE session_id_hash = ? AND expires_at > now()",
		hash,
	).Scan(&uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.New("session expired or not found")
		}
		return "", "", err
	}
	return uid, hash, nil
}

// tryAPIKey checks for a Bearer API key. Returns ("", nil) when no Bearer
// header is present. Returns an error when a key is present but invalid.
func tryAPIKey(c *echo.Context, db *bun.DB) (string, error) {
	authHeader := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", nil
	}
	raw := strings.TrimPrefix(authHeader, "Bearer ")
	hash := HashToken(raw)
	var userID string
	err := db.QueryRowContext(c.Request().Context(),
		`SELECT user_id FROM api_keys
		 WHERE key_hash = ? AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())`,
		hash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("invalid or expired api key")
		}
		return "", err
	}
	go func() {
		if _, err := db.ExecContext(context.Background(),
			"UPDATE api_keys SET last_used_at = now() WHERE key_hash = ?",
			hash,
		); err != nil {
			slog.Error("auth: update api key last_used_at", "err", err)
		}
	}()
	return userID, nil
}
