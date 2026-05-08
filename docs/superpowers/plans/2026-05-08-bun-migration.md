# Bun Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the three-tool database stack (sqlc + goqu/sqlx + golang-migrate) with Bun (`uptrace/bun`) while preserving all existing behaviour.

**Architecture:** Every file that touches the database switches from `*pgxpool.Pool` / `*db.Queries` to `*bun.DB`. The migrator replaces golang-migrate internals with `bun/migrate`. Raw SQL queries in auth/setup handlers use `db.QueryRowContext` for scalars and `db.NewRaw` for structs. A new `internal/filter/` package provides dynamic query building using Bun's `SelectQuery`.

**Tech Stack:** Go 1.25, uptrace/bun (core + pgdialect + pgdriver + migrate), testcontainers-go, Echo v5.

**Spec:** `docs/superpowers/specs/2026-05-07-bun-migration.md`

---

## File Map

### Delete
- `internal/db/gen/` (entire directory — 10 generated files)
- `internal/db/queries/` (entire directory — 8 SQL files)
- `sqlc.yaml`

### Create
- `internal/db/models/models.go` — all Bun model structs (Game, UserGame, UserGamePlatform, Platform, Storefront, PlatformStorefront, Tag, UserGameTag, User, UserSession)
- `internal/filter/builder.go` — filterBuilder type + Apply method
- `internal/filter/criteria.go` — one function per filter criterion

### Modify
- `go.mod` / `go.sum` — swap dependencies
- `devenv.nix` — remove `sqlc` from packages
- `Makefile` — remove `sqlc` target, remove from `all`
- `internal/db/migrations/migrations.go` — replace iofs with Bun Discover
- `internal/db/migrations/0001_initial.up.sql` — rename to timestamp format
- `internal/db/migrations/0001_initial.down.sql` — rename to timestamp format
- `internal/migrate/migrator.go` — replace golang-migrate internals with bun/migrate
- `internal/migrate/migrator_test.go` — update for new `NewMigrator(*bun.DB)` signature
- `internal/migrate/handler.go` — change `*pgxpool.Pool` to `*bun.DB`
- `internal/migrate/handler_test.go` — update pool references
- `cmd/nexorious/main.go` — replace pgxpool with Bun setup
- `internal/api/router.go` — inject `*bun.DB` instead of `*pgxpool.Pool`
- `internal/api/auth.go` — `*pgxpool.Pool` → `*bun.DB`, pgx queries → `db.QueryRowContext`
- `internal/api/auth_test.go` — update test helpers
- `internal/api/setup.go` — `*pgxpool.Pool` → `*bun.DB`, pgx tx → bun tx, `*pgconn.PgError` → `pgdriver.Error`
- `internal/api/setup_test.go` — update test helpers
- `internal/api/db_error.go` — no pgx imports to change (already clean)
- `internal/api/db_error_test.go` — no changes needed
- `internal/auth/jwt.go` — `*pgxpool.Pool` → `*bun.DB`, `pool.QueryRow` → `db.QueryRowContext`
- `internal/auth/jwt_test.go` — update test helpers

---

## Task 1: Swap Go module dependencies

**Files:**
- Modify: `go.mod`

