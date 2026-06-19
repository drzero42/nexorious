package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/filter"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/usergame"
)

// UserGamesHandler handles /api/user-games endpoints.
type UserGamesHandler struct {
	db  *bun.DB
	cfg *config.Config
}

// NewUserGamesHandler creates a UserGamesHandler.
func NewUserGamesHandler(db *bun.DB, cfg *config.Config) *UserGamesHandler {
	return &UserGamesHandler{db: db, cfg: cfg}
}

// createUserGameRequest is the body for POST /api/user-games.
type createUserGameRequest struct {
	GameID         int32             `json:"game_id"`
	PlayStatus     *string           `json:"play_status"`
	PersonalRating *int32            `json:"personal_rating"`
	IsLoved        bool              `json:"is_loved"`
	PersonalNotes  *string           `json:"personal_notes"`
	IsWishlisted   bool              `json:"is_wishlisted"`
	Platforms      []platformRequest `json:"platforms"`
}

// userGamePlatformResponse is the API DTO for a user game platform entry with nested detail objects.
type userGamePlatformResponse struct {
	ID                string              `json:"id"`
	UserGameID        string              `json:"user_game_id"`
	Platform          *string             `json:"platform"`
	Storefront        *string             `json:"storefront"`
	IsAvailable       bool                `json:"is_available"`
	HoursPlayed       *float64            `json:"hours_played"`
	OwnershipStatus   *string             `json:"ownership_status"`
	AcquiredDate      *time.Time          `json:"acquired_date"`
	ExternalGameID    *string             `json:"external_game_id"`
	SyncFromSource    bool                `json:"sync_from_source"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	PlatformDetails   *platformResponse   `json:"platform_details,omitempty"`
	StorefrontDetails *storefrontResponse `json:"storefront_details,omitempty"`
	StoreURL          *string             `json:"store_url,omitempty"`
}

func toUserGamePlatformResponse(ugp models.UserGamePlatform) userGamePlatformResponse {
	resp := userGamePlatformResponse{
		ID:              ugp.ID,
		UserGameID:      ugp.UserGameID,
		Platform:        ugp.Platform,
		Storefront:      ugp.Storefront,
		IsAvailable:     ugp.IsAvailable,
		HoursPlayed:     ugp.HoursPlayed,
		OwnershipStatus: ugp.OwnershipStatus,
		AcquiredDate:    ugp.AcquiredDate,
		ExternalGameID:  ugp.ExternalGameID,
		SyncFromSource:  ugp.SyncFromSource,
		CreatedAt:       ugp.CreatedAt,
		UpdatedAt:       ugp.UpdatedAt,
	}
	if ugp.PlatformRecord != nil {
		pr := toPlatformResponse(*ugp.PlatformRecord)
		resp.PlatformDetails = &pr
	}
	if ugp.StorefrontRecord != nil {
		sr := toStorefrontResponse(*ugp.StorefrontRecord)
		resp.StorefrontDetails = &sr
	}
	if ugp.ExternalGame != nil && ugp.ExternalGame.StoreLink != nil {
		if url, ok := buildStoreURL(ugp.ExternalGame.Storefront, *ugp.ExternalGame.StoreLink); ok {
			resp.StoreURL = &url
		}
	}
	return resp
}

// userGameWithPlatformsResponse wraps UserGame but serialises Platforms as DTOs with
// nested details and exposes a calculated game-level HoursPlayed (sum of platform hours).
type userGameWithPlatformsResponse struct {
	models.UserGame
	HoursPlayed    float64                    `json:"hours_played"`
	Platforms      []userGamePlatformResponse `json:"platforms"`
	Tags           []tagResponse              `json:"tags"`
	PoolMembership *string                    `json:"pool_membership,omitempty"`
}

func toUserGameWithPlatformsResponse(ug models.UserGame) userGameWithPlatformsResponse {
	resp := userGameWithPlatformsResponse{UserGame: ug}
	var totalHours float64
	for _, p := range ug.Platforms {
		if p.HoursPlayed != nil {
			totalHours += *p.HoursPlayed
		}
		resp.Platforms = append(resp.Platforms, toUserGamePlatformResponse(p))
	}
	resp.HoursPlayed = totalHours
	if resp.Platforms == nil {
		resp.Platforms = []userGamePlatformResponse{}
	}
	// Flatten the user_game_tags join rows into their nested Tag DTOs so the
	// client receives a plain []tagResponse rather than join-table internals.
	resp.Tags = make([]tagResponse, 0, len(ug.Tags))
	for _, link := range ug.Tags {
		if link.Tag != nil {
			resp.Tags = append(resp.Tags, toTagResponse(*link.Tag))
		}
	}
	return resp
}

// UserGameListResponse is the paginated response for GET /api/user-games.
type UserGameListResponse struct {
	UserGames []userGameWithPlatformsResponse `json:"user_games"`
	Total     int                             `json:"total"`
	Page      int                             `json:"page"`
	PerPage   int                             `json:"per_page"`
	Pages     int                             `json:"pages"`
}

var allowedUserGameSortFields = map[string]string{
	"title":           "g.title",
	"created_at":      "ug.created_at",
	"updated_at":      "ug.updated_at",
	"play_status":     "ug.play_status",
	"personal_rating": "ug.personal_rating",
	"is_loved":        "ug.is_loved",
	"release_date":    "g.release_date",
	// hours_played sorts on the joined aggregate alias `hp`; COALESCE so games with no
	// platforms (LEFT JOIN → NULL) sort as 0 instead of NULL-first under DESC.
	"hours_played":       "COALESCE(hp.total, 0)",
	"howlongtobeat_main": "g.howlongtobeat_main",
	"rating_average":     "g.rating_average",
}

var sortFieldsRequiringGamesJoin = map[string]bool{
	"title":              true,
	"release_date":       true,
	"howlongtobeat_main": true,
	"rating_average":     true,
}

var sortFieldsRequiringHoursJoin = map[string]bool{
	"hours_played": true,
}

// sortFieldsNullsLast lists sort fields whose ORDER BY clause should append
// "NULLS LAST", so games without IGDB data (NULL) sink to the bottom regardless
// of sort direction. release_date is intentionally NOT in this set — changing
// its NULL ordering would be a user-visible behavior change beyond the scope
// of issue #639.
var sortFieldsNullsLast = map[string]bool{
	"howlongtobeat_main": true,
	"rating_average":     true,
}

// HandleListUserGames handles GET /api/user-games.
func (h *UserGamesHandler) HandleListUserGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Parse pagination.
	page := 1
	perPage := 25
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

	// Parse sort.
	sortBy := c.QueryParam("sort_by")
	sortOrder := c.QueryParam("sort_order")
	var sortCol string
	if sortBy != "" {
		col, ok := allowedUserGameSortFields[sortBy]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid sort_by field")
		}
		sortCol = col
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}
	// Compose the ORDER BY expression once so both phases of the two-phase
	// list query stay in sync. NULLS LAST is opt-in per field.
	var orderExpr string
	if sortCol != "" {
		orderExpr = sortCol + " " + sortOrder
		if sortFieldsNullsLast[sortBy] {
			orderExpr += " NULLS LAST"
		}
	}

	// Build filter. With ?pool=:id the pool's saved filter drives the query
	// (owned + wishlist), AND NOT finished; ad-hoc facet params are not merged
	// in v1 (sort + pagination are still honoured).
	poolID := c.QueryParam("pool")
	fb := filter.NewFilterBuilder()

	if poolID != "" {
		isManual, err := h.applyPoolFilter(context.Background(), fb, poolID, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		if isManual {
			// Pure manual pool — no suggestions.
			return c.JSON(http.StatusOK, UserGameListResponse{
				UserGames: []userGameWithPlatformsResponse{},
				Total:     0, Page: page, PerPage: perPage, Pages: 1,
			})
		}
	} else {
		applyUserGameFacetFilters(fb, c)
	}

	// If sort field needs games join, add it.
	if sortBy != "" && sortFieldsRequiringGamesJoin[sortBy] {
		fb.AddJoin("g", "LEFT JOIN games AS g ON g.id = ug.game_id")
	}
	// If sort field needs the aggregated platform-hours join, add it.
	if sortBy != "" && sortFieldsRequiringHoursJoin[sortBy] {
		fb.AddJoin("hp", "LEFT JOIN (SELECT user_game_id, COALESCE(SUM(hours_played), 0) AS total FROM user_game_platforms GROUP BY user_game_id) hp ON hp.user_game_id = ug.id")
	}

	ctx := context.Background()

	// Count query.
	var total int
	countQ := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("COUNT(DISTINCT ug.id)").
		Where("ug.user_id = ?", userID)
	countQ = fb.Apply(countQ)
	if err := countQ.Scan(ctx, &total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	pages := int(math.Ceil(float64(total) / float64(perPage)))
	pages = max(pages, 1)

	// Short-circuit: no results.
	if total == 0 {
		return c.JSON(http.StatusOK, UserGameListResponse{
			UserGames: []userGameWithPlatformsResponse{},
			Total:     0,
			Page:      page,
			PerPage:   perPage,
			Pages:     pages,
		})
	}

	// ID query with pagination and sort.
	colExpr := "DISTINCT ug.id, ug.created_at"
	if sortCol != "" {
		colExpr = "DISTINCT ug.id, ug.created_at, " + sortCol
	}
	idQ := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr(colExpr).
		Where("ug.user_id = ?", userID)
	idQ = fb.Apply(idQ)
	if orderExpr != "" {
		idQ = idQ.OrderExpr(orderExpr)
	}
	// stable secondary sort
	idQ = idQ.OrderExpr("ug.created_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage)

	// Wrap in subquery to get only IDs.
	var ids []string
	if err := h.db.NewSelect().
		TableExpr("(?) AS sub", idQ).
		ColumnExpr("id").
		Scan(ctx, &ids); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if len(ids) == 0 {
		return c.JSON(http.StatusOK, UserGameListResponse{
			UserGames: []userGameWithPlatformsResponse{},
			Total:     total,
			Page:      page,
			PerPage:   perPage,
			Pages:     pages,
		})
	}

	// Fetch full records with relations, preserving sort order.
	var userGames []models.UserGame
	q := withUserGameRelations(h.db.NewSelect().
		Model(&userGames).
		Where("user_game.id IN (?)", bun.List(ids)))

	// Re-apply sort on the Model query.
	if sortCol != "" {
		// For game-table sorts, join games again on the model query.
		if sortFieldsRequiringGamesJoin[sortBy] {
			q = q.Join("LEFT JOIN games AS g ON g.id = user_game.game_id")
		}
		if sortFieldsRequiringHoursJoin[sortBy] {
			q = q.Join("LEFT JOIN (SELECT user_game_id, COALESCE(SUM(hours_played), 0) AS total FROM user_game_platforms GROUP BY user_game_id) hp ON hp.user_game_id = user_game.id")
		}
		q = q.OrderExpr(orderExpr)
	}
	q = q.OrderExpr("user_game.created_at DESC")

	if err := q.Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	dtos := make([]userGameWithPlatformsResponse, len(userGames))
	for i, ug := range userGames {
		dtos[i] = toUserGameWithPlatformsResponse(ug)
	}

	if poolID != "" && len(dtos) > 0 {
		pageIDs := make([]string, len(dtos))
		for i := range dtos {
			pageIDs[i] = dtos[i].ID
		}
		var members []poolMember
		if err := h.db.NewRaw(
			`SELECT user_game_id, position FROM pool_games WHERE pool_id = ? AND user_game_id IN (?)`,
			poolID, bun.List(pageIDs),
		).Scan(ctx, &members); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		membership := make(map[string]string, len(members))
		for _, m := range members {
			if m.Position != nil {
				membership[m.UserGameID] = "queued"
			} else {
				membership[m.UserGameID] = "candidate"
			}
		}
		for i := range dtos {
			if state, ok := membership[dtos[i].ID]; ok {
				s := state
				dtos[i].PoolMembership = &s
			}
		}
	}

	return c.JSON(http.StatusOK, UserGameListResponse{
		UserGames: dtos,
		Total:     total,
		Page:      page,
		PerPage:   perPage,
		Pages:     pages,
	})
}

// httpError maps a usergame operation error to an echo HTTP error, preserving
// the existing status codes. Unmapped errors become 500 and are logged.
func (h *UserGamesHandler) httpError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, usergame.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	case errors.Is(err, usergame.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, strings.TrimSuffix(err.Error(), ": conflict"))
	case errors.Is(err, usergame.ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		slog.ErrorContext(c.Request().Context(), "user_games: operation failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
}

// HandleCreateUserGame handles POST /api/user-games.
func (h *UserGamesHandler) HandleCreateUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req createUserGameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.GameID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "game_id is required")
	}

	if req.IsWishlisted && len(req.Platforms) > 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "a wishlisted game cannot have platforms")
	}

	// Validate game exists.
	ctx := context.Background()
	gameExists, err := h.db.NewSelect().Model((*models.Game)(nil)).
		Where("id = ?", req.GameID).
		Exists(ctx)
	if err != nil || !gameExists {
		return echo.NewHTTPError(http.StatusBadRequest, "game not found")
	}

	// Validate play_status if provided.
	if req.PlayStatus != nil && *req.PlayStatus != "" {
		if !enum.PlayStatus(*req.PlayStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}

	// Map request platforms → usergame.PlatformInput, validating each entry.
	plats := make([]usergame.PlatformInput, 0, len(req.Platforms))
	for _, p := range req.Platforms {
		if p.OwnershipStatus != nil && *p.OwnershipStatus != "" && !enum.OwnershipStatus(*p.OwnershipStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ownership_status: "+*p.OwnershipStatus)
		}
		var acquired *time.Time
		if p.AcquiredDate != nil && *p.AcquiredDate != "" {
			a, err := parseAcquiredDate(*p.AcquiredDate)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			acquired = a
		}
		plats = append(plats, usergame.PlatformInput{
			Platform: p.Platform, Storefront: p.Storefront, HoursPlayed: p.HoursPlayed,
			OwnershipStatus: p.OwnershipStatus, IsAvailable: p.IsAvailable, AcquiredDate: acquired,
		})
	}

	res, err := usergame.Acquire(ctx, h.db, usergame.AcquireParams{
		UserID:         userID,
		GameID:         req.GameID,
		Mode:           usergame.ModeCreate,
		Platforms:      plats,
		PlayStatus:     req.PlayStatus,
		PersonalRating: req.PersonalRating,
		IsLoved:        req.IsLoved,
		PersonalNotes:  req.PersonalNotes,
		IsWishlisted:   req.IsWishlisted,
	})
	if err != nil {
		return h.httpError(c, err)
	}

	ug, err := LoadUserGameDetail(ctx, h.db, res.UserGameID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusCreated, toUserGameWithPlatformsResponse(*ug))
}

// HandleGetUserGame handles GET /api/user-games/:id.
func (h *UserGamesHandler) HandleGetUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")
	ctx := context.Background()

	ug, err := LoadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
}

// allowedUpdateFields is the set of mutable fields on a user game.
var allowedUpdateFields = map[string]bool{
	"play_status":     true,
	"personal_rating": true,
	"is_loved":        true,
	"personal_notes":  true,
}

// immutableUpdateFields are fields that must never be changed via PUT.
var immutableUpdateFields = map[string]bool{
	"game_id": true,
	"user_id": true,
}

// HandleUpdateUserGame handles PUT /api/user-games/:id.
func (h *UserGamesHandler) HandleUpdateUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")

	// Decode into map to support partial updates (absent vs null distinction).
	var body map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Reject immutable fields and unknown fields.
	for k := range body {
		if immutableUpdateFields[k] {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("field %q cannot be updated", k))
		}
		if !allowedUpdateFields[k] {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("unknown field %q", k))
		}
	}

	if len(body) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no fields to update")
	}

	// Field-level validations.
	if v, ok := body["play_status"]; ok && v != nil {
		s, ok2 := v.(string)
		if !ok2 || !enum.PlayStatus(s).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}
	if v, ok := body["personal_rating"]; ok && v != nil {
		f, ok2 := v.(float64)
		if !ok2 || f < 1 || f > 5 {
			return echo.NewHTTPError(http.StatusBadRequest, "personal_rating must be between 1 and 5")
		}
	}

	ctx := context.Background()

	// Map allowed JSON keys onto UpdateFieldsParams.
	p := usergame.UpdateFieldsParams{UserID: userID, UserGameID: id}
	if v, ok := body["play_status"]; ok {
		if v == nil {
			s := ""
			p.PlayStatus = &s
		} else {
			s := v.(string)
			p.PlayStatus = &s
		}
	}
	if v, ok := body["personal_notes"]; ok {
		if v == nil {
			p.ClearPersonalNotes = true
		} else {
			s := v.(string)
			p.PersonalNotes = &s
		}
	}
	if v, ok := body["personal_rating"]; ok {
		if v == nil {
			p.ClearPersonalRating = true
		} else {
			r := int32(v.(float64))
			p.PersonalRating = &r
		}
	}
	if v, ok := body["is_loved"]; ok {
		if v == nil {
			b := false
			p.IsLoved = &b
		} else {
			b := v.(bool)
			p.IsLoved = &b
		}
	}

	if err := usergame.UpdateFields(ctx, h.db, p); err != nil {
		return h.httpError(c, err)
	}

	ug, err := LoadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
}

// HandleDeleteUserGame handles DELETE /api/user-games/:id.
func (h *UserGamesHandler) HandleDeleteUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")
	ctx := context.Background()

	if err := usergame.Delete(ctx, h.db, usergame.DeleteParams{UserID: userID, UserGameID: id}); err != nil {
		return h.httpError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// replaceTagsRequest is the body for PUT /api/user-games/:id/tags. The tags
// slice is the complete desired set of tag names.
type replaceTagsRequest struct {
	Tags []string `json:"tags"`
}

// HandleReplaceTags handles PUT /api/user-games/:id/tags. It validates the
// supplied tag names, then within one transaction verifies ownership of the
// user game and reconciles its tag set via usergame.ReplaceTags (resolving or
// creating each name within the caller's own tags). An empty or absent "tags"
// clears all tags. Returns the updated user game with its Tags relation.
func (h *UserGamesHandler) HandleReplaceTags(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")

	var req replaceTagsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	names := make([]string, 0, len(req.Tags))
	for _, raw := range req.Tags {
		name := strings.TrimSpace(raw)
		if name == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "tag name cannot be empty")
		}
		if len(name) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "tag name must be 100 characters or less")
		}
		names = append(names, name)
	}

	ctx := context.Background()

	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		exists, existsErr := tx.NewSelect().Model((*models.UserGame)(nil)).
			Where("id = ? AND user_id = ?", id, userID).Exists(ctx)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return usergame.ErrNotFound
		}
		return usergame.ReplaceTags(ctx, tx, id, userID, names)
	})
	if err != nil {
		return h.httpError(c, err)
	}

	ug, err := LoadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
}

type updateProgressRequest struct {
	PlayStatus *string `json:"play_status"`
}

func (h *UserGamesHandler) HandleUpdateProgress(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")
	var req updateProgressRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.PlayStatus == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "play_status is required")
	}

	if !enum.PlayStatus(*req.PlayStatus).Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
	}

	ctx := context.Background()
	if err := usergame.RecordProgress(ctx, h.db, usergame.ProgressParams{
		UserID:     userID,
		UserGameID: id,
		PlayStatus: *req.PlayStatus,
	}); err != nil {
		return h.httpError(c, err)
	}

	ug, err := LoadUserGameDetail(ctx, h.db, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
}

type bulkUpdateRequest struct {
	IDs     []string       `json:"ids"`
	Updates map[string]any `json:"updates"`
}

func (h *UserGamesHandler) HandleBulkUpdate(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req bulkUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids must not be empty")
	}
	if len(req.Updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "updates must not be empty")
	}

	ps, ok := req.Updates["play_status"]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "updates must include play_status")
	}
	psStr, ok := ps.(string)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "play_status must be a string")
	}

	ctx := context.Background()
	updated, err := usergame.SetPlayStatusBulk(ctx, h.db, usergame.BulkStatusParams{
		UserID:      userID,
		UserGameIDs: req.IDs,
		PlayStatus:  psStr,
	})
	if err != nil {
		return h.httpError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"updated": updated})
}

type bulkDeleteRequest struct {
	IDs []string `json:"ids"`
}

func (h *UserGamesHandler) HandleBulkDelete(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req bulkDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids must not be empty")
	}

	ctx := context.Background()
	deleted, err := usergame.DeleteBulk(ctx, h.db, usergame.BulkDeleteParams{UserID: userID, UserGameIDs: req.IDs})
	if err != nil {
		return h.httpError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"deleted": deleted})
}

type bulkAddPlatformsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
	Platform    string   `json:"platform"`
	Storefront  string   `json:"storefront"`
}

func (h *UserGamesHandler) HandleBulkAddPlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkAddPlatformsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids is required")
	}

	ctx := c.Request().Context()
	added, err := usergame.AddPlatformBulk(ctx, h.db, usergame.BulkAddPlatformParams{
		UserID:      userID,
		UserGameIDs: req.UserGameIDs,
		Platform: usergame.PlatformInput{
			Platform:   &req.Platform,
			Storefront: &req.Storefront,
		},
	})
	if err != nil {
		return h.httpError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]int{"added": added})
}

type bulkRemovePlatformsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
	Platform    string   `json:"platform"`
	Storefront  string   `json:"storefront"`
}

func (h *UserGamesHandler) HandleBulkRemovePlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkRemovePlatformsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids is required")
	}

	ctx := context.Background()
	removed, err := usergame.RemovePlatformBulk(ctx, h.db, usergame.BulkRemovePlatformParams{
		UserID:      userID,
		UserGameIDs: req.UserGameIDs,
		Platform:    req.Platform,
		Storefront:  req.Storefront,
	})
	if err != nil {
		return h.httpError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"removed": removed})
}

// verifyUserGameOwnership checks that userGameID belongs to userID.
// Returns sql.ErrNoRows if not found or not owned.
func (h *UserGamesHandler) verifyUserGameOwnership(ctx context.Context, userGameID, userID string) error {
	exists, err := h.db.NewSelect().Model((*models.UserGame)(nil)).
		Where("id = ?", userGameID).
		Where("user_id = ?", userID).
		Exists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	return nil
}

// platformRequest is the bind target for platform create/update.
type platformRequest struct {
	Platform        *string  `json:"platform"`
	Storefront      *string  `json:"storefront"`
	IsAvailable     *bool    `json:"is_available"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
}

