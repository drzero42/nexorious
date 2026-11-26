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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/crypto"
	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/externalgame"
	"github.com/drzero42/nexorious/internal/joblifecycle"
	"github.com/drzero42/nexorious/internal/logging"
	amazongamessvc "github.com/drzero42/nexorious/internal/services/amazongames"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/igdb"
	"github.com/drzero42/nexorious/internal/usergame"
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
	// Cleanup removes the user's legendary files that older versions left
	// on disk. Sign-ins and syncs remove their own files when they end.
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

// AmazonGamesClient abstracts the nile CLI calls used during Amazon Games
// account connection.
type AmazonGamesClient interface {
	Configured() bool
	// LoginURL starts a sign-in and returns its URL and the values nile needs
	// to finish it, which must stay server-side.
	LoginURL(ctx context.Context) (*AmazonGamesLogin, error)
	// Register finishes the sign-in with the authorization code and returns
	// the account and nile's user data, which the caller stores encrypted.
	Register(ctx context.Context, code string, login AmazonGamesLogin) (*AmazonGamesAccount, json.RawMessage, error)
	// Logout deregisters the device a stored sign-in (nile's user data)
	// belongs to.
	Logout(ctx context.Context, userData json.RawMessage) error
}

// AmazonGamesLogin is one sign-in attempt's state.
type AmazonGamesLogin struct {
	URL          string
	ClientID     string
	CodeVerifier string
	Serial       string
}

// AmazonGamesAccount identifies a connected Amazon account.
type AmazonGamesAccount struct {
	UserID string
	Name   string
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
var supportedStorefronts = []string{"steam", "playstation-store", "epic-games-store", "gog", "humble-bundle", "amazon-games"}

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
	db                     *bun.DB
	encrypter              *crypto.Encrypter
	riverClient            *river.Client[pgx.Tx]
	lc                     *joblifecycle.Module
	steamClient            SteamClient
	playstationStoreClient PlaystationStoreClient
	epicGamesStoreClient   EpicGamesStoreClient
	gogClient              GOGClient
	humbleClient           HumbleClient
	amazonGamesClient      AmazonGamesClient
	igdbClient             *igdb.Client
	amazonLoginClock       func() time.Time // nil: time.Now
	amazonSignOutTimeout   time.Duration    // 0: amazonSignOutLimit
	amazonSignOuts         sync.WaitGroup   // device sign-outs still running after a disconnect
}

// SyncDeps holds the dependencies of a SyncHandler. A storefront client may be
// left nil as long as no route that reaches that storefront is called.
type SyncDeps struct {
	Encrypter *crypto.Encrypter
	DB        *bun.DB
	// River inserts the immediate metadata fetch after a rematch or an
	// unskip, which is not Job work.
	River *river.Client[pgx.Tx]
	// Lifecycle owns the sync Jobs.
	Lifecycle        *joblifecycle.Module
	Steam            SteamClient
	PlaystationStore PlaystationStoreClient
	EpicGamesStore   EpicGamesStoreClient
	GOG              GOGClient
	Humble           HumbleClient
	AmazonGames      AmazonGamesClient
	// IGDB, nil or unconfigured: a Rematch stubs with the storefront title.
	IGDB *igdb.Client
}

