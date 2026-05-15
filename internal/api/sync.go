package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/worker/tasks"
)

// SteamClient abstracts the Steam Web API call used during credential verification.
type SteamClient interface {
	GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error)
}

// SteamPlayerSummary holds the relevant fields from a Steam player summary response.
type SteamPlayerSummary struct {
	PersonaName              string
	CommunityVisibilityState int
}

// PSNClient abstracts the PSN NPSSO exchange and account info retrieval.
type PSNClient interface {
	GetAccountInfo(ctx context.Context, npssoToken string) (*PSNAccountInfo, error)
}

// PSNAccountInfo holds the account details retrieved from PSN.
type PSNAccountInfo struct {
	OnlineID  string
	AccountID string
	Region    string
}

var (
	ErrInvalidNPSSOToken = errors.New("invalid npsso token")
	ErrSteamRateLimited  = errors.New("steam rate limited")
	ErrSteamNetwork      = errors.New("steam network error")
)

var (
	validConfigStorefronts  = map[string]bool{"steam": true, "psn": true, "epic": true}
	validTriggerStorefronts = map[string]bool{"steam": true, "psn": true}
	supportedStorefronts    = []string{"steam", "psn", "epic"}
)

var (
	steamIDRegex  = regexp.MustCompile(`^7656119[0-9]{10}$`)
	steamKeyRegex = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
)

