# Platforms & Tags Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add read-only platform/storefront endpoints and full CRUD for user-scoped tags — the first Phase 2 API endpoints.

**Architecture:** Two handler structs (`PlatformsHandler`, `TagsHandler`) following the established `AuthHandler` pattern. Platforms use Bun's m2m relation loading for storefronts. Tags are simple single-table CRUD scoped to the JWT user. Both groups are JWT-protected via `auth.JWTMiddleware`.

**Tech Stack:** Go, Echo v5, Bun ORM (m2m relations), testcontainers-go, PostgreSQL

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/db/models/models.go` | Modify: add `Storefronts` relation to `Platform`, add relation fields to `PlatformStorefront` |
| `internal/api/platforms.go` | Create: `PlatformsHandler` struct + 8 GET handler methods |
| `internal/api/platforms_test.go` | Create: integration tests for all platform/storefront endpoints |
| `internal/api/tags.go` | Create: `TagsHandler` struct + 4 CRUD handler methods |
| `internal/api/tags_test.go` | Create: integration tests for all tag endpoints |
| `internal/api/router.go` | Modify: register platform and tag route groups |
| `slumber.yaml` | Modify: add platform and tag request definitions |

---

### Task 1: Update Models — Platform m2m Relation

**Files:**
- Modify: `internal/db/models/models.go` (lines 99–124)

The `Platform` model needs a `Storefronts` slice for Bun's m2m relation loading. The `PlatformStorefront` join model needs relation pointer fields — its existing `Platform`/`Storefront` string fields must be renamed to avoid collision.

- [ ] **Step 1: Update the `Platform` struct**

Add the `Storefronts` relation field after `DefaultStorefront`:

```go
type Platform struct {
	bun.BaseModel `bun:"table:platforms"`

	Name              string       `bun:"name,pk"               json:"name"`
	DisplayName       string       `bun:"display_name,notnull"  json:"display_name"`
	Icon              *string      `bun:"icon"                  json:"icon"`
	IgdbPlatformID    *int32       `bun:"igdb_platform_id"      json:"igdb_platform_id"`
	DefaultStorefront *string      `bun:"default_storefront"    json:"default_storefront"`
	Storefronts       []Storefront `bun:"m2m:platform_storefronts,join:Platform=Storefront" json:"storefronts,omitempty"`
}
```

- [ ] **Step 2: Update the `PlatformStorefront` struct**

Rename the string fields to `PlatformName`/`StorefrontName` and add relation pointer fields. Drop the `json` tags — this model is never serialised directly:

```go
type PlatformStorefront struct {
	bun.BaseModel `bun:"table:platform_storefronts"`

	PlatformName   string      `bun:"platform,pk"`
	StorefrontName string      `bun:"storefront,pk"`
	Platform       *Platform   `bun:"rel:belongs-to,join:platform=name"`
	Storefront     *Storefront `bun:"rel:belongs-to,join:storefront=name"`
}
```

- [ ] **Step 3: Verify the build compiles**

Run: `cd /home/abo/workspace/home/nexorious-go && go build ./...`
Expected: clean build (no errors)

- [ ] **Step 4: Check for any references to the old field names**

Search the codebase for `PlatformStorefront.Platform` or `.Storefront` used as string access (not the relation). If filter code or tests reference these fields by old name, update them to `PlatformName`/`StorefrontName`.

Run: `cd /home/abo/workspace/home/nexorious-go && grep -rn 'PlatformStorefront' --include='*.go' | grep -v '_test.go' | grep -v 'models.go'`

Fix any compilation errors, then re-run: `go build ./...`

- [ ] **Step 5: Run existing tests to confirm nothing broke**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./...`
Expected: all tests pass

- [ ] **Step 6: Commit**

```
feat: add Bun m2m relation fields to Platform/PlatformStorefront models
```

---

### Task 2: Platforms Handler — List & Simple List

**Files:**
- Create: `internal/api/platforms.go`

- [ ] **Step 1: Write the test for `GET /api/platforms`**

