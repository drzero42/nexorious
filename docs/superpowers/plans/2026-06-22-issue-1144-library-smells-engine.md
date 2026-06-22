# Library Smells Detection Engine + REST API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `internal/librarysmells` detection engine (11 detectors + 4 auto-fixes), a `smell_ignores` table, and the `/api/library/smells` REST API — the backbone the Library Health UI (#1145) and `nexctl doctor` (#1146) consume.

**Architecture:** A `internal/librarysmells` package holds a detector registry: each `Check` carries metadata + a `Detect` query (anti-joined against `smell_ignores`) + an optional `Apply` that re-validates and routes the fix through `internal/usergame`. A thin `LibrarySmellsHandler` in `internal/api` exposes summary → per-check listing → apply → ignore/restore/list-dismissed. Detection is on-demand (solo user, small libraries; no caching).

**Tech Stack:** Go 1.26, Bun (`uptrace/bun`) + raw SQL, Echo v5, PostgreSQL, testcontainers-go.

**Design spec:** `docs/superpowers/specs/2026-06-22-library-smells-engine-design.md` (read it — the rules table and predicates live there).

## Global Constraints

- **Go 1.26+.** Echo v5 handler signature is `func (h *H) HandleX(c *echo.Context) error` (pointer context).
- **Migrations are new files only**, never edit existing ones. Naming `YYYYMMDD<nnnnnn>_name.{up,down}.sql`; confirm the running number against the latest file at implementation time (currently `20260621000001`).
- **All user-game mutations route through `internal/usergame`** — never hand-chain platform inserts or status updates.
- **`errcheck` runs with `check-blank`**: no bare `_ =`/`_, _ =` error discards outside `_test.go`. Use the `RowsAffected` `//nolint:errcheck` pattern only where the existing code does.
- **`gosec` enabled**: no findings outside `_test.go`.
- **Logging:** request/handler code uses `slog.*Context(ctx, …)` with `internal/logging` keys at error boundaries; never log secrets/bodies. (These detectors are read-only; logging is minimal.)
- **Tests with a real DB**: reuse the package's shared `testDB` via `TestMain`; call `truncateAllTables(t)` at the top of each test. Seed **only** seeded platform/storefront names (`pc-windows`, `steam`, …) — arbitrary strings violate the FK to `platforms`/`storefronts`.
- **bun raw-scan structs need explicit `bun:"column"` tags** on every scanned field, or the scan silently returns nil.
- **Commits/PRs carry no AI-attribution** trailer or note.
- **Check slugs (stable identifiers)**, in epic display order:
  `storefront-less-platform`, `orphan-game`, `storefront-without-platform`, `wishlisted-yet-owned`, `missing-ownership-status`, `impossible-acquired-date`, `invalid-storefront-for-platform`, `beat-but-not-marked`, `played-but-not-started`, `in-progress-untouched`, `unrated-after-finishing`.
- **Tiers:** `inconsistency` (epic Tier 1) and `nudge` (epic Tier 2).
- **Auto-fix set:** `wishlisted-yet-owned` (clear wishlist), `beat-but-not-marked` (→ completed), `played-but-not-started` (→ in_progress), `in-progress-untouched` (→ not_started). All others deep-link only.

## File Structure

- `internal/db/migrations/20260622000001_create_smell_ignores.{up,down}.sql` — new table.
- `internal/db/models/models.go` — add `SmellIgnore` model.
- `internal/usergame/wishlist.go` — add exported `ClearWishlist` (bulk). Test in `internal/usergame/wishlist_test.go`.
- `internal/librarysmells/registry.go` — `Tier`, `Check`, `FlaggedItem`, `Registry()`, `Lookup()`.
- `internal/librarysmells/detectors.go` — the 11 `detectX` funcs + the 11 `Check` vars.
- `internal/librarysmells/apply.go` — `revalidate` helper + the 4 `applyX` funcs.
- `internal/librarysmells/main_test.go` — `TestMain`, `truncateAllTables`, seed helpers.
- `internal/librarysmells/detectors_test.go` — per-detector tests.
- `internal/librarysmells/apply_test.go` — apply-path tests + `ClearWishlist` integration via the engine.
- `internal/api/library_smells.go` — `LibrarySmellsHandler` + request/response types.
- `internal/api/router.go` — register the `/api/library/smells` group.
- `internal/api/library_smells_test.go` — handler tests.

---

### Task 1: Migration + `SmellIgnore` model

**Files:**
- Create: `internal/db/migrations/20260622000001_create_smell_ignores.up.sql`
- Create: `internal/db/migrations/20260622000001_create_smell_ignores.down.sql`
- Modify: `internal/db/models/models.go` (add `SmellIgnore`)
- Test: `internal/db/migrations/` is exercised by every package's `TestMain`; add a focused model test under `internal/librarysmells` later — for this task the gate is "migration applies + unique constraint holds", proven by a quick psql-free check via the existing api test harness in Task 9. Here we only assert the files build and the model compiles.

**Interfaces:**
- Produces: table `smell_ignores(id, user_id, user_game_id, check_id, created_at)`, unique `(user_id, user_game_id, check_id)`, FKs to `users(id)`/`user_games(id)` `ON DELETE CASCADE`; Bun model `models.SmellIgnore`.

- [ ] **Step 1: Confirm the running number**

Run: `ls internal/db/migrations/ | tail -4`
Expected: the latest prefix is `20260621000001`. If a newer `20260622*` exists, bump to `20260622000002`. Use `20260622000001` below otherwise.

- [ ] **Step 2: Write the up migration**

Create `internal/db/migrations/20260622000001_create_smell_ignores.up.sql`:

```sql
CREATE TABLE smell_ignores (
    id text NOT NULL,
    user_id text NOT NULL,
    user_game_id text NOT NULL,
    check_id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_pkey PRIMARY KEY (id);
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_game_check_key UNIQUE (user_id, user_game_id, check_id);
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY smell_ignores
    ADD CONSTRAINT smell_ignores_user_game_id_fkey FOREIGN KEY (user_game_id)
        REFERENCES user_games(id) ON DELETE CASCADE;

CREATE INDEX smell_ignores_user_check_idx ON smell_ignores (user_id, check_id);
```

- [ ] **Step 3: Write the down migration**

Create `internal/db/migrations/20260622000001_create_smell_ignores.down.sql`:

```sql
DROP TABLE IF EXISTS smell_ignores;
```

- [ ] **Step 4: Add the Bun model**

In `internal/db/models/models.go`, add after the `PlatformStorefront` struct (around line 143):

```go
type SmellIgnore struct {
	bun.BaseModel `bun:"table:smell_ignores"`

	ID         string    `bun:"id,pk"                json:"id"`
	UserID     string    `bun:"user_id,notnull"      json:"user_id"`
	UserGameID string    `bun:"user_game_id,notnull" json:"user_game_id"`
	CheckID    string    `bun:"check_id,notnull"     json:"check_id"`
	CreatedAt  time.Time `bun:"created_at,notnull"   json:"created_at"`
}
```

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: success (no compile errors).

- [ ] **Step 6: Verify the migration applies in a real DB**

Run: `go test ./internal/usergame/ -run TestHarnessUp -v`
Expected: PASS — `TestHarnessUp` re-runs `TestMain`, which discovers and applies every migration including the new one. A bad SQL file fails here.

- [ ] **Step 7: Commit (with the spec + plan)**

```bash
git add internal/db/migrations/20260622000001_create_smell_ignores.up.sql \
        internal/db/migrations/20260622000001_create_smell_ignores.down.sql \
        internal/db/models/models.go \
        docs/superpowers/specs/2026-06-22-library-smells-engine-design.md \
        docs/superpowers/plans/2026-06-22-issue-1144-library-smells-engine.md
git commit -m "feat(smells): add smell_ignores table and model"
```

---

### Task 2: `usergame.ClearWishlist` bulk mutation

**Files:**
- Modify: `internal/usergame/wishlist.go` (add exported `ClearWishlist`)
- Test: `internal/usergame/wishlist_test.go`

**Interfaces:**
- Consumes: `*bun.DB`, the private `clearWishlistOnAcquire` pattern (do not call it — reuse the SQL shape).
- Produces: `func ClearWishlist(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (int, error)` — clears `is_wishlisted` for the user's games that have ≥1 platform row; returns rows updated. Used by `librarysmells.applyClearWishlist` in Task 8.

- [ ] **Step 1: Write the failing test**

In `internal/usergame/wishlist_test.go` (create if absent; package `usergame`), add:

