# Sev 4 silent error fixes (issue #586) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix seven Sev 4 sites where discarded errors produce wrong-but-non-fatal results (bad response data, wrong orphan decisions, failed rollback continuing silently), and resolve two investigation items in backup/service.go.

**Architecture:** All fixes are localised to existing files. The `RunPsqlCommand` global function becomes an assignable `var` to allow test injection. The `jobItemCounts` helper gains an error return. Import handler tracks and surfaces skipped-record counts. No new packages or abstractions are needed.

**Tech Stack:** Go 1.25, Echo v5, Bun ORM, `log/slog`, testcontainers-go (existing test infra)

---

## File map

| File | Change |
|---|---|
| `internal/backup/tools.go` | Convert `func RunPsqlCommand` → `var RunPsqlCommand` |
| `internal/backup/service.go` | Fix lines 834–835 (rollback); add comment at line 436; remove `_ = manifest` at 677 |
| `internal/backup/service_rollback_test.go` | New — unit test for handleRestoreFailure rollback failure |
| `internal/api/jobs.go` | Change `jobItemCounts` to `(map[string]any, error)`; update 3 callers |
| `internal/api/user_games.go` | Fix lines 947–951 and 1042–1047 (platform relation load) |
| `internal/api/sync.go` | Fix lines 1008–1011 (orphan count query) |
| `internal/api/import.go` | Add skip logic, TotalItems update, skipped_count in response |
| `internal/api/import_test.go` | Add TestImportNexorious_MalformedRecord |

---

## Task 1: Create feature branch

- [ ] **Step 1: Create branch**

```bash
git checkout -b fix/issue-586-sev4-silent-errors
```

---

## Task 2: Make RunPsqlCommand injectable

`tools.go` line 90 — convert the function declaration to a `var` so tests can override it.

**Files:**
- Modify: `internal/backup/tools.go:90`

- [ ] **Step 1: Read the current RunPsqlCommand declaration**

Read `internal/backup/tools.go` lines 88–105 to confirm current state.

- [ ] **Step 2: Convert to var**

In `internal/backup/tools.go`, replace:

```go
func RunPsqlCommand(conn DBConnParams, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql",
		"--host="+conn.Host, "--port="+conn.Port,
		"--username="+conn.User, "--dbname="+conn.DBName,
		"--command="+command,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql command failed: %w\noutput: %s", err, output)
	}
	return nil
}
```

With:

```go
var RunPsqlCommand = func(conn DBConnParams, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql",
		"--host="+conn.Host, "--port="+conn.Port,
		"--username="+conn.User, "--dbname="+conn.DBName,
		"--command="+command,
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql command failed: %w\noutput: %s", err, output)
	}
	return nil
}
```

- [ ] **Step 3: Build to confirm no compilation errors**

```bash
make build
```

Expected: binary compiles without errors.

---

## Task 3: Write failing test for handleRestoreFailure rollback DROP SCHEMA failure

**Files:**
- Create: `internal/backup/service_rollback_test.go`

- [ ] **Step 1: Create the test file**

Create `internal/backup/service_rollback_test.go` with `package backup` (internal, not `backup_test`) so it can call the unexported `handleRestoreFailure` method and override the `RunPsqlCommand` var.

```go
package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createRollbackTestArchive creates a minimal .tar.gz archive suitable for
// handleRestoreFailure to extract. It contains one subdirectory with an empty
// database.sql (psql is never actually called because RunPsqlCommand is
// stubbed to fail before RunPsqlFile is reached).
func createRollbackTestArchive(t *testing.T, backupDir, id string) {
	t.Helper()
	archivePath := filepath.Join(backupDir, id+".tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, entry := range []struct{ name string; isDir bool }{
		{id + "/", true},
		{id + "/database.sql", false},
	} {
		hdr := &tar.Header{
			Name: entry.name,
			Mode: 0o755,
		}
		if entry.isDir {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

func TestHandleRestoreFailure_RollbackDropSchemaFails(t *testing.T) {
	backupDir := t.TempDir()
	storageDir := t.TempDir()

	preRestoreID := "nexorious-backup-20260101-000000"
	createRollbackTestArchive(t, backupDir, preRestoreID)

	// Override RunPsqlCommand to fail on DROP SCHEMA.
	orig := RunPsqlCommand
	t.Cleanup(func() { RunPsqlCommand = orig })
	RunPsqlCommand = func(_ DBConnParams, cmd string) error {
		if strings.Contains(cmd, "DROP SCHEMA") {
			return errors.New("simulated drop schema failure")
		}
		return nil
	}

	var capturedState string
	opts := RestoreOpts{
		SetMaintenance:  func(bool) {},
		ShutdownPool:    func() {},
		StopScheduler:   func() {},
		CloseDB:         func() error { return nil },
		ReconnectDB:     func() (*bun.DB, error) { return nil, errors.New("should not reach reconnect") },
		RebuildServices: func(*bun.DB) error { return nil },
		ReinitMigrator:  func(*bun.DB) error { return nil },
		SetAppState:     func(s string) { capturedState = s },
		MaxMigration:    "99999999999999",
	}

	originalErr := errors.New("primary restore failed")
	svc := &Service{backupPath: backupDir, storagePath: storageDir}

	err := svc.handleRestoreFailure(originalErr, preRestoreID, DBConnParams{}, opts)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rollback failed") {
		t.Errorf("error %q should contain 'rollback failed'", err.Error())
	}
	if !errors.Is(err, originalErr) {
		t.Errorf("error should wrap originalErr; got: %v", err)
	}
	if capturedState != "db_unavailable" {
		t.Errorf("SetAppState got %q, want 'db_unavailable'", capturedState)
	}
}
```

