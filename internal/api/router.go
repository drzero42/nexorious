package api

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/drzero42/nexorious-go/internal/config"
)

// parseSlogLevel maps a LOG_LEVEL string to a slog.Level.
// Unrecognised values default to Info.
func parseSlogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New creates and configures the Echo instance with all middleware and routes.
func New(cfg *config.Config) *echo.Echo {
	// Configure the global slog logger from cfg.LogLevel.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(cfg.LogLevel),
	})))

	e := echo.New()
	e.Debug = cfg.Debug
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogMethod:   true,
		LogLatency:  true,
		LogError:    true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			if v.Error != nil {
				slog.Error("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency, "err", v.Error)
			} else {
				slog.Info("request", "method", v.Method, "uri", v.URI, "status", v.Status, "latency", v.Latency)
			}
			return nil
		},
	}))

	// Routes
	registerRoutes(e)

	return e
}

func registerRoutes(e *echo.Echo) {
	e.GET("/health", handleHealth)
}

// handleHealth returns 200 OK with a JSON body.
// GET /health
func handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
