package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
)

// bcryptCost is the work factor used for all password hash creation sites.
const bcryptCost = 12

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

// NewAuthHandler returns a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{pool: pool, cfg: cfg}
}

// loginRequest is the JSON body for POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// refreshRequest is the JSON body for POST /api/auth/refresh.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// logoutRequest is the JSON body for POST /api/auth/logout.
type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// tokenResponse is returned by login and refresh.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// HandleLogin handles POST /api/auth/login.
//
// Validates credentials, creates a session row, and returns access + refresh tokens.
func (h *AuthHandler) HandleLogin(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Look up the user by username.
	var (
		userID       string
		passwordHash string
		isActive     bool
	)
	err := h.pool.QueryRow(
		context.Background(),
		"SELECT id, password_hash, is_active FROM users WHERE username = $1",
		req.Username,
	).Scan(&userID, &passwordHash, &isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "incorrect username or password"})
		}
		slog.Error("login: query user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Verify the password before checking is_active (prevents timing-based user enumeration).
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "incorrect username or password"})
	}

	if !isActive {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user account is disabled"})
	}

	// Generate tokens.
	accessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("login: generate access token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	refreshToken, err := auth.GenerateRefreshToken(h.cfg.SecretKey, userID, h.cfg.RefreshTokenExpireDays)
	if err != nil {
		slog.Error("login: generate refresh token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Persist the session.
	sessionID := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(h.cfg.RefreshTokenExpireDays) * 24 * time.Hour)
	_, err = h.pool.Exec(
		context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID,
		userID,
		auth.HashToken(accessToken),
		auth.HashToken(refreshToken),
		c.Request().Header.Get("User-Agent"),
		c.RealIP(),
		expiresAt,
	)
	if err != nil {
		slog.Error("login: insert session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "bearer",
		ExpiresIn:    h.cfg.AccessTokenExpireMinutes * 60,
	})
}

// HandleRefresh handles POST /api/auth/refresh.
//
// Validates the refresh token, issues a new access token, and updates the session's token_hash.
// The refresh token itself is not rotated.
func (h *AuthHandler) HandleRefresh(c *echo.Context) error {
	var req refreshRequest
	if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate the refresh JWT.
	claims, err := auth.ParseToken(h.cfg.SecretKey, req.RefreshToken, "refresh")
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}
	userID := claims.Subject
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
	}

	// Look up the session.
	refreshHash := auth.HashToken(req.RefreshToken)
	var sessionID string
	err = h.pool.QueryRow(
		context.Background(),
		"SELECT id FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2 AND expires_at > now()",
		userID, refreshHash,
	).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired refresh token"})
		}
		slog.Error("refresh: query session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Check the user is still active.
	var isActive bool
	err = h.pool.QueryRow(
		context.Background(),
		"SELECT is_active FROM users WHERE id = $1",
		userID,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found or disabled"})
		}
		slog.Error("refresh: query user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if !isActive {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found or disabled"})
	}

	// Issue a new access token.
	newAccessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("refresh: generate access token", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Update the session's access token hash.
	_, err = h.pool.Exec(
		context.Background(),
		"UPDATE user_sessions SET token_hash = $1 WHERE id = $2",
		auth.HashToken(newAccessToken), sessionID,
	)
	if err != nil {
		slog.Error("refresh: update session", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: req.RefreshToken, // unchanged
		TokenType:    "bearer",
		ExpiresIn:    h.cfg.AccessTokenExpireMinutes * 60,
	})
}

// HandleLogout handles POST /api/auth/logout.
//
// Requires a valid access token (JWTMiddleware). Deletes the session identified by the
// refresh token. Always returns 200 — errors from an invalid refresh token are logged but
// do not prevent a successful logout response (security: don't reveal token validity).
func (h *AuthHandler) HandleLogout(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req logoutRequest
	// A missing/malformed body still results in 200 — logout is idempotent.
	_ = c.Bind(&req)

	if req.RefreshToken == "" {
		return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
	}

	// Parse the refresh token. On error, return 200 for security.
	claims, err := auth.ParseToken(h.cfg.SecretKey, req.RefreshToken, "refresh")
	if err != nil {
		slog.Info("logout: could not parse refresh token (returning 200 for security)", "err", err)
		return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
	}

	// Guard against logging out another user's session.
	if claims.Subject != userID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid refresh token for authenticated user"})
	}

	// Delete the session. Ignore "not found" — idempotent.
	_, err = h.pool.Exec(
		context.Background(),
		"DELETE FROM user_sessions WHERE user_id = $1 AND refresh_token_hash = $2",
		userID, auth.HashToken(req.RefreshToken),
	)
	if err != nil {
		slog.Error("logout: delete session", "err", err)
		// Still return 200 — the client can't do anything about a DB error here.
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Successfully logged out"})
}

// issueTokensAndSession generates an access + refresh token pair and persists a session row.
// Always uses context.Background() so client disconnect cannot abort DB writes.
func issueTokensAndSession(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg *config.Config,
	userID string,
	userAgent string,
	ip string,
) (accessToken, refreshToken string, err error) {
	accessToken, err = auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
	if err != nil {
		return "", "", fmt.Errorf("issueTokens: generate access token: %w", err)
	}
	refreshToken, err = auth.GenerateRefreshToken(cfg.SecretKey, userID, cfg.RefreshTokenExpireDays)
	if err != nil {
		return "", "", fmt.Errorf("issueTokens: generate refresh token: %w", err)
	}

	sessionID := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(cfg.RefreshTokenExpireDays) * 24 * time.Hour)
	_, err = pool.Exec(ctx,
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID,
		userID,
		auth.HashToken(accessToken),
		auth.HashToken(refreshToken),
		userAgent,
		ip,
		expiresAt,
	)
	if err != nil {
		return "", "", fmt.Errorf("issueTokens: insert session: %w", err)
	}
	return accessToken, refreshToken, nil
}

// meResponse is returned by GET /api/auth/me.
type meResponse struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	IsAdmin     bool            `json:"is_admin"`
	IsActive    bool            `json:"is_active"`
	Preferences json.RawMessage `json:"preferences"`
	CreatedAt   time.Time       `json:"created_at"`
}

// HandleGetMe handles GET /api/auth/me.
func (h *AuthHandler) HandleGetMe(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var resp meResponse
	var prefs []byte
	err := h.pool.QueryRow(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
		}
		slog.Error("get me: query user", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}

	return c.JSON(http.StatusOK, resp)
}
