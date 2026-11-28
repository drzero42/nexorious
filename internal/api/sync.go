package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/storefront"
	"github.com/drzero42/nexorious/internal/usergame"
	"github.com/drzero42/nexorious/internal/worker/tasks"
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
	// EditionsJSON is the entry's editions as the list query aggregates them;
	// Editions is what the client gets, with store_url built (#1452).
	EditionsJSON []byte                      `bun:"editions_json" json:"-"`
	Editions     []externalGameEditionResult `bun:"-"             json:"editions"`
}

// externalGameEditionResult is one edition of a storefront list entry, so a
// client can name it for a decision on that edition alone (#1452).
type externalGameEditionResult struct {
	ID          string   `json:"id"`
	ExternalID  string   `json:"external_id"`
	Platforms   []string `json:"platforms"`
	StoreLink   *string  `json:"store_link,omitempty"`
	StoreURL    *string  `json:"store_url"`
	IsAvailable bool     `json:"is_available"`
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
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	lc          *joblifecycle.Module
	connections *storefront.Connections
	igdbClient  *igdb.Client
}

// SyncDeps holds the dependencies of a SyncHandler.
type SyncDeps struct {
	DB *bun.DB
	// River inserts the immediate metadata fetch after a rematch or an
	// unskip, which is not Job work.
	River *river.Client[pgx.Tx]
	// Lifecycle owns the sync Jobs.
	Lifecycle *joblifecycle.Module
	// Connections owns the storefront Connections, and reaches the
	// storefronts through the clients it was built with. A client may be
	// left nil as long as no route that reaches that storefront is called.
	Connections *storefront.Connections
	// IGDB, nil or unconfigured: a Rematch stubs with the storefront title.
	IGDB *igdb.Client
}

// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(d SyncDeps) *SyncHandler {
	return &SyncHandler{
		db:          d.DB,
		riverClient: d.River,
		lc:          d.Lifecycle,
		connections: d.Connections,
		igdbClient:  d.IGDB,
	}
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
	g.POST("/amazon-games/connection/login", h.HandleAmazonGamesLoginStart)
	g.PUT("/amazon-games/connection", h.HandleAmazonGamesConnect)
	g.GET("/amazon-games/connection", h.HandleGetAmazonGamesConnection)
	g.DELETE("/amazon-games/connection", h.HandleAmazonGamesDisconnect)
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

// disconnectStorefront is the shared handler body for the simple
// DELETE /sync/<sf>/connection endpoints: authenticate, disconnect, and
// respond 204. Storefronts that need extra teardown (Epic's legendary
// cleanup, Amazon's device sign-out) have their own disconnect instead.
func (h *SyncHandler) disconnectStorefront(c *echo.Context, sf string) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.connections.Disconnect(context.Background(), userID, sf); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: disconnect failed", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect "+sf)
	}
	return c.NoContent(http.StatusNoContent)
}

