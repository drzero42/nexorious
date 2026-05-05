package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious-go/internal/config"
	migrate "github.com/drzero42/nexorious-go/internal/migrate"
	"github.com/drzero42/nexorious-go/ui"
)

// New creates and configures the Echo instance with all middleware and routes.
// The caller is responsible for configuring the global slog logger before calling New.
func New(cfg *config.Config, migrator *migrate.Migrator) *echo.Echo {
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

	// App-state middleware: redirect to /migrate unless state is Ready or path is bypassed.
	bypassPrefixes := []string{"/migrate", "/api/migrate"}
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if migrator.State() != migrate.AppStateReady {
				path := c.Request().URL.Path
				for _, prefix := range bypassPrefixes {
					if strings.HasPrefix(path, prefix) {
						return next(c)
					}
				}
				return c.Redirect(http.StatusFound, "/migrate")
			}
			return next(c)
		}
	})

	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
		}))
	}

	mh := migrate.NewHandler(migrator)
	registerRoutes(e, cfg, mh)

	return e
}

func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler) {
	// Migration routes (bypass app-state middleware via prefix list)
	e.GET("/migrate", mh.HandleMigrateUI)
	e.GET("/api/migrate/status", mh.HandleStatus)
	e.POST("/api/migrate/run", mh.HandleRun)
	e.GET("/api/migrate/progress", mh.HandleProgress)

	// Health check
	e.GET("/health", handleHealth)

	// Static cover art files from disk
	e.GET("/static/cover_art/*", func(c *echo.Context) error {
		http.StripPrefix("/static/cover_art/", http.FileServer(http.Dir(cfg.StoragePath+"/cover_art/"))).
			ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// SPA catch-all — serves ui.UIBox; falls back to index.html
	e.GET("/*", spaHandler())
}

// handleHealth returns 200 OK with a JSON body.
// GET /health
func handleHealth(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
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
