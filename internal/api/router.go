package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/uptrace/bun"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/backup"
	"github.com/drzero42/nexorious/internal/config"
	"github.com/drzero42/nexorious/internal/crypto"
	maint "github.com/drzero42/nexorious/internal/middleware"
	migrate "github.com/drzero42/nexorious/internal/migrate"
	"github.com/drzero42/nexorious/internal/notify"
	epicgamesstoresvc "github.com/drzero42/nexorious/internal/services/epicgamesstore"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	humblesvc "github.com/drzero42/nexorious/internal/services/humble"
	"github.com/drzero42/nexorious/internal/services/igdb"
	playstationstoresvc "github.com/drzero42/nexorious/internal/services/playstationstore"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/internal/services/updatecheck"
	"github.com/drzero42/nexorious/ui"
)

// appStateJSON responds to an /api/* request that is blocked by an app-state
// gate (DB unavailable, migrations pending, or setup required) with a
// machine-readable 503 instead of a 302 to an HTML page. A running SPA follows
// redirects transparently and would otherwise try to JSON.parse the HTML target
// (see issue #771); the JSON body lets the client detect the state and perform a
// hard navigation to the appropriate page.
func appStateJSON(c *echo.Context, appState string) error {
	return c.JSON(http.StatusServiceUnavailable, map[string]string{"app_state": appState})
}

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(encrypter *crypto.Encrypter, cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, version, commit string, updateState *updatecheck.State, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
	e := echo.New()

	var rc *river.Client[pgx.Tx]
	if len(riverClient) > 0 {
		rc = riverClient[0]
	}

	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				slog.Error("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "err", v.Error)
			} else {
				slog.Info("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency)
			}
			return nil
		},
	}))

	// Gate 1: DB unavailable — redirect everything except /db-error, /health, and /static/app.css
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			state := migrator.State()
			if state == migrate.AppStateDBUnavailable {
				path := c.Request().URL.Path
				if path == "/db-error" || path == "/health" || path == "/static/app.css" {
					return next(c)
				}
				if strings.HasPrefix(path, "/api/") {
					return appStateJSON(c, state.String())
				}
				return c.Redirect(http.StatusFound,
					"/db-error?from="+url.QueryEscape(c.Request().RequestURI))
			}
			return next(c)
		}
	})

	// Gate 2: migrations pending — redirect everything except /migrate*, /api/migrate*, /health, /static/app.css, brand icon assets
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			state := migrator.State()
			if state != migrate.AppStateReady && state != migrate.AppStateDBUnavailable {
				path := c.Request().URL.Path
				if strings.HasPrefix(path, "/migrate") || strings.HasPrefix(path, "/api/migrate") ||
					path == "/health" || path == "/static/app.css" ||
					path == "/logo.svg" || path == "/favicon.svg" ||
					path == "/favicon.ico" || path == "/apple-touch-icon.png" {
					return next(c)
				}
				if strings.HasPrefix(path, "/api/") {
					return appStateJSON(c, state.String())
				}
				return c.Redirect(http.StatusFound, "/migrate")
			}
			return next(c)
		}
	})

	// Gate 3: setup required — redirect everything except /setup, /api/auth/setup/*, /health, /api/migrate*, /static/app.css, brand icon assets
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if migrator.NeedsSetup() {
				path := c.Request().URL.Path
				if path == "/setup" || strings.HasPrefix(path, "/api/auth/setup") ||
					path == "/health" || strings.HasPrefix(path, "/api/migrate") ||
					path == "/static/app.css" ||
					path == "/logo.svg" || path == "/favicon.svg" ||
					path == "/favicon.ico" || path == "/apple-touch-icon.png" {
					return next(c)
				}
				if strings.HasPrefix(path, "/api/") {
					return appStateJSON(c, "needs_setup")
				}
				return c.Redirect(http.StatusFound, "/setup")
			}
			return next(c)
		}
	})

	// Gate 4: Maintenance mode — blocks most requests during restore
	e.Use(maint.MaintenanceMiddleware())

	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     cfg.CORSOrigins,
			AllowCredentials: true,
			AllowHeaders:     []string{"Content-Type", "Authorization"},
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		}))
	}

	mh := migrate.NewHandler(migrator, db)
	registerRoutes(e, encrypter, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, version, commit, updateState, rc)

	return e
}

