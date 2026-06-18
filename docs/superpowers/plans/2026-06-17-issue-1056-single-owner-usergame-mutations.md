# Single-Owner User-Game Mutations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every user-game mutation outcome (acquire, platform add/update/remove, set-status, record-progress, delete, clear) a single atomic operation in `internal/usergame` that all in-process callers (API handlers, sync worker, import workers) route through, so the invariant set (clear-wishlist, promote-if-played, remove-from-pools-if-finished, tag reconcile) can never be skipped.

**Architecture:** `internal/usergame` grows from loose helpers into the operation owner. Operations are free functions taking `*bun.DB`; each opens its own `db.RunInTx(...)` and runs its full invariant set atomically. The existing loose helpers become unexported and are called only from inside the operations. Job-scoped side-effects (the `changes` rows + `notify.Emit`) stay at the worker boundary, driven by a `Result` the operation returns. A canonical duplicate-key classifier lands in a new `internal/db` package.

**Tech Stack:** Go 1.26, Bun ORM (`uptrace/bun`) + `pgdriver`, Echo v5, River queue, `testing` + `testcontainers-go`.

## Global Constraints

- Operations live in `internal/usergame`; the package MUST stay echo-free (no `github.com/labstack/echo` import).
- Public operations take `*bun.DB` and own their transaction via `db.RunInTx`. Internal helpers take `bun.IDB` and run inside the operation's tx.
- Typed errors at the package boundary: `usergame.ErrNotFound`, `usergame.ErrConflict`, `usergame.ErrValidation`; `sql.ErrNoRows` maps to `ErrNotFound`. Wrap detail with `fmt.Errorf("...: %w", ErrX)`.
- Logging in request/job code uses `slog.*Context(ctx, …)` + `internal/logging` `Key*` constants + `logging.Cat(...)` on error/warn boundaries. Never log secrets/bodies.
- `errcheck` runs with `check-blank`: handle every error or annotate `//nolint:errcheck // <reason>`. `gosec` is enabled (non-test code).
- DB-backed tests use the shared `testDB` package var + `truncateAllTables(t)` at the top of each test (NO per-test container).
- Migrations: none required — this change adds no schema.
- Commit message convention (Conventional Commits): use `refactor:` for pure-routing commits with no behaviour change, `fix:` for the commits that change behaviour (the acquire-atomicity / import platform-merge convergence), `test:` for test-only commits. The squash PR title will be `refactor: single-owner user-game mutations (#1056)`.
- Final PR closes the issue: PR body MUST contain `Closes #1056`.

---

## File Structure

**New files:**
- `internal/db/errors.go` — package `db`: `IsUniqueViolation(err error) bool`.
- `internal/db/errors_test.go` — its test.
- `internal/usergame/errors.go` — sentinel errors.
- `internal/usergame/types.go` — `AcquireMode`, `TagMode`, `PlatformInput`, `TagInput`, param structs, `Result`, `PlatformChange`, `ownershipRank`.
- `internal/usergame/acquire.go` — `Acquire`, `AddPlatform`, `AddPlatformBulk`, `MoveToLibrary`, shared merge.
- `internal/usergame/platform.go` — `UpdatePlatform`, `RemovePlatform`, `RemovePlatformBulk`.
- `internal/usergame/status.go` — `UpdateFields`, `SetPlayStatusBulk`, `RecordProgress`.
- `internal/usergame/lifecycle.go` — `Delete`, `DeleteBulk`, `ClearLibrary`.
- `internal/usergame/main_test.go` — TestMain (shared container), `truncateAllTables`, seed helpers.
- `internal/usergame/acquire_test.go`, `platform_test.go`, `status_test.go`, `lifecycle_test.go` — operation tests.

**Modified files:**
- `internal/api/user_games.go` — route all mutating handlers through operations; add `httpError` mapper; delete `isDuplicateKeyError`.
- `internal/api/tags.go`, `internal/api/pools.go`, `internal/api/notifications.go` — delete `isDuplicateKeyError`, call `db.IsUniqueViolation`.
- `internal/worker/tasks/sync.go` — `UserGameWorker` routes acquire through `Acquire(ModeUpsert)`, emits `changes` rows from `Result`; delete `ownershipRank` (moved to usergame).
- `internal/worker/tasks/import_item.go`, `internal/worker/tasks/import_pipeline.go` — route acquire through `Acquire(ModeUpsert)`.
- `internal/usergame/wishlist.go`, `promote.go`, `pools.go` — unexport the helpers (final task).

---

## Task 1: Shared `db.IsUniqueViolation`, dedup the 4 copies

**Files:**
- Create: `internal/db/errors.go`, `internal/db/errors_test.go`
- Modify: `internal/api/user_games.go`, `internal/api/tags.go`, `internal/api/pools.go`, `internal/api/notifications.go` (delete `isDuplicateKeyError`, replace calls)

**Interfaces:**
- Produces: `func db.IsUniqueViolation(err error) bool`

- [ ] **Step 1: Write the failing test**

Create `internal/db/errors_test.go`:
```go
package db

import (
	"errors"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sqlstate 23505", errors.New(`ERROR: duplicate key value violates unique constraint "x" (SQLSTATE 23505)`), true},
		{"unique constraint text", errors.New("unique constraint failed"), true},
		{"unique_violation text", errors.New("pq: unique_violation"), true},
		{"unrelated", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUniqueViolation(tc.err); got != tc.want {
				t.Fatalf("IsUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestIsUniqueViolation -v`
Expected: FAIL — `undefined: IsUniqueViolation` (build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/db/errors.go` (preserve the existing detection strings verbatim — the 4 copies all match on these three substrings):
```go
// Package db holds shared database helpers used across the data layer.
package db

import "strings"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505). pgdriver wraps the error and embeds the code in
// the message, so this matches on the substrings the driver emits.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique_violation")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestIsUniqueViolation -v`
Expected: PASS (all sub-tests).

- [ ] **Step 5: Replace the 4 copies**

In each of `internal/api/user_games.go`, `tags.go`, `pools.go`, `notifications.go`:
1. Delete the `func isDuplicateKeyError(err error) bool { ... }` definition.
2. Replace every call `isDuplicateKeyError(err)` with `db.IsUniqueViolation(err)`.
3. Add the import `"github.com/drzero42/nexorious/internal/db"` (alias not needed — package name is `db`; confirm no local variable named `db` shadows it in those functions. In `user_games.go` the handler field is `h.db`, not a package-level `db`, so no shadow. If any function has a parameter/local named `db`, alias the import as `nexdb "github.com/drzero42/nexorious/internal/db"` in that file and use `nexdb.IsUniqueViolation`).

Find call sites: `grep -rn "isDuplicateKeyError" internal/api/`.

- [ ] **Step 6: Verify build, vet, and the affected package tests**

Run: `go build ./... && go test ./internal/api/ ./internal/db/`
Expected: PASS. Then `grep -rn "isDuplicateKeyError" internal/` returns nothing.

- [ ] **Step 7: Commit**

```bash
git add internal/db/errors.go internal/db/errors_test.go internal/api/user_games.go internal/api/tags.go internal/api/pools.go internal/api/notifications.go
git commit -m "refactor: extract db.IsUniqueViolation, dedup 4 copies"
```

