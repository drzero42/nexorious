package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetDoc_AuthedUserGetsUserGuide(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupRegularUser(t, testDB, e, "doc-ug")

	rec := getAuth(t, e, "/api/docs/user-guide", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty markdown body")
	}
}

func TestGetDoc_AnonymousIsUnauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())

	req := httptest.NewRequest(http.MethodGet, "/api/docs/user-guide", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body)
	}
}

func TestGetDoc_AdminGuideForbiddenForRegularUser(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupRegularUser(t, testDB, e, "doc-ag-reg")

	rec := getAuth(t, e, "/api/docs/admin-guide", tok)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body)
	}
}

func TestGetDoc_AdminGuideAllowedForAdmin(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupAdminUser(t, testDB, e, "doc-ag-adm")

	rec := getAuth(t, e, "/api/docs/admin-guide", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
}

func TestGetDoc_UnknownSlugIs404(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupRegularUser(t, testDB, e, "doc-404")

	rec := getAuth(t, e, "/api/docs/does-not-exist", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

func TestGetDoc_InvalidSlugRejected(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, tok := setupRegularUser(t, testDB, e, "doc-bad")

	rec := getAuth(t, e, "/api/docs/..%2f..%2fgo.mod", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal slug returned %d, want 404; body=%s", rec.Code, rec.Body)
	}
}
