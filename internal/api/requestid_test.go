package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRequestIDMiddleware_RejectsUntrustworthyInbound(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seen string
	e.GET("/x", func(c *echo.Context) error {
		seen = logging.RequestIDForTest(c.Request().Context())
		return c.NoContent(http.StatusOK)
	})

	cases := map[string]string{
		"too long":      strings.Repeat("a", 65),
		"newline":       "abc\ndef",
		"control char":  "abc\x00def",
		"non-ascii":     "abc def",
		"leading space": " abc",
		"tab":           "ab\tcd",
		"empty inbound": "",
	}
	for name, inbound := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if inbound != "" {
				req.Header.Set(echo.HeaderXRequestID, inbound)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if seen == "" {
				t.Fatal("middleware must always set a request id")
			}
			if seen == inbound {
				t.Errorf("untrustworthy inbound id %q was trusted verbatim", inbound)
			}
			if len(seen) > 64 {
				t.Errorf("generated id is too long: %q", seen)
			}
		})
	}
}

func TestRequestIDMiddleware_HonorsAcceptableInbound(t *testing.T) {
	e := echo.New()
	e.Use(RequestIDMiddleware())

	var seen string
	e.GET("/x", func(c *echo.Context) error {
		seen = logging.RequestIDForTest(c.Request().Context())
		return c.NoContent(http.StatusOK)
	})

	// Exactly 64 printable ASCII chars — the boundary — must be honored.
	inbound := strings.Repeat("z", 64)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(echo.HeaderXRequestID, inbound)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if seen != inbound {
		t.Errorf("ctx request id = %q, want %q (64-char printable id should be honored)", seen, inbound)
	}
}