---

## Task 2: `internal/usergame` test harness

**Files:**
- Create: `internal/usergame/main_test.go`

**Interfaces:**
- Produces (test-only): package var `testDB *bun.DB`; `func truncateAllTables(t *testing.T)`; `func seedUserGame(t *testing.T, userID string, gameID int32) (userGameID string)`; `func seedUser(t *testing.T) (userID string)`; `func seedGame(t *testing.T, gameID int32, title string)`.

Mirror `internal/api/main_test.go` (read it for the exact testcontainers + migrator + river-migrator boilerplate; copy it, dropping the `auth`/`crypto`/`testEncrypter` bits this package doesn't need).

- [ ] **Step 1: Write the harness + a smoke test**

Create `internal/usergame/main_test.go`:
```go
package usergame

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

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
	"github.com/drzero42/nexorious/internal/db/models"
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
	// Truncate the tables these operation tests touch. CASCADE clears dependents.
	_, err := testDB.ExecContext(context.Background(),
		`TRUNCATE users, games, user_games, user_game_platforms, tags, user_game_tags, pools, pool_games, changes CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func seedUser(t *testing.T) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := testDB.NewRaw(
		`INSERT INTO users (id, username, password_hash, is_admin, created_at, updated_at)
		 VALUES (?, ?, 'x', false, ?, ?)`,
		id, "u_"+id[:8], now, now).Exec(context.Background())
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
	ug := &models.UserGame{ID: uuid.NewString(), UserID: userID, GameID: gameID}
	if _, err := testDB.NewInsert().Model(ug).Exec(context.Background()); err != nil {
		t.Fatalf("seed user_game: %v", err)
	}
	return ug.ID
}

func TestHarnessUp(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	if u == "" {
		t.Fatal("expected user id")
	}
}
```

> **Verify before coding:** open `internal/db/models/models.go` and confirm the column/field names used above (`users.username`, `users.password_hash`, `users.is_admin`, `user_games.game_id` type, `games.id` type). Adjust the seed SQL and the `models.UserGame{GameID: ...}` type to match (the plan assumes `games.id`/`user_games.game_id` is `int32` — if the model uses a different width, use it consistently everywhere `GameID` appears in this plan).

- [ ] **Step 2: Run the smoke test**

Run: `go test ./internal/usergame/ -run TestHarnessUp -v`
Expected: PASS (container starts, migrations run, user seeds).

- [ ] **Step 3: Commit**

```bash
git add internal/usergame/main_test.go
git commit -m "test: add internal/usergame test harness"
```

---

## Task 3: Sentinel errors + operation types

**Files:**
- Create: `internal/usergame/errors.go`, `internal/usergame/types.go`

**Interfaces:**
- Produces:
  - `var ErrNotFound, ErrConflict, ErrValidation error`
  - `type AcquireMode int` with `ModeCreate`, `ModeUpsert`
  - `type TagMode int` with `TagMerge`, `TagReplace`
  - `type PlatformInput struct { Platform, Storefront *string; HoursPlayed *float64; OwnershipStatus *string; IsAvailable *bool; AcquiredDate *time.Time; ExternalGameID *string }`
  - `type TagInput struct { Name string; Color *string }`
  - `type AcquireParams struct { UserID string; GameID int32; Mode AcquireMode; Platforms []PlatformInput; Tags []TagInput; TagMode TagMode }`
  - `type Result struct { UserGameID string; Created bool; PlatformChanges []PlatformChange }`
  - `type PlatformChange struct { Platform, Storefront string; Created, OwnershipUpgraded bool; OldOwnership, NewOwnership *string }`
  - `func ownershipRank(status string) int`

- [ ] **Step 1: Write `errors.go`**

```go
package usergame

import "errors"

var (
	// ErrNotFound is returned when the target user game (or platform) does not
	// exist for the given user. Maps to HTTP 404.
	ErrNotFound = errors.New("user game not found")
	// ErrConflict is returned when an operation would violate uniqueness
	// (game already in collection / duplicate platform). Maps to HTTP 409.
	ErrConflict = errors.New("conflict")
	// ErrValidation is returned for invalid input (bad ownership status, empty
	// platform set where one is required, etc.). Maps to HTTP 400.
	ErrValidation = errors.New("validation")
)
```

- [ ] **Step 2: Write `types.go`**

Copy `ownershipRank` verbatim from `internal/worker/tasks/sync.go:427` (read it first) into this file as an unexported function. Define the types from the Interfaces block above.

```go
package usergame

import "time"

type AcquireMode int

const (
	// ModeCreate requires the user_games row not to exist; a pre-existing row
	// (or duplicate platform) yields ErrConflict. Used by the REST create path.
	ModeCreate AcquireMode = iota
	// ModeUpsert finds-or-creates idempotently and merges platforms. Used by
	// sync and import.
	ModeUpsert
)

type TagMode int

const (
	// TagMerge adds the supplied tags without removing existing ones (sync/import).
	TagMerge TagMode = iota
	// TagReplace reconciles to exactly the supplied set (explicit REST replace).
	TagReplace
)

type PlatformInput struct {
	Platform        *string
	Storefront      *string
	HoursPlayed     *float64
	OwnershipStatus *string
	IsAvailable     *bool
	AcquiredDate    *time.Time
	ExternalGameID  *string
}

type TagInput struct {
	Name  string
	Color *string
}

type AcquireParams struct {
	UserID    string
	GameID    int32
	Mode      AcquireMode
	Platforms []PlatformInput
	Tags      []TagInput
	TagMode   TagMode
}

type Result struct {
	UserGameID      string
	Created         bool
	PlatformChanges []PlatformChange
}

type PlatformChange struct {
	Platform          string
	Storefront        string
	Created           bool
	OwnershipUpgraded bool
	OldOwnership      *string
	NewOwnership      *string
}

// ownershipRank returns a numeric rank for an ownership status string.
// <copy body verbatim from internal/worker/tasks/sync.go>
```

- [ ] **Step 3: Verify build**

Run: `go build ./internal/usergame/`
Expected: PASS (no test yet; types compile). `ownershipRank` will report as unused until Task 4 — that is fine for `go build`; do NOT run lint on this package in isolation yet.

- [ ] **Step 4: Commit**

```bash
git add internal/usergame/errors.go internal/usergame/types.go
git commit -m "refactor: add usergame sentinel errors and operation types"
```

---

## Task 4: The `Acquire` keystone

**Files:**
- Create: `internal/usergame/acquire.go`, `internal/usergame/acquire_test.go`

**Interfaces:**
- Consumes: types from Task 3; `clearWishlistOnAcquire`/`promoteToInProgressIfPlayed` (currently exported as `ClearWishlistOnAcquire`/`PromoteToInProgressIfPlayed` — call the exported names for now; Task 13 renames them); `ReplaceTags`/`ResolveOrCreateTag` (exported, from `tags.go`).
- Produces: `func Acquire(ctx context.Context, db *bun.DB, p AcquireParams) (Result, error)`; `func mergePlatforms(ctx context.Context, tx bun.IDB, userGameID string, ins []PlatformInput) ([]PlatformChange, error)` (internal); `func reconcileTags(ctx context.Context, tx bun.IDB, userGameID, userID string, tags []TagInput, mode TagMode) error` (internal).

**Behaviour (all inside one `db.RunInTx`):**
1. user_games row: `ModeUpsert` → `INSERT ... ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now() RETURNING id, (xmax = 0) AS is_new`. `ModeCreate` → plain insert; unique violation (`db.IsUniqueViolation`) → `ErrConflict`.
2. `mergePlatforms`: per input, upsert by `(user_game_id, platform, storefront)` — insert if absent; else single UPDATE with `hours_played = max(existing, new)`, ownership = higher by `ownershipRank`, always backfill `external_game_id`. Record a `PlatformChange`.
3. tail: `ClearWishlistOnAcquire`, `PromoteToInProgressIfPlayed`, and (if `len(p.Tags) > 0`) `reconcileTags`.

- [ ] **Step 1: Write failing tests**

Create `internal/usergame/acquire_test.go`:
```go
package usergame