This task establishes the dependency foundation. No code changes yet — just make sure `go build` still works with the new deps alongside the old ones (old code still compiles because we haven't removed anything yet).

- [ ] **Step 1: Add Bun dependencies**

```bash
cd /home/abo/workspace/home/nexorious-go
go get github.com/uptrace/bun
go get github.com/uptrace/bun/dialect/pgdialect
go get github.com/uptrace/bun/driver/pgdriver
go get github.com/uptrace/bun/migrate
```

- [ ] **Step 2: Verify the build still passes**

Run: `go build ./...`
Expected: clean build (old code still compiles, new deps are just unused for now)

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add uptrace/bun packages for migration"
```

---

## Task 2: Create Bun model structs

**Files:**
- Create: `internal/db/models/models.go`

All model structs live in one file — they're simple data definitions. The spec defines Game explicitly; the rest are derived from `internal/db/gen/models.go` with `pgtype.*` → Go stdlib types per the spec's type mapping table.

- [ ] **Step 1: Create the models file**

```bash
mkdir -p internal/db/models
```

Create `internal/db/models/models.go`:

```go
package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Game struct {
	bun.BaseModel `bun:"table:games"`

	ID                         int32      `bun:"id,pk"                          json:"id"`
	Title                      string     `bun:"title,notnull"                  json:"title"`
	Description                *string    `bun:"description"                    json:"description"`
	Genre                      *string    `bun:"genre"                          json:"genre"`
	Developer                  *string    `bun:"developer"                      json:"developer"`
	Publisher                  *string    `bun:"publisher"                      json:"publisher"`
	ReleaseDate                *time.Time `bun:"release_date"                   json:"release_date"`
	CoverArtUrl                *string    `bun:"cover_art_url"                  json:"cover_art_url"`
	RatingAverage              *float64   `bun:"rating_average"                 json:"rating_average"`
	RatingCount                *int32     `bun:"rating_count"                   json:"rating_count"`
	EstimatedPlaytimeHours     *int32     `bun:"estimated_playtime_hours"       json:"estimated_playtime_hours"`
	HowlongtobeatMain         *float64   `bun:"howlongtobeat_main"             json:"howlongtobeat_main"`
	HowlongtobeatExtra        *float64   `bun:"howlongtobeat_extra"            json:"howlongtobeat_extra"`
	HowlongtobeatCompletionist *float64  `bun:"howlongtobeat_completionist"    json:"howlongtobeat_completionist"`
	IgdbSlug                   *string    `bun:"igdb_slug"                      json:"igdb_slug"`
	IgdbPlatformIds            *string    `bun:"igdb_platform_ids"              json:"igdb_platform_ids"`
	IgdbPlatformNames          *string    `bun:"igdb_platform_names"            json:"igdb_platform_names"`
	GameModes                  *string    `bun:"game_modes"                     json:"game_modes"`
	Themes                     *string    `bun:"themes"                         json:"themes"`
	PlayerPerspectives         *string    `bun:"player_perspectives"            json:"player_perspectives"`
	GameMetadata               *string    `bun:"game_metadata"                  json:"game_metadata"`
	LastUpdated                time.Time  `bun:"last_updated,notnull"           json:"last_updated"`
	CreatedAt                  time.Time  `bun:"created_at,notnull"             json:"created_at"`
}

type User struct {
	bun.BaseModel `bun:"table:users"`

	ID           string    `bun:"id,pk"                json:"id"`
	Username     string    `bun:"username,notnull"     json:"username"`
	PasswordHash string    `bun:"password_hash,notnull" json:"password_hash"`
	IsActive     bool      `bun:"is_active,notnull"    json:"is_active"`
	IsAdmin      bool      `bun:"is_admin,notnull"     json:"is_admin"`
	Preferences  string    `bun:"preferences,notnull"  json:"preferences"`
	CreatedAt    time.Time `bun:"created_at,notnull"   json:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at,notnull"   json:"updated_at"`
}

type UserSession struct {
	bun.BaseModel `bun:"table:user_sessions"`

	ID               string    `bun:"id,pk"                    json:"id"`
	UserID           string    `bun:"user_id,notnull"          json:"user_id"`
	TokenHash        string    `bun:"token_hash,notnull"       json:"token_hash"`
	RefreshTokenHash string    `bun:"refresh_token_hash,notnull" json:"refresh_token_hash"`
	UserAgent        *string   `bun:"user_agent"               json:"user_agent"`
	IpAddress        *string   `bun:"ip_address"               json:"ip_address"`
	CreatedAt        time.Time `bun:"created_at,notnull"       json:"created_at"`
	ExpiresAt        time.Time `bun:"expires_at,notnull"       json:"expires_at"`
}

type UserGame struct {
	bun.BaseModel `bun:"table:user_games"`

	ID             string     `bun:"id,pk"              json:"id"`
	UserID         string     `bun:"user_id,notnull"    json:"user_id"`
	GameID         int32      `bun:"game_id,notnull"    json:"game_id"`
	PlayStatus     *string    `bun:"play_status"        json:"play_status"`
	PersonalRating *int32     `bun:"personal_rating"    json:"personal_rating"`
	IsLoved        bool       `bun:"is_loved,notnull"   json:"is_loved"`
	HoursPlayed    *float64   `bun:"hours_played"       json:"hours_played"`
	PersonalNotes  *string    `bun:"personal_notes"     json:"personal_notes"`
	CreatedAt      time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull" json:"updated_at"`
}

type UserGamePlatform struct {
	bun.BaseModel `bun:"table:user_game_platforms"`

	ID                     string     `bun:"id,pk"                       json:"id"`
	UserGameID             string     `bun:"user_game_id,notnull"        json:"user_game_id"`
	Platform               string     `bun:"platform,notnull"            json:"platform"`
	Storefront             string     `bun:"storefront,notnull"          json:"storefront"`
	StoreGameID            *string    `bun:"store_game_id"               json:"store_game_id"`
	StoreUrl               *string    `bun:"store_url"                   json:"store_url"`
	IsAvailable            bool       `bun:"is_available,notnull"        json:"is_available"`
	HoursPlayed            *float64   `bun:"hours_played"                json:"hours_played"`
	OwnershipStatus        *string    `bun:"ownership_status"            json:"ownership_status"`
	AcquiredDate           *time.Time `bun:"acquired_date"               json:"acquired_date"`
	OriginalPlatformName   *string    `bun:"original_platform_name"      json:"original_platform_name"`
	OriginalStorefrontName *string    `bun:"original_storefront_name"    json:"original_storefront_name"`
	ExternalGameID         *string    `bun:"external_game_id"            json:"external_game_id"`
	SyncFromSource         bool       `bun:"sync_from_source,notnull"    json:"sync_from_source"`
	CreatedAt              time.Time  `bun:"created_at,notnull"          json:"created_at"`
	UpdatedAt              time.Time  `bun:"updated_at,notnull"          json:"updated_at"`
}

type Platform struct {
	bun.BaseModel `bun:"table:platforms"`

	Name              string  `bun:"name,pk"               json:"name"`
	DisplayName       string  `bun:"display_name,notnull"  json:"display_name"`
	Icon              *string `bun:"icon"                  json:"icon"`
	IgdbPlatformID    *int32  `bun:"igdb_platform_id"      json:"igdb_platform_id"`
	DefaultStorefront *string `bun:"default_storefront"    json:"default_storefront"`
}

type Storefront struct {
	bun.BaseModel `bun:"table:storefronts"`

	Name        string  `bun:"name,pk"              json:"name"`
	DisplayName string  `bun:"display_name,notnull" json:"display_name"`
	Icon        *string `bun:"icon"                 json:"icon"`
	BaseUrl     *string `bun:"base_url"             json:"base_url"`
}

type PlatformStorefront struct {
	bun.BaseModel `bun:"table:platform_storefronts"`

	Platform   string `bun:"platform,pk"   json:"platform"`
	Storefront string `bun:"storefront,pk" json:"storefront"`
}

type Tag struct {
	bun.BaseModel `bun:"table:tags"`

	ID        string    `bun:"id,pk"              json:"id"`
	UserID    string    `bun:"user_id,notnull"    json:"user_id"`
	Name      string    `bun:"name,notnull"       json:"name"`
	Color     *string   `bun:"color"              json:"color"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type UserGameTag struct {
	bun.BaseModel `bun:"table:user_game_tags"`

	ID         string    `bun:"id,pk"                json:"id"`
	UserGameID string    `bun:"user_game_id,notnull"  json:"user_game_id"`
	TagID      string    `bun:"tag_id,notnull"        json:"tag_id"`
	CreatedAt  time.Time `bun:"created_at,notnull"    json:"created_at"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/db/models/...`
Expected: clean build

- [ ] **Step 3: Commit**

```bash
git add internal/db/models/models.go
git commit -m "feat: add Bun model structs for all current tables"
```

---

## Task 3: Replace migration infrastructure

**Files:**
- Modify: `internal/db/migrations/migrations.go`
- Rename: `internal/db/migrations/0001_initial.up.sql` → `20260503000001_initial.up.sql`
- Rename: `internal/db/migrations/0001_initial.down.sql` → `20260503000001_initial.down.sql`

- [ ] **Step 1: Rename migration files to timestamp format**

```bash
cd /home/abo/workspace/home/nexorious-go/internal/db/migrations
mv 0001_initial.up.sql 20260503000001_initial.up.sql
mv 0001_initial.down.sql 20260503000001_initial.down.sql
```

- [ ] **Step 2: Rewrite migrations.go for Bun**

Replace `internal/db/migrations/migrations.go` with:

```go
package migrations

import (
	"embed"

	"github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var sqlFiles embed.FS

var Migrations = migrate.NewMigrations()

func init() {
	if err := Migrations.Discover(sqlFiles); err != nil {
		panic(err)
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/db/migrations/...`
Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations/
git commit -m "refactor: switch migration files to Bun timestamp format and Discover API"
```

---

## Task 4: Rewrite migrator.go

This is the largest single task. The spec gives exact code for every method. The struct fields change, `NewMigrator` takes `*bun.DB` instead of a URL, and all methods that touch golang-migrate are replaced.

**Files:**
- Modify: `internal/migrate/migrator.go`

- [ ] **Step 1: Rewrite migrator.go**

Replace the entire file. Key changes from the current code:

**Imports:** Remove `golang-migrate`, `iofs`, `pgxpool`, `strings`. Add `bun`, `bun/migrate`, `database/sql`.

**Struct:**
```go
type Migrator struct {
	state             atomic.Int32
	prevState         atomic.Int32
	lastUnavailableAt atomic.Value
	needsSetup        bool
	mu                sync.RWMutex
	migrateMu         sync.Mutex
	probeInterval     time.Duration
	db                *bun.DB
	bunMig            *bunmigrate.Migrator // nil until first determineState() call
	logCh             chan string
	logWriter         io.Writer
}
```

**NewMigrator:** Takes `*bun.DB`, no error return (no iofs source to create):
```go
func NewMigrator(db *bun.DB) *Migrator {
	return &Migrator{db: db}
}
```

**determineState():** Per spec — lazy-init `bunMig`, call `MigrationsWithStatus`, check `Unapplied()`. No dirty flag.

**PendingCount():** Per spec — guard against nil `bunMig`, call `MigrationsWithStatus`.

**CurrentVersion():** Remove entirely — Bun has no version/dirty concept. The handler that calls it (`HandleStatus`, `HandleMigrateUI`) will be updated in Task 5.

**RunMigrations():** Per spec — acquire lock, call `Migrate`, use `sendLog` helper.

**Close():** Returns nil (Bun migrator holds no independent connection).

**recoverFromUnavailable():** Change `mg.m` nil-reset block to `mg.bunMig = nil`.

**StartDBProbe:** Change second parameter from `*pgxpool.Pool` to `*bun.DB`. Change `pool.Ping(pingCtx)` to `db.PingContext(pingCtx)`.

**InitNeedsSetup:** Change from `pool.QueryRow(ctx, ...)` to `db.QueryRowContext(ctx, ...)`.

**Remove:** `logAdapter` struct (replaced by `sendLog` method), `CurrentVersion()` method.

**NewMigratorForTest:** Stays the same (no DB needed).

The full rewritten file content:

```go
package migrate

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uptrace/bun"
	bunmigrate "github.com/uptrace/bun/migrate"

	"github.com/drzero42/nexorious-go/internal/db/migrations"
)

// AppState represents the application migration state.
type AppState int32

const (
	AppStateDBUnavailable  AppState = iota // MUST be 0 — sentinel for prevState
	AppStateNeedsMigration
	AppStateMigrating
	AppStateReady
)

func (s AppState) String() string {
	switch s {
	case AppStateDBUnavailable:
		return "db_unavailable"
	case AppStateNeedsMigration:
		return "needs_migration"
	case AppStateMigrating:
		return "migrating"
	case AppStateReady:
		return "ready"
	default:
		return "unknown"
	}
}

// Migrator manages migration state.
type Migrator struct {
	state             atomic.Int32
	prevState         atomic.Int32 // state before DBUnavailable; zero == never operational
	lastUnavailableAt atomic.Value // stores time.Time
	needsSetup        bool
	mu                sync.RWMutex // guards needsSetup
	migrateMu         sync.Mutex   // held by RunMigrations for its entire duration
	probeInterval     time.Duration
	db                *bun.DB
	bunMig            *bunmigrate.Migrator // nil until first determineState() call
	logCh             chan string
	logWriter         io.Writer
}

// NewMigrator creates a Migrator ready to use.
// It does NOT connect to the database — state is DBUnavailable (zero value)
// until DetermineStateForTest() or determineState() is called.
func NewMigrator(db *bun.DB) *Migrator {
	return &Migrator{db: db}
}

func (mg *Migrator) determineState() error {
	if mg.bunMig == nil {
		mg.bunMig = bunmigrate.NewMigrator(mg.db, migrations.Migrations)
		if err := mg.bunMig.Init(context.Background()); err != nil {
			return fmt.Errorf("determine state: init: %w", err)
		}
	}
	ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
	if err != nil {
		return fmt.Errorf("determine state: %w", err)
	}
	if len(ms.Unapplied()) > 0 {
		mg.state.Store(int32(AppStateNeedsMigration))
	} else {
		mg.state.Store(int32(AppStateReady))
	}
	return nil
}

// DetermineStateForTest calls determineState and is intended for tests only.
func (mg *Migrator) DetermineStateForTest() error {
	return mg.determineState()
}

// TransitionToReady atomically sets state to Ready.
func (mg *Migrator) TransitionToReady() {
	mg.state.Store(int32(AppStateReady))
}

// State returns the current AppState atomically.
func (mg *Migrator) State() AppState {
	return AppState(mg.state.Load())
}

// PendingCount returns the number of migrations not yet applied.
func (mg *Migrator) PendingCount() (int, error) {
	if mg.bunMig == nil {
		if err := mg.determineState(); err != nil {
			return 0, fmt.Errorf("pending count: init: %w", err)
		}
	}
	ms, err := mg.bunMig.MigrationsWithStatus(context.Background())
	if err != nil {
		return 0, fmt.Errorf("pending count: %w", err)
	}
	return len(ms.Unapplied()), nil
}

// LogCh returns the current log channel (nil if RunMigrations has not been called).
func (mg *Migrator) LogCh() <-chan string {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()
	return mg.logCh
}

// SetLogWriter sets a writer to receive migration log output instead of logCh.
func (mg *Migrator) SetLogWriter(w io.Writer) {
	mg.logWriter = w
}

// RunMigrations applies all pending migrations and transitions state accordingly.
func (mg *Migrator) RunMigrations(ctx context.Context) error {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()

	if AppState(mg.state.Load()) == AppStateMigrating {
		return fmt.Errorf("migrations already in progress")
	}

	ch := make(chan string, 256)
	mg.logCh = ch
	mg.state.Store(int32(AppStateMigrating))

	if err := mg.bunMig.Lock(ctx); err != nil {
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return fmt.Errorf("migrate: acquire lock: %w", err)
	}
	defer mg.bunMig.Unlock(ctx) //nolint:errcheck

	group, err := mg.bunMig.Migrate(ctx)
	if err != nil {
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", err))
		mg.state.Store(int32(AppStateNeedsMigration))
		close(ch)
		return err
	}
	if group.IsZero() {
		mg.sendLog(ch, "No new migrations to run\n")
	} else {
		mg.sendLog(ch, fmt.Sprintf("Migrated to group %s\n", group))
	}
	close(ch)
	return nil
}

func (mg *Migrator) sendLog(ch chan string, line string) {
	if mg.logWriter != nil {
		_, _ = fmt.Fprint(mg.logWriter, line)
		return
	}
	select {
	case ch <- line:
	default: // drop if buffer full
	}
}

// Close releases resources held by the Migrator.
func (mg *Migrator) Close() error { return nil }

// SetStateForTest sets the state atomically (for tests only).
func (mg *Migrator) SetStateForTest(s AppState) {
	mg.state.Store(int32(s))
}

// NewMigratorForTest creates a Migrator with the given state for testing middleware.
func NewMigratorForTest(s AppState) *Migrator {
	mg := &Migrator{}
	mg.state.Store(int32(s))
	return mg
}

// NeedsSetup returns true if no admin user has been created yet.
func (mg *Migrator) NeedsSetup() bool {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return mg.needsSetup
}

// SetNeedsSetup sets the needsSetup flag.
func (mg *Migrator) SetNeedsSetup(v bool) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	mg.needsSetup = v
}

// InitNeedsSetup queries the users table and sets needsSetup = (count == 0).
func (mg *Migrator) InitNeedsSetup(ctx context.Context, db *bun.DB) error {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("InitNeedsSetup: %w", err)
	}
	mg.SetNeedsSetup(count == 0)
	return nil
}

// LastUnavailableAt returns the time the DB was last detected as unavailable.
func (mg *Migrator) LastUnavailableAt() time.Time {
	v := mg.lastUnavailableAt.Load()
	if v == nil {
		return time.Time{}
	}
	return v.(time.Time)
}

// SetProbeIntervalForTest overrides the probe ticker interval for unit tests.
func (mg *Migrator) SetProbeIntervalForTest(d time.Duration) {
	mg.probeInterval = d
}

// StartDBProbe polls db.PingContext() on a configurable interval and manages the
// DBUnavailable state. onRecovery is called when the DB first comes back.
func (mg *Migrator) StartDBProbe(ctx context.Context, db *bun.DB, onRecovery func(context.Context) error) {
	interval := mg.probeInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := db.PingContext(pingCtx)
			cancel()

			if err != nil {
				if AppState(mg.state.Load()) != AppStateDBUnavailable {
					mg.prevState.Store(mg.state.Load())
					mg.state.Store(int32(AppStateDBUnavailable))
					mg.lastUnavailableAt.Store(time.Now())
					slog.Warn("database unavailable", "err", err)
				}
			} else {
				if AppState(mg.state.Load()) == AppStateDBUnavailable {
					prev := AppState(mg.prevState.Load())
					if err := mg.recoverFromUnavailable(ctx, db, prev, onRecovery); err != nil {
						slog.Error("db probe: recovery failed, remaining in DBUnavailable", "err", err)
					}
				}
			}
		}
	}()
}

func (mg *Migrator) recoverFromUnavailable(ctx context.Context, db *bun.DB, prev AppState, onRecovery func(context.Context) error) error {
	switch prev {
	case AppStateDBUnavailable:
		if err := onRecovery(ctx); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (first init)")

	case AppStateMigrating:
		if err := mg.determineState(); err != nil {
			return err
		}
		slog.Info("db probe: recovery complete (re-determined state after migrating)", "state", mg.State())

	default:
		if mg.bunMig != nil {
			mg.bunMig = nil
		}
		if err := mg.determineState(); err != nil {
			return err
		}
		if prev == AppStateReady && mg.NeedsSetup() {
			if err := mg.InitNeedsSetup(ctx, db); err != nil {
				mg.state.Store(int32(AppStateDBUnavailable))
				return fmt.Errorf("re-check needsSetup: %w", err)
			}
		}
		slog.Info("db probe: recovery complete (re-determined state)", "state", mg.State())
	}
	return nil
}
```

- [ ] **Step 2: Verify it compiles (expect errors in consumers — that's ok)**

Run: `go build ./internal/migrate/...`
Expected: clean build for this package. Downstream consumers (`cmd/nexorious`, `internal/api`) will break — fixed in later tasks.

- [ ] **Step 3: Commit**

```bash
git add internal/migrate/migrator.go
git commit -m "refactor: rewrite migrator to use bun/migrate instead of golang-migrate"
```

---

## Task 5: Update migrate handler.go

**Files:**
- Modify: `internal/migrate/handler.go`

The handler changes `*pgxpool.Pool` → `*bun.DB` and drops `CurrentVersion()` / `dirty` references (Bun has no version/dirty concept).

- [ ] **Step 1: Rewrite handler.go**

Key changes:
- `Handler.pool *pgxpool.Pool` → `Handler.db *bun.DB`
- `NewHandler(m *Migrator, pool *pgxpool.Pool)` → `NewHandler(m *Migrator, db *bun.DB)`
- `HandleMigrateUI`: Remove `CurrentVersion()` call. Show `PendingCount` only.
- `HandleStatus`: Remove `current_version` and `dirty` from JSON response. Return `pending_count` and `state`.
- `HandleRun`: Change `h.pool` → `h.db` in the `InitNeedsSetup` call.

```go
package migrate

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious-go/ui"
)

// Handler holds the Echo handlers for migration routes.
type Handler struct {
	migrator *Migrator
	db       *bun.DB
	tmpl     *template.Template
}

// NewHandler creates a Handler. Panics if the migration template cannot be parsed.
func NewHandler(m *Migrator, db *bun.DB) *Handler {
	tmpl, err := template.ParseFS(ui.MigrateBox, "migrate/migrate.html")
	if err != nil {
		panic(fmt.Sprintf("migrate: failed to parse template: %v", err))
	}
	return &Handler{migrator: m, db: db, tmpl: tmpl}
}

// Migrator exposes the underlying Migrator for tests.
func (h *Handler) Migrator() *Migrator { return h.migrator }

// HandleMigrateUI renders the migration UI page.
// GET /migrate
func (h *Handler) HandleMigrateUI(c *echo.Context) error {
	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}

	data := struct {
		PendingCount int
	}{
		PendingCount: pending,
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
	return h.tmpl.Execute(c.Response(), data)
}

// HandleStatus returns migration status as JSON.
// GET /api/migrate/status
func (h *Handler) HandleStatus(c *echo.Context) error {
	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"pending_count": pending,
		"state":         h.migrator.State().String(),
	})
}

// HandleRun triggers migration asynchronously.
// POST /api/migrate/run
func (h *Handler) HandleRun(c *echo.Context) error {
	switch h.migrator.State() {
	case AppStateMigrating:
		return c.JSON(http.StatusConflict, map[string]string{"error": "migration already in progress"})
	case AppStateReady:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "already up to date"})
	}

	go func() {
		if err := h.migrator.RunMigrations(context.Background()); err != nil {
			_ = err
			return
		}
		if h.db != nil {
			if err := h.migrator.InitNeedsSetup(context.Background(), h.db); err != nil {
				_ = err
			}
		}
		h.migrator.TransitionToReady()
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "migration started"})
}

// HandleProgress streams migration log lines as Server-Sent Events.
// GET /api/migrate/progress
func (h *Handler) HandleProgress(c *echo.Context) error {
	ch := h.migrator.LogCh()
	if ch == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "no migration in progress"})
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	for line := range ch {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	_, _ = fmt.Fprintf(w, "event: complete\ndata: {}\n\n")
	flusher.Flush()
	return nil
}
```

- [ ] **Step 2: Note — the migrate HTML template references `{{.CurrentVersion}}`**

Check `ui/migrate/migrate.html` for references to `CurrentVersion`. If present, remove that reference from the template (replace with just showing pending count). This is a mechanical template change.

```bash
grep -n "CurrentVersion" ui/migrate/migrate.html
```

If found, remove the line or replace with pending-only display.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/migrate/...`
Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add internal/migrate/handler.go
# If template was changed:
# git add ui/migrate/migrate.html
git commit -m "refactor: update migrate handler for bun (remove CurrentVersion/dirty)"
```

---

## Task 6: Rewrite migrator_test.go

**Files:**
- Modify: `internal/migrate/migrator_test.go`

All tests must use `*bun.DB` instead of pgxpool. The `setupTestDB` helper returns a `*bun.DB` instead of a connection string. No more `pgx5://` scheme rewriting.

- [ ] **Step 1: Rewrite migrator_test.go**

Key changes to each function:

**`setupTestDB`** — returns `*bun.DB` instead of `string`:
```go
func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("nexorious_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}
```

**Imports:** Replace golang-migrate, iofs, pgxpool, pgx/stdlib with bun, pgdialect, pgdriver.

**All test functions:** Change `NewMigrator(connStr)` → `NewMigrator(db)`. Remove `m.Close()` calls (Close is now a no-op). Remove `setupPool` helper. Remove `badPool` helper — replace with a `badDB` helper:
```go
func badDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb := sql.OpenDB(pgdriver.NewConnector(
		pgdriver.WithDSN("postgres://bad:bad@127.0.0.1:19999/x?sslmode=disable&connect_timeout=1"),
	))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	return db
}
```

**`TestNewMigrator_SucceedsWhenDBUnreachable`:** `NewMigrator` no longer returns an error, so change to just `m := migrate.NewMigrator(badDB(t))`.

**`TestRunMigrations_AllTablesExist`:** Use `db.QueryRowContext` instead of opening a separate `sql.DB`.

**`TestStartDBProbe_*`:** Pass `badDB(t)` instead of `badPool(t)` to `StartDBProbe`.

**`TestInitNeedsSetup_*`:** Pass `db` (from `setupTestDB`) directly to `InitNeedsSetup` instead of creating a separate pool.

- [ ] **Step 2: Verify tests compile**

Run: `go build ./internal/migrate/...`
Expected: clean build

- [ ] **Step 3: Run tests**

Run: `go test ./internal/migrate/... -v -count=1`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/migrate/migrator_test.go
git commit -m "test: update migrator tests for bun"
```

---

## Task 7: Update handler_test.go for migrate package

**Files:**
- Modify: `internal/migrate/handler_test.go`

- [ ] **Step 1: Update handler_test.go**

Change `NewHandler(m, pool)` → `NewHandler(m, db)` where `db` is from `setupTestDB`. Remove references to `CurrentVersion` or `dirty` in assertions. Update the `HandleStatus` response assertions to match the new JSON shape (no `current_version`, no `dirty`).

- [ ] **Step 2: Run tests**

Run: `go test ./internal/migrate/... -v -count=1`
Expected: all tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/migrate/handler_test.go
git commit -m "test: update handler tests for bun"
```

---

## Task 8: Update auth.go — pgxpool to bun.DB

**Files:**
- Modify: `internal/api/auth.go`

Every `pool.QueryRow(ctx, ...)` becomes `h.db.QueryRowContext(ctx, ...)`. Every `pool.Exec(ctx, ...)` becomes `h.db.ExecContext(ctx, ...)`. The `pgx.ErrNoRows` sentinel changes to `sql.ErrNoRows`.

- [ ] **Step 1: Update auth.go**

Key changes:
- Import: Remove `pgx/v5`, `pgx/v5/pgxpool`. Add `database/sql`, `github.com/uptrace/bun`.
- `AuthHandler.pool *pgxpool.Pool` → `AuthHandler.db *bun.DB`
- `NewAuthHandler(pool *pgxpool.Pool, cfg *config.Config)` → `NewAuthHandler(db *bun.DB, cfg *config.Config)`
- All `h.pool.QueryRow(context.Background(), sql, args...)` → `h.db.QueryRowContext(context.Background(), sql, args...)`
- All `h.pool.Exec(context.Background(), sql, args...)` → `h.db.ExecContext(context.Background(), sql, args...)`
- All `errors.Is(err, pgx.ErrNoRows)` → `errors.Is(err, sql.ErrNoRows)`
- `issueTokensAndSession`: same changes for the `pool` parameter → `db *bun.DB`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/api/...`
Expected: may still fail due to downstream callers — that's ok for now

- [ ] **Step 3: Commit**

```bash
git add internal/api/auth.go
git commit -m "refactor: switch auth handler from pgxpool to bun.DB"
```

---

## Task 9: Update auth/jwt.go — pgxpool to bun.DB

**Files:**
- Modify: `internal/auth/jwt.go`

- [ ] **Step 1: Update jwt.go**

Key changes:
- Import: Remove `pgx/v5`, `pgx/v5/pgxpool`. Add `database/sql`, `github.com/uptrace/bun`.
- `JWTMiddleware(secretKey string, pool *pgxpool.Pool)` → `JWTMiddleware(secretKey string, db *bun.DB)`
- `pool.QueryRow(c.Request().Context(), sql, args...).Scan(...)` → `db.QueryRowContext(c.Request().Context(), sql, args...).Scan(...)`
- `errors.Is(err, pgx.ErrNoRows)` → `errors.Is(err, sql.ErrNoRows)`

- [ ] **Step 2: Commit**

```bash
git add internal/auth/jwt.go
git commit -m "refactor: switch JWT middleware from pgxpool to bun.DB"
```

---

## Task 10: Update setup.go — pgxpool to bun.DB + pgdriver error type

**Files:**
- Modify: `internal/api/setup.go`

This file has the most nuanced change: the PostgreSQL error type detection switches from `*pgconn.PgError` to `pgdriver.Error`.

- [ ] **Step 1: Update setup.go**

Key changes:
- Import: Remove `pgx/v5`, `pgx/v5/pgconn`, `pgx/v5/pgxpool`. Add `database/sql`, `github.com/uptrace/bun`, `github.com/uptrace/bun/driver/pgdriver`.
- `SetupHandler.pool *pgxpool.Pool` → `SetupHandler.db *bun.DB`
- `NewSetupHandler(pool *pgxpool.Pool, ...)` → `NewSetupHandler(db *bun.DB, ...)`
- `tryCreateAdmin`: `h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})` → `h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})`
- Transaction queries: `tx.QueryRow(ctx, sql, args...)` → `tx.QueryRowContext(ctx, sql, args...)`
- `issueTokensAndSession(ctx, h.pool, ...)` → `issueTokensAndSession(ctx, h.db, ...)`
- `isSerializationFailure`: change from `*pgconn.PgError` to:
```go
func isSerializationFailure(err error) bool {
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "40001"
	}
	return false
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/api/setup.go
git commit -m "refactor: switch setup handler from pgxpool to bun.DB, update PgError detection"
```

---

## Task 11: Update router.go — wire bun.DB throughout

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Update router.go**

Key changes:
- Import: Remove `pgx/v5/pgxpool`. Add `github.com/uptrace/bun`.
- `New(cfg, migrator, pool *pgxpool.Pool, resolvedDatabaseURL)` → `New(cfg, migrator, db *bun.DB, resolvedDatabaseURL)`
- `registerRoutes(... pool *pgxpool.Pool ...)` → `registerRoutes(... db *bun.DB ...)`
- `migrate.NewHandler(migrator, pool)` → `migrate.NewHandler(migrator, db)`
- `NewSetupHandler(pool, cfg, migrator)` → `NewSetupHandler(db, cfg, migrator)`
- `NewAuthHandler(pool, cfg)` → `NewAuthHandler(db, cfg)`
- `auth.JWTMiddleware(cfg.SecretKey, pool)` → `auth.JWTMiddleware(cfg.SecretKey, db)`
- The `if pool != nil` guard → `if db != nil`

- [ ] **Step 2: Commit**

```bash
git add internal/api/router.go
git commit -m "refactor: wire bun.DB through router instead of pgxpool"
```

---

## Task 12: Update main.go — replace pgxpool with Bun

**Files:**
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Update main.go**

Key changes:
- Import: Remove `pgx/v5/pgxpool`. Add `database/sql`, `github.com/uptrace/bun`, `github.com/uptrace/bun/dialect/pgdialect`, `github.com/uptrace/bun/driver/pgdriver`.
- Replace pool creation:
```go
// Before
pool, err := pgxpool.New(ctx, resolvedDatabaseURL)

// After
sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(resolvedDatabaseURL)))
db := bun.NewDB(sqldb, pgdialect.New())
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
defer db.Close()
```
- `migrate.NewMigrator(resolvedDatabaseURL)` → `migrate.NewMigrator(db)` (no error return now)
- Remove `migrator.Close()` defer (Close is a no-op)
- `migrator.InitNeedsSetup(ctx, pool)` → `migrator.InitNeedsSetup(ctx, db)`
- `pool.Ping(pingCtx)` → `db.PingContext(pingCtx)`
- `migrator.StartDBProbe(shutdownCtx, pool, ...)` → `migrator.StartDBProbe(shutdownCtx, db, ...)`
- `api.New(cfg, migrator, pool, resolvedDatabaseURL)` → `api.New(cfg, migrator, db, resolvedDatabaseURL)`
- In `--migrate-only` block: same pool→db changes
- Remove explicit `pool.Close()` calls before `os.Exit` — use `db.Close()` instead

- [ ] **Step 2: Verify full build**

Run: `go build ./...`
Expected: clean build

- [ ] **Step 3: Commit**

```bash
git add cmd/nexorious/main.go
git commit -m "refactor: replace pgxpool with bun.DB in main.go"
```

---

## Task 13: Update all test files

**Files:**
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/setup_test.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/auth/jwt_test.go`