```go
package usergame

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestClearWishlist(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)
	other := seedUser(t)

	// wishlisted + has a platform → cleared
	owned := seedUserGame(t, userID, 101)
	setWishlisted(t, owned, true)
	addPlatformRow(t, owned, "pc-windows", "steam")

	// wishlisted, no platform → NOT cleared (still a pure wishlist entry)
	pureWish := seedUserGame(t, userID, 102)
	setWishlisted(t, pureWish, true)

	// wishlisted + platform but other user → NOT cleared (ownership scope)
	foreign := seedUserGame(t, other, 103)
	setWishlisted(t, foreign, true)
	addPlatformRow(t, foreign, "pc-windows", "steam")

	n, err := ClearWishlist(ctx, testDB, userID, []string{owned, pureWish, foreign})
	if err != nil {
		t.Fatalf("ClearWishlist: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 cleared, got %d", n)
	}
	if isWishlisted(t, owned) {
		t.Error("owned game should have been cleared")
	}
	if !isWishlisted(t, pureWish) {
		t.Error("pure-wishlist game must stay wishlisted")
	}
	if !isWishlisted(t, foreign) {
		t.Error("other user's game must not be touched")
	}

	// Idempotent: a second call clears nothing.
	n2, err := ClearWishlist(ctx, testDB, userID, []string{owned})
	if err != nil {
		t.Fatalf("ClearWishlist (2nd): %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 on idempotent call, got %d", n2)
	}
	_ = uuid.NewString // keep import if unused elsewhere
}

// --- test helpers (add if not already present in the package's test files) ---

func setWishlisted(t *testing.T, ugID string, v bool) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = ? WHERE id = ?`, v, ugID).Exec(context.Background()); err != nil {
		t.Fatalf("setWishlisted: %v", err)
	}
}

func isWishlisted(t *testing.T, ugID string) bool {
	t.Helper()
	var v bool
	if err := testDB.NewRaw(`SELECT is_wishlisted FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &v); err != nil {
		t.Fatalf("isWishlisted: %v", err)
	}
	return v
}

func addPlatformRow(t *testing.T, ugID, platform, storefront string) {
	t.Helper()
	if _, err := testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), ugID, platform, storefront).Exec(context.Background()); err != nil {
		t.Fatalf("addPlatformRow: %v", err)
	}
}
```

> Before adding `setWishlisted`/`isWishlisted`/`addPlatformRow`, grep the package's existing `_test.go` files for these names; if one already exists, reuse it and drop the duplicate.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/usergame/ -run TestClearWishlist -v`
Expected: FAIL — `undefined: ClearWishlist`.

- [ ] **Step 3: Implement `ClearWishlist`**

In `internal/usergame/wishlist.go`, add below `clearWishlistOnAcquire`:

```go
// ClearWishlist is the exported bulk counterpart of clearWishlistOnAcquire: it
// clears is_wishlisted for each of the user's games in userGameIDs that has at
// least one platform row (i.e. is actually in the library). The EXISTS guard
// keeps it from clearing a pure wishlist entry, and makes it idempotent. It is
// the auto-fix for the "wishlisted yet owned" library smell (#1144). Returns the
// number of rows updated.
func ClearWishlist(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (int, error) {
	if len(userGameIDs) == 0 {
		return 0, nil
	}
	var updated int
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		res, err := tx.NewRaw(
			`UPDATE user_games
			 SET is_wishlisted = false, updated_at = now()
			 WHERE id IN (?)
			   AND user_id = ?
			   AND is_wishlisted
			   AND EXISTS (SELECT 1 FROM user_game_platforms WHERE user_game_id = user_games.id)`,
			bun.List(userGameIDs), userID,
		).Exec(ctx)
		if err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		updated = int(n)
		return nil
	})
	return updated, err
}
```

Add `"context"`, `"fmt"`, and `"github.com/uptrace/bun"` to the imports if not present (the file currently imports `context` and `bun`; add `fmt`).

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/usergame/ -run TestClearWishlist -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usergame/wishlist.go internal/usergame/wishlist_test.go
git commit -m "feat(usergame): add exported ClearWishlist bulk mutation"
```

---

### Task 3: Engine foundation + `orphan-game` detector

**Files:**
- Create: `internal/librarysmells/registry.go`
- Create: `internal/librarysmells/detectors.go`
- Create: `internal/librarysmells/main_test.go`
- Create: `internal/librarysmells/detectors_test.go`

**Interfaces:**
- Produces:
  - `type Tier string` with `TierInconsistency`/`TierNudge`.
  - `type Check struct { ID, Title, Description string; Tier Tier; AutoFixable bool; Detect func(context.Context, *bun.DB, string) ([]FlaggedItem, error); Apply func(context.Context, *bun.DB, string, []string) (int, int, error) }`
  - `type FlaggedItem struct { … }` (bun + json tags, below).
  - `func Registry() []Check` (epic order), `func Lookup(id string) (Check, bool)`.
  - `func detectOrphanGame(ctx, db, userID) ([]FlaggedItem, error)`.
  - Test harness: `testDB`, `truncateAllTables(t)`, `seedUser(t)`, `seedGame(t, id, title)`, `seedUserGame(t, userID, gameID)`, `seedPlatform(t, ugID, platform, storefront)`, `ignore(t, userID, ugID, checkID)`.

- [ ] **Step 1: Write the registry types**

Create `internal/librarysmells/registry.go`:

```go
// Package librarysmells detects data-quality issues ("library smells") in a
// user's game collection and, for a subset, applies one-click fixes. See
// docs/superpowers/specs/2026-06-22-library-smells-engine-design.md.
package librarysmells

import (
	"context"

	"github.com/uptrace/bun"
)

// Tier is the severity grouping of a check.
type Tier string

const (
	// TierInconsistency is "something is wrong" (epic Tier 1).
	TierInconsistency Tier = "inconsistency"
	// TierNudge is "you might want to update this" (epic Tier 2).
	TierNudge Tier = "nudge"
)

// FlaggedItem is one flagged game for one check, with check-specific context.
// It carries bun tags (raw-scan target) and json tags (API response).
type FlaggedItem struct {
	UserGameID  string  `bun:"user_game_id"  json:"user_game_id"`
	GameID      int32   `bun:"game_id"       json:"game_id"`
	Title       string  `bun:"title"         json:"title"`
	CoverArtURL *string `bun:"cover_art_url" json:"cover_art_url,omitempty"`

	PlatformRowID       *string `bun:"platform_row_id"      json:"platform_row_id,omitempty"`
	Platform            *string `bun:"platform"             json:"platform,omitempty"`
	Storefront          *string `bun:"storefront"           json:"storefront,omitempty"`
	SuggestedStorefront *string `bun:"suggested_storefront" json:"suggested_storefront,omitempty"`
	SuggestedStatus     *string `bun:"suggested_status"     json:"suggested_status,omitempty"`
	Detail              *string `bun:"detail"               json:"detail,omitempty"`
}

// Check is one library-smell detector and its optional one-click fix.
type Check struct {
	ID          string
	Title       string
	Description string
	Tier        Tier
	AutoFixable bool

	// Detect returns flagged items for userID, excluding rows the user has
	// dismissed via smell_ignores for this check. Read-only.
	Detect func(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error)

	// Apply performs the one-click fix for the given user_game IDs, routing
	// through internal/usergame. Non-nil only when AutoFixable. Returns
	// (applied, skipped). nil for deep-link-only checks.
	Apply func(ctx context.Context, db *bun.DB, userID string, userGameIDs []string) (applied, skipped int, err error)
}

// Registry returns every check in epic display order.
func Registry() []Check {
	return []Check{
		orphanGameCheck,
	}
}

// Lookup resolves a check by its slug.
func Lookup(id string) (Check, bool) {
	for _, c := range Registry() {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}
```

> Note: `Registry()` lists only `orphanGameCheck` now; later tasks append the other 10 vars to this slice literal in epic order.

- [ ] **Step 2: Write the orphan-game detector**

Create `internal/librarysmells/detectors.go`:

```go
package librarysmells

import (
	"context"

	"github.com/uptrace/bun"
)

var orphanGameCheck = Check{
	ID:          "orphan-game",
	Title:       "Orphan game",
	Description: "A game in your library with no platform or storefront recorded.",
	Tier:        TierInconsistency,
	Detect:      detectOrphanGame,
}

// detectOrphanGame flags non-wishlisted games with zero platform rows. A
// wishlisted game legitimately has no platforms, so it is excluded.
func detectOrphanGame(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.is_wishlisted = false
		   AND NOT EXISTS (SELECT 1 FROM user_game_platforms p WHERE p.user_game_id = ug.id)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "orphan-game",
	).Scan(ctx, &items)
	return items, err
}
```

- [ ] **Step 3: Write the test harness**

Create `internal/librarysmells/main_test.go` (model on `internal/usergame/main_test.go`):

```go
package librarysmells

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	riverdatabasesql "github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious/internal/db/migrations"
)

var testDB *bun.DB

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("nexorious_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "conn string: %v\n", err)
		os.Exit(1)
	}
	testDB = bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr))), pgdialect.New())
	defer func() { _ = testDB.Close() }()

	migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
	if err := migrator.Init(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrator init: %v\n", err)
		os.Exit(1)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
	riverMig, err := rivermigrate.New(riverdatabasesql.New(testDB.DB), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "river migrator: %v\n", err)
		os.Exit(1)
	}
	if _, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		fmt.Fprintf(os.Stderr, "river migrate: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func truncateAllTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`TRUNCATE users, games, user_games, user_game_platforms, smell_ignores CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at, updated_at)
		 VALUES (?, ?, 'x', false, now(), now())`,
		id, "u_"+id[:8]).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedGame(t *testing.T, gameID int32, title string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, now(), now())
		 ON CONFLICT (id) DO NOTHING`, gameID, title).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed game: %v", err)
	}
}