Create `internal/api/platforms_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/auth"
)

// getAuth fires a GET request with a Bearer token and returns the recorder.
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

func TestListPlatforms(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	// Create a user and get a valid token
	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/platforms", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var platforms []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Storefronts []struct {
			Name string `json:"name"`
		} `json:"storefronts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &platforms); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(platforms) == 0 {
		t.Fatal("expected seeded platforms, got empty list")
	}

	// Verify at least one platform has storefronts (PC has Steam)
	found := false
	for _, p := range platforms {
		if p.Name == "pc" && len(p.Storefronts) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected pc platform with storefronts")
	}
}

func TestListPlatforms_Unauthorized(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/platforms", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestListPlatforms -v`
Expected: FAIL (no route handler registered yet)

- [ ] **Step 3: Create the platforms handler file with `HandleListPlatforms` and `HandleSimpleList`**

Create `internal/api/platforms.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/db/models"
)

// PlatformsHandler handles platform and storefront endpoints.
type PlatformsHandler struct {
	db *bun.DB
}

// NewPlatformsHandler returns a new PlatformsHandler.
func NewPlatformsHandler(db *bun.DB) *PlatformsHandler {
	return &PlatformsHandler{db: db}
}

// simpleListItem is the response shape for simple-list endpoints.
type simpleListItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// HandleListPlatforms handles GET /api/platforms.
// Returns all platforms with nested storefronts.
func (h *PlatformsHandler) HandleListPlatforms(c *echo.Context) error {
	var platforms []models.Platform
	err := h.db.NewSelect().
		Model(&platforms).
		Relation("Storefronts").
		OrderExpr("platform.display_name").
		Scan(context.Background())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, platforms)
}

// HandleSimpleList handles GET /api/platforms/simple-list.
// Returns [{name, display_name}] for dropdowns.
func (h *PlatformsHandler) HandleSimpleList(c *echo.Context) error {
	var items []simpleListItem
	err := h.db.NewSelect().
		TableExpr("platforms").
		Column("name", "display_name").
		OrderExpr("display_name").
		Scan(context.Background(), &items)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, items)
}
```

- [ ] **Step 4: Register platform routes in router.go**

In `internal/api/router.go`, inside the `if db != nil` block, after the auth group, add:

```go
ph := NewPlatformsHandler(db)
platformsGroup := e.Group("/api/platforms", auth.JWTMiddleware(cfg.SecretKey, db))
platformsGroup.GET("", ph.HandleListPlatforms)
platformsGroup.GET("/simple-list", ph.HandleSimpleList)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestListPlatforms -v`
Expected: PASS

- [ ] **Step 6: Write and run the simple-list test**

Add to `platforms_test.go`:

```go
func TestPlatformSimpleList(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/platforms/simple-list", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected seeded platforms, got empty list")
	}
	// Verify no extra fields leaked (storefronts, icon, etc.)
	raw := rec.Body.String()
	if contains(raw, "storefronts") || contains(raw, "icon") {
		t.Error("simple-list should only contain name and display_name")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

Actually, simpler — just use `strings.Contains` (add `"strings"` to the import):

```go
func TestPlatformSimpleList(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/platforms/simple-list", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected seeded platforms, got empty list")
	}
}
```

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestPlatformSimpleList -v`
Expected: PASS

- [ ] **Step 7: Commit**

```
feat: add GET /api/platforms and GET /api/platforms/simple-list endpoints
```

---

### Task 3: Platforms Handler — Single Platform & Platform Storefronts

**Files:**
- Modify: `internal/api/platforms.go`
- Modify: `internal/api/platforms_test.go`

- [ ] **Step 1: Write tests for `GET /api/platforms/:platform` and 404 case**

Add to `platforms_test.go`:

