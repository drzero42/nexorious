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

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
)

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

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type messageResponse struct {
	Message string `json:"message"`
}

// issueSession creates a session row and returns the raw session ID.
// Always uses context.Background() so client disconnect cannot abort the DB write.
func issueSession(ctx context.Context, db *bun.DB, expireDays int, userID, userAgent, ip string) (string, error) {
	sessionID, err := auth.GenerateSessionID()
	if err != nil {
		return "", fmt.Errorf("issueSession: %w", err)
	}
	var uaVal, ipVal any
	if userAgent != "" {
		uaVal = userAgent
	}
	if ip != "" {
		ipVal = ip
	}
	expiresAt := time.Now().Add(time.Duration(expireDays) * 24 * time.Hour)
	_, err = db.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, session_id_hash, user_agent, ip_address, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(),
		userID,
		auth.HashToken(sessionID),
		uaVal,
		ipVal,
		expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("issueSession: insert: %w", err)
	}
	return sessionID, nil
}

// meResponse is returned by login, GET /api/auth/me, and setup.
type meResponse struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	IsAdmin     bool            `json:"is_admin"`
	IsActive    bool            `json:"is_active"`
	Preferences json.RawMessage `json:"preferences"`
	CreatedAt   time.Time       `json:"created_at"`
}

func loadMeResponse(ctx context.Context, db *bun.DB, userID string) (*meResponse, error) {
	var resp meResponse
	var prefs []byte
	err := db.QueryRowContext(ctx,
		`SELECT id, username, is_admin, is_active, preferences, created_at
		 FROM users WHERE id = ?`,
		userID,
	).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
	if err != nil {
		return nil, err
	}
	if prefs == nil {
		resp.Preferences = json.RawMessage("{}")
	} else {
		resp.Preferences = json.RawMessage(prefs)
	}
	return &resp, nil
}

// HandleLogin handles POST /api/auth/login.
func (h *AuthHandler) HandleLogin(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	var userID, passwordHash string
	var isActive bool
	err := h.db.QueryRowContext(context.Background(),
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

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
	}
	if !isActive {
		return echo.NewHTTPError(http.StatusUnauthorized, "user account is disabled")
	}

	sessionID, err := issueSession(context.Background(), h.db, h.cfg.SessionExpireDays,
		userID, c.Request().Header.Get("User-Agent"), c.RealIP())
	if err != nil {
		slog.Error("login: issue session", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	auth.SetSessionCookie(c, sessionID, h.cfg.SessionExpireDays)

	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("login: load user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleLogout handles POST /api/auth/logout.
func (h *AuthHandler) HandleLogout(c *echo.Context) error {
	cookie, err := c.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		hash := auth.HashToken(cookie.Value)
		if _, err := h.db.ExecContext(context.Background(),
			"DELETE FROM user_sessions WHERE session_id_hash = ?", hash,
		); err != nil {
			slog.Error("logout: delete session", "err", err)
		}
	}
	auth.ClearSessionCookie(c)
	return c.JSON(http.StatusOK, messageResponse{Message: "Successfully logged out"})
}

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
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.CurrentPassword)); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "current password is incorrect")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		slog.Error("change password: bcrypt", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET password_hash = ?, updated_at = NOW() WHERE id = ?",
		string(newHash), userID,
	); err != nil {
		slog.Error("change password: update hash", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}

	currentHash, _ := c.Get("session_hash").(string)
	if _, err := h.db.ExecContext(context.Background(),
		"DELETE FROM user_sessions WHERE user_id = ? AND session_id_hash != ?",
		userID, currentHash,
	); err != nil {
		slog.Error("change password: invalidate sessions", "err", err)
	}

	return c.JSON(http.StatusOK, messageResponse{Message: "Password changed successfully."})
}

// HandleGetMe handles GET /api/auth/me.
func (h *AuthHandler) HandleGetMe(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	resp, err := loadMeResponse(c.Request().Context(), h.db, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
		}
		slog.Error("get me: query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

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
	if req.Preferences == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	var obj map[string]any
	if err := json.Unmarshal(req.Preferences, &obj); err != nil || obj == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "preferences must be a JSON object")
	}
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET preferences = ?, updated_at = NOW() WHERE id = ?",
		string(req.Preferences), userID,
	); err != nil {
		slog.Error("update me: update preferences", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("update me: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}

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
	return c.JSON(http.StatusOK, usernameAvailabilityResponse{Available: available, Username: username})
}

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
	if _, err := h.db.ExecContext(context.Background(),
		"UPDATE users SET username = ?, updated_at = NOW() WHERE id = ?",
		req.NewUsername, userID,
	); err != nil {
		slog.Error("change username: update", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	resp, err := loadMeResponse(context.Background(), h.db, userID)
	if err != nil {
		slog.Error("change username: re-query user", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, resp)
}