- [ ] **Step 1: Update auth_test.go**

The `setupAuthTestDB` helper must:
1. Create a testcontainer
2. Open a `*bun.DB` via `pgdriver`
3. Run migrations using `bunmigrate.NewMigrator(db, migrations.Migrations)` + `.Init()` + `.Migrate()`
4. Return the `*bun.DB`

Remove all golang-migrate imports. Remove `iofs` usage. Replace `pgxpool.New` with Bun setup.

All `pool.Exec` / `pool.QueryRow` calls in helpers (`insertAuthTestUser`, `insertAuthTestSession`) become `db.ExecContext` / `db.QueryRowContext`.

`newTestEcho` passes `db` to `api.New` instead of `pool`.

- [ ] **Step 2: Update setup_test.go**

Same pattern — `setupAuthTestDB` is shared (defined in auth_test.go, same package). Update `api.NewSetupHandler(pool, ...)` → `api.NewSetupHandler(db, ...)`.

- [ ] **Step 3: Update router_test.go**

Update `api.New(cfg, m, pool, "")` calls to pass `db` instead of `pool`.

- [ ] **Step 4: Update jwt_test.go**

If jwt_test.go creates its own pools or references pgxpool, update similarly.

- [ ] **Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/api/auth_test.go internal/api/setup_test.go internal/api/router_test.go internal/auth/jwt_test.go
git commit -m "test: update all test files from pgxpool to bun.DB"
```

---

## Task 14: Create filter package

**Files:**
- Create: `internal/filter/builder.go`
- Create: `internal/filter/criteria.go`

- [ ] **Step 1: Create builder.go**

```go
package filter

