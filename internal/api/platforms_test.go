package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptrace/bun"
)

// getAuth fires a GET request with a Bearer authorization header.
func getAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// ─── Platform list tests ──────────────────────────────────────────────────────

func TestListPlatforms(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	userID := "u-plat-list-1"
	insertAuthTestUser(t, testDB, userID, "platlistuser", "pass123", true, false)
	insertAuthTestSession(t, testDB, userID, "access-plat-list", "refresh-plat-list", 1)
	token := loginAndGetToken(t, e, "platlistuser", "pass123")

	rec := getAuth(t, e, "/api/platforms", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	platforms, ok := resp["platforms"].([]any)
	if !ok || len(platforms) == 0 {
		t.Fatal("expected platforms, got empty list")
	}

	// Find pc-windows and check it has storefronts
	var pcWindows map[string]any
	for _, p := range platforms {
		pm := p.(map[string]any)
		if pm["name"] == "pc-windows" {
			pcWindows = pm
			break
		}
	}
	if pcWindows == nil {
		t.Fatal("expected pc-windows platform in list")
	}
	storefronts, ok := pcWindows["storefronts"].([]any)
	if !ok || len(storefronts) == 0 {
		t.Fatalf("expected pc-windows to have storefronts, got %v", pcWindows["storefronts"])
	}
}

func TestListPlatforms_Unauthorized(t *testing.T) {
	truncateAllTables(t)
	e := newTestEcho(t, testDB, testCfg())
	req := httptest.NewRequest(http.MethodGet, "/api/platforms", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPlatformSimpleList(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	rec := getAuth(t, e, "/api/platforms/simple-list", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected items")
	}
	// Each item should have name and display_name only (no storefronts)
	for _, item := range items {
		if _, ok := item["name"]; !ok {
			t.Fatal("expected name field")
		}
		if _, ok := item["display_name"]; !ok {
			t.Fatal("expected display_name field")
		}
		if _, ok := item["storefronts"]; ok {
			t.Fatal("unexpected storefronts field in simple-list")
		}
	}
}

// ─── Single platform tests ────────────────────────────────────────────────────

func TestGetPlatform(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	t.Run("found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc-windows", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var platform map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &platform); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if platform["name"] != "pc-windows" {
			t.Fatalf("expected pc-windows, got %v", platform["name"])
		}
		storefronts, ok := platform["storefronts"].([]any)
		if !ok || len(storefronts) == 0 {
			t.Fatalf("expected storefronts, got %v", platform["storefronts"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent-platform", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestPlatformStorefronts(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	t.Run("found with storefronts", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc-windows/storefronts", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var storefronts []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &storefronts); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(storefronts) == 0 {
			t.Fatal("expected storefronts for pc-windows")
		}
		// Check steam is in there
		found := false
		for _, s := range storefronts {
			if s["name"] == "steam" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected steam in pc-windows storefronts")
		}
	})

	t.Run("platform not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent-platform/storefronts", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestDefaultStorefront(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	t.Run("with default storefront", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc-windows/default-storefront", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["platform"] != "pc-windows" {
			t.Fatalf("expected platform=pc-windows, got %v", resp["platform"])
		}
		if resp["platform_display_name"] == nil {
			t.Fatal("expected platform_display_name")
		}
		defaultSF, ok := resp["default_storefront"].(map[string]any)
		if !ok || defaultSF == nil {
			t.Fatalf("expected default_storefront object, got %v", resp["default_storefront"])
		}
		if defaultSF["name"] != "steam" {
			t.Fatalf("expected steam as default storefront, got %v", defaultSF["name"])
		}
	})

	t.Run("platform not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent-platform/default-storefront", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// ─── Storefront tests ─────────────────────────────────────────────────────────

func TestListStorefronts(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	rec := getAuth(t, e, "/api/platforms/storefronts", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	storefronts, ok := resp["storefronts"].([]any)
	if !ok || len(storefronts) == 0 {
		t.Fatal("expected storefronts")
	}
}

func TestStorefrontSimpleList(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	rec := getAuth(t, e, "/api/platforms/storefronts/simple-list", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected items")
	}
	for _, item := range items {
		if _, ok := item["name"]; !ok {
			t.Fatal("expected name field")
		}
		if _, ok := item["display_name"]; !ok {
			t.Fatal("expected display_name field")
		}
		if _, ok := item["icon"]; ok {
			t.Fatal("unexpected icon field in simple-list")
		}
	}
}

func TestGetStorefront(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)

	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	t.Run("found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/storefronts/steam", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var sf map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &sf); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if sf["name"] != "steam" {
			t.Fatalf("expected name=steam, got %v", sf["name"])
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/storefronts/nonexistent-sf", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// ─── Test helper utilities ────────────────────────────────────────────────────

// setupUser inserts a test user and returns the username.
func setupUser(t *testing.T, db *bun.DB) string {
	t.Helper()
	username := "testuserplatforms"
	insertAuthTestUser(t, testDB, "u-plat-helper-1", username, "pass123", true, false)
	return username
}

// loginAndGetToken logs in and returns the access token.
func loginAndGetToken(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, username, password string) string {
	t.Helper()
	rec := postJSON(t, handler, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	token, ok := resp["access_token"].(string)
	if !ok || token == "" {
		t.Fatalf("no access_token in login response: %v", resp)
	}
	return token
}

func TestListPlatforms_HasIconURL(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	rec := getAuth(t, e, "/api/platforms", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	platforms, _ := resp["platforms"].([]any)

	var pcWindows map[string]any
	for _, p := range platforms {
		pm := p.(map[string]any)
		if pm["name"] == "pc-windows" {
			pcWindows = pm
			break
		}
	}
	if pcWindows == nil {
		t.Fatal("expected pc-windows platform in list")
	}

	iconURL, ok := pcWindows["icon_url"].(string)
	if !ok {
		t.Fatalf("expected icon_url string, got %T: %v", pcWindows["icon_url"], pcWindows["icon_url"])
	}
	want := "/logos/platforms/pc-windows/pc-windows-icon-light.svg"
	if iconURL != want {
		t.Fatalf("expected icon_url=%q, got %q", want, iconURL)
	}

	// Storefronts embedded in platform response also need icon_url
	storefronts, _ := pcWindows["storefronts"].([]any)
	for _, sfAny := range storefronts {
		sf, ok := sfAny.(map[string]any)
		if !ok {
			continue
		}
		if sf["icon"] != nil {
			if _, ok := sf["icon_url"].(string); !ok {
				t.Fatalf("storefront %v has icon but missing icon_url", sf["name"])
			}
		}
	}
}

func TestListStorefronts_HasIconURL(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	token := loginAndGetToken(t, e, setupUser(t, testDB), "pass123")

	rec := getAuth(t, e, "/api/platforms/storefronts", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var sfResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sfResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	storefronts, _ := sfResp["storefronts"].([]any)

	var steam map[string]any
	for _, s := range storefronts {
		sm := s.(map[string]any)
		if sm["name"] == "steam" {
			steam = sm
			break
		}
	}
	if steam == nil {
		t.Fatal("expected steam storefront in list")
	}

	iconURL, ok := steam["icon_url"].(string)
	if !ok {
		t.Fatalf("expected icon_url string, got %T: %v", steam["icon_url"], steam["icon_url"])
	}
	want := "/logos/storefronts/steam/steam-icon-light.svg"
	if iconURL != want {
		t.Fatalf("expected icon_url=%q, got %q", want, iconURL)
	}
}