// storefrontParam is the :storefront path parameter, or a 400 when it names
// no storefront.
func storefrontParam(c *echo.Context) (string, error) {
	sf := c.Param("storefront")
	if !storefront.Known(sf) {
		return "", echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	return sf, nil
}

// serveConnectionStatus is the shared handler body for the GET
// /sync/<sf>/connection endpoints: authenticate, load the Connection's status,
// and respond with the storefront-specific shape. notConnected and
// credentialsError are the fixed responses for no stored sign-in and one that
// does not decrypt; respond builds the response for the rest. A stored
// sign-in that does not decode answers 500. Storefronts with extra
// preconditions (e.g. Epic's legendary-configured check) run them before
// delegating here.
func (h *SyncHandler) serveConnectionStatus(c *echo.Context, sf string, notConnected, credentialsError any, respond func(st storefront.Status) any) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	st, err := h.connections.Status(c.Request().Context(), userID, sf)
	if errors.Is(err, storefront.ErrUndecodable) {
		slog.ErrorContext(c.Request().Context(), "sync: stored credentials are corrupted", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryValidation))
		return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
	}
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: load connection status failed", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get "+sf+" config")
	}
	if !st.Connected {
		return c.JSON(http.StatusOK, notConnected)
	}
	if st.Undecryptable {
		return c.JSON(http.StatusOK, credentialsError)
	}
	return c.JSON(http.StatusOK, respond(st))
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
	for _, sf := range storefront.Slugs() {
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
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	ctx := context.Background()

	var row models.UserSyncConfig
	err = h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
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
	sf, err := storefrontParam(c)
	if err != nil {
		return err
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
	err = h.db.NewSelect().Model(&row).Where("user_id = ? AND storefront = ?", userID, sf).Scan(ctx)
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
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	// The module's duplicate check runs under the (sync, storefront, user)
	// lock the scheduled sync shares, so two triggers never both create one.
	created, err := h.lc.CreateWithDispatch(context.Background(), joblifecycle.NewJob{
		UserID: userID, JobType: models.JobTypeSync, Source: sf, Priority: models.JobPriorityHigh,
	})
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: create job", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create job")
	}
	if !created.Created {
		return echo.NewHTTPError(http.StatusConflict, "sync already in progress")
	}
	jobID := created.JobID

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
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// A failed read answers 500, never an idle store: the store page treats a
	// cleared active job as an ended sync.
	statusReadFailed := func(what string, err error) error {
		slog.ErrorContext(c.Request().Context(), "sync: status read failed", "what", what, logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read sync status")
	}

	var activeJobID *string
	var jobID string
	err = h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND job_type = ? AND source = ? AND status IN (?) LIMIT 1`,
		userID, models.JobTypeSync, sf, bun.List(models.ActiveJobStatuses()),
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

	// One per set of editions with an available edition (ADR 0007, #1438), so
	// "N games" counts games, as the storefront list shows them.
	var externalGameCount int
	if err := h.db.NewRaw(
		`SELECT COUNT(*) FROM (
			SELECT 1 FROM external_games eg
			WHERE eg.user_id = ? AND eg.storefront = ? AND eg.is_available
			GROUP BY `+externalgame.EditionKey("eg")+`
		) sets`,
		userID, sf,
	).Scan(ctx, &externalGameCount); err != nil {
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

// HandleSteamConnect is PUT /sync/steam/connection. A sign-in Steam rejects
// answers 200 with valid false and an error code, not an HTTP error.
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

	name, err := h.connections.ConnectSteam(c.Request().Context(), userID, req.SteamID, req.WebAPIKey)
	if err != nil {
		code, ok := steamConnectErrorCode(err)
		if !ok {
			slog.ErrorContext(c.Request().Context(), "sync: persist steam credentials failed", logging.KeyErr, err, logging.KeySource, "steam", logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Steam connection")
		}
		return c.JSON(http.StatusOK, steamConnectResponse{Valid: false, Error: &code})
	}
	return c.JSON(http.StatusOK, steamConnectResponse{Valid: true, SteamUsername: &name})
}

// steamConnectErrorCode is the error code PUT /sync/steam/connection answers
// a rejected sign-in with; false for an error that is not a rejection.
func steamConnectErrorCode(err error) (string, bool) {
	switch {
	case errors.Is(err, storefront.ErrInvalidSteamID):
		return "invalid_steam_id", true
	case errors.Is(err, storefront.ErrInvalidSteamAPIKey):
		return "invalid_api_key", true
	case errors.Is(err, storefront.ErrSteamPrivateProfile):
		return "private_profile", true
	case errors.Is(err, storefront.ErrSteamRateLimited):
		return "rate_limited", true
	case errors.Is(err, storefront.ErrSteamUnavailable):
		return "network_error", true
	}
	return "", false
}

func (h *SyncHandler) HandleSteamDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "steam")
}

func (h *SyncHandler) HandleGetSteamConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "steam",
		steamConnectionResponse{Connected: false},
		steamConnectionResponse{Connected: true, CredentialsError: true},
		func(st storefront.Status) any {
			return steamConnectionResponse{Connected: true, Username: st.Account, CredentialsError: st.CredentialsError}
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

	account, err := h.connections.ConnectPlayStationStore(c.Request().Context(), userID, req.NpssoToken)
	switch {
	case errors.Is(err, storefront.ErrNPSSOTokenLength):
		return echo.NewHTTPError(http.StatusBadRequest, "npsso_token must be exactly 64 characters")
	case errors.Is(err, storefront.ErrInvalidNPSSOToken):
		return echo.NewHTTPError(http.StatusBadRequest, "invalid_npsso_token")
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "psn: persist storefront credentials failed", logging.KeyErr, err, logging.KeySource, "playstation-store", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist PSN connection")
	}

	return c.JSON(http.StatusOK, psnConnectResponse{
		Success:   true,
		OnlineID:  account.OnlineID,
		AccountID: account.AccountID,
		Message:   "PSN connected successfully",
	})
}

func (h *SyncHandler) HandleGetPlaystationStoreConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "playstation-store",
		psnConnectionResponse{},
		psnConnectionResponse{IsConfigured: true, CredentialsError: true},
		func(st storefront.Status) any {
			return psnConnectionResponse{IsConfigured: true, CredentialsError: st.CredentialsError, OnlineID: st.Account}
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

	err := h.connections.ConnectHumbleBundle(c.Request().Context(), userID, req.SessionCookie)
	switch {
	case errors.Is(err, storefront.ErrHumbleBundleCookieMissing):
		return echo.NewHTTPError(http.StatusBadRequest, "session_cookie is required")
	case errors.Is(err, storefront.ErrInvalidHumbleBundleCookie):
		return echo.NewHTTPError(http.StatusBadRequest, "invalid_session_cookie")
	case errors.Is(err, storefront.ErrHumbleBundleUnavailable):
		slog.WarnContext(c.Request().Context(), "humble: verify failed", logging.KeyErr, err, logging.KeySource, "humble-bundle", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadGateway, "could not reach Humble Bundle")
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "humble: persist storefront credentials failed", logging.KeyErr, err, logging.KeySource, "humble-bundle", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Humble Bundle connection")
	}

	return c.JSON(http.StatusOK, humbleConnectResponse{Success: true, Message: "Humble Bundle connected successfully"})
}

// HandleGetHumbleConnection is GET /sync/humble-bundle/connection.
func (h *SyncHandler) HandleGetHumbleConnection(c *echo.Context) error {
	return h.serveConnectionStatus(c, "humble-bundle",
		humbleStatusResponse{},
		humbleStatusResponse{IsConfigured: true, CredentialsError: true},
		func(st storefront.Status) any {
			return humbleStatusResponse{IsConfigured: true, CredentialsError: st.CredentialsError}
		})
}

// HandleHumbleDisconnect is DELETE /sync/humble-bundle/connection.
func (h *SyncHandler) HandleHumbleDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "humble-bundle")
}

// epicGamesStoreNotConfigured answers an Epic sign-in on a server without
// LEGENDARY_WORK_DIR.
const epicGamesStoreNotConfigured = "Epic sync not configured (LEGENDARY_WORK_DIR unset)"

// HandleEpicGamesStoreConnect exchanges an Epic auth code via legendary and stores the
// resulting legendary state snapshot in the DB.
func (h *SyncHandler) HandleEpicGamesStoreConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	// Checked before the body, so an unconfigured server answers 503 to any
	// request.
	if !h.connections.EpicGamesStoreConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, epicGamesStoreNotConfigured)
	}

	var body struct {
		AuthCode string `json:"auth_code"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	account, err := h.connections.ConnectEpicGamesStore(c.Request().Context(), userID, body.AuthCode)
	switch {
	case errors.Is(err, storefront.ErrEpicGamesStoreNotConfigured):
		return echo.NewHTTPError(http.StatusServiceUnavailable, epicGamesStoreNotConfigured)
	case errors.Is(err, storefront.ErrEpicGamesStoreAuthCodeMissing):
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	case errors.Is(err, storefront.ErrEpicGamesStoreSignInFailed):
		return echo.NewHTTPError(http.StatusBadRequest, "epic auth failed: "+err.Error())
	case errors.Is(err, storefront.ErrEpicGamesStoreNoAccount):
		slog.ErrorContext(c.Request().Context(), "epic: no account after sign-in", logging.KeyErr, err, logging.KeySource, "epic-games-store", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusInternalServerError, "epic: user.json missing after auth")
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "epic: persist storefront credentials failed", logging.KeyErr, err, logging.KeySource, "epic-games-store", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Epic connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"display_name": account.DisplayName,
		"account_id":   account.AccountID,
	})
}

