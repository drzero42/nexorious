# Testing Policy and Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 80% coverage target with a quality-based testing policy, delete/rewrite low-value tests, and reduce suite runtime by sharing one PostgreSQL container per test package.

**Architecture:** Phase 1 updates CLAUDE.md. Phase 2 audits and removes tautological tests. Phase 3 introduces package-level `TestMain` that starts one container per package and runs migrations once — each test then calls `truncateAllTables(t)` for isolation. Audit runs first so we do not optimise tests we intend to delete.

**Tech Stack:** `testcontainers-go`, `bun/migrate`, `migrations.Migrations`, `postgres:18-alpine`, stdlib `testing`

---

## Shared code reference

Every `TestMain` in this plan follows this identical structure — only the package name changes:

```go
package <pkg>_test   // e.g. api_test, auth_test, scheduler_test, …

import (
    "context"
    "database/sql"
    "os"
    "testing"

    "github.com/testcontainers/testcontainers-go"
    tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
    bunmigrate "github.com/uptrace/bun/migrate"

    "github.com/drzero42/nexorious-go/internal/db/migrations"
)

var testDB *bun.DB

func TestMain(m *testing.M) {
    ctx := context.Background()
    ctr, err := tcpostgres.Run(ctx,
        "postgres:18-alpine",
        tcpostgres.WithDatabase("nexorious_test"),
        tcpostgres.WithUsername("test"),
        tcpostgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2),
        ),
    )
    if err != nil {
        panic("start postgres container: " + err.Error())
    }

    connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        panic("connection string: " + err.Error())
    }

    sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
    testDB = bun.NewDB(sqldb, pgdialect.New())

    migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
    if err := migrator.Init(ctx); err != nil {
        panic("migrator init: " + err.Error())
    }
    if _, err := migrator.Migrate(ctx); err != nil {
        panic("migrate: " + err.Error())
    }

    code := m.Run()
    _ = testDB.Close()
    _ = ctr.Terminate(ctx)
    os.Exit(code)
}

func truncateAllTables(t *testing.T) {
    t.Helper()
    _, err := testDB.ExecContext(context.Background(), `
        TRUNCATE
            users, user_sessions, games, external_games,
            platforms, storefronts, platform_storefronts,
            tags, user_games, user_game_tags, user_game_platforms,
            jobs, job_items, pending_tasks,
            backup_config, user_sync_configs, rate_limiter_tokens
        RESTART IDENTITY CASCADE
    `)
    if err != nil {
        t.Fatalf("truncate: %v", err)
    }
}
```

---

## Task 1: Update CLAUDE.md testing policy

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Locate the Testing section in CLAUDE.md**

```bash
grep -n "## Testing\|coverage\|80%" /home/abo/workspace/home/nexorious-go/CLAUDE.md
```

- [ ] **Step 2: Replace or augment the Testing section**

Find the `## Testing` section. Replace its content with:

```markdown
## Testing

### Policy

Write a test when:
- The behaviour is security-sensitive (auth, token validation, permission checks)
- There are multiple meaningful edge cases (missing fields, wrong types, not found, conflict)
- The logic is non-obvious or involves a subtle invariant
- A real bug was found — the test documents that it cannot regress

Do NOT write a test when:
- The function is a thin wrapper or a struct field accessor
- The test only verifies that calling the function returns what it computes (tautology)
- The only assertion is "no panic on happy path" with no behavioural verification
- Coverage percentage is the motivation

There is no coverage gate in CI. The quality gate is: does the PR touching non-trivial logic include a test that would have caught a plausible bug in that logic?

### Performance

Each package that needs a real database uses a shared PostgreSQL container via `TestMain`. The container starts once per `go test` invocation per package; migrations run once at startup. Each test calls `truncateAllTables(t)` at the top for isolation. Do NOT call a per-test `setupXxxDB(t)` helper that starts a new container — use the shared `testDB` package variable instead.
```

Remove any sentence that mentions an 80% target.

- [ ] **Step 3: Verify no linter errors**

