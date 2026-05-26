# IGDB Platform Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce the IGDB candidate set during sync matching to only games released on the platforms the source storefront reported for the item, in both the auto-match worker and the pending_review manual-match UI.

**Architecture:** Three layers. (1) Data: seed `platforms.igdb_platform_id` so we can translate our internal platform slugs to IGDB's numeric IDs. (2) A small `platformresolution` helper turns an `external_game_id` into a `[]int` of IGDB platform IDs via one SQL join. (3) The `igdb.SearchGames` client gains a `platformIDs []int` parameter, appends `where platforms = (...)` to every IGDB query body when non-empty, and falls back once to an unfiltered retry if the filtered result set is empty (preserves recall against IGDB's incomplete platform tagging). Each caller composes these layers; the IGDB client knows nothing about DB or domain.

**Tech Stack:** Go 1.25, Bun ORM, IGDB Apicalypse query language, Echo v5, PostgreSQL via testcontainers, React 19 + TanStack Query + Vitest on the frontend.

**Spec:** [docs/superpowers/specs/2026-05-26-issue-615-igdb-platform-filter-design.md](../specs/2026-05-26-issue-615-igdb-platform-filter-design.md)

**Branch:** `issue-615-igdb-platform-filter` (already checked out)

---

## File Structure

**Created:**
- `internal/services/platformresolution/igdb_ids.go` — single helper function `IGDBPlatformIDsForExternalGame`
- `internal/services/platformresolution/igdb_ids_test.go` — tests for the helper

**Modified — backend:**
- `internal/db/migrations/20260503000001_initial.up.sql` — add `igdb_platform_id` values to the `INSERT INTO platforms` block
- `internal/services/igdb/igdb.go` — add `platformIDs []int` parameter to `SearchGames`; new `buildPlatformsClause` helper; empty-result fallback recursion
- `internal/services/igdb/igdb_test.go` (and/or `igdb_extra_test.go`) — new tests for the clause and the fallback
- `internal/worker/tasks/sync.go` — call resolver before `SearchGames`
- `internal/worker/tasks/sync_test.go` — extend an existing match test to assert filter is applied
- `internal/api/games.go` — `IGDBSearchRequest` gains `ExternalGameID`; `HandleSearchIGDB` adds ownership check (403) and resolver call
- `internal/api/games_test.go` — new tests for owned-EG filter pass-through, no-EG unfiltered, cross-user 403, non-existent 403
- `slumber.yaml` — add a `search_igdb_filtered` request demonstrating the new field

**Modified — frontend:**
- `ui/frontend/src/types/jobs.ts` — `JobItem` interface gains `externalGameId: string | null`
- `ui/frontend/src/api/jobs.ts` — `JobItemApiResponse` gains `external_game_id`; `transformJobItem` maps it
- `ui/frontend/src/api/jobs.test.ts` — assert mapping
- `ui/frontend/src/api/games.ts` — `searchIGDB(query, limit?, externalGameId?)`
- `ui/frontend/src/hooks/use-games.ts` — `useSearchIGDB(query, options?)` with `externalGameId` in the query key
- `ui/frontend/src/hooks/use-games.test.tsx` — assert forwarding and key change
- `ui/frontend/src/components/jobs/job-items-details.tsx` — pass `item.externalGameId ?? undefined` to `useSearchIGDB`

---

## Tasks

### Task 1: Seed `platforms.igdb_platform_id` in the baseline migration

**Files:**
- Modify: `internal/db/migrations/20260503000001_initial.up.sql`
- Test: `internal/db/migrations_seed_test.go` (new file)

- [ ] **Step 1: Write the failing test**

Create `internal/db/migrations_seed_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
)

// TestPlatformsSeed_IGDBPlatformIDsPopulated verifies the baseline migration
// seeds platforms.igdb_platform_id for every known platform. This is the data
// half of the IGDB platform-filter feature (issue #615) — if these values are
// NULL, the filter silently does nothing for affected platforms.
func TestPlatformsSeed_IGDBPlatformIDsPopulated(t *testing.T) {
	truncateAllTables(t)
	// Re-run the seed by re-applying the initial migration's data block, or use
	// the helper that loads platforms; here we read what the shared migration
	// (already applied at TestMain) left in the table.

	tests := []struct {
		name       string
		wantIGDBID int32
	}{
		{"pc-windows", 6},
		{"mac", 14},
		{"pc-linux", 3},
		{"playstation-5", 167},
		{"playstation-4", 48},
		{"playstation-3", 9},
		{"playstation-vita", 46},
		{"playstation-psp", 38},
		{"xbox-series", 169},
		{"xbox-one", 49},
		{"xbox-360", 12},
		{"nintendo-switch", 130},
		{"nintendo-wii", 5},
		{"ios", 39},
		{"android", 34},
		{"playstation-2", 8},
		{"playstation", 7},
		{"nintendo-wii-u", 41},
		{"nintendo-switch-2", 508},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p models.Platform
			if err := testDB.NewSelect().Model(&p).Where("name = ?", tt.name).Scan(context.Background()); err != nil {
				t.Fatalf("query platform %q: %v", tt.name, err)
			}
			if p.IgdbPlatformID == nil {
				t.Fatalf("platform %q has NULL igdb_platform_id; want %d", tt.name, tt.wantIGDBID)
			}
			if *p.IgdbPlatformID != tt.wantIGDBID {
				t.Fatalf("platform %q igdb_platform_id = %d; want %d", tt.name, *p.IgdbPlatformID, tt.wantIGDBID)
			}
		})
	}
}
```

