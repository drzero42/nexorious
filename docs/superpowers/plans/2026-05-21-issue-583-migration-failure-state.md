# Migration Failure State Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make migration failures visible by adding an `AppStateMigrationFailed` state to the migrate FSM, exposing the error through `/api/migrate/status`, and offering a Retry button on the `/migrate` page.

**Architecture:** Add an enum value + an `atomic.Value`-backed error slot on the `Migrator`. Convert the four "rollback to NeedsMigration" sites inside `RunMigrations` into `TransitionToFailed` calls. Update the handler goroutine to also catch `InitNeedsSetup` failures. Extend `HandleStatus` to include the error in the payload. Update the migrate-page JS to render the failed state and reuse `runMigrations()` for retry.

**Tech Stack:** Go 1.25 (stdlib `sync/atomic`, `log/slog`), Echo v5, Bun, vanilla JS in `ui/migrate/index.html`.

**Spec:** [docs/superpowers/specs/2026-05-21-issue-583-migration-failure-state-design.md](../specs/2026-05-21-issue-583-migration-failure-state-design.md)

**Branch:** `fix/issue-583-migration-failed-state`

---

## File Inventory

- Modify: `internal/migrate/migrator.go` — add enum value, `String()` case, `lastError` field, `TransitionToFailed`, `LastError`; rewrite the four internal rollback paths in `RunMigrations`; clear `lastError` when transitioning to `Migrating`.
- Modify: `internal/migrate/handler.go` — extend the `HandleRun` goroutine to surface `InitNeedsSetup` errors; rewrite `HandleStatus` to include the `error` field and skip `PendingCount` when state is `MigrationFailed`.
- Modify: `internal/migrate/migrator_test.go` — add unit tests for `TransitionToFailed`/`LastError` and `RunMigrations` clearing `lastError`.
- Modify: `internal/migrate/handler_test.go` — add a handler-level test for `HandleStatus` returning the error field, and an integration test that triggers `MigrationFailed` via a closed DB.
- Modify: `ui/migrate/index.html` — extract a `checkStatusAndAct()` helper; handle `migration_failed` in the poll and after `event: complete`; turn the button into "Retry" and reset state when re-clicked.

No new files. No changes to `internal/api/router.go` (the existing gate logic catches the new state via the `state != AppStateReady && state != AppStateDBUnavailable` check).

---

## Task 1: Add `AppStateMigrationFailed` enum value and `String()` case

**Files:**
- Modify: `internal/migrate/migrator.go:20-42`
- Test: `internal/migrate/migrator_test.go`

- [ ] **Step 1: Write the failing unit test**

Append this to `internal/migrate/migrator_test.go`:

```go
func TestAppState_String_MigrationFailed(t *testing.T) {
	if got := migrate.AppStateMigrationFailed.String(); got != "migration_failed" {
		t.Errorf("AppStateMigrationFailed.String() = %q, want %q", got, "migration_failed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -timeout 60s ./internal/migrate/ -run TestAppState_String_MigrationFailed -v`
Expected: build failure with `undefined: migrate.AppStateMigrationFailed`.

- [ ] **Step 3: Add the enum value and String case**

In `internal/migrate/migrator.go`, change the `const` block (currently lines 22-27) to:

```go
const (
	AppStateDBUnavailable AppState = iota
	AppStateNeedsMigration
	AppStateMigrating
	AppStateReady
	AppStateMigrationFailed
)
```

And add the case to `String()` (currently lines 29-42):

```go
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
	case AppStateMigrationFailed:
		return "migration_failed"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -timeout 60s ./internal/migrate/ -run TestAppState_String_MigrationFailed -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go
git commit -m "feat(migrate): add AppStateMigrationFailed enum value"
```

---

## Task 2: Add `lastError` storage with `TransitionToFailed` / `LastError`

**Files:**
- Modify: `internal/migrate/migrator.go` (`Migrator` struct ~line 44-56; new methods after `TransitionToReady`)
- Test: `internal/migrate/migrator_test.go`

- [ ] **Step 1: Write the failing unit test**

Append to `internal/migrate/migrator_test.go`:

