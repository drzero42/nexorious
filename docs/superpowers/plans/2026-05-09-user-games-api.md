# User Games API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the full `/api/user-games` endpoint surface — single-item CRUD, bulk operations, and platform sub-resource management.

**Architecture:** Handler struct `UserGamesHandler` in `internal/api/user_games.go` following the same pattern as `TagsHandler` and `GamesHandler`. Uses raw SQL queries (like tags) for dynamic partial updates, and Bun query builder with the existing `internal/filter` package for the list endpoint. Enum validation via a new `internal/enum/` package. All endpoints JWT-protected, user-scoped.

**Tech Stack:** Go, Echo v5, Bun ORM, PostgreSQL, testcontainers-go

---

## File Structure

| File | Change | Responsibility |
|------|--------|----------------|
| `internal/enum/enum.go` | **New** | `PlayStatus` and `OwnershipStatus` string enums with `Valid()` methods |
| `internal/db/models/models.go` | **Modify** | Add Bun relation tags to `UserGame` and `UserGameTag`; fix `UserGamePlatform` nullability |
| `internal/api/user_games.go` | **New** | Handler struct + all single-item CRUD + bulk operations + platform sub-resource handlers |
| `internal/api/user_games_test.go` | **New** | Full test suite |
| `internal/api/router.go` | **Modify** | Register `/api/user-games` routes |
| `slumber.yaml` | **Modify** | Add user-games request collection |

---

### Task 1: Enum Validation Package

**Files:**
- Create: `internal/enum/enum.go`

- [ ] **Step 1: Create the enum package with PlayStatus and OwnershipStatus**

```go
package enum

// PlayStatus represents valid play_status values for user_games.
type PlayStatus string

const (
	PlayStatusNotStarted  PlayStatus = "not_started"
	PlayStatusInProgress  PlayStatus = "in_progress"
	PlayStatusCompleted   PlayStatus = "completed"
	PlayStatusMastered    PlayStatus = "mastered"
	PlayStatusDominated   PlayStatus = "dominated"
	PlayStatusShelved     PlayStatus = "shelved"
	PlayStatusDropped     PlayStatus = "dropped"
	PlayStatusReplay      PlayStatus = "replay"
)

var validPlayStatuses = map[PlayStatus]bool{
	PlayStatusNotStarted: true,
	PlayStatusInProgress: true,
	PlayStatusCompleted:  true,
	PlayStatusMastered:   true,
	PlayStatusDominated:  true,
	PlayStatusShelved:    true,
	PlayStatusDropped:    true,
	PlayStatusReplay:     true,
}

// Valid reports whether s is a recognised play status.
func (s PlayStatus) Valid() bool {
	return validPlayStatuses[s]
}

// OwnershipStatus represents valid ownership_status values for user_game_platforms.
type OwnershipStatus string

const (
	OwnershipOwned        OwnershipStatus = "owned"
	OwnershipBorrowed     OwnershipStatus = "borrowed"
	OwnershipRented       OwnershipStatus = "rented"
	OwnershipSubscription OwnershipStatus = "subscription"
	OwnershipNoLongerOwned OwnershipStatus = "no_longer_owned"
)

var validOwnershipStatuses = map[OwnershipStatus]bool{
	OwnershipOwned:         true,
	OwnershipBorrowed:      true,
	OwnershipRented:        true,
	OwnershipSubscription:  true,
	OwnershipNoLongerOwned: true,
}

// Valid reports whether s is a recognised ownership status.
func (s OwnershipStatus) Valid() bool {
	return validOwnershipStatuses[s]
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/enum/...`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/enum/enum.go
git commit -m "feat: add enum validation package for PlayStatus and OwnershipStatus"
```

---

### Task 2: Model Relation Tags & Nullability Fix

**Files:**
- Modify: `internal/db/models/models.go`

- [ ] **Step 1: Add Bun relation tags to UserGame**

Add these fields to the `UserGame` struct, after the existing fields (before the closing brace):

```go
Game      *Game              `bun:"rel:belongs-to,join:game_id=id"       json:"game,omitempty"`
Platforms []UserGamePlatform `bun:"rel:has-many,join:id=user_game_id"    json:"platforms,omitempty"`
Tags      []UserGameTag      `bun:"rel:has-many,join:id=user_game_id"    json:"tags,omitempty"`
```

- [ ] **Step 2: Add Bun relation tag to UserGameTag**

Add this field to the `UserGameTag` struct, after the existing fields:

```go
Tag *Tag `bun:"rel:belongs-to,join:tag_id=id" json:"tag,omitempty"`
```

- [ ] **Step 3: Fix UserGamePlatform nullability**

Change the `Platform` and `Storefront` fields on `UserGamePlatform` from `string` with `notnull` to `*string` without `notnull`:

Before:
```go
Platform               string     `bun:"platform,notnull"             json:"platform"`
Storefront             string     `bun:"storefront,notnull"           json:"storefront"`
```

After:
```go
Platform               *string    `bun:"platform"                     json:"platform"`
Storefront             *string    `bun:"storefront"                   json:"storefront"`
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: success (no errors). If there are compile errors from code that uses `Platform`/`Storefront` as non-pointer strings, fix those callers to dereference or handle nil.

- [ ] **Step 5: Run existing tests to make sure nothing breaks**