// HandleEpicGamesStoreDisconnect clears the user's Epic credentials and snapshot, and
// removes any legendary files older versions left on disk. A failure to remove
// those is logged but still answers 204.
func (h *SyncHandler) HandleEpicGamesStoreDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.connections.DisconnectEpicGamesStore(c.Request().Context(), userID); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: epic disconnect failed", logging.KeyErr, err, logging.KeySource, "epic-games-store", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect Epic")
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

	if !h.connections.EpicGamesStoreConfigured() {
		return c.JSON(http.StatusOK, map[string]any{
			"connected": false,
			"disabled":  true,
			"reason":    "legendary_not_configured",
		})
	}

	return h.serveConnectionStatus(c, "epic-games-store",
		map[string]any{"connected": false, "disabled": false},
		map[string]any{"connected": true, "credentials_error": true, "disabled": false},
		func(st storefront.Status) any {
			return map[string]any{
				"connected":         true,
				"disabled":          false,
				"credentials_error": st.CredentialsError,
				"display_name":      st.Account,
			}
		})
}

// amazonGamesNotInstalled answers an Amazon sign-in on a server without nile.
const amazonGamesNotInstalled = "Amazon Games sync not available (nile not installed)"

// HandleAmazonGamesLoginStart is POST /sync/amazon-games/connection/login: it
// starts an Amazon sign-in and returns the URL to open and the attempt id to
// finish it with.
func (h *SyncHandler) HandleAmazonGamesLoginStart(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	signIn, err := h.connections.StartAmazonGamesSignIn(c.Request().Context(), userID)
	switch {
	case errors.Is(err, storefront.ErrAmazonGamesNotInstalled):
		return echo.NewHTTPError(http.StatusServiceUnavailable, amazonGamesNotInstalled)
	case errors.Is(err, storefront.ErrAmazonGamesStartFailed):
		slog.ErrorContext(c.Request().Context(), "amazon: start sign-in failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadGateway, "could not start an Amazon sign-in")
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "amazon: seal sign-in attempt failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryAuth))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not start an Amazon sign-in")
	}
	return c.JSON(http.StatusOK, map[string]string{
		"attempt_id": signIn.AttemptID,
		"login_url":  signIn.LoginURL,
	})
}