If `internal/db/` has no existing `_test.go` infrastructure with `testDB`/`truncateAllTables`/`TestMain`, place the test in `internal/db/migrations/` or piggy-back on the existing test package that uses the shared container. Look for `func TestMain` and pick the directory whose package can see `testDB`. If none exists in `internal/db/`, place this test under `internal/db/migrations_seed_test.go` with a copy of the container bootstrap; or put it in `internal/api/` next to the existing `truncateAllTables` (the assertions only need read access to the `platforms` table). The simplest and most likely path: add a single test in `internal/api/migrations_seed_test.go` that imports `testDB`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/api/... -run TestPlatformsSeed_IGDBPlatformIDsPopulated -v` (or whichever package now holds it)
Expected: FAIL for every row — `igdb_platform_id` is NULL because the column has never been seeded.

- [ ] **Step 3: Edit the baseline migration to seed the column**

In `internal/db/migrations/20260503000001_initial.up.sql`, find the `INSERT INTO platforms` statement and replace it with:

```sql
INSERT INTO platforms (name, display_name, icon, igdb_platform_id, default_storefront) VALUES
    ('pc-windows',        'PC (Windows)',               'pc-windows-icon-light.svg',        6,   'steam'),
    ('playstation-5',     'PlayStation 5',              'playstation-5-icon-light.svg',     167, 'playstation-store'),
    ('playstation-4',     'PlayStation 4',              'playstation-4-icon-light.svg',     48,  'playstation-store'),
    ('playstation-3',     'PlayStation 3',              'playstation-3-icon-light.svg',     9,   'playstation-store'),
    ('playstation-vita',  'PlayStation Vita',           NULL,                               46,  'playstation-store'),
    ('playstation-psp',   'PlayStation Portable (PSP)', NULL,                               38,  'playstation-store'),
    ('xbox-series',       'Xbox Series X/S',            'xbox-series-icon-light.svg',       169, 'microsoft-store'),
    ('xbox-one',          'Xbox One',                   'xbox-one-icon-light.svg',          49,  'microsoft-store'),
    ('xbox-360',          'Xbox 360',                   'xbox-360-icon-light.svg',          12,  'microsoft-store'),
    ('nintendo-switch',   'Nintendo Switch',            'nintendo-switch-icon-light.svg',   130, 'nintendo-eshop'),
    ('nintendo-wii',      'Nintendo Wii',               'nintendo-wii-icon-light.svg',      5,   'nintendo-eshop'),
    ('ios',               'iOS',                        'ios-icon-light.svg',               39,  'apple-app-store'),
    ('android',           'Android',                    'android-icon-light.svg',           34,  'google-play-store'),
    ('playstation-2',     'PlayStation 2',              'playstation-2-icon-light.svg',     8,   'physical'),
    ('playstation',       'PlayStation',                'playstation-icon-light.svg',       7,   'physical'),
    ('nintendo-wii-u',    'Nintendo Wii U',             'nintendo-wii-u-icon-light.svg',    41,  'nintendo-eshop'),
    ('pc-linux',          'PC (Linux)',                 'pc-linux-icon-light.svg',          3,   'steam'),
    ('mac',               'Mac',                        'mac-icon-light.svg',               14,  'steam'),
    ('nintendo-switch-2', 'Nintendo Switch 2',          'nintendo-switch-2-icon-light.svg', 508, 'nintendo-eshop');
```

No `.down.sql` change needed — that file already drops the table.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/api/... -run TestPlatformsSeed_IGDBPlatformIDsPopulated -v`
Expected: PASS for all 19 rows.