import (
	"context"
	"errors"
	"testing"

	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

func strptr(s string) *string    { return &s }
func fptr(f float64) *float64     { return &f }

// fetchStatus / fetchWishlist / countPlatforms small helpers
func fetchUG(t *testing.T, id string) models.UserGame {
	t.Helper()
	var ug models.UserGame
	if err := testDB.NewSelect().Model(&ug).Where("id = ?", id).Scan(context.Background()); err != nil {
		t.Fatalf("fetch ug: %v", err)
	}
	return ug
}

func TestAcquire_CreateInsertsPlatformsClearsWishlistPromotes(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 100, "Celeste")
	// Pre-existing wishlist row to prove clear-on-acquire.
	_, err := testDB.NewRaw(`INSERT INTO user_games (id, user_id, game_id, is_wishlisted, play_status, created_at, updated_at)
		VALUES (gen_random_uuid(), ?, 100, true, 'not_started', now(), now())`, u).Exec(context.Background())
	if err != nil { t.Fatal(err) }

	res, err := Acquire(context.Background(), testDB, AcquireParams{
		UserID: u, GameID: 100, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc"), Storefront: strptr("steam"), HoursPlayed: fptr(3)}},
	})
	if err != nil { t.Fatalf("acquire: %v", err) }
	ug := fetchUG(t, res.UserGameID)
	if ug.IsWishlisted { t.Error("wishlist should be cleared") }
	if ug.PlayStatus == nil || *ug.PlayStatus != "in_progress" {
		t.Errorf("expected promote to in_progress, got %v", ug.PlayStatus)
	}
}

func TestAcquire_ModeCreateConflictsOnExisting(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedUserGame(t, u, 200)
	_, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 200, Mode: ModeCreate})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAcquire_UpsertIsIdempotent(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 300, "Hades")
	p := AcquireParams{UserID: u, GameID: 300, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc"), Storefront: strptr("steam"), HoursPlayed: fptr(5)}}}
	r1, err := Acquire(context.Background(), testDB, p)
	if err != nil { t.Fatal(err) }
	r2, err := Acquire(context.Background(), testDB, p)
	if err != nil { t.Fatal(err) }
	if r1.UserGameID != r2.UserGameID { t.Error("upsert should return the same user_game") }
	var n int
	_ = testDB.NewRaw(`SELECT count(*) FROM user_game_platforms WHERE user_game_id = ?`, r1.UserGameID).Scan(context.Background(), &n)
	if n != 1 { t.Errorf("expected 1 platform after idempotent re-acquire, got %d", n) }
}

func TestAcquire_MergeKeepsMaxHoursAndUpgradesOwnership(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	seedGame(t, 400, "Tunic")
	_, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 400, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc"), Storefront: strptr("steam"), HoursPlayed: fptr(10), OwnershipStatus: strptr("subscription")}}})
	if err != nil { t.Fatal(err) }
	// Re-acquire with lower hours, higher ownership.
	res, err := Acquire(context.Background(), testDB, AcquireParams{UserID: u, GameID: 400, Mode: ModeUpsert,
		Platforms: []PlatformInput{{Platform: strptr("pc"), Storefront: strptr("steam"), HoursPlayed: fptr(2), OwnershipStatus: strptr("owned")}}})
	if err != nil { t.Fatal(err) }
	var hours float64
	var owner string
	_ = testDB.NewRaw(`SELECT hours_played, ownership_status FROM user_game_platforms WHERE user_game_id = ?`, res.UserGameID).Scan(context.Background(), &hours, &owner)
	if hours != 10 { t.Errorf("expected max hours 10, got %v", hours) }
	if owner != "owned" { t.Errorf("expected ownership upgraded to owned, got %v", owner) }
	if len(res.PlatformChanges) != 1 || !res.PlatformChanges[0].OwnershipUpgraded {
		t.Errorf("expected OwnershipUpgraded change, got %+v", res.PlatformChanges)
	}
}
```

> Confirm `models.UserGame` field names (`IsWishlisted`, `PlayStatus *string`) before running — adjust if the model differs.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usergame/ -run TestAcquire -v`
Expected: FAIL — `undefined: Acquire`.

- [ ] **Step 3: Implement `acquire.go`**

```go
package usergame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	nexdb "github.com/drzero42/nexorious/internal/db"
)

// Acquire ensures the user owns the game on the supplied platforms, running the
// full acquire invariant set (clear-wishlist, promote-if-played, optional tag
// reconcile) atomically. See docs/superpowers/specs/2026-06-17-issue-1056-*.
func Acquire(ctx context.Context, db *bun.DB, p AcquireParams) (Result, error) {
	var res Result
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		ugID, created, err := upsertUserGame(ctx, tx, p)
		if err != nil {
			return err
		}
		res.UserGameID = ugID
		res.Created = created

		changes, err := mergePlatforms(ctx, tx, ugID, p.Platforms)
		if err != nil {
			return err
		}
		res.PlatformChanges = changes

		if err := ClearWishlistOnAcquire(ctx, tx, ugID); err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		if err := PromoteToInProgressIfPlayed(ctx, tx, ugID); err != nil {
			return fmt.Errorf("promote if played: %w", err)
		}
		if len(p.Tags) > 0 {
			if err := reconcileTags(ctx, tx, ugID, p.UserID, p.Tags, p.TagMode); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func upsertUserGame(ctx context.Context, tx bun.IDB, p AcquireParams) (string, bool, error) {
	now := time.Now().UTC()
	id := uuid.NewString()
	if p.Mode == ModeUpsert {
		var row struct {
			ID    string `bun:"id"`
			IsNew bool   `bun:"is_new"`
		}
		err := tx.NewRaw(
			`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (user_id, game_id) DO UPDATE SET updated_at = now()
			 RETURNING id, (xmax = 0) AS is_new`,
			id, p.UserID, p.GameID, now, now,
		).Scan(ctx, &row)
		if err != nil {
			return "", false, fmt.Errorf("upsert user_game: %w", err)
		}
		return row.ID, row.IsNew, nil
	}
	// ModeCreate
	_, err := tx.NewRaw(
		`INSERT INTO user_games (id, user_id, game_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, p.UserID, p.GameID, now, now,
	).Exec(ctx)
	if err != nil {
		if nexdb.IsUniqueViolation(err) {
			return "", false, fmt.Errorf("game already in collection: %w", ErrConflict)
		}
		return "", false, fmt.Errorf("insert user_game: %w", err)
	}
	return id, true, nil
}

