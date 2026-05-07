package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/migrate"
)

func TestSetupAdmin_Success(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)

	sh := api.NewSetupHandler(pool, cfg, migrator)
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
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			IsAdmin  bool   `json:"is_admin"`
			IsActive bool   `json:"is_active"`
		} `json:"user"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.User.Username != "admin" {
		t.Errorf("username mismatch: got %q", resp.User.Username)
	}
	if !resp.User.IsAdmin {
		t.Error("expected is_admin=true")
	}
	if !resp.User.IsActive {
		t.Error("expected is_active=true")
	}
	if resp.AccessToken == "" {
		t.Error("expected access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected refresh_token")
	}

	var count int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
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
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)

	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','existing','hash',true)`)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	sh := api.NewSetupHandler(pool, cfg, migrator)
	e := echo.New()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := sh.HandleSetupAdmin(c); err != nil {
		t.Fatalf("HandleSetupAdmin: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body)
	}
}

func TestSetupAdmin_InvalidBody(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	sh := api.NewSetupHandler(pool, cfg, migrator)
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
			if err := sh.HandleSetupAdmin(c); err != nil {
				t.Fatalf("HandleSetupAdmin: %v", err)
			}
			if rec.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body)
			}
		})
	}
}

func TestSetupAdmin_ConcurrentRace(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	sh := api.NewSetupHandler(pool, cfg, migrator)
	e := echo.New()

	var (
		mu    sync.Mutex
		codes []int
		wg    sync.WaitGroup
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			_ = sh.HandleSetupAdmin(c)
			mu.Lock()
			codes = append(codes, rec.Code)
			mu.Unlock()
		}()
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
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 user in DB, got %d", count)
	}
}

func TestSetupAdmin_GetMeAfterSetup(t *testing.T) {
	pool := setupAuthTestDB(t)
	cfg := testCfg()
	migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
	migrator.SetNeedsSetup(true)
	sh := api.NewSetupHandler(pool, cfg, migrator)
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
		User        struct{ ID string `json:"id"` } `json:"user"`
		AccessToken string                          `json:"access_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&setupResp); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}

	ah := api.NewAuthHandler(pool, cfg)
	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+setupResp.AccessToken)
	meRec := httptest.NewRecorder()
	meCtx := e.NewContext(meReq, meRec)
	meCtx.Set("user_id", setupResp.User.ID)

	if err := ah.HandleGetMe(meCtx); err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if meRec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", meRec.Code, meRec.Body)
	}

	var meBody struct {
		Preferences json.RawMessage `json:"preferences"`
	}
	if err := json.NewDecoder(meRec.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if string(meBody.Preferences) == "null" || string(meBody.Preferences) == "" {
		t.Errorf("expected preferences={}, got %q", string(meBody.Preferences))
	}
}