Run: `go test ./... -count=1`
Expected: all existing tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/db/models/models.go
git commit -m "feat: add Bun relation tags to UserGame/UserGameTag and fix UserGamePlatform nullability"
```

---

### Task 3: UserGamesHandler — Create & Get Single

**Files:**
- Create: `internal/api/user_games.go`
- Create: `internal/api/user_games_test.go`

This task implements the handler struct, `POST /api/user-games` (create), and `GET /api/user-games/:id` (get single). Tests first.

- [ ] **Step 1: Write the test file scaffold with helpers and create tests**

Create `internal/api/user_games_test.go`:

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uptrace/bun"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// setupUserGamesUser inserts a user, session, logs in, returns (userID, token).
func setupUserGamesUser(t *testing.T, db *bun.DB, handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, suffix string) (string, string) {
	t.Helper()
	userID := "u-ug-" + suffix
	username := "uguser-" + suffix
	insertAuthTestUser(t, db, userID, username, "pass123", true, false)
	insertAuthTestSession(t, db, userID, "access-"+suffix, "refresh-"+suffix, 1)
	token := loginAndGetToken(t, handler, username, "pass123")
	return userID, token
}

// insertTestUserGame inserts a user_game row with defaults.
func insertTestUserGame(t *testing.T, db *bun.DB, id, userID string, gameID int) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
		id, userID, gameID,
	)
	if err != nil {
		t.Fatalf("insertTestUserGame: %v", err)
	}
}

// insertTestUserGamePlatform inserts a user_game_platforms row.
func insertTestUserGamePlatform(t *testing.T, db *bun.DB, id, userGameID string, platform, storefront *string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		id, userGameID, platform, storefront,
	)
	if err != nil {
		t.Fatalf("insertTestUserGamePlatform: %v", err)
	}
}

// ─── TestCreateUserGame ──────────────────────────────────────────────────────

func TestCreateUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "create")
	_ = userID

	gameID := insertTestGame(t, db, "Test Game Create")

	t.Run("success", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":     gameID,
			"play_status": "backlog",
		}, token)
		// Note: "backlog" is not a valid play_status — this should return 400.
		// Use a valid status instead.
	})

	t.Run("success with valid status", func(t *testing.T) {
		// Insert a second game to avoid duplicate constraint
		gameID2 := insertTestGame(t, db, "Test Game Create 2")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":     gameID2,
			"play_status": "not_started",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["game_id"] == nil {
			t.Fatal("expected game_id in response")
		}
	})

	t.Run("duplicate game", func(t *testing.T) {
		// First add succeeds
		gameID3 := insertTestGame(t, db, "Test Game Dup")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID3,
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 for first add, got %d: %s", rec.Code, rec.Body.String())
		}
		// Second add → 409
		rec2 := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": gameID3,
		}, token)
		if rec2.Code != http.StatusConflict {
			t.Fatalf("expected 409 for duplicate, got %d: %s", rec2.Code, rec2.Body.String())
		}
	})

	t.Run("invalid game_id", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id": 999999,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid game_id, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		gameID4 := insertTestGame(t, db, "Test Game Invalid Status")
		rec := postJSONAuth(t, e, "/api/user-games", map[string]any{
			"game_id":     gameID4,
			"play_status": "invalid_status",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid play_status, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestGetUserGame ─────────────────────────────────────────────────────────

func TestGetUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "get")
	gameID := insertTestGame(t, db, "Test Game Get")
	insertTestUserGame(t, db, "ug-get-1", userID, int(gameID))

	t.Run("success", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-get-1", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["id"] != "ug-get-1" {
			t.Fatalf("expected id=ug-get-1, got %v", resp["id"])
		}
		// Should have eager-loaded game
		if resp["game"] == nil {
			t.Fatal("expected game relation to be loaded")
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "get-other")
		rec := getAuth(t, e, "/api/user-games/ug-get-1", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestCreateUserGame|TestGetUserGame" -v`
Expected: compile errors — `UserGamesHandler` and routes don't exist yet.

- [ ] **Step 3: Implement the handler struct, constructor, create and get-single handlers**

Create `internal/api/user_games.go`:

```go
package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/internal/auth"
	"github.com/drzero42/nexorious-go/internal/config"
	"github.com/drzero42/nexorious-go/internal/db/models"
	"github.com/drzero42/nexorious-go/internal/enum"
)

// UserGamesHandler handles /api/user-games endpoints.
type UserGamesHandler struct {
	db  *bun.DB
	cfg *config.Config
}

// NewUserGamesHandler creates a UserGamesHandler.
func NewUserGamesHandler(db *bun.DB, cfg *config.Config) *UserGamesHandler {
	return &UserGamesHandler{db: db, cfg: cfg}
}

// createUserGameRequest is the body for POST /api/user-games.
type createUserGameRequest struct {
	GameID         int32    `json:"game_id"`
	PlayStatus     *string  `json:"play_status"`
	PersonalRating *int32   `json:"personal_rating"`
	IsLoved        bool     `json:"is_loved"`
	HoursPlayed    *float64 `json:"hours_played"`
	PersonalNotes  *string  `json:"personal_notes"`
}

// HandleCreateUserGame handles POST /api/user-games.
func (h *UserGamesHandler) HandleCreateUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req createUserGameRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.GameID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "game_id is required")
	}

	// Validate game exists.
	var gameExists bool
	err := h.db.NewSelect().Model((*models.Game)(nil)).
		Where("id = ?", req.GameID).
		ColumnExpr("1").
		Scan(context.Background(), &gameExists)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "game not found")
	}

	// Validate play_status if provided.
	if req.PlayStatus != nil && *req.PlayStatus != "" {
		if !enum.PlayStatus(*req.PlayStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}

	now := time.Now().UTC()
	ug := &models.UserGame{
		ID:             uuid.NewString(),
		UserID:         userID,
		GameID:         req.GameID,
		PlayStatus:     req.PlayStatus,
		PersonalRating: req.PersonalRating,
		IsLoved:        req.IsLoved,
		HoursPlayed:    req.HoursPlayed,
		PersonalNotes:  req.PersonalNotes,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err = h.db.NewInsert().Model(ug).Exec(context.Background())
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "game already in collection")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusCreated, ug)
}

// HandleGetUserGame handles GET /api/user-games/:id.
func (h *UserGamesHandler) HandleGetUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")

	var ug models.UserGame
	err := h.db.NewSelect().Model(&ug).
		Where("ug.id = ?", id).
		Where("ug.user_id = ?", userID).
		Relation("Game").
		Relation("Platforms").
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		Scan(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, ug)
}
```

- [ ] **Step 4: Register the create and get-single routes in router.go**

In `internal/api/router.go`, inside the `if db != nil` block, after the games routes, add:

```go
// User Games routes (all JWT-protected)
ugh := NewUserGamesHandler(db, cfg)
userGamesGroup := e.Group("/api/user-games", auth.JWTMiddleware(cfg.SecretKey, db))
userGamesGroup.POST("", ugh.HandleCreateUserGame)
userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
```

- [ ] **Step 5: Run the create and get tests**