import "github.com/uptrace/bun"

// filterBuilder accumulates JOINs, WHERE, and HAVING clauses for dynamic queries.
type filterBuilder struct {
	joins   map[string]string // alias → "LEFT JOIN table AS alias ON ..."
	wheres  []func(*bun.SelectQuery) *bun.SelectQuery
	havings []func(*bun.SelectQuery) *bun.SelectQuery
}

// NewFilterBuilder creates an empty filterBuilder.
func NewFilterBuilder() *filterBuilder {
	return &filterBuilder{joins: make(map[string]string)}
}

// AddJoin registers a JOIN clause, deduplicated by alias.
func (f *filterBuilder) AddJoin(alias, clause string) {
	f.joins[alias] = clause
}

// AddWhere appends a WHERE clause function.
func (f *filterBuilder) AddWhere(fn func(*bun.SelectQuery) *bun.SelectQuery) {
	f.wheres = append(f.wheres, fn)
}

// AddHaving appends a HAVING clause function.
func (f *filterBuilder) AddHaving(fn func(*bun.SelectQuery) *bun.SelectQuery) {
	f.havings = append(f.havings, fn)
}

// Apply applies all accumulated JOINs, WHEREs, and HAVINGs to the query.
func (f *filterBuilder) Apply(q *bun.SelectQuery) *bun.SelectQuery {
	for _, clause := range f.joins {
		q = q.Join(clause)
	}
	for _, fn := range f.wheres {
		q = fn(q)
	}
	for _, fn := range f.havings {
		q = fn(q)
	}
	return q
}
```

- [ ] **Step 2: Create criteria.go**

```go
package filter