func mergePlatforms(ctx context.Context, tx bun.IDB, ugID string, ins []PlatformInput) ([]PlatformChange, error) {
	var changes []PlatformChange
	for _, in := range ins {
		ch, err := mergeOnePlatform(ctx, tx, ugID, in)
		if err != nil {
			return nil, err
		}
		changes = append(changes, ch)
	}
	return changes, nil
}

func mergeOnePlatform(ctx context.Context, tx bun.IDB, ugID string, in PlatformInput) (PlatformChange, error) {
	ownership := "owned"
	if in.OwnershipStatus != nil && *in.OwnershipStatus != "" {
		ownership = *in.OwnershipStatus
	}
	available := true
	if in.IsAvailable != nil {
		available = *in.IsAvailable
	}
	var hours float64
	if in.HoursPlayed != nil {
		hours = *in.HoursPlayed
	}
	ch := PlatformChange{Platform: deref(in.Platform), Storefront: deref(in.Storefront)}

	var existingID string
	var existingOwnership *string
	var existingHours *float64
	err := tx.NewRaw(
		`SELECT id, ownership_status, hours_played FROM user_game_platforms
		 WHERE user_game_id = ? AND platform = ? AND storefront = ?`,
		ugID, in.Platform, in.Storefront,
	).Scan(ctx, &existingID, &existingOwnership, &existingHours)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		ch.Created = true
		_, err := tx.NewRaw(
			`INSERT INTO user_game_platforms
			 (id, user_game_id, platform, storefront, is_available, hours_played, ownership_status, external_game_id, acquired_date, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now())
			 ON CONFLICT (user_game_id, platform, storefront) DO NOTHING`,
			uuid.NewString(), ugID, in.Platform, in.Storefront, available, hours, ownership, in.ExternalGameID, in.AcquiredDate,
		).Exec(ctx)
		if err != nil {
			return ch, fmt.Errorf("insert platform: %w", err)
		}
	case err != nil:
		return ch, fmt.Errorf("select existing platform: %w", err)
	default:
		finalOwnership := ownership
		if existingOwnership != nil {
			finalOwnership = *existingOwnership
		}
		if ownershipRank(ownership) > ownershipRankPtr(existingOwnership) {
			ch.OwnershipUpgraded = true
			ch.OldOwnership = existingOwnership
			o := ownership
			ch.NewOwnership = &o
			finalOwnership = ownership
		}
		finalHours := hours
		if existingHours != nil && *existingHours > finalHours {
			finalHours = *existingHours
		}
		_, err := tx.NewRaw(
			`UPDATE user_game_platforms SET ownership_status = ?, hours_played = ?, external_game_id = COALESCE(?, external_game_id), updated_at = now() WHERE id = ?`,
			finalOwnership, finalHours, in.ExternalGameID, existingID,
		).Exec(ctx)
		if err != nil {
			return ch, fmt.Errorf("update platform: %w", err)
		}
	}
	return ch, nil
}

func ownershipRankPtr(s *string) int {
	if s == nil {
		return 0
	}
	return ownershipRank(*s)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

And `reconcileTags` (internal wrapper over the existing tag helpers; merge vs replace):
```go
func reconcileTags(ctx context.Context, tx bun.IDB, ugID, userID string, tags []TagInput, mode TagMode) error {
	if mode == TagReplace {
		names := make([]string, 0, len(tags))
		for _, t := range tags {
			names = append(names, t.Name)
		}
		return ReplaceTags(ctx, tx, ugID, userID, names)
	}
	// TagMerge: resolve/create each tag and insert the link if absent.
	for _, t := range tags {
		tagID, err := ResolveOrCreateTag(ctx, tx, userID, t.Name, t.Color)
		if err != nil {
			return err
		}
		if _, err := tx.NewRaw(
			`INSERT INTO user_game_tags (id, user_game_id, tag_id, created_at)
			 VALUES (?, ?, ?, now())
			 ON CONFLICT (user_game_id, tag_id) DO NOTHING`,
			uuid.NewString(), ugID, tagID,
		).Exec(ctx); err != nil {
			return fmt.Errorf("merge tag link: %w", err)
		}
	}
	return nil
}
```

> **Verify before coding:** confirm the `user_game_tags` unique constraint is on `(user_game_id, tag_id)` (read `internal/db/models/models.go` / the migration). If the constraint name/columns differ, adjust the `ON CONFLICT` target. Confirm `ReplaceTags` accepts `bun.IDB` (it does per `tags.go`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/usergame/ -run TestAcquire -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add internal/usergame/acquire.go internal/usergame/acquire_test.go
git commit -m "feat: add usergame.Acquire keystone operation"
```

---

## Task 5: Route the REST create path + `httpError` mapper

**Files:**
- Modify: `internal/api/user_games.go` — `HandleCreateUserGame` (`:388-505`), add `httpError` helper.

**Interfaces:**
- Consumes: `usergame.Acquire`, `usergame.AcquireParams`, `usergame.ModeCreate`, `usergame.Err*`.
- Produces: `func (h *UserGamesHandler) httpError(c *echo.Context, err error) error` (or a free `httpError(err) *echo.HTTPError`).

- [ ] **Step 1: Add the error mapper**

Add to `user_games.go`:
```go
// httpError maps a usergame operation error to an echo HTTP error, preserving
// the existing status codes. Unmapped errors become 500 and are logged.
func (h *UserGamesHandler) httpError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, usergame.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "user game not found")
	case errors.Is(err, usergame.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, "game already in collection")
	case errors.Is(err, usergame.ErrValidation):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		slog.ErrorContext(c.Request().Context(), "user_games: operation failed", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return echo.NewHTTPError(http.StatusInternalServerError, "database error")
	}
}
```

- [ ] **Step 2: Replace the create body**

In `HandleCreateUserGame`, after request validation and resolving `gameID`, replace the manual `user_games` insert + platform insert + `ClearWishlistOnAcquire`/`PromoteToInProgressIfPlayed` block (`:432-487`) with a single `Acquire(ModeCreate)` call. Map `req.Platforms []platformRequest` → `[]usergame.PlatformInput` (parse `AcquiredDate` via the existing `parseAcquiredDate`, validate `ownership_status` via `enum.OwnershipStatus(...).Valid()` returning `echo.NewHTTPError(400, ...)` before the call). Then re-select the row for the response exactly as today (`:489-504`).

```go
plats := make([]usergame.PlatformInput, 0, len(req.Platforms))
for _, p := range req.Platforms {
	if p.OwnershipStatus != nil && *p.OwnershipStatus != "" && !enum.OwnershipStatus(*p.OwnershipStatus).Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ownership_status: "+*p.OwnershipStatus)
	}
	var acquired *time.Time
	if p.AcquiredDate != nil && *p.AcquiredDate != "" {
		a, err := parseAcquiredDate(*p.AcquiredDate)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		acquired = a
	}
	plats = append(plats, usergame.PlatformInput{
		Platform: p.Platform, Storefront: p.Storefront, HoursPlayed: p.HoursPlayed,
		OwnershipStatus: p.OwnershipStatus, IsAvailable: p.IsAvailable, AcquiredDate: acquired,
	})
}
res, err := usergame.Acquire(ctx, h.db, usergame.AcquireParams{
	UserID: userID, GameID: gameID, Mode: usergame.ModeCreate, Platforms: plats,
})
if err != nil {
	return h.httpError(c, err)
}
ug.ID = res.UserGameID
```

> The create path's original 409 message was "game already in collection" for the user_games dup and "platform/storefront combination already exists" for the platform dup. Under `ModeCreate` both surface as `ErrConflict` → 409 "game already in collection". If preserving the distinct platform-conflict message matters, note it as acceptable message drift in the PR description (status code unchanged). Confirm with reviewer if needed.

- [ ] **Step 3: Run the handler tests**

Run: `go test ./internal/api/ -run 'TestCreateUserGame|TestHandleCreate|UserGame' -v`
Expected: PASS. Fix any test asserting the exact platform-conflict message (update to the 409 contract).

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go
git commit -m "refactor: route REST create through usergame.Acquire"
```