Run: `go test ./internal/api/... -run "TestCreateUserGame|TestGetUserGame" -v -count=1`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games create and get-single endpoints"
```

---

### Task 4: Update & Delete User Game

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write update and delete tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestUpdateUserGame ──────────────────────────────────────────────────────

func TestUpdateUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "update")
	gameID := insertTestGame(t, db, "Test Game Update")
	insertTestUserGame(t, db, "ug-upd-1", userID, int(gameID))

	t.Run("success partial update", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"play_status": "completed",
			"is_loved":    true,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["play_status"] != "completed" {
			t.Fatalf("expected play_status=completed, got %v", resp["play_status"])
		}
		if resp["is_loved"] != true {
			t.Fatalf("expected is_loved=true, got %v", resp["is_loved"])
		}
	})

	t.Run("set field to null", func(t *testing.T) {
		// First set a rating
		putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"personal_rating": 4,
		}, token)
		// Then set it to null
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"personal_rating": nil,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["personal_rating"] != nil {
			t.Fatalf("expected personal_rating=nil, got %v", resp["personal_rating"])
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"play_status": "invalid",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rating out of range", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"personal_rating": 6,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for rating > 5, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("reject game_id in update", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"game_id": 999,
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for game_id in update, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/nonexistent", map[string]any{
			"is_loved": true,
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "update-other")
		rec := putJSONAuth(t, e, "/api/user-games/ug-upd-1", map[string]any{
			"is_loved": true,
		}, token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestDeleteUserGame ──────────────────────────────────────────────────────

func TestDeleteUserGame(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "delete")
	gameID := insertTestGame(t, db, "Test Game Delete")
	insertTestUserGame(t, db, "ug-del-1", userID, int(gameID))

	t.Run("success", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games/ug-del-1", token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := deleteAuth(t, e, "/api/user-games/nonexistent", token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		gameID2 := insertTestGame(t, db, "Test Game Del Other")
		insertTestUserGame(t, db, "ug-del-other", userID, int(gameID2))
		_, token2 := setupUserGamesUser(t, db, e, "delete-other")
		rec := deleteAuth(t, e, "/api/user-games/ug-del-other", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// ─── TestDeleteUserGame_Cascades ─────────────────────────────────────────────

func TestDeleteUserGame_Cascades(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "del-cascade")
	gameID := insertTestGame(t, db, "Test Game Cascade Del")
	insertTestUserGame(t, db, "ug-cas-del-1", userID, int(gameID))

	pc := "pc"
	steam := "steam"
	insertTestUserGamePlatform(t, db, "ugp-cas-1", "ug-cas-del-1", &pc, &steam)
	insertTag(t, db, "tag-cas-del-1", userID, "CascadeTag", nil)
	insertUserGameTag(t, db, "ugt-cas-del-1", "ug-cas-del-1", "tag-cas-del-1")

	rec := deleteAuth(t, e, "/api/user-games/ug-cas-del-1", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify cascaded rows are gone.
	var count int
	_ = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-cas-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_platforms to be cascade-deleted")
	}
	_ = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM user_game_tags WHERE id = 'ugt-cas-del-1'").Scan(&count)
	if count != 0 {
		t.Fatal("expected user_game_tags to be cascade-deleted")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestUpdateUserGame|TestDeleteUserGame" -v -count=1`
Expected: compile/route errors — handlers don't exist yet.

- [ ] **Step 3: Implement update handler with map-based partial update**

Append to `internal/api/user_games.go`:

```go
// allowedUpdateFields is the set of fields accepted in PUT /api/user-games/:id.
var allowedUpdateFields = map[string]bool{
	"play_status":     true,
	"personal_rating": true,
	"is_loved":        true,
	"hours_played":    true,
	"personal_notes":  true,
}

// immutableFields are rejected with 400 if present in an update body.
var immutableFields = map[string]bool{
	"game_id": true,
	"user_id": true,
}

// HandleUpdateUserGame handles PUT /api/user-games/:id.
func (h *UserGamesHandler) HandleUpdateUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")

	// Decode into map to support partial updates.
	var body map[string]any
	if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Reject immutable fields.
	for key := range body {
		if immutableFields[key] {
			return echo.NewHTTPError(http.StatusBadRequest, key+" cannot be updated")
		}
	}

	// Validate keys.
	for key := range body {
		if !allowedUpdateFields[key] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown field: "+key)
		}
	}

	// Validate play_status if provided.
	if ps, ok := body["play_status"]; ok && ps != nil {
		psStr, isStr := ps.(string)
		if !isStr {
			return echo.NewHTTPError(http.StatusBadRequest, "play_status must be a string")
		}
		if !enum.PlayStatus(psStr).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}

	// Validate personal_rating if provided and non-null.
	if pr, ok := body["personal_rating"]; ok && pr != nil {
		prFloat, isFloat := pr.(float64) // JSON numbers decode as float64
		if !isFloat {
			return echo.NewHTTPError(http.StatusBadRequest, "personal_rating must be a number")
		}
		prInt := int32(prFloat)
		if prInt < 1 || prInt > 5 {
			return echo.NewHTTPError(http.StatusBadRequest, "personal_rating must be between 1 and 5")
		}
	}

	// Build dynamic SET clause.
	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	for key, val := range body {
		setClauses = append(setClauses, key+" = ?")
		args = append(args, val)
	}

	// WHERE args.
	args = append(args, id, userID)

	query := fmt.Sprintf(
		`UPDATE user_games SET %s
		 WHERE id = ? AND user_id = ?
		 RETURNING id, user_id, game_id, play_status, personal_rating, is_loved,
		           hours_played, personal_notes, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)

	var ug models.UserGame
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &ug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Eager-load relations for the response.
	err = h.db.NewSelect().Model(&ug).
		Where("ug.id = ?", ug.ID).
		Relation("Game").
		Relation("Platforms").
		Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Relation("Tag")
		}).
		WherePK().
		Scan(context.Background())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Non-critical — return without relations if eager-load fails.
	}

	return c.JSON(http.StatusOK, ug)
}

// HandleDeleteUserGame handles DELETE /api/user-games/:id.
func (h *UserGamesHandler) HandleDeleteUserGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")
	ctx := context.Background()

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	defer func() { _ = tx.Rollback() }()

	// Verify ownership.
	var exists bool
	err = tx.NewSelect().Model((*models.UserGame)(nil)).
		ColumnExpr("1").
		Where("id = ?", id).
		Where("user_id = ?", userID).
		Scan(ctx, &exists)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	// Delete (cascades platforms + tags via FK).
	_, err = tx.NewDelete().Model((*models.UserGame)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.NoContent(http.StatusNoContent)
}
```

Add these imports to the top of `user_games.go` (merge with existing):
```go
"encoding/json"
"fmt"
"strings"
```

- [ ] **Step 4: Register update and delete routes in router.go**

Add to the `userGamesGroup` block:

```go
userGamesGroup.PUT("/:id", ugh.HandleUpdateUserGame)
userGamesGroup.DELETE("/:id", ugh.HandleDeleteUserGame)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestUpdateUserGame|TestDeleteUserGame" -v -count=1`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games update and delete endpoints"
```

---

### Task 5: Update Progress Endpoint

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write update-progress tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestUpdateProgress ──────────────────────────────────────────────────────

func TestUpdateProgress(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "progress")
	gameID := insertTestGame(t, db, "Test Game Progress")
	insertTestUserGame(t, db, "ug-prog-1", userID, int(gameID))

	t.Run("success hours only", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"hours_played": 12.5,
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("success both fields", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"hours_played": 25.0,
			"play_status":  "in_progress",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty body", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty body, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid play_status", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/ug-prog-1/progress", map[string]any{
			"play_status": "nope",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/nonexistent/progress", map[string]any{
			"hours_played": 1.0,
		}, token)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestUpdateProgress" -v -count=1`