Note: this test references `*bun.DB` — add import `"github.com/uptrace/bun"` to the import block.

- [ ] **Step 2: Verify the test file compiles but the test fails (before the fix)**

```bash
go test ./internal/backup/... -run TestHandleRestoreFailure_RollbackDropSchemaFails -v
```

Expected: compilation succeeds; test FAILS because `handleRestoreFailure` currently ignores the RunPsqlCommand error and doesn't call `SetAppState("db_unavailable")` in that code path.

---

## Task 4: Fix backup/service.go — rollback RunPsqlCommand errors

**Files:**
- Modify: `internal/backup/service.go:834-835`

- [ ] **Step 1: Read the current rollback block**

Read `internal/backup/service.go` lines 828–845.

- [ ] **Step 2: Replace the two discarded RunPsqlCommand calls**

In `internal/backup/service.go`, inside `handleRestoreFailure`, replace:

```go
	terminateCmd := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid();"
	_ = RunPsqlCommand(conn, terminateCmd)
	_ = RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
```

With:

```go
	terminateCmd := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid();"
	if err := RunPsqlCommand(conn, terminateCmd); err != nil {
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed (terminate connections: %v). Original: %w", err, originalErr)
	}
	if err := RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		opts.SetAppState("db_unavailable")
		return fmt.Errorf("restore failed AND rollback failed (drop/recreate schema: %v). Original: %w", err, originalErr)
	}
```

- [ ] **Step 3: Run the backup rollback test — expect PASS**

```bash
go test ./internal/backup/... -run TestHandleRestoreFailure_RollbackDropSchemaFails -v
```

Expected: `--- PASS: TestHandleRestoreFailure_RollbackDropSchemaFails`

- [ ] **Step 4: Run the full backup test suite to confirm no regressions**

```bash
go test ./internal/backup/... -timeout 600s -v 2>&1 | tail -30
```

Expected: all existing tests pass.

---

## Task 5: Fix backup/service.go — investigations (io.Copy comment + manifest removal)

**Files:**
- Modify: `internal/backup/service.go:436` (comment)
- Modify: `internal/backup/service.go:677` (remove manifest suppressor)

- [ ] **Step 1: Read io.Copy context**

Read `internal/backup/service.go` lines 430–442.

- [ ] **Step 2: Add comment explaining why both returns are ignored**

In `internal/backup/service.go`, replace:

```go
		_, _ = io.Copy(h, f)
```

With:

```go
		_, _ = io.Copy(h, f) // hash.Hash.Write never returns an error
```

- [ ] **Step 3: Read the manifest suppressor context**

Read `internal/backup/service.go` lines 653–680.

- [ ] **Step 4: Remove the manifest variable suppressor**

In `internal/backup/service.go` inside `RestoreFromUpload`, replace:

```go
	manifest, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
```

With:

```go
	_, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
```

Then delete the line `_ = manifest`.

- [ ] **Step 5: Build and run backup tests**

```bash
make build && go test ./internal/backup/... -timeout 600s 2>&1 | tail -10
```

Expected: build succeeds, all tests pass.

- [ ] **Step 6: Commit backup fixes**