// parseAcquiredDate parses the request's acquired_date string into a time. It
// accepts the date-only form sent by the edit form (YYYY-MM-DD) as well as
// RFC3339, mirroring the import/export paths. A malformed value is an error.
func parseAcquiredDate(s string) (*time.Time, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("invalid acquired_date %q (want YYYY-MM-DD)", s)
}

// HandleListPlatforms handles GET /api/user-games/:id/platforms.
func (h *UserGamesHandler) HandleListPlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	userGameID := c.Param("id")
	ctx := c.Request().Context()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user game not found"})
	}

	var platforms []models.UserGamePlatform
	err := h.db.NewSelect().Model(&platforms).
		Where("user_game_id = ?", userGameID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	dtos := make([]userGamePlatformResponse, len(platforms))
	for i, p := range platforms {
		dtos[i] = toUserGamePlatformResponse(p)
	}
	return c.JSON(http.StatusOK, dtos)
}

// HandleCreatePlatform handles POST /api/user-games/:id/platforms.
func (h *UserGamesHandler) HandleCreatePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	userGameID := c.Param("id")
	ctx := c.Request().Context()

	var req platformRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate ownership_status if provided
	if req.OwnershipStatus != nil && *req.OwnershipStatus != "" {
		if !enum.OwnershipStatus(*req.OwnershipStatus).Valid() {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid ownership_status: " + *req.OwnershipStatus})
		}
	}

	// Validate platform exists
	if req.Platform != nil && *req.Platform != "" {
		exists, err := h.db.NewSelect().Model((*models.Platform)(nil)).
			Where("name = ?", *req.Platform).
			Exists(ctx)
		if err != nil || !exists {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "platform not found: " + *req.Platform})
		}
	}

	// Validate storefront exists
	if req.Storefront != nil && *req.Storefront != "" {
		exists, err := h.db.NewSelect().Model((*models.Storefront)(nil)).
			Where("name = ?", *req.Storefront).
			Exists(ctx)
		if err != nil || !exists {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "storefront not found: " + *req.Storefront})
		}
	}

	var acquiredDate *time.Time
	if req.AcquiredDate != nil && *req.AcquiredDate != "" {
		acquired, err := parseAcquiredDate(*req.AcquiredDate)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		acquiredDate = acquired
	}

	platIn := usergame.PlatformInput{
		Platform:        req.Platform,
		Storefront:      req.Storefront,
		HoursPlayed:     req.HoursPlayed,
		OwnershipStatus: req.OwnershipStatus,
		IsAvailable:     req.IsAvailable,
		AcquiredDate:    acquiredDate,
	}

	res, err := usergame.AddPlatform(ctx, h.db, usergame.AddPlatformParams{
		UserID:     userID,
		UserGameID: userGameID,
		Platform:   platIn,
	})
	if err != nil {
		return h.httpError(c, err)
	}

	var plat models.UserGamePlatform
	if err := h.db.NewSelect().Model(&plat).
		Where("id = ?", res.PlatformID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "user_games: load platform relations failed", logging.KeyErr, err, "platform_id", res.PlatformID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load platform")
	}
	return c.JSON(http.StatusCreated, toUserGamePlatformResponse(plat))
}

