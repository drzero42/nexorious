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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/logging"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
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

// PlaystationStoreClient abstracts the PSN NPSSO exchange and account info retrieval.
type PlaystationStoreClient interface {
	GetAccountInfo(ctx context.Context, npssoToken string) (*PlaystationStoreAccountInfo, error)
}

// PlaystationStoreAccountInfo holds the account details retrieved from PSN.
type PlaystationStoreAccountInfo struct {
	OnlineID  string
	AccountID string
}

// EpicGamesStoreClient abstracts legendary CLI calls used during Epic account connection.
type EpicGamesStoreClient interface {
	// Authenticate runs legendary auth, returns account info and the resulting
	// state snapshot. The caller stores the snapshot in the DB.
	Authenticate(ctx context.Context, userID, authCode string) (*EpicGamesStoreAccountInfo, map[string]string, error)
	Cleanup(ctx context.Context, userID string) error
	Configured() bool
}

// EpicGamesStoreAccountInfo holds the Epic account details.
type EpicGamesStoreAccountInfo struct {
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
	RefreshToken string
	Username     string
}

// HumbleClient abstracts the Humble Bundle credential-verification call.
type HumbleClient interface {
	Verify(ctx context.Context, sessionCookie string) error
}

var (
	ErrInvalidNPSSOToken   = errors.New("invalid npsso token")
	ErrSteamRateLimited    = errors.New("steam rate limited")
	ErrSteamNetwork        = errors.New("steam network error")
	ErrInvalidHumbleCookie = errors.New("invalid humble session cookie")
)

// supportedStorefronts is the ordered list of storefront slugs the sync API
// accepts. validStorefronts is its set form, derived from the same source so the
// two can never drift apart.
var supportedStorefronts = []string{"steam", "playstation-store", "epic-games-store", "gog", "humble-bundle"}

var validStorefronts = func() map[string]bool {
	m := make(map[string]bool, len(supportedStorefronts))
	for _, sf := range supportedStorefronts {
		m[sf] = true
	}
	return m
}()

var (
	steamIDRegex  = regexp.MustCompile(`^7656119[0-9]{10}$`)
	steamKeyRegex = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)
)

type syncConfigItem struct {
	ID           string     `json:"id"`
	Storefront   string     `json:"storefront"`
	Frequency    string     `json:"frequency"`
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
	Storefront        string     `json:"storefront"`
	IsSyncing         bool       `json:"is_syncing"`
	LastSyncedAt      *time.Time `json:"last_synced_at"`
	ActiveJobID       *string    `json:"active_job_id"`
	ExternalGameCount int        `json:"external_game_count"`
}

type externalGameResponse struct {
	ID                         string   `bun:"id"                             json:"id"`
	Storefront                 string   `bun:"storefront"                     json:"storefront"`
	ExternalID                 string   `bun:"external_id"                    json:"external_id"`
	Title                      string   `bun:"title"                          json:"title"`
	ResolvedIGDBID             *int32   `bun:"resolved_igdb_id"               json:"resolved_igdb_id"`
	IsSkipped                  bool     `bun:"is_skipped"                     json:"is_skipped"`
	IsAvailable                bool     `bun:"is_available"                   json:"is_available"`
	IsSubscription             bool     `bun:"is_subscription"                json:"is_subscription"`
	HasUserGame                bool     `bun:"has_user_game"                  json:"has_user_game"`
	UserGameID                 *string  `bun:"user_game_id"                   json:"user_game_id"`
	IGDBTitle                  *string  `bun:"igdb_title"                     json:"igdb_title"`
	UserGameOtherPlatformCount int      `bun:"user_game_other_platform_count" json:"user_game_other_platform_count"`
	SyncStatus                 string   `bun:"sync_status"                    json:"sync_status"`
	FailedJobItemID            *string  `bun:"failed_job_item_id"             json:"failed_job_item_id"`
	PlatformsCSV               string   `bun:"platforms_csv"                  json:"-"`
	Platforms                  []string `bun:"-"                              json:"platforms"`
	StoreLink                  *string  `bun:"store_link"                     json:"-"`
	StoreURL                   *string  `bun:"-"                              json:"store_url"`
}

type steamConnectResponse struct {
	Valid         bool    `json:"valid"`
	SteamUsername *string `json:"steam_username"`
	Error         *string `json:"error"`
}