type syncConfigItem struct {
	ID           string     `json:"id"`
	Storefront   string     `json:"storefront"`
	Frequency    string     `json:"frequency"`
	AutoAdd      bool       `json:"auto_add"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	IsConfigured bool       `json:"is_configured"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type syncConfigResponse struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Storefront   string     `json:"storefront"`
	Frequency    string     `json:"frequency"`
	AutoAdd      bool       `json:"auto_add"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	IsConfigured bool       `json:"is_configured"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type manualSyncTriggerResponse struct {
	Message    string `json:"message"`
	JobID      string `json:"job_id"`
	Storefront string `json:"storefront"`
	Status     string `json:"status"`
}

type syncStatusResponse struct {
	Storefront   string     `json:"storefront"`
	IsSyncing    bool       `json:"is_syncing"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	ActiveJobID  *string    `json:"active_job_id"`
}

type steamVerifyResponse struct {
	Valid         bool    `json:"valid"`
	SteamUsername *string `json:"steam_username"`
	Error         *string `json:"error"`
}

type psnConfigureResponse struct {
	Success   bool   `json:"success"`
	OnlineID  string `json:"online_id"`
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
	Message   string `json:"message"`
}

type psnStatusResponse struct {
	IsConfigured bool   `json:"is_configured"`
	OnlineID     string `json:"online_id"`
	AccountID    string `json:"account_id"`
	Region       string `json:"region"`
	TokenExpired bool   `json:"token_expired"`
}

// SyncHandler handles sync configuration, trigger, and status endpoints.
type SyncHandler struct {
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	steamClient SteamClient
	psnClient   PSNClient
}

// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamClient, psn PSNClient) *SyncHandler {
	return &SyncHandler{db: db, riverClient: riverClient, steamClient: steam, psnClient: psn}
}

// RegisterRoutes registers all sync routes on the given group.
// Static-segment routes are registered before parameterised routes to avoid conflicts.
func (h *SyncHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/steam/verify", h.HandleSteamVerify)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.POST("/psn/configure", h.HandlePSNConfigure)
	g.GET("/psn/status", h.HandleGetPSNStatus)
	g.DELETE("/psn/disconnect", h.HandlePSNDisconnect)
	g.GET("/ignored", h.HandleListIgnored)
	g.POST("/ignored/:id", h.HandleSkipGame)
	g.DELETE("/ignored/:id", h.HandleUnskipGame)
	g.GET("/config", h.HandleListConfig)
	g.GET("/config/:storefront", h.HandleGetConfig)
	g.PUT("/config/:storefront", h.HandleUpdateConfig)
	g.POST("/:storefront", h.HandleTriggerSync)
	g.GET("/:storefront/status", h.HandleGetSyncStatus)
}

func (h *SyncHandler) HandleListConfig(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()

	var rows []models.UserSyncConfig
	if err := h.db.NewSelect().Model(&rows).Where("user_id = ?", userID).Scan(ctx); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load configs")
	}
	byStorefront := make(map[string]*models.UserSyncConfig, len(rows))
	for i := range rows {
		byStorefront[rows[i].Storefront] = &rows[i]
	}

	now := time.Now().UTC()
	configs := make([]syncConfigItem, 0, 3)
	for _, sf := range supportedStorefronts {
		if row, ok := byStorefront[sf]; ok {
			configs = append(configs, syncConfigItem{
				ID:           row.ID,
				Storefront:   row.Storefront,
				Frequency:    row.Frequency,
				AutoAdd:      row.AutoAdd,
				LastSyncedAt: row.LastSyncedAt,
				IsConfigured: row.StorefrontCredentials != nil,
				CreatedAt:    row.CreatedAt,
				UpdatedAt:    row.UpdatedAt,
			})
		} else {
			configs = append(configs, syncConfigItem{
				ID:           uuid.NewString(),
				Storefront:   sf,
				Frequency:    "manual",
				AutoAdd:      false,
				LastSyncedAt: nil,
				IsConfigured: false,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"configs": configs, "total": 3})
}

func (h *SyncHandler) HandleGetConfig(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validConfigStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		return c.JSON(http.StatusOK, syncConfigResponse{
			ID: uuid.NewString(), UserID: userID, Storefront: sf,
			Frequency: "manual", AutoAdd: false, IsConfigured: false,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get config")
	}
	return c.JSON(http.StatusOK, syncConfigResponse{
		ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
		Frequency: row.Frequency, AutoAdd: row.AutoAdd,
		LastSyncedAt: row.LastSyncedAt,
		IsConfigured: row.StorefrontCredentials != nil,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
}

func (h *SyncHandler) HandleUpdateConfig(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validConfigStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}

	var body struct {
		Frequency *string `json:"frequency"`
		AutoAdd   *bool   `json:"auto_add"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	ctx := context.Background()
	now := time.Now().UTC()

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		row = models.UserSyncConfig{
			ID:         uuid.NewString(),
			UserID:     userID,
			Storefront: sf,
			Frequency:  "manual",
			AutoAdd:    false,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load config")
	}

	if body.Frequency != nil {
		row.Frequency = *body.Frequency
	}
	if body.AutoAdd != nil {
		row.AutoAdd = *body.AutoAdd
	}
	row.UpdatedAt = now

	_, err = h.db.NewInsert().Model(&row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET frequency = EXCLUDED.frequency, auto_add = EXCLUDED.auto_add, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	return c.JSON(http.StatusOK, syncConfigResponse{
		ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
		Frequency: row.Frequency, AutoAdd: row.AutoAdd,
		LastSyncedAt: row.LastSyncedAt,
		IsConfigured: row.StorefrontCredentials != nil,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
}

func (h *SyncHandler) HandleTriggerSync(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validTriggerStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// Check for an existing active job.
	var existingID string
	err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &existingID)
	if err == nil {
		return echo.NewHTTPError(http.StatusConflict, "sync already in progress")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to check existing job")
	}

	now := time.Now().UTC()
	jobID := uuid.NewString()
	// priority is TEXT in the jobs table with default 'high'.
	_, err = h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at) VALUES (?, ?, 'sync', ?, 'pending', 'high', ?)`,
		jobID, userID, sf, now,
	).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job")
	}

	if h.riverClient != nil {
		_, _ = h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID: jobID, UserID: userID, Storefront: sf,
		}, nil)
	}

	return c.JSON(http.StatusOK, manualSyncTriggerResponse{
		Message:    "Sync job created for " + sf,
		JobID:      jobID,
		Storefront: sf,
		Status:     "queued",
	})
}

func (h *SyncHandler) HandleGetSyncStatus(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validTriggerStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var activeJobID *string
	var jobID string
	err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &jobID)
	if err == nil {
		activeJobID = &jobID
	}

	var lastSyncedAt *time.Time
	var cfg models.UserSyncConfig
	if err := h.db.NewSelect().Model(&cfg).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx); err == nil {
		lastSyncedAt = cfg.LastSyncedAt
	}

	return c.JSON(http.StatusOK, syncStatusResponse{
		Storefront:   sf,
		IsSyncing:    activeJobID != nil,
		LastSyncedAt: lastSyncedAt,
		ActiveJobID:  activeJobID,
	})
}

func (h *SyncHandler) HandleSteamVerify(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req struct {
		SteamID   string `json:"steam_id"`
		WebAPIKey string `json:"web_api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	errStr := func(s string) *string { return &s }

	if !steamIDRegex.MatchString(req.SteamID) {
		return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("invalid_steam_id")})
	}
	if !steamKeyRegex.MatchString(req.WebAPIKey) {
		return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("invalid_api_key")})
	}

	summary, err := h.steamClient.GetPlayerSummaries(c.Request().Context(), req.WebAPIKey, req.SteamID)
	if err != nil {
		if errors.Is(err, ErrSteamRateLimited) {
			return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("rate_limited")})
		}
		return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("network_error")})
	}
	if summary == nil {
		return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("invalid_steam_id")})
	}
	if summary.CommunityVisibilityState != 3 {
		return c.JSON(http.StatusOK, steamVerifyResponse{Valid: false, Error: errStr("private_profile")})
	}

	creds := map[string]string{
		"web_api_key":  req.WebAPIKey,
		"steam_id":     req.SteamID,
		"display_name": summary.PersonaName,
	}
	credsJSON, _ := json.Marshal(creds)
	credsStr := string(credsJSON)
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID: uuid.NewString(), UserID: userID, Storefront: "steam",
		Frequency: "manual", StorefrontCredentials: &credsStr,
		CreatedAt: now, UpdatedAt: now,
	}
	_, _ = h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background())

	name := summary.PersonaName
	return c.JSON(http.StatusOK, steamVerifyResponse{Valid: true, SteamUsername: &name})
}

