package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The shared test container connects as its POSTGRES_USER, a superuser — the
// exact setup #1357 warns about.
func TestAdminHealthWarnings_ReportsPrivilegedDBRole(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, adminTok := setupAdminUser(t, testDB, e, "health-admin")

	rec := getAuth(t, e, "/api/admin/health-warnings", adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Warnings []struct {
			Code string `json:"code"`
			Role string `json:"role"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Warnings) != 1 || resp.Warnings[0].Code != "db_role_privileged" || resp.Warnings[0].Role != "test" {
		t.Fatalf("warnings = %+v, want one db_role_privileged for role test", resp.Warnings)
	}
}

// Security posture is admin-only information.
func TestAdminHealthWarnings_NonAdminForbidden(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	_, regTok := setupRegularUser(t, testDB, e, "health-regular")

	rec := getAuth(t, e, "/api/admin/health-warnings", regTok)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d: %s", rec.Code, rec.Body.String())
	}
}
