# External Games Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-storefront External Games section to each sync detail page, letting users see all their external games, change IGDB matches, and manage skipped games.

**Architecture:** New backend endpoints (`GET /:storefront/external-games`, `POST /external-games/:id/rematch`) added to `SyncHandler`; `HandleUnskipGame` extended to enqueue immediate re-processing. Frontend gains an `ExternalGamesSection` component on each sync detail page with three collapsible subsections (Unmatched / Skipped / Matched) and an IGDB match dialog that wraps the existing `IGDBSearch` component.

**Tech Stack:** Go + Echo v5 + Bun ORM + River (backend); React 19 + TanStack Query + shadcn/ui (frontend); Bun migrate (migrations).

---

## File Map

**Create:**
- `internal/db/migrations/20260516000001_external_games_raw_platform.up.sql`
- `internal/db/migrations/20260516000001_external_games_raw_platform.down.sql`
- `ui/frontend/src/components/sync/igdb-match-dialog.tsx`
- `ui/frontend/src/components/sync/external-games-section.tsx`

**Modify:**
- `internal/db/models/models.go` — add `RawPlatform` field to `ExternalGame`
- `internal/worker/tasks/sync.go` — persist `raw_platform` during upsert
- `internal/api/sync.go` — add `HandleListExternalGames`, `HandleRematchExternalGame`, extend `HandleUnskipGame`, update `RegisterRoutes`
- `internal/api/sync_test.go` — new tests
- `ui/frontend/src/types/sync.ts` — add `ExternalGame` interface
- `ui/frontend/src/api/sync.ts` — add `getExternalGames`, `skipExternalGame`, `unskipExternalGame`, `rematchExternalGame`
- `ui/frontend/src/api/index.ts` — export new functions
- `ui/frontend/src/hooks/use-sync.ts` — add `useExternalGames`, `useSkipExternalGame`, `useUnskipExternalGame`, `useRematchExternalGame`
- `ui/frontend/src/components/sync/index.ts` — export new components
- `ui/frontend/src/routes/_authenticated/sync/$platform.tsx` — mount `ExternalGamesSection`
- `slumber.yaml` — add new endpoints

---

## Task 1: Migration — add `raw_platform` to `external_games`

`raw_platform` is needed so unskip and rematch can create job_items with the correct platform metadata, without re-fetching from the storefront. The DispatchSyncWorker will populate it on every sync upsert; existing rows get an empty default (they will be updated on next full sync).

**Files:**
- Create: `internal/db/migrations/20260516000001_external_games_raw_platform.up.sql`
- Create: `internal/db/migrations/20260516000001_external_games_raw_platform.down.sql`

- [ ] **Step 1: Write the up migration**

`internal/db/migrations/20260516000001_external_games_raw_platform.up.sql`:
```sql
ALTER TABLE external_games ADD COLUMN raw_platform text NOT NULL DEFAULT '';
```

- [ ] **Step 2: Write the down migration**

`internal/db/migrations/20260516000001_external_games_raw_platform.down.sql`:
```sql
ALTER TABLE external_games DROP COLUMN raw_platform;
```

- [ ] **Step 3: Run the migration**

```bash
./nexorious migrate
```

Expected: migration applies cleanly, `./nexorious migrate status` shows it as applied.

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/20260516000001_external_games_raw_platform.up.sql \
        internal/db/migrations/20260516000001_external_games_raw_platform.down.sql
