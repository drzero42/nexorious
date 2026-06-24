package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/changelog"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
)

// ChangelogHandler serves the embedded release changelog, sliced per user, and
// owns the user's last_seen_changelog_version marker.
type ChangelogHandler struct {
	db      *bun.DB
	version string
	// Source returns the parsed changelog and whether it is available. It
	// defaults to changelog.All and is overridable in tests.
	Source func() ([]changelog.Entry, bool)
}

// NewChangelogHandler constructs a ChangelogHandler. version is the running
// binary's version (the build-time ldflag), used as the "current" baseline.
func NewChangelogHandler(db *bun.DB, version string) *ChangelogHandler {
	return &ChangelogHandler{db: db, version: version, Source: changelog.All}
}

type changelogResponse struct {
	Available bool              `json:"available"`
	Current   string            `json:"current"`
	LastSeen  string            `json:"last_seen,omitempty"`
	Markdown  string            `json:"markdown"`
	Entries   []changelog.Entry `json:"entries"`
}

type unseenResponse struct {
	HasUnseen bool `json:"has_unseen"`
}

// HandleGet serves GET /api/changelog. Modes: default (since-last, auto-marks
// seen), ?range=all (full, auto-marks seen), ?since=X.Y.Z (pure read).
func (h *ChangelogHandler) HandleGet(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	entries, ok := h.Source()
	if !ok {
		return c.JSON(http.StatusOK, changelogResponse{Available: false, Current: h.version, Entries: []changelog.Entry{}})
	}

	lastSeen, err := h.readLastSeen(ctx, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	var slice []changelog.Entry
	switch {
	case c.QueryParam("since") != "":
		// Pure read against an arbitrary version; invalid/unknown -> empty.
		slice = changelog.Newer(entries, c.QueryParam("since"))
	case c.QueryParam("range") == "all":
		slice = entries
		h.advanceSeen(ctx, userID, lastSeen)
	default:
		// since-last: empty (not full history) when no baseline captured yet.
		slice = changelog.Newer(entries, lastSeen)
		h.advanceSeen(ctx, userID, lastSeen)
	}

	if slice == nil {
		slice = []changelog.Entry{}
	}

	return c.JSON(http.StatusOK, changelogResponse{
		Available: true,
		Current:   h.version,
		LastSeen:  lastSeen,
		Markdown:  changelog.Render(slice),
		Entries:   slice,
	})
}

// HandleUnseen serves GET /api/changelog/unseen — the cheap "is there anything
// new?" signal for the web dot. It captures the baseline (last_seen = current)
// the first time it sees a null marker, and never advances past a real new
// release (only HandleGet marks releases seen).
func (h *ChangelogHandler) HandleUnseen(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	if !updatecheck.IsValidVersion(h.version) {
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}
	if _, ok := h.Source(); !ok {
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}

	lastSeen, err := h.readLastSeen(ctx, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if lastSeen == "" {
		// First authenticated encounter: capture the baseline so the user only
		// sees the indicator on the NEXT release, never a full-history blast.
		if err := h.setSeen(ctx, userID, h.version); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		return c.JSON(http.StatusOK, unseenResponse{HasUnseen: false})
	}
	return c.JSON(http.StatusOK, unseenResponse{HasUnseen: updatecheck.Compare(h.version, lastSeen) > 0})
}

func (h *ChangelogHandler) readLastSeen(ctx context.Context, userID string) (string, error) {
	var s models.UserSettings
	err := h.db.NewSelect().Model(&s).Where("user_id = ?", userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if s.LastSeenChangelogVersion == nil {
		return "", nil
	}
	return *s.LastSeenChangelogVersion, nil
}

// advanceSeen marks the running version as seen, but only moves the marker
// forward and only for valid release versions (dev builds never mark).
func (h *ChangelogHandler) advanceSeen(ctx context.Context, userID, lastSeen string) {
	if !updatecheck.IsValidVersion(h.version) {
		return
	}
	if lastSeen != "" && updatecheck.Compare(h.version, lastSeen) <= 0 {
		return
	}
	if err := h.setSeen(ctx, userID, h.version); err != nil {
		slog.ErrorContext(ctx, "advance changelog last-seen", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
	}
}

// setSeen upserts the caller's last_seen_changelog_version, preserving any
// existing deal_region (a new row gets the deal_region default).
func (h *ChangelogHandler) setSeen(ctx context.Context, userID, version string) error {
	now := time.Now().UTC()
	s := models.UserSettings{
		UserID:                   userID,
		DealRegion:               defaultDealRegion,
		DateFormat:               defaultDateFormat,
		Theme:                    defaultTheme,
		LastSeenChangelogVersion: &version,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	_, err := h.db.NewInsert().Model(&s).
		On("CONFLICT (user_id) DO UPDATE").
		Set("last_seen_changelog_version = EXCLUDED.last_seen_changelog_version").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