import (
	"github.com/uptrace/bun"
)

const (
	joinUserGamePlatforms = "LEFT JOIN user_game_platforms AS ugp ON ugp.user_game_id = ug.id"
	joinGames             = "LEFT JOIN games AS g ON g.id = ug.game_id"
)

// ApplyPlayStatus filters by user_games.play_status.
func ApplyPlayStatus(fb *filterBuilder, status string) {
	if status == "" {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.play_status = ?", status)
	})
}

// ApplyOwnershipStatus filters by user_game_platforms.ownership_status.
func ApplyOwnershipStatus(fb *filterBuilder, status string) {
	if status == "" {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ugp.ownership_status = ?", status)
	})
}

// ApplyIsLoved filters by user_games.is_loved.
func ApplyIsLoved(fb *filterBuilder, v *bool) {
	if v == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.is_loved = ?", *v)
	})
}

// ApplyRatingMin filters by user_games.personal_rating >= min.
func ApplyRatingMin(fb *filterBuilder, min *float64) {
	if min == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.personal_rating >= ?", *min)
	})
}

// ApplyRatingMax filters by user_games.personal_rating <= max.
func ApplyRatingMax(fb *filterBuilder, max *float64) {
	if max == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.personal_rating <= ?", *max)
	})
}

// ApplyHasNotes filters by whether personal_notes is present.
func ApplyHasNotes(fb *filterBuilder, v *bool) {
	if v == nil {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		if *v {
			return q.Where("ug.personal_notes IS NOT NULL AND ug.personal_notes != ''")
		}
		return q.Where("ug.personal_notes IS NULL OR ug.personal_notes = ''")
	})
}

