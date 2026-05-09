# User-Games Utility Endpoints Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 4 read-only GET endpoints (`/ids`, `/genres`, `/filter-options`, `/stats`) to the existing `UserGamesHandler` for collection management UI.

**Architecture:** All 4 handlers are methods on the existing `UserGamesHandler` struct. The `/ids` endpoint reuses the `filter.FilterBuilder` for WHERE/JOIN clauses. The other 3 are standalone queries. Routes must be registered before `/:id` in router.go to avoid Echo treating the path segments as ID params.

**Tech Stack:** Go, Echo v5, Bun ORM, PostgreSQL, testcontainers-go

---

## File Structure

| File | Changes |
|------|---------|
| `internal/api/user_games.go` | 4 new handler methods + 4 response types |
| `internal/api/user_games_test.go` | 4 new test functions with subtests |
| `internal/api/router.go` | 4 new route registrations (before `/:id`) |
| `slumber.yaml` | 4 new request entries in the `user_games` folder |

---

### Task 1: Route Registration

**Files:**
- Modify: `internal/api/router.go:198-203`

Register the 4 new routes before the `/:id` route so Echo doesn't interpret "ids", "genres", etc. as an `:id` parameter value.

- [ ] **Step 1: Add the 4 route registrations**

In `internal/api/router.go`, find this block (around line 198):

```go
		userGamesGroup.GET("", ugh.HandleListUserGames)
		userGamesGroup.POST("", ugh.HandleCreateUserGame)
		userGamesGroup.PUT("/bulk-update", ugh.HandleBulkUpdate)
		userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
		userGamesGroup.POST("/bulk-add-platforms", ugh.HandleBulkAddPlatforms)
		userGamesGroup.DELETE("/bulk-remove-platforms", ugh.HandleBulkRemovePlatforms)
		userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
```

Insert the 4 new routes after the bulk operations and **before** `/:id`:

```go
		userGamesGroup.GET("", ugh.HandleListUserGames)
		userGamesGroup.POST("", ugh.HandleCreateUserGame)
		userGamesGroup.PUT("/bulk-update", ugh.HandleBulkUpdate)
		userGamesGroup.DELETE("/bulk-delete", ugh.HandleBulkDelete)
		userGamesGroup.POST("/bulk-add-platforms", ugh.HandleBulkAddPlatforms)
		userGamesGroup.DELETE("/bulk-remove-platforms", ugh.HandleBulkRemovePlatforms)
		userGamesGroup.GET("/ids", ugh.HandleListUserGameIDs)
		userGamesGroup.GET("/genres", ugh.HandleListGenres)
		userGamesGroup.GET("/filter-options", ugh.HandleFilterOptions)
		userGamesGroup.GET("/stats", ugh.HandleCollectionStats)
		userGamesGroup.GET("/:id", ugh.HandleGetUserGame)
```

- [ ] **Step 2: Commit**

```bash
git add internal/api/router.go
git commit -m "feat(user-games): register /ids, /genres, /filter-options, /stats routes"
```

> Note: The build will fail until the handler methods exist. That's expected — we'll implement them in the next tasks using TDD.

---

### Task 2: `GET /api/user-games/ids` — List Matching IDs