// HandleUpdatePlatform handles PUT /api/user-games/:id/platforms/:platform_id.
func (h *UserGamesHandler) HandleUpdatePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	userGameID := c.Param("id")
	platformID := c.Param("platform_id")
	ctx := c.Request().Context()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user game not found"})
	}

	var plat models.UserGamePlatform
	err := h.db.NewSelect().Model(&plat).
		Where("id = ?", platformID).
		Where("user_game_id = ?", userGameID).
		Scan(ctx)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "platform not found"})
	}

	var req platformRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate ownership_status if provided
	if req.OwnershipStatus != nil && *req.OwnershipStatus != "" {
		if !enum.OwnershipStatus(*req.OwnershipStatus).Valid() {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid ownership_status: " + *req.OwnershipStatus})
		}
	}

	// Validate platform if provided
	if req.Platform != nil && *req.Platform != "" {
		exists, checkErr := h.db.NewSelect().Model((*models.Platform)(nil)).
			Where("name = ?", *req.Platform).
			Exists(ctx)
		if checkErr != nil || !exists {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "platform not found: " + *req.Platform})
		}
	}

	// Validate storefront if provided
	if req.Storefront != nil && *req.Storefront != "" {
		exists, checkErr := h.db.NewSelect().Model((*models.Storefront)(nil)).
			Where("name = ?", *req.Storefront).
			Exists(ctx)
		if checkErr != nil || !exists {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "storefront not found: " + *req.Storefront})
		}
	}

	// AcquiredDate is three-way: omitted (nil) leaves it unchanged, an empty
	// string clears it to NULL, a valid date sets it.
	var acquiredDate *time.Time
	acquiredDateProvided := false
	if req.AcquiredDate != nil {
		acquiredDateProvided = true
		if *req.AcquiredDate != "" {
			acquired, err := parseAcquiredDate(*req.AcquiredDate)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			acquiredDate = acquired
		}
		// empty string → acquiredDate stays nil → stored as NULL
	}

	fields := usergame.PlatformInput{
		Platform:        req.Platform,
		Storefront:      req.Storefront,
		IsAvailable:     req.IsAvailable,
		HoursPlayed:     req.HoursPlayed,
		OwnershipStatus: req.OwnershipStatus,
	}
	if acquiredDateProvided {
		if acquiredDate == nil {
			fields.ClearAcquiredDate = true
		} else {
			fields.AcquiredDate = acquiredDate
		}
	}

	if err := usergame.UpdatePlatform(ctx, h.db, usergame.UpdatePlatformParams{
		UserID:     userID,
		UserGameID: userGameID,
		PlatformID: platformID,
		Fields:     fields,
	}); err != nil {
		return h.httpError(c, err)
	}

	if err := h.db.NewSelect().Model(&plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.ErrorContext(ctx, "user_games: load platform relations failed", logging.KeyErr, err, "platform_id", plat.ID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load platform")
	}
	return c.JSON(http.StatusOK, toUserGamePlatformResponse(plat))
}