// seedUserGame inserts a user_games row (no platforms) and returns its id.
func seedUserGame(t *testing.T, userID string, gameID int32) string {
	t.Helper()
	seedGame(t, gameID, fmt.Sprintf("Game %d", gameID))
	id := uuid.NewString()
	_, err := testDB.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id) VALUES (?, ?, ?)`,
		id, userID, gameID).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed user_game: %v", err)
	}
	return id
}

// seedPlatform inserts a user_game_platforms row. Pass "" for a NULL platform or
// storefront. Use seeded names (pc-windows, steam) for non-null FK values.
func seedPlatform(t *testing.T, ugID, platform, storefront string) string {
	t.Helper()
	id := uuid.NewString()
	var p, s any
	if platform != "" {
		p = platform
	}
	if storefront != "" {
		s = storefront
	}
	_, err := testDB.NewRaw(
		`INSERT INTO user_game_platforms (id, user_game_id, platform, storefront) VALUES (?, ?, ?, ?)`,
		id, ugID, p, s).Exec(context.Background())
	if err != nil {
		t.Fatalf("seed platform: %v", err)
	}
	return id
}

func ignore(t *testing.T, userID, ugID, checkID string) {
	t.Helper()
	_, err := testDB.NewRaw(
		`INSERT INTO smell_ignores (id, user_id, user_game_id, check_id) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), userID, ugID, checkID).Exec(context.Background())
	if err != nil {
		t.Fatalf("ignore: %v", err)
	}
}

func flaggedIDs(items []FlaggedItem) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it.UserGameID] = true
	}
	return m
}
```

> `seedPlatform` uses `pc-windows`/`steam` for FK-valid rows. Empty string ⇒ NULL column (the `any`-typed `p`/`s` stay nil).

- [ ] **Step 4: Write the orphan-game test**

Create `internal/librarysmells/detectors_test.go`:

```go
package librarysmells

import (
	"context"
	"testing"
)

func TestDetectOrphanGame(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	orphan := seedUserGame(t, userID, 1)                     // no platform, not wishlisted → flags
	clean := seedUserGame(t, userID, 2)                      // has a platform → clean
	seedPlatform(t, clean, "pc-windows", "steam")
	wish := seedUserGame(t, userID, 3)                       // wishlisted, no platform → clean
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, wish).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ignored := seedUserGame(t, userID, 4)                    // orphan but dismissed → suppressed
	ignore(t, userID, ignored, "orphan-game")

	items, err := detectOrphanGame(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[orphan] {
		t.Error("orphan game should flag")
	}
	if got[clean] {
		t.Error("game with a platform must not flag")
	}
	if got[wish] {
		t.Error("wishlisted game with no platform must not flag")
	}
	if got[ignored] {
		t.Error("dismissed game must be suppressed")
	}
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/librarysmells/ -run TestDetectOrphanGame -v`
Expected: PASS (container starts, migrations apply, assertions hold).

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): engine registry + orphan-game detector"
```

---

### Task 4: Platform-row inconsistency detectors

Adds `storefront-less-platform`, `storefront-without-platform`, `missing-ownership-status`, `invalid-storefront-for-platform`.

**Files:**
- Modify: `internal/librarysmells/detectors.go` (4 vars + 4 funcs)
- Modify: `internal/librarysmells/registry.go` (append 4 vars to `Registry()` in order)
- Test: `internal/librarysmells/detectors_test.go`

**Interfaces:**
- Produces: `detectStorefrontLess`, `detectStorefrontWithoutPlatform`, `detectMissingOwnership`, `detectInvalidStorefront` and their `Check` vars.

- [ ] **Step 1: Write the failing tests**

Append to `internal/librarysmells/detectors_test.go`:

```go
func TestDetectStorefrontLess(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "") // storefront NULL → flags, suggests default
	clean := seedUserGame(t, userID, 2)
	seedPlatform(t, clean, "pc-windows", "steam")

	items, err := detectStorefrontLess(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !flaggedIDs(items)[flagged] {
		t.Fatal("storefront-less platform should flag")
	}
	if flaggedIDs(items)[clean] {
		t.Fatal("platform with a storefront must not flag")
	}
	// suggested_storefront comes from platforms.default_storefront (pc-windows seeds one).
	for _, it := range items {
		if it.UserGameID == flagged && it.SuggestedStorefront == nil {
			t.Error("expected a suggested storefront for pc-windows")
		}
	}
}

func TestDetectStorefrontWithoutPlatform(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "", "steam") // platform NULL, storefront set → flags
	clean := seedUserGame(t, userID, 2)
	seedPlatform(t, clean, "pc-windows", "steam")

	items, err := detectStorefrontWithoutPlatform(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !flaggedIDs(items)[flagged] {
		t.Fatal("storefront-without-platform should flag")
	}
	if flaggedIDs(items)[clean] {
		t.Fatal("complete row must not flag")
	}
}

func TestDetectMissingOwnership(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "steam") // ownership_status NULL by default → flags
	clean := seedUserGame(t, userID, 2)
	owned := seedPlatform(t, clean, "pc-windows", "steam")
	if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET ownership_status = 'owned' WHERE id = ?`, owned).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	items, err := detectMissingOwnership(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !flaggedIDs(items)[flagged] {
		t.Fatal("missing ownership_status should flag")
	}
	if flaggedIDs(items)[clean] {
		t.Fatal("row with ownership_status must not flag")
	}
}