git commit -m "feat(db): add raw_platform column to external_games"
```

---

## Task 2: Persist `raw_platform` in model and sync worker

**Files:**
- Modify: `internal/db/models/models.go`
- Modify: `internal/worker/tasks/sync.go`

- [ ] **Step 1: Add `RawPlatform` to the `ExternalGame` model**

In `internal/db/models/models.go`, add one field to `ExternalGame` after `OwnershipStatus`:

```go
RawPlatform     string    `bun:"raw_platform,notnull"    json:"raw_platform"`
```

The full struct becomes:
```go
type ExternalGame struct {
	bun.BaseModel `bun:"table:external_games"`

	ID              string    `bun:"id,pk"                  json:"id"`
	UserID          string    `bun:"user_id,notnull"         json:"user_id"`
	Storefront      string    `bun:"storefront,notnull"      json:"storefront"`
	ExternalID      string    `bun:"external_id,notnull"     json:"external_id"`
	Title           string    `bun:"title,notnull"           json:"title"`
	ResolvedIGDBID  *int32    `bun:"resolved_igdb_id"        json:"resolved_igdb_id"`
	IsSkipped       bool      `bun:"is_skipped,notnull"      json:"is_skipped"`
	IsAvailable     bool      `bun:"is_available,notnull"    json:"is_available"`
	IsSubscription  bool      `bun:"is_subscription,notnull" json:"is_subscription"`
	PlaytimeHours   int       `bun:"playtime_hours,notnull"  json:"playtime_hours"`
	OwnershipStatus *string   `bun:"ownership_status"        json:"ownership_status"`
	RawPlatform     string    `bun:"raw_platform,notnull"    json:"raw_platform"`
	CreatedAt       time.Time `bun:"created_at,notnull"      json:"created_at"`
	UpdatedAt       time.Time `bun:"updated_at,notnull"      json:"updated_at"`
}
```

- [ ] **Step 2: Set `RawPlatform` when building the upsert row in `sync.go`**

In `internal/worker/tasks/sync.go`, inside the `for _, e := range entries` loop (around line 180), add `RawPlatform: e.RawPlatform` to the `ExternalGame` row:

```go
row := &models.ExternalGame{
    ID:              uuid.NewString(),
    UserID:          p.UserID,
    Storefront:      p.Storefront,
    ExternalID:      e.ExternalID,
    Title:           e.Title,
    IsAvailable:     true,
    IsSubscription:  e.IsSubscription,
    PlaytimeHours:   e.PlaytimeHours,
    OwnershipStatus: &ownership,
    RawPlatform:     e.RawPlatform,
    CreatedAt:       upsertNow,
    UpdatedAt:       upsertNow,
}
```

Also extend the `ON CONFLICT ... DO UPDATE` clause to include `raw_platform = EXCLUDED.raw_platform`:

```go
_, _ = w.DB.NewInsert().Model(row).
    On(`CONFLICT (user_id, storefront, external_id) DO UPDATE SET
        title = EXCLUDED.title,
        playtime_hours = EXCLUDED.playtime_hours,
        is_subscription = EXCLUDED.is_subscription,
        ownership_status = EXCLUDED.ownership_status,
        raw_platform = EXCLUDED.raw_platform,
        is_available = true,
        updated_at = now()`).
    Exec(ctx)
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
make build
```

Expected: compiles without errors.

- [ ] **Step 4: Run the Go tests**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass (the sync worker tests use `insertExternalGame` which inserts with defaults; `raw_platform` defaults to `''` so no breakage).

- [ ] **Step 5: Commit**

```bash
git add internal/db/models/models.go internal/worker/tasks/sync.go
git commit -m "feat(sync): persist raw_platform on external_games during sync upsert"
```

---

## Task 3: Backend — `GET /api/sync/:storefront/external-games`

Returns all external games for one storefront with join data needed for the orphan warning on the frontend.

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Add at the bottom of `internal/api/sync_test.go`:

```go
// ─── TestListExternalGames ────────────────────────────────────────────────────

func TestListExternalGames_EmptyList(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "eg-empty")

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty list, got %d items", len(resp))
	}
}

func TestListExternalGames_InvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "eg-bad-sf")

	rec := getAuth(t, e, "/api/sync/notaplatform/external-games", token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListExternalGames_IsolatedByUser(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userA, tokenA := setupTagUser(t, testDB, e, "eg-user-a")
	_, tokenB := setupTagUser(t, testDB, e, "eg-user-b")
	insertExternalGame(t, testDB, "eg-a1", userA, "steam", "730", "CS2")

	rec := getAuth(t, e, "/api/sync/steam/external-games", tokenA)
	var respA []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &respA)
	if len(respA) != 1 {
		t.Fatalf("user A should see 1 game, got %d", len(respA))
	}

	rec2 := getAuth(t, e, "/api/sync/steam/external-games", tokenB)
	var respB []map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &respB)
	if len(respB) != 0 {
		t.Fatalf("user B should see 0 games, got %d", len(respB))
	}
}