```bash
golangci-lint run
```
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): replace coverage target with quality-based testing policy"
```

---

## Task 2: Audit pure-logic test packages (no containers)

These packages contain no testcontainer usage. Read each file, apply the policy, delete tests that fail it.

**Files to examine:**
- `internal/config/config_test.go`
- `internal/ratelimit/local_test.go`
- `internal/ratelimit/postgres_test.go`
- `internal/worker/tasks/helpers_test.go`
- `internal/worker/tasks/export_helpers_test.go`
- `internal/services/matching/matching_test.go`
- `internal/services/platformresolution/resolution_test.go`
- `internal/services/igdb/credentials_test.go`
- `internal/services/igdb/igdb_test.go`
- `internal/services/igdb/igdb_extra_test.go`
- `internal/middleware/maintenance_test.go`
- `internal/backup/tools_test.go`
- `internal/worker/pool_test.go`

**Policy checklist per test (apply to every function in every file):**
1. Would this test catch a plausible bug that a type checker would not catch?
2. Does it make assertions about observable behaviour (output values, error conditions, DB state, status codes)?
3. If someone introduced a subtle bug in the implementation, would this test fail?

If the answer to all three is "maybe not," delete the test.

**Known deletion:**
- `TestLocal_WaitSucceeds` in `internal/ratelimit/local_test.go`: calls `Wait` with no precondition and only checks there is no error. No real behaviour is verified beyond "it does not crash." Delete it.

- [ ] **Step 1: Read each file using get_file_content**

Read every file in the list above in full.

- [ ] **Step 2: Apply the policy and delete failing tests**

For `internal/ratelimit/local_test.go`, delete `TestLocal_WaitSucceeds` (the entire function). For all other identified low-value tests, delete them too.

- [ ] **Step 3: Run tests for each modified package**

```bash
go test ./internal/config/... -v
go test ./internal/ratelimit/... -v
go test ./internal/worker/tasks/... -run "TestParse|TestOwnership|TestIgdb|TestBuildCSV|TestBuildJSON" -v
go test ./internal/services/... -v
go test ./internal/middleware/... -v
go test ./internal/backup/... -run "TestTools\|TestCheck" -v
go test ./internal/worker/pool_test.go -v ./internal/worker/
```
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/
git commit -m "test: remove low-value tests in pure-logic packages"
```

---

## Task 3: Audit internal/api tests

**Files to examine (read each with get_file_content):**
- `internal/api/admin_users_test.go`
- `internal/api/backup_test.go`
- `internal/api/backup_helpers_test.go`
- `internal/api/db_error_test.go`
- `internal/api/export_test.go`
- `internal/api/import_test.go`
- `internal/api/job_items_test.go`
- `internal/api/jobs_test.go`
- `internal/api/platforms_test.go`
- `internal/api/setup_test.go`
- `internal/api/sync_test.go`
- `internal/api/tags_test.go`
- `internal/api/user_games_test.go`

(Files already reviewed in brainstorming: `auth_test.go`, `router_test.go`, `games_test.go` — all keep.)

**Additional rule for api tests:** If a test only checks a status code on a path that does not touch the database, it does not need a container. It can use `migrate.NewMigratorForTest(migrate.AppStateReady)` with `api.New(...)` directly (exactly as `router_test.go` does). Refactor any such tests to remove the DB dependency.

- [ ] **Step 1: Read each file using get_file_content**

- [ ] **Step 2: Apply deletions, rewrites, and no-DB conversions**

For each low-value test: delete. For each test that uses a container but only checks status codes on DB-independent paths, convert to the no-DB pattern:

```go
// No-DB pattern (follow router_test.go)
m := migrate.NewMigratorForTest(migrate.AppStateReady)
e := api.New(testCfg(), m, nil, "", nil, nil, nil)
req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
rec := httptest.NewRecorder()
e.ServeHTTP(rec, req)
if rec.Code != http.StatusOK { ... }
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/api/...
```
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/api/
git commit -m "test(api): remove low-value tests, convert pure-path tests to no-db pattern"
```

---

## Task 4: Audit remaining container packages

**Files to examine (read each with get_file_content):**
- `internal/auth/jwt_test.go`
- `internal/backup/service_test.go`
- `internal/scheduler/cleanup_test.go`
- `internal/scheduler/lifecycle_test.go`
- `internal/scheduler/stale_jobs_test.go`
- `internal/worker/tasks/export_test.go`
- `internal/worker/tasks/import_item_test.go`
- `internal/worker/tasks/metadata_refresh_test.go`
- `internal/worker/tasks/sync_test.go`
- `cmd/nexorious/main_test.go`

Apply the same policy checklist as Task 2.

- [ ] **Step 1: Read each file using get_file_content**

- [ ] **Step 2: Apply deletions and rewrites**

Document and delete each failing test.

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/auth/...
go build ./internal/backup/...
go build ./internal/scheduler/...
go build ./internal/worker/tasks/...
go build ./cmd/nexorious/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/ cmd/
git commit -m "test: remove low-value tests in container-backed packages"
```

