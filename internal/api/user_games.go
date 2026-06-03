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

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/filter"
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
	Platforms      []platformRequest `json:"platforms"`
}

// userGamePlatformResponse is the API DTO for a user game platform entry with nested detail objects.
type userGamePlatformResponse struct {
	ID                     string              `json:"id"`
	UserGameID             string              `json:"user_game_id"`
	Platform               *string             `json:"platform"`
	Storefront             *string             `json:"storefront"`
	StoreGameID            *string             `json:"store_game_id"`
	StoreUrl               *string             `json:"store_url"`
	IsAvailable            bool                `json:"is_available"`
	HoursPlayed            *float64            `json:"hours_played"`
	OwnershipStatus        *string             `json:"ownership_status"`
	AcquiredDate           *time.Time          `json:"acquired_date"`
	OriginalPlatformName   *string             `json:"original_platform_name"`
	OriginalStorefrontName *string             `json:"original_storefront_name"`
	ExternalGameID         *string             `json:"external_game_id"`
	SyncFromSource         bool                `json:"sync_from_source"`
	CreatedAt              time.Time           `json:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at"`
	PlatformDetails        *platformResponse   `json:"platform_details,omitempty"`
	StorefrontDetails      *storefrontResponse `json:"storefront_details,omitempty"`
}

func toUserGamePlatformResponse(ugp models.UserGamePlatform) userGamePlatformResponse {
	resp := userGamePlatformResponse{
		ID:                     ugp.ID,
		UserGameID:             ugp.UserGameID,
		Platform:               ugp.Platform,
		Storefront:             ugp.Storefront,
		StoreGameID:            ugp.StoreGameID,
		StoreUrl:               ugp.StoreUrl,
		IsAvailable:            ugp.IsAvailable,
		HoursPlayed:            ugp.HoursPlayed,
		OwnershipStatus:        ugp.OwnershipStatus,
		AcquiredDate:           ugp.AcquiredDate,
		OriginalPlatformName:   ugp.OriginalPlatformName,
		OriginalStorefrontName: ugp.OriginalStorefrontName,
		ExternalGameID:         ugp.ExternalGameID,
		SyncFromSource:         ugp.SyncFromSource,
		CreatedAt:              ugp.CreatedAt,
		UpdatedAt:              ugp.UpdatedAt,
	}
	if ugp.PlatformRecord != nil {
		pr := toPlatformResponse(*ugp.PlatformRecord)
		resp.PlatformDetails = &pr
	}
	if ugp.StorefrontRecord != nil {
		sr := toStorefrontResponse(*ugp.StorefrontRecord)
		resp.StorefrontDetails = &sr
	}
	return resp
}

