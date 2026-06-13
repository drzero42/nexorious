package api

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

// maxInboundRequestIDLen bounds how long an honored inbound X-Request-Id may be.
// The id is stamped on every log line of the request, so an unbounded
// caller-controlled value (Go accepts headers up to ~1MB) would bloat the logs
// and could smuggle log-injection payloads; an over-long or non-printable value
// is discarded in favor of a freshly generated id.
const maxInboundRequestIDLen = 64

// RequestIDMiddleware ensures every request carries an id: it honors an inbound
// X-Request-Id header if present and trustworthy, otherwise generates one. The
// id is echoed back in the response header and seeded into the request context
// so the slog ContextHandler stamps it on every in-request log line.
func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := c.Request().Header.Get(echo.HeaderXRequestID)
			if !trustworthyRequestID(id) {
				id = uuid.NewString()
			}
			c.Response().Header().Set(echo.HeaderXRequestID, id)
			ctx := logging.WithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

// trustworthyRequestID reports whether an inbound X-Request-Id may be honored:
// non-empty, within maxInboundRequestIDLen, and printable ASCII only (no
// control characters, spaces, or non-ASCII bytes that could corrupt or inject
// into log lines).
func trustworthyRequestID(id string) bool {
	if id == "" || len(id) > maxInboundRequestIDLen {
		return false
	}
	for _, r := range id {
		if r < '!' || r > '~' {
			return false
		}
	}
	return true
}