---

## Task 5: Add TestMain to internal/api

The largest refactor. Replaces ~40+ per-test container startups with one.

**Files:**
- Create: `internal/api/testmain_test.go`
- Modify: `internal/api/auth_test.go` — remove `setupAuthTestDB`, update all test bodies and usages
- Modify: any other `internal/api/*_test.go` files that call `setupAuthTestDB`

- [ ] **Step 1: Create internal/api/testmain_test.go**

Create the file using the shared `TestMain` template from the top of this plan, with `package api_test`. No additional variables needed beyond `testDB`.

- [ ] **Step 2: Remove setupAuthTestDB from auth_test.go**

In `internal/api/auth_test.go`:
1. Delete the entire `setupAuthTestDB` function.
2. Remove these imports that `setupAuthTestDB` was the only consumer of:
   - `"github.com/testcontainers/testcontainers-go"`
   - `tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"`
   - `"github.com/testcontainers/testcontainers-go/wait"`
   - `bunmigrate "github.com/uptrace/bun/migrate"`
   - `"github.com/drzero42/nexorious-go/internal/db/migrations"`

- [ ] **Step 3: Update every test body in auth_test.go**

Every test that opens with `db := setupAuthTestDB(t)` changes to `truncateAllTables(t)`. Every subsequent use of `db` becomes `testDB`. Every helper call that received `db` as a parameter now receives `testDB`.

```go
// Before:
db := setupAuthTestDB(t)
cfg := testCfg()
e := newTestEcho(t, db, cfg)
insertAuthTestUser(t, db, "user-001", "alice", "password123", true, false)
// ... later:
if err := db.QueryRowContext(context.Background(), ...).Scan(&count); err != nil { ... }

// After:
truncateAllTables(t)
cfg := testCfg()
e := newTestEcho(t, testDB, cfg)
insertAuthTestUser(t, testDB, "user-001", "alice", "password123", true, false)
// ... later:
if err := testDB.QueryRowContext(context.Background(), ...).Scan(&count); err != nil { ... }
```

Apply this transformation to every test function in the file. There are ~40 tests — apply mechanically.

- [ ] **Step 4: Update all other api test files that call setupAuthTestDB**

```bash
grep -rn "setupAuthTestDB" internal/api/
```

For every match: replace `db := setupAuthTestDB(t)` with `truncateAllTables(t)` and `db` with `testDB`.

- [ ] **Step 5: Build**

```bash
go build ./internal/api/...
```
Expected: clean.

- [ ] **Step 6: Run tests**

```bash
go test ./internal/api/... -v -timeout 300s
```
Expected: all tests pass. The container starts exactly once (one container-start log line), not ~40 times.

- [ ] **Step 7: Commit**

```bash
git add internal/api/
git commit -m "test(api): shared postgres container via TestMain, truncate per test"
```

---

## Task 6: Add TestMain to internal/auth

**Files:**
- Create: `internal/auth/testmain_test.go`
- Modify: `internal/auth/jwt_test.go`

- [ ] **Step 1: Create internal/auth/testmain_test.go**

Use the shared `TestMain` template with `package auth_test`.

- [ ] **Step 2: Find and remove the per-test container helper in jwt_test.go**

The container setup in `jwt_test.go` starts around line 431. It is likely a function such as `setupJWTTestDB(t)`. Identify it, then:
1. Delete the helper function.
2. Remove its testcontainers/bun/migrate imports.
3. Replace every `db := setupJWTTestDB(t)` call with `truncateAllTables(t)`.
4. Replace all `db` uses with `testDB`.

- [ ] **Step 3: Build and run**