type psnConnectResponse struct {
	Success   bool   `json:"success"`
	OnlineID  string `json:"online_id"`
	AccountID string `json:"account_id"`
	Message   string `json:"message"`
}

type psnConnectionResponse struct {
	IsConfigured     bool   `json:"is_configured"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	OnlineID         string `json:"online_id,omitempty"`
}

type humbleConnectResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type humbleStatusResponse struct {
	IsConfigured     bool `json:"is_configured"`
	CredentialsError bool `json:"credentials_error,omitempty"`
}

type steamConnectionResponse struct {
	Connected        bool   `json:"connected"`
	CredentialsError bool   `json:"credentials_error,omitempty"`
	Username         string `json:"username,omitempty"`
}

// SyncHandler handles sync configuration, trigger, and status endpoints.
type SyncHandler struct {
	db                     *bun.DB
	encrypter              *crypto.Encrypter
	riverClient            *river.Client[pgx.Tx]
	steamClient            SteamClient
	playstationStoreClient PlaystationStoreClient
	epicGamesStoreClient   EpicGamesStoreClient
	gogClient              GOGClient
	humbleClient           HumbleClient
}

// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(encrypter *crypto.Encrypter, db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamClient, playstationStore PlaystationStoreClient, epicGamesStore EpicGamesStoreClient, gog GOGClient, humble HumbleClient) *SyncHandler {
	return &SyncHandler{encrypter: encrypter, db: db, riverClient: riverClient, steamClient: steam, playstationStoreClient: playstationStore, epicGamesStoreClient: epicGamesStore, gogClient: gog, humbleClient: humble}
}

// RegisterRoutes registers all sync routes on the given group.
// Static-segment routes are registered before parameterised routes to avoid conflicts.
func (h *SyncHandler) RegisterRoutes(g *echo.Group) {
	g.PUT("/steam/connection", h.HandleSteamConnect)
	g.GET("/steam/connection", h.HandleGetSteamConnection)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.PUT("/playstation-store/connection", h.HandlePlaystationStoreConnect)
	g.GET("/playstation-store/connection", h.HandleGetPlaystationStoreConnection)
	g.DELETE("/playstation-store/connection", h.HandlePlaystationStoreDisconnect)
	g.PUT("/epic-games-store/connection", h.HandleEpicGamesStoreConnect)
	g.DELETE("/epic-games-store/connection", h.HandleEpicGamesStoreDisconnect)
	g.GET("/epic-games-store/connection", h.HandleGetEpicGamesStoreConnection)
	g.PUT("/gog/connection", h.HandleGOGConnect)
	g.GET("/gog/connection", h.HandleGetGOGConnection)
	g.DELETE("/gog/connection", h.HandleGOGDisconnect)
	g.PUT("/humble-bundle/connection", h.HandleHumbleConnect)
	g.GET("/humble-bundle/connection", h.HandleGetHumbleConnection)
	g.DELETE("/humble-bundle/connection", h.HandleHumbleDisconnect)
	g.GET("/ignored", h.HandleListIgnored)
	g.POST("/ignored/:id", h.HandleSkipGame)
	g.DELETE("/ignored/:id", h.HandleUnskipGame)
	// "external-games" is a static prefix — must be registered before /:storefront (POST)
	// per Echo v5 route ordering rules.
	g.POST("/external-games/:id/rematch", h.HandleRematchExternalGame)
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

// persistStorefrontCredentials marshals creds to JSON, encrypts it, and upserts
// the ciphertext into user_sync_configs for the given storefront, clearing the
// credentials_error flag. It is the shared persistence path for all storefront
// connect handlers.
func (h *SyncHandler) persistStorefrontCredentials(ctx context.Context, userID, sf string, creds any) error {
	credsJSON, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	ciphertext, err := h.encrypter.Encrypt(credsJSON)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	row := &models.UserSyncConfig{
		ID:                    uuid.NewString(),
		UserID:                userID,
		Storefront:            sf,
		Frequency:             "manual",
		StorefrontCredentials: &ciphertext,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	_, err = h.db.NewInsert().Model(row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET storefront_credentials = EXCLUDED.storefront_credentials, credentials_error = false, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// clearStorefrontCredentials nulls out the stored credentials for a storefront.
// It is idempotent: clearing an absent or already-disconnected storefront is a
// no-op that returns no error.
func (h *SyncHandler) clearStorefrontCredentials(ctx context.Context, userID, sf string) error {
	_, err := h.db.NewRaw(
		`UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = ?`,
		userID, sf,
	).Exec(ctx)
	return err
}

// disconnectStorefront is the shared handler body for the simple
// DELETE /sync/<sf>/connection endpoints: authenticate, clear credentials, and
// respond 204. Storefronts that need extra teardown (e.g. Epic's legendary
// cleanup) call clearStorefrontCredentials directly instead.
func (h *SyncHandler) disconnectStorefront(c *echo.Context, sf string) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.clearStorefrontCredentials(context.Background(), userID, sf); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: disconnect failed", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect "+sf)
	}
	return c.NoContent(http.StatusNoContent)
}

// connectionStatus is the decoded result of loadConnectionStatus, describing the
// stored-credential state for a storefront independent of its response shape.
type connectionStatus struct {
	Connected        bool   // a row with non-nil credentials exists
	CredentialsError bool   // decrypt failed, or the row's credentials_error flag is set
	Plaintext        []byte // decrypted credentials; nil unless Connected and decrypt succeeded
}

// loadConnectionStatus loads and decrypts the stored credentials for a
// storefront. A non-nil error signals a genuine database failure (callers map it
// to 500); every other state is reported via the returned connectionStatus.
// Callers decode Plaintext into their storefront-specific struct themselves.
func (h *SyncHandler) loadConnectionStatus(ctx context.Context, userID, sf string) (connectionStatus, error) {
	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).
		Where("user_id = ? AND storefront = ?", userID, sf).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return connectionStatus{}, nil
	}
	if err != nil {
		return connectionStatus{}, err
	}
	if row.StorefrontCredentials == nil {
		return connectionStatus{}, nil
	}
	plain, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
	if err != nil {
		slog.WarnContext(ctx, "sync: credentials decrypt failed", logging.KeySource, sf, logging.KeyUserID, userID, logging.KeyErr, err, logging.Cat(logging.CategoryAuth))
		return connectionStatus{Connected: true, CredentialsError: true}, nil
	}
	return connectionStatus{Connected: true, CredentialsError: row.CredentialsError, Plaintext: plain}, nil
}

// serveConnectionStatus is the shared handler body for the GET
// /sync/<sf>/connection endpoints: authenticate, load the stored-credential
// state, and respond with the storefront-specific shape. notConnected and
// credentialsError are the fixed responses for the no-credentials and
// undecodable-credentials states; respond builds the connected response from
// the decrypted plaintext (a returned error is treated as corrupted
// credentials → 500). Storefronts with extra preconditions (e.g. Epic's
// legendary-configured check) run them before delegating here.
func (h *SyncHandler) serveConnectionStatus(c *echo.Context, sf string, notConnected, credentialsError any, respond func(plaintext []byte, credentialsError bool) (any, error)) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	status, err := h.loadConnectionStatus(context.Background(), userID, sf)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get "+sf+" config")
	}
	if !status.Connected {
		return c.JSON(http.StatusOK, notConnected)
	}
	if status.Plaintext == nil {
		return c.JSON(http.StatusOK, credentialsError)
	}
	resp, err := respond(status.Plaintext, status.CredentialsError)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), sf+": stored credentials are corrupted", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
	return c.JSON(http.StatusOK, resp)
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
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var row models.UserSyncConfig
	err := h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		return c.JSON(http.StatusOK, syncConfigResponse{
			ID: uuid.NewString(), UserID: userID, Storefront: sf,
			Frequency: "manual", IsConfigured: false,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get config")
	}
	return c.JSON(http.StatusOK, syncConfigResponse{
		ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
		Frequency:    row.Frequency,
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
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}

	var body struct {
		Frequency *string `json:"frequency"`
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
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load config")
	}

	if body.Frequency != nil {
		row.Frequency = *body.Frequency
	}
	row.UpdatedAt = now

	_, err = h.db.NewInsert().Model(&row).
		On("CONFLICT (user_id, storefront) DO UPDATE SET frequency = EXCLUDED.frequency, updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	return c.JSON(http.StatusOK, syncConfigResponse{
		ID: row.ID, UserID: row.UserID, Storefront: row.Storefront,
		Frequency:    row.Frequency,
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
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	now := time.Now().UTC()
	jobID := uuid.NewString()
	// The active-job guard and the insert run in one transaction fronted by a
	// transaction-scoped advisory lock on the (sync, storefront, user) dedup key:
	// without it the bare SELECT+INSERT races under READ COMMITTED and two
	// concurrent triggers both pass the guard and both create an active job.
	var duplicate bool
	txErr := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if e := tasks.AcquireJobDedupLock(ctx, tx, "sync", sf, userID); e != nil {
			return e
		}
		var existingID string
		e := tx.NewRaw(
			`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
			userID, sf,
		).Scan(ctx, &existingID)
		if e == nil {
			duplicate = true
			return nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		// priority is TEXT in the jobs table with default 'high'.
		_, e = tx.NewRaw(
			`INSERT INTO jobs (id, user_id, job_type, source, status, priority, created_at) VALUES (?, ?, 'sync', ?, 'pending', 'high', ?)`,
			jobID, userID, sf, now,
		).Exec(ctx)
		return e
	})
	if txErr != nil {
		slog.ErrorContext(c.Request().Context(), "sync: create job", logging.KeyErr, txErr, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job")
	}
	if duplicate {
		return echo.NewHTTPError(http.StatusConflict, "sync already in progress")
	}

	if h.riverClient != nil {
		if _, err := h.riverClient.Insert(ctx, tasks.DispatchSyncArgs{
			JobID: jobID, UserID: userID, Storefront: sf,
		}, nil); err != nil {
			slog.ErrorContext(c.Request().Context(), "sync: enqueue dispatch failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
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
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// A failed read answers 500, never an idle store: the store page treats a
	// cleared active job as a finished sync.
	statusReadFailed := func(what string, err error) error {
		slog.ErrorContext(c.Request().Context(), "sync: status read failed", "what", what, logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read sync status")
	}

	var activeJobID *string
	var jobID string
	err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = ? AND status IN ('pending', 'processing') LIMIT 1`,
		userID, sf,
	).Scan(ctx, &jobID)
	switch {
	case err == nil:
		activeJobID = &jobID
	case !errors.Is(err, sql.ErrNoRows):
		return statusReadFailed("active job", err)
	}

	var lastSyncedAt *time.Time
	var cfg models.UserSyncConfig
	err = h.db.NewSelect().Model(&cfg).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
	switch {
	case err == nil:
		lastSyncedAt = cfg.LastSyncedAt
	case !errors.Is(err, sql.ErrNoRows):
		return statusReadFailed("sync config", err)
	}

	var externalGameCount int
	if err := h.db.NewSelect().
		TableExpr("external_games").
		ColumnExpr("COUNT(*)").
		Where("user_id = ? AND storefront = ? AND is_available = true", userID, sf).
		Scan(ctx, &externalGameCount); err != nil {
		return statusReadFailed("external game count", err)
	}

	return c.JSON(http.StatusOK, syncStatusResponse{
		Storefront:        sf,
		IsSyncing:         activeJobID != nil,
		LastSyncedAt:      lastSyncedAt,
		ActiveJobID:       activeJobID,
		ExternalGameCount: externalGameCount,
	})
}

func (h *SyncHandler) HandleSteamConnect(c *echo.Context) error {
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
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("invalid_steam_id")})
	}
	if !steamKeyRegex.MatchString(req.WebAPIKey) {
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("invalid_api_key")})
	}

	summary, err := h.steamClient.GetPlayerSummaries(c.Request().Context(), req.WebAPIKey, req.SteamID)
	if err != nil {
		if errors.Is(err, ErrSteamRateLimited) {
			return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("rate_limited")})
		}
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("network_error")})
	}
	if summary == nil {
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("invalid_steam_id")})
	}
	if summary.CommunityVisibilityState != 3 {
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: errStr("private_profile")})
	}

	creds := map[string]string{
		"web_api_key":  req.WebAPIKey,
		"steam_id":     req.SteamID,
		"display_name": summary.PersonaName,
	}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "steam", creds); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: persist steam credentials failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Steam connection")
	}

	name := summary.PersonaName
	return c.JSON(http.StatusOK, steamConnectResponse{Valid: true, SteamUsername: &name})
}

func (h *SyncHandler) HandleSteamDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "steam")
}

func (h *SyncHandler) HandleGetSteamConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "steam",
		steamConnectionResponse{Connected: false},
		steamConnectionResponse{Connected: true, CredentialsError: true},
		func(plaintext []byte, credentialsError bool) (any, error) {
			var creds struct {
				DisplayName string `json:"display_name"`
			}
			if err := json.Unmarshal(plaintext, &creds); err != nil {
				return nil, err
			}
			return steamConnectionResponse{
				Connected:        true,
				Username:         creds.DisplayName,
				CredentialsError: credentialsError,
			}, nil
		})
}

func (h *SyncHandler) HandlePlaystationStoreConnect(c *echo.Context) error {
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

	info, err := h.playstationStoreClient.GetAccountInfo(c.Request().Context(), req.NpssoToken)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid_npsso_token")
	}

	// Store only what is read back: the token (sync adapter factory) and the
	// online ID (status handler). Legacy rows may carry extra fields
	// (account_id, region, is_verified, token_expired_at); decoders ignore them.
	creds := map[string]any{
		"npsso_token": req.NpssoToken,
		"online_id":   info.OnlineID,
	}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "playstation-store", creds); err != nil {
		slog.ErrorContext(c.Request().Context(), "psn: persist storefront credentials failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist PSN connection")
	}

	return c.JSON(http.StatusOK, psnConnectResponse{
		Success:   true,
		OnlineID:  info.OnlineID,
		AccountID: info.AccountID,
		Message:   "PSN connected successfully",
	})
}

func (h *SyncHandler) HandleGetPlaystationStoreConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "playstation-store",
		psnConnectionResponse{},
		psnConnectionResponse{IsConfigured: true, CredentialsError: true},
		func(plaintext []byte, credentialsError bool) (any, error) {
			var creds struct {
				OnlineID string `json:"online_id"`
			}
			if err := json.Unmarshal(plaintext, &creds); err != nil {
				return nil, err
			}
			return psnConnectionResponse{
				IsConfigured:     true,
				CredentialsError: credentialsError,
				OnlineID:         creds.OnlineID,
			}, nil
		})
}

func (h *SyncHandler) HandlePlaystationStoreDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "playstation-store")
}

// HandleHumbleConnect is PUT /sync/humble-bundle/connection: it verifies the
// pasted session cookie against the Humble order API and, on success, stores it
// (create-or-replace). A rejected cookie returns 400 before anything is stored.
func (h *SyncHandler) HandleHumbleConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	var req struct {
		SessionCookie string `json:"session_cookie"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	req.SessionCookie = strings.TrimSpace(req.SessionCookie)
	if req.SessionCookie == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_cookie is required")
	}

	if err := h.humbleClient.Verify(c.Request().Context(), req.SessionCookie); err != nil {
		if errors.Is(err, ErrInvalidHumbleCookie) {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid_session_cookie")
		}
		slog.ErrorContext(c.Request().Context(), "humble: verify failed", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadGateway, "could not reach Humble Bundle")
	}

	creds := map[string]any{"session_cookie": req.SessionCookie}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "humble-bundle", creds); err != nil {
		slog.ErrorContext(c.Request().Context(), "humble: persist storefront credentials failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Humble Bundle connection")
	}

	return c.JSON(http.StatusOK, humbleConnectResponse{Success: true, Message: "Humble Bundle connected successfully"})
}

// HandleGetHumbleConnection is GET /sync/humble-bundle/connection.
func (h *SyncHandler) HandleGetHumbleConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "humble-bundle",
		humbleStatusResponse{},
		humbleStatusResponse{IsConfigured: true, CredentialsError: true},
		func(_ []byte, credentialsError bool) (any, error) {
			return humbleStatusResponse{IsConfigured: true, CredentialsError: credentialsError}, nil
		})
}

