package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
)

// bcryptCost is the work factor used for all password hash creation sites.
const bcryptCost = 12

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	db  *bun.DB
	cfg *config.Config
}

// NewAuthHandler returns a new AuthHandler.
func NewAuthHandler(db *bun.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Look up the user by username.
	var (
		userID       string
		passwordHash string
		isActive     bool
	)
	err := h.db.QueryRowContext(
		context.Background(),
		"SELECT id, password_hash, is_active FROM users WHERE username = ?",
		req.Username,
	).Scan(&userID, &passwordHash, &isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
		}
		slog.Error("login: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Verify the password before checking is_active (prevents timing-based user enumeration).
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
	}

	if !isActive {
		return echo.NewHTTPError(http.StatusUnauthorized, "user account is disabled")
	}

	// Generate tokens.
	accessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("login: generate access token", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	refreshToken, err := auth.GenerateRefreshToken(h.cfg.SecretKey, userID, h.cfg.RefreshTokenExpireDays)
	if err != nil {
		slog.Error("login: generate refresh token", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Persist the session.
	sessionID := uuid.NewString()
	expiresAt := time.Now().Add(time.Duration(h.cfg.RefreshTokenExpireDays) * 24 * time.Hour)
	_, err = h.db.ExecContext(
		context.Background(),
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
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
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate the refresh JWT.
	claims, err := auth.ParseToken(h.cfg.SecretKey, req.RefreshToken, "refresh")
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid refresh token")
	}
	userID := claims.Subject
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid refresh token")
	}

	// Look up the session.
	refreshHash := auth.HashToken(req.RefreshToken)
	var sessionID string
	err = h.db.QueryRowContext(
		context.Background(),
		"SELECT id FROM user_sessions WHERE user_id = ? AND refresh_token_hash = ? AND expires_at > now()",
		userID, refreshHash,
	).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired refresh token")
		}
		slog.Error("refresh: query session", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Check the user is still active.
	var isActive bool
	err = h.db.QueryRowContext(
		context.Background(),
		"SELECT is_active FROM users WHERE id = ?",
		userID,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not found or disabled")
		}
		slog.Error("refresh: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if !isActive {
		return echo.NewHTTPError(http.StatusUnauthorized, "user not found or disabled")
	}

	// Issue a new access token.
	newAccessToken, err := auth.GenerateAccessToken(h.cfg.SecretKey, userID, h.cfg.AccessTokenExpireMinutes)
	if err != nil {
		slog.Error("refresh: generate access token", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Update the session's access token hash.
	_, err = h.db.ExecContext(
		context.Background(),
		"UPDATE user_sessions SET token_hash = ? WHERE id = ?",
		auth.HashToken(newAccessToken), sessionID,
	)
	if err != nil {
		slog.Error("refresh: update session", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
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
		return echo.NewHTTPError(http.StatusBadRequest, "invalid refresh token for authenticated user")
	}

	// Delete the session. Ignore "not found" — idempotent.
	_, err = h.db.ExecContext(
		context.Background(),
		"DELETE FROM user_sessions WHERE user_id = ? AND refresh_token_hash = ?",
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
	db *bun.DB,
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
	_, err = db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
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

// messageResponse is a generic JSON response with a single message field.
type messageResponse struct {
	Message string `json:"message"`
}

// changePasswordRequest is the JSON body for PUT /api/auth/change-password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// HandleChangePassword handles PUT /api/auth/change-password.
func (h *AuthHandler) HandleChangePassword(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req changePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.CurrentPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is required")
	}
	if len(req.NewPassword) < 8 || len(req.NewPassword) > 128 {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be between 8 and 128 characters")
	}
	if req.CurrentPassword == req.NewPassword {
		return echo.NewHTTPError(http.StatusBadRequest, "new password must be different from current password")
	}

	// Fetch the stored password hash.
	var storedHash string
	err := h.db.QueryRowContext(context.Background(),
		"SELECT password_hash FROM users WHERE id = ?", userID,
	).Scan(&storedHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		slog.Error("change password: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Verify current password.
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.CurrentPassword)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is incorrect")
	}

	// Hash and store new password.
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		slog.Error("change password: bcrypt", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	_, err = h.db.ExecContext(context.Background(),
		`UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?`,
		string(newHash), userID,
	)
	if err != nil {
		slog.Error("change password: update hash", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Invalidate other sessions (keep the current one).
	authHeader := c.Request().Header.Get("Authorization")
	currentTokenHash := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		currentTokenHash = auth.HashToken(authHeader[7:])
	}

	_, err = h.db.ExecContext(context.Background(),
		`DELETE FROM user_sessions WHERE user_id = ? AND token_hash != ?`,
		userID, currentTokenHash,
	)
	if err != nil {
		slog.Error("change password: invalidate sessions", "err", err)
		// Non-fatal: password is already changed.
	}

	return c.JSON(http.StatusOK, messageResponse{Message: "Password changed successfully."})
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
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var resp meResponse
	var prefs []byte
	err := h.db.QueryRowContext(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
		}
		slog.Error("get me: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}

	return c.JSON(http.StatusOK, resp)
}

// updateMeRequest is the JSON body for PUT /api/auth/me.
type updateMeRequest struct {
	Preferences json.RawMessage `json:"preferences"`
}

// HandleUpdateMe handles PUT /api/auth/me.
func (h *AuthHandler) HandleUpdateMe(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req updateMeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate preferences is a JSON object (not null, array, or scalar).
	if req.Preferences == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	var obj map[string]any
	if err := json.Unmarshal(req.Preferences, &obj); err != nil || obj == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}

	_, err := h.db.ExecContext(context.Background(),
		`UPDATE users SET preferences = ?, updated_at = NOW() WHERE id = ?`,
		string(req.Preferences), userID,
	)
	if err != nil {
		slog.Error("update me: update preferences", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Re-query and return the full profile.
	var resp meResponse
	var prefs []byte
	err = h.db.QueryRowContext(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		slog.Error("update me: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}

	return c.JSON(http.StatusOK, resp)
}

// usernameAvailabilityResponse is returned by GET /api/auth/username/check/:username.
type usernameAvailabilityResponse struct {
	Available bool   `json:"available"`
	Username  string `json:"username"`
}

// HandleCheckUsername handles GET /api/auth/username/check/:username.
func (h *AuthHandler) HandleCheckUsername(c *echo.Context) error {
	username := c.Param("username")

	if len(username) < 3 || len(username) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be between 3 and 100 characters")
	}

	var exists int
	err := h.db.QueryRowContext(context.Background(),
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", username,
	).Scan(&exists)

	available := errors.Is(err, sql.ErrNoRows)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("check username: query", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	return c.JSON(http.StatusOK, usernameAvailabilityResponse{
		Available: available,
		Username:  username,
	})
}

// changeUsernameRequest is the JSON body for PUT /api/auth/username.
type changeUsernameRequest struct {
	NewUsername string `json:"new_username"`
}

// HandleChangeUsername handles PUT /api/auth/username.
func (h *AuthHandler) HandleChangeUsername(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req changeUsernameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.NewUsername) < 3 || len(req.NewUsername) > 100 {
		return echo.NewHTTPError(http.StatusBadRequest, "username must be between 3 and 100 characters")
	}

	// Check if same as current.
	var currentUsername string
	err := h.db.QueryRowContext(context.Background(),
		"SELECT username FROM users WHERE id = ?", userID,
	).Scan(&currentUsername)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		slog.Error("change username: query current", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if req.NewUsername == currentUsername {
		return echo.NewHTTPError(http.StatusBadRequest, "new username must be different from current username")
	}

	// Check availability.
	var usernameExists int
	err = h.db.QueryRowContext(context.Background(),
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", req.NewUsername,
	).Scan(&usernameExists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("change username: check availability", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if err == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "username already taken")
	}

	// Update username.
	_, err = h.db.ExecContext(context.Background(),
		`UPDATE users SET username = ?, updated_at = NOW() WHERE id = ?`,
		req.NewUsername, userID,
	)
	if err != nil {
		slog.Error("change username: update", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	// Re-query and return profile.
	var resp meResponse
	var updatedPrefs []byte
	err = h.db.QueryRowContext(context.Background(),
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &updatedPrefs, &resp.CreatedAt)
	if err != nil {
		slog.Error("change username: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	if updatedPrefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(updatedPrefs)
	}

	return c.JSON(http.StatusOK, resp)
}
