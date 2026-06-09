package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettings_GetDefaultAndPatch(t *testing.T) {
	truncateAllTables(t)
	userID := "u-settings-testuser1"
	insertAuthTestUser(t, testDB, userID, "settings-testuser1", "pass123", true, false)

	// Use the full Echo test harness via newTestEcho + session cookie
	// (same pattern as auth_test.go and games_test.go).
	e := newTestEcho(t, testDB, testCfg())
	sessionID := insertAuthTestSession(t, testDB, userID)

	doGetSettings := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	doPatchSettings := func(t *testing.T, bodyJSON string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/api/settings", bytes.NewBufferString(bodyJSON))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_id", Value: sessionID})
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	decodeResp := func(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("unmarshal settings response: %v", err)
		}
		return m
	}

	// GET with no row returns the default deal_region "us".
	rec := doGetSettings(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeResp(t, rec)
	if got["deal_region"] != "us" {
		t.Fatalf("default deal_region want us, got %v", got["deal_region"])
	}

	// PATCH to a valid region round-trips.
	rec = doPatchSettings(t, `{"deal_region":"gb"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeResp(t, rec)
	if got["deal_region"] != "gb" {
		t.Fatalf("PATCH response want deal_region=gb, got %v", got["deal_region"])
	}

	// GET again returns the persisted region.
	rec = doGetSettings(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET after PATCH want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeResp(t, rec)
	if got["deal_region"] != "gb" {
		t.Fatalf("GET after PATCH want deal_region=gb, got %v", got["deal_region"])
	}

	// PATCH with an empty body must preserve the existing value (partial PATCH).
	// This guards the pre-SELECT that loads the current row before the upsert:
	// omitting deal_region must NOT reset it to the "us" default.
	rec = doPatchSettings(t, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty PATCH want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeResp(t, rec)
	if got["deal_region"] != "gb" {
		t.Fatalf("empty PATCH want deal_region preserved as gb, got %v", got["deal_region"])
	}

	// PATCH with invalid region is rejected with 422.
	rec = doPatchSettings(t, `{"deal_region":"zz"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for invalid region, got %d: %s", rec.Code, rec.Body.String())
	}
}