func TestDetectInvalidStorefront(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	// pc-windows + nintendo-eshop is not a valid platform_storefronts pair.
	flagged := seedUserGame(t, userID, 1)
	seedPlatform(t, flagged, "pc-windows", "nintendo-eshop")
	clean := seedUserGame(t, userID, 2)
	seedPlatform(t, clean, "pc-windows", "steam") // valid pair
	nullRow := seedUserGame(t, userID, 3)
	seedPlatform(t, nullRow, "pc-windows", "") // NULL storefront → NOT this check's concern

	items, err := detectInvalidStorefront(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[flagged] {
		t.Error("invalid (platform, storefront) pair should flag")
	}
	if got[clean] {
		t.Error("valid pair must not flag")
	}
	if got[nullRow] {
		t.Error("NULL storefront must not flag here (covered by storefront-less)")
	}
}
```

> Verify the seed assumptions first: run `grep -n "pc-windows\|nintendo-eshop\|default_storefront" internal/db/migrations/20260620000001_baseline.up.sql` to confirm `pc-windows` has a `default_storefront` and that `(pc-windows, nintendo-eshop)` is **absent** from the `platform_storefronts` seed. If `pc-windows`+`nintendo-eshop` happens to be seeded as valid, pick any unseeded pair instead and adjust the test.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/librarysmells/ -run 'TestDetectStorefrontLess|TestDetectStorefrontWithoutPlatform|TestDetectMissingOwnership|TestDetectInvalidStorefront' -v`
Expected: FAIL — the four `detect*` funcs are undefined.

- [ ] **Step 3: Implement the four detectors**

Append to `internal/librarysmells/detectors.go`:

```go
var storefrontLessCheck = Check{
	ID:          "storefront-less-platform",
	Title:       "Storefront-less platform",
	Description: "A platform entry with no storefront recorded. Physical is a real choice — NULL means unknown provenance.",
	Tier:        TierInconsistency,
	Detect:      detectStorefrontLess,
}

func detectStorefrontLess(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront,
		        pl.default_storefront AS suggested_storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 LEFT JOIN platforms pl ON pl.name = p.platform
		 WHERE ug.user_id = ?
		   AND p.storefront IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "storefront-less-platform",
	).Scan(ctx, &items)
	return items, err
}

var storefrontWithoutPlatformCheck = Check{
	ID:          "storefront-without-platform",
	Title:       "Storefront without a platform",
	Description: "A storefront is recorded but no platform — the entry is half-filled.",
	Tier:        TierInconsistency,
	Detect:      detectStorefrontWithoutPlatform,
}

func detectStorefrontWithoutPlatform(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.storefront IS NOT NULL
		   AND p.platform IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "storefront-without-platform",
	).Scan(ctx, &items)
	return items, err
}

var missingOwnershipCheck = Check{
	ID:          "missing-ownership-status",
	Title:       "Missing ownership status",
	Description: "A platform entry with no ownership status (owned, borrowed, …).",
	Tier:        TierInconsistency,
	Detect:      detectMissingOwnership,
}

func detectMissingOwnership(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.ownership_status IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "missing-ownership-status",
	).Scan(ctx, &items)
	return items, err
}

var invalidStorefrontCheck = Check{
	ID:          "invalid-storefront-for-platform",
	Title:       "Invalid storefront for platform",
	Description: "The platform/storefront combination is not a recognised pairing.",
	Tier:        TierInconsistency,
	Detect:      detectInvalidStorefront,
}

func detectInvalidStorefront(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.platform IS NOT NULL
		   AND p.storefront IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM platform_storefronts ps
		                   WHERE ps.platform = p.platform AND ps.storefront = p.storefront)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "invalid-storefront-for-platform",
	).Scan(ctx, &items)
	return items, err
}
```

- [ ] **Step 4: Register the four checks**

In `internal/librarysmells/registry.go`, update `Registry()` to (epic order — these four slot in around orphan):

```go
func Registry() []Check {
	return []Check{
		storefrontLessCheck,
		orphanGameCheck,
		storefrontWithoutPlatformCheck,
		missingOwnershipCheck,
		invalidStorefrontCheck,
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/librarysmells/ -run 'TestDetectStorefrontLess|TestDetectStorefrontWithoutPlatform|TestDetectMissingOwnership|TestDetectInvalidStorefront' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): platform-row inconsistency detectors"
```

---

### Task 5: `impossible-acquired-date` detector

**Files:**
- Modify: `internal/librarysmells/detectors.go`
- Modify: `internal/librarysmells/registry.go`
- Test: `internal/librarysmells/detectors_test.go`

**Interfaces:**
- Produces: `detectImpossibleAcquiredDate` + `impossibleAcquiredDateCheck`.

- [ ] **Step 1: Write the failing test**

Append to `internal/librarysmells/detectors_test.go`:

```go
func TestDetectImpossibleAcquiredDate(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	setAcquired := func(platformID, date string) {
		if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET acquired_date = ?::date WHERE id = ?`, date, platformID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	setRelease := func(gameID int32, date string) {
		if _, err := testDB.NewRaw(`UPDATE games SET release_date = ?::date WHERE id = ?`, date, gameID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// future acquired date → flags
	future := seedUserGame(t, userID, 1)
	setAcquired(seedPlatform(t, future, "pc-windows", "steam"), "2099-01-01")

	// acquired before release → flags
	preRelease := seedUserGame(t, userID, 2)
	setRelease(2, "2020-01-01")
	setAcquired(seedPlatform(t, preRelease, "pc-windows", "steam"), "2019-01-01")

	// acquired after release, not future → clean
	ok := seedUserGame(t, userID, 3)
	setRelease(3, "2020-01-01")
	setAcquired(seedPlatform(t, ok, "pc-windows", "steam"), "2021-01-01")

	// acquired before "now" but game has no release date → clean (only future arm applies)
	noRelease := seedUserGame(t, userID, 4)
	setAcquired(seedPlatform(t, noRelease, "pc-windows", "steam"), "2010-01-01")

	items, err := detectImpossibleAcquiredDate(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[future] {
		t.Error("future acquired date should flag")
	}
	if !got[preRelease] {
		t.Error("acquired-before-release should flag")
	}
	if got[ok] {
		t.Error("valid acquired date must not flag")
	}
	if got[noRelease] {
		t.Error("old date with no release_date must not flag")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/librarysmells/ -run TestDetectImpossibleAcquiredDate -v`
Expected: FAIL — `detectImpossibleAcquiredDate` undefined.

- [ ] **Step 3: Implement the detector**

Append to `internal/librarysmells/detectors.go`:

```go
var impossibleAcquiredDateCheck = Check{
	ID:          "impossible-acquired-date",
	Title:       "Impossible acquired date",
	Description: "An acquired date in the future, or before the game was released.",
	Tier:        TierInconsistency,
	Detect:      detectImpossibleAcquiredDate,
}

func detectImpossibleAcquiredDate(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        p.id AS platform_row_id, p.platform, p.storefront,
		        CASE
		          WHEN p.acquired_date > now()::date THEN 'acquired date is in the future'
		          ELSE 'acquired before the game was released'
		        END AS detail
		 FROM user_game_platforms p
		 JOIN user_games ug ON ug.id = p.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND p.acquired_date IS NOT NULL
		   AND (p.acquired_date > now()::date
		        OR (g.release_date IS NOT NULL AND p.acquired_date < g.release_date))
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "impossible-acquired-date",
	).Scan(ctx, &items)
	return items, err
}
```

- [ ] **Step 4: Register it**

In `internal/librarysmells/registry.go`, insert `impossibleAcquiredDateCheck` after `missingOwnershipCheck` and before `invalidStorefrontCheck` (epic order #6 then #11):

```go
func Registry() []Check {
	return []Check{
		storefrontLessCheck,
		orphanGameCheck,
		storefrontWithoutPlatformCheck,
		missingOwnershipCheck,
		impossibleAcquiredDateCheck,
		invalidStorefrontCheck,
	}
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/librarysmells/ -run TestDetectImpossibleAcquiredDate -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): impossible-acquired-date detector"
```

---

### Task 6: `wishlisted-yet-owned` detector

**Files:**
- Modify: `internal/librarysmells/detectors.go`
- Modify: `internal/librarysmells/registry.go`
- Test: `internal/librarysmells/detectors_test.go`

**Interfaces:**
- Produces: `detectWishlistedYetOwned` + `wishlistedYetOwnedCheck` (Apply wired in Task 8).

- [ ] **Step 1: Write the failing test**

Append to `internal/librarysmells/detectors_test.go`:

```go
func TestDetectWishlistedYetOwned(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	setWish := func(ugID string) {
		if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, ugID).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	flagged := seedUserGame(t, userID, 1) // wishlisted + has a platform → flags
	setWish(flagged)
	seedPlatform(t, flagged, "pc-windows", "steam")

	pureWish := seedUserGame(t, userID, 2) // wishlisted, no platform → clean
	setWish(pureWish)

	owned := seedUserGame(t, userID, 3) // owned, not wishlisted → clean
	seedPlatform(t, owned, "pc-windows", "steam")

	items, err := detectWishlistedYetOwned(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[flagged] {
		t.Error("wishlisted-yet-owned should flag")
	}
	if got[pureWish] {
		t.Error("pure wishlist entry must not flag")
	}
	if got[owned] {
		t.Error("owned non-wishlisted game must not flag")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/librarysmells/ -run TestDetectWishlistedYetOwned -v`
Expected: FAIL — `detectWishlistedYetOwned` undefined.

- [ ] **Step 3: Implement the detector**

Append to `internal/librarysmells/detectors.go`:

```go
var wishlistedYetOwnedCheck = Check{
	ID:          "wishlisted-yet-owned",
	Title:       "Wishlisted yet owned",
	Description: "Still on your wishlist even though it's already in your library.",
	Tier:        TierInconsistency,
	Detect:      detectWishlistedYetOwned,
}

func detectWishlistedYetOwned(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.is_wishlisted = true
		   AND EXISTS (SELECT 1 FROM user_game_platforms p WHERE p.user_game_id = ug.id)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "wishlisted-yet-owned",
	).Scan(ctx, &items)
	return items, err
}
```

- [ ] **Step 4: Register it**

In `internal/librarysmells/registry.go`, insert `wishlistedYetOwnedCheck` after `storefrontWithoutPlatformCheck` (epic order #4):

```go
func Registry() []Check {
	return []Check{
		storefrontLessCheck,
		orphanGameCheck,
		storefrontWithoutPlatformCheck,
		wishlistedYetOwnedCheck,
		missingOwnershipCheck,
		impossibleAcquiredDateCheck,
		invalidStorefrontCheck,
	}
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/librarysmells/ -run TestDetectWishlistedYetOwned -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): wishlisted-yet-owned detector"
```

---

### Task 7: Nudge detectors (#7–#10) with precedence + HLTB silence

Adds `beat-but-not-marked`, `played-but-not-started`, `in-progress-untouched`, `unrated-after-finishing`.

**Files:**
- Modify: `internal/librarysmells/detectors.go`
- Modify: `internal/librarysmells/registry.go`
- Test: `internal/librarysmells/detectors_test.go`

**Interfaces:**
- Produces: `detectBeatButNotMarked`, `detectPlayedButNotStarted`, `detectInProgressUntouched`, `detectUnratedAfterFinishing` + their `Check` vars (Apply for the first three wired in Task 8).

- [ ] **Step 1: Write the failing tests**

Append to `internal/librarysmells/detectors_test.go`:

```go
// platformWithHours seeds a platform row carrying hours_played.
func platformWithHours(t *testing.T, ugID string, hours float64) {
	t.Helper()
	id := seedPlatform(t, ugID, "pc-windows", "steam")
	if _, err := testDB.NewRaw(`UPDATE user_game_platforms SET hours_played = ? WHERE id = ?`, hours, id).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func setStatus(t *testing.T, ugID, status string) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE user_games SET play_status = ? WHERE id = ?`, status, ugID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func setHLTB(t *testing.T, gameID int32, hours float64) {
	t.Helper()
	if _, err := testDB.NewRaw(`UPDATE games SET howlongtobeat_main = ? WHERE id = ?`, hours, gameID).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectBeatButNotMarked(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	beaten := seedUserGame(t, userID, 1) // 12h played, HLTB 10, in_progress → flags
	setStatus(t, beaten, "in_progress")
	setHLTB(t, 1, 10)
	platformWithHours(t, beaten, 12)

	noHLTB := seedUserGame(t, userID, 2) // lots of hours, HLTB NULL → silent
	setStatus(t, noHLTB, "in_progress")
	platformWithHours(t, noHLTB, 99)

	finished := seedUserGame(t, userID, 3) // already completed → not flagged
	setStatus(t, finished, "completed")
	setHLTB(t, 3, 10)
	platformWithHours(t, finished, 12)

	items, err := detectBeatButNotMarked(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[beaten] {
		t.Error("beaten in-progress game should flag")
	}
	if got[noHLTB] {
		t.Error("game with NULL howlongtobeat_main must stay silent")
	}
	if got[finished] {
		t.Error("already-completed game must not flag")
	}
}

func TestDetectPlayedButNotStarted_Precedence(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	// not_started, 2h, no HLTB → played-but-not-started
	played := seedUserGame(t, userID, 1)
	setStatus(t, played, "not_started")
	platformWithHours(t, played, 2)

	// not_started, 12h, HLTB 10 → belongs to beat-but-not-marked, NOT this check
	beaten := seedUserGame(t, userID, 2)
	setStatus(t, beaten, "not_started")
	setHLTB(t, 2, 10)
	platformWithHours(t, beaten, 12)

	// not_started, 0.2h → below threshold, clean
	barely := seedUserGame(t, userID, 3)
	setStatus(t, barely, "not_started")
	platformWithHours(t, barely, 0.2)

	items, err := detectPlayedButNotStarted(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[played] {
		t.Error("played not_started game should flag")
	}
	if got[beaten] {
		t.Error("beaten game must defer to beat-but-not-marked (precedence)")
	}
	if got[barely] {
		t.Error("under-0.5h game must not flag")
	}
}

func TestDetectInProgressUntouched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	untouched := seedUserGame(t, userID, 1) // in_progress, no platform hours → flags
	setStatus(t, untouched, "in_progress")
	seedPlatform(t, untouched, "pc-windows", "steam") // hours_played NULL

	touched := seedUserGame(t, userID, 2) // in_progress, 3h → clean
	setStatus(t, touched, "in_progress")
	platformWithHours(t, touched, 3)

	items, err := detectInProgressUntouched(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[untouched] {
		t.Error("in_progress with 0 hours should flag")
	}
	if got[touched] {
		t.Error("in_progress with hours must not flag")
	}
}

func TestDetectUnratedAfterFinishing(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	unrated := seedUserGame(t, userID, 1) // completed, no rating → flags
	setStatus(t, unrated, "completed")

	rated := seedUserGame(t, userID, 2) // completed, rated → clean
	setStatus(t, rated, "completed")
	if _, err := testDB.NewRaw(`UPDATE user_games SET personal_rating = 8 WHERE id = ?`, rated).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	dropped := seedUserGame(t, userID, 3) // dropped is NOT in this check's finished set → clean
	setStatus(t, dropped, "dropped")

	items, err := detectUnratedAfterFinishing(ctx, testDB, userID)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	got := flaggedIDs(items)
	if !got[unrated] {
		t.Error("completed unrated game should flag")
	}
	if got[rated] {
		t.Error("rated game must not flag")
	}
	if got[dropped] {
		t.Error("dropped game must not flag (only completed/mastered/dominated)")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/librarysmells/ -run 'TestDetectBeatButNotMarked|TestDetectPlayedButNotStarted_Precedence|TestDetectInProgressUntouched|TestDetectUnratedAfterFinishing' -v`
Expected: FAIL — the four `detect*` funcs are undefined.

- [ ] **Step 3: Implement the four detectors**

Append to `internal/librarysmells/detectors.go`:

```go
var beatButNotMarkedCheck = Check{
	ID:          "beat-but-not-marked",
	Title:       "Beat but not marked",
	Description: "You've played past its time-to-beat but it isn't marked completed.",
	Tier:        TierNudge,
	Detect:      detectBeatButNotMarked,
}

func detectBeatButNotMarked(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        'completed' AS suggested_status
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND g.howlongtobeat_main IS NOT NULL
		   AND ug.play_status IN ('not_started', 'in_progress')
		   AND (SELECT COALESCE(SUM(p.hours_played), 0) FROM user_game_platforms p
		        WHERE p.user_game_id = ug.id) >= g.howlongtobeat_main
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "beat-but-not-marked",
	).Scan(ctx, &items)
	return items, err
}

var playedButNotStartedCheck = Check{
	ID:          "played-but-not-started",
	Title:       "Played but \"not started\"",
	Description: "Marked not-started even though it has playtime.",
	Tier:        TierNudge,
	Detect:      detectPlayedButNotStarted,
}

func detectPlayedButNotStarted(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        'in_progress' AS suggested_status
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.play_status = 'not_started'
		   AND (SELECT COALESCE(SUM(p.hours_played), 0) FROM user_game_platforms p
		        WHERE p.user_game_id = ug.id) >= 0.5
		   AND NOT (g.howlongtobeat_main IS NOT NULL
		            AND (SELECT COALESCE(SUM(p.hours_played), 0) FROM user_game_platforms p
		                 WHERE p.user_game_id = ug.id) >= g.howlongtobeat_main)
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "played-but-not-started",
	).Scan(ctx, &items)
	return items, err
}

var inProgressUntouchedCheck = Check{
	ID:          "in-progress-untouched",
	Title:       "In progress but never touched",
	Description: "Marked in-progress but has no recorded playtime.",
	Tier:        TierNudge,
	Detect:      detectInProgressUntouched,
}

func detectInProgressUntouched(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url,
		        'not_started' AS suggested_status
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.play_status = 'in_progress'
		   AND (SELECT COALESCE(SUM(p.hours_played), 0) FROM user_game_platforms p
		        WHERE p.user_game_id = ug.id) = 0
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "in-progress-untouched",
	).Scan(ctx, &items)
	return items, err
}

var unratedAfterFinishingCheck = Check{
	ID:          "unrated-after-finishing",
	Title:       "Unrated after finishing",
	Description: "Finished but you never gave it a personal rating.",
	Tier:        TierNudge,
	Detect:      detectUnratedAfterFinishing,
}

func detectUnratedAfterFinishing(ctx context.Context, db *bun.DB, userID string) ([]FlaggedItem, error) {
	var items []FlaggedItem
	err := db.NewRaw(
		`SELECT ug.id AS user_game_id, ug.game_id, g.title, g.cover_art_url
		 FROM user_games ug
		 JOIN games g ON g.id = ug.game_id
		 WHERE ug.user_id = ?
		   AND ug.play_status IN ('completed', 'mastered', 'dominated')
		   AND ug.personal_rating IS NULL
		   AND NOT EXISTS (SELECT 1 FROM smell_ignores si
		                   WHERE si.user_id = ug.user_id AND si.user_game_id = ug.id AND si.check_id = ?)
		 ORDER BY g.title`,
		userID, "unrated-after-finishing",
	).Scan(ctx, &items)
	return items, err
}
```

- [ ] **Step 4: Register the four (completes the full epic-ordered registry)**

In `internal/librarysmells/registry.go`, the final `Registry()`:

```go
func Registry() []Check {
	return []Check{
		storefrontLessCheck,
		orphanGameCheck,
		storefrontWithoutPlatformCheck,
		wishlistedYetOwnedCheck,
		missingOwnershipCheck,
		impossibleAcquiredDateCheck,
		invalidStorefrontCheck,
		beatButNotMarkedCheck,
		playedButNotStartedCheck,
		inProgressUntouchedCheck,
		unratedAfterFinishingCheck,
	}
}
```

- [ ] **Step 5: Run to verify they pass + the registry is complete**

Run: `go test ./internal/librarysmells/ -v`
Expected: PASS for all detector tests. Also add and run this guard test (append to `detectors_test.go`):

```go
func TestRegistryComplete(t *testing.T) {
	if len(Registry()) != 11 {
		t.Fatalf("expected 11 checks, got %d", len(Registry()))
	}
	seen := map[string]bool{}
	for _, c := range Registry() {
		if c.ID == "" || c.Title == "" || c.Description == "" {
			t.Errorf("check %q missing metadata", c.ID)
		}
		if seen[c.ID] {
			t.Errorf("duplicate check id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Tier != TierInconsistency && c.Tier != TierNudge {
			t.Errorf("check %q has invalid tier %q", c.ID, c.Tier)
		}
		if c.Detect == nil {
			t.Errorf("check %q has nil Detect", c.ID)
		}
	}
}
```

Run: `go test ./internal/librarysmells/ -run TestRegistryComplete -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): nudge detectors with precedence and HLTB silence"
```

---

### Task 8: Apply layer (4 auto-fixes) with re-validation

**Files:**
- Create: `internal/librarysmells/apply.go`
- Modify: `internal/librarysmells/detectors.go` (set `AutoFixable: true, Apply: …` on the 4 auto-fix check vars)
- Test: `internal/librarysmells/apply_test.go`

**Interfaces:**
- Consumes: `usergame.ClearWishlist` (Task 2), `usergame.SetPlayStatusBulk`, `enum.PlayStatus*`.
- Produces: `revalidate(ctx, db, userID, ids, detect) ([]string, int, error)` and `applyClearWishlist`/`applyBeatButNotMarked`/`applyPlayedButNotStarted`/`applyInProgressUntouched`, each `func(ctx, db, userID, ids) (applied, skipped int, err error)`. After this task `Lookup("beat-but-not-marked").AutoFixable == true` and `.Apply != nil` (same for the other three).

- [ ] **Step 1: Write the failing tests**

Create `internal/librarysmells/apply_test.go`:

```go
package librarysmells

import (
	"context"
	"testing"
)

func playStatusOf(t *testing.T, ugID string) string {
	t.Helper()
	var s *string
	if err := testDB.NewRaw(`SELECT play_status FROM user_games WHERE id = ?`, ugID).Scan(context.Background(), &s); err != nil {
		t.Fatalf("playStatusOf: %v", err)
	}
	if s == nil {
		return ""
	}
	return *s
}

func TestApplyBeatButNotMarked(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	setStatus(t, flagged, "in_progress")
	setHLTB(t, 1, 10)
	platformWithHours(t, flagged, 12)

	// Not flagged (under HLTB): apply must skip it, never touch its status.
	notFlagged := seedUserGame(t, userID, 2)
	setStatus(t, notFlagged, "in_progress")
	setHLTB(t, 2, 100)
	platformWithHours(t, notFlagged, 3)

	check, _ := Lookup("beat-but-not-marked")
	if !check.AutoFixable || check.Apply == nil {
		t.Fatal("beat-but-not-marked must be auto-fixable")
	}
	applied, skipped, err := check.Apply(ctx, testDB, userID, []string{flagged, notFlagged})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 1 || skipped != 1 {
		t.Fatalf("expected applied=1 skipped=1, got applied=%d skipped=%d", applied, skipped)
	}
	if got := playStatusOf(t, flagged); got != "completed" {
		t.Errorf("flagged game should be completed, got %q", got)
	}
	if got := playStatusOf(t, notFlagged); got != "in_progress" {
		t.Errorf("stale id must be untouched, got %q", got)
	}
}

func TestApplyClearWishlist(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	if _, err := testDB.NewRaw(`UPDATE user_games SET is_wishlisted = true WHERE id = ?`, flagged).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	seedPlatform(t, flagged, "pc-windows", "steam")

	check, _ := Lookup("wishlisted-yet-owned")
	if !check.AutoFixable || check.Apply == nil {
		t.Fatal("wishlisted-yet-owned must be auto-fixable")
	}
	applied, skipped, err := check.Apply(ctx, testDB, userID, []string{flagged})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 1 || skipped != 0 {
		t.Fatalf("expected applied=1 skipped=0, got applied=%d skipped=%d", applied, skipped)
	}
	var wl bool
	if err := testDB.NewRaw(`SELECT is_wishlisted FROM user_games WHERE id = ?`, flagged).Scan(ctx, &wl); err != nil {
		t.Fatal(err)
	}
	if wl {
		t.Error("wishlist flag should be cleared")
	}
}

func TestApplyInProgressUntouched(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()
	userID := seedUser(t)

	flagged := seedUserGame(t, userID, 1)
	setStatus(t, flagged, "in_progress")
	seedPlatform(t, flagged, "pc-windows", "steam") // 0 hours

	check, _ := Lookup("in-progress-untouched")
	applied, skipped, err := check.Apply(ctx, testDB, userID, []string{flagged})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied != 1 || skipped != 0 {
		t.Fatalf("expected applied=1 skipped=0, got applied=%d skipped=%d", applied, skipped)
	}
	if got := playStatusOf(t, flagged); got != "not_started" {
		t.Errorf("expected not_started, got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/librarysmells/ -run 'TestApply' -v`
Expected: FAIL — the 4 check vars are not auto-fixable yet (`check.Apply == nil`).

- [ ] **Step 3: Implement the apply layer**

Create `internal/librarysmells/apply.go`:

```go
package librarysmells

import (
	"context"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/enum"
	"github.com/drzero42/nexorious/internal/usergame"
)

// revalidate runs the check's Detect and returns the subset of userGameIDs that
// are still flagged (de-duplicated), plus the count of requested ids that are
// not flagged. This makes Apply safe against a stale client and idempotent.
func revalidate(
	ctx context.Context, db *bun.DB, userID string, userGameIDs []string,
	detect func(context.Context, *bun.DB, string) ([]FlaggedItem, error),
) (subset []string, skipped int, err error) {
	flagged, err := detect(ctx, db, userID)
	if err != nil {
		return nil, 0, err
	}
	valid := flaggedIDSet(flagged)
	seen := make(map[string]bool, len(userGameIDs))
	for _, id := range userGameIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if valid[id] {
			subset = append(subset, id)
		}
	}
	return subset, len(seen) - len(subset), nil
}

func flaggedIDSet(items []FlaggedItem) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it.UserGameID] = true
	}
	return m
}

func applyStatus(
	ctx context.Context, db *bun.DB, userID string, ids []string,
	status enum.PlayStatus, detect func(context.Context, *bun.DB, string) ([]FlaggedItem, error),
) (applied, skipped int, err error) {
	subset, skipped, err := revalidate(ctx, db, userID, ids, detect)
	if err != nil {
		return 0, 0, err
	}
	if len(subset) == 0 {
		return 0, skipped, nil
	}
	n, err := usergame.SetPlayStatusBulk(ctx, db, usergame.BulkStatusParams{
		UserID:      userID,
		UserGameIDs: subset,
		PlayStatus:  string(status),
	})
	return n, skipped, err
}

func applyClearWishlist(ctx context.Context, db *bun.DB, userID string, ids []string) (applied, skipped int, err error) {
	subset, skipped, err := revalidate(ctx, db, userID, ids, detectWishlistedYetOwned)
	if err != nil {
		return 0, 0, err
	}
	if len(subset) == 0 {
		return 0, skipped, nil
	}
	n, err := usergame.ClearWishlist(ctx, db, userID, subset)
	return n, skipped, err
}

func applyBeatButNotMarked(ctx context.Context, db *bun.DB, userID string, ids []string) (int, int, error) {
	return applyStatus(ctx, db, userID, ids, enum.PlayStatusCompleted, detectBeatButNotMarked)
}

func applyPlayedButNotStarted(ctx context.Context, db *bun.DB, userID string, ids []string) (int, int, error) {
	return applyStatus(ctx, db, userID, ids, enum.PlayStatusInProgress, detectPlayedButNotStarted)
}

func applyInProgressUntouched(ctx context.Context, db *bun.DB, userID string, ids []string) (int, int, error) {
	return applyStatus(ctx, db, userID, ids, enum.PlayStatusNotStarted, detectInProgressUntouched)
}
```

> `flaggedIDs` already exists in `main_test.go` (test-only); `flaggedIDSet` here is the production copy so `apply.go` does not depend on test code.

- [ ] **Step 4: Wire Apply into the 4 check vars**

In `internal/librarysmells/detectors.go`, edit the four auto-fix check vars to set `AutoFixable` + `Apply`:

```go
var wishlistedYetOwnedCheck = Check{
	ID:          "wishlisted-yet-owned",
	Title:       "Wishlisted yet owned",
	Description: "Still on your wishlist even though it's already in your library.",
	Tier:        TierInconsistency,
	AutoFixable: true,
	Detect:      detectWishlistedYetOwned,
	Apply:       applyClearWishlist,
}
```

```go
var beatButNotMarkedCheck = Check{
	ID:          "beat-but-not-marked",
	Title:       "Beat but not marked",
	Description: "You've played past its time-to-beat but it isn't marked completed.",
	Tier:        TierNudge,
	AutoFixable: true,
	Detect:      detectBeatButNotMarked,
	Apply:       applyBeatButNotMarked,
}
```

```go
var playedButNotStartedCheck = Check{
	ID:          "played-but-not-started",
	Title:       "Played but \"not started\"",
	Description: "Marked not-started even though it has playtime.",
	Tier:        TierNudge,
	AutoFixable: true,
	Detect:      detectPlayedButNotStarted,
	Apply:       applyPlayedButNotStarted,
}
```

```go
var inProgressUntouchedCheck = Check{
	ID:          "in-progress-untouched",
	Title:       "In progress but never touched",
	Description: "Marked in-progress but has no recorded playtime.",
	Tier:        TierNudge,
	AutoFixable: true,
	Detect:      detectInProgressUntouched,
	Apply:       applyInProgressUntouched,
}
```

- [ ] **Step 5: Run to verify they pass**

Run: `go test ./internal/librarysmells/ -run 'TestApply' -v`
Expected: PASS (all three apply tests).

- [ ] **Step 6: Commit**

```bash
git add internal/librarysmells/
git commit -m "feat(smells): auto-fix apply layer with re-validation"
```

---

### Task 9: REST API handler + routes

**Files:**
- Create: `internal/api/library_smells.go`
- Modify: `internal/api/router.go` (register the group)
- Test: `internal/api/library_smells_test.go`

**Interfaces:**
- Consumes: `librarysmells.Registry()`/`Lookup()`, `librarysmells.FlaggedItem`, `auth.UserIDFromContext`, `models.SmellIgnore`.
- Produces: `LibrarySmellsHandler` with `HandleSummary`, `HandleList`, `HandleApply`, `HandleIgnore`, `HandleRestore`, `HandleListIgnored`.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/library_smells_test.go`. Reuse the existing helpers (`insertAuthTestUser`, `loginAndGetToken`, `getAuth`, `postJSONAuth`, `insertTestGame`, `insertTestUserGame`, `insertTestUserGamePlatform`, `newTestEcho`, `testCfg`, `truncateAllTables`):

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func setupSmellsUser(t *testing.T, suffix string) (string, string, interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}) {
	t.Helper()
	cfg := testCfg()
	e := newTestEcho(t, testDB, cfg)
	userID := "u-smell-" + suffix
	insertAuthTestUser(t, testDB, userID, "smelluser-"+suffix, "pass123", true, false)
	token := loginAndGetToken(t, e, "smelluser-"+suffix, "pass123")
	return userID, token, e
}

func TestSmellsSummary(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "summary")

	// One orphan game (no platform, not wishlisted).
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	rec := getAuth(t, e, "/api/library/smells", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var summary []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summary) != 11 {
		t.Fatalf("expected 11 checks, got %d", len(summary))
	}
	for _, s := range summary {
		if s["id"] == "orphan-game" {
			if int(s["count"].(float64)) != 1 {
				t.Fatalf("expected orphan count 1, got %v", s["count"])
			}
			return
		}
	}
	t.Fatal("orphan-game check missing from summary")
}

func TestSmellsListAndUnknownCheck(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "list")
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	rec := getAuth(t, e, "/api/library/smells/orphan-game", token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got total=%d len=%d", resp.Total, len(resp.Items))
	}
	if resp.Items == nil {
		t.Fatal("items must be [] not null")
	}

	// Unknown check → 404.
	rec404 := getAuth(t, e, "/api/library/smells/not-a-check", token)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown check, got %d", rec404.Code)
	}
}

func TestSmellsApplyNotAutoFixable(t *testing.T) {
	truncateAllTables(t)
	_, token, e := setupSmellsUser(t, "apply422")
	rec := postJSONAuth(t, e, "/api/library/smells/orphan-game/apply",
		map[string]any{"user_game_ids": []string{"x"}}, token)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-auto-fixable check, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSmellsIgnoreRestoreAndListDismissed(t *testing.T) {
	truncateAllTables(t)
	userID, token, e := setupSmellsUser(t, "ignore")
	gid := insertTestGame(t, testDB, "Orphan")
	insertTestUserGame(t, testDB, "ug-orphan", userID, int(gid))

	// Ignore it → listing drops to 0, dismissed listing shows 1.
	recIg := postJSONAuth(t, e, "/api/library/smells/orphan-game/ignore",
		map[string]any{"user_game_ids": []string{"ug-orphan"}}, token)
	if recIg.Code != http.StatusOK {
		t.Fatalf("ignore: expected 200, got %d: %s", recIg.Code, recIg.Body.String())
	}

	recList := getAuth(t, e, "/api/library/smells/orphan-game", token)
	var listResp struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	if listResp.Total != 0 {
		t.Fatalf("expected 0 flagged after ignore, got %d", listResp.Total)
	}

	recDismissed := getAuth(t, e, "/api/library/smells/orphan-game/ignored", token)
	var dis struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recDismissed.Body.Bytes(), &dis)
	if dis.Total != 1 {
		t.Fatalf("expected 1 dismissed, got %d", dis.Total)
	}

	// Restore → flagged again.
	recRestore := deleteJSONAuth(t, e, "/api/library/smells/orphan-game/ignore",
		map[string]any{"user_game_ids": []string{"ug-orphan"}}, token)
	if recRestore.Code != http.StatusOK {
		t.Fatalf("restore: expected 200, got %d: %s", recRestore.Code, recRestore.Body.String())
	}
	recList2 := getAuth(t, e, "/api/library/smells/orphan-game", token)
	var l2 struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(recList2.Body.Bytes(), &l2)
	if l2.Total != 1 {
		t.Fatalf("expected 1 flagged after restore, got %d", l2.Total)
	}
	_ = context.Background
}
```

> If a `deleteJSONAuth` helper does not already exist in the api test package, grep first (`grep -rn "func deleteJSONAuth\|MethodDelete" internal/api/*_test.go`); add a small one mirroring `postJSONAuth` but with `http.MethodDelete` if missing.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/api/ -run 'TestSmells' -v`
Expected: FAIL — routes 404 (group not registered) / handler undefined.

- [ ] **Step 3: Implement the handler**

Create `internal/api/library_smells.go`:

```go
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/auth"
	"github.com/drzero42/nexorious/internal/librarysmells"
	"github.com/drzero42/nexorious/internal/usergame"
)

// LibrarySmellsHandler serves the /api/library/smells endpoints.
type LibrarySmellsHandler struct {
	db *bun.DB
}

// NewLibrarySmellsHandler returns a new LibrarySmellsHandler.
func NewLibrarySmellsHandler(db *bun.DB) *LibrarySmellsHandler {
	return &LibrarySmellsHandler{db: db}
}

type smellSummaryItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tier        string `json:"tier"`
	AutoFixable bool   `json:"auto_fixable"`
	Count       int    `json:"count"`
}

type flaggedListResponse struct {
	Items   []librarysmells.FlaggedItem `json:"items"`
	Total   int                         `json:"total"`
	Page    int                         `json:"page"`
	PerPage int                         `json:"per_page"`
	Pages   int                         `json:"pages"`
}

type idsRequest struct {
	UserGameIDs []string `json:"user_game_ids"`
}

func smellsUserID(c *echo.Context) (string, error) {
	userID := auth.UserIDFromContext(c)
	if userID == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	return userID, nil
}

func paginate[T any](items []T, page, perPage int) ([]T, int) {
	total := len(items)
	start := (page - 1) * perPage
	if start >= total {
		return []T{}, total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return items[start:end], total
}

func parsePageParams(c *echo.Context) (page, perPage int) {
	page, perPage = 1, 25
	if p := c.QueryParam("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 1 {
			page = v
		}
	}
	if pp := c.QueryParam("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v >= 1 && v <= 200 {
			perPage = v
		}
	}
	return page, perPage
}

// HandleSummary: GET /api/library/smells — per-check counts (post-ignore).
func (h *LibrarySmellsHandler) HandleSummary(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	out := make([]smellSummaryItem, 0, len(librarysmells.Registry()))
	for _, check := range librarysmells.Registry() {
		items, err := check.Detect(ctx, h.db, userID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "detect failed")
		}
		out = append(out, smellSummaryItem{
			ID:          check.ID,
			Title:       check.Title,
			Description: check.Description,
			Tier:        string(check.Tier),
			AutoFixable: check.AutoFixable,
			Count:       len(items),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// HandleList: GET /api/library/smells/:checkID — paginated flagged items.
func (h *LibrarySmellsHandler) HandleList(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.PathParam("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	ctx := c.Request().Context()
	items, err := check.Detect(ctx, h.db, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "detect failed")
	}
	if items == nil {
		items = []librarysmells.FlaggedItem{}
	}
	page, perPage := parsePageParams(c)
	pageItems, total := paginate(items, page, perPage)
	pages := (total + perPage - 1) / perPage
	return c.JSON(http.StatusOK, flaggedListResponse{
		Items: pageItems, Total: total, Page: page, PerPage: perPage, Pages: pages,
	})
}

// HandleApply: POST /api/library/smells/:checkID/apply.
func (h *LibrarySmellsHandler) HandleApply(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.PathParam("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	if !check.AutoFixable || check.Apply == nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "check is not auto-fixable")
	}
	var req idsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	applied, skipped, err := check.Apply(c.Request().Context(), h.db, userID, req.UserGameIDs)
	if err != nil {
		if errors.Is(err, usergame.ErrValidation) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "apply failed")
	}
	return c.JSON(http.StatusOK, map[string]int{"applied": applied, "skipped": skipped})
}

// HandleIgnore: POST /api/library/smells/:checkID/ignore.
func (h *LibrarySmellsHandler) HandleIgnore(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.PathParam("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var req idsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	ctx := c.Request().Context()
	var ignored int
	for _, ugID := range req.UserGameIDs {
		// Insert only if the game belongs to the user; idempotent on conflict.
		res, err := h.db.NewRaw(
			`INSERT INTO smell_ignores (id, user_id, user_game_id, check_id)
			 SELECT ?, ?, ug.id, ?
			 FROM user_games ug WHERE ug.id = ? AND ug.user_id = ?
			 ON CONFLICT (user_id, user_game_id, check_id) DO NOTHING`,
			uuid.NewString(), userID, check.ID, ugID, userID,
		).Exec(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "ignore failed")
		}
		if n, _ := res.RowsAffected(); n > 0 { //nolint:errcheck // RowsAffected advisory; count only
			ignored++
		}
	}
	return c.JSON(http.StatusOK, map[string]int{"ignored": ignored})
}

// HandleRestore: DELETE /api/library/smells/:checkID/ignore.
func (h *LibrarySmellsHandler) HandleRestore(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.PathParam("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var req idsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}
	if len(req.UserGameIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "user_game_ids must be a non-empty array")
	}
	res, err := h.db.NewRaw(
		`DELETE FROM smell_ignores
		 WHERE user_id = ? AND check_id = ? AND user_game_id IN (?)`,
		userID, check.ID, bun.List(req.UserGameIDs),
	).Exec(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "restore failed")
	}
	n, _ := res.RowsAffected() //nolint:errcheck // RowsAffected advisory; count only
	return c.JSON(http.StatusOK, map[string]int{"restored": int(n)})
}

type ignoredItem struct {
	UserGameID string `bun:"user_game_id" json:"user_game_id"`
	Title      string `bun:"title"        json:"title"`
	CreatedAt  string `bun:"created_at"   json:"created_at"`
}

type ignoredListResponse struct {
	Items   []ignoredItem `json:"items"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
	Pages   int           `json:"pages"`
}

// HandleListIgnored: GET /api/library/smells/:checkID/ignored.
func (h *LibrarySmellsHandler) HandleListIgnored(c *echo.Context) error {
	userID, err := smellsUserID(c)
	if err != nil {
		return err
	}
	check, ok := librarysmells.Lookup(c.PathParam("checkID"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown check")
	}
	var items []ignoredItem
	err = h.db.NewRaw(
		`SELECT si.user_game_id, g.title, si.created_at::text AS created_at
		 FROM smell_ignores si
		 JOIN user_games ug ON ug.id = si.user_game_id
		 JOIN games g ON g.id = ug.game_id
		 WHERE si.user_id = ? AND si.check_id = ?
		 ORDER BY si.created_at DESC`,
		userID, check.ID,
	).Scan(c.Request().Context(), &items)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "list dismissed failed")
	}
	if items == nil {
		items = []ignoredItem{}
	}
	page, perPage := parsePageParams(c)
	pageItems, total := paginate(items, page, perPage)
	pages := (total + perPage - 1) / perPage
	return c.JSON(http.StatusOK, ignoredListResponse{
		Items: pageItems, Total: total, Page: page, PerPage: perPage, Pages: pages,
	})
}
```

> **Verify the Echo v5 path-param accessor before building.** This codebase's other handlers read route params — grep `grep -n "Param(\|PathParam(" internal/api/tags.go internal/api/pools.go` and use whichever the codebase uses (`c.PathParam("checkID")` in Echo v5, or `c.Param("checkID")`). Replace all `c.PathParam(...)` above to match. Likewise confirm `c.Bind(&req)` is the binding call used elsewhere.

- [ ] **Step 4: Register the routes**

In `internal/api/router.go`, after the pools group (around line 351), add:

```go
		// Library Smells routes (#1144). Static "" before "/:checkID"; the
		// /apply, /ignore, /ignored segments are children of :checkID.
		smellsHandler := NewLibrarySmellsHandler(db)
		smellsGroup := e.Group("/api/library/smells", auth.AuthMiddleware(db))
		smellsGroup.GET("", smellsHandler.HandleSummary)
		smellsGroup.GET("/:checkID", smellsHandler.HandleList)
		smellsGroup.POST("/:checkID/apply", smellsHandler.HandleApply)
		smellsGroup.POST("/:checkID/ignore", smellsHandler.HandleIgnore)
		smellsGroup.DELETE("/:checkID/ignore", smellsHandler.HandleRestore)
		smellsGroup.GET("/:checkID/ignored", smellsHandler.HandleListIgnored)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/ -run 'TestSmells' -v`
Expected: PASS for all `TestSmells*`.

- [ ] **Step 6: Commit**

```bash
git add internal/api/library_smells.go internal/api/router.go internal/api/library_smells_test.go
git commit -m "feat(api): library smells REST endpoints"
```

---

### Task 10: Final verification + docs

**Files:**
- Modify: `CLAUDE.md` (add `internal/librarysmells` to the package list + a one-line note on the endpoints)

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 2: Dead-code check (a new exported `usergame.ClearWishlist` was added)**

Run: `go run golang.org/x/tools/cmd/deadcode@latest -test ./... | grep -iE 'librarysmells|ClearWishlist' || echo 'no new dead code'`
Expected: no entries for `ClearWishlist` (it's reached via `applyClearWishlist`) or the engine. Reconcile any new finding against the diff. The `detect*`/`apply*` funcs are reached through the `Check` vars + `Registry()`/`Lookup()` — deadcode can't always see func-value indirection, so a false-positive on a `detect*`/`apply*` is acceptable; confirm it is genuinely referenced by a `Check` var before dismissing.

- [ ] **Step 3: Run the two new packages' suites**

Run: `go test ./internal/librarysmells/ ./internal/usergame/ ./internal/api/ -run 'TestDetect|TestApply|TestRegistry|TestClearWishlist|TestSmells' -v`
Expected: PASS.

- [ ] **Step 4: Update CLAUDE.md**

In the package list under "## Project Structure", add an entry:

```markdown
- `internal/librarysmells/` — library-smell detection engine (#1144): a `Check` registry of 11 detectors (slug-keyed) over the user's collection, each a read-only Bun query anti-joined against `smell_ignores`; 4 are auto-fixable (`wishlisted-yet-owned`→clear wishlist, `beat-but-not-marked`/`played-but-not-started`/`in-progress-untouched`→set-status) and route their `Apply` through `internal/usergame` after re-validating the still-flagged subset. Served at `/api/library/smells` (summary → `/:checkID` listing → `/apply` → `/ignore`+restore+`/ignored`).
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document the library smells engine in CLAUDE.md"
```

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin feat/1144-library-smells-engine
gh pr create --title "feat: library smells detection engine + REST API" \
  --label enhancement \
  --body "$(cat <<'EOF'
Implements the backbone of the Library Smells epic (#1143): the `internal/librarysmells` detection engine (11 detectors, 4 auto-fixes), the `smell_ignores` table, and the `/api/library/smells` REST API.

Closes #1144
EOF
)"
```

> No AI-attribution trailer in the title or body (user's global rule).

---

## Self-Review

**Spec coverage:**
- 11 detectors → Tasks 3–7 (orphan; 4 platform-row; date; wishlist; 4 nudge). ✓
- `smell_ignores` migration + model → Task 1. ✓
- `ClearWishlist` added to `internal/usergame` → Task 2. ✓
- Auto-fix routing through usergame (#4/#7/#8/#9) + re-validation → Task 8. ✓
- REST: summary / listing / apply / ignore / restore / ignored → Task 9. ✓
- #7→#8 precedence + HLTB-NULL silence → Task 7 tests + `detectPlayedButNotStarted` guard. ✓
- Ignores honored in detect + summary; per-(game,check) grain → anti-join in every detector + ignore tests. ✓
- "owned" = has-platform-row; orphan wishlist-exclusion → encoded in `detectWishlistedYetOwned` / `detectOrphanGame`. ✓
- Slug check IDs; tiers → Global Constraints + check vars. ✓

**Placeholder scan:** No TBD/TODO; every code step carries full code; SQL is complete per detector. Two explicit "verify-then-adapt" notes (Echo path-param accessor; seeded pair validity) are deliberate codebase-fit checks, not placeholders — each names the exact grep to run and the concrete fallback.

**Type consistency:** `Check`/`FlaggedItem` fields match across registry, detectors, apply, and handler. `Apply` signature `(ctx, db, userID, ids) (applied, skipped int, err error)` is identical in the type def, the `applyX` funcs, and the handler call. `usergame.ClearWishlist`/`SetPlayStatusBulk`/`BulkStatusParams` match Task 2 + verified package signatures. `enum.PlayStatus*` constants verified against `internal/enum/enum.go`.

## Execution Handoff

(Presented to the user after saving.)
