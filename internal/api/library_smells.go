package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/librarysmells"
	"github.com/drzero42/nexorious/internal/usergame"
)

// LibrarySmellsHandler serves the /api/library/smells endpoints.
type LibrarySmellsHandler struct {
	db *bun.DB
}

// NewLibrarySmellsHandler returns a new LibrarySmellsHandler.
func NewLibrarySmellsHandler(db *bun.DB) *LibrarySmellsHandler {
	return &LibrarySmellsHandler{db: db}
}

type smellSummaryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	AutoFixable bool   `json:"auto_fixable"`
	Count       int    `json:"count"`
}

type flaggedListResponse struct {
	Items   []librarysmells.FlaggedItem `json:"items"`
	Total   int                         `json:"total"`
	Page    int                         `json:"page"`
	PerPage int                         `json:"per_page"`
	Pages   int                         `json:"pages"`
}

type smellIDsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
}

func smellsUserID(c *echo.Context) (string, error) {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	return userID, nil
}

func paginateSmells[T any](items []T, page, perPage int) ([]T, int) {
	total := len(items)
	start := (page - 1) * perPage
	if start >= total {
		return []T{}, total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return items[start:end], total
}

func parseSmellPageParams(c *echo.Context) (page, perPage int) {
	page, perPage = 1, 25
	if p := c.QueryParam("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 1 {
			page = v
		}
	}
	if pp := c.QueryParam("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v >= 1 && v <= 200 {
			perPage = v
		}
	}
	return page, perPage
}

// HandleSummary: GET /api/library/smells — per-check counts (post-ignore).
func (h *LibrarySmellsHandler) HandleSummary(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	out := make([]smellSummaryItem, 0, len(librarysmells.Registry()))
	for _, check := range librarysmells.Registry() {
		items, err := check.Detect(ctx, h.db, userID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "detect failed")
		}
		out = append(out, smellSummaryItem{
			ID:          check.ID,
			Title:       check.Title,
			Description: check.Description,
			Tier:        string(check.Tier),
			AutoFixable: check.AutoFixable,
			Count:       len(items),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// HandleList: GET /api/library/smells/:checkID — paginated flagged items.
func (h *LibrarySmellsHandler) HandleList(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.Param("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	ctx := c.Request().Context()
	items, err := check.Detect(ctx, h.db, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "detect failed")
	}
	if items == nil {
		items = []librarysmells.FlaggedItem{}
	}
	page, perPage := parseSmellPageParams(c)
	pageItems, total := paginateSmells(items, page, perPage)
	pages := (total + perPage - 1) / perPage
	return c.JSON(http.StatusOK, flaggedListResponse{
		Items: pageItems, Total: total, Page: page, PerPage: perPage, Pages: pages,
	})
}

// HandleApply: POST /api/library/smells/:checkID/apply.
func (h *LibrarySmellsHandler) HandleApply(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.Param("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	if !check.AutoFixable || check.Apply == nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "check is not auto-fixable")
	}
	var req smellIDsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	applied, skipped, err := check.Apply(c.Request().Context(), h.db, userID, req.UserGameIDs)
	if err != nil {
		if errors.Is(err, usergame.ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "apply failed")
	}
	return c.JSON(http.StatusOK, map[string]int{"applied": applied, "skipped": skipped})
}

// HandleIgnore: POST /api/library/smells/:checkID/ignore.
func (h *LibrarySmellsHandler) HandleIgnore(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.Param("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var req smellIDsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	ctx := c.Request().Context()
	var ignored int
	for _, ugID := range req.UserGameIDs {
		// Insert only if the game belongs to the user; idempotent on conflict.
		res, err := h.db.NewRaw(
			`INSERT INTO smell_ignores (id, user_id, user_game_id, check_id)
			 SELECT ?, ?, ug.id, ?
			 FROM user_games ug WHERE ug.id = ? AND ug.user_id = ?
			 ON CONFLICT (user_id, user_game_id, check_id) DO NOTHING`,
			uuid.NewString(), userID, check.ID, ugID, userID,
		).Exec(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "ignore failed")
		}
		if n, _ := res.RowsAffected(); n > 0 { //nolint:errcheck // RowsAffected advisory; count only
			ignored++
		}
	}
	return c.JSON(http.StatusOK, map[string]int{"ignored": ignored})
}

// HandleRestore: DELETE /api/library/smells/:checkID/ignore.
func (h *LibrarySmellsHandler) HandleRestore(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.Param("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var req smellIDsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	res, err := h.db.NewRaw(
		`DELETE FROM smell_ignores
		 WHERE user_id = ? AND check_id = ? AND user_game_id IN (?)`,
		userID, check.ID, bun.List(req.UserGameIDs),
	).Exec(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "restore failed")
	}
	n, _ := res.RowsAffected() //nolint:errcheck // RowsAffected advisory; count only
	return c.JSON(http.StatusOK, map[string]int{"restored": int(n)})
}

type smellIgnoredItem struct {
	UserGameID string `bun:"user_game_id" json:"user_game_id"`
	Title      string `bun:"title"        json:"title"`
	CreatedAt  string `bun:"created_at"   json:"created_at"`
}

type ignoredListResponse struct {
	Items   []smellIgnoredItem `json:"items"`
	Total   int                `json:"total"`
	Page    int                `json:"page"`
	PerPage int                `json:"per_page"`
	Pages   int                `json:"pages"`
}

// HandleListIgnored: GET /api/library/smells/:checkID/ignored.
func (h *LibrarySmellsHandler) HandleListIgnored(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.Param("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var items []smellIgnoredItem
	err = h.db.NewRaw(
		`SELECT si.user_game_id, g.title, si.created_at::text AS created_at
		 FROM smell_ignores si
		 JOIN user_games ug ON ug.id = si.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE si.user_id = ? AND si.check_id = ?
		 ORDER BY si.created_at DESC`,
		userID, check.ID,
	).Scan(c.Request().Context(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list dismissed failed")
	}
	if items == nil {
		items = []smellIgnoredItem{}
	}
	page, perPage := parseSmellPageParams(c)
	pageItems, total := paginateSmells(items, page, perPage)
	pages := (total + perPage - 1) / perPage
	return c.JSON(http.StatusOK, ignoredListResponse{
		Items: pageItems, Total: total, Page: page, PerPage: perPage, Pages: pages,
	})
}