---

## Task 6: Route the sync worker through `Acquire`

**Files:**
- Modify: `internal/worker/tasks/sync.go` — `UserGameWorker.Work` acquire region (`:710-872`); delete `ownershipRank` (`:425-440`, now in usergame).

**Interfaces:**
- Consumes: `usergame.Acquire`, `usergame.Result`, `usergame.PlatformChange`.

**Approach:** Replace the hand-written user_game upsert + platform merge loop + `ClearWishlistOnAcquire`/`PromoteToInProgressIfPlayed` (`:721-851`) with one `Acquire(ModeUpsert)` call that takes the `egPlatforms` mapped to `[]usergame.PlatformInput` (carrying `ExternalGameID: &eg.ID`, ownership resolved as today at `:757-762`). Then emit the `changes` rows **from the returned `Result`**:
- `Result.Created` → `changes('added')`.
- not created and no platform upgraded → `changes('already_in_library')`.
- each `PlatformChange.OwnershipUpgraded` → `changes('status_changed')` with `OldOwnership`/`NewOwnership`.

- [ ] **Step 1: Map platforms and call Acquire**

Replace `:721-851` with:
```go
ownership := "owned"
if eg.OwnershipStatus != nil {
	ownership = *eg.OwnershipStatus
} else if eg.IsSubscription {
	ownership = "subscription"
}
plats := make([]usergame.PlatformInput, 0, len(egPlatforms))
for _, egp := range egPlatforms {
	egp := egp
	plats = append(plats, usergame.PlatformInput{
		Platform: egp.Platform, Storefront: &storefrontSlug,
		HoursPlayed: &egp.HoursPlayed, OwnershipStatus: &ownership,
		IsAvailable: boolptr(true), ExternalGameID: &eg.ID,
	})
}
res, err := usergame.Acquire(ctx, w.DB, usergame.AcquireParams{
	UserID: item.UserID, GameID: *eg.ResolvedIGDBID, Mode: usergame.ModeUpsert, Platforms: plats,
})
if err != nil {
	markItemFailed(ctx, w.DB, &item, fmt.Sprintf("acquire: %v", err), "process_sync_item: markItemFailed")
	SyncCheckJobCompletion(ctx, w.DB, w.RiverClient, item.JobID)
	return nil
}
ugID := res.UserGameID
```

> `egp.Platform` is `*string`; `storefrontSlug` is the `eg.Storefront` string. `egp.HoursPlayed` is a value — take its address via the loop-local copy. Add a tiny `func boolptr(b bool) *bool { return &b }` helper in the package if one doesn't exist (`grep -rn "func boolptr\|func ptr\[" internal/worker/tasks/`).

- [ ] **Step 2: Emit changes rows from the Result**

Replace the existing `changes` inserts (`:811-817` status_changed, `:853-872` added/already_in_library) with emission driven by `res`:
```go
platformUpgraded := false
for _, pc := range res.PlatformChanges {
	if pc.OwnershipUpgraded {
		platformUpgraded = true
		if _, err := w.DB.NewRaw(
			`INSERT INTO changes (id, job_id, user_id, external_game_id, user_game_id, change_type, title, old_status, new_status, created_at)
			 VALUES (?, ?, ?, ?, ?, 'status_changed', ?, ?, ?, now())`,
			uuid.NewString(), item.JobID, item.UserID, eg.ID, ugID, eg.Title, pc.OldOwnership, pc.NewOwnership,
		).Exec(ctx); err != nil {
			slog.WarnContext(ctx, "user_game_write: insert sync_change (status_changed)", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		}
	}
}
// external_games.updated_at bump (was :847-850) stays as-is.
if res.Created {
	// INSERT changes('added') — copy the existing statement verbatim from :855-862.
} else if !platformUpgraded {
	// INSERT changes('already_in_library') — copy the existing statement verbatim from :864-871.
}
```

> Behaviour parity note: previously `status_changed` was written *before* the platform UPDATE so `old_status` reflected the pre-update value. `Acquire` now captures old/new in `PlatformChange`, so emitting after the commit preserves the recorded values. This is intentional and equivalent.

- [ ] **Step 3: Delete the now-unused `ownershipRank` in sync.go**

Remove `func ownershipRank` (`:425-440`) from `sync.go` — it now lives in `usergame`. Run `grep -n "ownershipRank" internal/worker/tasks/` to confirm no remaining references in the package.

- [ ] **Step 4: Run sync tests + build**

Run: `go build ./... && go test ./internal/worker/tasks/ -run 'Sync|UserGame' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/tasks/sync.go
git commit -m "refactor: route sync worker acquire through usergame.Acquire"
```

---

## Task 7: Route the import workers through `Acquire`

**Files:**
- Modify: `internal/worker/tasks/import_item.go` (`:~230-319`), `internal/worker/tasks/import_pipeline.go` (`:~230-265`).

**Interfaces:** Consumes `usergame.Acquire(ModeUpsert)`.