If the test fails with "container already cached", restart the testcontainers reaper or `go clean -testcache` and re-run. The migration is re-applied per package on first test invocation.

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/20260503000001_initial.up.sql internal/api/migrations_seed_test.go
git commit -m "$(cat <<'EOF'
feat(platforms): seed igdb_platform_id for IGDB filter (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Resolution helper `IGDBPlatformIDsForExternalGame`

**Files:**
- Create: `internal/services/platformresolution/igdb_ids.go`
- Create: `internal/services/platformresolution/igdb_ids_test.go`

The existing package already lives at `internal/services/platformresolution/`. It currently has no DB access in its tests; we add one DB-driven test using the shared container.

- [ ] **Step 1: Write the failing tests**

Create `internal/services/platformresolution/igdb_ids_test.go`:

```go
package platformresolution_test

import (
	"context"
	"testing"

	"github.com/drzero42/nexorious/internal/services/platformresolution"
)

// These tests rely on a shared testDB initialised by TestMain in this package.
// If there is no TestMain yet, copy the testcontainer bootstrap from
// internal/api/main_test.go and expose testDB as a package var. The shared
// migration seeds the platforms table (after Task 1) so the test can rely on
// platforms.igdb_platform_id values being populated.

func TestIGDBPlatformIDsForExternalGame_ReturnsIDsForOwnedPlatforms(t *testing.T) {
	truncateAllTables(t)
	insertTestUser(t, testDB, "u1")
	egID := insertTestExternalGame(t, testDB, "u1", "steam", "Test Game")
	insertTestExternalGamePlatform(t, testDB, egID, "pc-windows") // igdb_platform_id = 6
	insertTestExternalGamePlatform(t, testDB, egID, "pc-linux")   // igdb_platform_id = 3

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, egID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsAll(ids, []int{3, 6}) || len(ids) != 2 {
		t.Fatalf("got %v, want [3 6] (any order)", ids)
	}
}

func TestIGDBPlatformIDsForExternalGame_MissingEGReturnsEmpty(t *testing.T) {
	truncateAllTables(t)

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("got %v, want empty", ids)
	}
}

func TestIGDBPlatformIDsForExternalGame_NullIDsAreSkipped(t *testing.T) {
	truncateAllTables(t)
	insertTestUser(t, testDB, "u2")
	egID := insertTestExternalGame(t, testDB, "u2", "physical", "Old Game")
	// Insert a synthetic platform with NULL igdb_platform_id, attach the EG to it.
	if _, err := testDB.NewRaw(
		`INSERT INTO platforms (name, display_name, igdb_platform_id, default_storefront) VALUES ('test-null-platform', 'Test Null', NULL, 'physical')`,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert null-id platform: %v", err)
	}
	insertTestExternalGamePlatform(t, testDB, egID, "test-null-platform")
	insertTestExternalGamePlatform(t, testDB, egID, "playstation-2") // igdb_platform_id = 8

	ids, err := platformresolution.IGDBPlatformIDsForExternalGame(context.Background(), testDB, egID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != 8 {
		t.Fatalf("got %v, want [8] only (NULL id skipped)", ids)
	}
}

// containsAll reports whether haystack contains every needle (order-independent).
func containsAll(haystack []int, needles []int) bool {
	set := make(map[int]bool, len(haystack))
	for _, v := range haystack {
		set[v] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}
```

If `truncateAllTables`, `insertTestUser`, `insertTestExternalGame`, `insertTestExternalGamePlatform`, or the `testDB` package var don't exist yet for `platformresolution`, add them in `internal/services/platformresolution/main_test.go` mirroring the pattern in `internal/api/main_test.go` / `internal/api/helpers_test.go` (shared package-level `testDB`, single `TestMain` that runs migrations against a testcontainer Postgres). Reuse the existing helper shapes verbatim — these tests are not the right place to redesign the test harness.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/services/platformresolution/... -run TestIGDBPlatformIDsForExternalGame -v`
Expected: FAIL — `IGDBPlatformIDsForExternalGame` does not exist.

- [ ] **Step 3: Implement the helper**

Create `internal/services/platformresolution/igdb_ids.go`:

```go
package platformresolution

import (
	"context"

	"github.com/uptrace/bun"
)

// IGDBPlatformIDsForExternalGame returns the IGDB numeric platform IDs for the
// platforms attached to this external_game. Platforms whose igdb_platform_id is
// NULL are silently skipped. Returns an empty slice (not an error) if the
// external_game has no platforms or no resolvable IDs. Returns an error only on
// DB failure.
//
// Used by the IGDB sync match path (both auto-match in the worker and manual
// match via POST /api/games/search/igdb) to scope IGDB search results to
// platforms the storefront actually reports for that specific game (issue #615).
func IGDBPlatformIDsForExternalGame(ctx context.Context, db *bun.DB, externalGameID string) ([]int, error) {
	var ids []int
	err := db.NewRaw(
		`SELECT DISTINCT p.igdb_platform_id
		 FROM external_game_platforms egp
		 JOIN platforms p ON p.name = egp.platform
		 WHERE egp.external_game_id = ? AND p.igdb_platform_id IS NOT NULL`,
		externalGameID,
	).Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/services/platformresolution/... -v`
Expected: PASS — all three new tests green, existing tests in the package still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/services/platformresolution/
git commit -m "$(cat <<'EOF'
feat(platformresolution): add IGDB platform ID resolver (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: IGDB client — `SearchGames` gains `platformIDs []int` with clause and empty-result fallback

**Files:**
- Modify: `internal/services/igdb/igdb.go`
- Modify: `internal/services/igdb/igdb_test.go` (or `igdb_extra_test.go` — wherever new tests fit)
- Modify (ripple): `internal/worker/tasks/sync.go` — pass `nil` for now
- Modify (ripple): `internal/api/games.go` — pass `nil` for now

This single task changes the IGDB client signature and necessarily updates the two non-test callers to keep the build green. Behaviour at those callers is unchanged (passing `nil` is byte-identical to today's queries). Subsequent tasks wire the real `platformIDs` value.

- [ ] **Step 1: Write the failing tests**

Add to `internal/services/igdb/igdb_test.go` (or `igdb_extra_test.go`):

```go
func TestClient_SearchGames_EmptyPlatformIDs_NoClause(t *testing.T) {
	var receivedBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "X", Slug: "x"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	_, err := client.SearchGames(context.Background(), "x", 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, b := range receivedBodies {
		if strings.Contains(b, "platforms") {
			t.Fatalf("nil platformIDs must produce a body without 'platforms'; got %q", b)
		}
	}
}

func TestClient_SearchGames_NonEmptyPlatformIDs_AppendsClause(t *testing.T) {
	var receivedBodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 1, Name: "X", Slug: "x"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	_, err := client.SearchGames(context.Background(), "x", 10, []int{6, 14, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(receivedBodies) == 0 {
		t.Fatal("no IGDB requests captured")
	}
	for _, b := range receivedBodies {
		if !strings.Contains(b, "platforms = (6,14,3)") {
			t.Fatalf("body must contain 'platforms = (6,14,3)'; got %q", b)
		}
	}
}

func TestClient_SearchGames_EmptyFilteredResult_FallsBackUnfiltered(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		// First few calls (filtered): return empty.
		// Once a body without 'platforms' arrives (fallback), return a result.
		if strings.Contains(string(body), "platforms = (") {
			_ = json.NewEncoder(w).Encode([]igdbGameResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode([]igdbGameResponse{
			{ID: 42, Name: "Fallback Hit", Slug: "fallback-hit"},
		})
	}))
	defer srv.Close()

	client := testIGDBClient(t, srv)

	results, err := client.SearchGames(context.Background(), "fallback hit", 10, []int{6})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback to return at least one result")
	}
	if results[0].IgdbID != 42 {
		t.Fatalf("expected fallback result IGDB ID 42, got %d", results[0].IgdbID)
	}

	// Sanity: at least one filtered body AND one unfiltered body were sent.
	var sawFiltered, sawUnfiltered bool
	for _, b := range bodies {
		if strings.Contains(b, "platforms = (") {
			sawFiltered = true
		} else {
			sawUnfiltered = true
		}
	}
	if !sawFiltered || !sawUnfiltered {
		t.Fatalf("expected both filtered and unfiltered request bodies; filtered=%v unfiltered=%v bodies=%v", sawFiltered, sawUnfiltered, bodies)
	}
}

// testIGDBClient constructs a minimal IGDB Client wired to a test server.
// If a similar helper already exists in this file, use that instead and delete this one.
func testIGDBClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return &Client{
		httpClient: srv.Client(),
		auth: &AuthManager{
			accessToken:  "test-token",
			expiresAt:    time.Now().Add(1 * time.Hour),
			clientID:     "test",
			clientSecret: "test",
			httpClient:   srv.Client(),
			tokenURL:     srv.URL,
		},
		limiter:    rate.NewLimiter(rate.Inf, 1),
		apiURL:     srv.URL,
		configured: true,
	}
}
```

The existing `TestClient_SearchGames` (which currently calls `SearchGames(ctx, query, 10)` with three args) will fail to compile after Step 3 — that's expected and gets fixed in Step 3 along with the production callers.

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/services/igdb/... -run TestClient_SearchGames -v`
Expected: COMPILE FAILURE — `SearchGames` signature only accepts three args; the new tests pass four. This is the failing state for TDD; treat compile failure as "test fails".

- [ ] **Step 3: Implement the platform clause and fallback**

In `internal/services/igdb/igdb.go`, change the `SearchGames` signature and apply the clause at every query body, then add the empty-result fallback.

3a. Update the function signature and add a helper near the top of the file (close to `escapeIGDB`):

```go
// buildPlatformsClause builds Apicalypse fragments that scope a query to a
// platform set. Returns ("", "") when platformIDs is empty.
//
// For queries that already carry a `where ... = "..."` clause (exact-name
// lookups), the caller appends whereSuffix — " & platforms = (6,14,3)" — to
// the existing where (Apicalypse AND-joins with &).
//
// For `search "..."; fields ...` queries (which take an optional standalone
// `where` placed after fields), the caller appends searchTail — a fully-formed
// `; where platforms = (6,14,3);` statement.
func buildPlatformsClause(platformIDs []int) (whereSuffix, searchTail string) {
	if len(platformIDs) == 0 {
		return "", ""
	}
	parts := make([]string, len(platformIDs))
	for i, id := range platformIDs {
		parts[i] = strconv.Itoa(id)
	}
	csv := strings.Join(parts, ",")
	whereSuffix = " & platforms = (" + csv + ")"
	searchTail = " where platforms = (" + csv + ");"
	return
}
```

3b. Modify `SearchGames`:

```go
// SearchGames implements the full IGDB search pipeline. When platformIDs is
// non-empty, every IGDB query is scoped to those platforms; if the filtered
// search returns zero candidates, SearchGames retries once unfiltered (IGDB's
// platform tagging is incomplete and some legitimate Steam titles lack PC tags).
func (c *Client) SearchGames(ctx context.Context, query string, limit int, platformIDs []int) ([]GameMetadata, error) {
	if !c.configured {
		return nil, ErrIGDBNotConfigured
	}

	whereSuffix, searchTail := buildPlatformsClause(platformIDs)

	queries := expandQueries(query)
	original := queries[0]

	var fuzzyResults, exactResults []igdbGameResponse
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		fuzzyResults, err = c.searchIGDB(gctx, fmt.Sprintf(`search "%s"; fields %s; limit %d;%s`, escapeIGDB(original), igdbGameFields, limit, searchTail))
		return err
	})

	g.Go(func() error {
		var err error
		exactResults, err = c.searchIGDB(gctx, fmt.Sprintf(`fields %s; where name = "%s"%s; limit %d;`, igdbGameFields, escapeIGDB(original), whereSuffix, limit))
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	seen := make(map[int]bool)
	var merged []igdbGameResponse
	for _, game := range exactResults {
		if !seen[game.ID] {
			seen[game.ID] = true
			merged = append(merged, game)
		}
	}
	for _, game := range fuzzyResults {
		if !seen[game.ID] {
			seen[game.ID] = true
			merged = append(merged, game)
		}
	}

	for _, expandedQuery := range queries[1:] {
		exactExp, err := c.searchIGDB(ctx, fmt.Sprintf(`fields %s; where name = "%s"%s; limit %d;`, igdbGameFields, escapeIGDB(expandedQuery), whereSuffix, limit))
		if err == nil {
			for _, game := range exactExp {
				if !seen[game.ID] {
					seen[game.ID] = true
					merged = append(merged, game)
				}
			}
		}
		fuzzyExp, err := c.searchIGDB(ctx, fmt.Sprintf(`search "%s"; fields %s; limit %d;%s`, escapeIGDB(expandedQuery), igdbGameFields, limit, searchTail))
		if err == nil {
			for _, game := range fuzzyExp {
				if !seen[game.ID] {
					seen[game.ID] = true
					merged = append(merged, game)
				}
			}
		}
	}

	normalizedQuery := matching.NormalizeTitle(query)
	var candidates []scoredCandidate
	for _, game := range merged {
		md := convertToGameMetadata(game)
		normalizedTitle := matching.NormalizeTitle(md.Title)
		score := matching.FuzzyConfidence(normalizedQuery, normalizedTitle)
		if score >= fuzzySearchThreshold {
			candidates = append(candidates, scoredCandidate{metadata: md, score: score})
		}
	}
	sortByScore(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]GameMetadata, len(candidates))
	for i, c := range candidates {
		results[i] = c.metadata
	}

	// Empty-result fallback: if a platform filter was applied and produced no
	// candidates, retry once without the filter. IGDB's platform tagging is
	// incomplete; legitimate Steam games occasionally lack a PC platform tag.
	if len(results) == 0 && len(platformIDs) > 0 {
		slog.Debug("igdb: platform-filtered search returned 0 candidates, retrying unfiltered",
			"query", query, "platform_ids", platformIDs)
		return c.SearchGames(ctx, query, limit, nil)
	}

	return results, nil
}
```

Make sure these imports are present at the top of the file: `"log/slog"`, `"strconv"`. They likely are already; if not, add them.

3c. Update the two non-test call sites to pass `nil` (pure mechanical change, no behaviour change):

- `internal/worker/tasks/sync.go:402`:
  ```go
  candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, nil)
  ```
- `internal/api/games.go:204`:
  ```go
  results, err := h.igdb.SearchGames(c.Request().Context(), req.Query, req.Limit, nil)
  ```

3d. Update the existing `TestClient_SearchGames` in `igdb_test.go` to pass `nil` as the new fourth argument (line ~185):

```go
results, err := client.SearchGames(context.Background(), "The Witcher 3", 10, nil)
```

If any other test file calls `SearchGames` (search via grep), update it the same way.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/services/igdb/... -v`
Expected: PASS — new tests green, existing `TestClient_SearchGames` still green.

Then run the full Go test suite to catch any other ripples:

Run: `go test -timeout 600s ./...`
Expected: PASS across all packages. If a test in a worker or API package fails to compile because it calls `SearchGames` with three args, add `, nil` and re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/services/igdb/ internal/worker/tasks/sync.go internal/api/games.go
git commit -m "$(cat <<'EOF'
feat(igdb): add optional platform filter and empty-result fallback to SearchGames (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Sync worker — call resolver before `SearchGames`

**Files:**
- Modify: `internal/worker/tasks/sync.go`
- Modify: `internal/worker/tasks/sync_test.go`

- [ ] **Step 1: Write the failing test**

Find the existing test in `sync_test.go` that covers the IGDB match path (search for a test that mocks `IGDBClient` and feeds an `ExternalGame` with `Platforms`). Extend it (or add a new dedicated test) so the mock captures the `platformIDs` argument and the test asserts non-empty.

If the existing mock doesn't capture `platformIDs`, modify the mock type. Example pattern:

```go
type capturingIGDB struct {
	lastQuery       string
	lastPlatformIDs []int
	resultsToReturn []igdb.GameMetadata
}

func (m *capturingIGDB) Configured() bool { return true }
func (m *capturingIGDB) SearchGames(ctx context.Context, query string, limit int, platformIDs []int) ([]igdb.GameMetadata, error) {
	m.lastQuery = query
	m.lastPlatformIDs = append(m.lastPlatformIDs[:0], platformIDs...)
	return m.resultsToReturn, nil
}
// ... implement the rest of the interface the worker uses ...
```

Add a new test:

```go
func TestSync_IGDBMatch_PassesPlatformIDsFromExternalGame(t *testing.T) {
	truncateAllTables(t)
	// Insert user, external_game on storefront 'steam' with platforms pc-windows + pc-linux.
	userID := "u-sync-match-1"
	insertTestUser(t, testDB, userID)
	egID := insertTestExternalGame(t, testDB, userID, "steam", "Test Title")
	insertTestExternalGamePlatform(t, testDB, egID, "pc-windows")
	insertTestExternalGamePlatform(t, testDB, egID, "pc-linux")

	mockIGDB := &capturingIGDB{
		resultsToReturn: []igdb.GameMetadata{{IgdbID: 999, Title: "Test Title"}},
	}
	// ... construct worker with mockIGDB, run the match for the job item that
	// points at egID ...

	// Expect the mock to have received platformIDs containing 6 (pc-windows) and 3 (pc-linux).
	got := mockIGDB.lastPlatformIDs
	if !containsInt(got, 6) || !containsInt(got, 3) {
		t.Fatalf("expected platformIDs to include 6 and 3, got %v", got)
	}
}
```

Use existing fixture helpers (`insertTestUser`, etc.) — copy from the same `sync_test.go` style. `containsInt` is a one-liner helper if not already present.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/tasks/... -run TestSync_IGDBMatch_PassesPlatformIDsFromExternalGame -v`
Expected: FAIL — current code passes `nil`, so `lastPlatformIDs` is empty.

- [ ] **Step 3: Wire the resolver in `sync.go`**

In `internal/worker/tasks/sync.go`, replace the line that currently passes `nil` (introduced in Task 3) with:

```go
platformIDs, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, w.DB, eg.ID)
if perErr != nil {
    slog.Debug("igdb_match: platform resolution failed, falling back to unfiltered",
        "item_id", p.JobItemID, "external_game_id", eg.ID, "err", perErr)
    platformIDs = nil
}
candidates, err := w.IGDBClient.SearchGames(ctx, eg.Title, 10, platformIDs)
```

Add the import: `"github.com/drzero42/nexorious/internal/services/platformresolution"` if not already present.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/worker/tasks/... -v`
Expected: PASS — the new test plus all existing tests in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go
git commit -m "$(cat <<'EOF'
feat(sync): scope IGDB auto-match to per-game platforms (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: HTTP handler — ownership check + EG-driven filter

**Files:**
- Modify: `internal/api/games.go`
- Modify: `internal/api/games_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/api/games_test.go`:

```go
func TestSearchIGDB_ExternalGameID_CrossUserReturns403(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB) // unconfigured IGDB client is fine; 403 returns before IGDB

	// Owning user.
	insertAuthTestUser(t, testDB, "u-owner", "owner", "pass123", true, false)
	otherEGID := insertTestExternalGameForUser(t, testDB, "u-owner", "steam", "Owner's Game")

	// Calling user.
	insertAuthTestUser(t, testDB, "u-caller", "caller", "pass123", true, false)
	insertAuthTestSession(t, testDB, "u-caller", "access-caller", "refresh-caller", 1)
	token := loginAndGetToken(t, e, "caller", "pass123")

	body := fmt.Sprintf(`{"query": "x", "limit": 10, "external_game_id": %q}`, otherEGID)
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for cross-user external_game_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchIGDB_ExternalGameID_NonExistentReturns403(t *testing.T) {
	truncateAllTables(t)
	e := newTestEchoWithIGDB(t, testDB)

	insertAuthTestUser(t, testDB, "u-caller-2", "caller2", "pass123", true, false)
	insertAuthTestSession(t, testDB, "u-caller-2", "access-caller-2", "refresh-caller-2", 1)
	token := loginAndGetToken(t, e, "caller2", "pass123")

	body := `{"query": "x", "limit": 10, "external_game_id": "ghost-id-does-not-exist"}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-existent external_game_id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSearchIGDB_ExternalGameID_OwnedPassesPlatformIDs(t *testing.T) {
	truncateAllTables(t)
	// Configure IGDB to point at a httptest server so we can capture the
	// outbound request body and assert it contains the platform clause.
	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, string(body))
		_ = json.NewEncoder(w).Encode([]map[string]any{}) // empty result; we only care about the request body
	}))
	defer srv.Close()

	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL) // new helper; see note below

	insertAuthTestUser(t, testDB, "u-caller-3", "caller3", "pass123", true, false)
	insertAuthTestSession(t, testDB, "u-caller-3", "access-caller-3", "refresh-caller-3", 1)
	token := loginAndGetToken(t, e, "caller3", "pass123")
	egID := insertTestExternalGameForUser(t, testDB, "u-caller-3", "steam", "Owned Title")
	insertTestExternalGamePlatformForUser(t, testDB, egID, "pc-windows")

	body := fmt.Sprintf(`{"query": "x", "limit": 10, "external_game_id": %q}`, egID)
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// At least one captured request body must contain the platform clause.
	found := false
	for _, b := range captured {
		if strings.Contains(b, "platforms = (6)") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one IGDB request body to contain 'platforms = (6)'; got %v", captured)
	}
}