Expected: route not found → 404 or 405

- [ ] **Step 3: Implement the progress handler**

Append to `internal/api/user_games.go`:

```go
// updateProgressRequest is the body for PUT /api/user-games/:id/progress.
type updateProgressRequest struct {
	HoursPlayed *float64 `json:"hours_played"`
	PlayStatus  *string  `json:"play_status"`
}

// HandleUpdateProgress handles PUT /api/user-games/:id/progress.
func (h *UserGamesHandler) HandleUpdateProgress(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	id := c.Param("id")

	var req updateProgressRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.HoursPlayed == nil && req.PlayStatus == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "at least one of hours_played or play_status is required")
	}

	if req.PlayStatus != nil {
		if !enum.PlayStatus(*req.PlayStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}

	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	if req.HoursPlayed != nil {
		setClauses = append(setClauses, "hours_played = ?")
		args = append(args, *req.HoursPlayed)
	}
	if req.PlayStatus != nil {
		setClauses = append(setClauses, "play_status = ?")
		args = append(args, *req.PlayStatus)
	}

	args = append(args, id, userID)

	query := fmt.Sprintf(
		`UPDATE user_games SET %s
		 WHERE id = ? AND user_id = ?
		 RETURNING id, user_id, game_id, play_status, personal_rating, is_loved,
		           hours_played, personal_notes, created_at, updated_at`,
		strings.Join(setClauses, ", "),
	)

	var ug models.UserGame
	err := h.db.NewRaw(query, args...).Scan(context.Background(), &ug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "user game not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, ug)
}
```

- [ ] **Step 4: Register the route**

Add to the `userGamesGroup` block in `router.go`:

```go
userGamesGroup.PUT("/:id/progress", ugh.HandleUpdateProgress)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestUpdateProgress" -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games update-progress endpoint"
```

---

### Task 6: List User Games with Filters

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write list tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestListUserGames ───────────────────────────────────────────────────────

func TestListUserGames(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "list")
	g1 := insertTestGame(t, db, "Alpha Game")
	g2 := insertTestGame(t, db, "Beta Game")
	g3 := insertTestGame(t, db, "Gamma Game")
	insertTestUserGame(t, db, "ug-list-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-list-2", userID, int(g2))
	insertTestUserGame(t, db, "ug-list-3", userID, int(g3))

	t.Run("basic list", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total := int(resp["total"].(float64))
		if total != 3 {
			t.Fatalf("expected total=3, got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 3 {
			t.Fatalf("expected 3 user_games, got %d", len(games))
		}
		// Each should have game relation loaded
		first := games[0].(map[string]any)
		if first["game"] == nil {
			t.Fatal("expected game relation to be loaded")
		}
	})

	t.Run("pagination", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?page=1&per_page=2", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		total := int(resp["total"].(float64))
		if total != 3 {
			t.Fatalf("expected total=3, got %d", total)
		}
		games := resp["user_games"].([]any)
		if len(games) != 2 {
			t.Fatalf("expected 2 items on page 1, got %d", len(games))
		}
		pages := int(resp["pages"].(float64))
		if pages != 2 {
			t.Fatalf("expected 2 pages, got %d", pages)
		}
	})

	t.Run("sort by title", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?sort_by=title&sort_order=asc", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		games := resp["user_games"].([]any)
		first := games[0].(map[string]any)
		game := first["game"].(map[string]any)
		if game["title"] != "Alpha Game" {
			t.Fatalf("expected Alpha Game first when sorted by title asc, got %v", game["title"])
		}
	})

	t.Run("search by title", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?q=Beta", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		total := int(resp["total"].(float64))
		if total != 1 {
			t.Fatalf("expected total=1 for 'Beta' search, got %d", total)
		}
	})

	t.Run("user scoped", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "list-other")
		rec := getAuth(t, e, "/api/user-games", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		total := int(resp["total"].(float64))
		if total != 0 {
			t.Fatalf("expected total=0 for other user, got %d", total)
		}
	})

	t.Run("invalid sort field", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games?sort_by=hacked", token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid sort_by, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestListUserGames" -v -count=1`
Expected: route not registered → 302 redirect or 404

- [ ] **Step 3: Implement the list handler**

Append to `internal/api/user_games.go`:

```go
// UserGameListResponse is the paginated response for user game listings.
type UserGameListResponse struct {
	UserGames []models.UserGame `json:"user_games"`
	Total     int               `json:"total"`
	Page      int               `json:"page"`
	PerPage   int               `json:"per_page"`
	Pages     int               `json:"pages"`
}

// allowedUserGameSortFields maps query param values to SQL expressions.
var allowedUserGameSortFields = map[string]string{
	"title":           "g.title",
	"created_at":      "ug.created_at",
	"updated_at":      "ug.updated_at",
	"play_status":     "ug.play_status",
	"personal_rating": "ug.personal_rating",
	"is_loved":        "ug.is_loved",
	"hours_played":    "ug.hours_played",
	"release_date":    "g.release_date",
}

// sortFieldsRequiringGamesJoin lists sort fields that need the games table joined.
var sortFieldsRequiringGamesJoin = map[string]bool{
	"title":        true,
	"release_date": true,
}

// HandleListUserGames handles GET /api/user-games.
func (h *UserGamesHandler) HandleListUserGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Parse pagination.
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.QueryParam("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Parse sort.
	sortBy := c.QueryParam("sort_by")
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortExpr, ok := allowedUserGameSortFields[sortBy]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid sort_by field: " + sortBy,
		})
	}
	sortOrder := c.QueryParam("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "sort_order must be 'asc' or 'desc'",
		})
	}

	ctx := context.Background()
	fb := filter.NewFilterBuilder()

	// Apply filters from query params.
	filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))

	if isLovedStr := c.QueryParam("is_loved"); isLovedStr != "" {
		v := isLovedStr == "true"
		filter.ApplyIsLoved(fb, &v)
	}
	if rminStr := c.QueryParam("rating_min"); rminStr != "" {
		if v, err := strconv.ParseFloat(rminStr, 64); err == nil {
			filter.ApplyRatingMin(fb, &v)
		}
	}
	if rmaxStr := c.QueryParam("rating_max"); rmaxStr != "" {
		if v, err := strconv.ParseFloat(rmaxStr, 64); err == nil {
			filter.ApplyRatingMax(fb, &v)
		}
	}
	if hasNotesStr := c.QueryParam("has_notes"); hasNotesStr != "" {
		v := hasNotesStr == "true"
		filter.ApplyHasNotes(fb, &v)
	}

	filter.ApplyPlatform(fb, c.QueryParams()["platform"])
	filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
	filter.ApplyGenre(fb, c.QueryParams()["genre"])
	filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
	filter.ApplyTheme(fb, c.QueryParams()["theme"])
	filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
	filter.ApplyTag(fb, c.QueryParams()["tag"])
	filter.ApplySearch(fb, c.QueryParam("q"))

	// If sort field requires games join, add it.
	if sortFieldsRequiringGamesJoin[sortBy] {
		fb.AddJoin("g", "LEFT JOIN games AS g ON g.id = ug.game_id")
	}

	// Count query.
	countQuery := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("COUNT(DISTINCT ug.id)").
		Where("ug.user_id = ?", userID)
	countQuery = fb.Apply(countQuery)

	total, err := countQuery.ScanAndCount(ctx)
	// Actually, use Count():
	// We need a different approach. Let's use two queries.

	// Build base query.
	baseQuery := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("DISTINCT ug.id").
		Where("ug.user_id = ?", userID)
	baseQuery = fb.Apply(baseQuery)

	var totalCount int
	totalCount, err = h.db.NewSelect().
		TableExpr("(?) AS sub", baseQuery).
		Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Fetch IDs with sort + pagination.
	offset := (page - 1) * perPage
	orderExpr := sortExpr + " " + strings.ToUpper(sortOrder)

	var ids []string
	idQuery := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("DISTINCT ug.id").
		Where("ug.user_id = ?", userID)
	idQuery = fb.Apply(idQuery)
	// For title/release_date sort, we need the games join in the ORDER BY.
	if sortFieldsRequiringGamesJoin[sortBy] {
		// Join is already added via fb. Just add the ORDER BY.
	}
	err = idQuery.OrderExpr(orderExpr).Offset(offset).Limit(perPage).Scan(ctx, &ids)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Fetch full records with relations.
	var userGames []models.UserGame
	if len(ids) > 0 {
		err = h.db.NewSelect().Model(&userGames).
			Where("ug.id IN (?)", bun.In(ids)).
			Relation("Game").
			Relation("Platforms").
			Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
				return q.Relation("Tag")
			}).
			OrderExpr(orderExpr).
			Scan(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
	}
	if userGames == nil {
		userGames = []models.UserGame{}
	}

	pages := (totalCount + perPage - 1) / perPage

	return c.JSON(http.StatusOK, UserGameListResponse{
		UserGames: userGames,
		Total:     totalCount,
		Page:      page,
		PerPage:   perPage,
		Pages:     pages,
	})
}
```

Add these imports (merge with existing):
```go
"strconv"

