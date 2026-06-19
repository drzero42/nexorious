# `nexctl` Phase 2 — `game` Command Group Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the full `nexctl game` command group (list/show/add/edit/acquire/rm) over the user-games/IGDB REST API.

**Architecture:** Add ~14 `internal/cliclient` methods (the client has none for games) behind a shared `doBearer` JSON helper, then build `cmd/nexctl/game*.go` commands that orchestrate them, reusing Phase 1's `cliui`, `resolveProfile`, `profileName`, `flagBool`. `add`/`edit` are multi-call orchestrations.

**Tech Stack:** Go 1.26, `spf13/cobra`, stdlib `net/http`/`net/url`/`net/http/httptest`, `internal/cliui`/`cliclient`/`clicfg`.

## Global Constraints

- Module `github.com/drzero42/nexorious`.
- Import boundary: `cmd/nexctl` + `cliclient` import only stdlib + cobra + `internal/clicfg`/`cliclient`/`cliui` — NO server/DB packages.
- All `/api/user-games/*`, `/api/games/*`, `/api/tags` endpoints require `Authorization: Bearer <key>`; commands get the key via `resolveProfile(cmd)`.
- errcheck check-blank on non-test code; `_ =`/`ok, _ :=` blank discards must be handled or annotated. `fmt.Fprint*` to `out` is allowlisted. Never blank-discard a `cliui.Confirm` error.
- gosec enabled on non-test code.
- Output conventions: human table/detail default; `--json` via `cliui.EncodeJSON`; `-q` bare ids. Pickers/confirms only on a TTY and never when `--json`/`-q`/`--yes` set.
- Play-status enum: `not_started`, `in_progress`, `completed`, `mastered`, `dominated`, `shelved`, `dropped`, `replay`. Ownership enum: `owned`, `borrowed`, `rented`, `subscription`, `no_longer_owned`.
- TDD: failing test → see it fail → minimal impl → pass → commit. Frequent commits.

---

## File Structure

- `internal/cliclient/client.go` (modify) — add `doBearer` helper + game/IGDB/tag methods + response/request types.
- `internal/cliclient/games_test.go` (create) — httptest-based tests for the new methods.
- `cmd/nexctl/game.go` (create) — `newGameCmd` parent + library ref-resolution helper + shared rendering helpers.
- `cmd/nexctl/game_list.go`, `game_show.go`, `game_add.go`, `game_acquire.go`, `game_rm.go`, `game_edit.go` (create) — one file per subcommand.
- `cmd/nexctl/game_*_test.go` (create) — per-command tests driven through `newRootCmd()` against an httptest mux.
- `cmd/nexctl/main.go` (modify) — register `newGameCmd()`.
- `cmd/nexctl/main_test.go` (modify) — add `"game"` to the `want` map.
- `CLAUDE.md` (modify) — note the `game` group.

---