// NewSyncHandler constructs a SyncHandler.
func NewSyncHandler(d SyncDeps) *SyncHandler {
	return &SyncHandler{
		encrypter:              d.Encrypter,
		db:                     d.DB,
		riverClient:            d.River,
		lc:                     d.Lifecycle,
		steamClient:            d.Steam,
		playstationStoreClient: d.PlaystationStore,
		epicGamesStoreClient:   d.EpicGamesStore,
		gogClient:              d.GOG,
		humbleClient:           d.Humble,
		amazonGamesClient:      d.AmazonGames,
		igdbClient:             d.IGDB,
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
	sf := c.Param("storefront")
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
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
// removes any legendary files older versions left on disk. A failure to remove
// those is logged but still answers 204: the stored credentials are already
// gone, so the disconnect itself succeeded.
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

// amazonLoginTTL bounds how long a started Amazon sign-in can be finished.
const amazonLoginTTL = 15 * time.Minute

// amazonLoginKind marks an attempt_id as an Amazon sign-in attempt, so no
// other ciphertext made with the same key (a stored credential) is ever read
// as one.
const amazonLoginKind = "amazon-games-login"

// maxAmazonAttemptIDLen bounds the attempt_id the finish will decrypt. A real
// one is about 450 characters; the cap keeps a pasted multi-kilobyte value
// from being decoded at all.
const maxAmazonAttemptIDLen = 2048

// amazonLoginAttempt is everything finishing an Amazon sign-in needs: the
// client id, code verifier and serial nile generated, who started it and until
// when it can be finished. The browser holds it encrypted with the server's
// key as the attempt_id, so a start and a finish may reach different servers,
// or one server either side of a restart. Starting again does not cancel an
// earlier attempt; whichever finishes last is the stored sign-in.
type amazonLoginAttempt struct {
	Kind         string    `json:"kind"`
	ClientID     string    `json:"client_id"`
	CodeVerifier string    `json:"code_verifier"`
	Serial       string    `json:"serial"`
	UserID       string    `json:"user_id"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// sealAmazonLogin returns the attempt_id for a sign-in userID just started.
func (h *SyncHandler) sealAmazonLogin(userID string, login AmazonGamesLogin) (string, error) {
	raw, err := json.Marshal(amazonLoginAttempt{
		Kind:         amazonLoginKind,
		ClientID:     login.ClientID,
		CodeVerifier: login.CodeVerifier,
		Serial:       login.Serial,
		UserID:       userID,
		ExpiresAt:    h.amazonLoginNow().Add(amazonLoginTTL),
	})
	if err != nil {
		return "", err
	}
	return h.encrypter.Encrypt(raw)
}

// openAmazonLogin returns the sign-in an attempt_id carries, when it decrypts
// with this server's key, is an Amazon sign-in attempt, was started by userID,
// is complete and has not expired.
func (h *SyncHandler) openAmazonLogin(userID, attemptID string) (AmazonGamesLogin, bool) {
	if len(attemptID) > maxAmazonAttemptIDLen {
		return AmazonGamesLogin{}, false
	}
	raw, err := h.encrypter.Decrypt(attemptID)
	if err != nil {
		return AmazonGamesLogin{}, false
	}
	var a amazonLoginAttempt
	if err := json.Unmarshal(raw, &a); err != nil {
		return AmazonGamesLogin{}, false
	}
	if a.Kind != amazonLoginKind || a.UserID != userID ||
		a.ClientID == "" || a.CodeVerifier == "" || a.Serial == "" ||
		a.ExpiresAt.IsZero() || !h.amazonLoginNow().Before(a.ExpiresAt) {
		return AmazonGamesLogin{}, false
	}
	return AmazonGamesLogin{ClientID: a.ClientID, CodeVerifier: a.CodeVerifier, Serial: a.Serial}, true
}

func (h *SyncHandler) amazonLoginNow() time.Time {
	if h.amazonLoginClock != nil {
		return h.amazonLoginClock()
	}
	return time.Now()
}

// amazonSignOutLimit bounds the best-effort device sign-out a disconnect
// starts.
const amazonSignOutLimit = 20 * time.Second

// SetAmazonSignOutTimeoutForTest replaces amazonSignOutLimit.
func (h *SyncHandler) SetAmazonSignOutTimeoutForTest(d time.Duration) {
	h.amazonSignOutTimeout = d
}

// WaitForAmazonSignOutsForTest waits for every device sign-out a disconnect
// started.
func (h *SyncHandler) WaitForAmazonSignOutsForTest() {
	h.amazonSignOuts.Wait()
}

// SetAmazonLoginClockForTest replaces the clock Amazon sign-in attempts are
// stamped and expire by.
func (h *SyncHandler) SetAmazonLoginClockForTest(now func() time.Time) {
	h.amazonLoginClock = now
}

func (h *SyncHandler) amazonGamesConfigured() bool {
	return h.amazonGamesClient != nil && h.amazonGamesClient.Configured()
}

// HandleAmazonGamesLoginStart is POST /sync/amazon-games/connection/login: it
// starts an Amazon sign-in and returns the URL to open and the attempt id to
// finish it with.
func (h *SyncHandler) HandleAmazonGamesLoginStart(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !h.amazonGamesConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Amazon Games sync not available (nile not installed)")
	}
	login, err := h.amazonGamesClient.LoginURL(c.Request().Context())
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "amazon: start sign-in failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadGateway, "could not start an Amazon sign-in")
	}
	attemptID, err := h.sealAmazonLogin(userID, *login)
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "amazon: seal sign-in attempt failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryAuth))
		return echo.NewHTTPError(http.StatusInternalServerError, "could not start an Amazon sign-in")
	}
	return c.JSON(http.StatusOK, map[string]string{
		"attempt_id": attemptID,
		"login_url":  login.URL,
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
	if !h.amazonGamesConfigured() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Amazon Games sync not available (nile not installed)")
	}
	var body struct {
		AttemptID   string `json:"attempt_id"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := c.Bind(&body); err != nil || body.AttemptID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "attempt_id is required")
	}
	code, err := amazongamessvc.ParseAuthCode(body.RedirectURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	login, ok := h.openAmazonLogin(userID, body.AttemptID)
	if !ok {
		return echo.NewHTTPError(http.StatusConflict, "this Amazon sign-in has expired or cannot be finished here; start again")
	}
	account, userData, err := h.amazonGamesClient.Register(c.Request().Context(), code, login)
	if err != nil {
		slog.WarnContext(c.Request().Context(), "amazon: device registration failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return echo.NewHTTPError(http.StatusBadRequest, "Amazon did not accept that sign-in; start again and paste the address you land on")
	}
	if err := h.persistStorefrontCredentials(context.Background(), userID, "amazon-games", userData); err != nil {
		slog.ErrorContext(c.Request().Context(), "amazon: persist storefront credentials failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to persist Amazon Games connection")
	}
	return c.JSON(http.StatusOK, map[string]string{"username": account.Name})
}

// HandleGetAmazonGamesConnection is GET /sync/amazon-games/connection.
func (h *SyncHandler) HandleGetAmazonGamesConnection(c *echo.Context) error {
	if auth.UserIDFromContext(c) == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	if !h.amazonGamesConfigured() {
		// Nothing can sign in or sync without nile, but a sign-in stored earlier
		// is still reported so the page can offer to remove it.
		st, err := h.loadConnectionStatus(context.Background(), auth.UserIDFromContext(c), "amazon-games")
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
		func(plaintext []byte, credentialsError bool) (any, error) {
			// The stored credential is nile's user data; the account name is
			// its customer_info.given_name.
			var creds struct {
				Extensions struct {
					CustomerInfo struct {
						GivenName string `json:"given_name"`
					} `json:"customer_info"`
				} `json:"extensions"`
			}
			if err := json.Unmarshal(plaintext, &creds); err != nil {
				return nil, err
			}
			return map[string]any{
				"connected":         true,
				"disabled":          false,
				"credentials_error": credentialsError,
				"username":          creds.Extensions.CustomerInfo.GivenName,
			}, nil
		})
}

// HandleAmazonGamesDisconnect is DELETE /sync/amazon-games/connection. It
// removes the stored sign-in and answers as the other stores do, then signs
// that device out of Amazon in the background, best effort, so reconnecting
// does not leave another registered device on the account. The sign-out uses
// only the sign-in this disconnect removed: a reconnect stored meanwhile is
// never touched, and a removal that failed signs nothing out.
func (h *SyncHandler) HandleAmazonGamesDisconnect(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	removed, err := h.takeStorefrontCredentials(context.Background(), userID, "amazon-games")
	if err != nil {
		slog.ErrorContext(c.Request().Context(), "sync: disconnect failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to disconnect amazon-games")
	}
	if removed != nil && h.amazonGamesConfigured() {
		// Detached from the request so the answer does not wait for Amazon,
		// keeping the request's logging values.
		ctx := context.WithoutCancel(c.Request().Context())
		h.amazonSignOuts.Go(func() { h.signOutAmazonDevice(ctx, *removed) })
	}
	return c.NoContent(http.StatusNoContent)
}

// takeStorefrontCredentials clears a storefront's stored credentials and
// returns the ciphertext it cleared, in one statement, so nothing stored after
// it can be returned. It returns nil when nothing was stored.
func (h *SyncHandler) takeStorefrontCredentials(ctx context.Context, userID, sf string) (*string, error) {
	var removed *string
	err := h.db.NewRaw(
		`UPDATE user_sync_configs u SET storefront_credentials = NULL, updated_at = now()
		 FROM (SELECT id, storefront_credentials FROM user_sync_configs WHERE user_id = ? AND storefront = ? FOR UPDATE) old
		 WHERE u.id = old.id
		 RETURNING old.storefront_credentials`,
		userID, sf,
	).Scan(ctx, &removed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return removed, err
}

// signOutAmazonDevice runs nile's logout with a removed Amazon sign-in, under
// a time limit; any failure is logged and otherwise ignored.
func (h *SyncHandler) signOutAmazonDevice(ctx context.Context, ciphertext string) {
	limit := amazonSignOutLimit
	if h.amazonSignOutTimeout > 0 {
		limit = h.amazonSignOutTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	userData, err := h.encrypter.Decrypt(ciphertext)
	if err != nil {
		slog.WarnContext(ctx, "amazon: device sign-out skipped: the removed sign-in does not decrypt", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryAuth))
		return
	}
	if err := h.amazonGamesClient.Logout(ctx, userData); err != nil {
		slog.WarnContext(ctx, "amazon: device sign-out after disconnect failed", logging.KeyErr, err, logging.KeySource, "amazon-games", logging.Cat(logging.CategoryExternalAPI))
		return
	}
	slog.InfoContext(ctx, "amazon: device signed out after disconnect", logging.KeySource, "amazon-games")
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
	sf := c.Param("storefront")
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// One entry per set of editions (#1449), from externalGameListQuery.
	var games []externalGameResponse
	err := h.db.NewRaw(externalGameListQuery,
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
	sf := c.Param("storefront")
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	// Cancel the storefront's active syncs through the module first. A
	// dispatch still running notices at its next batch: its writes are
	// conditional on the Job being active, so it cannot put back the games
	// deleted below.
	var activeJobs []string
	if err := h.db.NewRaw(
		`SELECT id FROM jobs WHERE user_id = ? AND source = ? AND job_type = 'sync' AND status IN ('pending', 'processing')`,
		userID, sf,
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
	sf := c.Param("storefront")
	if !validStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
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