// HandleDeletePlatform handles DELETE /api/user-games/:id/platforms/:platform_id.
func (h *UserGamesHandler) HandleDeletePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	userGameID := c.Param("id")
	platformID := c.Param("platform_id")
	ctx := c.Request().Context()

	if err := usergame.RemovePlatform(ctx, h.db, usergame.RemovePlatformParams{
		UserID:     userID,
		UserGameID: userGameID,
		PlatformID: platformID,
	}); err != nil {
		return h.httpError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type moveToLibraryRequest struct {
	Platforms []platformRequest `json:"platforms"`
}

// HandleMoveToLibrary handles POST /api/user-games/:id/move-to-library. It
// converts a wishlisted entry into a library entry by attaching the supplied
// platform(s) and clearing is_wishlisted, atomically. Notes/rating/loved/tags
// stay on the same row and so carry over automatically.
func (h *UserGamesHandler) HandleMoveToLibrary(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	userGameID := c.Param("id")
	ctx := c.Request().Context()

	var req moveToLibraryRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.Platforms) == 0 {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "at least one platform is required")
	}

	platIns := make([]usergame.PlatformInput, 0, len(req.Platforms))
	for _, p := range req.Platforms {
		if p.OwnershipStatus != nil && *p.OwnershipStatus != "" {
			if !enum.OwnershipStatus(*p.OwnershipStatus).Valid() {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid ownership_status: "+*p.OwnershipStatus)
			}
		}
		var acquiredDate *time.Time
		if p.AcquiredDate != nil && *p.AcquiredDate != "" {
			acquired, err := parseAcquiredDate(*p.AcquiredDate)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			acquiredDate = acquired
		}
		platIns = append(platIns, usergame.PlatformInput{
			Platform:        p.Platform,
			Storefront:      p.Storefront,
			HoursPlayed:     p.HoursPlayed,
			OwnershipStatus: p.OwnershipStatus,
			IsAvailable:     p.IsAvailable,
			AcquiredDate:    acquiredDate,
		})
	}

	if _, err := usergame.MoveToLibrary(ctx, h.db, usergame.MoveParams{
		UserID:     userID,
		UserGameID: userGameID,
		Platforms:  platIns,
	}); err != nil {
		// ErrValidation (not wishlisted) → 422 Unprocessable Entity
		if errors.Is(err, usergame.ErrValidation) {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "user game is not on the wishlist")
		}
		return h.httpError(c, err)
	}

	ug, err := LoadUserGameDetail(ctx, h.db, userGameID, userID)
	if err != nil {
		slog.ErrorContext(ctx, "user_games: reload after move-to-library", logging.KeyErr, err, "user_game_id", userGameID, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(*ug))
}

