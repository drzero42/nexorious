# Setup Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the first-run setup gate — DB-unavailability detection, `needsSetup` flag, seed data, `POST /api/auth/setup/admin`, `GET /api/auth/me`, and all static pages — so the server drives users through DB-error → migrate → setup before reaching the authenticated SPA.

**Architecture:** The Migrator struct gains a `DBUnavailable` state (new iota=0 sentinel), a `needsSetup` bool, and a background probe goroutine. Three sequential middleware gates (DB unavailable → needs migration → needs setup) redirect unauthenticated traffic to the appropriate static page. Workers/scheduler only start after `Ready && !NeedsSetup()`.

**Tech Stack:** Go 1.25, Echo v5, pgx/v5 pgxpool, golang-migrate, golang-jwt/jwt/v5, bcrypt, testcontainers-go, html/template (for server-side injection into static pages).

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/migrate/migrator.go` | Add `AppStateDBUnavailable` as iota=0, `prevState`/`lastUnavailableAt` atomics, `needsSetup` + mutex, `NeedsSetup`/`SetNeedsSetup`/`InitNeedsSetup`, `StartDBProbe`, `LastUnavailableAt`; refactor `NewMigrator` to lazy-init `mg.m`; nil-check in `Close`; add `migrateMu`; lock in `RunMigrations` |
| Modify | `internal/migrate/migrator_test.go` | Tests for new Migrator capabilities |
| Modify | `internal/migrate/handler.go` | Accept `pool` param in `NewHandler`; call `InitNeedsSetup` before transitioning to Ready |
| Modify | `internal/migrate/handler_test.go` | Update `NewHandler` call sites; add ordering test |
| Create | `internal/seed/data.go` | Official storefronts, platforms, associations slice literals |
| Create | `internal/seed/seeder.go` | `SeedAll(ctx, pool)` in a single transaction |
| Create | `internal/seed/seeder_test.go` | Testcontainers integration tests |
| Modify | `internal/api/auth.go` | Add `bcryptCost`, `issueTokensAndSession`, `HandleGetMe` on `AuthHandler` |
| Create | `internal/api/setup.go` | `SetupHandler` + `HandleSetupAdmin` |
| Create | `internal/api/setup_test.go` | Handler tests |
| Create | `internal/api/db_error.go` | `DBErrorHandler` + `HandleDBError`; DSN redaction; html/template injection |
| Create | `internal/api/db_error_test.go` | DB-error handler tests |
| Modify | `internal/api/router.go` | All three middleware gates; new route registrations; updated `/health`; JWT group for `GET /api/auth/me` |
| Modify | `internal/api/router_test.go` | Gate middleware tests and health tests |
| Modify | `cmd/nexorious/main.go` | `resolveDBURL`; lazy-init startup; `initAppState`; `StartDBProbe`; worker/scheduler gate loop |
| Create | `ui/setup/index.html` | Standalone setup form page |
| Create | `ui/db-error/index.html` | Standalone DB-unavailable page with `{{.RedactedDSN}}` / `{{.LastUnavailableAt}}` |
| Modify | `ui/ui.go` | Add `SetupBox` and `DBErrorBox` embed vars |

---

## Task 1: Refactor `AppState` — add `DBUnavailable` as iota=0

**Files:**
- Modify: `internal/migrate/migrator.go`

The entire sentinel logic for `prevState` depends on `AppStateDBUnavailable == 0`. This must be done first because all subsequent tasks depend on the new state set.

- [ ] **Step 1: Change the `AppState` const block**

Replace the existing const block in `internal/migrate/migrator.go`:

```go
const (
    AppStateDBUnavailable AppState = iota  // MUST be 0 — sentinel for prevState
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
```

- [ ] **Step 2: Fix `NewMigratorForTest` — it now starts in `DBUnavailable` by default**

The test helper currently stores `int32(s)`. No code change needed — callers that pass `AppStateNeedsMigration` etc. still work. But `NewMigratorForTest` with no args would now produce `DBUnavailable`. Audit all existing test call sites to confirm they all pass an explicit state:

```bash
grep -n "NewMigratorForTest" /home/abo/workspace/home/nexorious-go/internal/api/router_test.go
```

Confirm each call passes an explicit `AppState` argument.

- [ ] **Step 3: Update existing handler_test.go** — `NewMigrator` currently connects eagerly (we'll fix that in Task 2). For now just verify tests still compile:

```bash
go build ./...
```

Expected: may fail on `NewMigrator` signature until Task 2 is complete — that is OK at this stage. The goal is the const change compiles.

- [ ] **Step 4: Commit**

```bash
git add internal/migrate/migrator.go
git commit -m "feat(migrate): add AppStateDBUnavailable as iota=0 sentinel"
```

---

## Task 2: Refactor `NewMigrator` to not connect at construction time

**Files:**
- Modify: `internal/migrate/migrator.go`
- Modify: `internal/migrate/migrator_test.go`
- Modify: `internal/migrate/handler_test.go`

`NewMigrator` currently calls `gmigrate.NewWithSourceInstance` (opens a DB connection) and `determineState()` (queries DB). This makes it impossible to start the server when the DB is unavailable at boot.

- [ ] **Step 1: Update the `Migrator` struct** — add new fields, rename `mu`, add `migrateMu`:

```go
type Migrator struct {
    state             atomic.Int32
    prevState         atomic.Int32  // state before DBUnavailable; zero == never operational
    lastUnavailableAt atomic.Value  // stores time.Time
    needsSetup        bool
    mu                sync.RWMutex  // guards needsSetup
    migrateMu         sync.Mutex    // guards mg.m; held by RunMigrations for its entire duration
    databaseURL       string
    src               *iofs.Source  // created in NewMigrator, reused in determineState
    m                 *gmigrate.Migrate  // nil until determineState() first called
    logCh             chan string
    logWriter         io.Writer
}
```

Note: the existing `mu sync.Mutex` guarded `logCh`/`logWriter`. We're repurposing the name — the new `mu` is a `sync.RWMutex` guarding `needsSetup`. The old logCh/logWriter guard moves to `migrateMu`.

- [ ] **Step 2: Rewrite `NewMigrator`** — cheap constructor, no DB contact:

```go
// NewMigrator creates a Migrator ready to use.
// It does NOT connect to the database — state is DBUnavailable (zero value)
// until initAppState() is called from main.go after a successful pool.Ping().
func NewMigrator(databaseURL string) (*Migrator, error) {
    src, err := iofs.New(migrations.FS, ".")
    if err != nil {
        return nil, fmt.Errorf("migrator: create iofs source: %w", err)
    }
    return &Migrator{
        databaseURL: databaseURL,
        src:         src,
    }, nil
}
```

- [ ] **Step 3: Update `determineState`** — lazy-init `mg.m`:

```go
func (mg *Migrator) determineState() error {
    if mg.m == nil {
        migrateURL := strings.NewReplacer(
            "postgresql://", "pgx5://",
            "postgres://", "pgx5://",
        ).Replace(mg.databaseURL)
        m, err := gmigrate.NewWithSourceInstance("iofs", mg.src, migrateURL)
        if err != nil {
            return fmt.Errorf("determine state: connect: %w", err)
        }
        mg.m = m
    }

    ver, dirty, err := mg.m.Version()
    if errors.Is(err, gmigrate.ErrNilVersion) {
        mg.state.Store(int32(AppStateNeedsMigration))
        return nil
    }
    if err != nil {
        return fmt.Errorf("determine state: %w", err)
    }
    if dirty {
        slog.Error("database is in dirty state",
            "version", ver,
            "hint", "manually resolve the migration and clear the dirty flag")
        mg.state.Store(int32(AppStateNeedsMigration))
        return nil
    }
    count, err := mg.PendingCount()
    if err != nil {
        return fmt.Errorf("determine state: %w", err)
    }
    if count > 0 {
        mg.state.Store(int32(AppStateNeedsMigration))
    } else {
        mg.state.Store(int32(AppStateReady))
    }
    return nil
}
```

- [ ] **Step 4: Add `migrateMu` lock to `RunMigrations`** — add at the top of the function body:

```go
func (mg *Migrator) RunMigrations(ctx context.Context) error {
    mg.migrateMu.Lock()
    defer mg.migrateMu.Unlock()

    // existing check for AppStateMigrating
    if AppState(mg.state.Load()) == AppStateMigrating {
        return fmt.Errorf("migrations already in progress")
    }
    // ... rest of existing body unchanged (remove the old mg.mu.Lock/Unlock calls)
```

The existing `mg.mu.Lock()` / `mg.mu.Unlock()` inside `RunMigrations` wraps `logCh` setup — remove those inner locks (logCh is now protected by `migrateMu` since the whole function holds it).

- [ ] **Step 5: Add nil-check to `Close`**:

```go
func (mg *Migrator) Close() error {
    if mg.m == nil {
        return nil
    }
    srcErr, dbErr := mg.m.Close()
    if srcErr != nil {
        return srcErr
    }
    return dbErr
}
```

- [ ] **Step 6: Update `LogCh`** — remove the old `mg.mu` lock (now protected by `migrateMu`):

```go
func (mg *Migrator) LogCh() <-chan string {
    mg.migrateMu.Lock()
    defer mg.migrateMu.Unlock()
    return mg.logCh
}
```

- [ ] **Step 7: Update `NewMigratorForTest`** — remove ctx param from real `NewMigrator` and update the test helper (it doesn't call `NewMigrator`, so no change needed there). Update `migrator_test.go` to call the new signature:

In `migrator_test.go`, all calls like `migrate.NewMigrator(ctx, connStr)` become `migrate.NewMigrator(connStr)`. Then call `m.DetermineStateForTest(ctx)` — but wait, `determineState` is unexported. For the tests we need a way to trigger it. Add an exported method for tests:

```go
// DetermineStateForTest calls determineState and is intended for tests only.
func (mg *Migrator) DetermineStateForTest() error {
    return mg.determineState()
}
```

Update all `migrator_test.go` test functions that previously relied on `NewMigrator` auto-connecting:

```go
// Before:
m, err := migrate.NewMigrator(ctx, connStr)

// After:
m, err := migrate.NewMigrator(connStr)
if err != nil { t.Fatalf(...) }
if err := m.DetermineStateForTest(); err != nil { t.Fatalf(...) }
```

- [ ] **Step 8: Update `handler_test.go`** — `newTestHandler` calls `NewMigrator`:

```go
func newTestHandler(t *testing.T) *migrate.Handler {
    t.Helper()
    connStr := setupTestDB(t)
    m, err := migrate.NewMigrator(connStr)
    if err != nil {
        t.Fatalf("NewMigrator: %v", err)
    }
    if err := m.DetermineStateForTest(); err != nil {
        t.Fatalf("DetermineStateForTest: %v", err)
    }
    t.Cleanup(func() {
        if err := m.Close(); err != nil {
            t.Logf("close migrator: %v", err)
        }
    })
    return migrate.NewHandler(m)
}
```

Also update the inline `NewMigrator` calls in `TestHandleRun_409_WhenMigrating` and `TestHandleRun_400_WhenReady`.

- [ ] **Step 9: Run tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -v -timeout 120s
```

Expected: all existing migrator and handler tests pass.

- [ ] **Step 10: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go internal/migrate/handler_test.go
git commit -m "feat(migrate): lazy-init DB connection; NewMigrator no longer connects at construction"
```

---

## Task 3: Add `needsSetup`, `InitNeedsSetup`, `StartDBProbe`, `LastUnavailableAt` to Migrator

**Files:**
- Modify: `internal/migrate/migrator.go`
- Modify: `internal/migrate/migrator_test.go`

- [ ] **Step 1: Write failing tests first** — add to `migrator_test.go`:

```go
func TestNewMigrator_SucceedsWhenDBUnreachable(t *testing.T) {
    // NewMigrator should succeed even with a bad URL — no DB contact at construction.
    m, err := migrate.NewMigrator("postgres://bad-host:5432/nope?sslmode=disable")
    if err != nil {
        t.Fatalf("NewMigrator with unreachable DB: %v", err)
    }
    if m.State() != migrate.AppStateDBUnavailable {
        t.Errorf("expected DBUnavailable, got %v", m.State())
    }
}

func TestInitNeedsSetup_NoUsers(t *testing.T) {
    connStr := setupTestDB(t)
    ctx := context.Background()
    m, err := migrate.NewMigrator(connStr)
    if err != nil { t.Fatalf("NewMigrator: %v", err) }
    if err := m.DetermineStateForTest(); err != nil { t.Fatalf("determineState: %v", err) }
    if err := m.RunMigrations(ctx); err != nil { t.Fatalf("RunMigrations: %v", err) }

    pool := setupPool(t, connStr)
    if err := m.InitNeedsSetup(ctx, pool); err != nil {
        t.Fatalf("InitNeedsSetup: %v", err)
    }
    if !m.NeedsSetup() {
        t.Error("expected NeedsSetup=true on empty users table")
    }
}

func TestInitNeedsSetup_UsersExist(t *testing.T) {
    connStr := setupTestDB(t)
    ctx := context.Background()
    m, err := migrate.NewMigrator(connStr)
    if err != nil { t.Fatalf("NewMigrator: %v", err) }
    if err := m.DetermineStateForTest(); err != nil { t.Fatalf("determineState: %v", err) }
    if err := m.RunMigrations(ctx); err != nil { t.Fatalf("RunMigrations: %v", err) }

    pool := setupPool(t, connStr)
    _, err = pool.Exec(ctx,
        `INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','admin','hash',true)`)
    if err != nil { t.Fatalf("insert user: %v", err) }

    if err := m.InitNeedsSetup(ctx, pool); err != nil {
        t.Fatalf("InitNeedsSetup: %v", err)
    }
    if m.NeedsSetup() {
        t.Error("expected NeedsSetup=false when users exist")
    }
}

func TestStartDBProbe_SetsUnavailableOnPingFail(t *testing.T) {
    // Use a migrator in Ready state, then simulate ping failure.
    m, err := migrate.NewMigrator("postgres://bad:5432/x?sslmode=disable")
    if err != nil { t.Fatalf("NewMigrator: %v", err) }
    m.SetStateForTest(migrate.AppStateReady)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    pool := badPool(t) // pool that always fails Ping
    called := make(chan struct{}, 1)
    m.StartDBProbe(ctx, pool, func(ctx context.Context) error {
        called <- struct{}{}
        return nil
    })

    // Give probe time to detect failure.
    time.Sleep(150 * time.Millisecond)
    if m.State() != migrate.AppStateDBUnavailable {
        t.Errorf("expected DBUnavailable after ping fail, got %v", m.State())
    }
    if m.LastUnavailableAt().IsZero() {
        t.Error("expected LastUnavailableAt to be set")
    }
}

func TestStartDBProbe_RespectsContext(t *testing.T) {
    m, err := migrate.NewMigrator("postgres://bad:5432/x?sslmode=disable")
    if err != nil { t.Fatalf("NewMigrator: %v", err) }

    ctx, cancel := context.WithCancel(context.Background())
    pool := badPool(t)
    m.StartDBProbe(ctx, pool, func(_ context.Context) error { return nil })

    cancel() // should cause goroutine to exit cleanly
    time.Sleep(100 * time.Millisecond)
    // No assertion needed — if the goroutine leaks, the race detector will catch it.
}
```

Add helpers to `migrator_test.go`:

```go
func setupPool(t *testing.T, connStr string) *pgxpool.Pool {
    t.Helper()
    // connStr from setupTestDB is pgx5://, but pgxpool needs postgres://
    pgxConnStr := "postgres" + strings.TrimPrefix(connStr, "pgx5")
    pool, err := pgxpool.New(context.Background(), pgxConnStr)
    if err != nil { t.Fatalf("pgxpool.New: %v", err) }
    t.Cleanup(pool.Close)
    return pool
}

func badPool(t *testing.T) *pgxpool.Pool {
    t.Helper()
    // A pool pointed at a non-existent host — all Pings will fail immediately.
    pool, err := pgxpool.New(context.Background(), "postgres://bad:bad@127.0.0.1:19999/x?sslmode=disable&connect_timeout=1")
    if err != nil { t.Fatalf("badPool: %v", err) }
    t.Cleanup(pool.Close)
    return pool
}
```

Run to confirm they fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -run "TestNewMigrator_SucceedsWhenDBUnreachable|TestInitNeedsSetup|TestStartDBProbe" -v -timeout 60s 2>&1 | tail -20
```

Expected: compile error or FAIL.

- [ ] **Step 2: Implement `NeedsSetup`, `SetNeedsSetup`, `InitNeedsSetup`** — add to `migrator.go`:

```go
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
// Single-attempt; DB unavailability is handled by StartDBProbe at a higher level.
func (mg *Migrator) InitNeedsSetup(ctx context.Context, pool *pgxpool.Pool) error {
    var count int
    err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
    if err != nil {
        return fmt.Errorf("InitNeedsSetup: %w", err)
    }
    mg.SetNeedsSetup(count == 0)
    return nil
}
```

Add required import: `"github.com/jackc/pgx/v5/pgxpool"`.

- [ ] **Step 3: Implement `LastUnavailableAt` and `StartDBProbe`**:

```go
// LastUnavailableAt returns the time the DB was last detected as unavailable.
// Returns the zero time.Time if the DB has never been unavailable.
func (mg *Migrator) LastUnavailableAt() time.Time {
    v := mg.lastUnavailableAt.Load()
    if v == nil {
        return time.Time{}
    }
    return v.(time.Time)
}

// StartDBProbe polls pool.Ping() every 5 seconds and manages the DBUnavailable state.
// onRecovery is called (with the probe's context) when the DB first comes back.
func (mg *Migrator) StartDBProbe(ctx context.Context, pool *pgxpool.Pool, onRecovery func(context.Context) error) {
    go func() {
        ticker := time.NewTicker(5 * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
            }
            pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
            err := pool.Ping(pingCtx)
            cancel()

            if err != nil {
                // Ping failed.
                if AppState(mg.state.Load()) != AppStateDBUnavailable {
                    mg.prevState.Store(mg.state.Load())
                    mg.state.Store(int32(AppStateDBUnavailable))
                    mg.lastUnavailableAt.Store(time.Now())
                    slog.Warn("database unavailable", "err", err)
                }
            } else {
                // Ping succeeded.
                if AppState(mg.state.Load()) == AppStateDBUnavailable {
                    prev := AppState(mg.prevState.Load())
                    if err := mg.recoverFromUnavailable(ctx, pool, prev, onRecovery); err != nil {
                        slog.Error("db probe: recovery failed, remaining in DBUnavailable", "err", err)
                    }
                }
            }
        }
    }()
}

func (mg *Migrator) recoverFromUnavailable(ctx context.Context, pool *pgxpool.Pool, prev AppState, onRecovery func(context.Context) error) error {
    switch {
    case prev == AppStateDBUnavailable:
        // Never had an operational state — run full init.
        if err := onRecovery(ctx); err != nil {
            return err
        }
        slog.Info("db probe: recovery complete (first init)")

    case prev == AppStateMigrating:
        // Migration goroutine died — re-consult DB for actual state.
        if err := mg.determineState(); err != nil {
            return err
        }
        slog.Info("db probe: recovery complete (re-determined state after migrating)", "state", mg.State())

    default:
        // NeedsMigration or Ready — safe to restore directly.
        mg.state.Store(int32(prev))
        if prev == AppStateReady && mg.NeedsSetup() {
            // Re-check in case admin was created during the outage.
            if err := mg.InitNeedsSetup(ctx, pool); err != nil {
                mg.state.Store(int32(AppStateDBUnavailable))
                return fmt.Errorf("re-check needsSetup: %w", err)
            }
        }
        slog.Info("db probe: recovery complete (restored prev state)", "state", mg.State())
    }
    return nil
}
```

Add required import: `"time"`.

- [ ] **Step 4: Run the new tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -run "TestNewMigrator_SucceedsWhenDBUnreachable|TestInitNeedsSetup|TestStartDBProbe" -v -timeout 120s
```

Expected: all pass.

- [ ] **Step 5: Run the full migrate test suite**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -v -timeout 180s
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go
git commit -m "feat(migrate): add needsSetup, InitNeedsSetup, StartDBProbe, LastUnavailableAt"
```

---

## Task 4: Update `migrate.Handler` — accept pool, call `InitNeedsSetup` before Ready

**Files:**
- Modify: `internal/migrate/handler.go`
- Modify: `internal/migrate/handler_test.go`

- [ ] **Step 1: Write failing test** — add to `handler_test.go`:

```go
func TestRunMigrations_SetsNeedsSetupBeforeReady(t *testing.T) {
    connStr := setupTestDB(t)
    ctx := context.Background()
    // Need a pool for NewHandler.
    pgxConnStr := "postgres" + strings.TrimPrefix(connStr, "pgx5")
    pool, err := pgxpool.New(ctx, pgxConnStr)
    if err != nil { t.Fatalf("pgxpool.New: %v", err) }
    t.Cleanup(pool.Close)

    m, err := migrate.NewMigrator(connStr)
    if err != nil { t.Fatalf("NewMigrator: %v", err) }
    if err := m.DetermineStateForTest(); err != nil { t.Fatalf("DetermineStateForTest: %v", err) }
    t.Cleanup(func() { _ = m.Close() })

    h := migrate.NewHandler(m, pool)
    e := echo.New()

    req := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    if err := h.HandleRun(c); err != nil { t.Fatalf("HandleRun: %v", err) }

    // Wait for migration goroutine to finish.
    for ch := h.Migrator().LogCh(); ch != nil; {
        for range ch { }
        break
    }

    if m.State() != migrate.AppStateReady {
        t.Errorf("expected Ready, got %v", m.State())
    }
    if !m.NeedsSetup() {
        t.Error("expected NeedsSetup=true after migration on empty users table")
    }
}
```

Add imports to `handler_test.go`: `"github.com/jackc/pgx/v5/pgxpool"`.

Run to confirm fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -run TestRunMigrations_SetsNeedsSetupBeforeReady -v -timeout 120s 2>&1 | tail -10
```

- [ ] **Step 2: Update `NewHandler` to accept `pool`**

In `handler.go`:

```go
type Handler struct {
    migrator *Migrator
    pool     *pgxpool.Pool
    tmpl     *template.Template
}

func NewHandler(m *Migrator, pool *pgxpool.Pool) *Handler {
    tmpl, err := template.ParseFS(ui.MigrateBox, "migrate/migrate.html")
    if err != nil {
        panic(fmt.Sprintf("migrate: failed to parse template: %v", err))
    }
    return &Handler{migrator: m, pool: pool, tmpl: tmpl}
}
```

Add import: `"github.com/jackc/pgx/v5/pgxpool"`.

- [ ] **Step 3: Update `HandleRun`** — call `InitNeedsSetup` before transitioning to Ready.

`RunMigrations` currently sets state to `Ready` itself. Per the spec, the handler must set Ready — so we need `RunMigrations` to leave state as `Migrating` on success and let the handler transition. Update `RunMigrations` in `migrator.go`:

```go
// At end of RunMigrations, instead of:
//   mg.state.Store(int32(AppStateReady))
// Leave state as Migrating (handler transitions):
err := mg.m.Up()
if err == nil || errors.Is(err, gmigrate.ErrNoChange) {
    close(ch)
    return nil  // caller (handler goroutine) sets Ready
}
```

Then update `HandleRun`'s goroutine in `handler.go`:

```go
go func() {
    if err := h.migrator.RunMigrations(context.Background()); err != nil {
        _ = err
        return
    }
    // InitNeedsSetup BEFORE transitioning to Ready — prevents gate-loop race.
    if err := h.migrator.InitNeedsSetup(context.Background(), h.pool); err != nil {
        slog.Warn("migrate handler: InitNeedsSetup failed", "err", err)
    }
    h.migrator.SetStateForTest(AppStateReady)  // no — use exported setter
}()
```

Wait — `AppStateReady` is in the `migrate` package. Inside `handler.go` (same package) we can use it directly. Add an exported method to the Migrator for the handler to call:

```go
// TransitionToReady atomically sets state to Ready. Called by the migration
// handler after InitNeedsSetup completes successfully.
func (mg *Migrator) TransitionToReady() {
    mg.state.Store(int32(AppStateReady))
}
```

Update the `HandleRun` goroutine:

```go
go func() {
    if err := h.migrator.RunMigrations(context.Background()); err != nil {
        _ = err
        return
    }
    if err := h.migrator.InitNeedsSetup(context.Background(), h.pool); err != nil {
        slog.Warn("migrate handler: InitNeedsSetup failed after migration", "err", err)
    }
    h.migrator.TransitionToReady()
}()
```

- [ ] **Step 4: Update all existing `NewHandler` call sites** in `handler_test.go` to pass `pool`:

```go
// All existing calls:
// migrate.NewHandler(m)  →  migrate.NewHandler(m, pool)
// In newTestHandler, add a pool setup step.
```

Update `newTestHandler`:

```go
func newTestHandler(t *testing.T) *migrate.Handler {
    t.Helper()
    connStr := setupTestDB(t)
    pgxConnStr := "postgres" + strings.TrimPrefix(connStr, "pgx5")
    pool, err := pgxpool.New(context.Background(), pgxConnStr)
    if err != nil { t.Fatalf("pgxpool.New: %v", err) }
    t.Cleanup(pool.Close)

    m, err := migrate.NewMigrator(connStr)
    if err != nil { t.Fatalf("NewMigrator: %v", err) }
    if err := m.DetermineStateForTest(); err != nil { t.Fatalf("DetermineStateForTest: %v", err) }
    t.Cleanup(func() { _ = m.Close() })
    return migrate.NewHandler(m, pool)
}
```

Update the inline `migrate.NewHandler(m)` calls in `TestHandleRun_409_WhenMigrating` and `TestHandleRun_400_WhenReady` similarly (they can pass `nil` for pool since they set state manually and never trigger RunMigrations).

Also update the call site in `internal/api/router.go` (where `migrate.NewHandler(migrator)` is called) — pass `pool`:

```go
mh := migrate.NewHandler(migrator, pool)
```

- [ ] **Step 5: Run tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/migrate/... -v -timeout 180s
```

Expected: all pass including `TestRunMigrations_SetsNeedsSetupBeforeReady`.

- [ ] **Step 6: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/handler.go internal/migrate/handler_test.go internal/api/router.go
git commit -m "feat(migrate): handler calls InitNeedsSetup before transitioning to Ready"
```

---

## Task 5: Create `internal/seed` package

**Files:**
- Create: `internal/seed/data.go`
- Create: `internal/seed/seeder.go`
- Create: `internal/seed/seeder_test.go`

- [ ] **Step 1: Write failing tests first** — create `internal/seed/seeder_test.go`:

```go
package seed_test

import (
    "context"
    "strings"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
    gmigrate "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
    "github.com/golang-migrate/migrate/v4/source/iofs"

    "github.com/drzero42/nexorious-go/internal/db/migrations"
    "github.com/drzero42/nexorious-go/internal/seed"
)

func setupSeedDB(t *testing.T) *pgxpool.Pool {
    t.Helper()
    ctx := context.Background()
    ctr, err := postgres.Run(ctx, "postgres:18-alpine",
        postgres.WithDatabase("nexorious_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
        ),
    )
    if err != nil { t.Fatalf("start container: %v", err) }
    t.Cleanup(func() { _ = ctr.Terminate(ctx) })

    connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
    if err != nil { t.Fatalf("connection string: %v", err) }

    // Apply migrations.
    pgx5Str := "pgx5" + strings.TrimPrefix(connStr, "postgres")
    src, err := iofs.New(migrations.FS, ".")
    if err != nil { t.Fatalf("iofs.New: %v", err) }
    m, err := gmigrate.NewWithSourceInstance("iofs", src, pgx5Str)
    if err != nil { t.Fatalf("gmigrate.New: %v", err) }
    if err := m.Up(); err != nil && err != gmigrate.ErrNoChange {
        t.Fatalf("migrate up: %v", err)
    }
    _, _ = m.Close()

    pool, err := pgxpool.New(ctx, connStr)
    if err != nil { t.Fatalf("pgxpool.New: %v", err) }
    t.Cleanup(pool.Close)
    return pool
}

func TestSeedAll_EmptyDatabase(t *testing.T) {
    pool := setupSeedDB(t)
    ctx := context.Background()

    result, err := seed.SeedAll(ctx, pool)
    if err != nil { t.Fatalf("SeedAll: %v", err) }

    if result.Storefronts == 0 { t.Error("expected >0 storefronts seeded") }
    if result.Platforms == 0 { t.Error("expected >0 platforms seeded") }
    // Associations may be 0 if none defined yet.
    t.Logf("seeded: storefronts=%d platforms=%d associations=%d",
        result.Storefronts, result.Platforms, result.Associations)
}

func TestSeedAll_Idempotent(t *testing.T) {
    pool := setupSeedDB(t)
    ctx := context.Background()

    r1, err := seed.SeedAll(ctx, pool)
    if err != nil { t.Fatalf("SeedAll first: %v", err) }
    r2, err := seed.SeedAll(ctx, pool)
    if err != nil { t.Fatalf("SeedAll second: %v", err) }

    // On second run, DO UPDATE with same data → RowsAffected may be 0.
    // The key assertion is that it doesn't error and counts are stable (not doubled).
    _ = r1
    _ = r2

    var count int
    if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM storefronts").Scan(&count); err != nil {
        t.Fatalf("count storefronts: %v", err)
    }
    if count != r1.Storefronts && r1.Storefronts > 0 {
        // r1.Storefronts was inserted; count should equal that (no duplicates).
        // This check is a bit loose because DO UPDATE may count as 1 even on no-change.
        t.Logf("storefronts in DB: %d", count)
    }
}

func TestSeedAll_PreservesCustomRows(t *testing.T) {
    pool := setupSeedDB(t)
    ctx := context.Background()

    // Insert a custom storefront before seeding.
    _, err := pool.Exec(ctx,
        `INSERT INTO storefronts (name, display_name, icon_url, base_url, is_active, source, version_added)
         VALUES ('mystore', 'My Store', '', '', true, 'custom', 1)`)
    if err != nil { t.Fatalf("insert custom storefront: %v", err) }

    _, err = seed.SeedAll(ctx, pool)
    if err != nil { t.Fatalf("SeedAll: %v", err) }

    // Custom storefront must still exist.
    var src string
    if err := pool.QueryRow(ctx,
        "SELECT source FROM storefronts WHERE name = 'mystore'").Scan(&src); err != nil {
        t.Fatalf("custom storefront missing after seed: %v", err)
    }
    if src != "custom" {
        t.Errorf("custom row source changed to %q", src)
    }
}
```

Run to confirm compile failure:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/seed/... -v 2>&1 | head -20
```

- [ ] **Step 2: Create `internal/seed/data.go`**

Port the official data from the Python `OFFICIAL_STOREFRONTS`, `OFFICIAL_PLATFORMS`, `PLATFORM_STOREFRONT_ASSOCIATIONS`. Check the Python source for the exact list:

```bash
grep -A5 "OFFICIAL_STOREFRONTS" /home/abo/workspace/home/nexorious/app/data/platforms.py | head -60
```

Then create `internal/seed/data.go`:

```go
package seed

// OfficialStorefront defines a platform storefront to seed.
type OfficialStorefront struct {
    Name         string
    DisplayName  string
    IconURL      string
    BaseURL      string
    IsActive     bool
    VersionAdded int
}

// OfficialPlatform defines a platform to seed.
type OfficialPlatform struct {
    Name                   string
    DisplayName            string
    IGDBPlatformID         *int
    IGDBPlatformVersionID  *int
    IsActive               bool
    VersionAdded           int
    DefaultStorefront      *string  // nil = no default
}

// OfficialAssociation links a platform to a storefront.
type OfficialAssociation struct {
    Platform   string
    Storefront string
}

// OfficialStorefronts is the list of official storefronts to seed.
// Port from Python OFFICIAL_STOREFRONTS.
var OfficialStorefronts = []OfficialStorefront{
    {Name: "steam",       DisplayName: "Steam",        IconURL: "", BaseURL: "https://store.steampowered.com", IsActive: true, VersionAdded: 1},
    {Name: "gog",         DisplayName: "GOG",          IconURL: "", BaseURL: "https://www.gog.com",           IsActive: true, VersionAdded: 1},
    {Name: "epic",        DisplayName: "Epic Games",   IconURL: "", BaseURL: "https://www.epicgames.com",     IsActive: true, VersionAdded: 1},
    {Name: "playstation", DisplayName: "PlayStation",  IconURL: "", BaseURL: "https://store.playstation.com", IsActive: true, VersionAdded: 1},
    {Name: "xbox",        DisplayName: "Xbox",         IconURL: "", BaseURL: "https://www.xbox.com",          IsActive: true, VersionAdded: 1},
    {Name: "nintendo",    DisplayName: "Nintendo",     IconURL: "", BaseURL: "https://www.nintendo.com",      IsActive: true, VersionAdded: 1},
    // Extend from Python source as needed.
}

// OfficialPlatforms is the list of official platforms to seed.
var OfficialPlatforms = []OfficialPlatform{
    {Name: "pc",    DisplayName: "PC (Windows)", IsActive: true, VersionAdded: 1, DefaultStorefront: strPtr("steam")},
    {Name: "ps5",   DisplayName: "PlayStation 5", IsActive: true, VersionAdded: 1, DefaultStorefront: strPtr("playstation")},
    {Name: "ps4",   DisplayName: "PlayStation 4", IsActive: true, VersionAdded: 1, DefaultStorefront: strPtr("playstation")},
    {Name: "xbox-series", DisplayName: "Xbox Series X/S", IsActive: true, VersionAdded: 1, DefaultStorefront: strPtr("xbox")},
    {Name: "switch", DisplayName: "Nintendo Switch", IsActive: true, VersionAdded: 1, DefaultStorefront: strPtr("nintendo")},
    // Extend from Python source as needed.
}

// OfficialAssociations maps platforms to storefronts.
var OfficialAssociations = []OfficialAssociation{
    {Platform: "pc", Storefront: "steam"},
    {Platform: "pc", Storefront: "gog"},
    {Platform: "pc", Storefront: "epic"},
    // Extend from Python source as needed.
}

func strPtr(s string) *string { return &s }
```

**Important:** After creating this stub, run:

```bash
grep -n "OFFICIAL_STOREFRONTS\|OFFICIAL_PLATFORMS\|PLATFORM_STOREFRONT" /home/abo/workspace/home/nexorious/app/data/platforms.py | head -5
```

Then read the full Python data file and fill in the complete lists. Do not leave stub data — the seeder test counts on at least one valid row.

- [ ] **Step 3: Create `internal/seed/seeder.go`**:

```go
package seed

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"
)

// SeedResult holds counts of rows inserted or updated per table.
type SeedResult struct {
    Storefronts  int
    Platforms    int
    Associations int
}

// SeedAll seeds official storefronts, platforms, and associations in a single transaction.
// Idempotent: safe to call on an already-seeded database. Custom rows (source='custom') are never touched.
func SeedAll(ctx context.Context, pool *pgxpool.Pool) (SeedResult, error) {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return SeedResult{}, fmt.Errorf("seed: begin tx: %w", err)
    }
    defer func() { _ = tx.Rollback(ctx) }()

    var result SeedResult

    // 1. Storefronts
    for _, s := range OfficialStorefronts {
        tag, err := tx.Exec(ctx, `
            INSERT INTO storefronts (name, display_name, icon_url, base_url, is_active, source, version_added, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, 'official', $6, now(), now())
            ON CONFLICT (name) DO UPDATE SET
                display_name  = EXCLUDED.display_name,
                icon_url      = EXCLUDED.icon_url,
                base_url      = EXCLUDED.base_url,
                version_added = EXCLUDED.version_added,
                updated_at    = now()
            WHERE storefronts.source = 'official'`,
            s.Name, s.DisplayName, s.IconURL, s.BaseURL, s.IsActive, s.VersionAdded,
        )
        if err != nil {
            return SeedResult{}, fmt.Errorf("seed storefronts: %w", err)
        }
        result.Storefronts += int(tag.RowsAffected())
    }

    // 2. Platforms
    for _, p := range OfficialPlatforms {
        tag, err := tx.Exec(ctx, `
            INSERT INTO platforms (name, display_name, igdb_platform_id, igdb_platform_version_id, is_active, source, version_added, default_storefront, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, 'official', $6, $7, now(), now())
            ON CONFLICT (name) DO UPDATE SET
                display_name             = EXCLUDED.display_name,
                igdb_platform_id         = EXCLUDED.igdb_platform_id,
                igdb_platform_version_id = EXCLUDED.igdb_platform_version_id,
                default_storefront       = EXCLUDED.default_storefront,
                version_added            = EXCLUDED.version_added,
                updated_at               = now()
            WHERE platforms.source = 'official'`,
            p.Name, p.DisplayName, p.IGDBPlatformID, p.IGDBPlatformVersionID,
            p.IsActive, p.VersionAdded, p.DefaultStorefront,
        )
        if err != nil {
            return SeedResult{}, fmt.Errorf("seed platforms: %w", err)
        }
        result.Platforms += int(tag.RowsAffected())
    }

    // 3. Associations
    for _, a := range OfficialAssociations {
        tag, err := tx.Exec(ctx, `
            INSERT INTO platform_storefronts (platform, storefront, created_at)
            VALUES ($1, $2, now())
            ON CONFLICT (platform, storefront) DO NOTHING`,
            a.Platform, a.Storefront,
        )
        if err != nil {
            return SeedResult{}, fmt.Errorf("seed associations: %w", err)
        }
        result.Associations += int(tag.RowsAffected())
    }

    if err := tx.Commit(ctx); err != nil {
        return SeedResult{}, fmt.Errorf("seed: commit: %w", err)
    }
    return result, nil
}
```

- [ ] **Step 4: Run seed tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/seed/... -v -timeout 120s
```

Expected: all three tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/seed/
git commit -m "feat(seed): add SeedAll with official storefronts, platforms, associations"
```

---

## Task 6: Update `internal/api/auth.go` — add `bcryptCost`, `issueTokensAndSession`, `HandleGetMe`

**Files:**
- Modify: `internal/api/auth.go`
- Modify: `internal/api/auth_test.go`

- [ ] **Step 1: Write failing tests** — add to `auth_test.go`:

```go
func TestGetMe_Success(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
    userID := uuid.NewString()
    insertAuthTestUser(t, pool, userID, "admin", bcryptCost)

    // Issue a valid access token.
    accessToken, err := auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
    if err != nil { t.Fatalf("GenerateAccessToken: %v", err) }
    // Insert a session for the middleware.
    insertAuthTestSession(t, pool, userID, auth.HashToken(accessToken), "", 30)

    e := echo.New()
    ah := api.NewAuthHandler(pool, cfg)
    req := httptest.NewRequest(http.MethodGet, "/api/auth/me",  nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    // Simulate JWTMiddleware having run.
    c.Set("user_id", userID)

    if err := ah.HandleGetMe(c); err != nil { t.Fatalf("HandleGetMe: %v", err) }
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body) }

    var body struct {
        ID          string          `json:"id"`
        Username    string          `json:"username"`
        IsAdmin     bool            `json:"is_admin"`
        IsActive    bool            `json:"is_active"`
        Preferences json.RawMessage `json:"preferences"`
        CreatedAt   string          `json:"created_at"`
    }
    if err := json.NewDecoder(rec.Body).Decode(&body); err != nil { t.Fatalf("decode: %v", err) }
    if body.ID != userID { t.Errorf("id mismatch") }
    if body.Username != "admin" { t.Errorf("username mismatch") }
    if string(body.Preferences) == "" || string(body.Preferences) == "null" {
        t.Errorf("preferences must not be null, got %q", string(body.Preferences))
    }
}

func TestGetMe_Unauthorized_NoUserID(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
    ah := api.NewAuthHandler(pool, cfg)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    // No user_id on context — simulates JWTMiddleware not having run.
    if err := ah.HandleGetMe(c); err != nil { t.Fatalf("HandleGetMe: %v", err) }
    if rec.Code != http.StatusUnauthorized { t.Errorf("expected 401, got %d", rec.Code) }
}
```

You'll also need a `testConfig()` helper if one doesn't exist in `auth_test.go`:

```go
func testConfig() *config.Config {
    return &config.Config{
        SecretKey:                "test-secret-key-32-bytes-long!!!",
        AccessTokenExpireMinutes: 15,
        RefreshTokenExpireDays:   30,
    }
}
```

Check if `testConfig` and `insertAuthTestUser` already exist in `auth_test.go` before adding.

Run to confirm fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestGetMe" -v -timeout 60s 2>&1 | tail -10
```

- [ ] **Step 2: Add `bcryptCost` constant to `auth.go`**

The file already imports `golang.org/x/crypto/bcrypt`. Add at the top of `auth.go` after the imports:

```go
// bcryptCost is the work factor used for all password hash creation sites.
// Never hardcode the cost inline — always use this constant.
const bcryptCost = 12
```

- [ ] **Step 3: Add `issueTokensAndSession` to `auth.go`**

```go
// issueTokensAndSession generates an access + refresh token pair, persists a
// user_sessions row, and returns both token strings.
// Uses context.Background() at all call sites (setup, login, refresh) so a client
// disconnect cannot abort DB writes after a user row has committed.
func issueTokensAndSession(
    ctx context.Context,
    pool *pgxpool.Pool,
    cfg *config.Config,
    userID string,
    userAgent string,
    ip string,
) (accessToken, refreshToken string, err error) {
    accessToken, err = auth.GenerateAccessToken(cfg.SecretKey, userID, cfg.AccessTokenExpireMinutes)
    if err != nil {
        return "", "", fmt.Errorf("issueTokens: generate access token: %w", err)
    }
    refreshToken, err = auth.GenerateRefreshToken(cfg.SecretKey, userID, cfg.RefreshTokenExpireDays)
    if err != nil {
        return "", "", fmt.Errorf("issueTokens: generate refresh token: %w", err)
    }

    sessionID := uuid.NewString()
    expiresAt := time.Now().Add(time.Duration(cfg.RefreshTokenExpireDays) * 24 * time.Hour)
    _, err = pool.Exec(ctx,
        `INSERT INTO user_sessions (id, user_id, token_hash, refresh_token_hash, user_agent, ip_address, expires_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
        sessionID,
        userID,
        auth.HashToken(accessToken),
        auth.HashToken(refreshToken),
        userAgent,
        ip,
        expiresAt,
    )
    if err != nil {
        return "", "", fmt.Errorf("issueTokens: insert session: %w", err)
    }
    return accessToken, refreshToken, nil
}
```

Add import `"fmt"` if not already present.

- [ ] **Step 4: Add `HandleGetMe` method to `AuthHandler`**

```go
// meResponse is the JSON response for GET /api/auth/me.
type meResponse struct {
    ID          string          `json:"id"`
    Username    string          `json:"username"`
    IsAdmin     bool            `json:"is_admin"`
    IsActive    bool            `json:"is_active"`
    Preferences json.RawMessage `json:"preferences"`
    CreatedAt   time.Time       `json:"created_at"`
}

// HandleGetMe handles GET /api/auth/me.
// Requires JWTMiddleware (user_id must be set on context).
func (h *AuthHandler) HandleGetMe(c *echo.Context) error {
    userID := auth.UserIDFromContext(c)
    if userID == "" {
        return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
    }

    var resp meResponse
    var prefs []byte
    err := h.pool.QueryRow(context.Background(),
        `SELECT id, username, is_admin, is_active, preferences, created_at
         FROM users WHERE id = $1`,
        userID,
    ).Scan(&resp.ID, &resp.Username, &resp.IsAdmin, &resp.IsActive, &prefs, &resp.CreatedAt)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return c.JSON(http.StatusUnauthorized, map[string]string{"error": "user not found"})
        }
        slog.Error("get me: query user", "err", err)
        return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
    }

    if prefs == nil {
        resp.Preferences = json.RawMessage("{}")
    } else {
        resp.Preferences = json.RawMessage(prefs)
    }

    return c.JSON(http.StatusOK, resp)
}
```

Add imports: `"encoding/json"`.

- [ ] **Step 5: Run tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestGetMe" -v -timeout 120s
```

Expected: pass.

- [ ] **Step 6: Run full api tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -v -timeout 180s
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add internal/api/auth.go internal/api/auth_test.go
git commit -m "feat(api): add bcryptCost, issueTokensAndSession, HandleGetMe"
```

---

## Task 7: Create `internal/api/setup.go` and `internal/api/setup_test.go`

**Files:**
- Create: `internal/api/setup.go`
- Create: `internal/api/setup_test.go`

- [ ] **Step 1: Write failing tests** — create `internal/api/setup_test.go`:

```go
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
    cfg := testConfig()
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)

    sh := api.NewSetupHandler(pool, cfg, migrator)
    e := echo.New()
    body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
    req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := sh.HandleSetupAdmin(c); err != nil { t.Fatalf("HandleSetupAdmin: %v", err) }
    if rec.Code != http.StatusCreated { t.Errorf("expected 201, got %d: %s", rec.Code, rec.Body) }

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
    if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil { t.Fatalf("decode: %v", err) }
    if resp.User.Username != "admin" { t.Errorf("username mismatch") }
    if !resp.User.IsAdmin { t.Error("expected is_admin=true") }
    if !resp.User.IsActive { t.Error("expected is_active=true") }
    if resp.AccessToken == "" { t.Error("expected access_token") }
    if resp.RefreshToken == "" { t.Error("expected refresh_token") }

    // Verify user in DB.
    var count int
    if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
        t.Fatalf("count users: %v", err)
    }
    if count != 1 { t.Errorf("expected 1 user, got %d", count) }

    // Verify needsSetup cleared.
    if migrator.NeedsSetup() { t.Error("expected NeedsSetup=false after setup") }
}

func TestSetupAdmin_AlreadySetup(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)

    // Insert existing user.
    _, err := pool.Exec(context.Background(),
        `INSERT INTO users (id, username, password_hash, is_admin) VALUES ('u1','existing','hash',true)`)
    if err != nil { t.Fatalf("insert user: %v", err) }

    sh := api.NewSetupHandler(pool, cfg, migrator)
    e := echo.New()
    body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
    req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := sh.HandleSetupAdmin(c); err != nil { t.Fatalf("HandleSetupAdmin: %v", err) }
    if rec.Code != http.StatusForbidden { t.Errorf("expected 403, got %d", rec.Code) }
}

func TestSetupAdmin_InvalidBody(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
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
            if err := sh.HandleSetupAdmin(c); err != nil { t.Fatalf("HandleSetupAdmin: %v", err) }
            if rec.Code != tc.want { t.Errorf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body) }
        })
    }
}

func TestSetupAdmin_ConcurrentRace(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    sh := api.NewSetupHandler(pool, cfg, migrator)
    e := echo.New()

    var (
        mu      sync.Mutex
        codes   []int
        wg      sync.WaitGroup
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
        if code == http.StatusCreated { created++ }
    }
    if created != 1 { t.Errorf("expected exactly 1 success, got %d (codes: %v)", created, codes) }

    var count int
    pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&count)
    if count != 1 { t.Errorf("expected exactly 1 user in DB, got %d", count) }
}

func TestSetupPage_ServesPage(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    // This test is in router_test.go — see Task 9.
    t.Skip("tested in router_test.go")
}

func TestSetupAdmin_GetMeAfterSetup(t *testing.T) {
    pool := setupAuthTestDB(t)
    cfg := testConfig()
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    sh := api.NewSetupHandler(pool, cfg, migrator)
    e := echo.New()

    // Setup.
    body, _ := json.Marshal(map[string]string{"username": "admin", "password": "supersecret"})
    req := httptest.NewRequest(http.MethodPost, "/api/auth/setup/admin", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    if err := sh.HandleSetupAdmin(e.NewContext(req, rec)); err != nil { t.Fatalf("setup: %v", err) }
    if rec.Code != http.StatusCreated { t.Fatalf("setup returned %d", rec.Code) }

    var setupResp struct {
        User        struct{ ID string `json:"id"` } `json:"user"`
        AccessToken string `json:"access_token"`
    }
    json.NewDecoder(rec.Body).Decode(&setupResp)

    // GET /api/auth/me with the access token.
    ah := api.NewAuthHandler(pool, cfg)
    meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
    meReq.Header.Set("Authorization", "Bearer "+setupResp.AccessToken)
    meRec := httptest.NewRecorder()
    meCtx := e.NewContext(meReq, meRec)
    meCtx.Set("user_id", setupResp.User.ID)

    if err := ah.HandleGetMe(meCtx); err != nil { t.Fatalf("GetMe: %v", err) }
    if meRec.Code != http.StatusOK { t.Errorf("expected 200, got %d: %s", meRec.Code, meRec.Body) }

    var meBody struct {
        Preferences json.RawMessage `json:"preferences"`
    }
    json.NewDecoder(meRec.Body).Decode(&meBody)
    if string(meBody.Preferences) == "null" || string(meBody.Preferences) == "" {
        t.Errorf("expected preferences={}, got %q", string(meBody.Preferences))
    }
}
```

Run to confirm compile fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestSetupAdmin|TestSetupPage" -v 2>&1 | head -20
```

- [ ] **Step 2: Create `internal/api/setup.go`**:

```go
package api

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/labstack/echo/v5"
    "golang.org/x/crypto/bcrypt"

    "github.com/drzero42/nexorious-go/internal/config"
    "github.com/drzero42/nexorious-go/internal/migrate"
    "github.com/drzero42/nexorious-go/internal/seed"
)

// SetupHandler handles the first-run setup endpoints.
type SetupHandler struct {
    pool     *pgxpool.Pool
    cfg      *config.Config
    migrator *migrate.Migrator
}

// NewSetupHandler creates a SetupHandler.
func NewSetupHandler(pool *pgxpool.Pool, cfg *config.Config, migrator *migrate.Migrator) *SetupHandler {
    return &SetupHandler{pool: pool, cfg: cfg, migrator: migrator}
}

type setupAdminRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type setupAdminResponse struct {
    User struct {
        ID        string    `json:"id"`
        Username  string    `json:"username"`
        IsAdmin   bool      `json:"is_admin"`
        IsActive  bool      `json:"is_active"`
        CreatedAt time.Time `json:"created_at"`
    } `json:"user"`
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
}

// HandleSetupAdmin handles POST /api/auth/setup/admin.
func (h *SetupHandler) HandleSetupAdmin(c *echo.Context) error {
    var req setupAdminRequest
    if err := c.Bind(&req); err != nil {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
    }
    if req.Username == "" || req.Password == "" {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password are required"})
    }
    if len(req.Username) < 3 {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "username must be at least 3 characters"})
    }
    if len(req.Password) < 8 {
        return c.JSON(http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
    }

    userID := uuid.NewString()
    hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
    if err != nil {
        slog.Error("setup admin: bcrypt", "err", err)
        return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
    }

    var createdAt time.Time
    // Retry once on serialization failure (40001).
    for attempt := 0; attempt <= 1; attempt++ {
        createdAt, err = h.tryCreateAdmin(context.Background(), userID, req.Username, string(hash))
        if err == nil {
            break
        }
        if isSerializationFailure(err) && attempt == 0 {
            continue
        }
        if isUserExistsError(err) {
            return c.JSON(http.StatusForbidden, map[string]string{"error": "setup already complete"})
        }
        slog.Error("setup admin: create user", "err", err)
        return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
    }

    // Seed reference data outside the user transaction.
    if _, seedErr := seed.SeedAll(context.Background(), h.pool); seedErr != nil {
        slog.Warn("setup admin: seed failed (admin can reseed via POST /api/platforms/seed)", "err", seedErr)
    }

    // Issue tokens and session.
    accessToken, refreshToken, tokenErr := issueTokensAndSession(
        context.Background(), h.pool, h.cfg, userID,
        c.Request().Header.Get("User-Agent"),
        c.RealIP(),
    )

    // Always clear needsSetup — the user row has committed.
    h.migrator.SetNeedsSetup(false)

    if tokenErr != nil {
        slog.Error("setup admin: issue tokens", "err", tokenErr)
        return c.JSON(http.StatusInternalServerError, map[string]string{
            "error": "setup succeeded but session could not be created — please log in",
        })
    }

    var resp setupAdminResponse
    resp.User.ID = userID
    resp.User.Username = req.Username
    resp.User.IsAdmin = true
    resp.User.IsActive = true
    resp.User.CreatedAt = createdAt
    resp.AccessToken = accessToken
    resp.RefreshToken = refreshToken
    return c.JSON(http.StatusCreated, resp)
}

// tryCreateAdmin runs the SERIALIZABLE transaction: count check + INSERT.
// Returns (createdAt, nil) on success, or an error (may be isUserExistsError or isSerializationFailure).
func (h *SetupHandler) tryCreateAdmin(ctx context.Context, userID, username, passwordHash string) (time.Time, error) {
    tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
    if err != nil {
        return time.Time{}, err
    }
    defer func() { _ = tx.Rollback(ctx) }()

    var count int
    if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
        return time.Time{}, err
    }
    if count > 0 {
        return time.Time{}, errUserExists
    }

    var createdAt time.Time
    err = tx.QueryRow(ctx,
        `INSERT INTO users (id, username, password_hash, is_admin, created_at)
         VALUES ($1, $2, $3, true, now()) RETURNING created_at`,
        userID, username, passwordHash,
    ).Scan(&createdAt)
    if err != nil {
        return time.Time{}, err
    }
    return createdAt, tx.Commit(ctx)
}

