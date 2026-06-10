package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/drzero42/nexorious/internal/logging"
)

func TestPanicLogger_EmitsPanicLine(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(logging.NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	e := echo.New()
	e.Use(PanicLogger())
	e.Use(middleware.Recover())
	e.Use(RequestIDMiddleware())
	e.GET("/boom", func(_ *echo.Context) error { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	// Find the panic line among the emitted JSON lines (the access-log line is
	// also emitted; only the panic line carries category=panic).
	var found map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) == nil && m[logging.KeyCategory] == "panic" {
			found = m
		}
	}
	if found == nil {
		t.Fatalf("no category=panic line emitted; got:\n%s", buf.String())
	}
	if found["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", found["level"])
	}
	if found[logging.KeyRequestID] == nil || found[logging.KeyRequestID] == "" {
		t.Errorf("panic line missing request_id correlation; got %v", found[logging.KeyRequestID])
	}
}
