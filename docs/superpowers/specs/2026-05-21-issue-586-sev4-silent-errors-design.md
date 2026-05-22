# Sev 4 silent error fixes (issue #586)

Date: 2026-05-21
Tracking issue: [#586](https://github.com/drzero42/nexorious/issues/586)
Parent spec: [2026-05-21-issue-534-silent-errors-design.md](./2026-05-21-issue-534-silent-errors-design.md)
Milestone: 0.1.0

## Background

Child issue of #534 (Sev 4). Sev 4 covers SELECTs feeding response data, restore/cleanup operations,
and partial-data losses — failures produce a wrong but non-fatal result. The parent design doc defines
the classification criteria and fix conventions; this spec covers the seven named sites plus two inline
investigations.

**Dependency**: land after the Sev 3 PR (#585), which is already merged.

## Sites to fix

### `internal/api/user_games.go:947` — HandleCreatePlatform

Current:
```go
_ = h.db.NewSelect().Model(plat).
    Where("id = ?", plat.ID).
    Relation("PlatformRecord").
    Relation("StorefrontRecord").
    Scan(ctx)
return c.JSON(http.StatusCreated, toUserGamePlatformResponse(*plat))
```

If the SELECT fails, the handler returns 201 with an incompletely-populated struct (relations are nil).
Fix: check the error, log, and return 500.

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

### `internal/api/user_games.go:1042` — HandleUpdatePlatform

Identical pattern to the site above. Apply the same fix; include `"platform_id", plat.ID` in the log.

### `internal/api/import.go:131` — HandleImport loop

Current:
```go
_ = json.Unmarshal(raw, &gameFields)
```

If the unmarshal fails, item_key and source_title fall back to index-based defaults and a malformed
game record silently becomes a job item.

Fix: on failure, log at WARN, increment a skip counter, and `continue`.

```go
if err := json.Unmarshal(raw, &gameFields); err != nil {
    slog.Warn("import: malformed game record, skipping", "record_index", i, "err", err)
    skipCount++
    continue
}
```

Declare `var skipCount int` before the loop. After the loop, if `skipCount > 0`:

1. Update `job.TotalItems` in the DB:
   ```go
   if _, err := h.db.NewRaw(
       `UPDATE jobs SET total_items = total_items - ? WHERE id = ?`,
       skipCount, job.ID,
   ).Exec(ctx); err != nil {
       slog.Error("import: update total_items failed", "err", err, "job_id", job.ID)
   }
   ```
2. Include `"skipped_count": skipCount` in the handler JSON response alongside `job_id`, `status`, etc.

### `internal/backup/service.go:834-835` — `handleRestoreFailure` rollback

Current (rollback path, inside `handleRestoreFailure`):
```go
_ = RunPsqlCommand(conn, terminateCmd)
_ = RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
```

The primary restore path (`doRestore`, lines 755-759) already handles these identically — follow that
pattern. If either command fails, set the app state to `db_unavailable` and return a wrapped error that
includes the original error.

```go
if err := RunPsqlCommand(conn, terminateCmd); err != nil {
    opts.SetAppState("db_unavailable")
    return fmt.Errorf("restore failed AND rollback failed (terminate connections: %v). Original: %w", err, originalErr)
}
if err := RunPsqlCommand(conn, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
    opts.SetAppState("db_unavailable")
    return fmt.Errorf("restore failed AND rollback failed (drop/recreate schema: %v). Original: %w", err, originalErr)
}
```

### `internal/api/sync.go:1008-1011` — HandleRematchExternalGame orphan count

Current:
```go
var otherCount int
_ = h.db.NewRaw(
    `SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
).Scan(ctx, &otherCount)
```

`otherCount` drives the orphan-action check: if the count query fails, `otherCount` is 0, so the
handler may silently require `orphan_action` when it shouldn't (or skip it when it should require it).
Fix: log + 500.

```go
var otherCount int
if err := h.db.NewRaw(
    `SELECT COUNT(*) FROM user_game_platforms WHERE user_game_id = ? AND id != ?`, ugID, ugpID,
).Scan(ctx, &otherCount); err != nil {
    slog.Error("sync: count other platforms failed", "err", err, "user_game_id", ugID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to check platform count")
}
```

### `internal/api/jobs.go:43` — `jobItemCounts`

Current: `_ = h.db.NewRaw(...).Scan(ctx, &counts)` inside a helper that returns `map[string]any`.

If the query fails, the returned progress map has all-zero counts and the caller returns incorrect data.
Fix: change the helper signature to return `(map[string]any, error)` and update all three callers.

New signature:
```go
func (h *JobsHandler) jobItemCounts(ctx context.Context, jobID string) (map[string]any, error)
```

On scan failure, return `nil, err`. At the end, return `result, nil`.

Callers at lines 179, 294, 412 — each is inside a handler:
```go
progress, err := h.jobItemCounts(ctx, job.ID)
if err != nil {
    slog.Error("jobs: fetch item counts failed", "err", err, "job_id", job.ID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to load job progress")
}
```

The caller at line 179 uses `context.Background()` (list endpoint); the others use the request context.

## Investigations

### `internal/backup/service.go:436` — `io.Copy` for hashing (acceptable)

```go
_, _ = io.Copy(h, f)
```

`h` is a `hash.Hash` (e.g. `sha256.New()`). The `Write` method on hash implementations is specified to
never return an error. This is safe to leave as-is. Add a short inline comment:

```go
_, _ = io.Copy(h, f) // hash.Hash.Write never returns an error
```

### `internal/backup/service.go:677` — `_ = manifest` (variable suppression)

`manifest` is returned by `ValidateArchive` but never used — the blank-identifier line is a Go
"declared and not used" suppressor. Fix: discard the variable at the call site and remove the
`_ = manifest` line.

```go
// Before
manifest, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
// ...
_ = manifest

// After
_, err := s.ValidateArchive(uploadedPath, true, opts.MaxMigration)
```

## Tests

Two tests are required (as specified in the issue):

**1. Import: malformed record is skipped**
- Construct a nexorious export JSON where one game record is invalid JSON (e.g. `json.RawMessage("not json")`).
- POST to `/api/import`.
- Assert HTTP 200, response body contains `"skipped_count": 1`, and `"total_items"` equals the total count minus 1.
- Assert only the non-malformed records appear as job items in the DB.

**2. Backup restore: DROP SCHEMA failure aborts with wrapped error**
- Unit-test `handleRestoreFailure` directly via a stub or test double for `RunPsqlCommand`.
- Simulate `RunPsqlCommand` returning an error for the DROP SCHEMA command.
- Assert the returned error wraps both the original error and the rollback error.
- Assert `opts.SetAppState` was called with `"db_unavailable"`.

## Fix conventions (from parent spec)

Log format: `slog.Error("<domain>: <action> failed", "err", err, "<context_key>", value)`

API handler default: log + `echo.NewHTTPError(http.StatusInternalServerError, "<short message>")`.
