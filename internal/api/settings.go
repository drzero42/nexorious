package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/dealregion"
)

const defaultDealRegion = "us"

// SettingsHandler handles /api/settings endpoints.
type SettingsHandler struct {
	db *bun.DB
}

// NewSettingsHandler constructs a SettingsHandler.
func NewSettingsHandler(db *bun.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

type settingsResponse struct {
	DealRegion string `json:"deal_region"`
}

// HandleGet handles GET /api/settings, returning defaults when no row exists.
func (h *SettingsHandler) HandleGet(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	var s models.UserSettings
	err := h.db.NewSelect().Model(&s).Where("user_id = ?", userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, settingsResponse{DealRegion: defaultDealRegion})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion})
}

type updateSettingsRequest struct {
	DealRegion *string `json:"deal_region"`
}

// HandlePatch handles PATCH /api/settings, upserting the caller's settings row.
func (h *SettingsHandler) HandlePatch(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	var req updateSettingsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	now := time.Now().UTC()
	s := models.UserSettings{UserID: userID, DealRegion: defaultDealRegion, CreatedAt: now, UpdatedAt: now}
	var existing models.UserSettings
	err := h.db.NewSelect().Model(&existing).Where("user_id = ?", userID).Scan(ctx)
	switch {
	case err == nil:
		s = existing
		s.UpdatedAt = now
	case errors.Is(err, sql.ErrNoRows):
		// keep defaults
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if req.DealRegion != nil {
		if !dealregion.Valid(*req.DealRegion) {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid deal_region")
		}
		s.DealRegion = *req.DealRegion
	}

	_, err = h.db.NewInsert().Model(&s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("deal_region = EXCLUDED.deal_region").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, settingsResponse{DealRegion: s.DealRegion})
}