func TestListExternalGames_AllStates(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "eg-states")
	insertExternalGame(t, testDB, "eg-matched", userID, "steam", "1", "Matched Game")
	insertExternalGame(t, testDB, "eg-unmatched", userID, "steam", "2", "Unmatched Game")
	insertExternalGame(t, testDB, "eg-skipped", userID, "steam", "3", "Skipped Game")

	// Mark skipped
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET is_skipped = true WHERE id = 'eg-skipped'`)
	// Set resolved IGDB ID (insert games row first to satisfy FK)
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (9999, 'IGDB Title', now(), now()) ON CONFLICT DO NOTHING`)
	_, _ = testDB.ExecContext(context.Background(),
		`UPDATE external_games SET resolved_igdb_id = 9999 WHERE id = 'eg-matched'`)

	rec := getAuth(t, e, "/api/sync/steam/external-games", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp) != 3 {
		t.Fatalf("expected 3 games, got %d", len(resp))
	}
	byID := make(map[string]map[string]any)
	for _, g := range resp {
		byID[g["id"].(string)] = g
	}
	if byID["eg-matched"]["igdb_title"] != "IGDB Title" {
		t.Errorf("expected igdb_title='IGDB Title', got %v", byID["eg-matched"]["igdb_title"])
	}
	if byID["eg-unmatched"]["igdb_title"] != nil {
		t.Errorf("expected igdb_title=nil for unmatched, got %v", byID["eg-unmatched"]["igdb_title"])
	}
	if byID["eg-skipped"]["is_skipped"] != true {
		t.Errorf("expected is_skipped=true for skipped game")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
go test ./internal/api/... -run TestListExternalGames -v
```

Expected: FAIL (handler not implemented yet).

- [ ] **Step 3: Add the response type and handler to `sync.go`**

Add this type after the existing `syncStatusResponse` type (around line 99):

```go
type externalGameResponse struct {
	ID                         string  `bun:"id"                             json:"id"`
	Storefront                 string  `bun:"storefront"                     json:"storefront"`
	ExternalID                 string  `bun:"external_id"                    json:"external_id"`
	Title                      string  `bun:"title"                          json:"title"`
	ResolvedIGDBID             *int32  `bun:"resolved_igdb_id"               json:"resolved_igdb_id"`
	IsSkipped                  bool    `bun:"is_skipped"                     json:"is_skipped"`
	IsAvailable                bool    `bun:"is_available"                   json:"is_available"`
	IsSubscription             bool    `bun:"is_subscription"                json:"is_subscription"`
	PlaytimeHours              int     `bun:"playtime_hours"                 json:"playtime_hours"`
	HasUserGame                bool    `bun:"has_user_game"                  json:"has_user_game"`
	UserGameID                 *string `bun:"user_game_id"                   json:"user_game_id"`
	IGDBTitle                  *string `bun:"igdb_title"                     json:"igdb_title"`
	UserGameOtherPlatformCount int     `bun:"user_game_other_platform_count" json:"user_game_other_platform_count"`
}
```

Add the handler method at the end of `sync.go` (before `HandleSkipGame`):

```go
func (h *SyncHandler) HandleListExternalGames(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	sf := c.Param("storefront")
	if !validTriggerStorefronts[sf] {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid storefront")
	}
	ctx := context.Background()

	var games []externalGameResponse
	err := h.db.NewRaw(`
		SELECT
			eg.id,
			eg.storefront,
			eg.external_id,
			eg.title,
			eg.resolved_igdb_id,
			eg.is_skipped,
			eg.is_available,
			eg.is_subscription,
			eg.playtime_hours,
			(ugp.user_game_id IS NOT NULL) AS has_user_game,
			ugp.user_game_id,
			g.title AS igdb_title,
			COALESCE(
				(SELECT COUNT(*) FROM user_game_platforms o
				 WHERE o.user_game_id = ugp.user_game_id AND o.id != ugp.id),
				0
			) AS user_game_other_platform_count
		FROM external_games eg
		LEFT JOIN user_game_platforms ugp ON ugp.external_game_id = eg.id
		LEFT JOIN games g ON g.id = eg.resolved_igdb_id
		WHERE eg.user_id = ? AND eg.storefront = ?
		ORDER BY eg.title ASC`,
		userID, sf,
	).Scan(ctx, &games)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list external games")
	}
	if games == nil {
		games = []externalGameResponse{}
	}
	return c.JSON(http.StatusOK, games)
}
```

- [ ] **Step 4: Register the route in `RegisterRoutes`**

In `RegisterRoutes`, add the new GET route **before** `g.POST("/:storefront", ...)` (static segment takes priority, and having it nearby keeps the routes readable). Also add the rematch route placeholder now to ensure correct ordering:

```go
func (h *SyncHandler) RegisterRoutes(g *echo.Group) {
	g.POST("/steam/verify", h.HandleSteamVerify)
	g.DELETE("/steam/connection", h.HandleSteamDisconnect)
	g.POST("/psn/configure", h.HandlePSNConfigure)
	g.GET("/psn/status", h.HandleGetPSNStatus)
	g.DELETE("/psn/disconnect", h.HandlePSNDisconnect)
	g.GET("/ignored", h.HandleListIgnored)
	g.POST("/ignored/:id", h.HandleSkipGame)
	g.DELETE("/ignored/:id", h.HandleUnskipGame)
	// "external-games" is a static prefix — must be registered before /:storefront (POST)
	// per Echo v5 route ordering rules.
	g.POST("/external-games/:id/rematch", h.HandleRematchExternalGame) // implemented in Task 4
	g.GET("/config", h.HandleListConfig)
	g.GET("/config/:storefront", h.HandleGetConfig)
	g.PUT("/config/:storefront", h.HandleUpdateConfig)
	g.POST("/:storefront", h.HandleTriggerSync)
	g.GET("/:storefront/status", h.HandleGetSyncStatus)
	g.GET("/:storefront/external-games", h.HandleListExternalGames)
}
```

Add a stub for `HandleRematchExternalGame` so it compiles (full implementation in Task 4):

```go
func (h *SyncHandler) HandleRematchExternalGame(c *echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "not yet implemented")
}
```

- [ ] **Step 5: Run the tests**

```bash
go test ./internal/api/... -run TestListExternalGames -v
```

Expected: all `TestListExternalGames_*` tests pass.

- [ ] **Step 6: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): add GET /:storefront/external-games endpoint"
```

---

## Task 4: Backend — `POST /api/sync/external-games/:id/rematch`

Un-imports from the old user_game, updates the IGDB match, and enqueues a re-import.

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing tests**

Add at the bottom of `internal/api/sync_test.go`:

```go
// ─── TestRematchExternalGame ──────────────────────────────────────────────────

func insertUserGameAndPlatform(t *testing.T, db *bun.DB, ugID, userID, gameIDInt, ugpID, externalGameID string) {
	t.Helper()
	gameID := 0
	fmt.Sscanf(gameIDInt, "%d", &gameID)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, 'Test Game', now(), now()) ON CONFLICT DO NOTHING`,
		gameID)
	if err != nil {
		t.Fatalf("insert game: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, now(), now()) ON CONFLICT DO NOTHING`,
		ugID, userID, gameID)
	if err != nil {
		t.Fatalf("insert user_game: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO user_game_platforms (id, user_game_id, external_game_id, storefront, sync_from_source, is_available, created_at, updated_at)
		 VALUES (?, ?, ?, 'steam', true, true, now(), now())`,
		ugpID, ugID, externalGameID)
	if err != nil {
		t.Fatalf("insert user_game_platform: %v", err)
	}
}

func TestRematchExternalGame_NotFound(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	_, token := setupTagUser(t, testDB, e, "rm-404")

	rec := postJSONAuth(t, e, "/api/sync/external-games/nonexistent/rematch",
		map[string]any{"igdb_id": 1234}, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRematchExternalGame_OtherUsersGame(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userA, _ := setupTagUser(t, testDB, e, "rm-userA")
	_, tokenB := setupTagUser(t, testDB, e, "rm-userB")
	insertExternalGame(t, testDB, "eg-rm-a", userA, "steam", "42", "Some Game")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-a/rematch",
		map[string]any{"igdb_id": 1234}, tokenB)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for other user's game, got %d", rec.Code)
	}
}

func TestRematchExternalGame_OrphanRequiresAction(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-orphan")
	insertExternalGame(t, testDB, "eg-rm-1", userID, "steam", "1", "Game")
	// Link to a user_game that has only this one platform
	insertUserGameAndPlatform(t, testDB, testDB, "ug-rm-1", userID, "1111", "ugp-rm-1", "eg-rm-1")

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-1/rematch",
		map[string]any{"igdb_id": 9999}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when orphan_action missing, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRematchExternalGame_KeepOrphan(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-keep")
	insertExternalGame(t, testDB, "eg-rm-2", userID, "steam", "2", "Game Two")
	insertUserGameAndPlatform(t, testDB, testDB, "ug-rm-2", userID, "2222", "ugp-rm-2", "eg-rm-2")

	// Insert the target games row so FK is satisfied
	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (8888, 'New IGDB Game', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-2/rematch",
		map[string]any{"igdb_id": 8888, "orphan_action": "keep"}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// user_game_platform should be gone
	var ugpCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_game_platforms WHERE id = 'ugp-rm-2'`).Scan(context.Background(), &ugpCount)
	if ugpCount != 0 {
		t.Fatal("expected user_game_platform to be deleted")
	}
	// user_game should still exist
	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-rm-2'`).Scan(context.Background(), &ugCount)
	if ugCount != 1 {
		t.Fatal("expected user_game to be kept with orphan_action=keep")
	}
	// external_game resolved_igdb_id updated
	var resolvedID int32
	_ = testDB.NewRaw(`SELECT resolved_igdb_id FROM external_games WHERE id = 'eg-rm-2'`).Scan(context.Background(), &resolvedID)
	if resolvedID != 8888 {
		t.Fatalf("expected resolved_igdb_id=8888, got %d", resolvedID)
	}
}

func TestRematchExternalGame_RemoveOrphan(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "rm-remove")
	insertExternalGame(t, testDB, "eg-rm-3", userID, "steam", "3", "Game Three")
	insertUserGameAndPlatform(t, testDB, testDB, "ug-rm-3", userID, "3333", "ugp-rm-3", "eg-rm-3")

	_, _ = testDB.ExecContext(context.Background(),
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (7777, 'Another Game', now(), now()) ON CONFLICT DO NOTHING`)

	rec := postJSONAuth(t, e, "/api/sync/external-games/eg-rm-3/rematch",
		map[string]any{"igdb_id": 7777, "orphan_action": "remove"}, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var ugCount int
	_ = testDB.NewRaw(`SELECT COUNT(*) FROM user_games WHERE id = 'ug-rm-3'`).Scan(context.Background(), &ugCount)
	if ugCount != 0 {
		t.Fatal("expected user_game to be deleted with orphan_action=remove")
	}
}
```

Note: the `insertUserGameAndPlatform` helper uses `fmt` — add `"fmt"` to the test file imports if not already present.

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
go test ./internal/api/... -run TestRematchExternalGame -v
```

Expected: FAIL (handler returns 501).

- [ ] **Step 3: Replace the stub with the full `HandleRematchExternalGame`**

In `internal/api/sync.go`, replace the stub with:

```go
func (h *SyncHandler) HandleRematchExternalGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var body struct {
		IGDBID      int    `json:"igdb_id"`
		OrphanAction string `json:"orphan_action"`
	}
	if err := c.Bind(&body); err != nil || body.IGDBID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "igdb_id is required")
	}

	// Verify ownership.
	var eg models.ExternalGame
	err := h.db.NewSelect().Model(&eg).Where("id = ? AND user_id = ?", id, userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load external game")
	}

	// Find the existing user_game_platform linked to this external game.
	var ugpID, ugID string
	err = h.db.NewRaw(
		`SELECT id, user_game_id FROM user_game_platforms WHERE external_game_id = ? LIMIT 1`, id,
	).Scan(ctx, &ugpID, &ugID)
	platformFound := err == nil

	if platformFound {
		// Count other platforms on the same user_game.
		var otherCount int
		_ = h.db.NewRaw(
			`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
		).Scan(ctx, &otherCount)

		// Require orphan_action when this is the last platform.
		if otherCount == 0 && body.OrphanAction == "" {
			return echo.NewHTTPError(http.StatusConflict, "orphan_action required: game would lose its only storefront link")
		}

		// Delete the platform link.
		_, _ = h.db.NewRaw(`DELETE FROM user_game_platforms WHERE id = ?`, ugpID).Exec(ctx)

		// Apply orphan decision.
		if otherCount == 0 && body.OrphanAction == "remove" {
			_, _ = h.db.NewRaw(`DELETE FROM user_games WHERE id = ?`, ugID).Exec(ctx)
		}
	}

	// Ensure the games row exists (FK on external_games.resolved_igdb_id).
	_, _ = h.db.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now()) ON CONFLICT (id) DO NOTHING`,
		body.IGDBID, eg.Title,
	).Exec(ctx)

	// Update external_game.
	_, _ = h.db.NewRaw(
		`UPDATE external_games SET resolved_igdb_id = ?, is_skipped = false, updated_at = now() WHERE id = ?`,
		body.IGDBID, id,
	).Exec(ctx)

	// Create a mini-job and job_item, then enqueue ProcessSyncItem.
	jobID := uuid.NewString()
	now := time.Now().UTC()
	_, _ = h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'normal', 1, ?)`,
		jobID, userID, eg.Storefront, now,
	).Exec(ctx)

	meta, _ := json.Marshal(map[string]string{
		"external_game_id": eg.ID,
		"raw_platform":     eg.RawPlatform,
	})
	itemID := uuid.NewString()
	_, _ = h.db.NewRaw(
		`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
		itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
	).Exec(ctx)

	if h.riverClient != nil {
		_, _ = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil)
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run the rematch tests**

