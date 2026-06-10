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

// buildLoggingEcho sets up the request-id + request-logging middleware (as in
// router.New, minus the user_id branch which is irrelevant here) and writes log
// output into buf for assertions.
func buildLoggingEcho(buf *bytes.Buffer) *echo.Echo {
	slog.SetDefault(slog.New(logging.NewContextHandler(slog.NewJSONHandler(buf, nil))))
	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogURI: true, LogMethod: true, LogLatency: true,
		LogRoutePath: true, LogRequestID: true, HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			slog.InfoContext(c.Request().Context(), "request",
				"method", v.Method, "uri", v.URI,
				logging.KeyRoute, v.RoutePath, logging.KeyStatus, v.Status,
				logging.KeyLatency, v.Latency, logging.KeyRequestID, v.RequestID)
			return nil
		},
	}))
	e.POST("/login", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
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