func TestSearchIGDB_NoExternalGameID_UnfilteredCall(t *testing.T) {
	truncateAllTables(t)
	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = append(captured, string(body))
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	e := newTestEchoWithLiveIGDB(t, testDB, srv.URL)
	insertAuthTestUser(t, testDB, "u-caller-4", "caller4", "pass123", true, false)
	insertAuthTestSession(t, testDB, "u-caller-4", "access-caller-4", "refresh-caller-4", 1)
	token := loginAndGetToken(t, e, "caller4", "pass123")

	body := `{"query": "x", "limit": 10}`
	rec := postAuth(t, e, "/api/games/search/igdb", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, b := range captured {
		if strings.Contains(b, "platforms = (") {
			t.Fatalf("body without external_game_id must produce unfiltered IGDB calls; got %q", b)
		}
	}
}
```

Two new helpers are needed in `internal/api/helpers_test.go` (or `main_test.go`):

```go
// insertTestExternalGameForUser inserts a minimal external_game row and returns its id.
func insertTestExternalGameForUser(t *testing.T, db *bun.DB, userID, storefront, title string) string {
	t.Helper()
	id := "eg-" + uuid.NewString()
	if _, err := db.NewRaw(
		`INSERT INTO external_games (id, user_id, storefront, external_id, title, is_skipped, is_available, is_subscription, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, false, true, false, now(), now())`,
		id, userID, storefront, "ext-"+id, title,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert external_game: %v", err)
	}
	return id
}

// insertTestExternalGamePlatformForUser attaches a canonical platform slug to an external_game.
func insertTestExternalGamePlatformForUser(t *testing.T, db *bun.DB, externalGameID, platform string) {
	t.Helper()
	if _, err := db.NewRaw(
		`INSERT INTO external_game_platforms (id, external_game_id, platform, hours_played, created_at)
		 VALUES (?, ?, ?, 0, now())`,
		"egp-"+uuid.NewString(), externalGameID, platform,
	).Exec(context.Background()); err != nil {
		t.Fatalf("insert external_game_platform: %v", err)
	}
}

// newTestEchoWithLiveIGDB constructs the test Echo app with an IGDB client
// pointing at the given httptest URL (token endpoint + API endpoint).
func newTestEchoWithLiveIGDB(t *testing.T, db *bun.DB, igdbServerURL string) *echo.Echo {
	t.Helper()
	cfg := testCfg()
	cfg.IGDBClientID = "test-client"
	cfg.IGDBClientSecret = "test-secret"
	cfg.IGDBAccessToken = "test-token-preset"
	igdbClient := igdb.NewClientWithTokenURL(cfg, igdbServerURL, ratelimit.NewLocal(100, 100))
	igdbClient.SetAPIURLForTest(igdbServerURL)
	m := migrate.NewMigratorForTest(migrate.AppStateReady)
	return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil)
}
```

Adapt the constructor signature to match the actual `api.New` arguments in this codebase (read `internal/api/router.go` or follow the existing `newTestEchoWithIGDB` body).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/api/... -run TestSearchIGDB_ -v`
Expected: New tests FAIL — `IGDBSearchRequest` has no `external_game_id` field; handler returns 200 (or whatever the unconfigured path returns) instead of 403 for cross-user IDs.