```bash
go test ./internal/api/... -run TestRematchExternalGame -v
```

Expected: all `TestRematchExternalGame_*` tests pass.

- [ ] **Step 5: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): add POST /external-games/:id/rematch endpoint"
```

---

## Task 5: Backend — extend `HandleUnskipGame` to enqueue re-processing

**Files:**
- Modify: `internal/api/sync.go`
- Modify: `internal/api/sync_test.go`

- [ ] **Step 1: Write the failing test**

Add at the bottom of `internal/api/sync_test.go`:

```go
// ─── TestUnskipGame_EnqueuesJobItem ───────────────────────────────────────────

func TestUnskipGame_EnqueuesJobItem(t *testing.T) {
	truncateAllTables(t)
	e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
	userID, token := setupTagUser(t, testDB, e, "unskip-enqueue")
	insertExternalGame(t, testDB, "eg-unskip", userID, "steam", "42", "Half-Life 3")

	// Skip first
	postJSONAuth(t, e, "/api/sync/ignored/eg-unskip", nil, token)

	// Unskip
	rec := deleteAuth(t, e, "/api/sync/ignored/eg-unskip", token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	// is_skipped should be false
	var isSkipped bool
	_ = testDB.NewRaw(`SELECT is_skipped FROM external_games WHERE id = 'eg-unskip'`).
		Scan(context.Background(), &isSkipped)
	if isSkipped {
		t.Fatal("expected is_skipped=false after unskip")
	}

	// A job and job_item should exist
	var jobCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND job_type = 'sync' AND source = 'steam'`, userID,
	).Scan(context.Background(), &jobCount)
	if jobCount != 1 {
		t.Fatalf("expected 1 sync job created by unskip, got %d", jobCount)
	}

	var itemCount int
	_ = testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE item_key = '42' AND user_id = ?`, userID,
	).Scan(context.Background(), &itemCount)
	if itemCount != 1 {
		t.Fatalf("expected 1 job_item with item_key='42', got %d", itemCount)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
go test ./internal/api/... -run TestUnskipGame_EnqueuesJobItem -v
```

Expected: FAIL (no job/job_item created yet).

- [ ] **Step 3: Update `HandleUnskipGame` in `sync.go`**

Replace the existing `HandleUnskipGame` with:

```go
func (h *SyncHandler) HandleUnskipGame(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	id := c.Param("id")
	ctx := context.Background()

	var eg models.ExternalGame
	err := h.db.NewSelect().Model(&eg).Where("id = ? AND user_id = ?", id, userID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to find game")
	}

	_, _ = h.db.NewRaw(
		`UPDATE external_games SET is_skipped = false, updated_at = now() WHERE id = ?`, id,
	).Exec(ctx)

	// Enqueue immediate re-processing. Failure here is non-fatal — the game
	// will be picked up on the next full sync.
	jobID := uuid.NewString()
	now := time.Now().UTC()
	_, jerr := h.db.NewRaw(
		`INSERT INTO jobs (id, user_id, job_type, source, status, priority, total_items, created_at)
		 VALUES (?, ?, 'sync', ?, 'processing', 'normal', 1, ?)`,
		jobID, userID, eg.Storefront, now,
	).Exec(ctx)
	if jerr == nil {
		meta, _ := json.Marshal(map[string]string{
			"external_game_id": eg.ID,
			"raw_platform":     eg.RawPlatform,
		})
		itemID := uuid.NewString()
		_, _ = h.db.NewRaw(
			`INSERT INTO job_items (id, job_id, user_id, item_key, source_title, source_metadata, status, result, igdb_candidates, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', '[]', now())`,
			itemID, jobID, userID, eg.ExternalID, eg.Title, string(meta),
		).Exec(ctx)
		if h.riverClient != nil {
			_, _ = h.riverClient.Insert(ctx, tasks.ProcessSyncItemArgs{JobItemID: itemID}, nil)
		}
	}

	return c.NoContent(http.StatusNoContent)
}
```

- [ ] **Step 4: Run the new and existing unskip tests**

```bash
go test ./internal/api/... -run "TestUnskipGame|TestIgnored" -v
```

Expected: all pass.

- [ ] **Step 5: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat(sync): unskip enqueues immediate ProcessSyncItem River job"
```

---

## Task 6: Frontend — types and API functions

**Files:**
- Modify: `ui/frontend/src/types/sync.ts`
- Modify: `ui/frontend/src/api/sync.ts`
- Modify: `ui/frontend/src/api/index.ts`

- [ ] **Step 1: Add `ExternalGame` interface to `types/sync.ts`**

Add after the existing interfaces in `ui/frontend/src/types/sync.ts`:

```typescript
export interface ExternalGame {
  id: string;
  storefront: string;
  external_id: string;
  title: string;
  resolved_igdb_id: number | null;
  is_skipped: boolean;
  is_available: boolean;
  is_subscription: boolean;
  playtime_hours: number;
  has_user_game: boolean;
  user_game_id: string | null;
  igdb_title: string | null;
  user_game_other_platform_count: number;
}
```

- [ ] **Step 2: Add API functions to `api/sync.ts`**

Add these four functions at the end of `ui/frontend/src/api/sync.ts` (before or after the PSN block — keep it grouped logically):

```typescript
// ============================================================================
// External Games
// ============================================================================

export async function getExternalGames(platform: SyncPlatform): Promise<ExternalGame[]> {
  const response = await apiRequest<ExternalGame[]>(`/api/sync/${platform}/external-games`);
  return response;
}

export async function skipExternalGame(id: string): Promise<void> {
  await apiRequest(`/api/sync/ignored/${id}`, { method: 'POST' });
}

export async function unskipExternalGame(id: string): Promise<void> {
  await apiRequest(`/api/sync/ignored/${id}`, { method: 'DELETE' });
}

export async function rematchExternalGame(
  id: string,
  igdbId: number,
  orphanAction?: 'keep' | 'remove',
): Promise<void> {
  await apiRequest(`/api/sync/external-games/${id}/rematch`, {
    method: 'POST',
    body: JSON.stringify({ igdb_id: igdbId, orphan_action: orphanAction ?? '' }),
  });
}
```

You will need to add `ExternalGame` to the import from `@/types` at the top of the file.

- [ ] **Step 3: Export from `api/index.ts`**

Find the `export { ... } from './sync'` block in `ui/frontend/src/api/index.ts` and add the four new functions:

```typescript
export {
  // ... existing exports ...
  getExternalGames,
  skipExternalGame,
  unskipExternalGame,
  rematchExternalGame,
} from './sync';
```

- [ ] **Step 4: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/sync.ts ui/frontend/src/api/sync.ts ui/frontend/src/api/index.ts
git commit -m "feat(frontend): add ExternalGame type and API functions"
```

---

## Task 7: Frontend — hooks

**Files:**
- Modify: `ui/frontend/src/hooks/use-sync.ts`

- [ ] **Step 1: Add query key and four hooks to `use-sync.ts`**

Add `externalGames` to `syncKeys`:

```typescript
const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncPlatform) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncPlatform) => [...syncKeys.statuses(), platform] as const,
  externalGames: (platform: SyncPlatform) => [...syncKeys.all, 'external-games', platform] as const,
  // keep any other existing keys
};
```

Add the four hooks at the end of the file (before the closing of the module or after the PSN hooks):

```typescript
export function useExternalGames(platform: SyncPlatform) {
  return useQuery({
    queryKey: syncKeys.externalGames(platform),
    queryFn: () => getExternalGames(platform),
  });
}