"github.com/drzero42/nexorious-go/internal/filter"
```

- [ ] **Step 4: Register the list route**

In `router.go`, add the list route **before** the `/:id` routes to avoid route conflicts:

```go
userGamesGroup.GET("", ugh.HandleListUserGames)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestListUserGames" -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games list endpoint with filters and pagination"
```

---

### Task 7: Bulk Update & Bulk Delete

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write bulk operation tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestBulkUpdate ──────────────────────────────────────────────────────────

func TestBulkUpdate(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "bulk-upd")
	g1 := insertTestGame(t, db, "Bulk Upd 1")
	g2 := insertTestGame(t, db, "Bulk Upd 2")
	insertTestUserGame(t, db, "ug-bu-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bu-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids": []string{"ug-bu-1", "ug-bu-2"},
			"updates": map[string]any{
				"play_status": "completed",
				"is_loved":    true,
			},
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		updated := int(resp["updated"].(float64))
		if updated != 2 {
			t.Fatalf("expected updated=2, got %d", updated)
		}
	})

	t.Run("empty ids", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{},
			"updates": map[string]any{"is_loved": true},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty updates", func(t *testing.T) {
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bu-1"},
			"updates": map[string]any{},
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("skips non-owned ids", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "bulk-upd-other")
		rec := putJSONAuth(t, e, "/api/user-games/bulk-update", map[string]any{
			"ids":     []string{"ug-bu-1", "ug-bu-2"},
			"updates": map[string]any{"is_loved": false},
		}, token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		updated := int(resp["updated"].(float64))
		if updated != 0 {
			t.Fatalf("expected updated=0 for non-owned ids, got %d", updated)
		}
	})
}

// ─── TestBulkDelete ──────────────────────────────────────────────────────────

func TestBulkDelete(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "bulk-del")
	g1 := insertTestGame(t, db, "Bulk Del 1")
	g2 := insertTestGame(t, db, "Bulk Del 2")
	insertTestUserGame(t, db, "ug-bd-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bd-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"ids": []string{"ug-bd-1", "ug-bd-2"},
		})
		req := httptest.NewRequest(http.MethodDelete, "/api/user-games/bulk-delete", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		deleted := int(resp["deleted"].(float64))
		if deleted != 2 {
			t.Fatalf("expected deleted=2, got %d", deleted)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestBulkUpdate|TestBulkDelete" -v -count=1`
Expected: fail — routes/handlers don't exist

- [ ] **Step 3: Implement bulk update and bulk delete handlers**

Append to `internal/api/user_games.go`:

