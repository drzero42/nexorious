package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/api"
	"github.com/drzero42/nexorious/internal/migrate"
)

func TestSetupAdmin_Success(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)

	sh := api.NewSetupHandler(testDB, cfg, migrator)
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := sh.HandleSetupAdmin(c); err != nil {
		t.Fatalf("HandleSetupAdmin: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body)
	}

	var resp struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		IsAdmin  bool   `json:"is_admin"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Username != "admin" {
		t.Errorf("username mismatch: got %q", resp.Username)
	}
	if !resp.IsAdmin {
		t.Error("expected is_admin=true")
	}
	if !resp.IsActive {
		t.Error("expected is_active=true")
	}
	if resp.ID == "" {
		t.Error("expected user id in response")
	}

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session_id cookie set after setup")
	}

	var count int
	if err := testDB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}

	if migrator.NeedsSetup() {
		t.Error("expected NeedsSetup=false after setup")
	}
}

func TestSetupAdmin_AlreadySetup(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)

	_, err := testDB.ExecContext(context.Background(),
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','existing','hash',true)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	sh := api.NewSetupHandler(testDB, cfg, migrator)
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = sh.HandleSetupAdmin(c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *echo.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
	}
	if he.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", he.Code)
	}
}

func TestSetupAdmin_InvalidBody(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	sh := api.NewSetupHandler(testDB, cfg, migrator)
	e := echo.New()

	for _, tc := range []struct {
		name string
		body map[string]string
		want int
	}{
		{"missing username", map[string]string{"password": "supersecret"}, http.StatusBadRequest},
		{"missing password", map[string]string{"username": "admin"}, http.StatusBadRequest},
		{"short username", map[string]string{"username": "ab", "password": "supersecret"}, http.StatusBadRequest},
		{"short password", map[string]string{"username": "admin", "password": "short"}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := sh.HandleSetupAdmin(c)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var he *echo.HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("expected *echo.HTTPError, got %T: %v", err, err)
			}
			if he.Code != tc.want {
				t.Errorf("expected %d, got %d", tc.want, he.Code)
			}
		})
	}
}

func TestSetupAdmin_ConcurrentRace(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	sh := api.NewSetupHandler(testDB, cfg, migrator)
	e := echo.New()

	var (
		mu    sync.Mutex
		codes []int
		wg    sync.WaitGroup
	)
	for range 2 {
		wg.Go(func() {
			body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			var code int
			if err := sh.HandleSetupAdmin(c); err != nil {
				if he, ok := errors.AsType[*echo.HTTPError](err); ok {
					code = he.Code
				} else {
					code = http.StatusInternalServerError
				}
			} else {
				code = rec.Code
			}
			mu.Lock()
			codes = append(codes, code)
			mu.Unlock()
		})
	}
	wg.Wait()

	created := 0
	for _, code := range codes {
		if code == http.StatusCreated {
			created++
		}
	}
	if created != 1 {
		t.Errorf("expected exactly 1 success, got %d (codes: %v)", created, codes)
	}

	var count int
	if err := testDB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 user in DB, got %d", count)
	}
}

func TestSetupAdmin_GetMeAfterSetup(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	sh := api.NewSetupHandler(testDB, cfg, migrator)
	e := echo.New()

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if err := sh.HandleSetupAdmin(e.NewContext(req, rec)); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup returned %d: %s", rec.Code, rec.Body)
	}

	var setupResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&setupResp); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}

	// Extract session cookie from setup response
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("no session_id cookie in setup response")
	}

	ah := api.NewAuthHandler(testDB, cfg)
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	meCtx := e.NewContext(meReq, meRec)
	meCtx.Set("user_id", setupResp.ID)

	if err := ah.HandleGetMe(meCtx); err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if meRec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", meRec.Code, meRec.Body)
	}

	var meBody map[string]json.RawMessage
	if err := json.NewDecoder(meRec.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if _, ok := meBody["preferences"]; ok {
		t.Error("me response leaked dropped preferences field")
	}
}

func TestMigration_PlatformStorefrontSeedData(t *testing.T) {
	truncateAllTables(t)

	// Spot-check: pc-windows default_storefront
	var defaultSF *string
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT default_storefront FROM platforms WHERE name = 'pc-windows'").Scan(&defaultSF); err != nil {
		t.Fatalf("query pc-windows default_storefront: %v", err)
	}
	if defaultSF == nil || *defaultSF != "steam" {
		t.Errorf("expected pc-windows default_storefront='steam', got %v", defaultSF)
	}

	// NOTE: icon-filename values are intentionally NOT asserted here. They mirror
	// the migration literals (tautological) and forced a churn edit on every icon
	// migration. The icon contract that actually matters — a bare filename in the
	// DB resolving to a /logos/.../<name>-icon-light.svg URL — is covered for real
	// in platforms_test.go (TestListPlatforms_* / TestListStorefronts_HasIconURL).
	// This test guards only the non-obvious seed *decisions* below.

	// Spot-check #818 Part A: original Xbox seeds physical-only with IGDB id 11.
	var xboxIGDB *int
	var xboxDefaultSF *string
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT igdb_platform_id, default_storefront FROM platforms WHERE name = 'xbox'").Scan(&xboxIGDB, &xboxDefaultSF); err != nil {
		t.Fatalf("query xbox platform: %v", err)
	}
	if xboxIGDB == nil || *xboxIGDB != 11 {
		t.Errorf("expected xbox igdb_platform_id=11, got %v", xboxIGDB)
	}
	if xboxDefaultSF == nil || *xboxDefaultSF != "physical" {
		t.Errorf("expected xbox default_storefront='physical', got %v", xboxDefaultSF)
	}

	// Spot-check #818 Part B: uplay, previously associated with no platform, now
	// has a pc-windows association.
	var uplayAssoc int
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM platform_storefronts WHERE storefront = 'uplay' AND platform = 'pc-windows'").Scan(&uplayAssoc); err != nil {
		t.Fatalf("query uplay association: %v", err)
	}
	if uplayAssoc != 1 {
		t.Errorf("expected uplay<->pc-windows association, got %d rows", uplayAssoc)
	}

	// Telltale Games: manual-only PC storefront with a pc-windows association.
	var telltaleAssoc int
	if err := testDB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM platform_storefronts WHERE storefront = 'telltale' AND platform = 'pc-windows'").Scan(&telltaleAssoc); err != nil {
		t.Fatalf("query telltale association: %v", err)
	}
	if telltaleAssoc != 1 {
		t.Errorf("expected telltale<->pc-windows association, got %d rows", telltaleAssoc)
	}
}