export function useSkipExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => skipExternalGame(id),
    onSuccess: (_data, _vars, _ctx) => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to skip game');
    },
  });
}

export function useUnskipExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => unskipExternalGame(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to unskip game');
    },
  });
}

export function useRematchExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      igdbId,
      orphanAction,
    }: {
      id: string;
      igdbId: number;
      orphanAction?: 'keep' | 'remove';
    }) => rematchExternalGame(id, igdbId, orphanAction),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to rematch game');
    },
  });
}
```

Add necessary imports at the top of the file (`getExternalGames`, `skipExternalGame`, `unskipExternalGame`, `rematchExternalGame` from `@/api`; `ExternalGame` from `@/types`; `toast` from `sonner` if not already imported).

- [ ] **Step 2: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/hooks/use-sync.ts
git commit -m "feat(frontend): add useExternalGames and related mutations"
```

---

## Task 8: Frontend — `IGDBMatchDialog` component

Wraps the existing `IGDBSearch` component in a shadcn Dialog so it can be triggered from the external games section.

**Files:**
- Create: `ui/frontend/src/components/sync/igdb-match-dialog.tsx`
- Modify: `ui/frontend/src/components/sync/index.ts`

- [ ] **Step 1: Create `igdb-match-dialog.tsx`**

