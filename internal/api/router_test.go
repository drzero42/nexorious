package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{Port: 8000, LogLevel: "info", Debug: false}
	e := api.New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != `{"status":"ok"}`+"\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}
