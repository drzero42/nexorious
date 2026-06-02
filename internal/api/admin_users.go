package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/notify"
)

// adminUserResponse is the serialised user shape returned by every admin endpoint.
// It deliberately omits password_hash and preferences.
type adminUserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	IsActive  bool      `json:"is_active"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newAdminUserResponse(u *models.User) adminUserResponse {
	return adminUserResponse{
		ID:        u.ID,
		Username:  u.Username,
		IsActive:  u.IsActive,
		IsAdmin:   u.IsAdmin,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// adminCreateUserRequest is the body for POST /api/auth/admin/users.
type adminCreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

// adminUpdateUserRequest is the body for PUT /api/auth/admin/users/:id.
// Pointer fields distinguish "absent" from "explicit zero".
type adminUpdateUserRequest struct {
	Username *string `json:"username,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
	IsAdmin  *bool   `json:"is_admin,omitempty"`
}

// adminResetPasswordRequest is the body for PUT /api/auth/admin/users/:id/password.
type adminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// adminDeletionImpactResponse summarises the rows that will cascade-delete with a user.
type adminDeletionImpactResponse struct {
	UserID           string `json:"user_id"`
	Username         string `json:"username"`
	TotalGames       int    `json:"total_games"`
	TotalTags        int    `json:"total_tags"`
	TotalImportJobs  int    `json:"total_import_jobs"`
	TotalExportJobs  int    `json:"total_export_jobs"`
	TotalSyncJobs    int    `json:"total_sync_jobs"`
	TotalSyncConfigs int    `json:"total_sync_configs"`
	TotalSessions    int    `json:"total_sessions"`
	Warning          string `json:"warning"`
}

// adminSuccessResponse is a simple message envelope used by reset password and delete.
type adminSuccessResponse struct {
	Message string `json:"message"`
}

// AdminUsersHandler exposes the seven /api/auth/admin/users/* endpoints.
type AdminUsersHandler struct {
	db *bun.DB
}

// NewAdminUsersHandler constructs an AdminUsersHandler.
func NewAdminUsersHandler(db *bun.DB) *AdminUsersHandler {
	return &AdminUsersHandler{db: db}
}

// RegisterRoutes registers all admin user management routes on the given group.
// The caller is responsible for applying AuthMiddleware + AdminMiddleware to the group.
// Static-segment routes are registered before parameterised ones — Echo v5 does not
// auto-sort, and the wrong order would cause GET /:id/password to be matched by
// GET /:id.
func (h *AdminUsersHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/api/auth/admin/users", h.HandleCreate)
	g.GET("/api/auth/admin/users", h.HandleList)
	g.PUT("/api/auth/admin/users/:id/password", h.HandleResetPassword)
	g.GET("/api/auth/admin/users/:id/deletion-impact", h.HandleDeletionImpact)
	g.GET("/api/auth/admin/users/:id", h.HandleGet)
	g.PUT("/api/auth/admin/users/:id", h.HandleUpdate)
	g.DELETE("/api/auth/admin/users/:id", h.HandleDelete)
}