`ui/frontend/src/components/sync/igdb-match-dialog.tsx`:

```tsx
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IGDBSearch } from '@/components/games/igdb-search';
import type { IGDBGameCandidate } from '@/types';

interface IGDBMatchDialogProps {
  open: boolean;
  title?: string;
  onClose: () => void;
  onSelect: (candidate: IGDBGameCandidate) => void;
}

export function IGDBMatchDialog({
  open,
  title = 'Find IGDB Match',
  onClose,
  onSelect,
}: IGDBMatchDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(o) => { if (!o) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <IGDBSearch onSelect={onSelect} autoFocus />
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Export from `components/sync/index.ts`**

Add to `ui/frontend/src/components/sync/index.ts`:

```typescript
export { IGDBMatchDialog } from './igdb-match-dialog';
```

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/sync/igdb-match-dialog.tsx \
        ui/frontend/src/components/sync/index.ts
git commit -m "feat(frontend): add IGDBMatchDialog component"
```

---

## Task 9: Frontend — `ExternalGamesSection` component

Three collapsible subsections (Unmatched / Skipped / Matched) with inline actions and dialogs for IGDB match and orphan warning.

**Files:**
- Create: `ui/frontend/src/components/sync/external-games-section.tsx`
- Modify: `ui/frontend/src/components/sync/index.ts`

- [ ] **Step 1: Create `external-games-section.tsx`**

`ui/frontend/src/components/sync/external-games-section.tsx`:

```tsx
import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { IGDBMatchDialog } from './igdb-match-dialog';
import {
  useExternalGames,
  useSkipExternalGame,
  useUnskipExternalGame,
  useRematchExternalGame,
} from '@/hooks/use-sync';
import type { ExternalGame, IGDBGameCandidate, SyncPlatform } from '@/types';

interface ExternalGamesSectionProps {
  platform: SyncPlatform;
}

interface PendingRematch {
  game: ExternalGame;
  candidate: IGDBGameCandidate;
}

export function ExternalGamesSection({ platform }: ExternalGamesSectionProps) {
  const { data: games = [], isLoading } = useExternalGames(platform);
  const { mutate: skip, isPending: isSkipping } = useSkipExternalGame();
  const { mutate: unskip, isPending: isUnskipping } = useUnskipExternalGame();
  const { mutate: rematch, isPending: isRematching } = useRematchExternalGame();

  const [matchingGame, setMatchingGame] = useState<ExternalGame | null>(null);
  const [pendingRematch, setPendingRematch] = useState<PendingRematch | null>(null);

  if (isLoading || games.length === 0) return null;

  const unmatched = games.filter((g) => g.resolved_igdb_id === null && !g.is_skipped);
  const skipped = games.filter((g) => g.is_skipped);
  const matched = games.filter((g) => g.resolved_igdb_id !== null && !g.is_skipped);

  function handleSelect(game: ExternalGame, candidate: IGDBGameCandidate) {
    setMatchingGame(null);
    const wouldOrphan = game.has_user_game && game.user_game_other_platform_count === 0;
    if (wouldOrphan) {
      setPendingRematch({ game, candidate });
    } else {
      rematch({ id: game.id, igdbId: candidate.igdb_id });
    }
  }

  return (
    <>
      <div className="space-y-4">
        {unmatched.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Unmatched ({unmatched.length})</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {unmatched.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>{game.title}</TableCell>
                      <TableCell className="text-right space-x-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setMatchingGame(game)}
                          disabled={isRematching}
                        >
                          Find Match
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => skip(game.id)}
                          disabled={isSkipping}
                        >
                          Skip
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {skipped.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Skipped ({skipped.length})</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableBody>
                  {skipped.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>{game.title}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => unskip(game.id)}
                          disabled={isUnskipping}
                        >
                          Unskip
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}

        {matched.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Matched ({matched.length})</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Storefront Title</TableHead>
                    <TableHead>IGDB Title</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {matched.map((game) => (
                    <TableRow key={game.id}>
                      <TableCell>{game.title}</TableCell>
                      <TableCell className="text-muted-foreground">{game.igdb_title}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setMatchingGame(game)}
                          disabled={isRematching}
                        >
                          Change Match
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        )}
      </div>

      {matchingGame && (
        <IGDBMatchDialog
          open
          title={`Match "${matchingGame.title}"`}
          onClose={() => setMatchingGame(null)}
          onSelect={(candidate) => handleSelect(matchingGame, candidate)}
        />
      )}

      {pendingRematch && (
        <AlertDialog open onOpenChange={(o) => { if (!o) setPendingRematch(null); }}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Storefront link will be removed</AlertDialogTitle>
              <AlertDialogDescription>
                This game's only storefront link will be removed when rematching. Do you want to
                keep it in your library (unlinked) or remove it from your collection entirely?
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel onClick={() => setPendingRematch(null)}>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="outline"
                onClick={() => {
                  rematch({ id: pendingRematch.game.id, igdbId: pendingRematch.candidate.igdb_id, orphanAction: 'keep' });
                  setPendingRematch(null);
                }}
              >
                Keep in Library
              </AlertDialogAction>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  rematch({ id: pendingRematch.game.id, igdbId: pendingRematch.candidate.igdb_id, orphanAction: 'remove' });
                  setPendingRematch(null);
                }}
              >
                Remove from Collection
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </>
  );
}
```