## Task 1: cliclient — `doBearer` helper + IGDB methods

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/games_test.go`

**Interfaces — Produces:**
- `func (c *Client) doBearer(method, path, key string, body, out any) error`
- `type IGDBCandidate struct { IgdbID int; IgdbSlug, Title, ReleaseDate, CoverArtURL, Description string; Platforms []string; HowLongToBeatMain *float64; UserGameID *string; UserGameIsWishlisted *bool }`
- `type IGDBSearchResponse struct { Games []IGDBCandidate; Total int }`
- `type Game struct { ID int; Title string }`
- `func (c *Client) SearchIGDB(key, query string, limit int) (*IGDBSearchResponse, error)`
- `func (c *Client) GetIGDBGame(key string, igdbID int) (*IGDBSearchResponse, error)`
- `func (c *Client) ImportIGDBGame(key string, igdbID int) (*Game, error)`

- [ ] **Step 1: Write the failing test**

```go
package cliclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchIGDB(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/search/igdb", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth = %q", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["query"] != "hollow" {
			t.Errorf("query = %v", body["query"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{{"igdb_id": 2131, "title": "Hollow Knight", "release_date": "2017-02-24"}},
			"total": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).SearchIGDB("k", "hollow", 10)
	if err != nil {
		t.Fatalf("SearchIGDB: %v", err)
	}
	if res.Total != 1 || len(res.Games) != 1 || res.Games[0].IgdbID != 2131 || res.Games[0].Title != "Hollow Knight" {
		t.Fatalf("res = %+v", res)
	}
}

func TestImportIGDBGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/igdb-import", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 2131, "title": "Hollow Knight"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g, err := New(srv.URL).ImportIGDBGame("k", 2131)
	if err != nil {
		t.Fatalf("ImportIGDBGame: %v", err)
	}
	if g.ID != 2131 || g.Title != "Hollow Knight" {
		t.Fatalf("game = %+v", g)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliclient/... -run 'TestSearchIGDB|TestImportIGDBGame'`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/cliclient/client.go` (the imports `bytes`, `encoding/json`, `fmt`, `io`, `net/http` already exist):

```go
// doBearer performs an authenticated JSON request. A non-nil body is JSON-encoded
// as the request body; on a 2xx response a non-nil out is decoded from the body
// (skipped for 204). Non-2xx responses become an httpError.
func (c *Client) doBearer(method, path, key string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return httpError(resp)
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// IGDBCandidate is one IGDB search hit. UserGameID is non-nil when the game is
// already in the caller's library.
type IGDBCandidate struct {
	IgdbID               int      `json:"igdb_id"`
	IgdbSlug             string   `json:"igdb_slug"`
	Title                string   `json:"title"`
	ReleaseDate          string   `json:"release_date"`
	CoverArtURL          string   `json:"cover_art_url"`
	Description          string   `json:"description"`
	Platforms            []string `json:"platforms"`
	HowLongToBeatMain    *float64 `json:"howlongtobeat_main"`
	UserGameID           *string  `json:"user_game_id"`
	UserGameIsWishlisted *bool    `json:"user_game_is_wishlisted"`
}

// IGDBSearchResponse is the result of an IGDB search or single-game lookup.
type IGDBSearchResponse struct {
	Games []IGDBCandidate `json:"games"`
	Total int             `json:"total"`
}

// Game is the minimal local game record returned by igdb-import.
type Game struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// SearchIGDB searches the IGDB catalog for the given query (limit 1–50).
func (c *Client) SearchIGDB(key, query string, limit int) (*IGDBSearchResponse, error) {
	var out IGDBSearchResponse
	body := map[string]any{"query": query, "limit": limit}
	if err := c.doBearer(http.MethodPost, "/api/games/search/igdb", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetIGDBGame fetches a single IGDB game by id (response Total == 1).
func (c *Client) GetIGDBGame(key string, igdbID int) (*IGDBSearchResponse, error) {
	var out IGDBSearchResponse
	if err := c.doBearer(http.MethodGet, fmt.Sprintf("/api/games/igdb/%d", igdbID), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportIGDBGame imports IGDB metadata for igdbID into the local DB, returning
// the local game record.
func (c *Client) ImportIGDBGame(key string, igdbID int) (*Game, error) {
	var out Game
	body := map[string]any{"igdb_id": igdbID, "download_cover_art": true}
	if err := c.doBearer(http.MethodPost, "/api/games/igdb-import", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliclient/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/
git commit -m "feat: add cliclient IGDB search/import methods"
```

---

## Task 2: cliclient — user-game read methods + types

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/games_test.go`

**Interfaces — Consumes:** `doBearer` (Task 1). **Produces:**
- `type UserGame struct { ID string; GameID int; PlayStatus *string; PersonalRating *int; IsLoved, IsWishlisted bool; PersonalNotes *string; Game *GameRef; HoursPlayed float64; Platforms []UserGamePlatform; Tags []Tag }`
- `type GameRef struct { ID int; Title string }`
- `type UserGamePlatform struct { ID string; Platform, Storefront *string; HoursPlayed *float64; OwnershipStatus *string }`
- `type Tag struct { ID, Name string; Color *string }`
- `type UserGameListResponse struct { UserGames []UserGame; Total, Page, PerPage, Pages int }`
- `func (c *Client) ListUserGames(key string, params url.Values) (*UserGameListResponse, error)`
- `func (c *Client) GetUserGame(key, id string) (*UserGame, error)`

- [ ] **Step 1: Write the failing test**

```go
func TestListUserGames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{
				"id": "ug-1", "play_status": "completed", "hours_played": 12.5,
				"game": map[string]any{"id": 2131, "title": "Hollow Knight"},
				"tags": []map[string]any{{"id": "t1", "name": "Metroidvania"}},
			}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := New(srv.URL).ListUserGames("k", url.Values{"play_status": {"completed"}})
	if err != nil {
		t.Fatalf("ListUserGames: %v", err)
	}
	if res.Total != 1 || res.UserGames[0].Game.Title != "Hollow Knight" || res.UserGames[0].HoursPlayed != 12.5 {
		t.Fatalf("res = %+v", res.UserGames[0])
	}
	if len(res.UserGames[0].Tags) != 1 || res.UserGames[0].Tags[0].Name != "Metroidvania" {
		t.Fatalf("tags = %+v", res.UserGames[0].Tags)
	}
}

func TestGetUserGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/ug-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ug-1", "game": map[string]any{"id": 2131, "title": "Hollow Knight"},
			"platforms": []map[string]any{{"id": "p1", "platform": "pc_windows", "hours_played": 12.5}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ug, err := New(srv.URL).GetUserGame("k", "ug-1")
	if err != nil {
		t.Fatalf("GetUserGame: %v", err)
	}
	if ug.ID != "ug-1" || len(ug.Platforms) != 1 || *ug.Platforms[0].Platform != "pc_windows" {
		t.Fatalf("ug = %+v", ug)
	}
}
```

Add `"net/url"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliclient/... -run 'TestListUserGames|TestGetUserGame'`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

Add `"net/url"` to `client.go` imports, then append:

```go
// GameRef is the minimal game metadata embedded in a user-game (the title source).
type GameRef struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Tag is a user tag.
type Tag struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// UserGamePlatform is one platform row on a user-game.
type UserGamePlatform struct {
	ID              string   `json:"id"`
	Platform        *string  `json:"platform"`
	Storefront      *string  `json:"storefront"`
	HoursPlayed     *float64 `json:"hours_played"`
	OwnershipStatus *string  `json:"ownership_status"`
}

// UserGame is one collection entry (subset of the API response).
type UserGame struct {
	ID             string             `json:"id"`
	GameID         int                `json:"game_id"`
	PlayStatus     *string            `json:"play_status"`
	PersonalRating *int               `json:"personal_rating"`
	IsLoved        bool               `json:"is_loved"`
	IsWishlisted   bool               `json:"is_wishlisted"`
	PersonalNotes  *string            `json:"personal_notes"`
	Game           *GameRef           `json:"game"`
	HoursPlayed    float64            `json:"hours_played"`
	Platforms      []UserGamePlatform `json:"platforms"`
	Tags           []Tag              `json:"tags"`
}

// Title returns the game's title, or "" if the relation is absent.
func (u *UserGame) Title() string {
	if u.Game == nil {
		return ""
	}
	return u.Game.Title
}

// UserGameListResponse is the paged list payload.
type UserGameListResponse struct {
	UserGames []UserGame `json:"user_games"`
	Total     int        `json:"total"`
	Page      int        `json:"page"`
	PerPage   int        `json:"per_page"`
	Pages     int        `json:"pages"`
}

// ListUserGames returns user-games filtered by the given query params.
func (c *Client) ListUserGames(key string, params url.Values) (*UserGameListResponse, error) {
	path := "/api/user-games"
	if enc := params.Encode(); enc != "" {
		path += "?" + enc
	}
	var out UserGameListResponse
	if err := c.doBearer(http.MethodGet, path, key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUserGame fetches a single user-game by id.
func (c *Client) GetUserGame(key, id string) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodGet, "/api/user-games/"+url.PathEscape(id), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliclient/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/
git commit -m "feat: add cliclient user-game read methods"
```

---

## Task 3: cliclient — user-game mutation methods + ListTags

**Files:**
- Modify: `internal/cliclient/client.go`
- Test: `internal/cliclient/games_test.go`

**Interfaces — Consumes:** `doBearer`, `UserGame`, `Tag` (Tasks 1–2). **Produces:**
- `type PlatformInput struct { Platform, Storefront, OwnershipStatus string; HoursPlayed *float64 }` (omitempty json)
- `type CreateUserGameInput struct { GameID int; PlayStatus string; PersonalRating *int; IsLoved bool; PersonalNotes *string; IsWishlisted bool; Platforms []PlatformInput }`
- `func (c *Client) CreateUserGame(key string, in CreateUserGameInput) (*UserGame, error)`
- `func (c *Client) MoveToLibrary(key, id string, platforms []PlatformInput) (*UserGame, error)`
- `func (c *Client) UpdateUserGame(key, id string, fields map[string]any) (*UserGame, error)`
- `func (c *Client) UpdateProgress(key, id, playStatus string) (*UserGame, error)`
- `func (c *Client) AddPlatform(key, id string, p PlatformInput) error`
- `func (c *Client) UpdatePlatform(key, id, platformID string, fields map[string]any) error`
- `func (c *Client) DeletePlatform(key, id, platformID string) error`
- `func (c *Client) ReplaceTags(key, id string, tags []string) (*UserGame, error)`
- `func (c *Client) DeleteUserGame(key, id string) error`
- `func (c *Client) ListTags(key string) ([]Tag, error)`

- [ ] **Step 1: Write the failing test**

```go
func TestCreateUserGameAndReplaceTags(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["game_id"].(float64) != 2131 {
			t.Errorf("game_id = %v", body["game_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-9", "game_id": 2131})
	})
	mux.HandleFunc("/api/user-games/ug-9/tags", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		tags := body["tags"].([]any)
		if len(tags) != 2 {
			t.Errorf("tags = %v", tags)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-9", "tags": []map[string]any{{"id": "t1", "name": "RPG"}}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	ug, err := c.CreateUserGame("k", CreateUserGameInput{GameID: 2131, PlayStatus: "not_started"})
	if err != nil || ug.ID != "ug-9" {
		t.Fatalf("CreateUserGame: %v %+v", err, ug)
	}
	if _, err := c.ReplaceTags("k", "ug-9", []string{"RPG", "Backlog"}); err != nil {
		t.Fatalf("ReplaceTags: %v", err)
	}
}

func TestDeleteUserGame(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/ug-1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	if err := New(srv.URL).DeleteUserGame("k", "ug-1"); err != nil {
		t.Fatalf("DeleteUserGame: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cliclient/... -run 'TestCreateUserGameAndReplaceTags|TestDeleteUserGame'`
Expected: FAIL — undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `client.go`:

```go
// PlatformInput is a platform row for create / move-to-library / add-platform.
type PlatformInput struct {
	Platform        string   `json:"platform,omitempty"`
	Storefront      string   `json:"storefront,omitempty"`
	OwnershipStatus string   `json:"ownership_status,omitempty"`
	HoursPlayed     *float64 `json:"hours_played,omitempty"`
}

// CreateUserGameInput is the body for creating a collection entry.
type CreateUserGameInput struct {
	GameID         int             `json:"game_id"`
	PlayStatus     string          `json:"play_status,omitempty"`
	PersonalRating *int            `json:"personal_rating,omitempty"`
	IsLoved        bool            `json:"is_loved,omitempty"`
	PersonalNotes  *string         `json:"personal_notes,omitempty"`
	IsWishlisted   bool            `json:"is_wishlisted,omitempty"`
	Platforms      []PlatformInput `json:"platforms,omitempty"`
}

// CreateUserGame adds a game to the collection.
func (c *Client) CreateUserGame(key string, in CreateUserGameInput) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodPost, "/api/user-games", key, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MoveToLibrary promotes a wishlisted user-game to the library with platforms.
func (c *Client) MoveToLibrary(key, id string, platforms []PlatformInput) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"platforms": platforms}
	if err := c.doBearer(http.MethodPost, "/api/user-games/"+url.PathEscape(id)+"/move-to-library", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateUserGame applies a partial field update (play_status, personal_rating,
// is_loved, personal_notes). A nil map value clears the field server-side.
func (c *Client) UpdateUserGame(key, id string, fields map[string]any) (*UserGame, error) {
	var out UserGame
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id), key, fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateProgress sets play_status via the dedicated progress endpoint.
func (c *Client) UpdateProgress(key, id, playStatus string) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"play_status": playStatus}
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/progress", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddPlatform creates a platform row on a user-game.
func (c *Client) AddPlatform(key, id string, p PlatformInput) error {
	return c.doBearer(http.MethodPost, "/api/user-games/"+url.PathEscape(id)+"/platforms", key, p, nil)
}

// UpdatePlatform applies a partial update to one platform row.
func (c *Client) UpdatePlatform(key, id, platformID string, fields map[string]any) error {
	return c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/platforms/"+url.PathEscape(platformID), key, fields, nil)
}

// DeletePlatform removes a platform row.
func (c *Client) DeletePlatform(key, id, platformID string) error {
	return c.doBearer(http.MethodDelete, "/api/user-games/"+url.PathEscape(id)+"/platforms/"+url.PathEscape(platformID), key, nil, nil)
}

// ReplaceTags sets the complete tag set (by name) on a user-game.
func (c *Client) ReplaceTags(key, id string, tags []string) (*UserGame, error) {
	var out UserGame
	body := map[string]any{"tags": tags}
	if err := c.doBearer(http.MethodPut, "/api/user-games/"+url.PathEscape(id)+"/tags", key, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteUserGame removes a user-game.
func (c *Client) DeleteUserGame(key, id string) error {
	return c.doBearer(http.MethodDelete, "/api/user-games/"+url.PathEscape(id), key, nil, nil)
}

// ListTags returns the caller's tags (used to resolve --tag NAME to an id).
func (c *Client) ListTags(key string) ([]Tag, error) {
	var out []Tag
	if err := c.doBearer(http.MethodGet, "/api/tags", key, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cliclient/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cliclient/
git commit -m "feat: add cliclient user-game mutation + tag methods"
```

---

## Task 4: nexctl — `game` parent, registration, ref-resolution + render helpers

**Files:**
- Create: `cmd/nexctl/game.go`
- Modify: `cmd/nexctl/main.go`, `cmd/nexctl/main_test.go`
- Test: `cmd/nexctl/game_test.go`

**Interfaces — Consumes:** `resolveProfile`, `flagBool`, `cliclient.*`, `cliui`. **Produces:**
- `func newGameCmd() *cobra.Command` (registered on root; subcommands added in Tasks 5–10)
- `func resolveUserGameRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.UserGame, error)` — UUID passthrough else title search + TTY picker / off-TTY candidate error.
- `func statusOf(u *cliclient.UserGame) string`, `func ratingOf(u *cliclient.UserGame) string`, `func platformsOf(u *cliclient.UserGame) string`, `func tagsOf(u *cliclient.UserGame) string` — display helpers (handle nil/empty).
- `func looksLikeUUID(s string) bool`

For this task, `newGameCmd` registers no children yet (added in later tasks); the test asserts the command exists and that `resolveUserGameRef` resolves a unique title and errors with candidates off-TTY.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// seedProfile writes a logged-in profile pointing at srvURL and returns nothing.
func seedProfile(t *testing.T, srvURL string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := &clicfg.Config{}
	cfg.SetProfile("default", clicfg.Profile{URL: srvURL, Username: "alice", Key: "k", KeyID: "key-1"})
	if err := clicfg.Save(cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestResolveUserGameRefUniqueTitle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "hollow" {
			t.Errorf("q = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"id": 1, "title": "Hollow Knight"}}},
			"total": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd := newGameCmd()
	cmd.SetOut(&bytes.Buffer{})
	ug, err := resolveUserGameRef(cmd, cliclient.New(srv.URL), "k", "hollow")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if ug.ID != "ug-1" {
		t.Fatalf("ug = %+v", ug)
	}
}

func TestResolveUserGameRefAmbiguousOffTTYErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{
				{"id": "ug-1", "game": map[string]any{"id": 1, "title": "Final Fantasy VII"}},
				{"id": "ug-2", "game": map[string]any{"id": 2, "title": "Final Fantasy X"}},
			},
			"total": 2,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd := newGameCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(bytes.NewReader(nil)) // non-TTY
	_, err := resolveUserGameRef(cmd, cliclient.New(srv.URL), "k", "final fantasy")
	if err == nil {
		t.Fatal("expected ambiguous-ref error off-TTY")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("ug-1")) {
		t.Fatalf("error should list candidate ids: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestResolveUserGameRef`
Expected: FAIL — `newGameCmd`/`resolveUserGameRef` undefined.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newGameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "game",
		Short: "Manage your game collection",
	}
	// Subcommands are added by later tasks: list, show, add, edit, acquire, rm.
	return cmd
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func looksLikeUUID(s string) bool { return uuidRe.MatchString(s) }

// resolveUserGameRef turns a CLI reference into a user-game. A UUID is fetched
// directly; otherwise ref is a title query: 0 hits errors, 1 hit is used, and
// many hits prompt an interactive picker on a TTY or error with the candidate
// ids off-TTY.
func resolveUserGameRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.UserGame, error) {
	if looksLikeUUID(ref) {
		return c.GetUserGame(key, ref)
	}
	res, err := c.ListUserGames(key, urlValues("q", ref))
	if err != nil {
		return nil, err
	}
	switch len(res.UserGames) {
	case 0:
		return nil, fmt.Errorf("no game matching %q in your library", ref)
	case 1:
		return &res.UserGames[0], nil
	}
	if interactive(cmd) {
		return pickUserGame(cmd, res.UserGames)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d games; re-run with an id:", ref, len(res.UserGames))
	for _, u := range res.UserGames {
		fmt.Fprintf(&b, "\n  %s  %s", u.ID, u.Title())
	}
	return nil, fmt.Errorf("%s", b.String())
}

// interactive reports whether rich prompts are allowed: a TTY and none of
// --json/--quiet/--yes set.
func interactive(cmd *cobra.Command) bool {
	if flagBool(cmd, "json") || flagBool(cmd, "quiet") || flagBool(cmd, "yes") {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func pickUserGame(cmd *cobra.Command, games []cliclient.UserGame) (*cliclient.UserGame, error) {
	out := cmd.OutOrStdout()
	for i, u := range games {
		fmt.Fprintf(out, "%2d) %s  [%s]\n", i+1, u.Title(), statusOf(&games[i]))
	}
	fmt.Fprint(out, "Select a game [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return &games[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(games) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return &games[n-1], nil
}

func statusOf(u *cliclient.UserGame) string {
	if u.IsWishlisted {
		return "wishlist"
	}
	if u.PlayStatus == nil || *u.PlayStatus == "" {
		return "-"
	}
	return *u.PlayStatus
}

func ratingOf(u *cliclient.UserGame) string {
	if u.PersonalRating == nil {
		return "-"
	}
	return strconv.Itoa(*u.PersonalRating)
}

func platformsOf(u *cliclient.UserGame) string {
	if len(u.Platforms) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(u.Platforms))
	for _, p := range u.Platforms {
		if p.Platform != nil {
			parts = append(parts, *p.Platform)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func tagsOf(u *cliclient.UserGame) string {
	if len(u.Tags) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(u.Tags))
	for _, t := range u.Tags {
		parts = append(parts, t.Name)
	}
	return strings.Join(parts, ",")
}
```

Add a tiny query-string helper used across game commands — put it in `game.go`:

```go
// urlValues builds a url.Values from alternating key,value pairs, skipping
// pairs with an empty value.
func urlValues(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	return v
}
```

Add `"net/url"` to `game.go` imports.

Register the parent: in `cmd/nexctl/main.go` `newRootCmd`, add `root.AddCommand(newGameCmd())`. Add `"game"` to the `want` map in `main_test.go`'s `TestRootCmd_Structure`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game parent command and ref resolution"
```

---

## Task 5: `game list`

**Files:**
- Create: `cmd/nexctl/game_list.go`
- Test: `cmd/nexctl/game_list_test.go`

**Interfaces — Consumes:** `cliclient.ListUserGames`/`ListTags`, `resolveProfile`, `flagBool`, `cliui.EncodeJSON`. **Produces:** `func newGameListCmd() *cobra.Command` (added to `newGameCmd`).

- [ ] **Step 1: Write the failing test**

```go
func TestGameListTable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("play_status") != "completed" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{
				"id": "ug-1", "play_status": "completed",
				"game": map[string]any{"id": 1, "title": "Hollow Knight"},
			}},
			"total": 1, "page": 1, "per_page": 25, "pages": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "list", "--status", "completed"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Hollow Knight")) || !bytes.Contains(out.Bytes(), []byte("ug-1")) {
		t.Fatalf("table = %s", out.String())
	}
}

func TestGameListQuiet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_games": []map[string]any{{"id": "ug-1", "game": map[string]any{"title": "X"}}},
			"total": 1,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-q", "game", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list -q: %v", err)
	}
	if strings.TrimSpace(out.String()) != "ug-1" {
		t.Fatalf("quiet = %q", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameList`
Expected: FAIL — `game list` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_list.go`:

```go
package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameListCmd() *cobra.Command {
	var (
		status, ownership, tag, platform, storefront, genre string
		pool, sortBy, order                                 string
		wishlist, loved, hasNotes                           bool
		ratingMin, ratingMax, hoursMin, hoursMax            float64
		limit, page                                         int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List games in your collection",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			params := url.Values{}
			setIf(params, "play_status", status)
			setIf(params, "ownership_status", ownership)
			setIf(params, "platform", platform)
			setIf(params, "storefront", storefront)
			setIf(params, "genre", genre)
			setIf(params, "pool", pool)
			setIf(params, "sort_by", sortBy)
			setIf(params, "sort_order", order)
			if cmd.Flags().Changed("wishlist") {
				params.Set("wishlist", strconv.FormatBool(wishlist))
			}
			if cmd.Flags().Changed("loved") {
				params.Set("is_loved", strconv.FormatBool(loved))
			}
			if cmd.Flags().Changed("has-notes") {
				params.Set("has_notes", strconv.FormatBool(hasNotes))
			}
			if cmd.Flags().Changed("rating-min") {
				params.Set("rating_min", strconv.FormatFloat(ratingMin, 'f', -1, 64))
			}
			if cmd.Flags().Changed("rating-max") {
				params.Set("rating_max", strconv.FormatFloat(ratingMax, 'f', -1, 64))
			}
			if cmd.Flags().Changed("hours-min") {
				params.Set("time_to_beat_min", strconv.FormatFloat(hoursMin, 'f', -1, 64))
			}
			if cmd.Flags().Changed("hours-max") {
				params.Set("time_to_beat_max", strconv.FormatFloat(hoursMax, 'f', -1, 64))
			}
			if limit > 0 {
				params.Set("per_page", strconv.Itoa(limit))
			}
			if page > 0 {
				params.Set("page", strconv.Itoa(page))
			}
			if tag != "" {
				id, err := resolveTagID(c, p.Key, tag)
				if err != nil {
					return err
				}
				params.Set("tag", id)
			}

			res, err := c.ListUserGames(p.Key, params)
			if err != nil {
				return fmt.Errorf("list games failed: %w", err)
			}

			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, res)
			}
			if flagBool(cmd, "quiet") {
				for i := range res.UserGames {
					fmt.Fprintln(out, res.UserGames[i].ID)
				}
				return nil
			}
			if len(res.UserGames) == 0 {
				fmt.Fprintln(out, "No games.")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTITLE\tSTATUS\tRATING\tHOURS\tPLATFORMS\tTAGS")
			for i := range res.UserGames {
				u := &res.UserGames[i]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					u.ID, u.Title(), statusOf(u), ratingOf(u),
					strconv.FormatFloat(u.HoursPlayed, 'f', -1, 64), platformsOf(u), tagsOf(u))
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d of %d (page %d/%d)\n", len(res.UserGames), res.Total, max1(res.Page), max1(res.Pages))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Filter by play status")
	f.StringVar(&ownership, "ownership", "", "Filter by ownership status")
	f.StringVar(&tag, "tag", "", "Filter by tag name")
	f.StringVar(&platform, "platform", "", "Filter by platform slug")
	f.StringVar(&storefront, "storefront", "", "Filter by storefront slug")
	f.StringVar(&genre, "genre", "", "Filter by genre")
	f.StringVar(&pool, "pool", "", "Filter by pool id")
	f.StringVar(&sortBy, "sort", "", "Sort field (title, created_at, personal_rating, hours_played, …)")
	f.StringVar(&order, "order", "", "Sort order (asc|desc)")
	f.BoolVar(&wishlist, "wishlist", false, "Show only wishlisted games")
	f.BoolVar(&loved, "loved", false, "Filter by loved")
	f.BoolVar(&hasNotes, "has-notes", false, "Filter by has-notes")
	f.Float64Var(&ratingMin, "rating-min", 0, "Minimum personal rating")
	f.Float64Var(&ratingMax, "rating-max", 0, "Maximum personal rating")
	f.Float64Var(&hoursMin, "hours-min", 0, "Minimum time-to-beat hours")
	f.Float64Var(&hoursMax, "hours-max", 0, "Maximum time-to-beat hours")
	f.IntVar(&limit, "limit", 0, "Max results per page")
	f.IntVar(&page, "page", 0, "Page number")
	return cmd
}

func setIf(v url.Values, key, val string) {
	if val != "" {
		v.Set(key, val)
	}
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// resolveTagID maps a tag name to its id (case-insensitive), erroring if unknown.
func resolveTagID(c *cliclient.Client, key, name string) (string, error) {
	tags, err := c.ListTags(key)
	if err != nil {
		return "", fmt.Errorf("resolve tag %q: %w", name, err)
	}
	for _, t := range tags {
		if strings.EqualFold(t.Name, name) {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no tag named %q", name)
}
```

Register it: in `newGameCmd` add `cmd.AddCommand(newGameListCmd())`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestGameList`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game list"
```

---

## Task 6: `game show`

**Files:**
- Create: `cmd/nexctl/game_show.go`
- Test: `cmd/nexctl/game_show_test.go`

**Interfaces — Consumes:** `resolveUserGameRef`, `resolveProfile`, `flagBool`, `cliui.EncodeJSON`. **Produces:** `func newGameShowCmd() *cobra.Command`.

- [ ] **Step 1: Write the failing test**

```go
func TestGameShow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/123e4567-e89b-12d3-a456-426614174000", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "123e4567-e89b-12d3-a456-426614174000", "play_status": "completed",
			"game": map[string]any{"id": 1, "title": "Hollow Knight"},
			"platforms": []map[string]any{{"id": "p1", "platform": "pc_windows", "hours_played": 30.0}},
			"tags": []map[string]any{{"id": "t1", "name": "Metroidvania"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "show", "123e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err != nil {
		t.Fatalf("show: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Hollow Knight", "completed", "pc_windows", "Metroidvania"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("show missing %q: %s", want, out.String())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameShow`
Expected: FAIL — `game show` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_show.go`:

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show details for a game (by id or title)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			u, err := resolveUserGameRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			if flagBool(cmd, "json") {
				return cliui.EncodeJSON(out, u)
			}
			fmt.Fprintf(out, "%s\n", u.Title())
			fmt.Fprintf(out, "  id:      %s\n", u.ID)
			fmt.Fprintf(out, "  status:  %s\n", statusOf(u))
			fmt.Fprintf(out, "  rating:  %s\n", ratingOf(u))
			fmt.Fprintf(out, "  loved:   %t\n", u.IsLoved)
			fmt.Fprintf(out, "  hours:   %s\n", strconv.FormatFloat(u.HoursPlayed, 'f', -1, 64))
			fmt.Fprintf(out, "  tags:    %s\n", tagsOf(u))
			if u.PersonalNotes != nil && *u.PersonalNotes != "" {
				fmt.Fprintf(out, "  notes:   %s\n", *u.PersonalNotes)
			}
			if len(u.Platforms) > 0 {
				fmt.Fprintln(out, "  platforms:")
				for i := range u.Platforms {
					pl := &u.Platforms[i]
					hours := "-"
					if pl.HoursPlayed != nil {
						hours = strconv.FormatFloat(*pl.HoursPlayed, 'f', -1, 64)
					}
					fmt.Fprintf(out, "    - %s [%s] (%s h, id %s)\n", deref(pl.Platform), deref(pl.Storefront), hours, pl.ID)
				}
			}
			return nil
		},
	}
}

func deref(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
```

Note: `_ = cliclient.Client{}` is not needed; `c` is used. Register it: in `newGameCmd` add `newGameShowCmd()` to `AddCommand`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestGameShow`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game show"
```

---

## Task 7: `game add`

**Files:**
- Create: `cmd/nexctl/game_add.go`
- Test: `cmd/nexctl/game_add_test.go`

**Interfaces — Consumes:** `cliclient.{SearchIGDB,GetIGDBGame,ImportIGDBGame,CreateUserGame}`, `interactive`, `resolveProfile`, `flagBool`. **Produces:** `func newGameAddCmd() *cobra.Command` and `func splitPlatform(s string) (platform, storefront string)`.

Behaviour: resolve an IGDB candidate (via `--igdb-id` or title search + picker/candidate-error), `ImportIGDBGame`, then `CreateUserGame`. `--wishlist` and `--platform` are mutually exclusive.

- [ ] **Step 1: Write the failing test**

```go
func TestGameAddByIGDBID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/games/igdb/2131", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"games": []map[string]any{{"igdb_id": 2131, "title": "Hollow Knight"}}, "total": 1,
		})
	})
	var imported, created bool
	mux.HandleFunc("/api/games/igdb-import", func(w http.ResponseWriter, _ *http.Request) {
		imported = true
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 2131, "title": "Hollow Knight"})
	})
	mux.HandleFunc("/api/user-games", func(w http.ResponseWriter, r *http.Request) {
		created = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["game_id"].(float64) != 2131 || body["play_status"] != "in_progress" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "ug-1", "game": map[string]any{"title": "Hollow Knight"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "add", "--igdb-id", "2131", "--status", "in_progress"})
	if err := root.Execute(); err != nil {
		t.Fatalf("add: %v\n%s", err, out.String())
	}
	if !imported || !created {
		t.Fatalf("imported=%v created=%v", imported, created)
	}
}

func TestGameAddWishlistPlatformConflict(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "add", "--igdb-id", "1", "--wishlist", "--platform", "pc_windows"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected --wishlist/--platform conflict error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameAdd`
Expected: FAIL — `game add` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_add.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newGameAddCmd() *cobra.Command {
	var (
		igdbID                       int
		status, platform, storefront string
		notes                        string
		wishlist, loved              bool
		rating                       int
	)
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a game to your collection (IGDB lookup)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if wishlist && platform != "" {
				return fmt.Errorf("--wishlist and --platform are mutually exclusive")
			}
			if igdbID == 0 && len(args) == 0 {
				return fmt.Errorf("provide a title or --igdb-id")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			cand, err := resolveIGDBCandidate(cmd, c, p.Key, igdbID, firstArg(args))
			if err != nil {
				return err
			}
			if _, err := c.ImportIGDBGame(p.Key, cand.IgdbID); err != nil {
				return fmt.Errorf("import IGDB game failed: %w", err)
			}

			in := cliclient.CreateUserGameInput{
				GameID:       cand.IgdbID,
				PlayStatus:   status,
				IsLoved:      loved,
				IsWishlisted: wishlist,
			}
			if cmd.Flags().Changed("rating") {
				in.PersonalRating = &rating
			}
			if cmd.Flags().Changed("notes") {
				in.PersonalNotes = &notes
			}
			if platform != "" {
				pl, sf := splitPlatform(platform)
				if storefront != "" {
					sf = storefront
				}
				in.Platforms = []cliclient.PlatformInput{{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}}
			}

			ug, err := c.CreateUserGame(p.Key, in)
			if err != nil {
				return fmt.Errorf("add game failed: %w", err)
			}
			fmt.Fprintf(out, "Added %q (%s).\n", cand.Title, ug.ID)
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVar(&igdbID, "igdb-id", 0, "Add by IGDB id (skips title search)")
	f.StringVar(&status, "status", "not_started", "Initial play status")
	f.StringVar(&platform, "platform", "", "Platform slug, optionally platform/storefront")
	f.StringVar(&storefront, "storefront", "", "Storefront slug (overrides platform/storefront)")
	f.StringVar(&notes, "notes", "", "Personal notes")
	f.BoolVar(&wishlist, "wishlist", false, "Add to the wishlist instead of the library")
	f.BoolVar(&loved, "loved", false, "Mark as loved")
	f.IntVar(&rating, "rating", 0, "Personal rating 1–5")
	return cmd
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// splitPlatform splits "platform/storefront" into its parts; a bare value yields
// an empty storefront.
func splitPlatform(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// resolveIGDBCandidate finds the IGDB game to add: by id, or by title search with
// a TTY picker / off-TTY candidate-list error.
func resolveIGDBCandidate(cmd *cobra.Command, c *cliclient.Client, key string, igdbID int, title string) (*cliclient.IGDBCandidate, error) {
	if igdbID != 0 {
		res, err := c.GetIGDBGame(key, igdbID)
		if err != nil {
			return nil, err
		}
		if len(res.Games) == 0 {
			return nil, fmt.Errorf("no IGDB game with id %d", igdbID)
		}
		return &res.Games[0], nil
	}
	res, err := c.SearchIGDB(key, title, 10)
	if err != nil {
		return nil, err
	}
	switch len(res.Games) {
	case 0:
		return nil, fmt.Errorf("no IGDB results for %q", title)
	case 1:
		return &res.Games[0], nil
	}
	if interactive(cmd) {
		return pickIGDB(cmd, res.Games)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d IGDB games; re-run with --igdb-id:", title, len(res.Games))
	for i := range res.Games {
		g := &res.Games[i]
		marker := ""
		if g.UserGameID != nil {
			marker = " (in library)"
		}
		fmt.Fprintf(&b, "\n  %d  %s  %s%s", g.IgdbID, g.Title, g.ReleaseDate, marker)
	}
	return nil, fmt.Errorf("%s", b.String())
}

func pickIGDB(cmd *cobra.Command, games []cliclient.IGDBCandidate) (*cliclient.IGDBCandidate, error) {
	out := cmd.OutOrStdout()
	for i := range games {
		g := &games[i]
		marker := ""
		if g.UserGameID != nil {
			marker = " (in library)"
		}
		fmt.Fprintf(out, "%2d) %s  %s%s\n", i+1, g.Title, g.ReleaseDate, marker)
	}
	fmt.Fprint(out, "Select a game [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return &games[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(games) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return &games[n-1], nil
}
```

Register it: add `newGameAddCmd()` to `newGameCmd`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestGameAdd`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game add"
```

---

## Task 8: `game acquire`

**Files:**
- Create: `cmd/nexctl/game_acquire.go`
- Test: `cmd/nexctl/game_acquire_test.go`

**Interfaces — Consumes:** `resolveUserGameRef`, `cliclient.MoveToLibrary`, `splitPlatform`. **Produces:** `func newGameAcquireCmd() *cobra.Command`.

- [ ] **Step 1: Write the failing test**

```go
func TestGameAcquire(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/"+id+"/move-to-library", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		plats := body["platforms"].([]any)
		first := plats[0].(map[string]any)
		if first["platform"] != "pc_windows" || first["ownership_status"] != "owned" {
			t.Errorf("platforms = %v", plats)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "game": map[string]any{"title": "X"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "acquire", id, "--platform", "pc_windows"})
	if err := root.Execute(); err != nil {
		t.Fatalf("acquire: %v\n%s", err, out.String())
	}
}

func TestGameAcquireRequiresPlatform(t *testing.T) {
	seedProfile(t, "http://unused")
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"game", "acquire", "123e4567-e89b-12d3-a456-426614174000"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected --platform required error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameAcquire`
Expected: FAIL — `game acquire` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_acquire.go`:

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
)

func newGameAcquireCmd() *cobra.Command {
	var platform, storefront, ownership string
	cmd := &cobra.Command{
		Use:   "acquire <ref>",
		Short: "Promote a wishlisted game to the library",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if platform == "" {
				return fmt.Errorf("--platform is required")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			u, err := resolveUserGameRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			pl, sf := splitPlatform(platform)
			if storefront != "" {
				sf = storefront
			}
			own := ownership
			if own == "" {
				own = "owned"
			}
			if _, err := c.MoveToLibrary(p.Key, u.ID, []cliclient.PlatformInput{{Platform: pl, Storefront: sf, OwnershipStatus: own}}); err != nil {
				return fmt.Errorf("acquire failed: %w", err)
			}
			fmt.Fprintf(out, "Moved %q to your library.\n", u.Title())
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&platform, "platform", "", "Platform slug, optionally platform/storefront (required)")
	f.StringVar(&storefront, "storefront", "", "Storefront slug (overrides platform/storefront)")
	f.StringVar(&ownership, "ownership", "owned", "Ownership status")
	return cmd
}
```

Register it: add `newGameAcquireCmd()` to `newGameCmd`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestGameAcquire`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game acquire"
```

---

## Task 9: `game rm` (refs + `--filter`)

**Files:**
- Create: `cmd/nexctl/game_rm.go`
- Test: `cmd/nexctl/game_rm_test.go`

**Interfaces — Consumes:** `resolveUserGameRef`, `cliclient.{ListUserGames,DeleteUserGame}`, `cliui.Confirm`, `flagBool`. **Produces:** `func newGameRmCmd() *cobra.Command` and `func gamesForRefsOrFilter(...)` (shared with edit in Task 10 — define it here, edit reuses it).

Behaviour: with explicit refs, resolve each; with `--filter`, list matching games (reusing the same param-building as list — for this phase support `--status`/`--tag`/`--platform`/`--wishlist` on the filter). Confirm (unless `-y`), then delete each.

- [ ] **Step 1: Write the failing test**

```go
func TestGameRmByID(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var deleted bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/"+id, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "game": map[string]any{"title": "Doomed"}})
		case http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-y", "game", "rm", id})
	if err := root.Execute(); err != nil {
		t.Fatalf("rm: %v\n%s", err, out.String())
	}
	if !deleted {
		t.Fatal("delete not called")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameRm`
Expected: FAIL — `game rm` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_rm.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameRmCmd() *cobra.Command {
	var filterStatus, filterTag, filterPlatform string
	var filterWishlist, useFilter bool
	cmd := &cobra.Command{
		Use:   "rm <ref…>",
		Short: "Remove games from your collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, gameFilter{
				use: useFilter, status: filterStatus, tag: filterTag, platform: filterPlatform, wishlist: filterWishlist,
				wishlistSet: cmd.Flags().Changed("filter-wishlist"),
			})
			if err != nil {
				return err
			}
			if len(games) == 0 {
				fmt.Fprintln(out, "No games matched.")
				return nil
			}

			ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
				fmt.Sprintf("Delete %d game(s)?", len(games)), flagBool(cmd, "yes"))
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("aborted")
			}
			for i := range games {
				if err := c.DeleteUserGame(p.Key, games[i].ID); err != nil {
					return fmt.Errorf("delete %s failed: %w", games[i].ID, err)
				}
				fmt.Fprintf(out, "Removed %q (%s).\n", games[i].Title(), games[i].ID)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&useFilter, "filter", false, "Select games by filter instead of refs")
	f.StringVar(&filterStatus, "status", "", "Filter: play status")
	f.StringVar(&filterTag, "tag", "", "Filter: tag name")
	f.StringVar(&filterPlatform, "platform", "", "Filter: platform slug")
	f.BoolVar(&filterWishlist, "filter-wishlist", false, "Filter: only wishlisted")
	return cmd
}

// gameFilter is the subset of list filters supported for bulk edit/rm selection.
type gameFilter struct {
	use                  bool
	status, tag, platform string
	wishlist, wishlistSet bool
}

// gamesForRefsOrFilter resolves explicit refs (args) or, when f.use is set, runs
// a filtered list. Exactly one mode must be used.
func gamesForRefsOrFilter(cmd *cobra.Command, c *cliclient.Client, key string, args []string, f gameFilter) ([]cliclient.UserGame, error) {
	if f.use {
		if len(args) > 0 {
			return nil, fmt.Errorf("pass refs or --filter, not both")
		}
		params := url.Values{}
		setIf(params, "play_status", f.status)
		setIf(params, "platform", f.platform)
		if f.wishlistSet {
			params.Set("wishlist", strconv.FormatBool(f.wishlist))
		}
		if f.tag != "" {
			id, err := resolveTagID(c, key, f.tag)
			if err != nil {
				return nil, err
			}
			params.Set("tag", id)
		}
		params.Set("per_page", "200")
		res, err := c.ListUserGames(key, params)
		if err != nil {
			return nil, err
		}
		return res.UserGames, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("provide one or more refs, or --filter")
	}
	games := make([]cliclient.UserGame, 0, len(args))
	for _, ref := range args {
		u, err := resolveUserGameRef(cmd, c, key, ref)
		if err != nil {
			return nil, err
		}
		games = append(games, *u)
	}
	return games, nil
}
```

Register it: add `newGameRmCmd()` to `newGameCmd`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/... -run TestGameRm`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game rm with filter selection"
```

---

## Task 10: `game edit` (consolidated mutations)

**Files:**
- Create: `cmd/nexctl/game_edit.go`
- Test: `cmd/nexctl/game_edit_test.go`

**Interfaces — Consumes:** `gamesForRefsOrFilter`, `gameFilter`, `cliclient.{UpdateProgress,UpdateUserGame,AddPlatform,UpdatePlatform,DeletePlatform,ReplaceTags,GetUserGame}`, `splitPlatform`, `flagBool`, `cliui.Confirm`. **Produces:** `func newGameEditCmd() *cobra.Command`.

Per-game ordered application (each a separate call; fail-fast naming the step): add/rm platform → `--hours` (target platform) → `--status` → field update (rating/loved/notes) → tag add/remove. Bulk (`--filter` or >1 ref) confirms unless `-y`.

- [ ] **Step 1: Write the failing test**

```go
func TestGameEditStatusAndTags(t *testing.T) {
	const id = "123e4567-e89b-12d3-a456-426614174000"
	var gotStatus string
	var gotTags []any
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/"+id, func(w http.ResponseWriter, r *http.Request) {
		// GET for current tags (tag merge) and for ref resolution.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "game": map[string]any{"title": "X"},
			"tags": []map[string]any{{"id": "t1", "name": "RPG"}},
		})
	})
	mux.HandleFunc("/api/user-games/"+id+"/progress", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotStatus, _ = b["play_status"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	mux.HandleFunc("/api/user-games/"+id+"/tags", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		gotTags = b["tags"].([]any)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "edit", id, "--status", "completed", "--tag", "Favourite", "--untag", "RPG"})
	if err := root.Execute(); err != nil {
		t.Fatalf("edit: %v\n%s", err, out.String())
	}
	if gotStatus != "completed" {
		t.Fatalf("status = %q", gotStatus)
	}
	// current {RPG} + add {Favourite} - remove {RPG} = {Favourite}
	if len(gotTags) != 1 || gotTags[0] != "Favourite" {
		t.Fatalf("tags = %v", gotTags)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/nexctl/... -run TestGameEdit`
Expected: FAIL — `game edit` unknown.

- [ ] **Step 3: Write minimal implementation**

`cmd/nexctl/game_edit.go`:

```go
package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newGameEditCmd() *cobra.Command {
	var (
		status, notes, addPlatform, rmPlatform, hoursPlatform string
		rating                                                int
		hours                                                 float64
		loved, noLoved                                        bool
		addTags, rmTags                                       []string
		useFilter                                             bool
		filterStatus, filterTag, filterPlatform               string
		filterWishlist                                        bool
	)
	cmd := &cobra.Command{
		Use:   "edit <ref…>",
		Short: "Edit one or more games (status, rating, notes, platforms, tags)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if loved && noLoved {
				return fmt.Errorf("--loved and --no-loved are mutually exclusive")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)

			games, err := gamesForRefsOrFilter(cmd, c, p.Key, args, gameFilter{
				use: useFilter, status: filterStatus, tag: filterTag, platform: filterPlatform,
				wishlist: filterWishlist, wishlistSet: cmd.Flags().Changed("filter-wishlist"),
			})
			if err != nil {
				return err
			}
			if len(games) == 0 {
				fmt.Fprintln(out, "No games matched.")
				return nil
			}
			if len(games) > 1 {
				ok, err := cliui.Confirm(bufio.NewReader(cmd.InOrStdin()), out,
					fmt.Sprintf("Edit %d game(s)?", len(games)), flagBool(cmd, "yes"))
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("aborted")
				}
			}

			ch := cmd.Flags().Changed
			for i := range games {
				u := &games[i]
				if err := editOne(c, p.Key, u, editOpts{
					status: status, statusSet: ch("status"),
					rating: rating, ratingSet: ch("rating"),
					loved: loved, noLoved: noLoved,
					notes: notes, notesSet: ch("notes"),
					addPlatform: addPlatform, rmPlatform: rmPlatform,
					hours: hours, hoursSet: ch("hours"), hoursPlatform: hoursPlatform,
					addTags: addTags, rmTags: rmTags,
				}); err != nil {
					return fmt.Errorf("edit %q (%s): %w", u.Title(), u.ID, err)
				}
				fmt.Fprintf(out, "Updated %q (%s).\n", u.Title(), u.ID)
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Set play status")
	f.IntVar(&rating, "rating", 0, "Set personal rating 1–5")
	f.BoolVar(&loved, "loved", false, "Mark as loved")
	f.BoolVar(&noLoved, "no-loved", false, "Unmark loved")
	f.StringVar(&notes, "notes", "", "Set personal notes")
	f.StringVar(&addPlatform, "add-platform", "", "Add a platform slug (platform[/storefront])")
	f.StringVar(&rmPlatform, "rm-platform", "", "Remove a platform by slug")
	f.Float64Var(&hours, "hours", 0, "Set hours played on a platform")
	f.StringVar(&hoursPlatform, "platform", "", "Platform slug for --hours (when the game has several)")
	f.StringArrayVar(&addTags, "tag", nil, "Add a tag (repeatable)")
	f.StringArrayVar(&rmTags, "untag", nil, "Remove a tag (repeatable)")
	f.BoolVar(&useFilter, "filter", false, "Select games by filter instead of refs")
	f.StringVar(&filterStatus, "filter-status", "", "Filter: play status")
	f.StringVar(&filterTag, "filter-tag", "", "Filter: tag name")
	f.StringVar(&filterPlatform, "filter-platform", "", "Filter: platform slug")
	f.BoolVar(&filterWishlist, "filter-wishlist", false, "Filter: only wishlisted")
	return cmd
}

type editOpts struct {
	status                       string
	statusSet                    bool
	rating                       int
	ratingSet                    bool
	loved, noLoved               bool
	notes                        string
	notesSet                     bool
	addPlatform, rmPlatform      string
	hours                        float64
	hoursSet                     bool
	hoursPlatform                string
	addTags, rmTags              []string
}

func editOne(c *cliclient.Client, key string, u *cliclient.UserGame, o editOpts) error {
	if o.addPlatform != "" {
		pl, sf := splitPlatform(o.addPlatform)
		if err := c.AddPlatform(key, u.ID, cliclient.PlatformInput{Platform: pl, Storefront: sf, OwnershipStatus: "owned"}); err != nil {
			return fmt.Errorf("add platform: %w", err)
		}
	}
	if o.rmPlatform != "" {
		pid, err := platformIDBySlug(c, key, u, o.rmPlatform)
		if err != nil {
			return err
		}
		if err := c.DeletePlatform(key, u.ID, pid); err != nil {
			return fmt.Errorf("remove platform: %w", err)
		}
	}
	if o.hoursSet {
		pid, err := targetPlatformID(c, key, u, o.hoursPlatform)
		if err != nil {
			return err
		}
		if err := c.UpdatePlatform(key, u.ID, pid, map[string]any{"hours_played": o.hours}); err != nil {
			return fmt.Errorf("set hours: %w", err)
		}
	}
	if o.statusSet {
		if _, err := c.UpdateProgress(key, u.ID, o.status); err != nil {
			return fmt.Errorf("set status: %w", err)
		}
	}
	fields := map[string]any{}
	if o.ratingSet {
		fields["personal_rating"] = o.rating
	}
	if o.loved {
		fields["is_loved"] = true
	}
	if o.noLoved {
		fields["is_loved"] = false
	}
	if o.notesSet {
		fields["personal_notes"] = o.notes
	}
	if len(fields) > 0 {
		if _, err := c.UpdateUserGame(key, u.ID, fields); err != nil {
			return fmt.Errorf("update fields: %w", err)
		}
	}
	if len(o.addTags) > 0 || len(o.rmTags) > 0 {
		if err := applyTagEdits(c, key, u, o.addTags, o.rmTags); err != nil {
			return err
		}
	}
	return nil
}

// platformIDBySlug finds the platform row id whose platform slug matches.
func platformIDBySlug(c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return "", err
	}
	for i := range cur.Platforms {
		if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
			return cur.Platforms[i].ID, nil
		}
	}
	return "", fmt.Errorf("no platform %q on this game", slug)
}

// targetPlatformID picks the platform row for --hours: the named slug, else the
// sole platform, else an error asking which.
func targetPlatformID(c *cliclient.Client, key string, u *cliclient.UserGame, slug string) (string, error) {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return "", err
	}
	if slug != "" {
		for i := range cur.Platforms {
			if cur.Platforms[i].Platform != nil && *cur.Platforms[i].Platform == slug {
				return cur.Platforms[i].ID, nil
			}
		}
		return "", fmt.Errorf("no platform %q on this game", slug)
	}
	if len(cur.Platforms) == 1 {
		return cur.Platforms[0].ID, nil
	}
	return "", fmt.Errorf("game has %d platforms; specify which with --platform", len(cur.Platforms))
}

// applyTagEdits computes current ∪ add \ remove and replaces the tag set.
func applyTagEdits(c *cliclient.Client, key string, u *cliclient.UserGame, add, remove []string) error {
	cur, err := c.GetUserGame(key, u.ID)
	if err != nil {
		return err
	}
	set := map[string]string{} // lower -> display
	for _, t := range cur.Tags {
		set[strings.ToLower(t.Name)] = t.Name
	}
	for _, t := range add {
		set[strings.ToLower(t)] = t
	}
	for _, t := range remove {
		delete(set, strings.ToLower(t))
	}
	names := make([]string, 0, len(set))
	for _, disp := range set {
		names = append(names, disp)
	}
	if _, err := c.ReplaceTags(key, u.ID, names); err != nil {
		return fmt.Errorf("update tags: %w", err)
	}
	return nil
}
```

Register it: add `newGameEditCmd()` to `newGameCmd`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/nexctl/...`
Expected: PASS (full package, all game commands).

- [ ] **Step 5: Commit**

```bash
git add cmd/nexctl/
git commit -m "feat: add nexctl game edit consolidating status/rating/notes/platform/tags"
```

---

## Task 11: docs + dead-code reconciliation

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Document the `game` group in CLAUDE.md**

Under **Project Structure**, extend the `cmd/nexctl/` bullet to mention the `game` group (list/show/add/edit/acquire/rm) alongside account/profile.

- [ ] **Step 2: Build + smoke-test**

Run: `make build && ./nexctl game --help`
Expected: both binaries build; `game --help` lists `list`/`show`/`add`/`edit`/`acquire`/`rm`.

- [ ] **Step 3: Full package tests + lint**

Run: `go test ./internal/cliclient/... ./cmd/nexctl/... && golangci-lint run ./internal/cliclient/... ./cmd/nexctl/...`
Expected: PASS, 0 issues.

- [ ] **Step 4: Dead code**

Run: `make deadcode`
Expected: no NEW entries attributable to this branch. The new exported `cliclient` methods are all called by `cmd/nexctl`. Reconcile any new entry against the diff before deleting.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document nexctl game command group"
```

---

## Self-Review

**Spec coverage:** list (T5), show (T6), add (T7), acquire (T8), rm + --filter (T9), edit consolidation (T10); the client methods they need (T1–T3); the parent + ref resolution (T4); docs (T11). The spec's resolution rules (UUID passthrough, title search, TTY picker / off-TTY candidate error) are in T4 (library) and T7 (IGDB). `--tag` name→id resolution is in T5 (`resolveTagID`). The `--hours`-targets-a-platform rule and incremental `--tag/--untag` over the replace-set endpoint are in T10.

**Placeholder scan:** every step carries complete code + exact commands. No TBD/TODO.

**Type consistency:** `doBearer(method, path, key string, body, out any) error` (T1) is the basis for every method in T1–T3. `cliclient.UserGame`/`UserGamePlatform`/`Tag`/`PlatformInput`/`CreateUserGameInput` defined in T2–T3 are consumed unchanged in T4–T10. `gameFilter`/`gamesForRefsOrFilter` defined in T9 are reused in T10. `interactive`/`statusOf`/`ratingOf`/`platformsOf`/`tagsOf`/`splitPlatform`/`setIf`/`resolveTagID` are each defined once (T4/T5/T7) and reused. `flagBool`/`resolveProfile`/`cliui.{Confirm,EncodeJSON}` come from Phase 1.

**Known follow-ups (note, not blockers):** `--filter` selection caps at `per_page=200` (one page) — `log`/note if a bulk op could exceed it; `--pool` takes a UUID until the pool group lands; `edit`/`rm` filter flags are a subset of `list`'s filters.