```go
// bulkUpdateRequest is the body for PUT /api/user-games/bulk-update.
type bulkUpdateRequest struct {
	IDs     []string       `json:"ids"`
	Updates map[string]any `json:"updates"`
}

// allowedBulkUpdateFields is the set of fields accepted in bulk update.
var allowedBulkUpdateFields = map[string]bool{
	"play_status":     true,
	"is_loved":        true,
	"personal_rating": true,
}

// HandleBulkUpdate handles PUT /api/user-games/bulk-update.
func (h *UserGamesHandler) HandleBulkUpdate(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids is required and must not be empty")
	}
	if len(req.Updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "updates must contain at least one field")
	}

	// Validate update fields.
	for key := range req.Updates {
		if !allowedBulkUpdateFields[key] {
			return echo.NewHTTPError(http.StatusBadRequest, "unknown or disallowed field: "+key)
		}
	}

	// Validate play_status if provided.
	if ps, ok := req.Updates["play_status"]; ok && ps != nil {
		psStr, isStr := ps.(string)
		if !isStr || !enum.PlayStatus(psStr).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid play_status")
		}
	}

	ctx := context.Background()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	defer func() { _ = tx.Rollback() }()

	setClauses := []string{"updated_at = ?"}
	args := []any{time.Now().UTC()}

	for key, val := range req.Updates {
		setClauses = append(setClauses, key+" = ?")
		args = append(args, val)
	}

	args = append(args, bun.In(req.IDs), userID)

	query := fmt.Sprintf(
		`UPDATE user_games SET %s WHERE id IN (?) AND user_id = ?`,
		strings.Join(setClauses, ", "),
	)

	result, err := tx.NewRaw(query, args...).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, _ := result.RowsAffected()
	return c.JSON(http.StatusOK, map[string]int64{"updated": rows})
}

// bulkDeleteRequest is the body for DELETE /api/user-games/bulk-delete.
type bulkDeleteRequest struct {
	IDs []string `json:"ids"`
}

// HandleBulkDelete handles DELETE /api/user-games/bulk-delete.
func (h *UserGamesHandler) HandleBulkDelete(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids is required and must not be empty")
	}

	ctx := context.Background()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.NewDelete().Model((*models.UserGame)(nil)).
		Where("id IN (?)", bun.In(req.IDs)).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, _ := result.RowsAffected()
	return c.JSON(http.StatusOK, map[string]int64{"deleted": rows})
}
```

- [ ] **Step 4: Register bulk routes**

In `router.go`, add these **before** the `/:id` routes to avoid route conflicts:

```go
userGamesGroup.PUT("/bulk-update", ugh.HandleBulkUpdate)
userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestBulkUpdate|TestBulkDelete" -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games bulk-update and bulk-delete endpoints"
```

---

### Task 8: Bulk Platform Operations

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write bulk platform tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestBulkAddPlatforms ────────────────────────────────────────────────────

func TestBulkAddPlatforms(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "bulk-plat-add")
	g1 := insertTestGame(t, db, "Bulk Plat Add 1")
	g2 := insertTestGame(t, db, "Bulk Plat Add 2")
	insertTestUserGame(t, db, "ug-bpa-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-bpa-2", userID, int(g2))

	t.Run("success", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/bulk-add-platforms", map[string]any{
			"user_game_ids": []string{"ug-bpa-1", "ug-bpa-2"},
			"platform":      "pc",
			"storefront":    "steam",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		added := int(resp["added"].(float64))
		if added != 2 {
			t.Fatalf("expected added=2, got %d", added)
		}
	})

	t.Run("duplicate skipped", func(t *testing.T) {
		// Re-add same platform — should skip duplicates.
		rec := postJSONAuth(t, e, "/api/user-games/bulk-add-platforms", map[string]any{
			"user_game_ids": []string{"ug-bpa-1"},
			"platform":      "pc",
			"storefront":    "steam",
		}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		added := int(resp["added"].(float64))
		if added != 0 {
			t.Fatalf("expected added=0 for duplicate, got %d", added)
		}
	})
}

// ─── TestBulkRemovePlatforms ─────────────────────────────────────────────────

func TestBulkRemovePlatforms(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "bulk-plat-rm")
	g1 := insertTestGame(t, db, "Bulk Plat Rm 1")
	insertTestUserGame(t, db, "ug-bpr-1", userID, int(g1))
	pc := "pc"
	steam := "steam"
	insertTestUserGamePlatform(t, db, "ugp-bpr-1", "ug-bpr-1", &pc, &steam)

	t.Run("success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"user_game_ids": []string{"ug-bpr-1"},
			"platform":      "pc",
			"storefront":    "steam",
		})
		req := httptest.NewRequest(http.MethodDelete, "/api/user-games/bulk-remove-platforms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		removed := int(resp["removed"].(float64))
		if removed != 1 {
			t.Fatalf("expected removed=1, got %d", removed)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestBulkAddPlatforms|TestBulkRemovePlatforms" -v -count=1`
Expected: fail

- [ ] **Step 3: Implement bulk platform handlers**

Append to `internal/api/user_games.go`:

```go
// bulkAddPlatformsRequest is the body for POST /api/user-games/bulk-add-platforms.
type bulkAddPlatformsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
	Platform    string   `json:"platform"`
	Storefront  string   `json:"storefront"`
}

// HandleBulkAddPlatforms handles POST /api/user-games/bulk-add-platforms.
func (h *UserGamesHandler) HandleBulkAddPlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkAddPlatformsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids is required")
	}

	ctx := context.Background()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	defer func() { _ = tx.Rollback() }()

	// Verify ownership — get only IDs that belong to the current user.
	var ownedIDs []string
	err = tx.NewSelect().Model((*models.UserGame)(nil)).
		Column("id").
		Where("id IN (?)", bun.In(req.UserGameIDs)).
		Where("user_id = ?", userID).
		Scan(ctx, &ownedIDs)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if len(ownedIDs) == 0 {
		return c.JSON(http.StatusOK, map[string]int{"added": 0})
	}

	now := time.Now().UTC()
	var added int64
	for _, ugID := range ownedIDs {
		platform := req.Platform
		storefront := req.Storefront
		result, err := tx.NewRaw(
			`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			uuid.NewString(), ugID, &platform, &storefront, now, now,
		).Exec(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		rows, _ := result.RowsAffected()
		added += rows
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, map[string]int64{"added": added})
}

// bulkRemovePlatformsRequest is the body for DELETE /api/user-games/bulk-remove-platforms.
type bulkRemovePlatformsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
	Platform    string   `json:"platform"`
	Storefront  string   `json:"storefront"`
}