// HandleHumbleDisconnect is DELETE /sync/humble-bundle/connection.
func (h *SyncHandler) HandleHumbleDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "humble-bundle")
}

// HandleEpicGamesStoreConnect exchanges an Epic auth code via legendary and stores the
// resulting legendary state snapshot in the DB.
func (h *SyncHandler) HandleEpicGamesStoreConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if h.epicGamesStoreClient == nil || !h.epicGamesStoreClient.Configured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Epic sync not configured (LEGENDARY_WORK_DIR unset)")
	}

	var body struct {
		AuthCode string `json:"auth_code"`
	}
	if err := c.Bind(&body); err != nil || body.AuthCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	info, snapshot, err := h.epicGamesStoreClient.Authenticate(c.Request().Context(), userID, body.AuthCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("epic auth failed: %v", err))
	}
	if info == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "epic: user.json missing after auth")
	}

	if err := h.persistStorefrontCredentials(context.Background(), userID, "epic-games-store", snapshot); err != nil {
		slog.ErrorContext(c.Request().Context(), "epic: persist storefront credentials failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Epic connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"display_name": info.DisplayName,
		"account_id":   info.AccountID,
	})
}

// HandleEpicGamesStoreDisconnect clears the user's Epic credentials and snapshot, and
// removes the legendary working directory.
func (h *SyncHandler) HandleEpicGamesStoreDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := context.Background()
	if err := h.clearStorefrontCredentials(ctx, userID, "epic-games-store"); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: epic disconnect failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Epic")
	}
	if h.epicGamesStoreClient != nil {
		if err := h.epicGamesStoreClient.Cleanup(ctx, userID); err != nil {
			slog.ErrorContext(c.Request().Context(), "sync: epic cleanup failed", logging.KeyErr, err, logging.Cat(logging.CategoryExternalAPI))
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleGetEpicGamesStoreConnection returns the current Epic connection status for the
// authenticated user.
func (h *SyncHandler) HandleGetEpicGamesStoreConnection(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	if h.epicGamesStoreClient == nil || !h.epicGamesStoreClient.Configured() {
		return c.JSON(http.StatusOK, map[string]any{
			"connected": false,
			"disabled":  true,
			"reason":    "legendary_not_configured",
		})
	}

	return h.serveConnectionStatus(c, "epic-games-store",
		map[string]any{"connected": false, "disabled": false},
		map[string]any{"connected": true, "credentials_error": true, "disabled": false},
		func(plaintext []byte, credentialsError bool) (any, error) {
			// Epic persists the raw legendary snapshot: a map[relPath]content where the
			// account details live inside user.json (field displayName), not as
			// top-level keys. Decode the snapshot, then parse user.json.
			var snapshot map[string]string
			if err := json.Unmarshal(plaintext, &snapshot); err != nil {
				return nil, err
			}
			var creds struct {
				DisplayName string `json:"displayName"`
			}
			if userJSON, ok := snapshot["user.json"]; ok {
				if err := json.Unmarshal([]byte(userJSON), &creds); err != nil {
					return nil, fmt.Errorf("user.json: %w", err)
				}
			}
			return map[string]any{
				"connected":         true,
				"disabled":          false,
				"credentials_error": credentialsError,
				"display_name":      creds.DisplayName,
			}, nil
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

	var ownerRow struct {
		UserID string `bun:"user_id"`
		Title  string `bun:"title"`
	}
	err := h.db.NewRaw(`SELECT user_id, title FROM external_games WHERE id = ?`, id).Scan(ctx, &ownerRow)
	if errors.Is(err, sql.ErrNoRows) || ownerRow.UserID != userID {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	if _, err := h.db.NewRaw(
		`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: skip game failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}

	// Cascade skip to children.
	var childIDs []string
	if err := h.db.NewRaw(
		`SELECT id FROM external_games WHERE parent_id = ?`, id,
	).Scan(ctx, &childIDs); err == nil {
		for _, childID := range childIDs {
			if _, err := h.db.NewRaw(
				`UPDATE external_games SET is_skipped = true, updated_at = now() WHERE id = ?`, childID,
			).Exec(ctx); err != nil {
				slog.ErrorContext(c.Request().Context(), "sync: skip game: cascade child failed", logging.KeyErr, err, "child_id", childID, logging.Cat(logging.CategoryDB))
				continue
			}
			var childItem struct {
				ID    string `bun:"id"`
				JobID string `bun:"job_id"`
			}
			if err := h.db.NewRaw(`
				SELECT id, job_id FROM job_items
				WHERE external_game_id = ? AND status IN ('pending_review', 'pending')
				ORDER BY created_at DESC
				LIMIT 1`, childID,
			).Scan(ctx, &childItem); err == nil {
				if _, err := h.db.NewRaw(
					`UPDATE job_items SET status = 'skipped', processed_at = now() WHERE id = ?`,
					childItem.ID,
				).Exec(ctx); err != nil {
					slog.ErrorContext(c.Request().Context(), "sync: skip game: cascade child job_item", logging.KeyErr, err, logging.KeyJobItemID, childItem.ID, logging.Cat(logging.CategoryDB))
				} else {
					tasks.SyncCheckJobCompletion(ctx, h.db, h.riverClient, childItem.JobID)
				}
			}
		}
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
			slog.ErrorContext(c.Request().Context(), "sync: skip game: mark job_item skipped", logging.KeyErr, err, logging.KeyJobItemID, jobItemRow.ID, logging.Cat(logging.CategoryDB))
		} else {
			if _, err := h.db.NewRaw(
				`INSERT INTO changes (id, job_id, user_id, external_game_id, change_type, title, created_at)
				 VALUES (?, ?, ?, ?, 'skipped', ?, now())`,
				uuid.NewString(), jobItemRow.JobID, userID, id, ownerRow.Title,
			).Exec(ctx); err != nil {
				slog.ErrorContext(c.Request().Context(), "sync: skip game: insert sync_change (skipped)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			}
			tasks.SyncCheckJobCompletion(ctx, h.db, h.riverClient, jobItemRow.JobID)
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
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// One row per top-level game. Status comes from the newest item (see
	// sync_view.go). The orphan fields are per family, matching rematch's
	// orphan check: links of the game or its Child games, "other" meaning
	// outside the family; with the family on several user games, the smallest
	// count among them (zero: at least one would be orphaned), and the user game
	// the game's own links are on, else the lowest id.
	var games []externalGameResponse
	err := h.db.NewRaw(`
		SELECT id, storefront, external_id, title, resolved_igdb_id, is_skipped, is_available, is_subscription,
		       has_user_game, user_game_id, igdb_title, user_game_other_platform_count,
		       CASE
		           WHEN needs_review THEN 'needs_review'
		           WHEN is_skipped THEN 'skipped'
		           WHEN failed THEN 'failed'
		           WHEN resolved_igdb_id IS NOT NULL THEN 'matched'
		           ELSE 'unmatched'
		       END AS sync_status,
		       CASE WHEN failed AND NOT needs_review AND NOT is_skipped THEN newest_item_id END AS failed_job_item_id,
		       platforms_csv, store_link
		FROM (
			SELECT
				eg.id, eg.storefront, eg.external_id, eg.title, eg.resolved_igdb_id,
				eg.is_skipped, eg.is_available, eg.is_subscription, eg.store_link,
				g.title AS igdb_title,
				COALESCE(fl.has_user_game, false) AS has_user_game,
				fl.user_game_id,
				COALESCE(fl.other_count, 0) AS user_game_other_platform_count,
				ni.id AS newest_item_id,
				`+syncNeedsReview+` AS needs_review,
				`+syncFailed+` AS failed,
				`+syncInFlight+` AS in_flight,
				COALESCE(
					(SELECT string_agg(DISTINCT egp.platform, ',' ORDER BY egp.platform)
					 FROM external_game_platforms egp
					 WHERE egp.external_game_id = eg.id
					    OR egp.external_game_id IN (
					        SELECT id FROM external_games WHERE parent_id = eg.id
					    )
					),
					''
				) AS platforms_csv
			FROM external_games eg
			LEFT JOIN games g ON g.id = eg.resolved_igdb_id
			`+syncNewestItemJoin+`
			LEFT JOIN LATERAL (
				SELECT true AS has_user_game,
				       COALESCE(MIN(ug_id) FILTER (WHERE own), MIN(ug_id)) AS user_game_id,
				       MIN(others) AS other_count
				FROM (
					SELECT l.user_game_id AS ug_id,
					       bool_or(l.external_game_id = eg.id) AS own,
					       (SELECT COUNT(*) FROM user_game_platforms o
					        WHERE o.user_game_id = l.user_game_id
					          AND (o.external_game_id IS NULL
					               OR (o.external_game_id <> eg.id
					                   AND o.external_game_id NOT IN (
					                       SELECT c.id FROM external_games c WHERE c.parent_id = eg.id)))
					       ) AS others
					FROM user_game_platforms l
					WHERE l.external_game_id = eg.id
					   OR l.external_game_id IN (SELECT c.id FROM external_games c WHERE c.parent_id = eg.id)
					GROUP BY l.user_game_id
				) per_user_game
				HAVING COUNT(*) > 0
			) fl ON true
			WHERE eg.user_id = ? AND eg.storefront = ?
			  AND eg.parent_id IS NULL
		) v
		WHERE NOT in_flight
		ORDER BY title ASC`,
		userID, sf,
	).Scan(ctx, &games)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list external games")
	}
	if games == nil {
		games = []externalGameResponse{}
	}
	for i := range games {
		if games[i].PlatformsCSV != "" {
			games[i].Platforms = strings.Split(games[i].PlatformsCSV, ",")
		} else {
			games[i].Platforms = []string{}
		}
		if games[i].StoreLink != nil {
			if url, ok := buildStoreURL(games[i].Storefront, *games[i].StoreLink); ok {
				games[i].StoreURL = &url
			}
		}
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
	if !validStorefronts[sf] {
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
			slog.ErrorContext(c.Request().Context(), "sync: cancel active job failed", logging.KeyErr, err, logging.KeyJobID, activeJob.ID, logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel active job")
		}
		if _, err := h.db.NewRaw(`
			UPDATE river_job
			SET state = 'cancelled', finalized_at = now()
			WHERE state IN ('available', 'scheduled', 'retryable', 'pending')
			  AND args->>'job_item_id' IN (SELECT id FROM job_items WHERE job_id = ?)`,
			activeJob.ID,
		).Exec(ctx); err != nil {
			slog.ErrorContext(c.Request().Context(), "sync: cancel river jobs failed", logging.KeyErr, err, logging.KeyJobID, activeJob.ID, logging.Cat(logging.CategoryDB))
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

// HandleRetryFailedExternalGames retries the storefront's games the view
// shows as failed (their newest item failed, and nothing wrote them since), and
// the Child games whose own newest item failed by the same rule. Every item is
// reset first and queued afterwards, parents before children, so a child never
// runs before its parent has been reset (#1338).
func (h *SyncHandler) HandleRetryFailedExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var itemIDs []string
	err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.NewRaw(`
			SELECT ni.id
			FROM external_games eg
			`+syncNewestItemJoin+`
			WHERE eg.user_id = ? AND eg.storefront = ? AND NOT eg.is_skipped
			  AND `+syncFailed+`
			  AND NOT `+syncNeedsReview+`
			ORDER BY eg.parent_id IS NOT NULL, ni.id`,
			userID, sf,
		).Scan(ctx, &itemIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("query failed items: %w", err)
		}
		if len(itemIDs) == 0 {
			return nil
		}
		if _, err := tx.NewRaw(
			`UPDATE job_items SET status = 'pending', error_message = NULL, processed_at = NULL
			 WHERE id IN (?) AND status = 'failed'`, bun.List(itemIDs),
		).Exec(ctx); err != nil {
			return fmt.Errorf("reset failed items: %w", err)
		}
		return nil
	})
	if err != nil {
		slog.ErrorContext(reqCtx, "sync: retry-failed", logging.KeySource, sf, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retry failed items")
	}
	for _, id := range itemIDs {
		if err := tasks.EnqueueOrFail(ctx, h.db, h.riverClient, id, tasks.IGDBMatchArgs{JobItemID: id}); err != nil {
			slog.ErrorContext(reqCtx, "sync: retry-failed: enqueue", logging.KeyJobItemID, id, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
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

	code, err := gogsvc.ParseAuthCode(body.AuthCode)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	tok, err := h.gogClient.ExchangeCode(c.Request().Context(), code)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("gog auth failed: %v", err))
	}

	creds := map[string]string{
		"refresh_token": tok.RefreshToken,
		"username":      tok.Username,
	}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "gog", creds); err != nil {
		slog.ErrorContext(c.Request().Context(), "gog: persist credentials failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist GOG connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"username": tok.Username,
	})
}

func (h *SyncHandler) HandleGOGDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "gog")
}

func (h *SyncHandler) HandleGetGOGConnection(c *echo.Context) error {
	authURL := ""
	if h.gogClient != nil {
		authURL = h.gogClient.BuildAuthURL()
	}

	return h.serveConnectionStatus(c, "gog",
		map[string]any{"connected": false, "auth_url": authURL},
		map[string]any{"connected": true, "credentials_error": true, "auth_url": authURL},
		func(plaintext []byte, credentialsError bool) (any, error) {
			var creds struct {
				Username string `json:"username"`
			}
			if err := json.Unmarshal(plaintext, &creds); err != nil {
				return nil, err
			}
			return map[string]any{
				"connected":         true,
				"credentials_error": credentialsError,
				"username":          creds.Username,
				"auth_url":          authURL,
			}, nil
		})
}