// ApplyPlatform filters by platform name(s). "unknown" maps to NULL.
func ApplyPlatform(fb *filterBuilder, platforms []string) {
	if len(platforms) == 0 {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)

	hasUnknown := false
	known := make([]string, 0, len(platforms))
	for _, p := range platforms {
		if p == "unknown" {
			hasUnknown = true
		} else {
			known = append(known, p)
		}
	}

	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(known) > 0 {
				q = q.WhereOr("ugp.platform IN (?)", bun.In(known))
			}
			if hasUnknown {
				q = q.WhereOr("ugp.platform IS NULL")
			}
			return q
		})
	})
}

// ApplyStorefront filters by storefront name(s). "unknown" maps to NULL.
func ApplyStorefront(fb *filterBuilder, storefronts []string) {
	if len(storefronts) == 0 {
		return
	}
	fb.AddJoin("ugp", joinUserGamePlatforms)

	hasUnknown := false
	known := make([]string, 0, len(storefronts))
	for _, s := range storefronts {
		if s == "unknown" {
			hasUnknown = true
		} else {
			known = append(known, s)
		}
	}

	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			if len(known) > 0 {
				q = q.WhereOr("ugp.storefront IN (?)", bun.In(known))
			}
			if hasUnknown {
				q = q.WhereOr("ugp.storefront IS NULL")
			}
			return q
		})
	})
}