- [ ] **Step 3: Update `IGDBSearchRequest` and `HandleSearchIGDB`**

In `internal/api/games.go`:

```go
type IGDBSearchRequest struct {
	Query          string  `json:"query"`
	Limit          int     `json:"limit"`
	ExternalGameID *string `json:"external_game_id,omitempty"`
}
```

Replace the body of `HandleSearchIGDB` (the existing checks for `req.Query == ""` and `req.Limit` stay) with the ownership-aware version:

```go
func (h *GamesHandler) HandleSearchIGDB(c *echo.Context) error {
	var req IGDBSearchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query is required"})
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 50 {
		req.Limit = 50
	}

	ctx := c.Request().Context()

	var platformIDs []int
	if req.ExternalGameID != nil && *req.ExternalGameID != "" {
		userID := auth.UserIDFromContext(c)
		if userID == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		var exists bool
		if err := h.db.NewRaw(
			`SELECT EXISTS(SELECT 1 FROM external_games WHERE id = ? AND user_id = ?)`,
			*req.ExternalGameID, userID,
		).Scan(ctx, &exists); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "ownership check failed"})
		}
		if !exists {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "external_game not found or not owned by user"})
		}

		if ids, perErr := platformresolution.IGDBPlatformIDsForExternalGame(ctx, h.db, *req.ExternalGameID); perErr == nil {
			platformIDs = ids
		} else {
			slog.Debug("HandleSearchIGDB: platform resolution failed, falling back to unfiltered",
				"external_game_id", *req.ExternalGameID, "err", perErr)
		}
	}

	results, err := h.igdb.SearchGames(ctx, req.Query, req.Limit, platformIDs)
	if err != nil {
		return h.mapIGDBError(c, err)
	}

	candidates := make([]IGDBGameCandidate, len(results))
	for i, md := range results {
		candidates[i] = metadataToCandidate(md)
	}
	return c.JSON(http.StatusOK, IGDBSearchResponse{
		Games: candidates,
		Total: len(candidates),
	})
}
```