```bash
git add internal/backup/tools.go internal/backup/service.go internal/backup/service_rollback_test.go
git commit -m "$(cat <<'EOF'
fix: handle RunPsqlCommand errors in backup rollback path (issue #586)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Fix jobs.go — jobItemCounts error return

**Files:**
- Modify: `internal/api/jobs.go:37-75` (helper + 3 callers at lines 179, 294, 412)

- [ ] **Step 1: Read jobItemCounts and its callers**

Read `internal/api/jobs.go` lines 35–80, then lines 175–185, 290–300, and 408–418.

- [ ] **Step 2: Update the jobItemCounts function signature and body**

In `internal/api/jobs.go`, replace:

```go
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) map[string]any {
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	_ = h.db.NewRaw(`
		SELECT status, COUNT(*)::int AS count
		FROM job_items
		WHERE job_id = ?
		GROUP BY status`,
		jobID,
	).Scan(ctx, &counts)
```

With:

```go
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) (map[string]any, error) {
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	if err := h.db.NewRaw(`
		SELECT status, COUNT(*)::int AS count
		FROM job_items
		WHERE job_id = ?
		GROUP BY status`,
		jobID,
	).Scan(ctx, &counts); err != nil {
		return nil, err
	}
```

Then at the end of `jobItemCounts`, change `return map[string]any{...}` to `return map[string]any{...}, nil`.

The full function body after edit (replace everything from `func (h *JobsHandler) jobItemCounts` through its closing brace):

```go
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) (map[string]any, error) {
	type statusCount struct {
		Status string `bun:"status"`
		Count  int    `bun:"count"`
	}
	var counts []statusCount
	if err := h.db.NewRaw(`
		SELECT status, COUNT(*)::int AS count
		FROM job_items
		WHERE job_id = ?
		GROUP BY status`,
		jobID,
	).Scan(ctx, &counts); err != nil {
		return nil, err
	}

	m := map[string]int{
		"pending": 0, "processing": 0, "completed": 0,
		"pending_review": 0, "skipped": 0, "failed": 0, "igdb_failed": 0,
	}
	for _, sc := range counts {
		m[sc.Status] = sc.Count
	}
	total := 0
	for _, v := range m {
		total += v
	}
	percent := 0
	if total > 0 {
		percent = (m["completed"] + m["skipped"]) * 100 / total
	}
	return map[string]any{
		"pending": m["pending"], "processing": m["processing"],
		"completed": m["completed"], "pending_review": m["pending_review"],
		"skipped": m["skipped"], "failed": m["failed"],
		"igdb_failed": m["igdb_failed"],
		"total": total, "percent": percent,
	}, nil
}
```

- [ ] **Step 3: Update the three callers**

**Caller 1 — HandleListJobs (line ~179), uses `context.Background()`:**

Replace:
```go
		progress := h.jobItemCounts(context.Background(), jobs[i].ID)
