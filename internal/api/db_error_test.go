package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/migrate"
)

func TestDBErrorPage_ServesHTML(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	dh := api.NewDBErrorHandler("postgres://user:secret@db.example.com:5432/nexorious?sslmode=require", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "db.example.com") {
		t.Error("body should contain host")
	}
	if strings.Contains(body, "secret") {
		t.Error("body must not contain plaintext password")
	}
	if !strings.Contains(body, "***") {
		t.Error("body should contain *** for redacted password")
	}
}

func TestDBErrorPage_RedirectsOnRecovery(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error?from=/foo", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/foo" {
		t.Errorf("expected Location=/foo, got %q", loc)
	}
}

func TestDBErrorPage_RedirectsToRootWithNoFrom(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("expected Location=/, got %q", loc)
	}
}

func TestDBErrorPage_RejectsExternalFrom(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error?from=https://evil.com", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("expected Location=/ to block open-redirect, got %q", loc)
	}
}

func TestDBErrorHandler_RedactsDSN(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	dh := api.NewDBErrorHandler("postgres://myuser:supersecret@db.example.com:5432/nexorious?sslmode=require&password=leak", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "supersecret") {
		t.Error("password must be redacted")
	}
	if strings.Contains(body, "leak") {
		t.Error("password query param must be redacted")
	}
	if !strings.Contains(body, "myuser") {
		t.Error("username should be visible")
	}
}

func TestDBErrorHandler_InjectsUnknownWhenNeverUnavailable(t *testing.T) {
	migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
	dh := api.NewDBErrorHandler("postgres://u:p@h/db", migrator)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := dh.HandleDBError(c); err != nil {
		t.Fatalf("HandleDBError: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "unknown") {
		t.Errorf("expected 'unknown' for never-unavailable timestamp, body: %s", body)
	}
}
