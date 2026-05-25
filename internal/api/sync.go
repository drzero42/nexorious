package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/worker/tasks"
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

// EpicClient abstracts legendary CLI calls used during Epic account connection.
type EpicClient interface {
	// Authenticate runs legendary auth, returns account info and the resulting
	// state snapshot. The caller stores the snapshot in the DB.
	Authenticate(ctx context.Context, userID, authCode string) (*EpicAccountInfo, map[string]string, error)
	Cleanup(ctx context.Context, userID string) error
	Configured() bool
}

// EpicAccountInfo holds the Epic account details.
type EpicAccountInfo struct {
	DisplayName string
	AccountID   string
}

// GOGClient abstracts the GOG OAuth calls used during account connection.
type GOGClient interface {
	BuildAuthURL() string
	ExchangeCode(ctx context.Context, code string) (*GOGTokenResponse, error)
}

// GOGTokenResponse holds the tokens and identity returned by GOG auth.
type GOGTokenResponse struct {
	AccessToken  string
	RefreshToken string
	UserID       string
	Username     string
}

var (
	ErrInvalidNPSSOToken = errors.New("invalid npsso token")
	ErrSteamRateLimited  = errors.New("steam rate limited")
	ErrSteamNetwork      = errors.New("steam network error")
)

var (
	validConfigStorefronts  = map[string]bool{"steam": true, "psn": true, "epic": true, "gog": true}
	validTriggerStorefronts = map[string]bool{"steam": true, "psn": true, "epic": true, "gog": true}
	supportedStorefronts    = []string{"steam", "psn", "epic", "gog"}
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

type externalGameResponse struct {
	ID                         string  `bun:"id"                             json:"id"`
	Storefront                 string  `bun:"storefront"                     json:"storefront"`
	ExternalID                 string  `bun:"external_id"                    json:"external_id"`
	Title                      string  `bun:"title"                          json:"title"`
	ResolvedIGDBID             *int32  `bun:"resolved_igdb_id"               json:"resolved_igdb_id"`
	IsSkipped                  bool    `bun:"is_skipped"                     json:"is_skipped"`
	IsAvailable                bool    `bun:"is_available"                   json:"is_available"`
	IsSubscription             bool    `bun:"is_subscription"                json:"is_subscription"`
	HasUserGame                bool    `bun:"has_user_game"                  json:"has_user_game"`
	UserGameID                 *string `bun:"user_game_id"                   json:"user_game_id"`
	IGDBTitle                  *string `bun:"igdb_title"                     json:"igdb_title"`
	UserGameOtherPlatformCount int     `bun:"user_game_other_platform_count" json:"user_game_other_platform_count"`
	SyncStatus                 string  `bun:"sync_status"                    json:"sync_status"`
	FailedJobItemID            *string `bun:"failed_job_item_id"             json:"failed_job_item_id"`
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
	IsConfigured     bool   `json:"is_configured"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	OnlineID         string `json:"online_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	Region           string `json:"region,omitempty"`
}

type steamConnectionResponse struct {
	Connected        bool   `json:"connected"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	SteamID          string `json:"steam_id,omitempty"`
	Username         string `json:"username,omitempty"`
}

// SyncHandler handles sync configuration, trigger, and status endpoints.
type SyncHandler struct {
	db          *bun.DB
	encrypter   *crypto.Encrypter
	riverClient *river.Client[pgx.Tx]
	steamClient SteamClient
	psnClient   PSNClient
	epicClient  EpicClient
	gogClient   GOGClient
}

// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(encrypter *crypto.Encrypter, db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamClient, psn PSNClient, epic EpicClient, gog GOGClient) *SyncHandler {
	return &SyncHandler{encrypter: encrypter, db: db, riverClient: riverClient, steamClient: steam, psnClient: psn, epicClient: epic, gogClient: gog}
}

// RegisterRoutes registers all sync routes on the given group.
// Static-segment routes are registered before parameterised routes to avoid conflicts.
func (h *SyncHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/steam/verify", h.HandleSteamVerify)
	g.GET("/steam/connection", h.HandleGetSteamConnection)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.POST("/psn/configure", h.HandlePSNConfigure)
	g.GET("/psn/connection", h.HandleGetPSNStatus)
	g.DELETE("/psn/connection", h.HandlePSNDisconnect)
	g.POST("/epic/connect", h.HandleEpicConnect)
	g.DELETE("/epic/connection", h.HandleEpicDisconnect)
	g.GET("/epic/connection", h.HandleGetEpicConnection)
	g.POST("/gog/connect", h.HandleGOGConnect)
	g.GET("/gog/connection", h.HandleGetGOGConnection)
	g.DELETE("/gog/connection", h.HandleGOGDisconnect)
	g.GET("/ignored", h.HandleListIgnored)
	g.POST("/ignored/:id", h.HandleSkipGame)
	g.DELETE("/ignored/:id", h.HandleUnskipGame)
	// "external-games" is a static prefix — must be registered before /:storefront (POST)
	// per Echo v5 route ordering rules.
	g.POST("/external-games/:id/rematch", h.HandleRematchExternalGame) // implemented in Task 4
	// static route /:storefront/external-games/retry-failed registered before
	// parameterised /:storefront routes per Echo v5 ordering rules.
	g.POST("/:storefront/external-games/retry-failed", h.HandleRetryFailedExternalGames)
	g.GET("/config", h.HandleListConfig)
	g.GET("/config/:storefront", h.HandleGetConfig)
	g.PUT("/config/:storefront", h.HandleUpdateConfig)
	// "/:storefront/data" must be registered before "/:storefront" (POST) per Echo v5
	// static-before-parameterised ordering; the leading DELETE /ignored/:id registration
	// means "ignored" resolves as a storefront slug rather than "data" as an ignored-item ID.
	g.DELETE("/:storefront/data", h.HandleResetSyncData)
	g.POST("/:storefront", h.HandleTriggerSync)
	g.GET("/:storefront/status", h.HandleGetSyncStatus)
	g.GET("/:storefront/external-games", h.HandleListExternalGames)
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
	return c.JSON(http.StatusOK, map[string]any{"configs": configs, "total": len(configs)})
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
		CreatedAt:    row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
		CreatedAt:    row.CreatedAt, UpdatedAt: row.UpdatedAt,
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
		if _, err = h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID: jobID, UserID: userID, Storefront: sf,
		}, nil); err != nil {
			slog.Error("sync: enqueue dispatch failed", "err", err, "job_id", jobID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync job")
		}
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
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	ciphertext, err := h.encrypter.Encrypt(credsJSON)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID: uuid.NewString(), UserID: userID, Storefront: "steam",
		Frequency: "manual", StorefrontCredentials: &ciphertext,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background()); err != nil {
		slog.Error("sync: persist steam credentials failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Steam connection")
	}

	name := summary.PersonaName
	return c.JSON(http.StatusOK, steamVerifyResponse{Valid: true, SteamUsername: &name})
}

func (h *SyncHandler) HandleSteamDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'steam'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: steam disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Steam")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleGetSteamConnection(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).
		Where("user_id = ? AND storefront = 'steam'", userID).
		Scan(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, steamConnectionResponse{Connected: false})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get steam config")
	}
	if row.StorefrontCredentials == nil {
		return c.JSON(http.StatusOK, steamConnectionResponse{Connected: false})
	}

	plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.Warn("steam: credentials decrypt failed", "user_id", userID, "err", err)
		return c.JSON(http.StatusOK, steamConnectionResponse{Connected: true, CredentialsError: true})
	}
	var creds struct {
		SteamID     string `json:"steam_id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(plainCreds, &creds); err != nil {
		slog.Error("steam: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, steamConnectionResponse{
		Connected: true,
		SteamID:   creds.SteamID,
		Username:  creds.DisplayName,
	})
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
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	ciphertext, err := h.encrypter.Encrypt(credsJSON)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID: uuid.NewString(), UserID: userID, Storefront: "psn",
		Frequency: "manual", StorefrontCredentials: &ciphertext,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background()); err != nil {
		slog.Error("psn: persist storefront credentials failed", "user_id", userID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist PSN connection")
	}

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

	plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.Warn("psn: credentials decrypt failed", "user_id", userID, "err", err)
		return c.JSON(http.StatusOK, psnStatusResponse{IsConfigured: true, CredentialsError: true})
	}
	var creds struct {
		OnlineID   string `json:"online_id"`
		AccountID  string `json:"account_id"`
		Region     string `json:"region"`
		IsVerified bool   `json:"is_verified"`
	}
	if err := json.Unmarshal(plainCreds, &creds); err != nil {
		slog.Error("psn: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, psnStatusResponse{
		IsConfigured:     true,
		CredentialsError: !creds.IsVerified,
		OnlineID:         creds.OnlineID,
		AccountID:        creds.AccountID,
		Region:           creds.Region,
	})
}