Add imports if not already present: `"log/slog"`, `"github.com/drzero42/nexorious/internal/services/platformresolution"`, `"github.com/drzero42/nexorious/internal/auth"`. (`auth` is already imported elsewhere in this file — see `HandleStartMetadataRefreshJob` around line 314.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: All four new tests PASS. Existing tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/games.go internal/api/games_test.go internal/api/helpers_test.go
git commit -m "$(cat <<'EOF'
feat(api): IGDB search accepts external_game_id, enforces ownership (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Frontend `JobItem` type and DTO mapping for `external_game_id`

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts`
- Modify: `ui/frontend/src/api/jobs.ts`
- Modify: `ui/frontend/src/api/jobs.test.ts`

- [ ] **Step 1: Write the failing test**

Append to `ui/frontend/src/api/jobs.test.ts`:

```ts
describe('transformJobItem external_game_id mapping', () => {
  it('maps external_game_id to externalGameId (non-null)', () => {
    const apiItem = {
      id: 'item-1',
      job_id: 'job-1',
      item_key: 'k',
      source_title: 'Test',
      status: 'pending_review',
      error_message: null,
      result_game_title: null,
      result_igdb_id: null,
      result_user_game_id: null,
      created_at: '2026-05-26T00:00:00Z',
      processed_at: null,
      external_game_id: 'eg-123',
    };
    // Need to import transformJobItem (export it from jobs.ts if not already exported,
    // or test indirectly via fetchJobItems with a mocked response).
    const result = transformJobItem(apiItem);
    expect(result.externalGameId).toBe('eg-123');
  });

  it('maps external_game_id null to externalGameId null', () => {
    const apiItem = {
      id: 'item-2',
      job_id: 'job-1',
      item_key: 'k',
      source_title: 'Test',
      status: 'completed',
      error_message: null,
      result_game_title: null,
      result_igdb_id: null,
      result_user_game_id: null,
      created_at: '2026-05-26T00:00:00Z',
      processed_at: null,
      external_game_id: null,
    };
    const result = transformJobItem(apiItem);
    expect(result.externalGameId).toBeNull();
  });
});
```

If `transformJobItem` is not currently exported from `ui/frontend/src/api/jobs.ts`, export it (`export function transformJobItem...`) — this enables direct unit testing.

- [ ] **Step 2: Run the test to verify it fails**

From `ui/frontend/`:

Run: `npm run test -- jobs.test`
Expected: FAIL — `transformJobItem` does not set `externalGameId`; the `JobItem` interface does not declare the field.

- [ ] **Step 3: Update the types and the transformer**

In `ui/frontend/src/types/jobs.ts`, find `export interface JobItem` and add the field:

```ts
export interface JobItem {
  // ... existing fields ...
  externalGameId: string | null;
}
```

In `ui/frontend/src/api/jobs.ts`:

```ts
interface JobItemApiResponse {
  // ... existing fields ...
  external_game_id: string | null;
}

function transformJobItem(apiItem: JobItemApiResponse): JobItem {
  return {
    // ... existing mappings ...
    externalGameId: apiItem.external_game_id,
  };
}
```

If `JobItemDetail extends JobItem` also pulls from `JobItemDetailApiResponse`, ensure the detail response interface also carries `external_game_id` and the detail transformer maps it (most likely it does, via `transformJobItem` spread — verify).

- [ ] **Step 4: Run the test to verify it passes**

Run from `ui/frontend/`:

```bash
npm run check
npm run test -- jobs.test
```

Expected: zero TypeScript errors; the two new tests PASS; existing `jobs.test.ts` cases still PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/jobs.ts ui/frontend/src/api/jobs.ts ui/frontend/src/api/jobs.test.ts
git commit -m "$(cat <<'EOF'
feat(frontend): expose externalGameId on JobItem (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Frontend `searchIGDB` API client accepts `externalGameId`

**Files:**
- Modify: `ui/frontend/src/api/games.ts`
- Modify: `ui/frontend/src/api/games.test.ts`

- [ ] **Step 1: Write the failing tests**

Append to `ui/frontend/src/api/games.test.ts`:

```ts
describe('searchIGDB external_game_id', () => {
  it('sends external_game_id in the request body when provided', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    server.use(
      http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ games: [], total: 0 });
      }),
    );

    await searchIGDB('zelda', 10, 'eg-abc-123');

    expect(capturedBody).not.toBeNull();
    expect(capturedBody!.external_game_id).toBe('eg-abc-123');
  });

  it('omits external_game_id when not provided', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    server.use(
      http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ games: [], total: 0 });
      }),
    );

    await searchIGDB('zelda');

    expect(capturedBody).not.toBeNull();
    expect('external_game_id' in capturedBody!).toBe(false);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run from `ui/frontend/`: `npm run test -- games.test`
