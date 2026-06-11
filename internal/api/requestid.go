package api

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

// RequestIDMiddleware ensures every request carries an id: it honors an inbound
// X-Request-Id header if present, otherwise generates one. The id is echoed back
// in the response header and seeded into the request context so the slog
// ContextHandler stamps it on every in-request log line.
func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := c.Request().Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = uuid.NewString()
			}
			c.Response().Header().Set(echo.HeaderXRequestID, id)
			ctx := logging.WithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