func (h *SyncHandler) HandlePSNDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: psn disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect PSN")
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleEpicConnect exchanges an Epic auth code via legendary and stores the
// resulting legendary state snapshot in the DB.
func (h *SyncHandler) HandleEpicConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if h.epicClient == nil || !h.epicClient.Configured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Epic sync not configured (LEGENDARY_WORK_DIR unset)")
	}

	var body struct {
		AuthCode string `json:"auth_code"`
	}
	if err := c.Bind(&body); err != nil || body.AuthCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	info, snapshot, err := h.epicClient.Authenticate(c.Request().Context(), userID, body.AuthCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("epic auth failed: %v", err))
	}
	if info == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "epic: user.json missing after auth")
	}

	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	stateCiphertext, err := h.encrypter.Encrypt(snapshotJSON)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	now := time.Now().UTC()

	if _, err := h.db.NewRaw(
		`INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
		 VALUES (?, ?, 'epic', 'manual', ?, ?, ?)
		 ON CONFLICT (user_id, storefront) DO UPDATE SET
		     storefront_credentials = EXCLUDED.storefront_credentials,
		     updated_at = EXCLUDED.updated_at`,
		uuid.NewString(), userID, stateCiphertext, now, now,
	).Exec(context.Background()); err != nil {
		slog.Error("epic: persist storefront credentials failed", "user_id", userID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Epic connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"display_name": info.DisplayName,
		"account_id":   info.AccountID,
	})
}