// HandleBulkRemovePlatforms handles DELETE /api/user-games/bulk-remove-platforms.
func (h *UserGamesHandler) HandleBulkRemovePlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var req bulkRemovePlatformsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids is required")
	}

	ctx := context.Background()

	// Delete only for user-owned user games.
	result, err := h.db.NewRaw(
		`DELETE FROM user_game_platforms
		 WHERE user_game_id IN (
		   SELECT id FROM user_games WHERE id IN (?) AND user_id = ?
		 )
		 AND platform = ? AND storefront = ?`,
		bun.In(req.UserGameIDs), userID, req.Platform, req.Storefront,
	).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, _ := result.RowsAffected()
	return c.JSON(http.StatusOK, map[string]int64{"removed": rows})
}
```

- [ ] **Step 4: Register bulk platform routes**

In `router.go`, add to the `userGamesGroup`:

```go
userGamesGroup.POST("/bulk-add-platforms", ugh.HandleBulkAddPlatforms)
userGamesGroup.DELETE("/bulk-remove-platforms", ugh.HandleBulkRemovePlatforms)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestBulkAddPlatforms|TestBulkRemovePlatforms" -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games bulk-add-platforms and bulk-remove-platforms endpoints"
```

---

### Task 9: Platform Sub-Resource CRUD

**Files:**
- Modify: `internal/api/user_games.go`
- Modify: `internal/api/user_games_test.go`

- [ ] **Step 1: Write platform CRUD tests**

Append to `internal/api/user_games_test.go`:

```go
// ─── TestPlatformCRUD ────────────────────────────────────────────────────────

func TestPlatformCRUD(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)

	userID, token := setupUserGamesUser(t, db, e, "plat-crud")
	gameID := insertTestGame(t, db, "Plat CRUD Game")
	insertTestUserGame(t, db, "ug-plat-1", userID, int(gameID))

	t.Run("list empty", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var platforms []any
		_ = json.Unmarshal(rec.Body.Bytes(), &platforms)
		if len(platforms) != 0 {
			t.Fatalf("expected 0 platforms, got %d", len(platforms))
		}
	})

	var platformID string

	t.Run("create", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":         "pc",
			"storefront":       "steam",
			"ownership_status": "owned",
		}, token)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		platformID = resp["id"].(string)
		if platformID == "" {
			t.Fatal("expected non-empty id")
		}
	})

	t.Run("create duplicate", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":   "pc",
			"storefront": "steam",
		}, token)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 for duplicate, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create invalid ownership_status", func(t *testing.T) {
		rec := postJSONAuth(t, e, "/api/user-games/ug-plat-1/platforms", map[string]any{
			"platform":         "pc",
			"storefront":       "gog",
			"ownership_status": "stolen",
		}, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list after create", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var platforms []any
		_ = json.Unmarshal(rec.Body.Bytes(), &platforms)
		if len(platforms) != 1 {
			t.Fatalf("expected 1 platform, got %d", len(platforms))
		}
	})

	t.Run("update", func(t *testing.T) {
		rec := putJSONAuth(t, e,
			fmt.Sprintf("/api/user-games/ug-plat-1/platforms/%s", platformID),
			map[string]any{
				"ownership_status": "subscription",
			}, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delete", func(t *testing.T) {
		rec := deleteAuth(t, e,
			fmt.Sprintf("/api/user-games/ug-plat-1/platforms/%s", platformID), token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong owner", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "plat-crud-other")
		rec := getAuth(t, e, "/api/user-games/ug-plat-1/platforms", token2)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for wrong owner, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run "TestPlatformCRUD" -v -count=1`
Expected: fail

- [ ] **Step 3: Implement platform sub-resource handlers**

Append to `internal/api/user_games.go`:

```go
// verifyUserGameOwnership checks that the user game exists and belongs to the user.
// Returns an error response if not.
func (h *UserGamesHandler) verifyUserGameOwnership(ctx context.Context, userGameID, userID string) error {
	var exists bool
	err := h.db.NewSelect().Model((*models.UserGame)(nil)).
		ColumnExpr("1").
		Where("id = ?", userGameID).
		Where("user_id = ?", userID).
		Scan(ctx, &exists)
	if err != nil {
		return err
	}
	return nil
}

// HandleListPlatforms handles GET /api/user-games/:id/platforms.
func (h *UserGamesHandler) HandleListPlatforms(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	userGameID := c.Param("id")
	ctx := context.Background()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	var platforms []models.UserGamePlatform
	err := h.db.NewSelect().Model(&platforms).
		Where("user_game_id = ?", userGameID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if platforms == nil {
		platforms = []models.UserGamePlatform{}
	}

	return c.JSON(http.StatusOK, platforms)
}

// createPlatformRequest is the body for POST /api/user-games/:id/platforms.
type createPlatformRequest struct {
	Platform        *string    `json:"platform"`
	Storefront      *string    `json:"storefront"`
	StoreGameID     *string    `json:"store_game_id"`
	StoreUrl        *string    `json:"store_url"`
	IsAvailable     bool       `json:"is_available"`
	HoursPlayed     *float64   `json:"hours_played"`
	OwnershipStatus *string    `json:"ownership_status"`
	AcquiredDate    *time.Time `json:"acquired_date"`
}

// HandleCreatePlatform handles POST /api/user-games/:id/platforms.
func (h *UserGamesHandler) HandleCreatePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	userGameID := c.Param("id")
	ctx := context.Background()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	var req createPlatformRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate platform exists in platforms table if provided.
	if req.Platform != nil && *req.Platform != "" {
		var platformExists bool
		err := h.db.NewSelect().TableExpr("platforms").
			ColumnExpr("1").
			Where("name = ?", *req.Platform).
			Scan(ctx, &platformExists)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "platform not found")
		}
	}

	// Validate storefront exists if provided.
	if req.Storefront != nil && *req.Storefront != "" {
		var sfExists bool
		err := h.db.NewSelect().TableExpr("storefronts").
			ColumnExpr("1").
			Where("name = ?", *req.Storefront).
			Scan(ctx, &sfExists)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "storefront not found")
		}
	}

	// Validate ownership_status if provided.
	if req.OwnershipStatus != nil && *req.OwnershipStatus != "" {
		if !enum.OwnershipStatus(*req.OwnershipStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ownership_status")
		}
	}

	now := time.Now().UTC()
	plat := &models.UserGamePlatform{
		ID:              uuid.NewString(),
		UserGameID:      userGameID,
		Platform:        req.Platform,
		Storefront:      req.Storefront,
		StoreGameID:     req.StoreGameID,
		StoreUrl:        req.StoreUrl,
		IsAvailable:     req.IsAvailable,
		HoursPlayed:     req.HoursPlayed,
		OwnershipStatus: req.OwnershipStatus,
		AcquiredDate:    req.AcquiredDate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	_, err := h.db.NewInsert().Model(plat).Exec(ctx)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "platform and storefront association already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusCreated, plat)
}

// HandleUpdatePlatform handles PUT /api/user-games/:id/platforms/:platform_id.
func (h *UserGamesHandler) HandleUpdatePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	userGameID := c.Param("id")
	platformID := c.Param("platform_id")
	ctx := context.Background()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	var req createPlatformRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	// Validate ownership_status if provided.
	if req.OwnershipStatus != nil && *req.OwnershipStatus != "" {
		if !enum.OwnershipStatus(*req.OwnershipStatus).Valid() {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid ownership_status")
		}
	}

	// Validate platform exists if provided.
	if req.Platform != nil && *req.Platform != "" {
		var platformExists bool
		err := h.db.NewSelect().TableExpr("platforms").
			ColumnExpr("1").
			Where("name = ?", *req.Platform).
			Scan(ctx, &platformExists)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "platform not found")
		}
	}

	// Validate storefront exists if provided.
	if req.Storefront != nil && *req.Storefront != "" {
		var sfExists bool
		err := h.db.NewSelect().TableExpr("storefronts").
			ColumnExpr("1").
			Where("name = ?", *req.Storefront).
			Scan(ctx, &sfExists)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "storefront not found")
		}
	}

	// Fetch existing platform.
	var plat models.UserGamePlatform
	err := h.db.NewSelect().Model(&plat).
		Where("id = ?", platformID).
		Where("user_game_id = ?", userGameID).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "platform not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Apply updates.
	if req.Platform != nil {
		plat.Platform = req.Platform
	}
	if req.Storefront != nil {
		plat.Storefront = req.Storefront
	}
	if req.StoreGameID != nil {
		plat.StoreGameID = req.StoreGameID
	}
	if req.StoreUrl != nil {
		plat.StoreUrl = req.StoreUrl
	}
	if req.OwnershipStatus != nil {
		plat.OwnershipStatus = req.OwnershipStatus
	}
	if req.HoursPlayed != nil {
		plat.HoursPlayed = req.HoursPlayed
	}
	if req.AcquiredDate != nil {
		plat.AcquiredDate = req.AcquiredDate
	}
	plat.IsAvailable = req.IsAvailable
	plat.UpdatedAt = time.Now().UTC()

	_, err = h.db.NewUpdate().Model(&plat).WherePK().Exec(ctx)
	if err != nil {
		if isDuplicateKeyError(err) {
			return echo.NewHTTPError(http.StatusConflict, "platform and storefront association already exists")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	return c.JSON(http.StatusOK, plat)
}

// HandleDeletePlatform handles DELETE /api/user-games/:id/platforms/:platform_id.
func (h *UserGamesHandler) HandleDeletePlatform(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	userGameID := c.Param("id")
	platformID := c.Param("platform_id")
	ctx := context.Background()

	if err := h.verifyUserGameOwnership(ctx, userGameID, userID); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	}

	result, err := h.db.NewDelete().Model((*models.UserGamePlatform)(nil)).
		Where("id = ?", platformID).
		Where("user_game_id = ?", userGameID).
		Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "platform not found")
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Register platform sub-resource routes**