```go
func TestGetPlatform(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	t.Run("found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var p struct {
			Name        string `json:"name"`
			Storefronts []struct {
				Name string `json:"name"`
			} `json:"storefronts"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if p.Name != "pc" {
			t.Errorf("expected pc, got %s", p.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

func TestPlatformStorefronts(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	t.Run("found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc/storefronts", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var storefronts []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &storefronts); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(storefronts) == 0 {
			t.Fatal("expected storefronts for pc")
		}
	})

	t.Run("platform not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent/storefronts", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestGetPlatform|TestPlatformStorefronts" -v`
Expected: FAIL

- [ ] **Step 3: Implement `HandleGetPlatform` and `HandlePlatformStorefronts`**

Add to `platforms.go`:

```go
// HandleGetPlatform handles GET /api/platforms/:platform.
// Returns a single platform with nested storefronts; 404 if not found.
func (h *PlatformsHandler) HandleGetPlatform(c *echo.Context) error {
	name := c.PathParam("platform")

	var platform models.Platform
	err := h.db.NewSelect().
		Model(&platform).
		Relation("Storefronts").
		Where("platform.name = ?", name).
		Scan(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, platform)
}

// HandlePlatformStorefronts handles GET /api/platforms/:platform/storefronts.
// Returns storefronts associated with a platform; 404 if platform not found.
func (h *PlatformsHandler) HandlePlatformStorefronts(c *echo.Context) error {
	name := c.PathParam("platform")

	// Verify platform exists.
	exists, err := h.db.NewSelect().
		TableExpr("platforms").
		Where("name = ?", name).
		Exists(context.Background())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	var storefronts []models.Storefront
	err = h.db.NewSelect().
		Model(&storefronts).
		Join("JOIN platform_storefronts ps ON ps.storefront = storefront.name").
		Where("ps.platform = ?", name).
		OrderExpr("storefront.display_name").
		Scan(context.Background())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, storefronts)
}
```

Add `"database/sql"` to the imports in `platforms.go`.

- [ ] **Step 4: Register the routes in router.go**

Add to the `platformsGroup` in `registerRoutes` (note ordering — these go after `/simple-list` but before `/:platform`):

```go
platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
platformsGroup.GET("/:platform", ph.HandleGetPlatform)
```

**Important:** `/:platform` must be the LAST route registered in the group to avoid catching `/simple-list`, `/storefronts/...`, etc. as a platform name.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestGetPlatform|TestPlatformStorefronts" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
feat: add GET /api/platforms/:platform and /:platform/storefronts endpoints
```

---

### Task 4: Platforms Handler — Default Storefront

**Files:**
- Modify: `internal/api/platforms.go`
- Modify: `internal/api/platforms_test.go`

- [ ] **Step 1: Write tests**

Add to `platforms_test.go`:

```go
func TestDefaultStorefront(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	t.Run("with default", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/pc/default-storefront", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Platform         string `json:"platform"`
			DefaultStorefront *struct {
				Name string `json:"name"`
			} `json:"default_storefront"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Platform != "pc" {
			t.Errorf("expected platform pc, got %s", resp.Platform)
		}
		if resp.DefaultStorefront == nil {
			t.Fatal("expected default storefront for pc")
		}
	})

	t.Run("platform not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/nonexistent/default-storefront", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestDefaultStorefront -v`
Expected: FAIL

- [ ] **Step 3: Implement `HandleDefaultStorefront`**

Add to `platforms.go`:

```go
// defaultStorefrontResponse is the response shape for GET /api/platforms/:platform/default-storefront.
type defaultStorefrontResponse struct {
	Platform            string            `json:"platform"`
	PlatformDisplayName string            `json:"platform_display_name"`
	DefaultStorefront   *models.Storefront `json:"default_storefront"`
}

// HandleDefaultStorefront handles GET /api/platforms/:platform/default-storefront.
func (h *PlatformsHandler) HandleDefaultStorefront(c *echo.Context) error {
	name := c.PathParam("platform")

	var platform models.Platform
	err := h.db.NewSelect().
		Model(&platform).
		Where("name = ?", name).
		Scan(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	resp := defaultStorefrontResponse{
		Platform:            platform.Name,
		PlatformDisplayName: platform.DisplayName,
	}

	if platform.DefaultStorefront != nil && *platform.DefaultStorefront != "" {
		var sf models.Storefront
		err := h.db.NewSelect().
			Model(&sf).
			Where("name = ?", *platform.DefaultStorefront).
			Scan(context.Background())
		if err == nil {
			resp.DefaultStorefront = &sf
		}
	}

	return c.JSON(http.StatusOK, resp)
}
```

- [ ] **Step 4: Register the route**

Add to the `platformsGroup` in `registerRoutes`, before `/:platform`:

```go
platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
```

The full route registration order should now be:
```go
platformsGroup.GET("", ph.HandleListPlatforms)
platformsGroup.GET("/simple-list", ph.HandleSimpleList)
// storefront routes registered in Task 5
platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
platformsGroup.GET("/:platform", ph.HandleGetPlatform)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestDefaultStorefront -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
feat: add GET /api/platforms/:platform/default-storefront endpoint
```

---

### Task 5: Platforms Handler — Storefront Endpoints

**Files:**
- Modify: `internal/api/platforms.go`
- Modify: `internal/api/platforms_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Write tests for all storefront endpoints**

Add to `platforms_test.go`:

```go
func TestListStorefronts(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/platforms/storefronts/", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var storefronts []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &storefronts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(storefronts) == 0 {
		t.Fatal("expected seeded storefronts")
	}
}

func TestStorefrontSimpleList(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/platforms/storefronts/simple-list", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected seeded storefronts")
	}
}

func TestGetStorefront(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	t.Run("found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/storefronts/steam", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var sf struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &sf); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if sf.Name != "steam" {
			t.Errorf("expected steam, got %s", sf.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/platforms/storefronts/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestListStorefronts|TestStorefrontSimpleList|TestGetStorefront" -v`
Expected: FAIL

- [ ] **Step 3: Implement `HandleListStorefronts`, `HandleStorefrontSimpleList`, and `HandleGetStorefront`**

Add to `platforms.go`:

```go
// HandleListStorefronts handles GET /api/platforms/storefronts/.
// Returns all storefronts.
func (h *PlatformsHandler) HandleListStorefronts(c *echo.Context) error {
	var storefronts []models.Storefront
	err := h.db.NewSelect().
		Model(&storefronts).
		OrderExpr("display_name").
		Scan(context.Background())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, storefronts)
}

// HandleStorefrontSimpleList handles GET /api/platforms/storefronts/simple-list.
// Returns [{name, display_name}] for dropdowns.
func (h *PlatformsHandler) HandleStorefrontSimpleList(c *echo.Context) error {
	var items []simpleListItem
	err := h.db.NewSelect().
		TableExpr("storefronts").
		Column("name", "display_name").
		OrderExpr("display_name").
		Scan(context.Background(), &items)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, items)
}

// HandleGetStorefront handles GET /api/platforms/storefronts/:storefront.
// Returns a single storefront; 404 if not found.
func (h *PlatformsHandler) HandleGetStorefront(c *echo.Context) error {
	name := c.PathParam("storefront")

	var sf models.Storefront
	err := h.db.NewSelect().
		Model(&sf).
		Where("name = ?", name).
		Scan(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, sf)
}
```

- [ ] **Step 4: Register the storefront routes in router.go**

The full platform route block in `registerRoutes` should now be:

```go
ph := NewPlatformsHandler(db)
platformsGroup := e.Group("/api/platforms", auth.JWTMiddleware(cfg.SecretKey, db))
platformsGroup.GET("", ph.HandleListPlatforms)
platformsGroup.GET("/simple-list", ph.HandleSimpleList)
platformsGroup.GET("/storefronts/simple-list", ph.HandleStorefrontSimpleList)
platformsGroup.GET("/storefronts/:storefront", ph.HandleGetStorefront)
platformsGroup.GET("/storefronts/", ph.HandleListStorefronts)
platformsGroup.GET("/:platform/storefronts", ph.HandlePlatformStorefronts)
platformsGroup.GET("/:platform/default-storefront", ph.HandleDefaultStorefront)
platformsGroup.GET("/:platform", ph.HandleGetPlatform)
```

**Route ordering is critical:** `/storefronts/simple-list` and `/storefronts/:storefront` must come before `/:platform` to prevent Echo from matching "storefronts" as a platform name param.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestListStorefronts|TestStorefrontSimpleList|TestGetStorefront" -v`
Expected: PASS

- [ ] **Step 6: Run ALL platform tests to confirm nothing regressed**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestListPlatforms|TestPlatformSimpleList|TestGetPlatform|TestPlatformStorefronts|TestDefaultStorefront|TestListStorefronts|TestStorefrontSimpleList|TestGetStorefront" -v`
Expected: all PASS

- [ ] **Step 7: Commit**

```
feat: add storefront listing endpoints (list, simple-list, get by name)
```

---

### Task 6: Tags Handler — List & Create

**Files:**
- Create: `internal/api/tags.go`
- Create: `internal/api/tags_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Write tests for list and create**

Create `internal/api/tags_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious-go/internal/auth"
)

func TestListTags_Empty(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	rec := getAuth(t, e, "/api/tags", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tags []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty list, got %d tags", len(tags))
	}
}

func TestCreateTag(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	t.Run("success", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]string{
			"name":  "Favorites",
			"color": "#ff0000",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var tag struct {
			ID     string  `json:"id"`
			UserID string  `json:"user_id"`
			Name   string  `json:"name"`
			Color  *string `json:"color"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &tag); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if tag.Name != "Favorites" {
			t.Errorf("expected Favorites, got %s", tag.Name)
		}
		if tag.UserID != "user-1" {
			t.Errorf("expected user-1, got %s", tag.UserID)
		}
		if tag.ID == "" {
			t.Error("expected non-empty id")
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]string{
			"name": "Favorites",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing name", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/tags", map[string]string{
			"color": "#00ff00",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("name too long", func(t *testing.T) {
		longName := ""
		for i := 0; i < 101; i++ {
			longName += "a"
		}
		rec := postJSONAuth(t, e, "/api/tags", map[string]string{
			"name": longName,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestListTags_WithTags(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	// Create two tags
	postJSONAuth(t, e, "/api/tags", map[string]string{"name": "AAA"}, token)
	postJSONAuth(t, e, "/api/tags", map[string]string{"name": "BBB"}, token)

	rec := getAuth(t, e, "/api/tags", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var tags []struct {
		Name      string `json:"name"`
		GameCount int    `json:"game_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].Name != "AAA" {
		t.Errorf("expected first tag AAA (sorted), got %s", tags[0].Name)
	}
}

func TestTags_Unauthorized(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestListTags|TestCreateTag|TestTags_Unauthorized" -v`
Expected: FAIL

- [ ] **Step 3: Create the tags handler**

Create `internal/api/tags.go`:

```go
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
)

// TagsHandler handles tag CRUD endpoints.
type TagsHandler struct {
	db *bun.DB
}

// NewTagsHandler returns a new TagsHandler.
func NewTagsHandler(db *bun.DB) *TagsHandler {
	return &TagsHandler{db: db}
}

// createTagRequest is the JSON body for POST /api/tags.
type createTagRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// tagResponse is the JSON response for a single tag.
type tagResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// tagListResponse includes game_count for the list endpoint.
type tagListResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	GameCount int       `json:"game_count"`
}

// HandleListTags handles GET /api/tags.
// Returns all tags for the current user with game_count.
func (h *TagsHandler) HandleListTags(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var tags []tagListResponse
	err := h.db.NewRawQuery(`
		SELECT t.id, t.user_id, t.name, t.color, t.created_at, t.updated_at,
		       COUNT(ugt.id) AS game_count
		FROM tags t
		LEFT JOIN user_game_tags ugt ON ugt.tag_id = t.id
		WHERE t.user_id = ?
		GROUP BY t.id
		ORDER BY t.name
	`, userID).Scan(context.Background(), &tags)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if tags == nil {
		tags = []tagListResponse{}
	}
	return c.JSON(http.StatusOK, tags)
}

// HandleCreateTag handles POST /api/tags.
func (h *TagsHandler) HandleCreateTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)

	var req createTagRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}
	if len(req.Name) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or less"})
	}

	now := time.Now()
	resp := tagResponse{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      req.Name,
		Color:     req.Color,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := h.db.ExecContext(context.Background(),
		`INSERT INTO tags (id, user_id, name, color, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		resp.ID, resp.UserID, resp.Name, resp.Color, resp.CreatedAt, resp.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation (user_id, name)
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return c.JSON(http.StatusConflict, map[string]string{"error": "tag name already exists"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusCreated, resp)
}
```

- [ ] **Step 4: Register tag routes in router.go**

Add to `registerRoutes` inside the `if db != nil` block, after the platforms group:

```go
th := NewTagsHandler(db)
tagsGroup := e.Group("/api/tags", auth.JWTMiddleware(cfg.SecretKey, db))
tagsGroup.GET("", th.HandleListTags)
tagsGroup.POST("", th.HandleCreateTag)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestListTags|TestCreateTag|TestTags_Unauthorized" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```
feat: add GET /api/tags and POST /api/tags endpoints
```

---

### Task 7: Tags Handler — Update & Delete

**Files:**
- Modify: `internal/api/tags.go`
- Modify: `internal/api/tags_test.go`
- Modify: `internal/api/router.go`

- [ ] **Step 1: Write tests for update and delete**

Add to `tags_test.go`:

```go
func TestUpdateTag(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	// Create a tag first
	rec := postJSONAuth(t, e, "/api/tags", map[string]string{"name": "Original"}, token)
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	t.Run("success", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/"+created.ID, map[string]string{
			"name":  "Updated",
			"color": "#00ff00",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var tag struct {
			Name  string  `json:"name"`
			Color *string `json:"color"`
		}
		json.Unmarshal(rec.Body.Bytes(), &tag)
		if tag.Name != "Updated" {
			t.Errorf("expected Updated, got %s", tag.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/tags/nonexistent-id", map[string]string{
			"name": "X",
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		insertAuthTestUser(t, db, "user-2", "bob", "password123", true, false)
		token2, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-2", cfg.AccessTokenExpireMinutes)
		insertAuthTestSession(t, db, "user-2", token2, "refresh-tok-2", 30)

		rec := putJSONAuth(t, e, "/api/tags/"+created.ID, map[string]string{
			"name": "Stolen",
		}, token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 (not 403), got %d", rec.Code)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		postJSONAuth(t, e, "/api/tags", map[string]string{"name": "Another"}, token)
		rec := putJSONAuth(t, e, "/api/tags/"+created.ID, map[string]string{
			"name": "Another",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestDeleteTag(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	// Create a tag
	rec := postJSONAuth(t, e, "/api/tags", map[string]string{"name": "ToDelete"}, token)
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)

	t.Run("success", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/tags/"+created.ID, token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify it's gone
		listRec := getAuth(t, e, "/api/tags", token)
		var tags []struct {
			Name string `json:"name"`
		}
		json.Unmarshal(listRec.Body.Bytes(), &tags)
		for _, tag := range tags {
			if tag.Name == "ToDelete" {
				t.Error("tag should have been deleted")
			}
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/tags/nonexistent-id", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		// Create tag as user-1
		rec := postJSONAuth(t, e, "/api/tags", map[string]string{"name": "Private"}, token)
		var tag struct {
			ID string `json:"id"`
		}
		json.Unmarshal(rec.Body.Bytes(), &tag)

		// Try to delete as user-2
		insertAuthTestUser(t, db, "user-2", "bob", "password123", true, false)
		token2, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-2", cfg.AccessTokenExpireMinutes)
		insertAuthTestSession(t, db, "user-2", token2, "refresh-tok-2", 30)

		rec2 := deleteAuth(t, e, "/api/tags/"+tag.ID, token2)
		if rec2.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec2.Code)
		}
	})
}
```

Also add these test helpers to `tags_test.go`:

```go
// putJSONAuth fires a PUT with a JSON body and a Bearer authorization header.
func putJSONAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, body any, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// deleteAuth fires a DELETE with a Bearer authorization header.
func deleteAuth(t *testing.T, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, path string, accessToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
```

Note: add `"bytes"` to the import block.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestUpdateTag|TestDeleteTag" -v`
Expected: FAIL

- [ ] **Step 3: Implement `HandleUpdateTag` and `HandleDeleteTag`**

Add to `tags.go`:

```go
// updateTagRequest is the JSON body for PUT /api/tags/:id.
type updateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// HandleUpdateTag handles PUT /api/tags/:id.
func (h *TagsHandler) HandleUpdateTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	tagID := c.PathParam("id")

	var req updateTagRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if *req.Name == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
		}
		if len(*req.Name) > 100 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or less"})
		}
	}

	// Build the SET clause dynamically for partial update.
	setClauses := []string{"updated_at = now()"}
	args := []any{}

	if req.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Color != nil {
		setClauses = append(setClauses, "color = ?")
		args = append(args, *req.Color)
	}

	query := "UPDATE tags SET " + strings.Join(setClauses, ", ") + " WHERE id = ? AND user_id = ? RETURNING id, user_id, name, color, created_at, updated_at"
	args = append(args, tagID, userID)

	var resp tagResponse
	err := h.db.QueryRowContext(context.Background(), query, args...).
		Scan(&resp.ID, &resp.UserID, &resp.Name, &resp.Color, &resp.CreatedAt, &resp.UpdatedAt)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "no rows") {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}
		if strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "unique constraint") {
			return c.JSON(http.StatusConflict, map[string]string{"error": "tag name already exists"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleDeleteTag handles DELETE /api/tags/:id.
func (h *TagsHandler) HandleDeleteTag(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	tagID := c.PathParam("id")

	result, err := h.db.ExecContext(context.Background(),
		"DELETE FROM tags WHERE id = ? AND user_id = ?",
		tagID, userID,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Register the remaining tag routes in router.go**

Update the tag route registration:

```go
th := NewTagsHandler(db)
tagsGroup := e.Group("/api/tags", auth.JWTMiddleware(cfg.SecretKey, db))
tagsGroup.GET("", th.HandleListTags)
tagsGroup.POST("", th.HandleCreateTag)
tagsGroup.PUT("/:id", th.HandleUpdateTag)
tagsGroup.DELETE("/:id", th.HandleDeleteTag)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestUpdateTag|TestDeleteTag" -v`
Expected: PASS

- [ ] **Step 6: Run ALL tests to confirm nothing regressed**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./...`
Expected: all PASS

- [ ] **Step 7: Commit**

```
feat: add PUT /api/tags/:id and DELETE /api/tags/:id endpoints
```

---

### Task 8: Tag Delete Cascade Test

**Files:**
- Modify: `internal/api/tags_test.go`

This test verifies that deleting a tag cascades to `user_game_tags` via the DB `ON DELETE CASCADE`.

- [ ] **Step 1: Write the cascade test**

Add to `tags_test.go`:

```go
func TestDeleteTag_CascadesUserGameTags(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token, "refresh-tok", 30)

	// Create a tag
	rec := postJSONAuth(t, e, "/api/tags", map[string]string{"name": "CascadeTest"}, token)
	var tag struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tag)

	// Insert a game and user_game, then link via user_game_tags
	ctx := context.Background()
	_, err := db.ExecContext(ctx,
		"INSERT INTO games (id, title, last_updated, created_at) VALUES (1, 'Test Game', now(), now())")
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES ('ug-1', 'user-1', 1, now(), now())")
	if err != nil {
		t.Fatalf("insert user_game: %v", err)
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO user_game_tags (id, user_game_id, tag_id, created_at) VALUES ('ugt-1', 'ug-1', ?, now())",
		tag.ID)
	if err != nil {
		t.Fatalf("insert user_game_tag: %v", err)
	}

	// Delete the tag
	delRec := deleteAuth(t, e, "/api/tags/"+tag.ID, token)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delRec.Code)
	}

	// Verify user_game_tags row is gone
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_game_tags WHERE tag_id = ?", tag.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count user_game_tags: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 user_game_tags after cascade delete, got %d", count)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestDeleteTag_CascadesUserGameTags -v`
Expected: PASS

- [ ] **Step 3: Commit**

```
test: verify tag deletion cascades to user_game_tags
```

---

### Task 9: User-Scoped Tag Isolation Test

**Files:**
- Modify: `internal/api/tags_test.go`

Verify that listing tags only returns the current user's tags, not other users' tags.

- [ ] **Step 1: Write the isolation test**

Add to `tags_test.go`:

```go
func TestListTags_UserScoped(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	// User 1 creates tags
	insertAuthTestUser(t, db, "user-1", "alice", "password123", true, false)
	token1, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-1", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-1", token1, "refresh-tok-1", 30)
	postJSONAuth(t, e, "/api/tags", map[string]string{"name": "Alice Tag"}, token1)

	// User 2 creates tags
	insertAuthTestUser(t, db, "user-2", "bob", "password123", true, false)
	token2, _ := auth.GenerateAccessToken(cfg.SecretKey, "user-2", cfg.AccessTokenExpireMinutes)
	insertAuthTestSession(t, db, "user-2", token2, "refresh-tok-2", 30)
	postJSONAuth(t, e, "/api/tags", map[string]string{"name": "Bob Tag"}, token2)

	// User 1 should only see their tag
	rec := getAuth(t, e, "/api/tags", token1)
	var tags []struct {
		Name string `json:"name"`
	}
	json.Unmarshal(rec.Body.Bytes(), &tags)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag for user-1, got %d", len(tags))
	}
	if tags[0].Name != "Alice Tag" {
		t.Errorf("expected 'Alice Tag', got '%s'", tags[0].Name)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run TestListTags_UserScoped -v`
Expected: PASS

- [ ] **Step 3: Commit**

```
test: verify tag listing is user-scoped
```

---

### Task 10: Slumber Collection Update

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add platform and tag request definitions to `slumber.yaml`**

Add after the existing `auth` folder (maintaining alphabetical order of domain folders):

```yaml
  platforms:
    name: Platforms
    requests:
      list_platforms:
        name: List Platforms
        method: GET
        url: "{{base_url}}/api/platforms"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      platform_simple_list:
        name: Platform Simple List
        method: GET
        url: "{{base_url}}/api/platforms/simple-list"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      get_platform:
        name: Get Platform
        method: GET
        url: "{{base_url}}/api/platforms/pc"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      platform_storefronts:
        name: Platform Storefronts
        method: GET
        url: "{{base_url}}/api/platforms/pc/storefronts"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      default_storefront:
        name: Default Storefront
        method: GET
        url: "{{base_url}}/api/platforms/pc/default-storefront"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      list_storefronts:
        name: List Storefronts
        method: GET
        url: "{{base_url}}/api/platforms/storefronts/"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      storefront_simple_list:
        name: Storefront Simple List
        method: GET
        url: "{{base_url}}/api/platforms/storefronts/simple-list"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      get_storefront:
        name: Get Storefront
        method: GET
        url: "{{base_url}}/api/platforms/storefronts/steam"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

  tags:
    name: Tags
    requests:
      list_tags:
        name: List Tags
        method: GET
        url: "{{base_url}}/api/tags"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      create_tag:
        name: Create Tag
        method: POST
        url: "{{base_url}}/api/tags"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            name: Favorites
            color: "#ff0000"

      update_tag:
        name: Update Tag
        method: PUT
        url: "{{base_url}}/api/tags/TAG_ID_HERE"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            name: Updated Name
            color: "#00ff00"

      delete_tag:
        name: Delete Tag
        method: DELETE
        url: "{{base_url}}/api/tags/TAG_ID_HERE"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
```

- [ ] **Step 2: Verify the collection loads**

Run: `cd /home/abo/workspace/home/nexorious-go && slumber show collection`
Expected: no errors

- [ ] **Step 3: Commit**

```
chore: add platform and tag requests to slumber collection
```

---

### Task 11: Final Verification

- [ ] **Step 1: Run the full test suite**

Run: `cd /home/abo/workspace/home/nexorious-go && go test ./...`
Expected: all PASS

- [ ] **Step 2: Run the linter**

Run: `cd /home/abo/workspace/home/nexorious-go && golangci-lint run`
Expected: no errors

- [ ] **Step 3: Verify the build compiles**

Run: `cd /home/abo/workspace/home/nexorious-go && go build ./...`
Expected: clean build