// ── Utility endpoint types ──────────────────────────────────────────────

// UserGameIDsResponse is the response for GET /api/user-games/ids.
type UserGameIDsResponse struct {
	IDs []string `json:"ids"`
}

// GenresResponse is the response for GET /api/user-games/genres.
type GenresResponse struct {
	Genres []string `json:"genres"`
}

// FilterOptionsResponse is the response for GET /api/user-games/filter-options.
type FilterOptionsResponse struct {
	Genres             []string `json:"genres"`
	GameModes          []string `json:"game_modes"`
	Themes             []string `json:"themes"`
	PlayerPerspectives []string `json:"player_perspectives"`
}

// CollectionStatsResponse is the response for GET /api/user-games/stats.
type CollectionStatsResponse struct {
	TotalGames       int            `json:"total_games"`
	CompletionStats  map[string]int `json:"completion_stats"`
	OwnershipStats   map[string]int `json:"ownership_stats"`
	PlatformStats    map[string]int `json:"platform_stats"`
	GenreStats       map[string]int `json:"genre_stats"`
	PileOfShame      int            `json:"pile_of_shame"`
	CompletionRate   float64        `json:"completion_rate"`
	AverageRating    *float64       `json:"average_rating"`
	TotalHoursPlayed float64        `json:"total_hours_played"`
}

