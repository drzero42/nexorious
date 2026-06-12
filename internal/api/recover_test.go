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

// TestObservabilityMiddleware_PanicEmitsBothLines drives a panicking handler
// through the exact production middleware stack (registerObservabilityMiddleware)
// and asserts BOTH signals fire: the category=panic line (carrying the stack
// trace) AND the "request" access-log line with a resolved 500 status. The old
// ordering (Recover outside RequestLogger) silently dropped the access-log line
// for panics — this test would have caught issue #943 item 1.
func TestObservabilityMiddleware_PanicEmitsBothLines(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(logging.NewContextHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	defer slog.SetDefault(prev)

	e := echo.New()
	registerObservabilityMiddleware(e)
	e.GET("/boom", func(_ *echo.Context) error { panic("kaboom") })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	var panicLine, accessLine map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		switch {
		case m[logging.KeyCategory] == "panic":
			panicLine = m
		case m["msg"] == "request":
			accessLine = m
		}
	}

	if panicLine == nil {
		t.Fatalf("no category=panic line emitted; got:\n%s", buf.String())
	}
	if stack, _ := panicLine[logging.KeyStack].(string); stack == "" {
		t.Errorf("panic line missing stack trace; got %v", panicLine[logging.KeyStack])
	}

	if accessLine == nil {
		t.Fatalf("no access-log line emitted for panicking request; got:\n%s", buf.String())
	}
	if accessLine[logging.KeyStatus] != float64(http.StatusInternalServerError) {
		t.Errorf("access-log status = %v, want 500", accessLine[logging.KeyStatus])
	}
}

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

	// This focused test wires only PanicLogger+Recover (no RequestLogger), so the
	// only line emitted is the panic line; it carries category=panic. The full
	// stack including the access-log line is covered by
	// TestObservabilityMiddleware_PanicEmitsBothLines.
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
