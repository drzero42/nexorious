# Testing Policy and Performance Design

**Date:** 2026-05-14  
**Status:** Approved

## Problem

An 80% coverage target caused a large number of low-value tests to be written — tests that only verify the code does what it does, with no ability to catch real bugs. Additionally, each test that requires a database spins up a fresh testcontainer and runs all migrations from scratch, making the test suite very slow.

## Goals

1. Replace the coverage target with a quality-based testing policy
2. Audit all existing tests and remove or rewrite those that don't follow the policy
3. Dramatically reduce test suite runtime by sharing containers within packages

---

## Design

### 1. Testing Policy

**Write a test when:**
- The behavior is security-sensitive (auth, token validation, permission checks)
- There are multiple meaningful edge cases (missing fields, wrong types, not found, conflict)
- The logic is non-obvious or involves a subtle invariant
- A real bug was found — the test documents that it cannot regress

**Do NOT write a test when:**
- The function is a thin wrapper or a struct field accessor
- The test only verifies that calling the function returns what the function computes (tautology)
- The only assertion is "no panic on happy path"
- Coverage percentage is the motivation

**No coverage gate in CI.** The quality gate is: does the PR touching non-trivial logic include a test that would have caught a plausible bug in that logic? `golangci-lint` remains the code correctness gate.

This policy goes into CLAUDE.md under the Testing section, replacing any reference to the 80% target.

---

### 2. Container Architecture: TestMain + Shared Container

**Current problem:** Every individual test function calls `setupAuthTestDB(t)` (or equivalent), which starts a fresh PostgreSQL container and runs all migrations. With ~44 tests in `auth_test.go` alone, that is 44 container startups and 44 full migration runs.

**Solution:** One container per package, owned by `TestMain`.

```go
// testmain_test.go (one per package that needs a real DB)
var testDB *bun.DB

func TestMain(m *testing.M) {
    ctx := context.Background()
    ctr, db := startSharedPostgres(ctx)
    testDB = db
    code := m.Run()
    _ = ctr.Terminate(ctx)
    os.Exit(code)
}
```

`startSharedPostgres` starts the container once, runs all migrations, and returns. Migrations run exactly once per `go test` invocation per package.

**Test isolation:** Each test truncates all tables at the start via a shared helper:

```go
func truncateAllTables(t *testing.T) {
    t.Helper()
    _, err := testDB.ExecContext(context.Background(),
        `TRUNCATE users, user_sessions, games, platforms, tags,
                 user_games, jobs, job_items, user_game_tags,
                 backup_configs, sync_configs
         RESTART IDENTITY CASCADE`)
    if err != nil {
        t.Fatalf("truncate: %v", err)
    }
}
```

Individual insert helpers (`insertAuthTestUser`, `insertTestGame`, etc.) use `testDB` directly instead of receiving a `*bun.DB` parameter. The per-test `setupXxxDB(t)` calls disappear.

**Affected packages:**
- `internal/api` — largest, most tests
- `internal/auth` — jwt_test.go
- `internal/backup` — service_test.go
- `internal/scheduler` — cleanup_test.go, lifecycle_test.go, stale_jobs_test.go
- `internal/worker/tasks` — export_test.go, import_item_test.go, etc.
- `cmd/nexorious` — main_test.go

**Unaffected packages** (no containers today):
- `internal/migrate` — uses `NewMigratorForTest`, no real DB
- `internal/ratelimit/local` — pure in-memory
- `internal/config` — struct tests
- `internal/middleware` — httptest only
- `internal/services/matching`, `platformresolution` — pure logic

---

### 3. Test Audit

Before the container refactor, every existing test is evaluated against the policy:

| Verdict | Criteria |
|---------|----------|
| **Keep** | Tests real observable behavior: status codes, DB state, error messages, token validity |
| **Rewrite** | Exercises a real path but makes weak or missing assertions |
| **Delete** | Tautological: only verifies the code does what it does; no plausible bug could be caught |

**Expected outcomes by package:**
- `internal/api/auth_test.go` — mostly Keep (strong behavioral coverage)
- `internal/api/router_test.go` — mostly Keep (real middleware state machine)
- `internal/api/games_test.go` — likely Keep (real query/filter behavior)
- `internal/config/config_test.go` — likely Delete (config struct accessors)
- `internal/backup/tools_test.go` — needs review
- `internal/ratelimit/local_test.go` — needs review
- `internal/scheduler/*_test.go` — flagged for close review; scheduler tests added to hit coverage are often tautological

**Ordering:** Audit and cleanup runs first, then the container refactor. No point optimizing tests we are about to delete.

---

## Implementation Order

1. Audit all test files; delete/rewrite low-value tests
2. Add `TestMain` + shared container to each affected package
3. Convert per-test `setupXxxDB(t)` calls to use `testDB` + `truncateAllTables(t)`
4. Update CLAUDE.md: replace coverage target with quality-based testing policy
5. Verify full test suite passes and runtime is significantly reduced
