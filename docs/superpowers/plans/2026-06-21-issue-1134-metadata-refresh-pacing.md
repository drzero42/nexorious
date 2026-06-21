# Pace Metadata-Refresh Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the nightly metadata-refresh batch from producing a transaction-abort burst by giving it a dedicated single-worker River queue and a staleness guard, while still refreshing the whole library daily.

**Architecture:** Two independent levers. (1) Route `metadata_refresh_item` jobs onto a new `metadata_refresh` River queue with `MaxWorkers` default 1 — refresh can no longer starve the shared worker pool, and a single serial worker sips only ~half the IGDB rate budget so user IGDB traffic stays responsive. (2) The dispatch query gains a staleness filter (`last_updated IS NULL OR last_updated < now() - MinAge`) so a re-dispatch/overlap can't re-pull the whole library. No count cap, no schema change, no migration.

**Tech Stack:** Go, River (`riverqueue/river`), Bun, `caarlos0/env` config, Postgres, testcontainers-based package tests.

## Global Constraints

- Issue: #1134. Spec: `docs/superpowers/specs/2026-06-21-issue-1134-metadata-refresh-pacing-design.md`.
- No new migration (no schema change).
- New config knobs: `METADATA_REFRESH_WORKERS` (int, default `1`), `METADATA_REFRESH_MIN_AGE` (Go duration string, default `"23h"`).
- Queue name has ONE source of truth: `tasks.QueueMetadataRefresh = "metadata_refresh"`.
- A River job inserted to a queue NOT registered in the client's `Queues` map sits unworked — the queue MUST be registered in **both** River-client construction sites in `cmd/nexorious/serve.go` (~line 300 and ~line 414).
- The dispatch worker is constructed in **two** places in `serve.go` (~line 242 and ~line 366); `MinAge` must be set in both.
- Conventional Commits; this is bug-driven → `fix:`.
- Tests: package `internal/worker/tasks` uses a shared `testDB` + `truncateAllTables(t)`; do NOT spin up a new container. `internal/config` tests use `config.Load()` with `t.Setenv`.

---

### Task 1: Config knobs + min-age parse helper

**Files:**
- Modify: `internal/config/config.go` (add two fields near the Scheduler block ~line 100; add a parse helper)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.MetadataRefreshWorkers int`, `Config.MetadataRefreshMinAge string`, and `func (c *Config) MetadataRefreshMinAgeDuration() time.Duration` (parses the string; on error or non-positive, returns `23 * time.Hour`).

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoad_MetadataRefreshDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@h/db")
	t.Setenv("DB_ENCRYPTION_KEY", "test-db-encryption-key-32-bytes!!")
	t.Setenv("IGDB_CLIENT_ID", "testclientid")
	t.Setenv("IGDB_CLIENT_SECRET", "testclientsecret")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.MetadataRefreshWorkers != 1 {
		t.Errorf("MetadataRefreshWorkers = %d; want 1", cfg.MetadataRefreshWorkers)
	}
	if cfg.MetadataRefreshMinAge != "23h" {
		t.Errorf("MetadataRefreshMinAge = %q; want \"23h\"", cfg.MetadataRefreshMinAge)
	}
	if got := cfg.MetadataRefreshMinAgeDuration(); got != 23*time.Hour {
		t.Errorf("MetadataRefreshMinAgeDuration() = %v; want 23h", got)
	}
}

func TestMetadataRefreshMinAgeDuration_FallbackOnGarbage(t *testing.T) {
	c := &config.Config{MetadataRefreshMinAge: "not-a-duration"}
	if got := c.MetadataRefreshMinAgeDuration(); got != 23*time.Hour {
		t.Errorf("fallback = %v; want 23h", got)
	}
}
```

Ensure `"time"` is imported in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run 'TestLoad_MetadataRefreshDefaults|TestMetadataRefreshMinAgeDuration_FallbackOnGarbage' -v`
Expected: FAIL — `cfg.MetadataRefreshWorkers` / `MetadataRefreshMinAge` / `MetadataRefreshMinAgeDuration` undefined (compile error).

- [ ] **Step 3: Add the fields and helper**

In `internal/config/config.go`, in the Scheduler block right after the `MetadataRefreshInterval` field (~line 100):

```go
	// MetadataRefreshWorkers is the concurrency of the dedicated metadata_refresh
	// River queue. Default 1 keeps the nightly refresh to ~half the IGDB rate
	// budget so user-facing IGDB traffic stays responsive; raise it to finish
	// faster at the cost of crowding user IGDB requests.
	MetadataRefreshWorkers int `env:"METADATA_REFRESH_WORKERS" envDefault:"1"`

	// MetadataRefreshMinAge is a Go duration string; a game is only re-refreshed
	// once its last_updated is older than this. Set slightly below
	// METADATA_REFRESH_INTERVAL (default 23h vs 24h) so a game refreshed
	// yesterday is reliably eligible today despite scheduler jitter.
	MetadataRefreshMinAge string `env:"METADATA_REFRESH_MIN_AGE" envDefault:"23h"`