// ApplyGenre filters by game genre (ILIKE match).
func ApplyGenre(fb *filterBuilder, genres []string) {
	if len(genres) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, g := range genres {
				q = q.WhereOr("g.genre ILIKE ?", "%"+g+"%")
			}
			return q
		})
	})
}

// ApplyGameMode filters by game mode (ILIKE match).
func ApplyGameMode(fb *filterBuilder, modes []string) {
	if len(modes) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, m := range modes {
				q = q.WhereOr("g.game_modes ILIKE ?", "%"+m+"%")
			}
			return q
		})
	})
}

// ApplyTheme filters by theme (ILIKE match).
func ApplyTheme(fb *filterBuilder, themes []string) {
	if len(themes) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, t := range themes {
				q = q.WhereOr("g.themes ILIKE ?", "%"+t+"%")
			}
			return q
		})
	})
}

// ApplyPlayerPerspective filters by player perspective (ILIKE match).
func ApplyPlayerPerspective(fb *filterBuilder, perspectives []string) {
	if len(perspectives) == 0 {
		return
	}
	fb.AddJoin("g", joinGames)
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			for _, p := range perspectives {
				q = q.WhereOr("g.player_perspectives ILIKE ?", "%"+p+"%")
			}
			return q
		})
	})
}

// ApplyTag filters by tag IDs via subquery.
func ApplyTag(fb *filterBuilder, tagIDs []string) {
	if len(tagIDs) == 0 {
		return
	}
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("ug.id IN (SELECT user_game_id FROM user_game_tags WHERE tag_id IN (?))", bun.In(tagIDs))
	})
}

