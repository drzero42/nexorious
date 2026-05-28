package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/auth"
)

func TestGenerateSessionID(t *testing.T) {
	id1, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	if len(id1) != 64 {
		t.Errorf("GenerateSessionID() length = %d, want 64", len(id1))
	}
	id2, err := auth.GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID() error = %v", err)
	}
	if id1 == id2 {
		t.Error("GenerateSessionID() returned duplicate values")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(key, "nxr_") {
		t.Errorf("GenerateAPIKey() = %q, want prefix %q", key, "nxr_")
	}
	if len(key) != 68 {
		t.Errorf("GenerateAPIKey() length = %d, want 68 (4 prefix + 64 hex)", len(key))
	}
	key2, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() second call error = %v", err)
	}
	if key == key2 {
		t.Error("GenerateAPIKey() returned duplicate values")
	}
}

func newSessionTestContext(t *testing.T) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSetSessionCookie(t *testing.T) {
	c, rec := newSessionTestContext(t)
	auth.SetSessionCookie(c, "test-session-id-value", 30)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "session_id" {
		t.Errorf("Name = %q, want %q", cookie.Name, "session_id")
	}
	if cookie.Value != "test-session-id-value" {
		t.Errorf("Value = %q, want %q", cookie.Value, "test-session-id-value")
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly = false, want true")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if !cookie.Secure {
		t.Error("Secure = false, want true")
	}
	if cookie.MaxAge != 30*86400 {
		t.Errorf("MaxAge = %d, want %d", cookie.MaxAge, 30*86400)
	}
}

func TestClearSessionCookie(t *testing.T) {
	c, rec := newSessionTestContext(t)
	auth.ClearSessionCookie(c)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session_id" {
		t.Errorf("Name = %q, want %q", cookies[0].Name, "session_id")
	}
	if cookies[0].MaxAge != 0 {
		t.Errorf("MaxAge = %d, want 0", cookies[0].MaxAge)
	}
}