In `router.go`, add to the `userGamesGroup`:

```go
userGamesGroup.GET("/:id/platforms", ugh.HandleListPlatforms)
userGamesGroup.POST("/:id/platforms", ugh.HandleCreatePlatform)
userGamesGroup.PUT("/:id/platforms/:platform_id", ugh.HandleUpdatePlatform)
userGamesGroup.DELETE("/:id/platforms/:platform_id", ugh.HandleDeletePlatform)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/... -run "TestPlatformCRUD" -v -count=1`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go internal/api/router.go
git commit -m "feat: add user-games platform sub-resource CRUD endpoints"
```

---

### Task 10: Full Test Suite Run & Slumber Collection

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass, zero failures

- [ ] **Step 2: Run golangci-lint**

Run: `golangci-lint run`
Expected: no errors

- [ ] **Step 3: Add user-games requests to slumber.yaml**

Add a `user_games` folder to the `requests` section of `slumber.yaml` (alphabetical order, after `tags`):

```yaml
  user_games:
    name: User Games
    requests:
      list:
        name: List User Games
        method: GET
        url: "{{base_url}}/api/user-games"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        query:
          - name: page
            value: "1"
          - name: per_page
            value: "20"

      get:
        name: Get User Game
        method: GET
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      create:
        name: Create User Game
        method: POST
        url: "{{base_url}}/api/user-games"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            game_id: 1
            play_status: not_started

      update:
        name: Update User Game
        method: PUT
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            play_status: completed
            is_loved: true

      update_progress:
        name: Update Progress
        method: PUT
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID/progress"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            hours_played: 12.5
            play_status: in_progress

      delete:
        name: Delete User Game
        method: DELETE
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      bulk_update:
        name: Bulk Update
        method: PUT
        url: "{{base_url}}/api/user-games/bulk-update"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            ids:
              - "REPLACE_WITH_ID_1"
              - "REPLACE_WITH_ID_2"
            updates:
              play_status: completed

      bulk_delete:
        name: Bulk Delete
        method: DELETE
        url: "{{base_url}}/api/user-games/bulk-delete"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            ids:
              - "REPLACE_WITH_ID_1"

      list_platforms:
        name: List Platforms
        method: GET
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID/platforms"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      create_platform:
        name: Create Platform
        method: POST
        url: "{{base_url}}/api/user-games/REPLACE_WITH_ID/platforms"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
        body:
          type: json
          data:
            platform: pc
            storefront: steam
            ownership_status: owned
```

- [ ] **Step 4: Verify slumber collection loads**

Run: `slumber show collection`
Expected: collection loads without errors, user_games folder visible

- [ ] **Step 5: Commit**

```bash
git add slumber.yaml
git commit -m "feat: add user-games requests to slumber collection"
```

---

### Task 11: Final Verification

- [ ] **Step 1: Run all tests one final time**

Run: `go test ./... -count=1 -v`
Expected: all tests pass

- [ ] **Step 2: Build the binary**

Run: `make build`
Expected: builds successfully

- [ ] **Step 3: Run golangci-lint one final time**

Run: `golangci-lint run`
Expected: no errors
