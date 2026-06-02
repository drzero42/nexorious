package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"
)

// AdminResetHandler exposes POST /api/auth/admin/reset.
type AdminResetHandler struct {
	db *bun.DB
}

// NewAdminResetHandler constructs an AdminResetHandler.
func NewAdminResetHandler(db *bun.DB) *AdminResetHandler {
	return &AdminResetHandler{db: db}
}

// HandleReset handles POST /api/auth/admin/reset.
// Truncates all user data (library, jobs, sync configs, tags, non-admin users)
// while preserving the admin account, catalog tables, and backup config.
func (h *AdminResetHandler) HandleReset(c *echo.Context) error {
	ctx := context.Background()
	var deleted int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Cancel all active River jobs.
		if _, err := tx.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')`).
			Exec(ctx); err != nil {
			return err
		}
		// Delete all user games (cascades user_game_platforms + user_game_tags).
		res, err := tx.NewRaw("DELETE FROM user_games").Exec(ctx)
		if err != nil {
			return err
		}
		var rowsErr error
		deleted, rowsErr = res.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		// Delete all jobs (cascades job_items + changes).
		if _, err := tx.NewRaw("DELETE FROM jobs").Exec(ctx); err != nil {
			return err
		}
		// Delete all sync configs.
		if _, err := tx.NewRaw("DELETE FROM user_sync_configs").Exec(ctx); err != nil {
			return err
		}
		// Delete all tags.
		if _, err := tx.NewRaw("DELETE FROM tags").Exec(ctx); err != nil {
			return err
		}
		// Delete all external games (cascades external_game_platforms).
		// Non-admin users' rows cascade from users, but the admin's rows must be
		// deleted explicitly since the admin account is preserved.
		if _, err := tx.NewRaw("DELETE FROM external_games").Exec(ctx); err != nil {
			return err
		}
		// Delete non-admin users (cascades user_sessions + api_keys).
		if _, err := tx.NewRaw("DELETE FROM users WHERE NOT is_admin").Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("admin reset: transaction failed", "err", err)
		return errorJSON(c, http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, map[string]any{"deleted": deleted})
}
