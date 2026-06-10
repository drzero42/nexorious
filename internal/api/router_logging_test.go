package api

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

// buildLoggingEcho sets up the request-id + request-logging middleware exactly as
// router.New does (request_id/user_id are injected by logging.ContextHandler from
// ctx, never passed as explicit attrs) and writes log output into buf for
// assertions. The /login handler seeds a user_id into the request ctx to mimic
// AuthMiddleware on an authenticated route.
func buildLoggingEcho(buf *bytes.Buffer) *echo.Echo {
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(buf, nil))))
	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogURI: true, LogMethod: true, LogLatency: true,
		LogRoutePath: true, HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			slog.InfoContext(c.Request().Context(), "request",
				"method", v.Method, "uri", v.URI,
				logging.KeyRoute, v.RoutePath, logging.KeyStatus, v.Status,
				logging.KeyLatency, v.Latency)
			return nil
		},
	}))
	e.POST("/login", func(c *echo.Context) error {
		c.SetRequest(c.Request().WithContext(logging.WithUserID(c.Request().Context(), "u1")))
		return c.NoContent(http.StatusOK)
	})
	return e
}

func TestRequestLogger_NoSecretLeak(t *testing.T) {
	var buf bytes.Buffer
	e := buildLoggingEcho(&buf)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"password":"hunter2"}`))
	req.Header.Set("Authorization", "Bearer SECRETTOKEN")
	req.Header.Set("Cookie", "session=SECRETSESSION")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	out := buf.String()
	for _, secret := range []string{"hunter2", "SECRETTOKEN", "SECRETSESSION"} {
		if strings.Contains(out, secret) {
			t.Errorf("request log leaked %q: %s", secret, out)
		}
	}
}

// TestRequestLogger_NoDuplicateCorrelationKeys guards against the request log line
// carrying request_id/user_id twice — once from an explicit attr and once injected
// from ctx by ContextHandler. A JSON-into-map decode silently collapses duplicate
// keys, so this asserts on the raw bytes instead.
func TestRequestLogger_NoDuplicateCorrelationKeys(t *testing.T) {
	var buf bytes.Buffer
	e := buildLoggingEcho(&buf)

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	out := buf.String()
	if got := strings.Count(out, `"request_id"`); got != 1 {
		t.Errorf(`request_id key count = %d, want 1 (duplicate JSON key); line: %s`, got, out)
	}
	if got := strings.Count(out, `"user_id"`); got != 1 {
		t.Errorf(`user_id key count = %d, want 1 (duplicate JSON key); line: %s`, got, out)
	}
}