// ApplySearch filters by title or personal notes (ILIKE match).
func ApplySearch(fb *filterBuilder, query string) {
	if query == "" {
		return
	}
	fb.AddJoin("g", joinGames)
	pattern := "%" + query + "%"
	fb.AddWhere(func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			q = q.WhereOr("g.title ILIKE ?", pattern)
			q = q.WhereOr("ug.personal_notes IS NOT NULL AND ug.personal_notes ILIKE ?", pattern)
			return q
		})
	})
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/filter/...`
Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add internal/filter/builder.go internal/filter/criteria.go
git commit -m "feat: add filter package with Bun-based dynamic query builder"
```

---

## Task 15: Delete old files and clean up build config

**Files:**
- Delete: `internal/db/gen/` (entire directory)
- Delete: `internal/db/queries/` (entire directory)
- Delete: `sqlc.yaml`
- Modify: `Makefile`
- Modify: `devenv.nix`

- [ ] **Step 1: Delete old generated code and query files**

```bash
rm -rf internal/db/gen internal/db/queries sqlc.yaml
```

- [ ] **Step 2: Update Makefile**

Remove `sqlc` target and remove `sqlc` from `all`:

```makefile
.PHONY: all frontend build test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend build

frontend:
	cd ui && npm install && npm run build

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

test:
	go test ./...
```

- [ ] **Step 3: Update devenv.nix — remove sqlc**

Remove `sqlc` from the packages list.

- [ ] **Step 4: Remove old Go module dependencies**

```bash
go mod tidy
```

This removes `golang-migrate`, `pgx/v5`, `goqu`, `sqlx`, and any other now-unused deps.

- [ ] **Step 5: Verify full build and tests**

Run: `go build ./... && go vet ./...`
Expected: clean

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove sqlc/golang-migrate/pgx deps, clean up Makefile and devenv.nix"
```

---

## Task 16: Final verification

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -count=1 -v
```

Expected: all tests pass

- [ ] **Step 2: Run linter**

```bash
golangci-lint run
```

Expected: no new errors

- [ ] **Step 3: Run build**

```bash
make
```

Expected: clean build (frontend + Go binary)

- [ ] **Step 4: Verify no old imports remain**

```bash
grep -r "pgxpool\|golang-migrate\|goqu\|sqlx\|pgx/v5" --include="*.go" internal/ cmd/ | grep -v "_test.go" | grep -v "vendor/"
```

Expected: no output (all old imports removed)

```bash
grep -r "pgxpool\|golang-migrate\|goqu\|sqlx" --include="*.go" internal/ cmd/
```

Expected: no output at all (including test files)

- [ ] **Step 5: Commit any final fixes if needed**

---

## Summary

| Task | Description | Est. |
|------|------------|------|
| 1 | Add Bun Go module dependencies | 2 min |
| 2 | Create Bun model structs | 5 min |
| 3 | Replace migration infrastructure (rename files, rewrite migrations.go) | 3 min |
| 4 | Rewrite migrator.go | 5 min |
| 5 | Update migrate handler.go | 3 min |
| 6 | Rewrite migrator_test.go | 5 min |
| 7 | Update handler_test.go | 3 min |
| 8 | Update auth.go (pgxpool → bun.DB) | 5 min |
| 9 | Update auth/jwt.go (pgxpool → bun.DB) | 3 min |
| 10 | Update setup.go (pgxpool → bun.DB + pgdriver error) | 5 min |
| 11 | Update router.go (wire bun.DB) | 3 min |
| 12 | Update main.go (replace pgxpool) | 5 min |
| 13 | Update all test files | 10 min |
| 14 | Create filter package | 5 min |
| 15 | Delete old files, clean deps | 3 min |
| 16 | Final verification | 5 min |
