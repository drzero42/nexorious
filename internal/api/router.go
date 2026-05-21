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
	maint "github.com/drzero42/nexorious/internal/middleware"
	migrate "github.com/drzero42/nexorious/internal/migrate"
	epicsvc "github.com/drzero42/nexorious/internal/services/epic"
	gogsvc "github.com/drzero42/nexorious/internal/services/gog"
	"github.com/drzero42/nexorious/internal/services/igdb"
	psnsvc "github.com/drzero42/nexorious/internal/services/psn"
	steamsvc "github.com/drzero42/nexorious/internal/services/steam"
	"github.com/drzero42/nexorious/ui"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
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
				return c.Redirect(http.StatusFound, "/setup")
			}
			return next(c)
		}
	})

	// Gate 4: Maintenance mode — blocks most requests during restore
	e.Use(maint.MaintenanceMiddleware())

	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
		}))
	}

	mh := migrate.NewHandler(migrator, db)
	registerRoutes(e, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, rc)

	return e
}

func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient *river.Client[pgx.Tx]) {
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

	// Auth routes — only registered when a DB is available.
	if db != nil {
		ah := NewAuthHandler(db, cfg)

		// Public auth routes (no JWT required)
		e.POST("/api/auth/login", ah.HandleLogin)
		e.POST("/api/auth/refresh", ah.HandleRefresh)

		// JWT-protected auth routes
		authGroup := e.Group("/api/auth", auth.JWTMiddleware(cfg.SecretKey, db))
		authGroup.POST("/logout", ah.HandleLogout)
		authGroup.GET("/me", ah.HandleGetMe)
		authGroup.PUT("/me", ah.HandleUpdateMe)
		authGroup.PUT("/change-password", ah.HandleChangePassword)
		authGroup.GET("/username/check/:username", ah.HandleCheckUsername)
		authGroup.PUT("/username", ah.HandleChangeUsername)

		// Platform and storefront routes (all JWT-protected)
		ph := NewPlatformsHandler(db)
		platformsGroup := e.Group("/api/platforms", auth.JWTMiddleware(cfg.SecretKey, db))
		platformsGroup.GET("", ph.HandleListPlatforms)
		platformsGroup.GET("/simple-list", ph.HandleSimpleList)
		platformsGroup.GET("/storefronts/simple-list", ph.HandleStorefrontSimpleList)
		platformsGroup.GET("/storefronts", ph.HandleListStorefronts)
		platformsGroup.GET("/storefronts/:storefront", ph.HandleGetStorefront)
		platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
		platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
		platformsGroup.GET("/:platform", ph.HandleGetPlatform)

		// Tag routes (all JWT-protected)
		th := NewTagsHandler(db)
		tagsGroup := e.Group("/api/tags", auth.JWTMiddleware(cfg.SecretKey, db))
		tagsGroup.GET("", th.HandleListTags)
		tagsGroup.POST("", th.HandleCreateTag)
		tagsGroup.PUT("/:id", th.HandleUpdateTag)
		tagsGroup.DELETE("/:id", th.HandleDeleteTag)

		// Games routes (all JWT-protected)
		gh := NewGamesHandler(db, igdbClient, cfg, riverClient)
		gamesGroup := e.Group("/api/games", auth.JWTMiddleware(cfg.SecretKey, db))
		gamesGroup.GET("", gh.HandleListGames)
		gamesGroup.POST("/metadata/refresh-job", gh.HandleStartMetadataRefreshJob)
		gamesGroup.POST("/search/igdb", gh.HandleSearchIGDB)
		gamesGroup.GET("/igdb/:igdb_id", gh.HandleGetIGDBGame)
		gamesGroup.POST("/igdb-import", gh.HandleImportFromIGDB)
		gamesGroup.GET("/:id", gh.HandleGetGame)

		// User Games routes (all JWT-protected)
		ugh := NewUserGamesHandler(db, cfg)
		userGamesGroup := e.Group("/api/user-games", auth.JWTMiddleware(cfg.SecretKey, db))
		userGamesGroup.GET("", ugh.HandleListUserGames)
		userGamesGroup.POST("", ugh.HandleCreateUserGame)
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
		userGamesGroup.PUT("/:id/platforms/:platform_id", ugh.HandleUpdatePlatform)
		userGamesGroup.DELETE("/:id/platforms/:platform_id", ugh.HandleDeletePlatform)

		// Jobs routes (all JWT-protected)
		jh := NewJobsHandler(db, riverClient)
		jobsGroup := e.Group("/api/jobs", auth.JWTMiddleware(cfg.SecretKey, db))
		jobsGroup.GET("", jh.HandleListJobs)
		jobsGroup.GET("/summary", jh.HandleJobsSummary)
		jobsGroup.GET("/pending-review-count", jh.HandlePendingReviewCount)
		jobsGroup.GET("/active/:job_type", jh.HandleActiveJob)
		jobsGroup.GET("/recent/:source", jh.HandleRecentJobs)
		jobsGroup.GET("/:id", jh.HandleGetJob)
		jobsGroup.GET("/:id/items", jh.HandleGetJobItems)
		jobsGroup.POST("/:id/cancel", jh.HandleCancelJob)
		jobsGroup.DELETE("/:id", jh.HandleDeleteJob)
		jobsGroup.POST("/:id/retry-failed", jh.HandleRetryFailed)

		// Job Items routes (all JWT-protected)
		jih := NewJobItemsHandler(db, riverClient)
		jobItemsGroup := e.Group("/api/job-items", auth.JWTMiddleware(cfg.SecretKey, db))
		jobItemsGroup.GET("/:id", jih.HandleGetJobItem)
		jobItemsGroup.POST("/:id/resolve", jih.HandleResolveItem)
		jobItemsGroup.POST("/:id/skip", jih.HandleSkipItem)
		jobItemsGroup.POST("/:id/retry", jih.HandleRetryItem)

		// Import routes (all JWT-protected)
		imh := NewImportHandler(db, riverClient)
		importGroup := e.Group("/api/import", auth.JWTMiddleware(cfg.SecretKey, db))
		importGroup.POST("/nexorious", imh.HandleImportNexorious)

		// Export routes (all JWT-protected)
		exh := NewExportHandler(db, riverClient, cfg)
		exportGroup := e.Group("/api/export", auth.JWTMiddleware(cfg.SecretKey, db))
		exportGroup.POST("/json", exh.HandleExportJSON)
		exportGroup.POST("/csv", exh.HandleExportCSV)
		exportGroup.GET("/:id/download", exh.HandleDownload)

		// Admin backup routes (JWT + admin required)
		adminGroup := e.Group("", auth.JWTMiddleware(cfg.SecretKey, db), auth.AdminMiddleware())
		adminBackups := adminGroup.Group("/api/admin/backups")
		adminBackups.GET("/config", bh.HandleGetConfig)
		adminBackups.PUT("/config", bh.HandleUpdateConfig)
		adminBackups.GET("", bh.HandleListBackups)
		adminBackups.POST("", bh.HandleCreateBackup)
		adminBackups.DELETE("/:id", bh.HandleDeleteBackup)
		adminBackups.GET("/:id/download", bh.HandleDownloadBackup)
		adminBackups.POST("/:id/restore", bh.HandleRestore)
		adminBackups.POST("/restore/upload", bh.HandleRestoreUpload)

		// Admin user management routes (JWT + admin required)
		auh := NewAdminUsersHandler(db)
		auh.RegisterRoutes(adminGroup)

		// Sync routes (all JWT-protected)
		steamSvc := steamsvc.NewClient()
		psnSvc := psnsvc.NewClient()
		epicSvc := epicsvc.NewClient(cfg.LegendaryWorkDir)
		gogSvc := gogsvc.NewClient()
		synch := NewSyncHandler(db, riverClient, &steamClientAdapter{c: steamSvc}, &psnClientAdapter{c: psnSvc}, &epicClientAdapter{c: epicSvc}, &gogClientAdapter{c: gogSvc})
		syncGroup := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
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

// psnClientAdapter bridges psnsvc.Client to the PSNClient interface
// without creating an import cycle between internal/api and internal/services/psn.
type psnClientAdapter struct{ c *psnsvc.Client }

func (a *psnClientAdapter) GetAccountInfo(ctx context.Context, npssoToken string) (*PSNAccountInfo, error) {
	info, err := a.c.GetAccountInfo(ctx, npssoToken)
	if err != nil {
		if errors.Is(err, psnsvc.ErrInvalidNPSSOToken) {
			return nil, ErrInvalidNPSSOToken
		}
		return nil, err
	}
	return &PSNAccountInfo{
		OnlineID:  info.OnlineID,
		AccountID: info.AccountID,
		Region:    info.Region,
	}, nil
}

// epicClientAdapter bridges epicsvc.Client to the EpicClient interface
// without creating an import cycle between internal/api and internal/services/epic.
type epicClientAdapter struct{ c *epicsvc.Client }

func (a *epicClientAdapter) Authenticate(ctx context.Context, userID, authCode string) (*EpicAccountInfo, map[string]string, error) {
	info, snapshot, err := a.c.Authenticate(ctx, userID, authCode)
	if err != nil {
		return nil, nil, err
	}
	return &EpicAccountInfo{DisplayName: info.DisplayName, AccountID: info.AccountID}, snapshot, nil
}

func (a *epicClientAdapter) Cleanup(ctx context.Context, userID string) error {
	return a.c.Cleanup(ctx, userID)
}

func (a *epicClientAdapter) Configured() bool {
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
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		UserID:       tok.UserID,
		Username:     tok.Username,
	}, nil
}

func spaHandler() echo.HandlerFunc {
	fsys, err := fs.Sub(ui.UIBox, "frontend/dist")
	if err != nil {
		panic(fmt.Sprintf("api: failed to create SPA sub-FS: %v", err))
	}
	fileServer := http.FileServer(http.FS(fsys))
	return func(c *echo.Context) error {
		path := c.Request().URL.Path
		if _, err := fs.Stat(fsys, strings.TrimPrefix(path, "/")); err != nil {
			// File not found → serve index.html for SPA routing
			c.Request().URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}