// ── Utility helpers ─────────────────────────────────────────────────────

// HandleClearLibrary handles DELETE /api/user-games.
// Removes all games and jobs for the authenticated user. Sync configs are
// intentionally preserved so storefronts can be re-synced to repopulate the library.
func (h *UserGamesHandler) HandleClearLibrary(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()
	deleted, err := usergame.ClearLibrary(ctx, h.db, userID)
	if err != nil {
		return h.httpError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"deleted": deleted})
}

// splitAndCollect splits a comma-separated string and adds trimmed non-empty
// values to the provided set.
func splitAndCollect(s *string, set map[string]bool) {
	if s == nil {
		return
	}
	splitAndCollectStr(*s, set)
}

// splitAndCollectStr splits a comma-separated string and adds trimmed non-empty
// values to the provided set.
func splitAndCollectStr(s string, set map[string]bool) {
	for v := range strings.SplitSeq(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			set[v] = true
		}
	}
}

// splitAndCount splits a comma-separated string and increments counts for each
// trimmed non-empty value.
func splitAndCount(s string, counts map[string]int) {
	for v := range strings.SplitSeq(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			counts[v]++
		}
	}
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// validPlayStatusParams reads the repeatable play_status query param, keeping
// only recognised statuses. Unknown values are dropped rather than rejected,
// mirroring the other lenient multi-value facet params.
func validPlayStatusParams(c *echo.Context) []string {
	var out []string
	for _, s := range c.QueryParams()["play_status"] {
		if enum.PlayStatus(s).Valid() {
			out = append(out, s)
		}
	}
	return out
}