```
With:
```go
		progress, err := h.jobItemCounts(context.Background(), jobs[i].ID)
		if err != nil {
			slog.Error("jobs: fetch item counts failed", "err", err, "job_id", jobs[i].ID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
		}
```

**Caller 2 — HandleGetActiveJob (line ~294), uses `ctx`:**

Replace:
```go
	progress := h.jobItemCounts(ctx, job.ID)
```
With:
```go
	progress, err := h.jobItemCounts(ctx, job.ID)
	if err != nil {
		slog.Error("jobs: fetch item counts failed", "err", err, "job_id", job.ID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
	}
```

**Caller 3 — HandleGetJob (line ~412), uses `ctx`:**

Replace:
```go
	progress := h.jobItemCounts(ctx, job.ID)
```
With:
```go
	progress, err := h.jobItemCounts(ctx, job.ID)
	if err != nil {
		slog.Error("jobs: fetch item counts failed", "err", err, "job_id", job.ID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
	}
```

Note: the `err` variable at caller 2 and 3 — check whether `err` is already declared in scope from an earlier `:=`. If so, use `=` for `err` in the second assignment and declare `progress` separately with `var progress map[string]any`.

- [ ] **Step 4: Build to confirm no errors**

```bash
make build
```

Expected: no compilation errors.

- [ ] **Step 5: Run jobs tests**

```bash
go test ./internal/api/... -run TestJobs -v -timeout 600s 2>&1 | tail -30
```

Expected: all jobs tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/jobs.go
git commit -m "$(cat <<'EOF'
fix: surface jobItemCounts query errors instead of returning zeroed progress (issue #586)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Fix user_games.go — platform relation load after create/update

**Files:**
- Modify: `internal/api/user_games.go:947-951` (HandleCreatePlatform)
- Modify: `internal/api/user_games.go:1042-1047` (HandleUpdatePlatform)

- [ ] **Step 1: Read both sites**

Read `internal/api/user_games.go` lines 944–955 and lines 1039–1050.

- [ ] **Step 2: Fix HandleCreatePlatform (line 947)**

Replace:

```go
	_ = h.db.NewSelect().Model(plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx)
	return c.JSON(http.StatusCreated, toUserGamePlatformResponse(*plat))
```

With:

```go
	if err := h.db.NewSelect().Model(plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.Error("user_games: load platform relations failed", "err", err, "platform_id", plat.ID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load platform")
	}
	return c.JSON(http.StatusCreated, toUserGamePlatformResponse(*plat))
```

- [ ] **Step 3: Fix HandleUpdatePlatform (line 1042)**

Replace:

```go
	_ = h.db.NewSelect().Model(&plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx)
	return c.JSON(http.StatusOK, toUserGamePlatformResponse(plat))
```

With:

```go
	if err := h.db.NewSelect().Model(&plat).
		Where("id = ?", plat.ID).
		Relation("PlatformRecord").
		Relation("StorefrontRecord").
		Scan(ctx); err != nil {
		slog.Error("user_games: load platform relations failed", "err", err, "platform_id", plat.ID)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load platform")
	}
	return c.JSON(http.StatusOK, toUserGamePlatformResponse(plat))
```

- [ ] **Step 4: Build and run user_games tests**

```bash
make build && go test ./internal/api/... -run TestUserGame -v -timeout 600s 2>&1 | tail -30
```

Expected: all user_games tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/user_games.go
git commit -m "$(cat <<'EOF'
fix: return 500 instead of partial data when platform relation load fails (issue #586)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Fix sync.go — orphan count query

**Files:**
- Modify: `internal/api/sync.go:1008-1011`

- [ ] **Step 1: Read the orphan count block**

Read `internal/api/sync.go` lines 1005–1020.

- [ ] **Step 2: Apply fix**

Replace:

```go
		var otherCount int
		_ = h.db.NewRaw(
			`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
		).Scan(ctx, &otherCount)
```

With:

```go
		var otherCount int
		if err := h.db.NewRaw(
			`SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
		).Scan(ctx, &otherCount); err != nil {
			slog.Error("sync: count other platforms failed", "err", err, "user_game_id", ugID)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to check platform count")
		}
```

- [ ] **Step 3: Build and run sync tests**

```bash
make build && go test ./internal/api/... -run TestSync -v -timeout 600s 2>&1 | tail -30
```

Expected: all sync tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/api/sync.go
git commit -m "$(cat <<'EOF'
fix: return 500 when orphan-count query fails instead of defaulting to zero (issue #586)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Write failing test for import malformed record

**Files:**
- Modify: `internal/api/import_test.go`

- [ ] **Step 1: Add the test to import_test.go**

Append to `internal/api/import_test.go`:

```go
func TestImportNexorious_MalformedRecord(t *testing.T) {
	truncateAllTables(t)
	cfg := testCfg()
	e := newTestEchoPool(t, testDB, cfg)

	_, token := setupTagUser(t, testDB, e, "imp-malformed")

	// Build an export where the second game entry is not valid JSON.
	export := map[string]any{
		"export_version": "1.2",
		"games": []json.RawMessage{
			json.RawMessage(`{"igdb_id":1,"title":"Good Game"}`),
			json.RawMessage(`not-valid-json`),
			json.RawMessage(`{"igdb_id":3,"title":"Another Good Game"}`),
		},
	}
	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}

	rec := postMultipartFile(t, e, "/api/import/nexorious", "export.json", data, token)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	// skipped_count must be 1 (the malformed record).
	skippedCount, ok := resp["skipped_count"].(float64)
	if !ok || int(skippedCount) != 1 {
		t.Fatalf("skipped_count = %v, want 1", resp["skipped_count"])
	}

	// total_items must reflect the 2 good records only.
	totalItems, ok := resp["total_items"].(float64)
	if !ok || int(totalItems) != 2 {
		t.Fatalf("total_items = %v, want 2", resp["total_items"])
	}

	// Only 2 job_items should exist in the DB.
	jobID, _ := resp["job_id"].(string)
	var count int
	if err := testDB.NewRaw(
		`SELECT COUNT(*) FROM job_items WHERE job_id = ?`, jobID,
	).Scan(context.Background(), &count); err != nil {
		t.Fatalf("count job_items: %v", err)
	}
	if count != 2 {
		t.Errorf("job_items count = %d, want 2", count)
	}
}
```

- [ ] **Step 2: Run the test — expect FAIL**

```bash
go test ./internal/api/... -run TestImportNexorious_MalformedRecord -v -timeout 600s
```

Expected: FAIL — `skipped_count` is absent from the response (not yet implemented) and `total_items` is 3.

---

## Task 10: Implement import.go malformed record skip

**Files:**
- Modify: `internal/api/import.go`

- [ ] **Step 1: Read the import handler loop**

Read `internal/api/import.go` lines 107–185.

- [ ] **Step 2: Add skipCount var before the loop**

In `import.go`, find the line:

```go
	// Create one JobItem per game and enqueue a task.
	for i, raw := range export.Games {
```

Replace with:

```go
	// Create one JobItem per game and enqueue a task.
	var skipCount int
	for i, raw := range export.Games {
```

- [ ] **Step 3: Replace the discarded Unmarshal with skip logic**

Replace:

```go
		_ = json.Unmarshal(raw, &gameFields)
```

With:

```go
		if err := json.Unmarshal(raw, &gameFields); err != nil {
			slog.Warn("import: malformed game record, skipping", "record_index", i, "err", err)
			skipCount++
			continue
		}
```

- [ ] **Step 4: Update TotalItems in DB and build the response after the loop**

Find the existing response block:

```go
	return c.JSON(http.StatusOK, map[string]any{
		"job_id":      job.ID,
		"source":      job.Source,
		"status":      job.Status,
		"message":     fmt.Sprintf("Import job created. Processing %d games.", job.TotalItems),
		"total_items": job.TotalItems,
	})
```

Replace with:

```go
	if skipCount > 0 {
		if _, err := h.db.NewRaw(
			`UPDATE jobs SET total_items = total_items - ? WHERE id = ?`,
			skipCount, job.ID,
		).Exec(ctx); err != nil {
			slog.Error("import: update total_items failed", "err", err, "job_id", job.ID)
		} else {
			job.TotalItems -= skipCount
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"job_id":        job.ID,
		"source":        job.Source,
		"status":        job.Status,
		"message":       fmt.Sprintf("Import job created. Processing %d games.", job.TotalItems),
		"total_items":   job.TotalItems,
		"skipped_count": skipCount,
	})
```

- [ ] **Step 5: Run the import malformed record test — expect PASS**

```bash
go test ./internal/api/... -run TestImportNexorious_MalformedRecord -v -timeout 600s
```

Expected: `--- PASS: TestImportNexorious_MalformedRecord`

- [ ] **Step 6: Run full import test suite**

```bash
go test ./internal/api/... -run TestImportNexorious -v -timeout 600s 2>&1 | tail -20
```

Expected: all import tests pass, including the existing `TestImportNexorious_Success` which checks `total_items = 3` (all valid records, `skipped_count` defaults to `0` in the response since `skipCount` is 0).

- [ ] **Step 7: Commit**

```bash
git add internal/api/import.go internal/api/import_test.go
git commit -m "$(cat <<'EOF'
fix: skip malformed import records, log warning, surface skipped_count (issue #586)

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Final quality gate

- [ ] **Step 1: Run all Go tests**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 2: Run linter**

```bash
golangci-lint run
```

Expected: zero lint errors.

- [ ] **Step 3: Run frontend type-check and knip (if any frontend files were touched)**

No frontend files were touched in this PR, so skip this step.

- [ ] **Step 4: Open PR**

```bash
gh pr create \
  --title "fix: stop silently producing wrong data (issue #534 sev 4) (#586)" \
  --body "$(cat <<'EOF'
Closes #586.

## Changes

- `internal/backup/tools.go` — `RunPsqlCommand` converted to an assignable `var` (enables test injection)
- `internal/backup/service.go` — rollback path now propagates `RunPsqlCommand` errors; `io.Copy` annotated; `_ = manifest` removed
- `internal/api/jobs.go` — `jobItemCounts` returns `(map[string]any, error)`; all 3 callers log + 500 on failure
- `internal/api/user_games.go` — platform relation loads after create/update now log + 500 on failure
- `internal/api/sync.go` — orphan-count query logs + 500 on failure
- `internal/api/import.go` — malformed game records are skipped with a WARN log; `skipped_count` surfaced in response; `TotalItems` updated in DB

## Tests

- `TestHandleRestoreFailure_RollbackDropSchemaFails` — backup rollback aborts with wrapped error when DROP SCHEMA fails
- `TestImportNexorious_MalformedRecord` — malformed record is skipped, `skipped_count: 1`, `total_items: 2`, 2 job_items in DB

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
