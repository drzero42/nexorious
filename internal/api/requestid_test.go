package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/logging"
)

func TestRequestIDMiddleware_GeneratesAndPropagates(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seenInCtx, seenHeader string
	e.GET("/x", func(c *echo.Context) error {
		seenInCtx = logging.RequestIDForTest(c.Request().Context())
		seenHeader = c.Response().Header().Get(echo.HeaderXRequestID)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if seenInCtx == "" {
		t.Error("request id not present in request context")
	}
	if seenHeader == "" || seenHeader != seenInCtx {
		t.Errorf("response header %q should match ctx id %q", seenHeader, seenInCtx)
	}
}

func TestRequestIDMiddleware_HonorsInbound(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seen string
	e.GET("/x", func(c *echo.Context) error {
		seen = logging.RequestIDForTest(c.Request().Context())
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(echo.HeaderXRequestID, "inbound-abc")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if seen != "inbound-abc" {
		t.Errorf("ctx request id = %q, want inbound-abc", seen)
	}
}