```go
func TestTransitionToFailed_SetsStateAndStoresError(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrating)

	if got := m.LastError(); got != "" {
		t.Fatalf("LastError before transition = %q, want empty", got)
	}

	m.TransitionToFailed(errors.New("boom"))

	if got := m.State(); got != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", got)
	}
	if got := m.LastError(); got != "boom" {
		t.Errorf("LastError = %q, want %q", got, "boom")
	}
}

func TestTransitionToFailed_OverwritesPreviousError(t *testing.T) {
	m := migrate.NewMigratorForTest(migrate.AppStateMigrating)
	m.TransitionToFailed(errors.New("first"))
	m.TransitionToFailed(errors.New("second"))
	if got := m.LastError(); got != "second" {
		t.Errorf("LastError = %q, want %q", got, "second")
	}
}
```

Add `"errors"` to the imports at the top of the file if it is not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -timeout 60s ./internal/migrate/ -run TestTransitionToFailed -v`
Expected: build failure with `m.TransitionToFailed undefined` and `m.LastError undefined`.

- [ ] **Step 3: Add the field and methods**

In `internal/migrate/migrator.go`, change the `Migrator` struct (currently lines 44-56) to add the `lastError` field. The new field goes at the end of the struct:

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
	bunMig            *bunmigrate.Migrator
	logCh             chan string
	logWriter         io.Writer
	lastError         atomic.Value // string; "" or absent means no failure recorded
}
```

After the existing `TransitionToReady` method (currently lines 107-109), add:

```go
// TransitionToFailed records the error and switches state to AppStateMigrationFailed.
// The stored value is always a string; never store an error value here or
// atomic.Value will panic on subsequent loads of a different concrete type.
func (mg *Migrator) TransitionToFailed(err error) {
	mg.lastError.Store(err.Error())
	mg.state.Store(int32(AppStateMigrationFailed))
}

// LastError returns the most recent migration error message, or "" if none.
func (mg *Migrator) LastError() string {
	v := mg.lastError.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -timeout 60s ./internal/migrate/ -run TestTransitionToFailed -v`
Expected: PASS for both `TestTransitionToFailed_SetsStateAndStoresError` and `TestTransitionToFailed_OverwritesPreviousError`.

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go
git commit -m "feat(migrate): add TransitionToFailed and LastError accessors"
```

---

## Task 3: Convert `RunMigrations` rollback paths to `TransitionToFailed` and clear `lastError` on `Migrating`

**Files:**
- Modify: `internal/migrate/migrator.go:166-221` (`RunMigrations`)
- Test: `internal/migrate/migrator_test.go`

- [ ] **Step 1: Write the failing unit test**

Append to `internal/migrate/migrator_test.go`:

```go
func TestRunMigrations_FailureTransitionsToFailedWithError(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}

	// Close the underlying *sql.DB so bunMig.Lock fails inside RunMigrations.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	err := m.RunMigrations(context.Background())
	if err == nil {
		t.Fatal("RunMigrations: expected error from closed DB, got nil")
	}
	if m.State() != migrate.AppStateMigrationFailed {
		t.Errorf("State = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty, want non-empty")
	}
}