// userGameWithPlatformsResponse wraps UserGame but serialises Platforms as DTOs with
// nested details and exposes a calculated game-level HoursPlayed (sum of platform hours).
type userGameWithPlatformsResponse struct {
	models.UserGame
	HoursPlayed float64                    `json:"hours_played"`
	Platforms   []userGamePlatformResponse `json:"platforms"`
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

	// Build filter.
	fb := filter.NewFilterBuilder()
	filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
	filter.ApplySearch(fb, c.QueryParam("q"))

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
	filter.ApplyPlatform(fb, c.QueryParams()["platform"])
	filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
	filter.ApplyGenre(fb, c.QueryParams()["genre"])
	filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
	filter.ApplyTheme(fb, c.QueryParams()["theme"])
	filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
	filter.ApplyTag(fb, c.QueryParams()["tag"])

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
	q := h.db.NewSelect().
		Model(&userGames).
		Where("user_game.id IN (?)", bun.List(ids)).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		})

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
	return c.JSON(http.StatusOK, UserGameListResponse{
		UserGames: dtos,
		Total:     total,
		Page:      page,
		PerPage:   perPage,
		Pages:     pages,
	})
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

	now := time.Now().UTC()
	ug := &models.UserGame{
		ID:             uuid.NewString(),
		UserID:         userID,
		GameID:         req.GameID,
		PlayStatus:     req.PlayStatus,
		PersonalRating: req.PersonalRating,
		IsLoved:        req.IsLoved,
		PersonalNotes:  req.PersonalNotes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err = h.db.NewInsert().Model(ug).Exec(ctx)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "game already in collection")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if len(req.Platforms) > 0 {
		plats := make([]*models.UserGamePlatform, len(req.Platforms))
		for i, p := range req.Platforms {
			pl := &models.UserGamePlatform{
				ID:              uuid.New().String(),
				UserGameID:      ug.ID,
				Platform:        p.Platform,
				Storefront:      p.Storefront,
				StoreGameID:     p.StoreGameID,
				StoreUrl:        p.StoreUrl,
				HoursPlayed:     p.HoursPlayed,
				OwnershipStatus: p.OwnershipStatus,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if p.IsAvailable != nil {
				pl.IsAvailable = *p.IsAvailable
			} else {
				pl.IsAvailable = true
			}
			plats[i] = pl
		}
		if _, err := h.db.NewInsert().Model(&plats).Exec(ctx); err != nil {
			slog.Error("user_games: failed to insert platforms on create", "err", err, "user_game_id", ug.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		if err := usergame.PromoteToInProgressIfPlayed(ctx, h.db, ug.ID); err != nil {
			slog.Error("user_games: auto-promote play_status on create", "err", err, "user_game_id", ug.ID)
		}
	}

	// Eager-load relations so the response includes the game, platforms and tags.
	if err := h.db.NewSelect().Model(ug).
		Where("user_game.id = ?", ug.ID).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx); err != nil {
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

	var ug models.UserGame
	err := h.db.NewSelect().Model(&ug).
		Where("user_game.id = ?", id).
		Where("user_game.user_id = ?", userID).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(ug))
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

	// Build dynamic SET clause. Column names match the bun struct tags.
	colMap := map[string]string{
		"play_status":     "play_status",
		"personal_rating": "personal_rating",
		"is_loved":        "is_loved",
		"personal_notes":  "personal_notes",
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []any{}
	for k, v := range body {
		col := colMap[k]
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", col))
		args = append(args, v)
	}

	query := fmt.Sprintf(
		`UPDATE user_games SET %s WHERE id = ? AND user_id = ?
		 RETURNING id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)
	args = append(args, id, userID)

	var ug models.UserGame
	if err := h.db.NewRaw(query, args...).Scan(ctx, &ug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Eager-load relations for the response.
	if err := h.db.NewSelect().Model(&ug).
		Where("user_game.id = ?", ug.ID).
		Relation("Game").
		Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("PlatformRecord").Relation("StorefrontRecord")
		}).
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(ug))
}

// HandleDeleteUserGame handles DELETE /api/user-games/:id.
func (h *UserGamesHandler) HandleDeleteUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")
	ctx := context.Background()

	res, err := h.db.NewDelete().
		Model((*models.UserGame)(nil)).
		Where("user_game.id = ?", id).
		Where("user_game.user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, err := res.RowsAffected()
	if err != nil || rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	return c.NoContent(http.StatusNoContent)
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

	args := []any{time.Now().UTC(), *req.PlayStatus, id, userID}

	query := `UPDATE user_games SET updated_at = ?, play_status = ? WHERE id = ? AND user_id = ? RETURNING id, user_id, game_id, play_status, personal_rating, is_loved, personal_notes, created_at, updated_at`

	ctx := context.Background()
	var ug models.UserGame
	err := h.db.NewRaw(query, args...).Scan(ctx, &ug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, toUserGameWithPlatformsResponse(ug))
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

	allowedFields := map[string]string{
		"play_status":     "play_status",
		"is_loved":        "is_loved",
		"personal_rating": "personal_rating",
	}

	var setClauses []string
	args := []any{time.Now().UTC()}

	for key, val := range req.Updates {
		col, ok := allowedFields[key]
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown update field: "+key)
		}
		if key == "play_status" {
			ps, ok := val.(string)
			if !ok {
				return echo.NewHTTPError(http.StatusBadRequest, "play_status must be a string")
			}
			if !enum.PlayStatus(ps).Valid() {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
			}
		}
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}

	// append WHERE args: list of IDs, then userID
	args = append(args, bun.List(req.IDs), userID)

	query := fmt.Sprintf(
		`UPDATE user_games SET updated_at = ?, %s WHERE id IN (?) AND user_id = ?`,
		strings.Join(setClauses, ", "),
	)

	ctx := context.Background()
	var rowsAffected int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		rowsAffected, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, map[string]any{"updated": rowsAffected})
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
	var rowsAffected int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewDelete().
			Model((*models.UserGame)(nil)).
			Where("id IN (?)", bun.List(req.IDs)).
			Where("user_game.user_id = ?", userID).
			Exec(ctx)
		if err != nil {
			return err
		}
		rowsAffected, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, map[string]any{"deleted": rowsAffected})
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

	ctx := context.Background()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	defer func() { _ = tx.Rollback() }()

	// Get only owned IDs.
	var ownedIDs []string
	err = tx.NewSelect().Model((*models.UserGame)(nil)).
		Column("id").
		Where("id IN (?)", bun.List(req.UserGameIDs)).
		Where("user_id = ?", userID).
		Scan(ctx, &ownedIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if len(ownedIDs) == 0 {
		return c.JSON(http.StatusOK, map[string]int{"added": 0})
	}

	now := time.Now().UTC()
	var added int64
	for _, ugID := range ownedIDs {
		result, insertErr := tx.NewRaw(
			`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			uuid.NewString(), ugID, req.Platform, req.Storefront, now, now,
		).Exec(ctx)
		if insertErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
		added += rows
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, map[string]int64{"added": added})
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
	result, err := h.db.NewRaw(
		`DELETE FROM user_game_platforms
		 WHERE user_game_id IN (
		   SELECT id FROM user_games WHERE id IN (?) AND user_id = ?
		 )
		 AND platform = ? AND storefront = ?`,
		bun.List(req.UserGameIDs), userID, req.Platform, req.Storefront,
	).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	return c.JSON(http.StatusOK, map[string]int64{"removed": rows})
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
	StoreGameID     *string  `json:"store_game_id"`
	StoreUrl        *string  `json:"store_url"`
	IsAvailable     *bool    `json:"is_available"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
	AcquiredDate    *string  `json:"acquired_date"`
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

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user game not found"})
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

	now := time.Now()
	plat := &models.UserGamePlatform{
		ID:              uuid.New().String(),
		UserGameID:      userGameID,
		Platform:        req.Platform,
		Storefront:      req.Storefront,
		StoreGameID:     req.StoreGameID,
		StoreUrl:        req.StoreUrl,
		HoursPlayed:     req.HoursPlayed,
		OwnershipStatus: req.OwnershipStatus,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if req.IsAvailable != nil {
		plat.IsAvailable = *req.IsAvailable
	}

	_, err := h.db.NewInsert().Model(plat).Exec(ctx)
	if err != nil {
		if isDuplicateKeyError(err) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "platform/storefront combination already exists"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	if err := usergame.PromoteToInProgressIfPlayed(ctx, h.db, userGameID); err != nil {
		slog.Error("user_games: auto-promote play_status on create platform", "err", err, "user_game_id", userGameID)
	}
	if err := h.db.NewSelect().Model(plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.Error("user_games: load platform relations failed", "err", err, "platform_id", plat.ID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load platform")
	}
	return c.JSON(http.StatusCreated, toUserGamePlatformResponse(*plat))
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

	// Apply updates
	if req.Platform != nil {
		plat.Platform = req.Platform
	}
	if req.Storefront != nil {
		plat.Storefront = req.Storefront
	}
	if req.StoreGameID != nil {
		plat.StoreGameID = req.StoreGameID
	}
	if req.StoreUrl != nil {
		plat.StoreUrl = req.StoreUrl
	}
	if req.IsAvailable != nil {
		plat.IsAvailable = *req.IsAvailable
	}
	if req.HoursPlayed != nil {
		plat.HoursPlayed = req.HoursPlayed
	}
	if req.OwnershipStatus != nil {
		plat.OwnershipStatus = req.OwnershipStatus
	}
	plat.UpdatedAt = time.Now()

	_, err = h.db.NewUpdate().Model(&plat).WherePK().Exec(ctx)
	if err != nil {
		if isDuplicateKeyError(err) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "platform/storefront combination already exists"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	if err := usergame.PromoteToInProgressIfPlayed(ctx, h.db, userGameID); err != nil {
		slog.Error("user_games: auto-promote play_status on update platform", "err", err, "user_game_id", userGameID)
	}
	if err := h.db.NewSelect().Model(&plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.Error("user_games: load platform relations failed", "err", err, "platform_id", plat.ID)
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

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user game not found"})
	}

	result, err := h.db.NewDelete().Model((*models.UserGamePlatform)(nil)).
		Where("id = ?", platformID).
		Where("user_game_id = ?", userGameID).
		Exec(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
	}
	rows, _ := result.RowsAffected() //nolint:errcheck // RowsAffected never errors for the pq driver; count is advisory
	if rows == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "platform not found"})
	}
	return c.NoContent(http.StatusNoContent)
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
	var deleted int64
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Cancel active River jobs whose items belong to this user.
		if _, err := tx.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = NOW()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE user_id = ?)`,
			userID,
		).Exec(ctx); err != nil {
			return err
		}
		// Delete jobs (cascades job_items + changes).
		if _, err := tx.NewDelete().Model((*models.Job)(nil)).
			Where("user_id = ?", userID).Exec(ctx); err != nil {
			return err
		}
		// Delete user games (cascades user_game_platforms + user_game_tags).
		res, err := tx.NewDelete().Model((*models.UserGame)(nil)).
			Where("user_id = ?", userID).Exec(ctx)
		if err != nil {
			return err
		}
		deleted, err = res.RowsAffected()
		return err
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
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

// ── Utility endpoint handlers ───────────────────────────────────────────

// HandleListUserGameIDs handles GET /api/user-games/ids.
func (h *UserGamesHandler) HandleListUserGameIDs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	fb := filter.NewFilterBuilder()
	filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
	filter.ApplySearch(fb, c.QueryParam("q"))

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
	filter.ApplyPlatform(fb, c.QueryParams()["platform"])
	filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
	filter.ApplyGenre(fb, c.QueryParams()["genre"])
	filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
	filter.ApplyTheme(fb, c.QueryParams()["theme"])
	filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
	filter.ApplyTag(fb, c.QueryParams()["tag"])

	ctx := context.Background()

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

	// 1. total_games
	total, err := h.db.NewSelect().
		TableExpr("user_games").
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	resp.TotalGames = total

	// 2. completion_stats
	type statusCount struct {
		PlayStatus string `bun:"play_status"`
		Count      int    `bun:"count"`
	}
	var statusCounts []statusCount
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("COALESCE(play_status, 'not_started') AS play_status, COUNT(*) AS count").
		Where("user_id = ?", userID).
		GroupExpr("play_status").
		Scan(ctx, &statusCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, sc := range statusCounts {
		resp.CompletionStats[sc.PlayStatus] = sc.Count
	}

	// 3. ownership_stats
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
		GroupExpr("ugp.ownership_status").
		Scan(ctx, &ownershipCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, oc := range ownershipCounts {
		resp.OwnershipStats[oc.OwnershipStatus] = oc.Count
	}

	// 4. platform_stats
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
		GroupExpr("p.name, p.display_name").
		Scan(ctx, &platformCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, pc := range platformCounts {
		resp.PlatformStats[pc.DisplayName] = pc.Count
	}

	// 5. genre_stats
	var rawGenres []string
	err = h.db.NewSelect().
		TableExpr("games AS g").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		ColumnExpr("g.genre").
		Where("ug.user_id = ?", userID).
		Where("g.genre IS NOT NULL").
		Scan(ctx, &rawGenres)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, raw := range rawGenres {
		splitAndCount(raw, resp.GenreStats)
	}

	// 6. pile_of_shame
	resp.PileOfShame = resp.CompletionStats["not_started"]

	// 7. completion_rate
	if total > 0 {
		completed := resp.CompletionStats["completed"] + resp.CompletionStats["mastered"] + resp.CompletionStats["dominated"]
		resp.CompletionRate = math.Round(float64(completed)/float64(total)*10000) / 100
	}

	// 8. average_rating
	var avgRating sql.NullFloat64
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("AVG(personal_rating)").
		Where("user_id = ?", userID).
		Where("personal_rating IS NOT NULL").
		Scan(ctx, &avgRating)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if avgRating.Valid {
		v := math.Round(avgRating.Float64*100) / 100
		resp.AverageRating = &v
	}

	// 9. total_hours_played — sum platform hours across all user_game_platforms
	var totalHours float64
	err = h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("COALESCE(SUM(ugp.hours_played), 0)").
		Join("LEFT JOIN user_game_platforms AS ugp ON ugp.user_game_id = ug.id").
		Where("ug.user_id = ?", userID).
		Scan(ctx, &totalHours)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	resp.TotalHoursPlayed = totalHours

	return c.JSON(http.StatusOK, resp)
}