// applyUserGameFacetFilters applies every ad-hoc facet query param from the
// request onto fb. It is the single source of truth for the user-game list
// facets, shared by HandleListUserGames (non-pool branch) and
// HandleListUserGameIDs so the list view and "select all matching filter"
// can never drift on a facet again (issue #969).
func applyUserGameFacetFilters(fb *filter.FilterBuilder, c *echo.Context) {
	filter.ApplyPlayStatus(fb, validPlayStatusParams(c))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
	filter.ApplySearch(fb, c.QueryParam("q"))
	filter.ApplyWishlist(fb, c.QueryParam("wishlist") == "true")

	if str := c.QueryParam("is_loved"); str != "" {
		v := str == "true"
		filter.ApplyIsLoved(fb, &v)
	}
	if str := c.QueryParam("has_notes"); str != "" {
		v := str == "true"
		filter.ApplyHasNotes(fb, &v)
	}
	if str := c.QueryParam("rating_min"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			filter.ApplyRatingMin(fb, &v)
		}
	}
	if str := c.QueryParam("rating_max"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			filter.ApplyRatingMax(fb, &v)
		}
	}
	var ttbMin, ttbMax *float64
	if str := c.QueryParam("time_to_beat_min"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			ttbMin = &v
		}
	}
	if str := c.QueryParam("time_to_beat_max"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			ttbMax = &v
		}
	}
	filter.ApplyTimeToBeat(fb, ttbMin, ttbMax)
	filter.ApplyPlatform(fb, c.QueryParams()["platform"])
	filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
	filter.ApplyGenre(fb, c.QueryParams()["genre"])
	filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
	filter.ApplyTheme(fb, c.QueryParams()["theme"])
	filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
	filter.ApplyTag(fb, c.QueryParams()["tag"])
}

// applyPoolFilter loads pool poolID's saved filter (owned by userID) and applies
// it — plus the global finished-status exclusion (NULL stays eligible) — to fb.
// It is the single source of truth for the ?pool= branch, shared by
// HandleListUserGames and HandleListUserGameIDs so the pool list view and "select
// all matching filter" can never drift on the pool dimension (issue #997).
//
// isManual=true means the pool has no stored filter (a pure manual pool); callers
// should return an empty result set rather than every game. A missing pool returns
// sql.ErrNoRows (caller maps to 404); a malformed stored filter returns a non-nil
// err (caller maps to 500).
func (h *UserGamesHandler) applyPoolFilter(ctx context.Context, fb *filter.FilterBuilder, poolID, userID string) (isManual bool, err error) {
	// Scan the nullable jsonb column into *string: a NULL filter yields nil
	// (a pure manual pool); a stored filter yields its JSON text.
	var rawStr *string
	if err := h.db.NewRaw(
		`SELECT filter FROM pools WHERE id = ? AND user_id = ?`, poolID, userID,
	).Scan(ctx, &rawStr); err != nil {
		return false, err
	}
	if rawStr == nil || *rawStr == "" || *rawStr == "null" {
		return true, nil
	}
	pf, err := filter.ParsePoolFilter([]byte(*rawStr))
	if err != nil {
		return false, err
	}
	filter.ApplyPoolFilter(fb, pf)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("(ug.play_status IS NULL OR ug.play_status NOT IN (?))",
			bun.List(enum.FinishedPlayStatusStrings()))
	})
	return false, nil
}

// ── Utility endpoint handlers ───────────────────────────────────────────

// HandleListUserGameIDs handles GET /api/user-games/ids.
func (h *UserGamesHandler) HandleListUserGameIDs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	// With ?pool=:id the pool's saved filter drives the id set, mirroring
	// HandleListUserGames; otherwise apply the ad-hoc facet params.
	fb := filter.NewFilterBuilder()
	if poolID := c.QueryParam("pool"); poolID != "" {
		isManual, err := h.applyPoolFilter(ctx, fb, poolID, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return echo.NewHTTPError(http.StatusNotFound, "pool not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		if isManual {
			// Pure manual pool — the list view returns no suggestions.
			return c.JSON(http.StatusOK, UserGameIDsResponse{IDs: []string{}})
		}
	} else {
		applyUserGameFacetFilters(fb, c)
	}

	q := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("DISTINCT ug.id").
		Where("ug.user_id = ?", userID)
	q = fb.Apply(q)

	var ids []string
	if err := q.Scan(ctx, &ids); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if ids == nil {
		ids = []string{}
	}

	return c.JSON(http.StatusOK, UserGameIDsResponse{IDs: ids})
}

// HandleListGenres handles GET /api/user-games/genres.
func (h *UserGamesHandler) HandleListGenres(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	var rawGenres []string
	err := h.db.NewSelect().
		TableExpr("games AS g").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		ColumnExpr("g.genre").
		Where("ug.user_id = ?", userID).
		Where("g.genre IS NOT NULL").
		Scan(ctx, &rawGenres)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	set := make(map[string]bool)
	for _, raw := range rawGenres {
		splitAndCollectStr(raw, set)
	}

	return c.JSON(http.StatusOK, GenresResponse{Genres: sortedKeys(set)})
}

