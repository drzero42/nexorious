package api

import (
	"errors"
	"log/slog"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

// PanicLogger emits a structured category=panic error line for panics recovered
// by the downstream middleware.Recover(). Echo v5's Recover converts a panic into
// a *middleware.PanicStackError and returns it up the chain (it exposes no logging
// hook of its own); registering PanicLogger immediately outside Recover lets us
// detect that error and log a distinct panic signal — separate from the HTTP 500
// access-log line — correlated by the request_id already in the request ctx.
// RequestIDMiddleware (registered inside this one) seeds request_id via
// c.SetRequest, which mutates the shared *echo.Context in place, so the id is
// present on c.Request().Context() by the time the panic error unwinds back here.
func PanicLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			var pse *middleware.PanicStackError
			if errors.As(err, &pse) {
				slog.ErrorContext(c.Request().Context(), "http: recovered panic",
					logging.KeyErr, pse.Err,
					logging.Cat(logging.CategoryPanic),
				)
			}
			return err
		}
	}
}