func (h *SyncHandler) HandleSteamDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Exec(context.Background())
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandlePSNConfigure(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req struct {
		NpssoToken string `json:"npsso_token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.NpssoToken) != 64 {
		return echo.NewHTTPError(http.StatusBadRequest, "npsso_token must be exactly 64 characters")
	}

	info, err := h.psnClient.GetAccountInfo(c.Request().Context(), req.NpssoToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid_npsso_token")
	}

	creds := map[string]any{
		"npsso_token":      req.NpssoToken,
		"online_id":        info.OnlineID,
		"account_id":       info.AccountID,
		"region":           info.Region,
		"is_verified":      true,
		"token_expired_at": nil,
	}
	credsJSON, _ := json.Marshal(creds)
	credsStr := string(credsJSON)
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID: uuid.NewString(), UserID: userID, Storefront: "psn",
		Frequency: "manual", StorefrontCredentials: &credsStr,
		CreatedAt: now, UpdatedAt: now,
	}
	_, _ = h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background())

	return c.JSON(http.StatusOK, psnConfigureResponse{
		Success:   true,
		OnlineID:  info.OnlineID,
		AccountID: info.AccountID,
		Region:    info.Region,
		Message:   "PSN configured successfully",
	})
}

func (h *SyncHandler) HandleGetPSNStatus(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = 'psn'", userID).Scan(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, psnStatusResponse{})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get psn config")
	}
	if row.StorefrontCredentials == nil {
		return c.JSON(http.StatusOK, psnStatusResponse{})
	}

	var creds struct {
		OnlineID       string     `json:"online_id"`
		AccountID      string     `json:"account_id"`
		Region         string     `json:"region"`
		IsVerified     bool       `json:"is_verified"`
		TokenExpiredAt *time.Time `json:"token_expired_at"`
	}
	_ = json.Unmarshal([]byte(*row.StorefrontCredentials), &creds)

	return c.JSON(http.StatusOK, psnStatusResponse{
		IsConfigured: true,
		OnlineID:     creds.OnlineID,
		AccountID:    creds.AccountID,
		Region:       creds.Region,
		TokenExpired: !creds.IsVerified && creds.TokenExpiredAt != nil,
	})
}

func (h *SyncHandler) HandlePSNDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	_, _ = h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
		userID,
	).Exec(context.Background())
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleListIgnored(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var games []models.ExternalGame
	err := h.db.NewSelect().Model(&games).
		Where("user_id = ? AND is_skipped = true", userID).
		Scan(context.Background())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list ignored games")
	}
	if games == nil {
		games = []models.ExternalGame{}
	}
	return c.JSON(http.StatusOK, games)
}

func (h *SyncHandler) HandleSkipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var ownerID string
	err := h.db.NewRaw(`SELECT user_id FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerID)
	if errors.Is(err, sql.ErrNoRows) || ownerID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	_, _ = h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx)
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleUnskipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var ownerID string
	err := h.db.NewRaw(`SELECT user_id FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerID)
	if errors.Is(err, sql.ErrNoRows) || ownerID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	_, _ = h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx)
	return c.NoContent(http.StatusNoContent)
}