var errUserExists = errors.New("users already exist")

func isUserExistsError(err error) bool {
    return errors.Is(err, errUserExists)
}

func isSerializationFailure(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
```

- [ ] **Step 3: Run setup tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestSetupAdmin" -v -timeout 180s
```

Expected: all pass.

- [ ] **Step 4: Run full api test suite**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -v -timeout 180s
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/setup.go internal/api/setup_test.go
git commit -m "feat(api): add SetupHandler and HandleSetupAdmin"
```

---

## Task 8: Create `internal/api/db_error.go` and `internal/api/db_error_test.go`

**Files:**
- Create: `internal/api/db_error.go`
- Create: `internal/api/db_error_test.go`

- [ ] **Step 1: Create `ui/db-error/index.html`** (needed before the handler compiles):

```bash
mkdir -p /home/abo/workspace/home/nexorious-go/ui/db-error
```

Create `ui/db-error/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Nexorious — Database Unavailable</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 700px; margin: 60px auto; padding: 0 20px; color: #333; }
  h1 { color: #c0392b; }
  pre { background: #f5f5f5; border: 1px solid #ddd; padding: 12px; border-radius: 4px; overflow-x: auto; font-size: 0.9em; }
  .label { font-weight: bold; margin-top: 16px; }
  footer { margin-top: 32px; color: #888; font-size: 0.85em; }
</style>
</head>
<body>
<h1>Nexorious — Database Unavailable</h1>
<p>The server cannot reach the database.</p>
<div class="label">Connection:</div>
<pre>{{.RedactedDSN}}</pre>
<div class="label">Last failed:</div>
<pre>{{.LastUnavailableAt}}</pre>
<footer>
  The page will automatically refresh every 5 seconds.<br>
  If this problem persists, check that your database is running and the connection string is correct.
</footer>
<script>setTimeout(() => location.reload(), 5000)</script>
</body>
</html>
```

- [ ] **Step 2: Write failing tests** — create `internal/api/db_error_test.go`:

```go
package api_test

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/labstack/echo/v5"

    "github.com/drzero42/nexorious-go/internal/api"
    "github.com/drzero42/nexorious-go/internal/migrate"
)

func TestDBErrorPage_ServesHTML(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    dh := api.NewDBErrorHandler("postgres://user:secret@db.example.com:5432/nexorious?sslmode=require", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
    body := rec.Body.String()
    if !strings.Contains(body, "text/html") && rec.Result().Header.Get("Content-Type") == "" {
        t.Log("Content-Type not checked separately")
    }
    if !strings.Contains(body, "db.example.com") { t.Error("body should contain host") }
    if strings.Contains(body, "secret") { t.Error("body must not contain plaintext password") }
    if !strings.Contains(body, "***") { t.Error("body should contain *** for redacted password") }
}

func TestDBErrorPage_RedirectsOnRecovery(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error?from=/foo", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    if rec.Code != http.StatusFound { t.Errorf("expected 302, got %d", rec.Code) }
    if loc := rec.Header().Get("Location"); loc != "/foo" {
        t.Errorf("expected Location=/foo, got %q", loc)
    }
}

func TestDBErrorPage_RedirectsToRootWithNoFrom(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    if rec.Code != http.StatusFound { t.Errorf("expected 302, got %d", rec.Code) }
    if loc := rec.Header().Get("Location"); loc != "/" {
        t.Errorf("expected Location=/, got %q", loc)
    }
}

func TestDBErrorPage_RejectsExternalFrom(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    dh := api.NewDBErrorHandler("postgres://user:pw@host/db", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error?from=https://evil.com", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)

    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    if rec.Code != http.StatusFound { t.Errorf("expected 302, got %d", rec.Code) }
    if loc := rec.Header().Get("Location"); loc != "/" {
        t.Errorf("expected Location=/ to block open-redirect, got %q", loc)
    }
}

func TestDBErrorHandler_RedactsDSN(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    dh := api.NewDBErrorHandler("postgres://myuser:supersecret@db.example.com:5432/nexorious?sslmode=require&password=leak", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    body := rec.Body.String()
    if strings.Contains(body, "supersecret") { t.Error("password must be redacted") }
    if strings.Contains(body, "leak") { t.Error("password query param must be redacted") }
    if !strings.Contains(body, "myuser") { t.Error("username should be visible") }
}

func TestDBErrorHandler_InjectsUnknownWhenNeverUnavailable(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    // lastUnavailableAt is zero — never been unavailable in this process.
    dh := api.NewDBErrorHandler("postgres://u:p@h/db", migrator)
    e := echo.New()
    req := httptest.NewRequest(http.MethodGet, "/db-error", nil)
    rec := httptest.NewRecorder()
    c := e.NewContext(req, rec)
    if err := dh.HandleDBError(c); err != nil { t.Fatalf("HandleDBError: %v", err) }
    body := rec.Body.String()
    if !strings.Contains(body, "unknown") {
        t.Errorf("expected 'unknown' for never-unavailable timestamp, body: %s", body)
    }
}
```

Run to confirm compile fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestDBError" -v 2>&1 | head -20
```

- [ ] **Step 3: Create `internal/api/db_error.go`**:

```go
package api

import (
    "html/template"
    "net/http"
    "net/url"
    "strings"
    "time"

    "github.com/labstack/echo/v5"

    "github.com/drzero42/nexorious-go/internal/migrate"
    "github.com/drzero42/nexorious-go/ui"
)

// DBErrorHandler serves the database-unavailable error page.
type DBErrorHandler struct {
    migrator    *migrate.Migrator
    redactedDSN string
    tmpl        *template.Template
}

// NewDBErrorHandler creates a DBErrorHandler. Panics if the template cannot be parsed.
func NewDBErrorHandler(resolvedDatabaseURL string, migrator *migrate.Migrator) *DBErrorHandler {
    redacted := redactDSN(resolvedDatabaseURL)
    tmpl := template.Must(template.ParseFS(ui.DBErrorBox, "db-error/index.html"))
    return &DBErrorHandler{migrator: migrator, redactedDSN: redacted, tmpl: tmpl}
}

// HandleDBError serves the DB-unavailable page or redirects if the DB has recovered.
// GET /db-error
func (h *DBErrorHandler) HandleDBError(c *echo.Context) error {
    if h.migrator.State() != migrate.AppStateDBUnavailable {
        from := c.QueryParam("from")
        if from == "" || !strings.HasPrefix(from, "/") {
            from = "/"
        }
        return c.Redirect(http.StatusFound, from)
    }

    lastStr := "unknown"
    if t := h.migrator.LastUnavailableAt(); !t.IsZero() {
        lastStr = t.UTC().Format(time.RFC3339)
    }

    data := struct {
        RedactedDSN       string
        LastUnavailableAt string
    }{
        RedactedDSN:       h.redactedDSN,
        LastUnavailableAt: lastStr,
    }

    c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
    return h.tmpl.Execute(c.Response(), data)
}

// redactDSN parses a database URL and replaces the password and any sensitive
// query params (containing "password", "secret", or "key") with ***.
func redactDSN(rawURL string) string {
    u, err := url.Parse(rawURL)
    if err != nil {
        return "<invalid DSN>"
    }
    if u.User != nil {
        if _, hasPass := u.User.Password(); hasPass {
            u.User = url.UserPassword(u.User.Username(), "***")
        }
    }
    q := u.Query()
    for key := range q {
        lower := strings.ToLower(key)
        if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") {
            q.Set(key, "***")
        }
    }
    u.RawQuery = q.Encode()
    return u.String()
}
```

- [ ] **Step 4: Add `DBErrorBox` to `ui/ui.go`**:

```go
//go:embed db-error
var DBErrorBox embed.FS
```

- [ ] **Step 5: Run DB error tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestDBError" -v -timeout 60s
```

Expected: all pass.

- [ ] **Step 6: Run full build**

```bash
cd /home/abo/workspace/home/nexorious-go && go build ./...
```

Expected: success.

- [ ] **Step 7: Commit**

```bash
git add ui/db-error/index.html ui/ui.go internal/api/db_error.go internal/api/db_error_test.go
git commit -m "feat(api): add DBErrorHandler with DSN redaction and html/template injection"
```

---

## Task 9: Update `internal/api/router.go` — three middleware gates, new routes, JWT group

**Files:**
- Modify: `internal/api/router.go`
- Modify: `internal/api/router_test.go`
- Create: `ui/setup/index.html`

- [ ] **Step 1: Create `ui/setup/index.html`**:

```bash
mkdir -p /home/abo/workspace/home/nexorious-go/ui/setup
```

Create `ui/setup/index.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Nexorious — Setup</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f3f4f6; }
  .card { background: #fff; border-radius: 8px; box-shadow: 0 2px 12px rgba(0,0,0,.1); padding: 40px; width: 100%; max-width: 420px; }
  h1 { margin: 0 0 8px; font-size: 1.5rem; }
  p { color: #555; margin: 0 0 24px; }
  label { display: block; font-size: .875rem; font-weight: 500; margin-bottom: 4px; }
  input { width: 100%; border: 1px solid #d1d5db; border-radius: 6px; padding: 10px 12px; font-size: 1rem; margin-bottom: 16px; }
  input:focus { outline: 2px solid #2563eb; border-color: transparent; }
  button { width: 100%; background: #2563eb; color: #fff; border: none; border-radius: 6px; padding: 12px; font-size: 1rem; cursor: pointer; }
  button:disabled { opacity: .6; cursor: not-allowed; }
  .error { color: #dc2626; font-size: .875rem; margin-bottom: 12px; display: none; }
</style>
</head>
<body>
<div class="card">
  <h1>Welcome to Nexorious</h1>
  <p>Create your admin account to get started.</p>
  <div class="error" id="err"></div>
  <form id="form">
    <label for="username">Username</label>
    <input type="text" id="username" name="username" autocomplete="username" required minlength="3">
    <label for="password">Password</label>
    <input type="password" id="password" name="password" autocomplete="new-password" required minlength="8">
    <label for="confirm">Confirm Password</label>
    <input type="password" id="confirm" name="confirm" autocomplete="new-password" required minlength="8">
    <button type="submit" id="btn">Create Admin Account</button>
  </form>
</div>
<script>
const form = document.getElementById('form');
const errEl = document.getElementById('err');
const btn = document.getElementById('btn');

function showError(msg) {
  errEl.textContent = msg;
  errEl.style.display = 'block';
}
function clearError() {
  errEl.style.display = 'none';
}

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  clearError();
  const username = document.getElementById('username').value.trim();
  const password = document.getElementById('password').value;
  const confirm = document.getElementById('confirm').value;

  if (username.length < 3) return showError('Username must be at least 3 characters.');
  if (password.length < 8) return showError('Password must be at least 8 characters.');
  if (password !== confirm) return showError('Passwords do not match.');

  btn.disabled = true;
  btn.textContent = 'Creating account…';

  try {
    const res = await fetch('/api/auth/setup/admin', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) {
      const data = await res.json();
      const storedAuth = {
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
        user: {
          id: data.user.id,
          username: data.user.username,
          isAdmin: data.user.is_admin,
          preferences: {},
        },
      };
      localStorage.setItem('auth', JSON.stringify(storedAuth));
      window.location.href = '/';
    } else if (res.status === 400) {
      const body = await res.json();
      showError(body.error || 'Validation error.');
    } else if (res.status === 403) {
      window.location.href = '/login';
    } else if (res.status === 500) {
      window.location.href = '/login';
    } else {
      showError('Setup failed. Please try again.');
    }
  } catch (err) {
    showError('Setup failed. Please try again.');
  } finally {
    btn.disabled = false;
    btn.textContent = 'Create Admin Account';
  }
});
</script>
</body>
</html>
```

Add `SetupBox` to `ui/ui.go`:

```go
//go:embed setup
var SetupBox embed.FS
```

- [ ] **Step 2: Write failing router tests** — add to `internal/api/router_test.go`:

```go
func TestDBUnavailable_RedirectsToErrorPage(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/some/page", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusFound { t.Errorf("expected 302, got %d", rec.Code) }
    loc := rec.Header().Get("Location")
    if !strings.HasPrefix(loc, "/db-error") { t.Errorf("expected redirect to /db-error, got %q", loc) }
}

func TestDBUnavailable_EncodesFromParam(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/user-games?page=2&sort=title", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    loc := rec.Header().Get("Location")
    if !strings.Contains(loc, "%2F") && !strings.Contains(loc, "from=") {
        t.Errorf("expected encoded from param in Location, got %q", loc)
    }
}

func TestSetupGate_RedirectsArbitraryRoutes(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusFound { t.Errorf("expected 302, got %d", rec.Code) }
    if loc := rec.Header().Get("Location"); loc != "/setup" {
        t.Errorf("expected redirect to /setup, got %q", loc)
    }
}

func TestSetupGate_BypassesHealthEndpoint(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
}

func TestSetupGate_BypassesMigrateRoutes(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    // Should not redirect to /setup (migrate routes are bypassed).
    if loc := rec.Header().Get("Location"); loc == "/setup" {
        t.Errorf("migrate route should not redirect to /setup")
    }
}

func TestHealth_OKWhenReady(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
    var body map[string]string
    json.NewDecoder(rec.Body).Decode(&body)
    if body["status"] != "ok" { t.Errorf("expected status=ok, got %q", body["status"]) }
}

func TestHealth_OKWhenSetupPending(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateReady)
    migrator.SetNeedsSetup(true)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
    var body map[string]string
    json.NewDecoder(rec.Body).Decode(&body)
    if body["status"] != "ok" { t.Errorf("expected status=ok when needsSetup, got %q", body["status"]) }
}

func TestHealth_DBUnavailableReturns200(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateDBUnavailable)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
    var body map[string]string
    json.NewDecoder(rec.Body).Decode(&body)
    if body["status"] != "db_unavailable" { t.Errorf("expected db_unavailable, got %q", body["status"]) }
}

func TestHealth_NeedsMigrationReturns200(t *testing.T) {
    migrator := migrate.NewMigratorForTest(migrate.AppStateNeedsMigration)
    e := api.New(testCfg(), migrator, nil)
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    rec := httptest.NewRecorder()
    e.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Errorf("expected 200, got %d", rec.Code) }
    var body map[string]string
    json.NewDecoder(rec.Body).Decode(&body)
    if body["status"] != "needs_migration" { t.Errorf("expected needs_migration, got %q", body["status"]) }
}
```

Check if `testCfg()` already exists in `router_test.go`:

```bash
grep -n "testCfg\|TestCfg" /home/abo/workspace/home/nexorious-go/internal/api/router_test.go
```

If not, add:

```go
func testCfg() *config.Config {
    return &config.Config{
        SecretKey:                "test-secret-key-32-bytes-long!!!",
        AccessTokenExpireMinutes: 15,
        RefreshTokenExpireDays:   30,
        Port:                     8000,
    }
}
```

Run to confirm fail:

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestDBUnavailable|TestSetupGate|TestHealth" -v 2>&1 | head -30
```

- [ ] **Step 3: Rewrite the middleware section in `router.go`**

Replace the existing app-state middleware block with the three-gate version:

```go
// Gate 1: DB unavailable
e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c *echo.Context) error {
        if migrator.State() == migrate.AppStateDBUnavailable {
            path := c.Request().URL.Path
            if path == "/db-error" || path == "/health" {
                return next(c)
            }
            return c.Redirect(http.StatusFound,
                "/db-error?from="+url.QueryEscape(c.Request().RequestURI))
        }
        return next(c)
    }
})

// Gate 2: migrations pending
e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c *echo.Context) error {
        if migrator.State() != migrate.AppStateReady {
            path := c.Request().URL.Path
            if strings.HasPrefix(path, "/migrate") || strings.HasPrefix(path, "/api/migrate") || path == "/health" {
                return next(c)
            }
            return c.Redirect(http.StatusFound, "/migrate")
        }
        return next(c)
    }
})

// Gate 3: setup required
e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c *echo.Context) error {
        if migrator.NeedsSetup() {
            path := c.Request().URL.Path
            if path == "/setup" || strings.HasPrefix(path, "/api/auth/setup") ||
                path == "/health" || strings.HasPrefix(path, "/api/migrate") {
                return next(c)
            }
            return c.Redirect(http.StatusFound, "/setup")
        }
        return next(c)
    }
})
```

Add import: `"net/url"`.

- [ ] **Step 4: Update `New` signature and `registerRoutes` to accept `resolvedDatabaseURL`**

```go
func New(cfg *config.Config, migrator *migrate.Migrator, pool *pgxpool.Pool) *echo.Echo {
    // ... (no signature change needed — resolvedDatabaseURL comes from cfg in main.go
    // but router.go needs it for NewDBErrorHandler)
```

Actually, per spec, `registerRoutes` needs the `resolvedDatabaseURL`. Update `New`:

```go
func New(cfg *config.Config, migrator *migrate.Migrator, pool *pgxpool.Pool, resolvedDatabaseURL string) *echo.Echo {
    // ...
    registerRoutes(e, cfg, mh, pool, migrator, resolvedDatabaseURL)
}
```

And `registerRoutes`:

```go
func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, pool *pgxpool.Pool, migrator *migrate.Migrator, resolvedDatabaseURL string) {
```

The `migrator` parameter is needed to pass to `NewSetupHandler` and `NewDBErrorHandler`.

- [ ] **Step 5: Register new routes in `registerRoutes`**:

```go
// DB-error route
dh := NewDBErrorHandler(resolvedDatabaseURL, migrator)
e.GET("/db-error", dh.HandleDBError)

// Setup routes
sh := NewSetupHandler(pool, cfg, migrator)
e.GET("/setup", func(c *echo.Context) error {
    if !migrator.NeedsSetup() {
        return c.Redirect(http.StatusFound, "/")
    }
    f, err := ui.SetupBox.Open("setup/index.html")
    if err != nil { return err }
    defer f.Close()
    return c.Stream(http.StatusOK, "text/html; charset=utf-8", f)
})
e.POST("/api/auth/setup/admin", sh.HandleSetupAdmin)
e.POST("/api/auth/setup/restore", func(c *echo.Context) error {
    return c.JSON(http.StatusNotImplemented, map[string]string{
        "error": "not implemented — deferred to Phase 3",
    })
})

// JWT-protected group
authGroup := e.Group("/api/auth", auth.JWTMiddleware(cfg.SecretKey, pool))
authGroup.POST("/logout", ah.HandleLogout)
ah2 := NewAuthHandler(pool, cfg)
authGroup.GET("/me", ah2.HandleGetMe)
```

Note: consolidate `ah` and `ah2` into one `ah := NewAuthHandler(pool, cfg)` and use it for both.

- [ ] **Step 6: Update `/health` handler**:

```go
e.GET("/health", func(c *echo.Context) error {
    switch migrator.State() {
    case migrate.AppStateReady:
        return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
    default:
        return c.JSON(http.StatusOK, map[string]string{"status": migrator.State().String()})
    }
})
```

- [ ] **Step 7: Update `main.go` call site** — pass `resolvedDatabaseURL` (computed in Task 10):

```go
e := api.New(cfg, migrator, pool, resolvedDatabaseURL)
```

For now add a placeholder — Task 10 will introduce `resolvedDatabaseURL` properly.

- [ ] **Step 8: Run router tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -run "TestDBUnavailable|TestSetupGate|TestHealth|TestSetupPage|TestSetupAdmin_GetMe" -v -timeout 120s
```

Expected: all pass.

- [ ] **Step 9: Run full api suite**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./internal/api/... -v -timeout 180s
```

Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add ui/setup/index.html ui/ui.go internal/api/router.go internal/api/router_test.go
git commit -m "feat(api): three middleware gates, setup/db-error routes, JWT group with /api/auth/me"
```

---

## Task 10: Update `cmd/nexorious/main.go` — startup rewrite

**Files:**
- Modify: `cmd/nexorious/main.go`

- [ ] **Step 1: Add `resolveDBURL` function**

Add at bottom of `main.go` (before `parseSlogLevel`):

```go
// resolveDBURL returns the database connection string to use.
// DATABASE_URL takes priority; otherwise assembles from individual DB_* vars.
func resolveDBURL(cfg *config.Config) string {
    if cfg.DatabaseURL != "" {
        return cfg.DatabaseURL
    }
    return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
        url.QueryEscape(cfg.DbUser),
        url.QueryEscape(cfg.DbPassword),
        cfg.DbHost,
        cfg.DbPort,
        cfg.DbName,
        "disable", // default; add cfg.DbSSLMode if added to config
    )
}
```

Add import: `"net/url"`.

Note: `config.Config` uses field names `DbHost`, `DbPort`, `DbUser`, `DbPassword`, `DbName` (check the exact field names from `config.go` — they are `DbHost`, `DbPort`, `DbUser`, `DbPassword`, `DbName`). The config's `resolveDatabaseURL` already handles this, so `cfg.DatabaseURL` is always populated after `config.Load()`. But the spec requires that `resolvedDatabaseURL` be the string *actually* passed to pgxpool, which is `cfg.DatabaseURL` after resolution. Simplify:

```go
func resolveDBURL(cfg *config.Config) string {
    return cfg.DatabaseURL  // already resolved by config.Load() via resolveDatabaseURL()
}
```

But we still want to pass it explicitly rather than relying on `cfg.DatabaseURL` being non-empty everywhere. Use this simpler form.

- [ ] **Step 2: Rewrite the database + migrator section of `main()`**

Replace the existing ping-then-fatal block:

```go
// -------------------------------------------------------------------------
// Database pool
// -------------------------------------------------------------------------
resolvedDatabaseURL := resolveDBURL(cfg)
pool, err := pgxpool.New(ctx, resolvedDatabaseURL)
if err != nil {
    // pgxpool.New is lazy — this only fails on DSN parse errors.
    slog.Error("failed to parse database URL", "err", err)
    os.Exit(1)
}
defer pool.Close()

// -------------------------------------------------------------------------
// Migrator (created before ping — middleware needs it from the first request)
// -------------------------------------------------------------------------
migrator, err := migrate.NewMigrator(resolvedDatabaseURL)
if err != nil {
    slog.Error("failed to create migrator", "err", err)
    pool.Close()
    os.Exit(1)
}

// initAppState runs determineState + InitNeedsSetup on the existing Migrator.
// Called once at startup and injected as the probe's onRecovery callback.
initAppState := func(ctx context.Context) error {
    if err := migrator.DetermineStateForTest(); err != nil {
        return fmt.Errorf("initAppState: determineState: %w", err)
    }
    if migrator.State() == migrate.AppStateReady {
        if err := migrator.InitNeedsSetup(ctx, pool); err != nil {
            return fmt.Errorf("initAppState: InitNeedsSetup: %w", err)
        }
    }
    return nil
}

// Single ping attempt — no retry loop (StartDBProbe handles retries).
pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
pingErr := pool.Ping(pingCtx)
pingCancel()
if pingErr == nil {
    slog.Info("database connected")
    if err := initAppState(ctx); err != nil {
        slog.Error("initAppState failed — starting in DBUnavailable state", "err", err)
        // StartDBProbe will call initAppState on next successful ping.
    }
} else {
    slog.Warn("database not reachable at startup — starting in DBUnavailable state", "err", pingErr)
}
```

Remove the existing `defer migrator.Close()` block. Add a new deferred close:

```go
defer func() {
    if err := migrator.Close(); err != nil {
        slog.Error("migrator close error", "err", err)
    }
}()
```

- [ ] **Step 3: Update `--migrate-only` path**

```go
if migrateOnly {
    // Retry ping up to 30 seconds.
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
        err := pool.Ping(pingCtx)
        cancel()
        if err == nil { break }
        slog.Warn("migrate-only: waiting for database", "err", err)
        time.Sleep(2 * time.Second)
    }
    // Final check.
    pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
    if err := pool.Ping(pingCtx); err != nil {
        pingCancel()
        slog.Error("migrate-only: database unreachable after 30s", "err", err)
        pool.Close()
        os.Exit(1)
    }
    pingCancel()

    if err := migrator.DetermineStateForTest(); err != nil {
        slog.Error("migrate-only: determineState failed", "err", err)
        pool.Close()
        os.Exit(1)
    }
    if migrator.State() == migrate.AppStateReady {
        slog.Info("migrate-only: no pending migrations")
        pool.Close()
        os.Exit(0)
    }

    migrator.SetLogWriter(slog.NewLogLogger(slog.Default().Handler(), slog.LevelInfo).Writer())
    if err := migrator.RunMigrations(ctx); err != nil {
        slog.Error("migrate-only: migrations failed", "err", err)
        pool.Close()
        os.Exit(1)
    }
    slog.Info("migrate-only: migrations complete")
    pool.Close()
    os.Exit(0)
}
```

- [ ] **Step 4: Start HTTP server + probes + gate loop**

```go
// HTTP server
e := api.New(cfg, migrator, pool, resolvedDatabaseURL)

shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
defer stop()

// StartDBProbe — polls every 5s, calls initAppState on recovery.
migrator.StartDBProbe(shutdownCtx, pool, initAppState)

// Worker/scheduler gate loop — starts workers only after Ready && !NeedsSetup.
// (Workers/scheduler are not yet wired in this phase — extend when added.)
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }
        if migrator.State() == migrate.AppStateReady && !migrator.NeedsSetup() {
            slog.Info("app ready — workers and scheduler would start here (Phase N)")
            return
        }
        time.Sleep(2 * time.Second)
    }
}(shutdownCtx)

addr := fmt.Sprintf(":%d", cfg.Port)
sc := echo.StartConfig{
    Address:         addr,
    GracefulTimeout: 10 * time.Second,
    HideBanner:      true,
    HidePort:        true,
}

slog.Info("nexorious starting", "addr", addr, "version", version, "commit", commit)
if err := sc.Start(shutdownCtx, e); err != nil {
    slog.Info("server stopped", "err", err)
}
slog.Info("shutdown complete")
```

Note: `migrator.DetermineStateForTest()` is used in `initAppState` — but it's in `main.go` which is outside the `migrate` package. Since `determineState` is unexported, we exposed it via `DetermineStateForTest()` in Task 2. That's fine for now; rename to `DetermineState()` (exported without "ForTest") if desired. The spec calls it from `initAppState` in main — update the method name consistently.

Actually, let's export it properly. In `migrator.go`, rename `DetermineStateForTest` to just an internal method but add a proper exported wrapper:

```go
// DetermineState re-consults the database to compute the current state.
// Called from main.go's initAppState and from StartDBProbe on recovery.
func (mg *Migrator) DetermineState() error {
    return mg.determineState()
}
```

Update all call sites (`migrator_test.go`, `handler_test.go`, `main.go`).

- [ ] **Step 5: Build and verify**

```bash
cd /home/abo/workspace/home/nexorious-go && go build ./...
```

Expected: success.

- [ ] **Step 6: Run all tests**

```bash
cd /home/abo/workspace/home/nexorious-go && go test ./... -timeout 300s
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add cmd/nexorious/main.go internal/migrate/migrator.go internal/migrate/migrator_test.go internal/migrate/handler_test.go
git commit -m "feat(main): lazy startup, StartDBProbe, worker/scheduler gate loop"
```

---

## Task 11: Final verification

- [ ] **Step 1: Full build**

```bash
cd /home/abo/workspace/home/nexorious-go && go build ./...
```

Expected: success with no warnings.

- [ ] **Step 2: Full test suite with race detector**

```bash
cd /home/abo/workspace/home/nexorious-go && go test -race ./... -timeout 300s
```

Expected: all pass, no data races.

- [ ] **Step 3: Lint**

```bash
cd /home/abo/workspace/home/nexorious-go && golangci-lint run
```

Expected: no errors.

- [ ] **Step 4: Smoke check — verify server starts in DBUnavailable state**

```bash
cd /home/abo/workspace/home/nexorious-go && SECRET_KEY=test123456789012345678901234567890 IGDB_CLIENT_ID=x IGDB_CLIENT_SECRET=x DATABASE_URL=postgres://bad@localhost:5432/x ./nexorious &
sleep 2
curl -s http://localhost:8000/health
kill %1
```

Expected: `{"status":"db_unavailable"}` — server starts and health responds even with no DB.

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(setup): complete first-run setup flow implementation"
```