```

Then add the helper (place it after `Load`, and ensure `"time"` is imported — it already is in this file):

```go
// MetadataRefreshMinAgeDuration parses MetadataRefreshMinAge, falling back to
// 23h on an empty/invalid value or a non-positive duration.
func (c *Config) MetadataRefreshMinAgeDuration() time.Duration {
	d, err := time.ParseDuration(c.MetadataRefreshMinAge)
	if err != nil || d <= 0 {
		return 23 * time.Hour
	}
	return d
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run 'TestLoad_MetadataRefreshDefaults|TestMetadataRefreshMinAgeDuration_FallbackOnGarbage' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "fix(config): add metadata-refresh worker count and min-age knobs (#1134)"
```

---

### Task 2: Dedicated `metadata_refresh` queue + item routing

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go` (add queue const; set `Queue` in `MetadataRefreshItemArgs.InsertOpts()`)
- Modify: `cmd/nexorious/serve.go` (register the queue in both `Queues` maps, ~line 300 and ~line 414)
- Test: `internal/worker/tasks/metadata_refresh_test.go`

**Interfaces:**
- Consumes: `config.Config.MetadataRefreshWorkers` (Task 1).
- Produces: `tasks.QueueMetadataRefresh = "metadata_refresh"` const; `MetadataRefreshItemArgs.InsertOpts().Queue == tasks.QueueMetadataRefresh`.

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/metadata_refresh_test.go`:

```go
func TestMetadataRefreshItemArgs_InsertOptsQueue(t *testing.T) {
	opts := tasks.MetadataRefreshItemArgs{}.InsertOpts()
	if opts.Queue != tasks.QueueMetadataRefresh {
		t.Errorf("Queue = %q; want %q", opts.Queue, tasks.QueueMetadataRefresh)
	}
	if tasks.QueueMetadataRefresh != "metadata_refresh" {
		t.Errorf("QueueMetadataRefresh = %q; want \"metadata_refresh\"", tasks.QueueMetadataRefresh)
	}
	if opts.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %d; want 5 (unchanged)", opts.MaxAttempts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshItemArgs_InsertOptsQueue -v`
Expected: FAIL — `tasks.QueueMetadataRefresh` undefined (compile error).

- [ ] **Step 3: Add the const and route the item jobs**

In `internal/worker/tasks/metadata_refresh.go`, add near the top of the file (after the imports, before the Dispatch worker section):

```go
// QueueMetadataRefresh is the dedicated, low-concurrency River queue for
// metadata_refresh_item jobs. Keeping refresh items off the default queue stops
// the nightly batch from starving user-initiated syncs/imports; its low worker
// count (config METADATA_REFRESH_WORKERS, default 1) also caps how much of the
// shared IGDB rate budget the refresh consumes. Registered in cmd/nexorious/serve.go.
const QueueMetadataRefresh = "metadata_refresh"
```

Then change `MetadataRefreshItemArgs.InsertOpts()` (currently `return river.InsertOpts{MaxAttempts: 5, Priority: 3}`) to:

```go
func (MetadataRefreshItemArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, Priority: 3, Queue: QueueMetadataRefresh}
}
```

- [ ] **Step 4: Register the queue in both River clients**

In `cmd/nexorious/serve.go`, change the `Queues` map at ~line 300 from:

```go
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.WorkerCount}},
```

to:

```go
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:           {MaxWorkers: cfg.WorkerCount},
			tasks.QueueMetadataRefresh:   {MaxWorkers: cfg.MetadataRefreshWorkers},
		},
```

Apply the identical change to the second `Queues` map (~line 414, the post-restore re-init client).

- [ ] **Step 5: Run test + build to verify**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshItemArgs_InsertOptsQueue -v && go build ./...`
Expected: test PASS; build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go cmd/nexorious/serve.go
git commit -m "fix(worker): run metadata-refresh items on a dedicated low-concurrency queue (#1134)"
```

---

### Task 3: Staleness filter in the dispatch query

**Files:**
- Modify: `internal/worker/tasks/metadata_refresh.go` (add `MinAge` field to `MetadataRefreshDispatchWorker`; filter the games query)
- Modify: `cmd/nexorious/serve.go` (set `MinAge` in both worker constructions, ~line 242 and ~line 366)
- Test: `internal/worker/tasks/metadata_refresh_test.go`

**Interfaces:**
- Consumes: `config.Config.MetadataRefreshMinAgeDuration()` (Task 1).
- Produces: `MetadataRefreshDispatchWorker.MinAge time.Duration` — when `> 0`, only games with `last_updated IS NULL OR last_updated < now() - MinAge` are enqueued; when `0`, all games (back-compat for any caller that leaves it unset).

- [ ] **Step 1: Write the failing test**

Add to `internal/worker/tasks/metadata_refresh_test.go`. This mirrors the existing `TestMetadataRefreshDispatch_CreatesJobAndItems` setup (self-created path: needs an admin user + a non-nil River client via `newTestMetadataRiverClient`):

```go
func TestMetadataRefreshDispatch_StalenessFilter(t *testing.T) {
	truncateAllTables(t)
	ctx := context.Background()

	// Admin owner required by the self-created dispatch path.
	insertMetaRefreshAdminUser(t)

	now := time.Now().UTC()
	insertTestGame(t, 101, "Fresh", now.Add(-1*time.Hour))    // within 23h → excluded
	insertTestGame(t, 102, "Stale", now.Add(-48*time.Hour))   // older than 23h → included
	// Never-refreshed game (last_updated NULL) → always included.
	if _, err := testDB.NewRaw(
		`INSERT INTO games (id, title, last_updated, created_at) VALUES (?, ?, NULL, now())`,
		int32(103), "NeverRefreshed",
	).Exec(ctx); err != nil {
		t.Fatalf("insert null game: %v", err)
	}

	srv := igdbTestServer(t, `[]`)
	igdbClient := newTestIGDBClient(t, srv)
	rc := newTestMetadataRiverClient(t)
	w := &tasks.MetadataRefreshDispatchWorker{DB: testDB, IGDBClient: igdbClient, RiverClient: rc, MinAge: 23 * time.Hour}

	if err := w.Work(ctx, &river.Job[tasks.MetadataRefreshDispatchArgs]{Args: tasks.MetadataRefreshDispatchArgs{}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	var keys []string
	if err := testDB.NewRaw(
		`SELECT item_key FROM job_items ORDER BY item_key`,
	).Scan(ctx, &keys); err != nil {
		t.Fatalf("scan job_items: %v", err)
	}
	// 102 (stale) and 103 (null) enqueued; 101 (fresh) excluded.
	want := []string{"102", "103"}
	if len(keys) != len(want) || keys[0] != want[0] || keys[1] != want[1] {
		t.Errorf("enqueued item_keys = %v; want %v", keys, want)
	}
}
```

The admin-owner helper `insertMetaRefreshAdminUser(t)` is defined in `metadata_refresh_test.go` (used by `TestMetadataRefreshDispatch_CreatesJobAndItems`) — reuse it as written.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshDispatch_StalenessFilter -v`
Expected: FAIL — currently all three games are enqueued (no filter), so `keys` is `[101 102 103]`, not `[102 103]`. (Also `MinAge` field undefined → compile error first; add the field in Step 3.)

- [ ] **Step 3: Add the `MinAge` field and filter the query**

In `internal/worker/tasks/metadata_refresh.go`, add the field to the worker struct:

```go
type MetadataRefreshDispatchWorker struct {
	river.WorkerDefaults[MetadataRefreshDispatchArgs]
	DB          *bun.DB
	IGDBClient  *igdbsvc.Client
	RiverClient *river.Client[pgx.Tx]
	// MinAge, when > 0, restricts dispatch to games not refreshed within the
	// window (or never refreshed). 0 means "all games" (back-compat).
	MinAge time.Duration
}
```

Then replace the games-selection block (currently the single `w.DB.NewRaw(\`SELECT id, title FROM games ORDER BY last_updated ASC\`)` call ~line 110) with a conditional query that applies the staleness filter when `MinAge > 0`:

```go
	// Oldest-refreshed first; skip games refreshed within MinAge (NULL = never
	// refreshed, always eligible).
	var games []struct {
		ID    int32  `bun:"id"`
		Title string `bun:"title"`
	}
	var err error
	if w.MinAge > 0 {
		err = w.DB.NewRaw(
			`SELECT id, title FROM games
			 WHERE last_updated IS NULL OR last_updated < now() - make_interval(secs => ?)
			 ORDER BY last_updated ASC NULLS FIRST`,
			w.MinAge.Seconds(),
		).Scan(ctx, &games)
	} else {
		err = w.DB.NewRaw(`SELECT id, title FROM games ORDER BY last_updated ASC NULLS FIRST`).Scan(ctx, &games)
	}
	if err != nil {
		slog.ErrorContext(ctx, "metadata_refresh_dispatch: query games", logging.KeyErr, err, logging.Cat(logging.CategoryDB))
		return nil
	}
```

Note: this replaces the existing `if err := w.DB.NewRaw(...).Scan(...); err != nil { ... }` block. Make sure no duplicate `err` declaration remains and the later code still references `games`.

- [ ] **Step 4: Wire `MinAge` from config in both worker constructions**

In `cmd/nexorious/serve.go`, set `MinAge` in the dispatch worker at ~line 242:

```go
	metaDispatchWorker := &tasks.MetadataRefreshDispatchWorker{
		DB:         db,
		IGDBClient: igdbClient,
		MinAge:     cfg.MetadataRefreshMinAgeDuration(),
	}
```

And in the post-restore re-init at ~line 366:

```go
			newMetaDispatch := &tasks.MetadataRefreshDispatchWorker{
				DB:         newDB,
				IGDBClient: igdbClient,
				MinAge:     cfg.MetadataRefreshMinAgeDuration(),
			}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/worker/tasks/ -run TestMetadataRefreshDispatch_StalenessFilter -v`
Expected: PASS.

- [ ] **Step 6: Run the full metadata-refresh test group + build**

Run: `go test ./internal/worker/tasks/ -run 'TestMetadataRefresh' -v && go build ./...`
Expected: all PASS; build succeeds. (Confirms the existing dispatch tests — which construct the worker without `MinAge`, i.e. `MinAge == 0` → all-games path — still pass.)

- [ ] **Step 7: Commit**

```bash
git add internal/worker/tasks/metadata_refresh.go cmd/nexorious/serve.go
git commit -m "fix(worker): only refresh games older than the min-age window (#1134)"
```

---

### Task 4: Docs + deadcode reconcile

**Files:**
- Modify: `docs/maintenance.md` (if it documents the metadata-refresh job / its env vars — verify first)
- Possibly Modify: `deploy/helm/values.yaml` + `deploy/helm/values.schema.json` (only if metadata-refresh env vars are surfaced there — verify first)

**Interfaces:** none.

- [ ] **Step 1: Check where the metadata-refresh env vars are documented**

Run:

```bash
grep -rn "METADATA_REFRESH_INTERVAL" docs/ deploy/ README.md 2>/dev/null
```

Expected: shows the existing doc/Helm sites for `METADATA_REFRESH_INTERVAL`. For each file listed, add `METADATA_REFRESH_WORKERS` and `METADATA_REFRESH_MIN_AGE` alongside it, matching the surrounding format. If the grep returns nothing, there is nothing to document — skip to Step 3.

> If a Helm `values.yaml` entry is added, you MUST also register the field in `deploy/helm/values.schema.json` (it uses `additionalProperties: false`) or `helm lint --strict` fails. Mirror how `METADATA_REFRESH_INTERVAL` is represented there.

- [ ] **Step 2: Apply doc/Helm edits**

Add the two new variables with one-line descriptions copied from the config struct doc comments (Task 1). Default values: `METADATA_REFRESH_WORKERS=1`, `METADATA_REFRESH_MIN_AGE=23h`.

- [ ] **Step 3: Deadcode reconcile**

This change adds an exported const and a method and removes no callers, so `make deadcode` is not strictly required by the project rules (pure additions). Run a quick build + vet to be safe:

Run: `go build ./... && go vet ./internal/worker/tasks/ ./internal/config/`
Expected: clean.

- [ ] **Step 4: Commit (only if files changed)**

```bash
git add -A
git commit -m "docs: document metadata-refresh worker/min-age env vars (#1134)"
```

If Step 1 found nothing to change, skip this commit.

---

## Self-Review

**Spec coverage:**
- Dedicated `metadata_refresh` queue, `MaxWorkers` default 1, configurable → Task 1 (config) + Task 2 (const, routing, both client registrations). ✓
- Item jobs routed via `InsertOpts.Queue`; dispatch stays on default queue → Task 2 (dispatch's `MetadataRefreshDispatchArgs.InsertOpts()` is untouched). ✓
- Staleness guard `last_updated IS NULL OR last_updated < now() - interval`, oldest-first, NULL always eligible → Task 3. ✓
- Config knobs `METADATA_REFRESH_WORKERS` (1) and `METADATA_REFRESH_MIN_AGE` ("23h"), safe duration fallback → Task 1. ✓
- No count cap; retries (`MaxAttempts: 5`) unchanged → Task 2 asserts MaxAttempts==5; no cap added. ✓
- No migration → none added. ✓
- Acceptance: queue routing verified by test (Task 2); staleness selection + NULL inclusion verified by test (Task 3). ✓
- Docs/Helm sync → Task 4. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. The two "verify first" steps (Task 3 admin-user helper name, Task 4 doc locations) are explicit grep-and-match instructions, not vague placeholders. ✓

**Type consistency:** `QueueMetadataRefresh` (const), `MinAge time.Duration` (field), `MetadataRefreshMinAgeDuration()` (method), `MetadataRefreshWorkers`/`MetadataRefreshMinAge` (fields) used identically across tasks. ✓

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-21-issue-1134-metadata-refresh-pacing.md`. Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