**Files:**
- Modify: `internal/api/user_games.go` (add response type + handler)
- Modify: `internal/api/user_games_test.go` (add `TestListUserGameIDs`)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/user_games_test.go`:

```go
func TestListUserGameIDs(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "ids")
	g1 := insertTestGame(t, db, "IDs Alpha")
	g2 := insertTestGame(t, db, "IDs Beta")
	g3 := insertTestGame(t, db, "IDs Gamma")
	insertTestUserGame(t, db, "ug-ids-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-ids-2", userID, int(g2))
	insertTestUserGame(t, db, "ug-ids-3", userID, int(g3))

	// Set play_status on one game for filter test.
	_, err := db.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'completed' WHERE id = ?`, "ug-ids-1")
	if err != nil {
		t.Fatalf("update play_status: %v", err)
	}

	t.Run("basic", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ids", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.IDs) != 3 {
			t.Fatalf("expected 3 ids, got %d", len(resp.IDs))
		}
	})

	t.Run("with filter", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/ids?play_status=completed", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.IDs) != 1 {
			t.Fatalf("expected 1 id, got %d", len(resp.IDs))
		}
		if resp.IDs[0] != "ug-ids-1" {
			t.Fatalf("expected ug-ids-1, got %s", resp.IDs[0])
		}
	})

	t.Run("user scoped", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "ids-other")
		rec := getAuth(t, e, "/api/user-games/ids", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.IDs) != 0 {
			t.Fatalf("expected 0 ids, got %d", len(resp.IDs))
		}
	})

	t.Run("empty collection", func(t *testing.T) {
		_, token3 := setupUserGamesUser(t, db, e, "ids-empty")
		rec := getAuth(t, e, "/api/user-games/ids", token3)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			IDs []string `json:"ids"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.IDs) != 0 {
			t.Fatalf("expected 0 ids, got %d", len(resp.IDs))
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestListUserGameIDs -v`
Expected: Compilation error — `HandleListUserGameIDs` not defined.

- [ ] **Step 3: Add the response type and handler**

Add to `internal/api/user_games.go`, after the existing `UserGameListResponse` type (around line 55):

```go
// UserGameIDsResponse is the response for GET /api/user-games/ids.
type UserGameIDsResponse struct {
	IDs []string `json:"ids"`
}
```

Then add the handler method. This reuses the same filter-building logic as `HandleListUserGames` but selects only `DISTINCT ug.id` with no sort/pagination:

```go
// HandleListUserGameIDs handles GET /api/user-games/ids.
func (h *UserGamesHandler) HandleListUserGameIDs(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	// Build filter — same logic as HandleListUserGames.
	fb := filter.NewFilterBuilder()
	filter.ApplyPlayStatus(fb, c.QueryParam("play_status"))
	filter.ApplyOwnershipStatus(fb, c.QueryParam("ownership_status"))
	filter.ApplySearch(fb, c.QueryParam("q"))

	if str := c.QueryParam("is_loved"); str != "" {
		v := str == "true"
		filter.ApplyIsLoved(fb, &v)
	}
	if str := c.QueryParam("has_notes"); str != "" {
		v := str == "true"
		filter.ApplyHasNotes(fb, &v)
	}
	if str := c.QueryParam("rating_min"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			filter.ApplyRatingMin(fb, &v)
		}
	}
	if str := c.QueryParam("rating_max"); str != "" {
		if v, err := strconv.ParseFloat(str, 64); err == nil {
			filter.ApplyRatingMax(fb, &v)
		}
	}
	filter.ApplyPlatform(fb, c.QueryParams()["platform"])
	filter.ApplyStorefront(fb, c.QueryParams()["storefront"])
	filter.ApplyGenre(fb, c.QueryParams()["genre"])
	filter.ApplyGameMode(fb, c.QueryParams()["game_mode"])
	filter.ApplyTheme(fb, c.QueryParams()["theme"])
	filter.ApplyPlayerPerspective(fb, c.QueryParams()["player_perspective"])
	filter.ApplyTag(fb, c.QueryParams()["tag"])

	ctx := context.Background()

	var ids []string
	q := h.db.NewSelect().
		TableExpr("user_games AS ug").
		ColumnExpr("DISTINCT ug.id").
		Where("ug.user_id = ?", userID)
	q = fb.Apply(q)

	if err := q.Scan(ctx, &ids); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	if ids == nil {
		ids = []string{}
	}

	return c.JSON(http.StatusOK, UserGameIDsResponse{IDs: ids})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestListUserGameIDs -v`
Expected: All 4 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat(user-games): add GET /ids endpoint for bulk selection"
```

---

### Task 3: `GET /api/user-games/genres` — List User's Genres

**Files:**
- Modify: `internal/api/user_games.go` (add response type + handler)
- Modify: `internal/api/user_games_test.go` (add `TestListGenres`)

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/user_games_test.go`. This needs a helper to insert games with genre data:

```go
func insertTestGameWithGenre(t *testing.T, db *bun.DB, title, genre string) int32 {
	t.Helper()
	id := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET genre = ? WHERE id = ?`, genre, id)
	if err != nil {
		t.Fatalf("update genre: %v", err)
	}
	return id
}

func TestListGenres(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "genres")

	g1 := insertTestGameWithGenre(t, db, "Genre Alpha", "Action, RPG")
	g2 := insertTestGameWithGenre(t, db, "Genre Beta", "RPG, Simulation")
	g3 := insertTestGame(t, db, "Genre Gamma") // null genre
	insertTestUserGame(t, db, "ug-genres-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-genres-2", userID, int(g2))
	insertTestUserGame(t, db, "ug-genres-3", userID, int(g3))

	t.Run("basic", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/genres", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Genres []string `json:"genres"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// Should be: Action, RPG, Simulation (sorted, deduplicated)
		expected := []string{"Action", "RPG", "Simulation"}
		if len(resp.Genres) != len(expected) {
			t.Fatalf("expected %d genres, got %d: %v", len(expected), len(resp.Genres), resp.Genres)
		}
		for i, g := range expected {
			if resp.Genres[i] != g {
				t.Fatalf("expected genre[%d]=%s, got %s", i, g, resp.Genres[i])
			}
		}
	})

	t.Run("comma separation", func(t *testing.T) {
		// Already tested above — "Action, RPG" produces both entries
		rec := getAuth(t, e, "/api/user-games/genres", token)
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		found := map[string]bool{}
		for _, g := range resp.Genres {
			found[g] = true
		}
		if !found["Action"] || !found["RPG"] {
			t.Fatalf("expected both Action and RPG from comma-separated genre, got %v", resp.Genres)
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "genres-empty")
		rec := getAuth(t, e, "/api/user-games/genres", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.Genres) != 0 {
			t.Fatalf("expected 0 genres, got %d", len(resp.Genres))
		}
	})

	t.Run("null genres excluded", func(t *testing.T) {
		// Game g3 has null genre — should not contribute any entries
		rec := getAuth(t, e, "/api/user-games/genres", token)
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		for _, g := range resp.Genres {
			if g == "" {
				t.Fatal("empty string genre should not appear")
			}
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestListGenres -v`
Expected: Compilation error — `HandleListGenres` not defined.

- [ ] **Step 3: Add the response type and handler**

Add to `internal/api/user_games.go`:

```go
// GenresResponse is the response for GET /api/user-games/genres.
type GenresResponse struct {
	Genres []string `json:"genres"`
}

// HandleListGenres handles GET /api/user-games/genres.
func (h *UserGamesHandler) HandleListGenres(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	var rawGenres []string
	err := h.db.NewSelect().
		TableExpr("games AS g").
		ColumnExpr("g.genre").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		Where("ug.user_id = ?", userID).
		Where("g.genre IS NOT NULL").
		Scan(ctx, &rawGenres)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Split comma-separated genres, deduplicate, sort.
	genreSet := make(map[string]bool)
	for _, raw := range rawGenres {
		for _, g := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(g)
			if trimmed != "" {
				genreSet[trimmed] = true
			}
		}
	}

	genres := make([]string, 0, len(genreSet))
	for g := range genreSet {
		genres = append(genres, g)
	}
	sort.Strings(genres)

	return c.JSON(http.StatusOK, GenresResponse{Genres: genres})
}
```

You'll need to add `"sort"` to the import block at the top of `user_games.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestListGenres -v`
Expected: All 4 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat(user-games): add GET /genres endpoint for filter dropdown"
```

---

### Task 4: `GET /api/user-games/filter-options` — All Filter Dropdown Data

**Files:**
- Modify: `internal/api/user_games.go` (add response type + handler)
- Modify: `internal/api/user_games_test.go` (add `TestFilterOptions`)

- [ ] **Step 1: Write the failing tests**

Add a helper for inserting games with all metadata fields, then the test:

```go
func insertTestGameWithMetadata(t *testing.T, db *bun.DB, title, genre, gameModes, themes, playerPerspectives string) int32 {
	t.Helper()
	id := insertTestGame(t, db, title)
	_, err := db.ExecContext(context.Background(),
		`UPDATE games SET genre = ?, game_modes = ?, themes = ?, player_perspectives = ? WHERE id = ?`,
		genre, gameModes, themes, playerPerspectives, id)
	if err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	return id
}

func TestFilterOptions(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "filteropts")

	g1 := insertTestGameWithMetadata(t, db, "FO Alpha", "Action, RPG", "Single player", "Horror", "First person")
	g2 := insertTestGameWithMetadata(t, db, "FO Beta", "RPG, Simulation", "Multiplayer", "Sci-fi", "Third person")
	insertTestUserGame(t, db, "ug-fo-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-fo-2", userID, int(g2))

	t.Run("basic", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/filter-options", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Genres             []string `json:"genres"`
			GameModes          []string `json:"game_modes"`
			Themes             []string `json:"themes"`
			PlayerPerspectives []string `json:"player_perspectives"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resp.Genres) != 3 { // Action, RPG, Simulation
			t.Fatalf("expected 3 genres, got %d: %v", len(resp.Genres), resp.Genres)
		}
		if len(resp.GameModes) != 2 {
			t.Fatalf("expected 2 game_modes, got %d: %v", len(resp.GameModes), resp.GameModes)
		}
		if len(resp.Themes) != 2 {
			t.Fatalf("expected 2 themes, got %d: %v", len(resp.Themes), resp.Themes)
		}
		if len(resp.PlayerPerspectives) != 2 {
			t.Fatalf("expected 2 player_perspectives, got %d: %v", len(resp.PlayerPerspectives), resp.PlayerPerspectives)
		}
		// Verify alphabetical sort.
		if resp.Genres[0] != "Action" {
			t.Fatalf("expected first genre=Action, got %s", resp.Genres[0])
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, token2 := setupUserGamesUser(t, db, e, "filteropts-empty")
		rec := getAuth(t, e, "/api/user-games/filter-options", token2)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var resp struct {
			Genres             []string `json:"genres"`
			GameModes          []string `json:"game_modes"`
			Themes             []string `json:"themes"`
			PlayerPerspectives []string `json:"player_perspectives"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(resp.Genres) != 0 || len(resp.GameModes) != 0 || len(resp.Themes) != 0 || len(resp.PlayerPerspectives) != 0 {
			t.Fatalf("expected all empty arrays, got genres=%d game_modes=%d themes=%d perspectives=%d",
				len(resp.Genres), len(resp.GameModes), len(resp.Themes), len(resp.PlayerPerspectives))
		}
	})

	t.Run("deduplication", func(t *testing.T) {
		// Both g1 and g2 have "RPG" — it should appear only once
		rec := getAuth(t, e, "/api/user-games/filter-options", token)
		var resp struct {
			Genres []string `json:"genres"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		count := 0
		for _, g := range resp.Genres {
			if g == "RPG" {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected RPG to appear once, appeared %d times", count)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestFilterOptions -v`
Expected: Compilation error — `HandleFilterOptions` not defined.

- [ ] **Step 3: Add the response type and handler**

Add to `internal/api/user_games.go`:

```go
// FilterOptionsResponse is the response for GET /api/user-games/filter-options.
type FilterOptionsResponse struct {
	Genres             []string `json:"genres"`
	GameModes          []string `json:"game_modes"`
	Themes             []string `json:"themes"`
	PlayerPerspectives []string `json:"player_perspectives"`
}

// HandleFilterOptions handles GET /api/user-games/filter-options.
func (h *UserGamesHandler) HandleFilterOptions(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	type row struct {
		Genre              *string `bun:"genre"`
		GameModes          *string `bun:"game_modes"`
		Themes             *string `bun:"themes"`
		PlayerPerspectives *string `bun:"player_perspectives"`
	}

	var rows []row
	err := h.db.NewSelect().
		TableExpr("games AS g").
		ColumnExpr("g.genre, g.game_modes, g.themes, g.player_perspectives").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		Where("ug.user_id = ?", userID).
		Scan(ctx, &rows)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	genreSet := make(map[string]bool)
	modeSet := make(map[string]bool)
	themeSet := make(map[string]bool)
	perspectiveSet := make(map[string]bool)

	splitAndCollect := func(val *string, set map[string]bool) {
		if val == nil {
			return
		}
		for _, s := range strings.Split(*val, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				set[trimmed] = true
			}
		}
	}

	for _, r := range rows {
		splitAndCollect(r.Genre, genreSet)
		splitAndCollect(r.GameModes, modeSet)
		splitAndCollect(r.Themes, themeSet)
		splitAndCollect(r.PlayerPerspectives, perspectiveSet)
	}

	sortedKeys := func(set map[string]bool) []string {
		keys := make([]string, 0, len(set))
		for k := range set {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}

	return c.JSON(http.StatusOK, FilterOptionsResponse{
		Genres:             sortedKeys(genreSet),
		GameModes:          sortedKeys(modeSet),
		Themes:             sortedKeys(themeSet),
		PlayerPerspectives: sortedKeys(perspectiveSet),
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestFilterOptions -v`
Expected: All 3 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat(user-games): add GET /filter-options endpoint for sidebar dropdowns"
```

---

### Task 5: `GET /api/user-games/stats` — Collection Statistics

**Files:**
- Modify: `internal/api/user_games.go` (add response types + handler)
- Modify: `internal/api/user_games_test.go` (add `TestCollectionStats`)

This is the most complex handler — it runs multiple targeted queries and assembles the response.

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/user_games_test.go`:

```go
func TestCollectionStats(t *testing.T) {
	db := setupAuthTestDB(t)
	cfg := testCfg()
	e := newTestEcho(t, db, cfg)
	userID, token := setupUserGamesUser(t, db, e, "stats")

	t.Run("empty collection", func(t *testing.T) {
		_, emptyToken := setupUserGamesUser(t, db, e, "stats-empty")
		rec := getAuth(t, e, "/api/user-games/stats", emptyToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["total_games"].(float64) != 0 {
			t.Fatalf("expected total_games=0, got %v", resp["total_games"])
		}
		if resp["completion_rate"].(float64) != 0 {
			t.Fatalf("expected completion_rate=0, got %v", resp["completion_rate"])
		}
		if resp["average_rating"] != nil {
			t.Fatalf("expected average_rating=null, got %v", resp["average_rating"])
		}
		if resp["total_hours_played"].(float64) != 0 {
			t.Fatalf("expected total_hours_played=0, got %v", resp["total_hours_played"])
		}
		if resp["pile_of_shame"].(float64) != 0 {
			t.Fatalf("expected pile_of_shame=0, got %v", resp["pile_of_shame"])
		}
	})

	// Set up data for the main test.
	g1 := insertTestGameWithGenre(t, db, "Stats Alpha", "Action, RPG")
	g2 := insertTestGameWithGenre(t, db, "Stats Beta", "RPG")
	g3 := insertTestGame(t, db, "Stats Gamma")

	insertTestUserGame(t, db, "ug-stats-1", userID, int(g1))
	insertTestUserGame(t, db, "ug-stats-2", userID, int(g2))
	insertTestUserGame(t, db, "ug-stats-3", userID, int(g3))

	// Set play statuses.
	_, _ = db.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'completed', personal_rating = 4 WHERE id = ?`, "ug-stats-1")
	_, _ = db.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'not_started', personal_rating = 3 WHERE id = ?`, "ug-stats-2")
	_, _ = db.ExecContext(context.Background(),
		`UPDATE user_games SET play_status = 'in_progress' WHERE id = ?`, "ug-stats-3")

	// Add platforms with hours and ownership.
	pc := "pc"
	steam := "steam"
	insertTestUserGamePlatform(t, db, "ugp-stats-1", "ug-stats-1", &pc, &steam)
	_, _ = db.ExecContext(context.Background(),
		`UPDATE user_game_platforms SET hours_played = 50.5, ownership_status = 'owned' WHERE id = ?`, "ugp-stats-1")

	// Add a legacy hours_played on user_game (no platform hours).
	_, _ = db.ExecContext(context.Background(),
		`UPDATE user_games SET hours_played = 10.0 WHERE id = ?`, "ug-stats-3")

	// Insert a platform entry for a known platform.
	_, _ = db.ExecContext(context.Background(),
		`INSERT INTO platforms (name, display_name) VALUES ('pc', 'PC') ON CONFLICT DO NOTHING`)

	t.Run("basic", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/stats", token)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if resp["total_games"].(float64) != 3 {
			t.Fatalf("expected total_games=3, got %v", resp["total_games"])
		}

		completionStats := resp["completion_stats"].(map[string]any)
		if completionStats["completed"].(float64) != 1 {
			t.Fatalf("expected completed=1, got %v", completionStats["completed"])
		}
		if completionStats["not_started"].(float64) != 1 {
			t.Fatalf("expected not_started=1, got %v", completionStats["not_started"])
		}

		if resp["pile_of_shame"].(float64) != 1 {
			t.Fatalf("expected pile_of_shame=1, got %v", resp["pile_of_shame"])
		}

		// completion_rate = (1 completed + 0 mastered + 0 dominated) / 3 * 100 = 33.33
		cr := resp["completion_rate"].(float64)
		if cr < 33.32 || cr > 33.34 {
			t.Fatalf("expected completion_rate ~33.33, got %v", cr)
		}

		// average_rating = (4 + 3) / 2 = 3.5
		ar := resp["average_rating"].(float64)
		if ar != 3.5 {
			t.Fatalf("expected average_rating=3.5, got %v", ar)
		}

		genreStats := resp["genre_stats"].(map[string]any)
		if genreStats["RPG"].(float64) != 2 {
			t.Fatalf("expected RPG=2 in genre_stats, got %v", genreStats["RPG"])
		}
		if genreStats["Action"].(float64) != 1 {
			t.Fatalf("expected Action=1 in genre_stats, got %v", genreStats["Action"])
		}
	})

	t.Run("hours fallback", func(t *testing.T) {
		rec := getAuth(t, e, "/api/user-games/stats", token)
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		// ug-stats-1 has platform hours 50.5, ug-stats-3 has legacy hours 10.0
		// ug-stats-2 has no hours → 0
		// total = 50.5 + 0 + 10.0 = 60.5
		hours := resp["total_hours_played"].(float64)
		if hours != 60.5 {
			t.Fatalf("expected total_hours_played=60.5, got %v", hours)
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/... -run TestCollectionStats -v`
Expected: Compilation error — `HandleCollectionStats` not defined.

- [ ] **Step 3: Add the response types and handler**

Add to `internal/api/user_games.go`:

```go
// CollectionStatsResponse is the response for GET /api/user-games/stats.
type CollectionStatsResponse struct {
	TotalGames       int                `json:"total_games"`
	CompletionStats  map[string]int     `json:"completion_stats"`
	OwnershipStats   map[string]int     `json:"ownership_stats"`
	PlatformStats    map[string]int     `json:"platform_stats"`
	GenreStats       map[string]int     `json:"genre_stats"`
	PileOfShame      int                `json:"pile_of_shame"`
	CompletionRate   float64            `json:"completion_rate"`
	AverageRating    *float64           `json:"average_rating"`
	TotalHoursPlayed float64            `json:"total_hours_played"`
}

// HandleCollectionStats handles GET /api/user-games/stats.
func (h *UserGamesHandler) HandleCollectionStats(c *echo.Context) error {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	ctx := context.Background()

	// 1. total_games
	totalGames, err := h.db.NewSelect().
		TableExpr("user_games").
		Where("user_id = ?", userID).
		Count(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// 2. completion_stats
	completionStats := map[string]int{
		"not_started": 0, "in_progress": 0, "completed": 0, "mastered": 0,
		"dominated": 0, "shelved": 0, "dropped": 0, "replay": 0,
	}
	type statusCount struct {
		PlayStatus string `bun:"play_status"`
		Count      int    `bun:"count"`
	}
	var statusCounts []statusCount
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("play_status, COUNT(*) AS count").
		Where("user_id = ?", userID).
		Where("play_status IS NOT NULL").
		GroupExpr("play_status").
		Scan(ctx, &statusCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, sc := range statusCounts {
		completionStats[sc.PlayStatus] = sc.Count
	}

	// 3. ownership_stats
	ownershipStats := map[string]int{
		"owned": 0, "borrowed": 0, "rented": 0, "subscription": 0, "no_longer_owned": 0,
	}
	var ownerCounts []struct {
		OwnershipStatus string `bun:"ownership_status"`
		Count           int    `bun:"count"`
	}
	err = h.db.NewSelect().
		TableExpr("user_game_platforms AS ugp").
		ColumnExpr("ugp.ownership_status, COUNT(DISTINCT ugp.user_game_id) AS count").
		Join("JOIN user_games AS ug ON ug.id = ugp.user_game_id").
		Where("ug.user_id = ?", userID).
		Where("ugp.ownership_status IS NOT NULL").
		GroupExpr("ugp.ownership_status").
		Scan(ctx, &ownerCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, oc := range ownerCounts {
		ownershipStats[oc.OwnershipStatus] = oc.Count
	}

	// 4. platform_stats
	platformStats := map[string]int{}
	var platCounts []struct {
		DisplayName string `bun:"display_name"`
		Count       int    `bun:"count"`
	}
	err = h.db.NewSelect().
		TableExpr("user_game_platforms AS ugp").
		ColumnExpr("p.display_name, COUNT(*) AS count").
		Join("JOIN platforms AS p ON p.name = ugp.platform").
		Join("JOIN user_games AS ug ON ug.id = ugp.user_game_id").
		Where("ug.user_id = ?", userID).
		GroupExpr("p.name, p.display_name").
		Scan(ctx, &platCounts)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, pc := range platCounts {
		platformStats[pc.DisplayName] = pc.Count
	}

	// 5. genre_stats
	genreStats := map[string]int{}
	var rawGenres []string
	err = h.db.NewSelect().
		TableExpr("games AS g").
		ColumnExpr("g.genre").
		Join("JOIN user_games AS ug ON g.id = ug.game_id").
		Where("ug.user_id = ?", userID).
		Where("g.genre IS NOT NULL").
		Scan(ctx, &rawGenres)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	for _, raw := range rawGenres {
		for _, g := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(g)
			if trimmed != "" {
				genreStats[trimmed]++
			}
		}
	}

	// 6. pile_of_shame
	pileOfShame := completionStats["not_started"]

	// 7. completion_rate
	var completionRate float64
	if totalGames > 0 {
		completed := completionStats["completed"] + completionStats["mastered"] + completionStats["dominated"]
		completionRate = math.Round(float64(completed)/float64(totalGames)*10000) / 100
	}

	// 8. average_rating
	var avgRating *float64
	var avg sql.NullFloat64
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("AVG(personal_rating)").
		Where("user_id = ?", userID).
		Where("personal_rating IS NOT NULL").
		Scan(ctx, &avg)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
	if avg.Valid {
		avgRating = &avg.Float64
	}

	// 9. total_hours_played
	var totalHoursPlayed float64
	type ugHours struct {
		ID          string   `bun:"id"`
		HoursPlayed *float64 `bun:"hours_played"`
	}
	var userGames []ugHours
	err = h.db.NewSelect().
		TableExpr("user_games").
		ColumnExpr("id, hours_played").
		Where("user_id = ?", userID).
		Scan(ctx, &userGames)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}

	// Collect platform hours per user_game.
	ugIDs := make([]string, len(userGames))
	for i, ug := range userGames {
		ugIDs[i] = ug.ID
	}

	platHoursMap := map[string]float64{} // user_game_id → sum of platform hours
	if len(ugIDs) > 0 {
		type platHours struct {
			UserGameID string  `bun:"user_game_id"`
			Total      float64 `bun:"total"`
		}
		var ph []platHours
		err = h.db.NewSelect().
			TableExpr("user_game_platforms").
			ColumnExpr("user_game_id, COALESCE(SUM(hours_played), 0) AS total").
			Where("user_game_id IN (?)", bun.List(ugIDs)).
			GroupExpr("user_game_id").
			Scan(ctx, &ph)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "database error")
		}
		for _, p := range ph {
			platHoursMap[p.UserGameID] = p.Total
		}
	}

	for _, ug := range userGames {
		platSum := platHoursMap[ug.ID]
		if platSum > 0 {
			totalHoursPlayed += platSum
		} else if ug.HoursPlayed != nil {
			totalHoursPlayed += *ug.HoursPlayed
		}
	}

	return c.JSON(http.StatusOK, CollectionStatsResponse{
		TotalGames:       totalGames,
		CompletionStats:  completionStats,
		OwnershipStats:   ownershipStats,
		PlatformStats:    platformStats,
		GenreStats:       genreStats,
		PileOfShame:      pileOfShame,
		CompletionRate:   completionRate,
		AverageRating:    avgRating,
		TotalHoursPlayed: totalHoursPlayed,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/... -run TestCollectionStats -v`
Expected: All 3 subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go internal/api/user_games_test.go
git commit -m "feat(user-games): add GET /stats endpoint for dashboard statistics"
```

---

### Task 6: Full Test Suite + Slumber Collection

**Files:**
- Modify: `slumber.yaml` (add 4 new request entries)

- [ ] **Step 1: Run all tests**

Run: `go test ./internal/api/... -v`
Expected: All tests pass, including the 4 new test functions.

- [ ] **Step 2: Add slumber requests**

Add these 4 entries to the `user_games` section of `slumber.yaml`, after the existing `list` entry and before `get`:

```yaml
      ids:
        name: List User Game IDs
        method: GET
        url: "{{base_url}}/api/user-games/ids"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      genres:
        name: List Genres
        method: GET
        url: "{{base_url}}/api/user-games/genres"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      filter_options:
        name: Filter Options
        method: GET
        url: "{{base_url}}/api/user-games/filter-options"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"

      stats:
        name: Collection Stats
        method: GET
        url: "{{base_url}}/api/user-games/stats"
        authentication:
          type: bearer
          token: "{{response('login', trigger='no_history') | jsonpath('$.access_token')}}"
```

- [ ] **Step 3: Verify slumber collection loads**

Run: `slumber show collection`
Expected: No errors; the 4 new requests appear under `user_games`.

- [ ] **Step 4: Run lint**

Run: `golangci-lint run`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add slumber.yaml
git commit -m "chore: add slumber requests for user-games utility endpoints"
```