// HandleFilterOptions handles GET /api/user-games/filter-options.
func (h *UserGamesHandler) HandleFilterOptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	type row struct {
		Genre              *string `bun:"genre"`
		GameModes          *string `bun:"game_modes"`
		Themes             *string `bun:"themes"`
		PlayerPerspectives *string `bun:"player_perspectives"`
	}

	var rows []row
	err := h.db.NewSelect().
		TableExpr("games AS g").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		ColumnExpr("g.genre, g.game_modes, g.themes, g.player_perspectives").
		Where("ug.user_id = ?", userID).
		Scan(ctx, &rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	genres := make(map[string]bool)
	gameModes := make(map[string]bool)
	themes := make(map[string]bool)
	perspectives := make(map[string]bool)

	for _, r := range rows {
		splitAndCollect(r.Genre, genres)
		splitAndCollect(r.GameModes, gameModes)
		splitAndCollect(r.Themes, themes)
		splitAndCollect(r.PlayerPerspectives, perspectives)
	}

	return c.JSON(http.StatusOK, FilterOptionsResponse{
		Genres:             sortedKeys(genres),
		GameModes:          sortedKeys(gameModes),
		Themes:             sortedKeys(themes),
		PlayerPerspectives: sortedKeys(perspectives),
	})
}

// HandleCollectionStats handles GET /api/user-games/stats.
func (h *UserGamesHandler) HandleCollectionStats(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()
	resp := CollectionStatsResponse{
		CompletionStats: map[string]int{
			"not_started": 0, "in_progress": 0, "completed": 0, "mastered": 0,
			"dominated": 0, "shelved": 0, "dropped": 0, "replay": 0,
		},
		OwnershipStats: map[string]int{
			"owned": 0, "borrowed": 0, "rented": 0, "subscription": 0, "no_longer_owned": 0,
		},
		PlatformStats: map[string]int{},
		GenreStats:    map[string]int{},
	}

	// total_games
	total, err := h.db.NewSelect().
		TableExpr("user_games").
		Where("user_id = ?", userID).
		Where("is_wishlisted = ?", false).
		Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	resp.TotalGames = total

	// completion_stats
	type statusCount struct {
		PlayStatus string `bun:"play_status"`
		Count      int    `bun:"count"`
	}
	var statusCounts []statusCount
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("COALESCE(play_status, 'not_started') AS play_status, COUNT(*) AS count").
		Where("user_id = ?", userID).
		Where("is_wishlisted = ?", false).
		GroupExpr("play_status").
		Scan(ctx, &statusCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, sc := range statusCounts {
		resp.CompletionStats[sc.PlayStatus] = sc.Count
	}

	// ownership_stats
	type ownershipCount struct {
		OwnershipStatus string `bun:"ownership_status"`
		Count           int    `bun:"count"`
	}
	var ownershipCounts []ownershipCount
	err = h.db.NewSelect().
		TableExpr("user_game_platforms AS ugp").
		Join("JOIN user_games AS ug ON ug.id = ugp.user_game_id").
		ColumnExpr("COALESCE(ugp.ownership_status, 'owned') AS ownership_status, COUNT(DISTINCT ugp.user_game_id) AS count").
		Where("ug.user_id = ?", userID).
		Where("ug.is_wishlisted = ?", false).
		GroupExpr("ugp.ownership_status").
		Scan(ctx, &ownershipCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, oc := range ownershipCounts {
		resp.OwnershipStats[oc.OwnershipStatus] = oc.Count
	}

	// platform_stats
	type platformCount struct {
		DisplayName string `bun:"display_name"`
		Count       int    `bun:"count"`
	}
	var platformCounts []platformCount
	err = h.db.NewSelect().
		TableExpr("user_game_platforms AS ugp").
		Join("JOIN user_games AS ug ON ug.id = ugp.user_game_id").
		Join("JOIN platforms AS p ON p.name = ugp.platform").
		ColumnExpr("p.display_name, COUNT(*) AS count").
		Where("ug.user_id = ?", userID).
		Where("ug.is_wishlisted = ?", false).
		GroupExpr("p.name, p.display_name").
		Scan(ctx, &platformCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, pc := range platformCounts {
		resp.PlatformStats[pc.DisplayName] = pc.Count
	}

	// genre_stats
	var rawGenres []string
	err = h.db.NewSelect().
		TableExpr("games AS g").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		ColumnExpr("g.genre").
		Where("ug.user_id = ?", userID).
		Where("ug.is_wishlisted = ?", false).
		Where("g.genre IS NOT NULL").
		Scan(ctx, &rawGenres)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, raw := range rawGenres {
		splitAndCount(raw, resp.GenreStats)
	}

	// pile_of_shame
	resp.PileOfShame = resp.CompletionStats["not_started"]

	// completion_rate
	if total > 0 {
		completed := resp.CompletionStats["completed"] + resp.CompletionStats["mastered"] + resp.CompletionStats["dominated"]
		resp.CompletionRate = math.Round(float64(completed)/float64(total)*10000) / 100
	}

	// average_rating
	var avgRating sql.NullFloat64
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("AVG(personal_rating)").
		Where("user_id = ?", userID).
		Where("is_wishlisted = ?", false).
		Where("personal_rating IS NOT NULL").
		Scan(ctx, &avgRating)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if avgRating.Valid {
		v := math.Round(avgRating.Float64*100) / 100
		resp.AverageRating = &v
	}

	// total_hours_played — sum platform hours across all user_game_platforms
	var totalHours float64
	err = h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("COALESCE(SUM(ugp.hours_played), 0)").
		Join("LEFT JOIN user_game_platforms AS ugp ON ugp.user_game_id = ug.id").
		Where("ug.user_id = ?", userID).
		Where("ug.is_wishlisted = ?", false).
		Scan(ctx, &totalHours)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	resp.TotalHoursPlayed = totalHours

	return c.JSON(http.StatusOK, resp)
}