func TestRunMigrations_ClearsLastErrorOnStart(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// Seed a previous failure.
	m.TransitionToFailed(errors.New("previous run failed"))

	if err := m.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if got := m.LastError(); got != "" {
		t.Errorf("LastError after successful run = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -timeout 120s ./internal/migrate/ -run TestRunMigrations_Failure -v`
Expected: FAIL — current `RunMigrations` rolls back to `AppStateNeedsMigration`, not `AppStateMigrationFailed`, and never stores an error.

Run: `go test -timeout 120s ./internal/migrate/ -run TestRunMigrations_ClearsLastError -v`
Expected: FAIL — current `RunMigrations` does not clear `lastError`.

- [ ] **Step 3: Update `RunMigrations` in `internal/migrate/migrator.go`**

Replace the body of `RunMigrations` (currently lines 166-221) with:

```go
func (mg *Migrator) RunMigrations(ctx context.Context) error {
	mg.migrateMu.Lock()
	defer mg.migrateMu.Unlock()

	if AppState(mg.state.Load()) == AppStateMigrating {
		return fmt.Errorf("migrations already in progress")
	}

	ch := make(chan string, 256)
	mg.logCh = ch
	mg.lastError.Store("") // clear any previous failure before starting
	mg.state.Store(int32(AppStateMigrating))

	if err := mg.bunMig.Lock(ctx); err != nil {
		wrapped := fmt.Errorf("migrate: acquire lock: %w", err)
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
	defer mg.bunMig.Unlock(ctx) //nolint:errcheck

	group, err := mg.bunMig.Migrate(ctx)
	if err != nil {
		mg.sendLog(ch, fmt.Sprintf("migration failed: %v\n", err))
		mg.TransitionToFailed(err)
		close(ch)
		return err
	}
	if group.IsZero() {
		mg.sendLog(ch, "No new migrations to run\n")
	} else {
		mg.sendLog(ch, fmt.Sprintf("Migrated to group %s\n", group))
	}

	riverMig, err := rivermigrate.New(riverdatabasesql.New(mg.db.DB), nil)
	if err != nil {
		wrapped := fmt.Errorf("migrate: River migrator: %w", err)
		mg.sendLog(ch, fmt.Sprintf("River migration setup failed: %v\n", err))
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
	riverRes, err := riverMig.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		wrapped := fmt.Errorf("migrate: River: %w", err)
		mg.sendLog(ch, fmt.Sprintf("River migration failed: %v\n", err))
		mg.TransitionToFailed(wrapped)
		close(ch)
		return wrapped
	}
	if len(riverRes.Versions) == 0 {
		mg.sendLog(ch, "No new River migrations to run\n")
	} else {
		for _, v := range riverRes.Versions {
			mg.sendLog(ch, fmt.Sprintf("River migrated version %d\n", v.Version))
		}
	}
	close(ch)
	return nil
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test -timeout 120s ./internal/migrate/ -run TestRunMigrations -v`
Expected: PASS — the new tests pass and the existing `TestRunMigrations_TransitionsToReady` still passes (success path leaves state at `Migrating`, `lastError` cleared).

- [ ] **Step 5: Run the full migrate package test suite to catch regressions**

Run: `go test -timeout 300s ./internal/migrate/...`
Expected: PASS — all migrate tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/migrate/migrator.go internal/migrate/migrator_test.go
git commit -m "feat(migrate): RunMigrations transitions to MigrationFailed on error"
```

---

## Task 4: Extend `HandleStatus` to include the `error` field

**Files:**
- Modify: `internal/migrate/handler.go:47-57` (`HandleStatus`)
- Test: `internal/migrate/handler_test.go`

- [ ] **Step 1: Write the failing test**

Append this test to `internal/migrate/handler_test.go`. Add `"errors"` to the imports at the top of the file if it is not already present.

```go
func TestHandleStatus_MigrationFailedIncludesError(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	m.TransitionToFailed(errors.New("boom: schema is haunted"))

	h := migrate.NewHandler(m, nil)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HandleStatus(c); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var body struct {
		PendingCount int    `json:"pending_count"`
		State        string `json:"state"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.State != "migration_failed" {
		t.Errorf("state = %q, want migration_failed", body.State)
	}
	if body.Error != "boom: schema is haunted" {
		t.Errorf("error = %q, want %q", body.Error, "boom: schema is haunted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -timeout 60s ./internal/migrate/ -run TestHandleStatus_MigrationFailedIncludesError -v`
Expected: FAIL — current `HandleStatus` does not include the `error` field and returns `state="migrating"` from `SetStateForTest` defaults. (Note: even with the new `TransitionToFailed` from Task 2, the handler does not yet expose the error.)

- [ ] **Step 3: Update `HandleStatus`**

Replace `HandleStatus` in `internal/migrate/handler.go` (currently lines 47-57) with:

```go
func (h *Handler) HandleStatus(c *echo.Context) error {
	state := h.migrator.State()

	// In the failed state PendingCount may itself error (e.g. DB closed),
	// and the UI does not need the pending count to render the failure card.
	if state == AppStateMigrationFailed {
		return c.JSON(http.StatusOK, map[string]any{
			"pending_count": 0,
			"state":         state.String(),
			"error":         h.migrator.LastError(),
		})
	}

	pending, err := h.migrator.PendingCount()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get pending count")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"pending_count": pending,
		"state":         state.String(),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -timeout 60s ./internal/migrate/ -run TestHandleStatus -v`
Expected: PASS — both `TestHandleStatus_NeedsMigration` and `TestHandleStatus_MigrationFailedIncludesError` pass.

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/handler.go internal/migrate/handler_test.go
git commit -m "feat(migrate): include error in /api/migrate/status when failed"
```

---

## Task 5: Update `HandleRun` to surface `InitNeedsSetup` failures and clarify allowed states

**Files:**
- Modify: `internal/migrate/handler.go:59-81` (`HandleRun`)
- Test: `internal/migrate/handler_test.go`

- [ ] **Step 1: Write the failing integration test**

Append this test to `internal/migrate/handler_test.go`. It triggers a real migration failure by closing the DB before kicking off `HandleRun`.

```go
func TestHandleRun_MigrationFailure_StateAndStatus(t *testing.T) {
	db := setupTestDB(t)
	m := migrate.NewMigrator(db)
	if err := m.DetermineState(); err != nil {
		t.Fatalf("DetermineState: %v", err)
	}
	// Close the underlying *sql.DB so RunMigrations fails inside the goroutine.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}
	h := migrate.NewHandler(m, db)

	e := echo.New()
	runReq := httptest.NewRequest(http.MethodPost, "/api/migrate/run", nil)
	runRec := httptest.NewRecorder()
	if err := h.HandleRun(e.NewContext(runReq, runRec)); err != nil {
		t.Fatalf("HandleRun: %v", err)
	}
	if runRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", runRec.Code)
	}

	// Wait up to 2s for the goroutine to transition to MigrationFailed.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.State() == migrate.AppStateMigrationFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.State() != migrate.AppStateMigrationFailed {
		t.Fatalf("state = %v, want AppStateMigrationFailed", m.State())
	}
	if m.LastError() == "" {
		t.Errorf("LastError is empty after failed run")
	}

	// Verify /api/migrate/status reflects the failure.
	statusReq := httptest.NewRequest(http.MethodGet, "/api/migrate/status", nil)
	statusRec := httptest.NewRecorder()
	if err := h.HandleStatus(e.NewContext(statusReq, statusRec)); err != nil {
		t.Fatalf("HandleStatus: %v", err)
	}
	var body struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(statusRec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.State != "migration_failed" {
		t.Errorf("state = %q, want migration_failed", body.State)
	}
	if body.Error == "" {
		t.Errorf("error field is empty in status payload")
	}
}
```

Add `"time"` to the imports at the top of `handler_test.go` if it is not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -timeout 60s ./internal/migrate/ -run TestHandleRun_MigrationFailure -v`
Expected: PASS — the test should actually pass at this point, because Task 3 already wired RunMigrations to call `TransitionToFailed` and Task 4 made the status payload expose the error.

If it does pass, that's expected: this test simply locks in the end-to-end behavior. Treat the next step as a tidy-up for the `InitNeedsSetup` path that is not directly exercised by the closed-DB scenario.

- [ ] **Step 3: Update `HandleRun` goroutine and switch**

Replace `HandleRun` in `internal/migrate/handler.go` (currently lines 59-81) with:

```go
func (h *Handler) HandleRun(c *echo.Context) error {
	switch h.migrator.State() {
	case AppStateMigrating:
		return c.JSON(http.StatusConflict, map[string]string{"error": "migration already in progress"})
	case AppStateReady:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "already up to date"})
	}
	// Allowed: AppStateNeedsMigration and AppStateMigrationFailed.
	// (Gate 1 redirects callers away before AppStateDBUnavailable can reach here.)

	go func() {
		ctx := context.Background()
		if err := h.migrator.RunMigrations(ctx); err != nil {
			slog.Error("migrate: run migrations failed", "err", err)
			// RunMigrations already called TransitionToFailed; nothing else to do.
			return
		}
		if h.db != nil {
			if err := h.migrator.InitNeedsSetup(ctx, h.db); err != nil {
				slog.Error("migrate: init needs-setup failed", "err", err)
				h.migrator.TransitionToFailed(err)
				return
			}
		}
		h.migrator.TransitionToReady()
	}()

	return c.JSON(http.StatusAccepted, map[string]string{"status": "migration started"})
}
```

Add `"log/slog"` to the imports at the top of `handler.go` if it is not already present (the rest of the file does not currently use slog).

- [ ] **Step 4: Run the full migrate test suite**

Run: `go test -timeout 300s ./internal/migrate/...`
Expected: PASS — including the existing `TestHandleRun_202_ThenReady` (success path), `TestHandleRun_409_WhenMigrating`, `TestHandleRun_400_WhenReady`, and the new `TestHandleRun_MigrationFailure_StateAndStatus`.

- [ ] **Step 5: Lint and full Go test pass**

Run: `golangci-lint run ./internal/migrate/...`
Expected: zero findings.

Run: `go test -timeout 600s ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/migrate/handler.go internal/migrate/handler_test.go
git commit -m "fix(migrate): surface migration failures via app state (issue #583)"
```

---

## Task 6: Update `ui/migrate/index.html` to render the failed state and offer Retry

**Files:**
- Modify: `ui/migrate/index.html`

There is no JS test harness for this page, so this task is structural plus a manual smoke check at the end.

- [ ] **Step 1: Replace the inline script with the new structure**

Open `ui/migrate/index.html` and replace the `<script>` block (currently lines 30-99) with:

```html
  <script>
    var pollIntervalMs = 5000;
    var pollTimer = null;

    function startPolling() {
      stopPolling();
      pollTimer = setInterval(function() {
        checkStatusAndAct().catch(function() { /* network blip — keep polling */ });
      }, pollIntervalMs);
    }

    function stopPolling() {
      if (pollTimer !== null) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    }

    // Fetch /api/migrate/status once and react to its state.
    // Returns a promise that resolves after the side-effect (redirect / render) runs.
    function checkStatusAndAct() {
      return fetch('/api/migrate/status')
        .then(function(res) {
          // Gate 1 redirects /api/migrate/status to /db-error when DB is down;
          // fetch follows the redirect and we land on HTML, not JSON.
          var ct = res.headers.get('content-type') || '';
          if (!ct.includes('application/json')) {
            window.location.reload();
            return null;
          }
          return res.json();
        })
        .then(function(data) {
          if (!data) return;
          if (data.state === 'ready') {
            window.location.href = '/';
          } else if (data.state === 'migration_failed') {
            showFailure(data.error);
          }
        });
    }

    function showFailure(message) {
      var btn = document.getElementById('btn');
      var status = document.getElementById('status');
      status.textContent = 'Migration failed: ' + (message || 'unknown error');
      status.className = 'meta meta--error';
      btn.disabled = false;
      btn.textContent = 'Retry';
    }

    function runMigrations() {
      stopPolling();
      var btn = document.getElementById('btn');
      var log = document.getElementById('log');
      var status = document.getElementById('status');

      btn.disabled = true;
      btn.textContent = 'Run Migrations';
      log.textContent = '';
      log.classList.add('visible');
      status.textContent = 'Running migrations…';
      status.className = 'meta';

      fetch('/api/migrate/run', { method: 'POST' })
        .then(function(res) {
          if (!res.ok) {
            throw new Error('Failed to start migration (HTTP ' + res.status + ')');
          }
          var es = new EventSource('/api/migrate/progress');

          es.onmessage = function(e) {
            log.textContent += e.data + '\n';
            log.scrollTop = log.scrollHeight;
          };

          es.addEventListener('complete', function() {
            es.close();
            // RunMigrations closes the SSE channel before the handler runs
            // InitNeedsSetup and calls TransitionToReady, so the server may
            // still report state="migrating" briefly. checkStatusAndAct
            // handles ready/migration_failed; the resumed poll catches up
            // for the brief still-migrating window.
            checkStatusAndAct()
              .then(function() { startPolling(); })
              .catch(function() {
                status.textContent = 'Could not verify migration status. Refresh to retry.';
                status.className = 'meta meta--error';
                btn.disabled = false;
              });
          });

          es.onerror = function() {
            es.close();
            status.textContent = 'Connection lost. Check logs and refresh to retry.';
            status.className = 'meta meta--error';
            btn.disabled = false;
          };
        })
        .catch(function(err) {
          status.textContent = err.message;
          status.className = 'meta meta--error';
          btn.disabled = false;
        });
    }

    // On page load: immediate status check (so a previously-failed state is
    // shown without waiting for the first 5s tick), then start the poll.
    checkStatusAndAct().catch(function() {}).then(function() { startPolling(); });
  </script>
```

- [ ] **Step 2: Rebuild the backend so the embedded UI picks up the change**

Run: `make build`
Expected: build succeeds (the migrate HTML is embedded via `//go:embed all:migrate` in `ui/ui.go`).

- [ ] **Step 3: Manual smoke test — happy path**

1. Drop and recreate the dev DB, or run against a fresh DB so migrations are pending.
2. Run: `./nexorious`
3. Open `http://localhost:8000/migrate` in a browser.
4. Confirm the page shows the pending migration count and a "Run Migrations" button.
5. Click "Run Migrations" — the log streams, status text changes to "Running migrations…", and after completion the page redirects to `/`.

- [ ] **Step 4: Manual smoke test — failure path**

This is the failure scenario the PR fixes. Force a failure by stopping the DB mid-migration or by introducing a deliberately broken migration locally (do not commit the broken migration).

Simpler reproduction: in a `psql` session before clicking "Run Migrations", execute `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = 'nexorious';` and quickly stop postgres. Easier approach: comment out the body of one of the embedded migrations with intentionally invalid SQL in a working-copy edit, rebuild, run, click Run.

1. Trigger a failure with one of the above approaches.
2. Confirm the page shows "Migration failed: <error>" in the error styling and the button becomes "Retry".
3. Fix the underlying cause (e.g., restart postgres, revert the broken-migration edit + rebuild).
4. Click "Retry" — the page kicks off `/api/migrate/run` again and proceeds normally.
5. Also confirm: navigating to `/migrate` from scratch (after a failure has already been recorded) shows the failed state immediately without needing to wait for the 5-second poll.

- [ ] **Step 5: Commit**

```bash
git add ui/migrate/index.html
git commit -m "feat(ui/migrate): render migration_failed state and retry button"
```

---

## Task 7: Final verification and PR

**Files:** none (verification + git only)

- [ ] **Step 1: Full Go test pass**

Run: `go test -timeout 600s ./...`
Expected: PASS.

- [ ] **Step 2: Full lint pass**

Run: `golangci-lint run`
Expected: zero findings.

- [ ] **Step 3: Frontend quality gates (in case any shared assets were touched)**

Run from `ui/frontend/`: `npm run check && npm run knip`
Expected: zero errors/findings. (The migrate page is a separate static HTML — these commands cover the React SPA. They should be unaffected by this PR.)

- [ ] **Step 4: Push and open a PR**

```bash
git push -u origin fix/issue-583-migration-failed-state
gh pr create --title "fix(migrate): surface migration failures via app state (#583)" --body "$(cat <<'EOF'
## Summary
- Adds `AppStateMigrationFailed` to the migrate FSM with a stored error message.
- `RunMigrations` and the `HandleRun` goroutine now transition to the failed state instead of silently rolling back / swallowing the error.
- `/api/migrate/status` includes the error in the failed state and the `/migrate` page renders it with a Retry button.

Closes #583. Child A of #534.

## Test plan
- [ ] `go test -timeout 600s ./...` passes
- [ ] `golangci-lint run` passes
- [ ] Manual: fresh DB → migrate → success redirect to `/`
- [ ] Manual: force migration failure → page shows the error + Retry button; Retry runs the migration again
- [ ] Manual: navigate to `/migrate` after a prior failure → failed state renders immediately (no 5s wait)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Do not merge — wait for human review.

---

## Self-Review Notes

- **Spec coverage:**
  - State machine (`AppStateMigrationFailed`) → Task 1.
  - Error storage + transition methods → Task 2.
  - Internal rollback paths replaced → Task 3.
  - Handler goroutine wraps `InitNeedsSetup` → Task 5.
  - Status payload includes `error` and tolerates `PendingCount` failure → Task 4.
  - Route gates unchanged → verified in the File Inventory (no router edits).
  - UI Retry affordance + initial-load handling → Task 6.
  - Tests (migrator unit + handler integration) → Tasks 2, 3, 4, 5.

- **Placeholder scan:** all code blocks contain final code; no TBDs.

- **Type/name consistency:**
  - `TransitionToFailed(err error)` and `LastError() string` — used identically across Tasks 2-6.
  - `AppStateMigrationFailed` capitalization matches the existing `AppStateXxx` naming.
  - JSON field is `"error"` everywhere (handler, test, UI fetch).
  - Status string is `"migration_failed"` everywhere (handler `String()`, test, UI poll branch).
  - JS function names (`checkStatusAndAct`, `showFailure`, `startPolling`, `stopPolling`, `runMigrations`) match between the inline script and the call sites within it.