```bash
go build ./internal/auth/...
go test ./internal/auth/... -v -timeout 120s
```
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/auth/
git commit -m "test(auth): shared postgres container via TestMain"
```

---

## Task 7: Add TestMain to internal/backup

The backup service test needs both `testDB` and a DSN string because `pg_dump` is invoked with a connection string.

**Files:**
- Create: `internal/backup/testmain_test.go`
- Modify: `internal/backup/service_test.go`

- [ ] **Step 1: Create internal/backup/testmain_test.go**

Use the shared `TestMain` template with `package backup_test`, but add `var testDSN string` and capture the connection string:

```go
package backup_test

import (
    "context"
    "database/sql"
    "os"
    "testing"

    "github.com/testcontainers/testcontainers-go"
    tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
    bunmigrate "github.com/uptrace/bun/migrate"

    "github.com/drzero42/nexorious-go/internal/db/migrations"
)

var testDB *bun.DB
var testDSN string

func TestMain(m *testing.M) {
    ctx := context.Background()
    ctr, err := tcpostgres.Run(ctx,
        "postgres:18-alpine",
        tcpostgres.WithDatabase("nexorious_test"),
        tcpostgres.WithUsername("test"),
        tcpostgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2),
        ),
    )
    if err != nil {
        panic("start postgres container: " + err.Error())
    }

    connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        panic("connection string: " + err.Error())
    }
    testDSN = connStr

    sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(connStr)))
    testDB = bun.NewDB(sqldb, pgdialect.New())

    migrator := bunmigrate.NewMigrator(testDB, migrations.Migrations)
    if err := migrator.Init(ctx); err != nil {
        panic("migrator init: " + err.Error())
    }
    if _, err := migrator.Migrate(ctx); err != nil {
        panic("migrate: " + err.Error())
    }

    code := m.Run()
    _ = testDB.Close()
    _ = ctr.Terminate(ctx)
    os.Exit(code)
}