**Behaviour change (convergence #1 from the spec):** import currently *skips* an existing `(platform, storefront)` pair; under `Acquire(ModeUpsert)` it now merges (max-hours, ownership upgrade). Tags: pass `TagMode: TagMerge` to preserve additive semantics. The two paths currently building `existingPlatforms` skip-sets and looping inserts are replaced by mapping the parsed platform data to `[]usergame.PlatformInput` and one `Acquire` call; the per-tag resolve/insert loop becomes `Tags: []usergame.TagInput{...}` on the same call.

- [ ] **Step 1: import_item.go — replace platform loop + helpers + tag loop**

Map the parsed `pd` platform entries (the loop around `:255-308`) to `[]usergame.PlatformInput` (ownership default-to-owned logic at `:282-289` stays as input prep; `IsAvailable` absent⇒true → leave `IsAvailable: nil` and let the operation default it, OR set explicitly to match `:296`). Map `gd.Tags` to `[]usergame.TagInput`. Replace the user_game find/insert + platform inserts + `ClearWishlistOnAcquire` + `PromoteToInProgressIfPlayed` (`:309-319`) + tag loop (`:321-end`) with one call:
```go
res, err := usergame.Acquire(ctx, w.DB, usergame.AcquireParams{
	UserID: item.UserID, GameID: resolvedIGDBID, Mode: usergame.ModeUpsert,
	Platforms: plats, Tags: tags, TagMode: usergame.TagMerge,
})
if err != nil {
	markItemFailed(context.Background(), w.DB, &item, fmt.Sprintf("acquire: %v", err), "import_item: markItemFailed")
	checkJobCompletion(w.DB, item.JobID)
	return nil
}
_ = res
```

> Read `import_item.go:230-360` first to capture the exact game-id resolution (`resolvedIGDBID`) and the `alreadyExists` reselect logic, so the mapping preserves them. Where the worker later uses `ug.ID`, use `res.UserGameID`.

- [ ] **Step 2: import_pipeline.go — same transformation**

Replace the platform build + `ClearWishlistOnAcquire`/`PromoteToInProgressIfPlayed` (`:~240-265`) and the tag write (`:283`) with one `Acquire(ModeUpsert, TagMode: TagMerge)` call, mapping `payload`/`ugp` data to `PlatformInput` (game-level `HoursPlayed` on the first entry — preserve current behaviour by placing it on the first `PlatformInput`).

- [ ] **Step 3: Run import tests + build**

Run: `go build ./... && go test ./internal/worker/tasks/ -run 'Import' -v`
Expected: PASS. If an existing test asserted that re-import *skips* an existing platform, update it to assert the new merge (max-hours) behaviour and note it in the PR description.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/tasks/import_item.go internal/worker/tasks/import_pipeline.go
git commit -m "fix: route import acquire through usergame.Acquire (merge existing platforms)"
```

---

## Task 8: `AddPlatform`, `AddPlatformBulk`, `MoveToLibrary` + routing

**Files:**
- Modify: `internal/usergame/acquire.go` (add ops), `internal/usergame/acquire_test.go` (tests)
- Modify: `internal/api/user_games.go` — `HandleCreatePlatform` (`:1089`), `HandleBulkAddPlatforms` (`:922`), `HandleMoveToLibrary` (`:1314`)

**Interfaces:**
- Produces:
  - `func AddPlatform(ctx, db, AddPlatformParams) (Result, error)` — params `{UserID, UserGameID string; Platform PlatformInput}`. Verifies ownership (`ErrNotFound`), inserts the platform (duplicate → `ErrConflict`), runs clear-wishlist + promote tail, all in-tx.
  - `func AddPlatformBulk(ctx, db, BulkAddPlatformParams) (int, error)` — params `{UserID string; UserGameIDs []string; Platform PlatformInput}`.
  - `func MoveToLibrary(ctx, db, MoveParams) (Result, error)` — params `{UserID, UserGameID string; Platforms []PlatformInput}`. Asserts the row exists and `is_wishlisted` (`ErrValidation` if not wishlisted), inserts platforms (dup → `ErrConflict`), clears wishlist, promotes.

- [ ] **Step 1: Write failing tests** (in `acquire_test.go`)

```go
func TestAddPlatform_ConflictOnDuplicate(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 500)
	in := PlatformInput{Platform: strptr("pc"), Storefront: strptr("steam")}
	if _, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: in}); err != nil {
		t.Fatal(err)
	}
	_, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: in})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAddPlatform_NotFoundForOtherUser(t *testing.T) {
	truncateAllTables(t)
	owner := seedUser(t)
	other := seedUser(t)
	ugID := seedUserGame(t, owner, 501)
	_, err := AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: other, UserGameID: ugID, Platform: PlatformInput{Platform: strptr("pc"), Storefront: strptr("gog")}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMoveToLibrary_RequiresWishlisted(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 502) // not wishlisted
	_, err := MoveToLibrary(context.Background(), testDB, MoveParams{UserID: u, UserGameID: ugID, Platforms: []PlatformInput{{Platform: strptr("pc"), Storefront: strptr("steam")}}})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/usergame/ -run 'TestAddPlatform|TestMoveToLibrary' -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the ops**

Add to `acquire.go`. Each opens `db.RunInTx`; verify ownership with a `SELECT is_wishlisted FROM user_games WHERE id = ? AND user_id = ?` returning `sql.ErrNoRows → ErrNotFound`; reuse `mergeOnePlatform` for the insert path but in `ModeCreate`-style a pre-existing platform must conflict — so for `AddPlatform`/`MoveToLibrary` use an explicit insert (no `ON CONFLICT DO NOTHING`) and map `db.IsUniqueViolation` → `ErrConflict`:
```go
type AddPlatformParams struct {
	UserID      string
	UserGameID  string
	Platform    PlatformInput
}

func AddPlatform(ctx context.Context, db *bun.DB, p AddPlatformParams) (Result, error) {
	var res Result
	res.UserGameID = p.UserGameID
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := assertOwned(ctx, tx, p.UserGameID, p.UserID); err != nil {
			return err
		}
		if err := insertPlatformStrict(ctx, tx, p.UserGameID, p.Platform); err != nil {
			return err
		}
		if err := ClearWishlistOnAcquire(ctx, tx, p.UserGameID); err != nil {
			return fmt.Errorf("clear wishlist: %w", err)
		}
		return PromoteToInProgressIfPlayed(ctx, tx, p.UserGameID)
	})
	return res, err
}
```
with helpers:
```go
func assertOwned(ctx context.Context, tx bun.IDB, ugID, userID string) error {
	var x int
	err := tx.NewRaw(`SELECT 1 FROM user_games WHERE id = ? AND user_id = ?`, ugID, userID).Scan(ctx, &x)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func insertPlatformStrict(ctx context.Context, tx bun.IDB, ugID string, in PlatformInput) error {
	// ... build values like mergeOnePlatform's insert branch, but WITHOUT ON CONFLICT;
	// map db.IsUniqueViolation(err) → ErrConflict.
}
```
Implement `MoveToLibrary` likewise but `assertWishlisted` (select `is_wishlisted`; not found → `ErrNotFound`; found && !wishlisted → `fmt.Errorf("not on wishlist: %w", ErrValidation)`), then insert each platform strict, clear wishlist, promote. Implement `AddPlatformBulk` by looping `UserGameIDs` inside one tx (skip rows not owned; count successes).

- [ ] **Step 4: Run, verify pass**

Run: `go test ./internal/usergame/ -run 'TestAddPlatform|TestMoveToLibrary' -v`
Expected: PASS.

- [ ] **Step 5: Route the three handlers**

`HandleCreatePlatform` (`:1089`): replace its insert + helper calls with `usergame.AddPlatform(...)`, map errors via `h.httpError`. `HandleBulkAddPlatforms` (`:922`): `usergame.AddPlatformBulk`. `HandleMoveToLibrary` (`:1314`): `usergame.MoveToLibrary` (delete the manual `BeginTx`/`Commit` block `:1377-1399`); keep the response reselect.

- [ ] **Step 6: Build + tests**

Run: `go build ./... && go test ./internal/api/ ./internal/usergame/ -run 'Platform|MoveToLibrary' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/acquire.go internal/usergame/acquire_test.go internal/api/user_games.go
git commit -m "refactor: add usergame AddPlatform/MoveToLibrary ops and route handlers"
```

---

## Task 9: `UpdatePlatform`, `RemovePlatform`, `RemovePlatformBulk` + routing

**Files:**
- Create: `internal/usergame/platform.go`, `internal/usergame/platform_test.go`
- Modify: `internal/api/user_games.go` — `HandleUpdatePlatform` (`:1178`), `HandleDeletePlatform` (`:1282`), `HandleBulkRemovePlatforms` (`:993`)

**Interfaces:**
- Produces:
  - `func UpdatePlatform(ctx, db, UpdatePlatformParams) error` — params `{UserID, UserGameID, PlatformID string; Fields PlatformInput}`. Verifies ownership; updates the row; runs promote tail (hours may have changed). Not found → `ErrNotFound`.
  - `func RemovePlatform(ctx, db, RemovePlatformParams) error` — `{UserID, UserGameID, PlatformID string}`. Not found → `ErrNotFound`.
  - `func RemovePlatformBulk(ctx, db, BulkRemovePlatformParams) (int, error)` — `{UserID string; UserGameIDs []string; Platform, Storefront string}`.

- [ ] **Step 1: Write failing tests** (`platform_test.go`)

```go
func TestUpdatePlatform_PromotesOnHours(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 600) // play_status defaults not_started
	// add a platform with 0 hours via AddPlatform
	_, _ = AddPlatform(context.Background(), testDB, AddPlatformParams{UserID: u, UserGameID: ugID, Platform: PlatformInput{Platform: strptr("pc"), Storefront: strptr("steam")}})
	var pid string
	_ = testDB.NewRaw(`SELECT id FROM user_game_platforms WHERE user_game_id = ?`, ugID).Scan(context.Background(), &pid)
	if err := UpdatePlatform(context.Background(), testDB, UpdatePlatformParams{UserID: u, UserGameID: ugID, PlatformID: pid, Fields: PlatformInput{HoursPlayed: fptr(4)}}); err != nil {
		t.Fatal(err)
	}
	if ug := fetchUG(t, ugID); ug.PlayStatus == nil || *ug.PlayStatus != "in_progress" {
		t.Errorf("expected promote, got %v", ug.PlayStatus)
	}
}

func TestRemovePlatform_NotFound(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 601)
	if err := RemovePlatform(context.Background(), testDB, RemovePlatformParams{UserID: u, UserGameID: ugID, PlatformID: "00000000-0000-0000-0000-000000000000"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run, verify fail.** `go test ./internal/usergame/ -run 'TestUpdatePlatform|TestRemovePlatform' -v` → FAIL undefined.

- [ ] **Step 3: Implement `platform.go`.** Each op `RunInTx`; `assertOwned`; for update, build a dynamic `UPDATE user_game_platforms SET ... WHERE id = ? AND user_game_id = ?` from the non-nil `Fields` (mirror the existing `HandleUpdatePlatform` field handling at `:1178-1281` — read it), then `PromoteToInProgressIfPlayed`; `RowsAffected()==0` → `ErrNotFound`. For remove, `DELETE ... WHERE id = ? AND user_game_id = ?`; `RowsAffected()==0` → `ErrNotFound`.

- [ ] **Step 4: Run, verify pass.** Expected: PASS.

- [ ] **Step 5: Route the three handlers** via `h.httpError`. Preserve current status codes (404/204).

- [ ] **Step 6: Build + tests.** `go build ./... && go test ./internal/api/ ./internal/usergame/ -run 'Platform' -v` → PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/platform.go internal/usergame/platform_test.go internal/api/user_games.go
git commit -m "refactor: add usergame platform update/remove ops and route handlers"
```

---

## Task 10: `UpdateFields`, `SetPlayStatusBulk` (pools tail) + routing

**Files:**
- Create: `internal/usergame/status.go`, `internal/usergame/status_test.go`
- Modify: `internal/api/user_games.go` — `HandleUpdateUserGame` (`:553`), `HandleBulkUpdate` (`:804`)

**Interfaces:**
- Produces:
  - `func UpdateFields(ctx, db, UpdateFieldsParams) error` — `{UserID, UserGameID string; PlayStatus, PersonalNotes *string; PersonalRating *int; IsLoved *bool}`. Applies only the supplied fields; if `PlayStatus` is set, runs `removeFromPoolsIfFinished` in-tx. Not found → `ErrNotFound`. Invalid play_status → `ErrValidation`.
  - `func SetPlayStatusBulk(ctx, db, BulkStatusParams) (int, error)` — `{UserID string; UserGameIDs []string; PlayStatus string}`. Updates each owned row + `removeFromPoolsIfFinished`; returns count.

- [ ] **Step 1: Write failing tests** (`status_test.go`)

```go
func TestUpdateFields_FinishedRemovesFromPools(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	ugID := seedUserGame(t, u, 700)
	// create a pool and add the game
	poolID := uuid.NewString()
	_, _ = testDB.NewRaw(`INSERT INTO pools (id, user_id, name, created_at, updated_at) VALUES (?, ?, 'P', now(), now())`, poolID, u).Exec(context.Background())
	_, _ = testDB.NewRaw(`INSERT INTO pool_games (id, pool_id, user_game_id, created_at) VALUES (gen_random_uuid(), ?, ?, now())`, poolID, ugID).Exec(context.Background())
	if err := UpdateFields(context.Background(), testDB, UpdateFieldsParams{UserID: u, UserGameID: ugID, PlayStatus: strptr("completed")}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = testDB.NewRaw(`SELECT count(*) FROM pool_games WHERE user_game_id = ?`, ugID).Scan(context.Background(), &n)
	if n != 0 {
		t.Errorf("expected pool membership removed on finish, got %d", n)
	}
}

func TestUpdateFields_NotFound(t *testing.T) {
	truncateAllTables(t)
	u := seedUser(t)
	if err := UpdateFields(context.Background(), testDB, UpdateFieldsParams{UserID: u, UserGameID: uuid.NewString(), IsLoved: boolptrUG(true)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```
> Add a local `func boolptrUG(b bool) *bool { return &b }` in the test file (or reuse one). Confirm `pools`/`pool_games` column names against the models before running.

- [ ] **Step 2: Run, verify fail.** → FAIL undefined.

- [ ] **Step 3: Implement `status.go`.** `UpdateFields`: `RunInTx`; build dynamic UPDATE from non-nil fields (validate `play_status` via `enum.PlayStatus(...).Valid()` → `ErrValidation`; reuse the `allowedUpdateFields` intent — only these columns); `RowsAffected()==0` → `ErrNotFound`; if `PlayStatus != nil` call `removeFromPoolsIfFinished`. `SetPlayStatusBulk`: loop in one tx. Import `internal/enum`.

- [ ] **Step 4: Run, verify pass.** → PASS.

- [ ] **Step 5: Route `HandleUpdateUserGame` + `HandleBulkUpdate`** via `h.httpError`. The `HandleUpdateUserGame` currently parses a generic field map (`:553-659`) — map the allowed keys onto `UpdateFieldsParams`. `HandleBulkUpdate` (`:804-883`) → `SetPlayStatusBulk` (delete its inline `RunInTx` + `removeFromPoolsIfFinished` at `:794`).

- [ ] **Step 6: Build + tests.** `go build ./... && go test ./internal/api/ ./internal/usergame/ -run 'Update|Bulk|Status' -v` → PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/status.go internal/usergame/status_test.go internal/api/user_games.go
git commit -m "refactor: add usergame UpdateFields/SetPlayStatusBulk ops and route handlers"
```

---

## Task 11: `RecordProgress` + `ReplaceTags` routing

**Files:**
- Modify: `internal/usergame/status.go` (add `RecordProgress`), `internal/usergame/status_test.go`
- Modify: `internal/api/user_games.go` — `HandleUpdateProgress` (`:762`), `HandleReplaceTags` (`:697`)

**Interfaces:**
- Produces: `func RecordProgress(ctx, db, ProgressParams) error` — `{UserID, UserGameID string; PlatformID string; HoursPlayed *float64; ... other progress fields per the existing handler}`. Updates hours on the platform/row, then `PromoteToInProgressIfPlayed`. Not found → `ErrNotFound`.

- [ ] **Step 1: Read `HandleUpdateProgress` (`:762-803`)** to capture exactly which columns it writes (hours, progress). Write a failing test asserting promote fires after recording hours (mirror `TestUpdatePlatform_PromotesOnHours`).

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement `RecordProgress`** in `status.go` (`RunInTx`; update; `PromoteToInProgressIfPlayed`; `RowsAffected==0 → ErrNotFound`).

- [ ] **Step 4: Run, verify pass.**

- [ ] **Step 5: Route both handlers.** `HandleUpdateProgress` → `RecordProgress`. `HandleReplaceTags` (`:697-761`) → `usergame.ReplaceTags(ctx, h.db, ugID, userID, names)` wrapped in ownership check (it already verifies ownership — keep that), mapping errors via `h.httpError`. (`ReplaceTags` already exists; this just routes the handler through it consistently and drops any inline duplication.)

- [ ] **Step 6: Build + tests.** `go build ./... && go test ./internal/api/ ./internal/usergame/ -run 'Progress|Tags' -v` → PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/status.go internal/usergame/status_test.go internal/api/user_games.go
git commit -m "refactor: add usergame RecordProgress op and route progress/tags handlers"
```

---

## Task 12: `Delete`, `DeleteBulk`, `ClearLibrary` + routing

**Files:**
- Create: `internal/usergame/lifecycle.go`, `internal/usergame/lifecycle_test.go`
- Modify: `internal/api/user_games.go` — `HandleDeleteUserGame` (`:660`), `HandleBulkDelete` (`:884`), `HandleClearLibrary` (`:1455`)

**Interfaces:**
- Produces:
  - `func Delete(ctx, db, DeleteParams) error` — `{UserID, UserGameID string}`. `RowsAffected==0 → ErrNotFound`. Cascades handle platforms/tags/pool_games.
  - `func DeleteBulk(ctx, db, BulkDeleteParams) (int, error)` — `{UserID string; UserGameIDs []string}`. Returns deleted count.
  - `func ClearLibrary(ctx, db, userID string) (int, error)` — deletes all the user's user_games; returns count. Mirror the existing `HandleClearLibrary` semantics (read `:1455-end` — confirm whether it clears wishlist rows too or only library; preserve exactly).

- [ ] **Step 1: Write failing tests** (`lifecycle_test.go`): delete scopes to the user (deleting another user's id → `ErrNotFound`, no rows removed); `ClearLibrary` removes the caller's rows and returns the count; bulk delete returns the count and removes only owned rows.

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement `lifecycle.go`** (each `RunInTx` or a single scoped `DELETE`; `WHERE user_id = ?` scoping is the ownership guard).

- [ ] **Step 4: Run, verify pass.**

- [ ] **Step 5: Route the three handlers** via `h.httpError`. Preserve current status codes and the `ClearLibrary` response shape.

- [ ] **Step 6: Build + tests.** `go build ./... && go test ./internal/api/ ./internal/usergame/ -run 'Delete|Clear' -v` → PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/usergame/lifecycle.go internal/usergame/lifecycle_test.go internal/api/user_games.go
git commit -m "refactor: add usergame Delete/DeleteBulk/ClearLibrary ops and route handlers"
```

---

## Task 13: Unexport helpers, dead-code sweep, full verification

**Files:**
- Modify: `internal/usergame/wishlist.go`, `promote.go`, `pools.go` (rename exported → unexported), and the internal callers in `acquire.go`/`status.go`/`platform.go`.

- [ ] **Step 1: Confirm no external callers remain**

Run: `grep -rn "usergame.ClearWishlistOnAcquire\|usergame.PromoteToInProgressIfPlayed\|usergame.RemoveFromPoolsIfFinished" internal/ --include="*.go"`
Expected: **no matches** (all routed through operations). If any remain, route them before renaming.

- [ ] **Step 2: Rename to unexported**

Rename `ClearWishlistOnAcquire → clearWishlistOnAcquire`, `PromoteToInProgressIfPlayed → promoteToInProgressIfPlayed`, `RemoveFromPoolsIfFinished → removeFromPoolsIfFinished` (in `wishlist.go`/`promote.go`/`pools.go` and every internal caller in the `usergame` package). Update the package doc comment in `promote.go` (it says "invoked from both the API handlers and the sync worker" — change to "invoked from the usergame operations").

- [ ] **Step 3: Dead-code sweep**

Run: `make deadcode` (i.e. `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`)
Expected: no **new** entries vs. the pre-change baseline. Reconcile any newly-orphaned exported symbol (e.g. a now-unused handler helper) against the diff and delete it. Confirm `ownershipRank` is reported only as used-in-usergame (not orphaned).

- [ ] **Step 4: Full build, lint, suites**

Run:
```bash
go build ./...
golangci-lint run
go test -timeout 600s ./...
```
Expected: all green. (Frontend untouched — no `npm` run needed.)

- [ ] **Step 5: Commit**

```bash
git add internal/usergame/
git commit -m "refactor: unexport usergame invariant helpers (now operation-internal)"
```

---

## Final: Open the PR

- [ ] Push the branch and open a PR titled `refactor: single-owner user-game mutations (#1056)`.
- [ ] PR body MUST include `Closes #1056` and a "Behaviour changes" section enumerating spec convergence #1 (import now merges existing platform rows) and #4 (acquire is now atomic), plus any 409-message drift from Task 5.
- [ ] Confirm `CI Gate` is green before requesting merge. Do not merge without explicit instruction.

---

## Self-Review (completed during planning)

- **Spec coverage:** Acquire keystone (Task 4), all routing (5–7), platform ops (8–9), status/progress (10–11), lifecycle (12), errors+dedup (1,3), helpers internal (13), tests (2 + per-task), `IsUniqueViolation` in `internal/db` (1), typed errors (3), atomicity via `RunInTx` (every op). Reads explicitly untouched. All spec sections map to a task.
- **Placeholder scan:** code provided for foundational tasks; mechanical ops (9–12) give exact signatures, canonical behaviour, real test code, and cite the exact handler line ranges to mirror — implementers read those regions, not guesses.
- **Type consistency:** `Acquire`/`AcquireParams`/`Result`/`PlatformChange`/`PlatformInput`/`AcquireMode`/`TagMode` used consistently across tasks 3–8; `httpError` defined in Task 5 and reused 8–12; helper names (`clearWishlistOnAcquire` etc.) introduced exported and renamed once in Task 13.
