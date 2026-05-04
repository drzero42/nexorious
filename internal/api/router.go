package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/drzero42/nexorious-go/internal/config"
)

// New creates and configures the Echo instance with all middleware and routes.
func New(cfg *config.Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

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