// errorJSON writes a plain {"error":"..."} body with the given status code.
func errorJSON(c *echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

// HandleCreate creates a new user. Admin-only.
func (h *AdminUsersHandler) HandleCreate(c *echo.Context) error {
	var req adminCreateUserRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	username := strings.TrimSpace(req.Username)
	if username == "" {
		return errorJSON(c, http.StatusBadRequest, "username is required")
	}
	if len(username) < 3 {
		return errorJSON(c, http.StatusBadRequest, "username must be at least 3 characters")
	}
	if req.Password == "" {
		return errorJSON(c, http.StatusBadRequest, "password is required")
	}
	if len(req.Password) < 6 {
		return errorJSON(c, http.StatusBadRequest, "password must be at least 6 characters")
	}

	ctx := context.Background()

	// Uniqueness check (case-sensitive, matching the existing users.username index).
	var existing int
	err := h.db.QueryRowContext(ctx,
		"SELECT 1 FROM users WHERE username = ? LIMIT 1", username,
	).Scan(&existing)
	if err == nil {
		return errorJSON(c, http.StatusBadRequest, "username already taken")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.Error("admin create user: uniqueness check", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), auth.BcryptCost)
	if err != nil {
		slog.Error("admin create user: bcrypt", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	now := time.Now().UTC()
	user := &models.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		IsActive:     true,
		IsAdmin:      req.IsAdmin,
		Preferences:  "{}",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if _, err := h.db.NewInsert().Model(user).Exec(ctx); err != nil {
		slog.Error("admin create user: insert", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	if err := notify.SeedDefaultSubscriptions(ctx, h.db, user.ID, user.IsAdmin); err != nil {
		slog.Error("admin create user: seed notification subscriptions", "user_id", user.ID, "err", err)
	}

	return c.JSON(http.StatusCreated, newAdminUserResponse(user))
}

// HandleList returns all users, newest first.
func (h *AdminUsersHandler) HandleList(c *echo.Context) error {
	ctx := context.Background()
	var users []models.User
	if err := h.db.NewSelect().Model(&users).Order("created_at DESC").Scan(ctx); err != nil {
		slog.Error("admin list users: scan", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}
	out := make([]adminUserResponse, 0, len(users))
	for i := range users {
		out = append(out, newAdminUserResponse(&users[i]))
	}
	return c.JSON(http.StatusOK, out)
}

// HandleGet returns a single user by ID.
func (h *AdminUsersHandler) HandleGet(c *echo.Context) error {
	id := c.Param("id")
	user, err := h.loadUser(context.Background(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errorJSON(c, http.StatusNotFound, "user not found")
		}
		slog.Error("admin get user: scan", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}
	return c.JSON(http.StatusOK, newAdminUserResponse(user))
}

// HandleUpdate applies a partial update to the user.
func (h *AdminUsersHandler) HandleUpdate(c *echo.Context) error {
	id := c.Param("id")
	var req adminUpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	ctx := context.Background()
	user, err := h.loadUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errorJSON(c, http.StatusNotFound, "user not found")
		}
		slog.Error("admin update user: load", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	currentUserID := auth.UserIDFromContext(c)

	// Self-protection
	if req.IsActive != nil && !*req.IsActive && id == currentUserID {
		return errorJSON(c, http.StatusBadRequest, "Cannot deactivate your own account")
	}
	if req.IsAdmin != nil && !*req.IsAdmin && id == currentUserID {
		return errorJSON(c, http.StatusBadRequest, "Cannot remove your own admin privileges")
	}

	// Apply username change with validation + uniqueness against other rows.
	if req.Username != nil {
		newName := strings.TrimSpace(*req.Username)
		if newName == "" {
			return errorJSON(c, http.StatusBadRequest, "username is required")
		}
		if len(newName) < 3 {
			return errorJSON(c, http.StatusBadRequest, "username must be at least 3 characters")
		}
		if newName != user.Username {
			var existing int
			err := h.db.QueryRowContext(ctx,
				"SELECT 1 FROM users WHERE username = ? AND id != ? LIMIT 1",
				newName, id,
			).Scan(&existing)
			if err == nil {
				return errorJSON(c, http.StatusBadRequest, "username already taken")
			}
			if !errors.Is(err, sql.ErrNoRows) {
				slog.Error("admin update user: uniqueness check", "err", err)
				return errorJSON(c, http.StatusInternalServerError, "internal server error")
			}
			user.Username = newName
		}
	}

	deactivating := false
	if req.IsActive != nil {
		if !*req.IsActive && user.IsActive {
			deactivating = true
		}
		user.IsActive = *req.IsActive
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}

	user.UpdatedAt = time.Now().UTC()

	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model(user).WherePK().Exec(ctx); err != nil {
			return err
		}
		if deactivating {
			if _, err := tx.NewDelete().
				Model((*models.UserSession)(nil)).
				Where("user_id = ?", id).
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("admin update user: tx", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	return c.JSON(http.StatusOK, newAdminUserResponse(user))
}

// HandleResetPassword sets a new password and wipes all sessions for the target user.
func (h *AdminUsersHandler) HandleResetPassword(c *echo.Context) error {
	id := c.Param("id")
	var req adminResetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return errorJSON(c, http.StatusBadRequest, "invalid request body")
	}
	if req.NewPassword == "" {
		return errorJSON(c, http.StatusBadRequest, "new password is required")
	}
	if len(req.NewPassword) < 6 {
		return errorJSON(c, http.StatusBadRequest, "new password must be at least 6 characters")
	}

	ctx := context.Background()
	if _, err := h.loadUser(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errorJSON(c, http.StatusNotFound, "user not found")
		}
		slog.Error("admin reset password: load", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), auth.BcryptCost)
	if err != nil {
		slog.Error("admin reset password: bcrypt", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	now := time.Now().UTC()
	err = h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
			string(hash), now, id,
		); err != nil {
			return err
		}
		if _, err := tx.NewDelete().
			Model((*models.UserSession)(nil)).
			Where("user_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("admin reset password: tx", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	return c.JSON(http.StatusOK, adminSuccessResponse{
		Message: "Password reset successfully. User will need to log in again.",
	})
}

// HandleDeletionImpact returns the per-table row counts that would be deleted
// alongside the user.
func (h *AdminUsersHandler) HandleDeletionImpact(c *echo.Context) error {
	id := c.Param("id")
	ctx := context.Background()

	user, err := h.loadUser(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errorJSON(c, http.StatusNotFound, "user not found")
		}
		slog.Error("admin deletion impact: load", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	if id == auth.UserIDFromContext(c) {
		return errorJSON(c, http.StatusBadRequest, "Cannot delete your own account")
	}

	type countQuery struct {
		query string
		args  []any
		dest  *int
	}

	var games, tags, importJobs, exportJobs, syncJobs, syncConfigs, sessions int
	queries := []countQuery{
		{`SELECT COUNT(*) FROM user_games WHERE user_id = ?`, []any{id}, &games},
		{`SELECT COUNT(*) FROM tags WHERE user_id = ?`, []any{id}, &tags},
		{`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'import'`, []any{id}, &importJobs},
		{`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'export'`, []any{id}, &exportJobs},
		{`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync'`, []any{id}, &syncJobs},
		{`SELECT COUNT(*) FROM user_sync_configs WHERE user_id = ?`, []any{id}, &syncConfigs},
		{`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`, []any{id}, &sessions},
	}
	for _, q := range queries {
		if err := h.db.QueryRowContext(ctx, q.query, q.args...).Scan(q.dest); err != nil {
			slog.Error("admin deletion impact: count", "err", err, "query", q.query)
			return errorJSON(c, http.StatusInternalServerError, "internal server error")
		}
	}

	return c.JSON(http.StatusOK, adminDeletionImpactResponse{
		UserID:           user.ID,
		Username:         user.Username,
		TotalGames:       games,
		TotalTags:        tags,
		TotalImportJobs:  importJobs,
		TotalExportJobs:  exportJobs,
		TotalSyncJobs:    syncJobs,
		TotalSyncConfigs: syncConfigs,
		TotalSessions:    sessions,
		Warning:          "This action cannot be undone. All data listed above will be permanently deleted.",
	})
}

// HandleDelete removes a user (FK ON DELETE CASCADE wipes all related rows).
func (h *AdminUsersHandler) HandleDelete(c *echo.Context) error {
	id := c.Param("id")
	ctx := context.Background()

	if _, err := h.loadUser(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errorJSON(c, http.StatusNotFound, "user not found")
		}
		slog.Error("admin delete user: load", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	if id == auth.UserIDFromContext(c) {
		return errorJSON(c, http.StatusBadRequest, "Cannot delete your own account")
	}

	if _, err := h.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
		slog.Error("admin delete user: delete", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "internal server error")
	}

	return c.JSON(http.StatusOK, adminSuccessResponse{
		Message: "User and all associated data deleted successfully",
	})
}

// loadUser fetches the full users row for id, returning sql.ErrNoRows when missing.
func (h *AdminUsersHandler) loadUser(ctx context.Context, id string) (*models.User, error) {
	user := &models.User{}
	err := h.db.NewSelect().Model(user).Where("id = ?", id).Scan(ctx)
	return user, err
}