func truncateAllTables(t *testing.T) {
    t.Helper()
    _, err := testDB.ExecContext(context.Background(), `
        TRUNCATE
            users, user_sessions, games, external_games,
            platforms, storefronts, platform_storefronts,
            tags, user_games, user_game_tags, user_game_platforms,
            jobs, job_items, pending_tasks,
            backup_config, user_sync_configs, rate_limiter_tokens
        RESTART IDENTITY CASCADE
    `)
    if err != nil {
        t.Fatalf("truncate: %v", err)
    }
}
```

- [ ] **Step 2: Update service_test.go**

In `internal/backup/service_test.go`:
1. Delete `setupTestDB` (the function that created a minimal schema with only 3 tables — the shared container uses the full schema, which is fine for `pg_dump`).
2. Replace `db, dsn := setupTestDB(t)` calls with:
   ```go
   truncateAllTables(t)
   db := testDB
   dsn := testDSN
   ```
3. Remove now-unused imports.

- [ ] **Step 3: Build and run**

```bash
go build ./internal/backup/...
go test ./internal/backup/... -v -timeout 120s
```
Expected: all tests pass; backup tests that require `pg_dump` self-skip if the binary is absent.

- [ ] **Step 4: Commit**

```bash
git add internal/backup/
git commit -m "test(backup): shared postgres container via TestMain"
```

---

## Task 8: Add TestMain to internal/scheduler

**Files:**
- Create: `internal/scheduler/testmain_test.go`
- Modify: `internal/scheduler/scheduler_test.go` — remove `setupTestDB`
- Modify: other `internal/scheduler/*_test.go` files that call `setupTestDB`

- [ ] **Step 1: Create internal/scheduler/testmain_test.go**

Use the shared `TestMain` template with `package scheduler_test`.

- [ ] **Step 2: Update scheduler_test.go**

In `internal/scheduler/scheduler_test.go`:
1. Delete the `setupTestDB` function.
2. The `insertUser` helper takes `db *bun.DB` — keep the signature, update call sites to pass `testDB`.
3. Replace every `db := setupTestDB(t)` with `truncateAllTables(t)`.
4. Replace every `db.` reference with `testDB.`.
5. Remove unused imports.

```go
// Before:
db := setupTestDB(t)
ctx := context.Background()
userID := insertUser(t, ctx, db)
_, err := db.NewRaw(`INSERT INTO jobs ...`, userID).Exec(ctx)
scheduler.CleanupOldJobs(ctx, db)

// After:
truncateAllTables(t)
ctx := context.Background()
userID := insertUser(t, ctx, testDB)
_, err := testDB.NewRaw(`INSERT INTO jobs ...`, userID).Exec(ctx)
scheduler.CleanupOldJobs(ctx, testDB)
```

- [ ] **Step 3: Check and update remaining scheduler test files**

```bash
grep -rn "setupTestDB\|setupDB" internal/scheduler/
```

Apply the same replacement to every match found.

- [ ] **Step 4: Build and run**

```bash
go build ./internal/scheduler/...
go test ./internal/scheduler/... -v -timeout 120s
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/
git commit -m "test(scheduler): shared postgres container via TestMain"
```

---

## Task 9: Add TestMain to internal/worker/tasks

The `helpers_test.go` and `export_helpers_test.go` files are `package tasks` (internal) and do not use containers — leave them alone. The integration test files (`export_test.go`, `import_item_test.go`, `metadata_refresh_test.go`, `sync_test.go`) are `package tasks_test` (external) and may use containers.

**Files:**
- Create: `internal/worker/tasks/testmain_test.go` (if container tests exist)
- Modify: container-using test files

- [ ] **Step 1: Check which files use containers**

```bash
grep -l "testcontainers\|setupTestDB\|setupDB\|postgres" internal/worker/tasks/*_test.go
```

If none use containers, skip this task and note it in the commit.

- [ ] **Step 2: If container tests exist, create testmain_test.go**

`TestMain` must be in `package tasks_test` (the external package). The internal tests (`package tasks`) have their own binary and will not conflict. Create `internal/worker/tasks/testmain_test.go` using the shared `TestMain` template with `package tasks_test`.

- [ ] **Step 3: Update each container-using test file**

For each file found in Step 1: remove the per-test container helper, replace calls with `truncateAllTables(t)` + `testDB`, remove unused imports.

- [ ] **Step 4: Build and run**

```bash
go build ./internal/worker/...
go test ./internal/worker/... -v -timeout 180s
```
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/worker/
git commit -m "test(worker/tasks): shared postgres container via TestMain"
```

---

## Task 10: Add TestMain to cmd/nexorious

**Files:**
- Create: `cmd/nexorious/testmain_test.go`
- Modify: `cmd/nexorious/main_test.go`

- [ ] **Step 1: Determine the package name of main_test.go**

```bash
head -3 cmd/nexorious/main_test.go
```

The package will be `main` or `main_test`. Use the same package name in `testmain_test.go`.

- [ ] **Step 2: Create cmd/nexorious/testmain_test.go**

Use the shared `TestMain` template with the package name found in Step 1.

- [ ] **Step 3: Read main_test.go in full**

Use `get_file_content` on `cmd/nexorious/main_test.go`. Note:
- `startPostgresContainer` is the per-test helper to remove.
- `TestMigrator_Status_ReadOnly` is the test that calls it. Since `TestMain` already runs migrations, the status will show "up to date" when this test runs. Verify the test assertion is still valid; adjust if it was asserting a specific migration count or "needs migration" state.

- [ ] **Step 4: Remove startPostgresContainer and update test bodies**

1. Delete `startPostgresContainer` function.
2. Replace `db := startPostgresContainer(t)` with `truncateAllTables(t)` and `testDB`.
3. Remove now-unused imports.

- [ ] **Step 5: Build and run**

```bash
go build ./cmd/nexorious/...
go test ./cmd/nexorious/... -v -timeout 120s
```
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/nexorious/
git commit -m "test(cmd): shared postgres container via TestMain"
```

---

## Task 11: Final verification

- [ ] **Step 1: Run the full test suite**

```bash
go test -timeout 600s ./...
```
Expected: all tests pass, no panics.

- [ ] **Step 2: Time the run**

```bash
time go test -timeout 600s ./...
```

Note the wall-clock time. The reduction comes from eliminating N container startups per package (one startup log per package instead of one per test).

- [ ] **Step 3: Run linter**

```bash
golangci-lint run
```
Expected: clean.

- [ ] **Step 4: Push**

```bash
bd dolt push
git pull --rebase
git push
git status
```
Expected: "Your branch is up to date with 'origin/main'."