// HandleAmazonGamesConnect is PUT /sync/amazon-games/connection: it finishes
// the sign-in with the landing address (or bare code), registers the device
// through nile, and stores nile's user data encrypted.
func (h *SyncHandler) HandleAmazonGamesConnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	// Checked before the body, so a server without nile answers 503 to any
	// request.
	if !h.connections.AmazonGamesConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, amazonGamesNotInstalled)
	}
	var body struct {
		AttemptID   string `json:"attempt_id"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "attempt_id is required")
	}
	username, err := h.connections.ConnectAmazonGames(c.Request().Context(), userID, body.AttemptID, body.RedirectURL)
	switch {
	case errors.Is(err, storefront.ErrAmazonGamesNotInstalled):
		return echo.NewHTTPError(http.StatusServiceUnavailable, amazonGamesNotInstalled)
	case errors.Is(err, storefront.ErrAmazonGamesAttemptIDMissing):
		return echo.NewHTTPError(http.StatusBadRequest, "attempt_id is required")
	case errors.Is(err, storefront.ErrInvalidAmazonGamesRedirect):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, storefront.ErrAmazonGamesAttemptUnusable):
		return echo.NewHTTPError(http.StatusConflict, "this Amazon sign-in has expired or cannot be finished here; start again")
	case errors.Is(err, storefront.ErrAmazonGamesSignInRejected):
		slog.WarnContext(c.Request().Context(), "amazon: device registration failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadRequest, "Amazon did not accept that sign-in; start again and paste the address you land on")
	case errors.Is(err, storefront.ErrAmazonGamesNoAccount):
		slog.ErrorContext(c.Request().Context(), "amazon: no account after registration", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Amazon Games connection")
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "amazon: persist storefront credentials failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Amazon Games connection")
	}
	return c.JSON(http.StatusOK, map[string]string{"username": username})
}

// HandleGetAmazonGamesConnection is GET /sync/amazon-games/connection.
func (h *SyncHandler) HandleGetAmazonGamesConnection(c *echo.Context) error {
	if auth.UserIDFromContext(c) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !h.connections.AmazonGamesConfigured() {
		// Nothing can sign in or sync without nile, but a sign-in stored earlier
		// is still reported so the page can offer to remove it.
		st, err := h.connections.Load(context.Background(), auth.UserIDFromContext(c), "amazon-games")
		if err != nil {
			slog.ErrorContext(c.Request().Context(), "sync: load connection status failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load Amazon Games connection")
		}
		return c.JSON(http.StatusOK, map[string]any{
			"connected": st.Connected,
			"disabled":  true,
			"reason":    "nile_not_installed",
		})
	}
	return h.serveConnectionStatus(c, "amazon-games",
		map[string]any{"connected": false, "disabled": false},
		map[string]any{"connected": true, "credentials_error": true, "disabled": false},
		func(st storefront.Status) any {
			return map[string]any{
				"connected":         true,
				"disabled":          false,
				"credentials_error": st.CredentialsError,
				"username":          st.Account,
			}
		})
}

// HandleAmazonGamesDisconnect is DELETE /sync/amazon-games/connection. It
// removes the stored sign-in and answers as the other stores do; the module
// signs that device out of Amazon in the background.
func (h *SyncHandler) HandleAmazonGamesDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if err := h.connections.DisconnectAmazonGames(c.Request().Context(), userID); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: disconnect failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect amazon-games")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SyncHandler) HandleListIgnored(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// One row per set of editions, its representative, as the storefront list
	// shows it (ADR 0007, #1438).
	key := externalgame.EditionKey("eg")
	var games []models.ExternalGame
	err := h.db.NewRaw(
		`SELECT DISTINCT ON (`+key+`) eg.*
		FROM external_games eg
		WHERE eg.user_id = ? AND eg.is_skipped
		ORDER BY `+key+`, `+syncRepresentativeOrder("eg"),
		userID,
	).Scan(c.Request().Context(), &games)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: list ignored failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
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
	onlyThis := onlyThisEdition(c)
	ctx := context.Background()

	// A plain read, no lock: the Jobs in which the game or one of its
	// editions has an open item (with the option: the game itself). Each is
	// entered through the module, which locks the Job row before the editions
	// (one lock order everywhere).
	findJobs := externalgame.EditionsOpenItemJobs
	if onlyThis {
		findJobs = externalgame.OwnOpenItemJobs
	}
	jobIDs, err := findJobs(ctx, h.db, userID, id)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: skip game: find items", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
	}

	flagged := false
	for _, jobID := range jobIDs {
		_, err := h.lc.SkipItems(ctx, jobID, func(ctx context.Context, tx bun.Tx, job *models.Job) ([]string, error) {
			return skipEditions(ctx, tx, userID, id, job.ID, onlyThis)
		})
		switch {
		case err == nil:
			flagged = true
		case errors.Is(err, joblifecycle.ErrJobNotActive), errors.Is(err, joblifecycle.ErrGone):
			// Ended meanwhile: only the flags, below.
		default:
			return skipHTTPError(c, err, id)
		}
	}
	if !flagged {
		if err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			_, err := skipEditions(ctx, tx, userID, id, "", onlyThis)
			return err
		}); err != nil {
			return skipHTTPError(c, err, id)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

func skipHTTPError(c *echo.Context, err error, id string) error {
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he
	}
	slog.ErrorContext(c.Request().Context(), "sync: skip game failed", logging.KeyErr, err, logging.KeyExternalGameID, id, logging.Cat(logging.CategoryDB))
	return echo.NewHTTPError(http.StatusInternalServerError, "failed to skip game")
}

// skipEditions skips an External game and its editions
// (externalgame.Editions.Skip). With jobID set it runs in the module's
// transaction for that Job and returns the changed games' open items in it,
// for the module to skip; the game's 'skipped' row is written only when the
// game's own item is among them (its editions get none).
//
// With onlyThis it skips the given game alone and returns only its own items.
func skipEditions(ctx context.Context, tx bun.Tx, userID, id, jobID string, onlyThis bool) ([]string, error) {
	set, err := lockEditions(ctx, tx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := set.Skip(ctx, onlyThis); err != nil {
		return nil, err
	}
	if jobID == "" {
		return nil, nil
	}
	items, err := externalgame.EditionsOpenItems(ctx, tx, jobID, set.Changed(onlyThis))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
		if it.ExternalGameID != id {
			continue
		}
		if err := tasks.InsertSkippedChange(ctx, tx, jobID, userID, id, set.Game().Title); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (h *SyncHandler) HandleListExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// One entry per set of editions (#1449), from externalGameListQuery.
	var games []externalGameResponse
	err = h.db.NewRaw(externalGameListQuery,
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
		if err := json.Unmarshal(games[i].EditionsJSON, &games[i].Editions); err != nil {
			slog.ErrorContext(c.Request().Context(), "sync: list external games: decode editions", logging.KeyErr, err, logging.KeyExternalGameID, games[i].ID, logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to list external games")
		}
		for j := range games[i].Editions {
			e := &games[i].Editions[j]
			if e.StoreLink != nil {
				if url, ok := buildStoreURL(games[i].Storefront, *e.StoreLink); ok {
					e.StoreURL = &url
				}
				e.StoreLink = nil
			}
		}
	}
	return c.JSON(http.StatusOK, games)
}

// externalGameListQuery lists a storefront's External games, one entry per set
// of editions (externalgame.EditionKey). Parameters: user id, storefront.
//
// The sets come from syncEditionSets, shared with the review badge. The entry
// is the set's representative; a set with an edition in flight is hidden. Its
// platforms are those of all editions. failed_job_item_id is the newest
// item of the first retryable edition, only while the entry shows as failed.
// editions_json lists every edition of the set in id order, each with its own
// platforms and store link (#1452).
//
// The orphan fields are per set, matching rematch's orphan check: links of any
// edition, "other" meaning outside the set; with the set on several user
// games, the smallest count among them (zero: at least one would be orphaned),
// and the user game the representative's own links are on, else the lowest id.
var externalGameListQuery = syncEditionSets(`eg.user_id = ? AND eg.storefront = ?`) + `
	SELECT r.id, r.storefront, r.external_id, r.title, r.resolved_igdb_id, r.is_skipped,
	       s.is_available, r.is_subscription,
	       COALESCE(fl.has_user_game, false) AS has_user_game, fl.user_game_id,
	       g.title AS igdb_title,
	       COALESCE(fl.other_count, 0) AS user_game_other_platform_count,
	       CASE
	           WHEN s.needs_review THEN 'needs_review'
	           WHEN r.is_skipped THEN 'skipped'
	           WHEN s.failed THEN 'failed'
	           WHEN r.resolved_igdb_id IS NOT NULL THEN 'matched'
	           ELSE 'unmatched'
	       END AS sync_status,
	       CASE WHEN NOT s.needs_review AND NOT r.is_skipped THEN s.retry_item_id END AS failed_job_item_id,
	       COALESCE(
	           (SELECT string_agg(DISTINCT egp.platform, ',' ORDER BY egp.platform)
	            FROM external_game_platforms egp
	            WHERE egp.external_game_id = ANY(s.ids)),
	           '') AS platforms_csv,
	       r.store_link,
	       (SELECT json_agg(json_build_object(
	                   'id', e.id,
	                   'external_id', e.external_id,
	                   'platforms', COALESCE(
	                       (SELECT json_agg(egp.platform ORDER BY egp.platform)
	                        FROM external_game_platforms egp
	                        WHERE egp.external_game_id = e.id),
	                       '[]'::json),
	                   'store_link', e.store_link,
	                   'is_available', e.is_available)
	               ORDER BY e.id)
	        FROM edition_rows e WHERE e.rep_id = s.rep_id) AS editions_json
	FROM edition_sets s
	JOIN edition_rows r ON r.id = s.rep_id
	LEFT JOIN games g ON g.id = r.resolved_igdb_id
	LEFT JOIN LATERAL (
		SELECT true AS has_user_game,
		       COALESCE(MIN(ug_id) FILTER (WHERE own), MIN(ug_id)) AS user_game_id,
		       MIN(others) AS other_count
		FROM (
			SELECT l.user_game_id AS ug_id,
			       bool_or(l.external_game_id = r.id) AS own,
			       (SELECT COUNT(*) FROM user_game_platforms o
			        WHERE o.user_game_id = l.user_game_id
			          AND (o.external_game_id IS NULL OR o.external_game_id <> ALL(s.ids))
			       ) AS others
			FROM user_game_platforms l
			WHERE l.external_game_id = ANY(s.ids)
			GROUP BY l.user_game_id
		) per_user_game
		HAVING COUNT(*) > 0
	) fl ON true
	WHERE ` + syncSetListed + `
	ORDER BY r.title ASC, r.id`

// HandleResetSyncData handles DELETE /api/sync/:storefront/data.
// It cancels any active sync job for the storefront, then deletes all
// external_games and user_game_platforms rows for the user+storefront,
// and resets last_synced_at. Credentials are not affected.
func (h *SyncHandler) HandleResetSyncData(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Cancel the storefront's active syncs through the module first. A
	// dispatch still running notices at its next batch: its writes are
	// conditional on the Job being active, so it cannot put back the games
	// deleted below.
	var activeJobs []string
	if err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND source = ? AND job_type = ? AND status IN (?)`,
		userID, sf, models.JobTypeSync, bun.List(models.ActiveJobStatuses()),
	).Scan(ctx, &activeJobs); err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: find active jobs failed", logging.KeyErr, err, logging.KeySource, sf, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel active job")
	}
	for _, jobID := range activeJobs {
		if err := h.lc.Cancel(ctx, jobID); err != nil && !errors.Is(err, joblifecycle.ErrJobNotActive) && !errors.Is(err, joblifecycle.ErrGone) {
			slog.ErrorContext(c.Request().Context(), "sync: cancel active job failed", logging.KeyErr, err, logging.KeyJobID, jobID, logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to cancel active job")
		}
	}

	if err := h.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := usergame.RemoveStorefrontPlatforms(ctx, tx, userID, sf); err != nil {
			return err
		}
		if err := externalgame.DeleteForStorefront(ctx, tx, userID, sf); err != nil {
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

// HandleRetryFailedExternalGames retries every External game of the
// storefront whose newest item failed by the view's rule (it failed, nothing
// wrote the game since, and its Job is active or completed), each edition of a
// set on its own. Items are retried through the module per Job,
// newest Job first: a Job that cannot be retried (failed or cancelled, or
// refused because another sync is active, including one this request has just
// reopened) is left as it is. It answers 204 either way.
func (h *SyncHandler) HandleRetryFailedExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf, err := storefrontParam(c)
	if err != nil {
		return err
	}
	ctx := context.Background()
	reqCtx := c.Request().Context()

	var rows []struct {
		ItemID string `bun:"item_id"`
		JobID  string `bun:"job_id"`
	}
	if err := h.db.NewRaw(`
		SELECT ni.id AS item_id, ni.job_id
		FROM external_games eg
		`+syncNewestItemJoin+`
		WHERE eg.user_id = ? AND eg.storefront = ? AND NOT eg.is_skipped
		  AND `+syncRetryable+`
		ORDER BY nj.created_at DESC, nj.id, ni.id`,
		userID, sf,
	).Scan(ctx, &rows); err != nil {
		slog.ErrorContext(reqCtx, "sync: retry-failed", logging.KeySource, sf, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to retry failed items")
	}
	var order []string
	byJob := map[string][]string{}
	for _, r := range rows {
		if _, ok := byJob[r.JobID]; !ok {
			order = append(order, r.JobID)
		}
		byJob[r.JobID] = append(byJob[r.JobID], r.ItemID)
	}
	for _, jobID := range order {
		_, err := h.lc.Retry(ctx, jobID, byJob[jobID])
		switch {
		case err == nil, errors.Is(err, joblifecycle.ErrNotRetryable), errors.Is(err, joblifecycle.ErrDuplicateActive),
			errors.Is(err, joblifecycle.ErrJobNotActive), errors.Is(err, joblifecycle.ErrGone):
		default:
			slog.ErrorContext(reqCtx, "sync: retry-failed", logging.KeySource, sf, logging.KeyJobID, jobID, logging.KeyErr, err, logging.Cat(logging.CategoryDB))
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to retry failed items")
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
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	}

	username, err := h.connections.ConnectGOG(c.Request().Context(), userID, body.AuthCode)
	switch {
	case errors.Is(err, storefront.ErrGOGAuthCodeMissing):
		return echo.NewHTTPError(http.StatusBadRequest, "auth_code is required")
	case errors.Is(err, storefront.ErrInvalidGOGAuthCode):
		// The message is only the reason, worded for the user.
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, storefront.ErrGOGSignInFailed):
		// The message is only GOG's answer (#1528).
		return echo.NewHTTPError(http.StatusBadRequest, "gog auth failed: "+err.Error())
	case err != nil:
		slog.ErrorContext(c.Request().Context(), "gog: persist credentials failed", logging.KeyErr, err, logging.KeySource, "gog", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist GOG connection")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"username": username,
	})
}

func (h *SyncHandler) HandleGOGDisconnect(c *echo.Context) error {
	return h.disconnectStorefront(c, "gog")
}

func (h *SyncHandler) HandleGetGOGConnection(c *echo.Context) error {
	authURL := h.connections.GOGSignInURL()

	return h.serveConnectionStatus(c, "gog",
		map[string]any{"connected": false, "auth_url": authURL},
		map[string]any{"connected": true, "credentials_error": true, "auth_url": authURL},
		func(st storefront.Status) any {
			return map[string]any{
				"connected":         true,
				"credentials_error": st.CredentialsError,
				"username":          st.Account,
				"auth_url":          authURL,
			}
		})
}