func registerRoutes(e *echo.Echo, encrypter *crypto.Encrypter, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, version, commit string, updateState *updatecheck.State, riverClient *river.Client[pgx.Tx]) {
	// Migration routes (bypass gate 2 via prefix)
	e.GET("/migrate", mh.HandleMigrateUI)
	e.GET("/api/migrate/status", mh.HandleStatus)
	e.POST("/api/migrate/run", mh.HandleRun)
	e.GET("/api/migrate/progress", mh.HandleProgress)

	// Health check — bypassed by all gates
	igdbStatus := igdb.StatusNotConfigured
	if igdbClient != nil {
		igdbStatus = igdbClient.Status()
	}
	e.GET("/health", func(c *echo.Context) error {
		state := migrator.State()
		status := "ok"
		if state != migrate.AppStateReady {
			status = state.String()
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status":           status,
			"igdb_status":      igdbStatus,
			"backup_available": backup.PgDumpAvailable() && backup.PsqlAvailable(),
		})
	})

	// Version — public, not cached (changes on every deploy)
	e.GET("/api/version", func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "no-store")
		resp := map[string]any{
			"version":              version,
			"commit":               commit,
			"update_check_enabled": cfg.UpdateCheckEnabled,
			"update_available":     false,
			"latest_version":       "",
			"release_url":          "",
		}
		if cfg.UpdateCheckEnabled && updateState != nil {
			if latest, releaseURL := updateState.Latest(); updatecheck.UpdateAvailable(version, latest) {
				resp["update_available"] = true
				resp["latest_version"] = latest
				resp["release_url"] = releaseURL
			}
		}
		return c.JSON(http.StatusOK, resp)
	})

	// DB-error route (bypassed by Gate 1)
	dh := NewDBErrorHandler(resolvedDatabaseURL, migrator)
	e.GET("/db-error", dh.HandleDBError)

	// Shared stylesheet for /migrate, /db-error, /setup.
	// Must be allow-listed by every state gate (see Gate 1/2/3/4 above and the maintenance middleware).
	e.GET("/static/app.css", func(c *echo.Context) error {
		f, err := ui.SharedBox.Open("shared/app.css")
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		c.Response().Header().Set("Cache-Control", "public, max-age=3600")
		return c.Stream(http.StatusOK, "text/css; charset=utf-8", f)
	})

	// Setup page (bypassed by Gate 3)
	e.GET("/setup", func(c *echo.Context) error {
		if !migrator.NeedsSetup() {
			return c.Redirect(http.StatusFound, "/")
		}
		f, err := ui.SetupBox.Open("setup/index.html")
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		return c.Stream(http.StatusOK, "text/html; charset=utf-8", f)
	})

	// Setup API routes (bypassed by Gate 3)
	sh := NewSetupHandler(db, cfg, migrator)
	e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)

	// Backup handler — used by both setup restore and admin routes
	bh := NewBackupHandler(backupSvc, db, restoreCallbacks)
	e.POST("/api/auth/setup/restore", bh.HandleSetupRestore)
	e.GET("/api/auth/setup/backups", bh.HandleSetupListBackups)
	e.POST("/api/auth/setup/restore/disk", bh.HandleSetupRestoreFromDisk)

	// Auth routes — only registered when a DB is available.
	if db != nil {
		ah := NewAuthHandler(db, cfg)

		// Public auth routes (no auth required)
		e.POST("/api/auth/login", ah.HandleLogin)

		// Auth-protected auth routes
		authGroup := e.Group("/api/auth", auth.AuthMiddleware(db))
		authGroup.POST("/logout", ah.HandleLogout)
		authGroup.GET("/me", ah.HandleGetMe)
		authGroup.PUT("/change-password", ah.HandleChangePassword)
		authGroup.GET("/username/check/:username", ah.HandleCheckUsername)
		authGroup.PUT("/username", ah.HandleChangeUsername)
		authGroup.GET("/sessions", ah.HandleListSessions)
		authGroup.DELETE("/sessions", ah.HandleRevokeAllOtherSessions)
		authGroup.DELETE("/sessions/:id", ah.HandleRevokeSession)
		authGroup.GET("/api-keys", ah.HandleListAPIKeys)
		authGroup.POST("/api-keys", ah.HandleCreateAPIKey)
		authGroup.DELETE("/api-keys/:id", ah.HandleRevokeAPIKey)

		// Platform and storefront routes (all auth-protected)
		ph := NewPlatformsHandler(db)
		platformsGroup := e.Group("/api/platforms", auth.AuthMiddleware(db))
		platformsGroup.GET("", ph.HandleListPlatforms)
		platformsGroup.GET("/simple-list", ph.HandleSimpleList)
		platformsGroup.GET("/storefronts/simple-list", ph.HandleStorefrontSimpleList)
		platformsGroup.GET("/storefronts", ph.HandleListStorefronts)
		platformsGroup.GET("/storefronts/:storefront", ph.HandleGetStorefront)
		platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
		platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
		platformsGroup.GET("/:platform", ph.HandleGetPlatform)

		// Tag routes (all auth-protected)
		th := NewTagsHandler(db)
		tagsGroup := e.Group("/api/tags", auth.AuthMiddleware(db))
		tagsGroup.GET("", th.HandleListTags)
		tagsGroup.POST("", th.HandleCreateTag)
		tagsGroup.PUT("/:id", th.HandleUpdateTag)
		tagsGroup.DELETE("/:id", th.HandleDeleteTag)

		// Docs routes (auth-protected; admin-guide additionally gated in-handler)
		dch := NewDocsHandler()
		docsGroup := e.Group("/api/docs", auth.AuthMiddleware(db))
		docsGroup.GET("/:slug", dch.HandleGetDoc)

		// Games routes (all auth-protected)
		gh := NewGamesHandler(db, igdbClient, cfg, riverClient)
		gamesGroup := e.Group("/api/games", auth.AuthMiddleware(db))
		gamesGroup.GET("", gh.HandleListGames)
		gamesGroup.POST("/metadata/refresh-job", gh.HandleStartMetadataRefreshJob)
		gamesGroup.POST("/store-links/refresh-job", gh.HandleStartStoreLinkRefreshJob)
		gamesGroup.POST("/search/igdb", gh.HandleSearchIGDB)
		gamesGroup.GET("/igdb/:igdb_id", gh.HandleGetIGDBGame)
		gamesGroup.POST("/igdb-import", gh.HandleImportFromIGDB)
		gamesGroup.GET("/:id", gh.HandleGetGame)

		// User Games routes (all auth-protected)
		ugh := NewUserGamesHandler(db, cfg)
		userGamesGroup := e.Group("/api/user-games", auth.AuthMiddleware(db))
		userGamesGroup.GET("", ugh.HandleListUserGames)
		userGamesGroup.POST("", ugh.HandleCreateUserGame)
		userGamesGroup.DELETE("", ugh.HandleClearLibrary)
		userGamesGroup.PUT("/bulk-update", ugh.HandleBulkUpdate)
		userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
		userGamesGroup.POST("/bulk-add-platforms", ugh.HandleBulkAddPlatforms)
		userGamesGroup.DELETE("/bulk-remove-platforms", ugh.HandleBulkRemovePlatforms)
		userGamesGroup.GET("/ids", ugh.HandleListUserGameIDs)
		userGamesGroup.GET("/genres", ugh.HandleListGenres)
		userGamesGroup.GET("/filter-options", ugh.HandleFilterOptions)
		userGamesGroup.GET("/stats", ugh.HandleCollectionStats)
		userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
		userGamesGroup.PUT("/:id", ugh.HandleUpdateUserGame)
		userGamesGroup.DELETE("/:id", ugh.HandleDeleteUserGame)
		userGamesGroup.PUT("/:id/progress", ugh.HandleUpdateProgress)
		userGamesGroup.GET("/:id/platforms", ugh.HandleListPlatforms)
		userGamesGroup.POST("/:id/platforms", ugh.HandleCreatePlatform)
		userGamesGroup.POST("/:id/move-to-library", ugh.HandleMoveToLibrary)
		userGamesGroup.PUT("/:id/platforms/:platform_id", ugh.HandleUpdatePlatform)
		userGamesGroup.DELETE("/:id/platforms/:platform_id", ugh.HandleDeletePlatform)

		// Jobs routes (all auth-protected)
		jh := NewJobsHandler(db, riverClient)
		jobsGroup := e.Group("/api/jobs", auth.AuthMiddleware(db))
		jobsGroup.GET("", jh.HandleListJobs)
		jobsGroup.GET("/summary", jh.HandleJobsSummary)
		jobsGroup.GET("/pending-review-count", jh.HandlePendingReviewCount)
		jobsGroup.GET("/status/:job_type", jh.HandleJobTypeStatus)
		jobsGroup.GET("/recent", jh.HandleRecentJobs)
		jobsGroup.GET("/:id", jh.HandleGetJob)
		jobsGroup.GET("/:id/items", jh.HandleGetJobItems)
		jobsGroup.POST("/:id/cancel", jh.HandleCancelJob)
		jobsGroup.DELETE("/:id", jh.HandleDeleteJob)
		jobsGroup.POST("/:id/retry-failed", jh.HandleRetryFailed)

		// Job Items routes (all auth-protected)
		jih := NewJobItemsHandler(db, riverClient)
		jobItemsGroup := e.Group("/api/job-items", auth.AuthMiddleware(db))
		jobItemsGroup.GET("/:id", jih.HandleGetJobItem)
		jobItemsGroup.POST("/:id/retry", jih.HandleRetryItem)
		jobItemsGroup.POST("/:id/resolve", jih.HandleResolveItem)
		jobItemsGroup.POST("/:id/skip", jih.HandleSkipItem)

		// Import routes (all auth-protected)
		imh := NewImportHandler(db, riverClient, igdbClient)
		importGroup := e.Group("/api/import", auth.AuthMiddleware(db))
		importGroup.POST("/nexorious", imh.HandleImportNexorious)
		importGroup.POST("/darkadia", imh.HandleImportDarkadia)

		// Export routes (all auth-protected)
		exh := NewExportHandler(db, riverClient, cfg)
		exportGroup := e.Group("/api/export", auth.AuthMiddleware(db))
		exportGroup.POST("/json", exh.HandleExportJSON)
		exportGroup.POST("/csv", exh.HandleExportCSV)
		exportGroup.GET("/:id/download", exh.HandleDownload)

		// Admin backup routes (auth + admin required)
		adminGroup := e.Group("", auth.AuthMiddleware(db), auth.AdminMiddleware())
		adminBackups := adminGroup.Group("/api/admin/backups")
		adminBackups.GET("/config", bh.HandleGetConfig)
		adminBackups.PUT("/config", bh.HandleUpdateConfig)
		adminBackups.GET("", bh.HandleListBackups)
		adminBackups.POST("", bh.HandleCreateBackup)
		adminBackups.DELETE("/:id", bh.HandleDeleteBackup)
		adminBackups.GET("/:id/download", bh.HandleDownloadBackup)
		adminBackups.POST("/:id/restore", bh.HandleRestore)
		adminBackups.POST("/restore/upload", bh.HandleRestoreUpload)

		// Admin user management routes (auth + admin required)
		auh := NewAdminUsersHandler(db)
		auh.RegisterRoutes(adminGroup)

		arh := NewAdminResetHandler(db)
		adminGroup.POST("/api/auth/admin/reset", arh.HandleReset)

		// Admin activity / events feed (auth + admin required)
		eh := NewEventsHandler(db)
		adminGroup.GET("/api/admin/events", eh.HandleList)

		// Notifications routes (all auth-protected)
		nh := NewNotificationsHandler(db, encrypter, notify.NewShoutrrrSender())
		notificationsGroup := e.Group("/api/notifications", auth.AuthMiddleware(db))
		// static segments before parameterized (Echo v5 does not auto-sort)
		notificationsGroup.POST("/test", nh.HandleTestURL)
		notificationsGroup.GET("/channels", nh.HandleListChannels)
		notificationsGroup.POST("/channels", nh.HandleCreateChannel)
		notificationsGroup.POST("/channels/:id/test", nh.HandleTestChannel)
		notificationsGroup.PATCH("/channels/:id", nh.HandleUpdateChannel)
		notificationsGroup.DELETE("/channels/:id", nh.HandleDeleteChannel)
		notificationsGroup.GET("/event-types", nh.HandleListEventTypes)
		notificationsGroup.GET("/subscriptions", nh.HandleListSubscriptions)
		notificationsGroup.PUT("/subscriptions", nh.HandlePutSubscriptions)
		notificationsGroup.POST("/subscriptions/reset", nh.HandleResetSubscriptions)

		// Settings routes (all auth-protected)
		sh := NewSettingsHandler(db)
		settingsGroup := e.Group("/api/settings", auth.AuthMiddleware(db))
		settingsGroup.GET("", sh.HandleGet)
		settingsGroup.PATCH("", sh.HandlePatch)

		// Sync routes (all auth-protected)
		steamSvc := steamsvc.NewClient()
		psnSvc := playstationstoresvc.NewClient()
		epicSvc := epicgamesstoresvc.NewClient(cfg.LegendaryWorkDir)
		gogSvc := gogsvc.NewClient()
		humbleSvc := humblesvc.NewClient()
		synch := NewSyncHandler(encrypter, db, riverClient, &steamClientAdapter{c: steamSvc}, &playstationStoreClientAdapter{c: psnSvc}, &epicGamesStoreClientAdapter{c: epicSvc}, &gogClientAdapter{c: gogSvc}, &humbleClientAdapter{c: humbleSvc})
		syncGroup := e.Group("/api/sync", auth.AuthMiddleware(db))
		synch.RegisterRoutes(syncGroup)
	}

	// Static cover art files from disk
	e.GET("/static/cover_art/*", func(c *echo.Context) error {
		http.StripPrefix("/static/cover_art/", http.FileServer(http.Dir(cfg.StoragePath+"/cover_art/"))).
			ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// SPA catch-all — serves ui.UIBox; falls back to index.html
	e.GET("/*", spaHandler())
}

// steamClientAdapter bridges steamsvc.Client to the SteamClient interface
// without creating an import cycle between internal/api and internal/services/steam.
type steamClientAdapter struct{ c *steamsvc.Client }

func (a *steamClientAdapter) GetPlayerSummaries(ctx context.Context, apiKey, steamID string) (*SteamPlayerSummary, error) {
	s, err := a.c.GetPlayerSummaries(ctx, apiKey, steamID)
	if s == nil {
		return nil, err
	}
	return &SteamPlayerSummary{
		PersonaName:              s.PersonaName,
		CommunityVisibilityState: s.CommunityVisibilityState,
	}, err
}

// playstationStoreClientAdapter bridges playstationstoresvc.Client to the PlaystationStoreClient interface
// without creating an import cycle between internal/api and internal/services/playstationstore.
type playstationStoreClientAdapter struct{ c *playstationstoresvc.Client }

func (a *playstationStoreClientAdapter) GetAccountInfo(ctx context.Context, npssoToken string) (*PlaystationStoreAccountInfo, error) {
	info, err := a.c.GetAccountInfo(ctx, npssoToken)
	if err != nil {
		if errors.Is(err, playstationstoresvc.ErrInvalidNPSSOToken) {
			return nil, ErrInvalidNPSSOToken
		}
		return nil, err
	}
	return &PlaystationStoreAccountInfo{
		OnlineID:  info.OnlineID,
		AccountID: info.AccountID,
	}, nil
}

// epicGamesStoreClientAdapter bridges epicgamesstoresvc.Client to the EpicGamesStoreClient interface
// without creating an import cycle between internal/api and internal/services/epicgamesstore.
type epicGamesStoreClientAdapter struct{ c *epicgamesstoresvc.Client }

func (a *epicGamesStoreClientAdapter) Authenticate(ctx context.Context, userID, authCode string) (*EpicGamesStoreAccountInfo, map[string]string, error) {
	info, snapshot, err := a.c.Authenticate(ctx, userID, authCode)
	if err != nil {
		return nil, nil, err
	}
	return &EpicGamesStoreAccountInfo{DisplayName: info.DisplayName, AccountID: info.AccountID}, snapshot, nil
}

func (a *epicGamesStoreClientAdapter) Cleanup(ctx context.Context, userID string) error {
	return a.c.Cleanup(ctx, userID)
}

func (a *epicGamesStoreClientAdapter) Configured() bool {
	return a.c.Configured()
}

// gogClientAdapter bridges gogsvc.Client to the GOGClient interface
// without creating an import cycle between internal/api and internal/services/gog.
type gogClientAdapter struct{ c *gogsvc.Client }

func (a *gogClientAdapter) BuildAuthURL() string {
	return a.c.BuildAuthURL()
}

func (a *gogClientAdapter) ExchangeCode(ctx context.Context, code string) (*GOGTokenResponse, error) {
	tok, err := a.c.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return &GOGTokenResponse{
		RefreshToken: tok.RefreshToken,
		Username:     tok.Username,
	}, nil
}

// humbleClientAdapter bridges humblesvc.Client to the HumbleClient interface
// without creating an import cycle between internal/api and internal/services/humble.
type humbleClientAdapter struct{ c *humblesvc.Client }

func (a *humbleClientAdapter) Verify(ctx context.Context, sessionCookie string) error {
	err := a.c.Verify(ctx, sessionCookie)
	if errors.Is(err, humblesvc.ErrCredentials) {
		return ErrInvalidHumbleCookie
	}
	return err
}

func spaHandler() echo.HandlerFunc {
	fsys, err := fs.Sub(ui.UIBox, "frontend/dist")
	if err != nil {
		panic(fmt.Sprintf("api: failed to create SPA sub-FS: %v", err))
	}
	fileServer := http.FileServer(http.FS(fsys))
	return func(c *echo.Context) error {
		path := c.Request().URL.Path
		// Never serve the SPA shell for unmatched API routes — return a JSON
		// 404 instead. Otherwise a mistyped or trailing-slash API path (e.g.
		// "/api/jobs/" vs the registered "/api/jobs") silently returns
		// index.html with a 200, which clients then fail to parse as JSON.
		if strings.HasPrefix(path, "/api/") {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		if _, err := fs.Stat(fsys, strings.TrimPrefix(path, "/")); err != nil {
			// File not found → serve index.html for SPA routing
			c.Request().URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