// HandleEpicDisconnect clears the user's Epic credentials and snapshot, and
// removes the legendary working directory.
func (h *SyncHandler) HandleEpicDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
		userID,
	).Exec(ctx); err != nil {
		slog.Error("sync: epic disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Epic")
	}
	if h.epicClient != nil {
		if err := h.epicClient.Cleanup(ctx, userID); err != nil {
			slog.Error("sync: epic cleanup failed", "err", err, "user_id", userID)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleGetEpicConnection returns the current Epic connection status for the
// authenticated user.
func (h *SyncHandler) HandleGetEpicConnection(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	if h.epicClient == nil || !h.epicClient.Configured() {
		return c.JSON(http.StatusOK, map[string]any{
			"connected": false,
			"disabled":  true,
			"reason":    "legendary_not_configured",
		})
	}

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).
		Where("user_id = ? AND storefront = 'epic'", userID).
		Scan(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, map[string]any{"connected": false, "disabled": false})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get epic config")
	}
	if row.StorefrontCredentials == nil {
		return c.JSON(http.StatusOK, map[string]any{"connected": false, "disabled": false})
	}

	plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.Warn("epic: credentials decrypt failed", "user_id", userID, "err", err)
		return c.JSON(http.StatusOK, map[string]any{"connected": true, "credentials_error": true, "disabled": false})
	}
	var creds struct {
		DisplayName string `json:"display_name"`
		AccountID   string `json:"account_id"`
	}
	if err := json.Unmarshal(plainCreds, &creds); err != nil {
		slog.Error("epic: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected":    true,
		"disabled":     false,
		"display_name": creds.DisplayName,
		"account_id":   creds.AccountID,
	})
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

	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: skip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}

	// Mark the most recent pending_review or pending job_item for this game as skipped,
	// then check whether the job can now complete.
	var jobItemRow struct {
		ID    string `bun:"id"`
		JobID string `bun:"job_id"`
	}
	if err := h.db.NewRaw(`
		SELECT id, job_id FROM job_items
		WHERE external_game_id = ? AND status IN ('pending_review', 'pending')
		ORDER BY created_at DESC
		LIMIT 1`, id,
	).Scan(ctx, &jobItemRow); err == nil {
		if _, err := h.db.NewRaw(
			`UPDATE job_items SET status = 'skipped', processed_at = now() WHERE id = ?`,
			jobItemRow.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: skip game: mark job_item skipped", "err", err, "job_item_id", jobItemRow.ID)
		} else {
			tasks.SyncCheckJobCompletion(ctx, h.db, jobItemRow.JobID)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleUnskipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var eg models.ExternalGame
	err := h.db.NewSelect().Model(&eg).Where("id = ? AND user_id = ?", id, userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: unskip game failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to unskip game")
	}

	// Enqueue immediate re-processing. Failure here is non-fatal — the game
	// will be picked up on the next full sync.
	jobID := uuid.NewString()
	now := time.Now().UTC()
	_, jerr := h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'high', 1, ?)`,
		jobID, userID, eg.Storefront, now,
	).Exec(ctx)
	if jerr == nil {
		itemID := uuid.NewString()
		if _, err := h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, eg.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: insert job_item for unskip failed", "err", err, "external_game_id", eg.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.IGDBMatchArgs{JobItemID: itemID}, nil); err != nil {
				slog.Error("sync: enqueue igdb_match failed", "err", err, "job_item_id", itemID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
			}
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleListExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validTriggerStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var games []externalGameResponse
	err := h.db.NewRaw(`
		SELECT
			eg.id,
			eg.storefront,
			eg.external_id,
			eg.title,
			eg.resolved_igdb_id,
			eg.is_skipped,
			eg.is_available,
			eg.is_subscription,
			(ugp.user_game_id IS NOT NULL) AS has_user_game,
			ugp.user_game_id,
			g.title AS igdb_title,
			COALESCE(
				(SELECT COUNT(*) FROM user_game_platforms o
				 WHERE o.user_game_id = ugp.user_game_id AND o.id != ugp.id),
				0
			) AS user_game_other_platform_count,
			CASE
				WHEN EXISTS (
					SELECT 1 FROM job_items ji
					WHERE ji.external_game_id = eg.id AND ji.status = 'pending_review'
				) THEN 'needs_review'
				WHEN EXISTS (
					SELECT 1 FROM job_items ji
					WHERE ji.external_game_id = eg.id AND ji.status = 'failed'
				) THEN 'failed'
				WHEN eg.is_skipped THEN 'skipped'
				WHEN eg.resolved_igdb_id IS NOT NULL THEN 'matched'
				ELSE 'unmatched'
			END AS sync_status,
			(
				SELECT ji.id FROM job_items ji
				WHERE ji.external_game_id = eg.id AND ji.status = 'failed'
				ORDER BY ji.created_at DESC
				LIMIT 1
			) AS failed_job_item_id
		FROM external_games eg
		LEFT JOIN user_game_platforms ugp ON ugp.external_game_id = eg.id
		LEFT JOIN games g ON g.id = eg.resolved_igdb_id
		WHERE eg.user_id = ? AND eg.storefront = ?
		ORDER BY eg.title ASC`,
		userID, sf,
	).Scan(ctx, &games)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list external games")
	}
	if games == nil {
		games = []externalGameResponse{}
	}
	return c.JSON(http.StatusOK, games)
}

// HandleResetSyncData handles DELETE /api/sync/:storefront/data.
// It cancels any active sync job for the storefront, then deletes all
// external_games and user_game_platforms rows for the user+storefront,
// and resets last_synced_at. Credentials are not affected.
func (h *SyncHandler) HandleResetSyncData(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validConfigStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// Cancel any active sync job for this user+storefront.
	var activeJob models.Job
	if err := h.db.NewRaw(
		`SELECT * FROM jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &activeJob); err == nil {
		if _, err := h.db.NewRaw(
			`UPDATE jobs SET status = ?, completed_at = now() WHERE id = ?`,
			models.JobStatusCancelled, activeJob.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: cancel active job failed", "err", err, "job_id", activeJob.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel active job")
		}
		if _, err := h.db.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = now()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
			activeJob.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: cancel river jobs failed", "err", err, "job_id", activeJob.ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel queued tasks")
		}
	}

	if err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// storefront = ? intentionally excludes NULL-storefront rows (manually-added entries); sync-created rows always carry an explicit storefront.
		if _, err := tx.NewRaw(
			`DELETE FROM user_game_platforms
			 WHERE storefront = ? AND user_game_id IN (SELECT id FROM user_games WHERE user_id = ?)`,
			sf, userID,
		).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`DELETE FROM external_games WHERE user_id = ? AND storefront = ?`,
			userID, sf,
		).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewRaw(
			`UPDATE user_sync_configs SET last_synced_at = NULL, updated_at = now()
			 WHERE user_id = ? AND storefront = ?`,
			userID, sf,
		).Exec(ctx)
		return err
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to reset sync data")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleRematchExternalGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var body struct {
		IGDBID       int    `json:"igdb_id"`
		OrphanAction string `json:"orphan_action"`
	}
	if err := c.Bind(&body); err != nil || body.IGDBID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	// Verify ownership.
	var eg models.ExternalGame
	err := h.db.NewSelect().Model(&eg).Where("id = ? AND user_id = ?", id, userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load external game")
	}

	// Find the existing user_game_platform linked to this external game.
	var ugpID, ugID string
	err = h.db.NewRaw(
		`SELECT id, user_game_id FROM user_game_platforms WHERE external_game_id = ? LIMIT 1`, id,
	).Scan(ctx, &ugpID, &ugID)
	platformFound := err == nil

	if platformFound {
		// Count other platforms on the same user_game.
		var otherCount int
		if err := h.db.NewRaw(
			`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
		).Scan(ctx, &otherCount); err != nil {
			slog.Error("sync: count other platforms failed", "err", err, "user_game_id", ugID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to check platform count")
		}

		// Require orphan_action when this is the last platform.
		if otherCount == 0 && body.OrphanAction == "" {
			return echo.NewHTTPError(http.StatusConflict, "orphan_action required: game would lose its only storefront link")
		}

		// Delete the platform link.
		if _, err := h.db.NewRaw(`DELETE FROM user_game_platforms WHERE id = ?`, ugpID).Exec(ctx); err != nil {
			slog.Error("sync: delete user_game_platform failed", "err", err, "ugp_id", ugpID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove platform link")
		}

		// Apply orphan decision.
		if otherCount == 0 && body.OrphanAction == "remove" {
			if _, err := h.db.NewRaw(`DELETE FROM user_games WHERE id = ?`, ugID).Exec(ctx); err != nil {
				slog.Error("sync: delete user_game failed", "err", err, "user_game_id", ugID)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to remove game")
			}
		}
	}

	// Ensure the games row exists (FK on external_games.resolved_igdb_id).
	if _, err := h.db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		body.IGDBID, eg.Title,
	).Exec(ctx); err != nil {
		slog.Error("sync: ensure game row failed", "err", err, "igdb_id", body.IGDBID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve game")
	}

	// Update external_game.
	if _, err := h.db.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id = ?`,
		body.IGDBID, id,
	).Exec(ctx); err != nil {
		slog.Error("sync: update external_game resolution failed", "err", err, "external_game_id", id)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to update external game")
	}

	// Find the existing pending_review job_item for this external game.
	var jobItemID string
	if err := h.db.NewRaw(`
		SELECT id FROM job_items
		WHERE external_game_id = ? AND status = 'pending_review'
		ORDER BY created_at DESC
		LIMIT 1`, id,
	).Scan(ctx, &jobItemID); err != nil || jobItemID == "" {
		// No pending_review item — create a minimal one attached to the
		// most recent job for this user+storefront.
		var recentJobID string
		if err2 := h.db.NewRaw(`
			SELECT id FROM jobs
			WHERE user_id = ? AND source = ? AND job_type = 'sync'
			ORDER BY created_at DESC LIMIT 1`,
			userID, eg.Storefront,
		).Scan(ctx, &recentJobID); err2 != nil {
			slog.Error("sync: rematch — no recent job found", "user_id", userID, "storefront", eg.Storefront, "err", err2)
			return echo.NewHTTPError(http.StatusInternalServerError, "no sync job context found")
		}
		jobItemID = uuid.NewString()
		if _, err3 := h.db.NewRaw(`
			INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
			VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
			jobItemID, recentJobID, userID, eg.ExternalID, eg.Title, eg.ID,
		).Exec(ctx); err3 != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job item")
		}
	}

	if h.riverClient != nil {
		if _, err := h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: jobItemID}, nil); err != nil {
			slog.Error("sync: enqueue user_game_write failed", "err", err, "job_item_id", jobItemID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to enqueue sync item")
		}
	}

	// Resolve siblings: other external_games for the same (user, storefront, title) that are
	// still unresolved. Each gets the same IGDB ID and its own Stage 3 job.
	var siblings []struct {
		ID         string `bun:"id"`
		ExternalID string `bun:"external_id"`
		Title      string `bun:"title"`
	}
	if err := h.db.NewRaw(`
		SELECT id, external_id, title FROM external_games
		WHERE user_id = ? AND storefront = ? AND title = ?
		  AND id != ? AND resolved_igdb_id IS NULL AND is_skipped = false`,
		userID, eg.Storefront, eg.Title, id,
	).Scan(ctx, &siblings); err == nil {
		for _, sib := range siblings {
			if _, err := h.db.NewRaw(
				`UPDATE external_games SET resolved_igdb_id = ?, updated_at = now() WHERE id = ?`,
				body.IGDBID, sib.ID,
			).Exec(ctx); err != nil {
				slog.Error("sync: rematch: resolve sibling", "err", err, "sibling_id", sib.ID)
				continue
			}
			var sibItemID string
			if err := h.db.NewRaw(`
				SELECT id FROM job_items
				WHERE external_game_id = ? AND status = 'pending_review'
				ORDER BY created_at DESC LIMIT 1`, sib.ID,
			).Scan(ctx, &sibItemID); err != nil || sibItemID == "" {
				var recentJobID string
				if err2 := h.db.NewRaw(`
					SELECT id FROM jobs
					WHERE user_id = ? AND source = ? AND job_type = 'sync'
					ORDER BY created_at DESC LIMIT 1`,
					userID, eg.Storefront,
				).Scan(ctx, &recentJobID); err2 != nil {
					slog.Error("sync: rematch: sibling no recent job", "sibling_id", sib.ID, "err", err2)
					continue
				}
				sibItemID = uuid.NewString()
				if _, err3 := h.db.NewRaw(`
					INSERT INTO job_items (id, job_id, user_id, item_key, source_title, external_game_id, source_metadata, status, result, igdb_candidates, created_at)
					VALUES (?, ?, ?, ?, ?, ?, '{}', 'pending', '{}', '[]', now())`,
					sibItemID, recentJobID, userID, sib.ExternalID, sib.Title, sib.ID,
				).Exec(ctx); err3 != nil {
					slog.Error("sync: rematch: create sibling job_item", "sibling_id", sib.ID, "err", err3)
					continue
				}
			}
			if h.riverClient != nil {
				if _, err := h.riverClient.Insert(ctx, tasks.UserGameArgs{JobItemID: sibItemID}, nil); err != nil {
					slog.Error("sync: rematch: enqueue sibling Stage 3", "sibling_id", sib.ID, "err", err)
				}
			}
		}
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleRetryFailedExternalGames re-enqueues all failed job_items for external
// games belonging to this user+storefront.
func (h *SyncHandler) HandleRetryFailedExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validTriggerStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var items []struct {
		ID string `bun:"id"`
	}
	if err := h.db.NewRaw(`
		SELECT ji.id
		FROM job_items ji
		JOIN external_games eg ON eg.id = ji.external_game_id
		WHERE eg.user_id = ? AND eg.storefront = ? AND ji.status = 'failed'`,
		userID, sf,
	).Scan(ctx, &items); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to query failed items")
	}

	for _, item := range items {
		if _, err := h.db.NewRaw(
			`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL WHERE id = ?`,
			item.ID,
		).Exec(ctx); err != nil {
			slog.Error("sync: retry-failed: reset item", "id", item.ID, "err", err)
			continue
		}
		if h.riverClient != nil {
			if _, err := h.riverClient.Insert(ctx, tasks.IGDBMatchArgs{JobItemID: item.ID}, nil); err != nil {
				slog.Error("sync: retry-failed: enqueue", "id", item.ID, "err", err)
			}
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleGOGConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var body struct {
		AuthCode string `json:"auth_code"`
	}
	if err := c.Bind(&body); err != nil || body.AuthCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	tok, err := h.gogClient.ExchangeCode(c.Request().Context(), body.AuthCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("gog auth failed: %v", err))
	}

	creds := map[string]string{
		"access_token":  tok.AccessToken,
		"refresh_token": tok.RefreshToken,
		"user_id":       tok.UserID,
		"username":      tok.Username,
	}
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	ciphertext, err := h.encrypter.Encrypt(credsJSON)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	now := time.Now().UTC()

	row := &models.UserSyncConfig{
		ID:                    uuid.NewString(),
		UserID:                userID,
		Storefront:            "gog",
		Frequency:             "manual",
		StorefrontCredentials: &ciphertext,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if _, err := h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, updated_at = EXCLUDED.updated_at").
		Exec(context.Background()); err != nil {
		slog.Error("gog: persist credentials failed", "user_id", userID, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist GOG connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"username": tok.Username,
		"user_id":  tok.UserID,
	})
}

func (h *SyncHandler) HandleGOGDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if _, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
		userID,
	).Exec(context.Background()); err != nil {
		slog.Error("sync: gog disconnect failed", "err", err, "user_id", userID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect GOG")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleGetGOGConnection(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	authURL := ""
	if h.gogClient != nil {
		authURL = h.gogClient.BuildAuthURL()
	}

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).
		Where("user_id = ? AND storefront = 'gog'", userID).
		Scan(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, map[string]any{"connected": false, "auth_url": authURL})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get gog config")
	}
	if row.StorefrontCredentials == nil {
		return c.JSON(http.StatusOK, map[string]any{"connected": false, "auth_url": authURL})
	}

	plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.Warn("gog: credentials decrypt failed", "user_id", userID, "err", err)
		return c.JSON(http.StatusOK, map[string]any{"connected": true, "credentials_error": true, "auth_url": authURL})
	}
	var creds struct {
		Username string `json:"username"`
		UserID   string `json:"user_id"`
	}
	if err := json.Unmarshal(plainCreds, &creds); err != nil {
		slog.Error("gog: stored credentials are corrupted", "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"connected": true,
		"username":  creds.Username,
		"user_id":   creds.UserID,
		"auth_url":  authURL,
	})
}