Expected: FAIL — `searchIGDB` does not accept a third parameter; the body never includes `external_game_id`.

- [ ] **Step 3: Update `searchIGDB`**

In `ui/frontend/src/api/games.ts`, replace the existing function:

```ts
export async function searchIGDB(
  query: string,
  limit?: number,
  externalGameId?: string,
): Promise<IGDBGameCandidate[]> {
  const body: { query: string; limit: number; external_game_id?: string } = {
    query,
    limit: limit ?? 10,
  };
  if (externalGameId) {
    body.external_game_id = externalGameId;
  }
  const response = await api.post<IGDBSearchApiResponse>('/games/search/igdb', body);
  return response.games.map(transformIGDBGameCandidate);
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run from `ui/frontend/`:

```bash
npm run check
npm run test -- games.test
```

Expected: zero TS errors; the two new cases PASS; existing `searchIGDB` cases still PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/api/games.ts ui/frontend/src/api/games.test.ts
git commit -m "$(cat <<'EOF'
feat(frontend): searchIGDB accepts optional externalGameId (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Frontend `useSearchIGDB` hook accepts `externalGameId` option

**Files:**
- Modify: `ui/frontend/src/hooks/use-games.ts`
- Modify: `ui/frontend/src/hooks/use-games.test.tsx`

- [ ] **Step 1: Write the failing tests**

Append to `ui/frontend/src/hooks/use-games.test.tsx`:

```tsx
describe('useSearchIGDB with externalGameId', () => {
  it('forwards externalGameId to the API client', async () => {
    let capturedBody: Record<string, unknown> | null = null;
    server.use(
      http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ games: [], total: 0 });
      }),
    );

    const { result } = renderHook(
      () => useSearchIGDB('zelda', { externalGameId: 'eg-xyz' }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(capturedBody).not.toBeNull();
    expect(capturedBody!.external_game_id).toBe('eg-xyz');
  });

  it('produces a different cache key when externalGameId changes', async () => {
    // Two hook invocations with same query but different externalGameId should
    // independently fetch (i.e., two server requests).
    let calls = 0;
    server.use(
      http.post(`${API_URL}/games/search/igdb`, () => {
        calls++;
        return HttpResponse.json({ games: [], total: 0 });
      }),
    );

    const wrapper = createWrapper();
    renderHook(() => useSearchIGDB('zelda', { externalGameId: 'eg-a' }), { wrapper });
    renderHook(() => useSearchIGDB('zelda', { externalGameId: 'eg-b' }), { wrapper });

    await waitFor(() => expect(calls).toBe(2));
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run from `ui/frontend/`: `npm run test -- use-games.test`
Expected: FAIL — `useSearchIGDB` has no `options` parameter.

- [ ] **Step 3: Update `useSearchIGDB`**

In `ui/frontend/src/hooks/use-games.ts`, locate `useSearchIGDB` (around line 87) and change its signature and implementation:

```ts
export function useSearchIGDB(
  query: string,
  options?: { limit?: number; externalGameId?: string },
) {
  const limit = options?.limit;
  const externalGameId = options?.externalGameId;
  return useQuery({
    queryKey: ['games', 'searchIgdb', query, externalGameId ?? null],
    queryFn: () => gamesApi.searchIGDB(query, limit, externalGameId),
    enabled: query.length >= 3, // preserve any existing enabling condition; keep what was there before
    // ... preserve other existing options ...
  });
}
```

Read the existing implementation carefully and preserve any logic around `enabled`, `staleTime`, IGDB-ID prefix shortcut (`igdb:12345`), etc. The signature change should be the minimum delta needed for the new behaviour.

If any existing call site passed `useSearchIGDB(query, limit)` as positional args (i.e. with the legacy two-arg form), update those callers in the same commit to use the options object: `useSearchIGDB(query, { limit })`. Search:

```bash
grep -rn "useSearchIGDB(" ui/frontend/src --include="*.tsx" --include="*.ts"
```

- [ ] **Step 4: Run the tests to verify they pass**

Run from `ui/frontend/`:

```bash
npm run check
npm run test -- use-games.test
```

Expected: zero TS errors; new tests PASS; existing `useSearchIGDB` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/hooks/use-games.ts ui/frontend/src/hooks/use-games.test.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): useSearchIGDB accepts externalGameId option (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Manual-match UI passes `externalGameId`

**Files:**
- Modify: `ui/frontend/src/components/jobs/job-items-details.tsx`

- [ ] **Step 1: Verify the build catches the missing wire-up first**

This is a thin glue change — there is no new behaviour to test in isolation that the previous tasks didn't already verify. Skip writing a new unit test here; rely on:
1. The type system (Task 8's `useSearchIGDB` now requires the options object form),
2. The HTTP handler test from Task 5 (the filter is applied end-to-end when the EG ID is sent),
3. Manual smoke check (Step 4 below).

- [ ] **Step 2: Update the component**

In `ui/frontend/src/components/jobs/job-items-details.tsx`, locate the existing `useSearchIGDB(searchQuery)` call (around line 128 inside `ReviewItemWidget`) and change it to:

```tsx
const { data: searchResults, isLoading: isSearching } = useSearchIGDB(searchQuery, {
  externalGameId: item.externalGameId ?? undefined,
});
```

The `item` prop is already in scope (it's `JobItem` from Task 6, which now carries `externalGameId`).

- [ ] **Step 3: Type-check, dead-code check, and unit tests**

Run from `ui/frontend/`:

```bash
npm run check
npm run knip
npm run test
```

Expected: zero errors across all three commands.

- [ ] **Step 4: Manual smoke check**

Start the backend and the frontend dev server in separate shells:

```bash
make
./nexorious migrate
./nexorious &
cd ui/frontend && npm run dev
```

In a browser, run a sync that produces a `pending_review` item (e.g. a Steam library sync with a title that doesn't auto-resolve), open the item's review widget, type a search query, and confirm:
- A normal filtered query returns IGDB candidates only on the platforms the source storefront reported.
- Open DevTools → Network → click the `search/igdb` POST → confirm the request body includes `"external_game_id": "<id>"`.
- Search the same title from the generic "add game from IGDB" route (`/games` page) and confirm the request body does NOT include `external_game_id`.

If you cannot run the app locally, document this step as "deferred to PR review" in the commit body.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/jobs/job-items-details.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): wire pending_review manual match to platform filter (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 10: Slumber update + final quality gates

**Files:**
- Modify: `slumber.yaml`

- [ ] **Step 1: Add a `search_igdb_filtered` request**

In `slumber.yaml`, after the existing `search_igdb` entry (around line 484), add:

```yaml
      search_igdb_filtered:
        name: Search IGDB (platform-filtered)
        method: POST
        url: "{{base_url}}/api/games/search/igdb"
        $ref: "#/.authenticated"
        body:
          type: json
          data:
            query: "{{prompt(message='IGDB search query')}}"
            limit: 10
            external_game_id: "{{prompt(message='External game ID (from pending_review)')}}"
```

- [ ] **Step 2: Verify the slumber collection loads**

Run: `slumber collection`
Expected: the collection loads without errors. If `slumber` is not on PATH, document the verification as deferred to manual review.

- [ ] **Step 3: Final quality gates**

Run the full set of quality gates listed in CLAUDE.md:

```bash
golangci-lint run
go test -timeout 600s ./...
cd ui/frontend && npm run check && npm run knip && npm run test
```

Expected: every command exits 0.

If any of these report issues, fix them and re-run before committing. Common issues likely to surface:
- An older test file in `internal/worker/tasks/` or elsewhere that calls `SearchGames` with three arguments and was missed — append `, nil`.
- A `useSearchIGDB(query, limit)` call site in the frontend that was missed — update to `useSearchIGDB(query, { limit })`.

- [ ] **Step 4: Commit**

```bash
git add slumber.yaml
git commit -m "$(cat <<'EOF'
chore(slumber): add platform-filtered IGDB search request (#615)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Done When

- All ten tasks committed on `issue-615-igdb-platform-filter`.
- `git log --oneline main..HEAD` shows one commit per task plus the design-spec commit.
- `golangci-lint run` clean.
- `go test ./...` green.
- `npm run check && npm run knip && npm run test` (in `ui/frontend/`) all green.
- Manual smoke check from Task 9 confirms platform-filtered candidates appear in the pending_review UI.

The branch is then ready for `gh pr create`. Do not open the PR yourself — wait for the user's explicit instruction (CLAUDE.md: "Never merge a PR on your own initiative — only when the user explicitly instructs").
