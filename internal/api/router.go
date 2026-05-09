package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	migrate "github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/internal/services/igdb"
	"github.com/drzero42/nexorious-go/ui"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client) *echo.Echo {
	e := echo.New()

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

	// Gate 1: DB unavailable — redirect everything except /db-error and /health
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			state := migrator.State()
			if state == migrate.AppStateDBUnavailable {
				path := c.Request().URL.Path
				if path == "/db-error" || path == "/health" {
					return next(c)
				}
				return c.Redirect(http.StatusFound,
					"/db-error?from="+url.QueryEscape(c.Request().RequestURI))
			}
			return next(c)
		}
	})

	// Gate 2: migrations pending — redirect everything except /migrate*, /api/migrate*, /health
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			state := migrator.State()
			if state != migrate.AppStateReady && state != migrate.AppStateDBUnavailable {
				path := c.Request().URL.Path
				if strings.HasPrefix(path, "/migrate") || strings.HasPrefix(path, "/api/migrate") || path == "/health" {
					return next(c)
				}
				return c.Redirect(http.StatusFound, "/migrate")
			}
			return next(c)
		}
	})

	// Gate 3: setup required — redirect everything except /setup, /api/auth/setup/*, /health, /api/migrate*
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if migrator.NeedsSetup() {
				path := c.Request().URL.Path
				if path == "/setup" || strings.HasPrefix(path, "/api/auth/setup") ||
					path == "/health" || strings.HasPrefix(path, "/api/migrate") {
					return next(c)
				}
				return c.Redirect(http.StatusFound, "/setup")
			}
			return next(c)
		}
	})

	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
		}))
	}

	mh := migrate.NewHandler(migrator, db)
	registerRoutes(e, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient)

	return e
}

func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client) {
	// Migration routes (bypass gate 2 via prefix)
	e.GET("/migrate", mh.HandleMigrateUI)
	e.GET("/api/migrate/status", mh.HandleStatus)
	e.POST("/api/migrate/run", mh.HandleRun)
	e.GET("/api/migrate/progress", mh.HandleProgress)

	// Health check — bypassed by all gates
	igdbConfigured := igdbClient != nil && igdbClient.Configured()
	e.GET("/health", func(c *echo.Context) error {
		state := migrator.State()
		status := "ok"
		if state != migrate.AppStateReady {
			status = state.String()
		}
		return c.JSON(http.StatusOK, map[string]any{
			"status":          status,
			"igdb_configured": igdbConfigured,
		})
	})

	// DB-error route (bypassed by Gate 1)
	dh := NewDBErrorHandler(resolvedDatabaseURL, migrator)
	e.GET("/db-error", dh.HandleDBError)

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
	e.POST("/api/auth/setup/restore", func(c *echo.Context) error {
		return c.JSON(http.StatusNotImplemented, map[string]string{
			"error": "not implemented — deferred to Phase 3",
		})
	})

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
		platformsGroup.GET("/storefronts/:storefront", ph.HandleGetStorefront)
		platformsGroup.GET("/storefronts/", ph.HandleListStorefronts)
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
		gh := NewGamesHandler(db, igdbClient, cfg)
		gamesGroup := e.Group("/api/games", auth.JWTMiddleware(cfg.SecretKey, db))
		gamesGroup.GET("", gh.HandleListGames)
		gamesGroup.GET("/:id", gh.HandleGetGame)
		gamesGroup.POST("/search/igdb", gh.HandleSearchIGDB)
		gamesGroup.GET("/igdb/:igdb_id", gh.HandleGetIGDBGame)
		gamesGroup.POST("/igdb-import", gh.HandleImportFromIGDB)

		// User Games routes (all JWT-protected)
		ugh := NewUserGamesHandler(db, cfg)
		userGamesGroup := e.Group("/api/user-games", auth.JWTMiddleware(cfg.SecretKey, db))
		userGamesGroup.GET("", ugh.HandleListUserGames)
		userGamesGroup.POST("", ugh.HandleCreateUserGame)
		userGamesGroup.PUT("/bulk-update", ugh.HandleBulkUpdate)
		userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
		userGamesGroup.POST("/bulk-add-platforms", ugh.HandleBulkAddPlatforms)
		userGamesGroup.DELETE("/bulk-remove-platforms", ugh.HandleBulkRemovePlatforms)
		userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
		userGamesGroup.PUT("/:id", ugh.HandleUpdateUserGame)
		userGamesGroup.DELETE("/:id", ugh.HandleDeleteUserGame)
		userGamesGroup.PUT("/:id/progress", ugh.HandleUpdateProgress)
		userGamesGroup.GET("/:id/platforms", ugh.HandleListPlatforms)
		userGamesGroup.POST("/:id/platforms", ugh.HandleCreatePlatform)
		userGamesGroup.PUT("/:id/platforms/:platform_id", ugh.HandleUpdatePlatform)
		userGamesGroup.DELETE("/:id/platforms/:platform_id", ugh.HandleDeletePlatform)
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

func spaHandler() echo.HandlerFunc {
	fsys, err := fs.Sub(ui.UIBox, "dist")
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
