package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	migrate "github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/ui"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config, migrator *migrate.Migrator, pool *pgxpool.Pool, resolvedDatabaseURL string) *echo.Echo {
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

	mh := migrate.NewHandler(migrator, pool)
	registerRoutes(e, cfg, mh, pool, migrator, resolvedDatabaseURL)

	return e
}

func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, pool *pgxpool.Pool, migrator *migrate.Migrator, resolvedDatabaseURL string) {
	// Migration routes (bypass gate 2 via prefix)
	e.GET("/migrate", mh.HandleMigrateUI)
	e.GET("/api/migrate/status", mh.HandleStatus)
	e.POST("/api/migrate/run", mh.HandleRun)
	e.GET("/api/migrate/progress", mh.HandleProgress)

	// Health check — bypassed by all gates
	e.GET("/health", func(c *echo.Context) error {
		state := migrator.State()
		if state == migrate.AppStateReady {
			return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": state.String()})
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
	sh := NewSetupHandler(pool, cfg, migrator)
	e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)
	e.POST("/api/auth/setup/restore", func(c *echo.Context) error {
		return c.JSON(http.StatusNotImplemented, map[string]string{
			"error": "not implemented — deferred to Phase 3",
		})
	})

	// Auth routes — only registered when a DB pool is available.
	if pool != nil {
		ah := NewAuthHandler(pool, cfg)

		// Public auth routes (no JWT required)
		e.POST("/api/auth/login", ah.HandleLogin)
		e.POST("/api/auth/refresh", ah.HandleRefresh)

		// JWT-protected auth routes
		authGroup := e.Group("/api/auth", auth.JWTMiddleware(cfg.SecretKey, pool))
		authGroup.POST("/logout", ah.HandleLogout)
		authGroup.GET("/me", ah.HandleGetMe)
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