- [ ] **Step 2: Export from `components/sync/index.ts`**

Add to `ui/frontend/src/components/sync/index.ts`:

```typescript
export { ExternalGamesSection } from './external-games-section';
```

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors. Fix any import issues (shadcn component paths, type imports).

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/sync/external-games-section.tsx \
        ui/frontend/src/components/sync/index.ts
git commit -m "feat(frontend): add ExternalGamesSection component"
```

---

## Task 10: Frontend — mount `ExternalGamesSection` on the sync detail page

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$platform.tsx`

- [ ] **Step 1: Import `ExternalGamesSection`**

In `ui/frontend/src/routes/_authenticated/sync/$platform.tsx`, add to the existing imports:

```tsx
import { ExternalGamesSection } from '@/components/sync';
```

- [ ] **Step 2: Mount the section below the Configuration card**

Find the `{/* Recent Sync Activity */}` comment at the bottom of the JSX return and add `ExternalGamesSection` **above** it (between Configuration and Recent Activity):

```tsx
      {/* External Games Library */}
      <ExternalGamesSection platform={platform} />

      {/* Recent Sync Activity */}
      <RecentActivity platform={platform} />
```

- [ ] **Step 3: Type-check**

```bash
cd ui/frontend && npm run check
```

Expected: no errors.

- [ ] **Step 4: Build the frontend**

```bash
make frontend
```

Expected: builds without errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/\$platform.tsx
git commit -m "feat(frontend): mount ExternalGamesSection on sync detail page"
```

---

## Task 11: Update Slumber collection

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add the three new endpoints to `slumber.yaml`**

Find the `sync/` folder in `slumber.yaml` and add three new requests (maintain alphabetical order within the folder):

```yaml
external_games_list:
  method: GET
  url: "{{base_url}}/api/sync/steam/external-games"
  authentication:
    type: bearer
    token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

external_game_skip:
  method: POST
  url: "{{base_url}}/api/sync/ignored/{{external_game_id}}"
  authentication:
    type: bearer
    token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

external_game_unskip:
  method: DELETE
  url: "{{base_url}}/api/sync/ignored/{{external_game_id}}"
  authentication:
    type: bearer
    token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

external_game_rematch:
  method: POST
  url: "{{base_url}}/api/sync/external-games/{{external_game_id}}/rematch"
  body:
    type: json
    content: |
      {
        "igdb_id": 1234,
        "orphan_action": "keep"
      }
  authentication:
    type: bearer
    token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
```

- [ ] **Step 2: Verify the collection loads**

```bash
slumber collection
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add slumber.yaml
git commit -m "chore(slumber): add external games endpoints to API collection"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| GET /api/sync/:storefront/external-games with join data | Task 3 |
| POST /api/sync/external-games/:id/rematch with orphan handling | Task 4 |
| HandleUnskipGame enqueues immediate ProcessSyncItem | Task 5 |
| raw_platform persisted so unskip/rematch job_items work | Tasks 1–2 |
| ExternalGame type + API functions | Task 6 |
| useExternalGames / useSkipExternalGame / useUnskipExternalGame / useRematchExternalGame | Task 7 |
| IGDBMatchDialog (wraps existing IGDBSearch) | Task 8 |
| ExternalGamesSection with Unmatched / Skipped / Matched sections | Task 9 |
| Orphan warning dialog with Keep / Remove choice | Task 9 |
| Mounted below Configuration on $platform.tsx | Task 10 |
| Slumber collection entries | Task 11 |
| Tests: list (empty, invalid storefront, isolation, all states) | Task 3 |
| Tests: rematch (404, other user, orphan 409, keep, remove) | Task 4 |
| Tests: unskip enqueues job+item | Task 5 |
| Section only renders when external games exist | Task 9 (isLoading guard) |
| Echo v5 route ordering (static before param) | Task 3 RegisterRoutes |
