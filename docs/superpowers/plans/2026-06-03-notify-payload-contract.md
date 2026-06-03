# Notify Payload Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the swallowed `json.Unmarshal` decodes in `internal/notify` with handled, logged errors and a compile-time-enforced typed payload contract, backed by a registry-driven round-trip test.

**Architecture:** Three layers. (1) `Format` returns a `decodeErr` and renders a safe, non-misleading body on failure; its two callers log the error. (2) One typed payload struct per event shape becomes the single source of truth shared by every emit site and `Format`. (3) A registry-driven test forces every registered event type to have a sample payload + `Format` case and locks the rendered copy.

**Tech Stack:** Go 1.25, `encoding/json`, `log/slog`, stdlib `testing`. Spec: `docs/superpowers/specs/2026-06-03-notify-payload-contract-design.md`.

---

## File Structure

- `internal/notify/payloads.go` — **new.** All typed payload structs + `DiffGame` (moved from `formatters.go`). Single source of truth for the emit↔format contract.
- `internal/notify/formatters.go` — `Format` gains a `decodeErr` return; decodes into the named structs; safe fallbacks; `failBody`/`maintBody` become pure formatters.
- `internal/notify/emit.go` — `EmitParams.Payload` changes `map[string]any` → `any`.
- `internal/notify/worker.go` — log `decodeErr` from `Format`.
- `internal/notify/prune.go` — emit typed `MaintPayload`.
- `internal/notify/formatters_test.go` — update existing tests to the 3-value signature; add the safe-fallback test and the registry round-trip golden test.
- `internal/api/events.go` — log `decodeErr` from `Format`.
- `internal/scheduler/scheduler.go` — narrow `emitMaint`; drop `maps` import.
- `internal/scheduler/orphaned_items.go`, `internal/scheduler/stale_jobs.go` — typed `MaintPayload` call sites.
- `internal/scheduler/backup_poll.go`, `internal/worker/tasks/{sync,export,import_item,metadata_refresh}.go` — typed payloads at emit sites.

---

## Task 1: Typed payload structs

Pure additive type definitions. `DiffGame` moves out of `formatters.go` into the new file (same package, so no import churn). No behavior change.

**Files:**
- Create: `internal/notify/payloads.go`
- Modify: `internal/notify/formatters.go` (remove the `DiffGame` definition)

- [ ] **Step 1: Create `internal/notify/payloads.go`**

```go
package notify

// Typed event payloads — the single source of truth for the emit↔format
// contract. Every emit site constructs one of these; Format decodes into the
// same type. A renamed field or changed type therefore moves both sides
// together and cannot compile out of sync.

// DiffGame is one entry in a sync.diff payload.
type DiffGame struct {
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}

type SyncCompletedPayload struct {
	Storefront string `json:"storefront"`
	JobID      string `json:"job_id"`
}

type SyncCompletedWithErrorsPayload struct {
	Storefront string `json:"storefront"`
	Failed     int    `json:"failed"`
	JobID      string `json:"job_id"`
}

type SyncFailedPayload struct {
	Storefront string `json:"storefront"`
	Error      string `json:"error"`
	JobID      string `json:"job_id"`
}

type SyncNeedsReviewPayload struct {
	Storefront string `json:"storefront"`
	Count      int    `json:"count"`
	JobID      string `json:"job_id"`
}

type SyncDiffPayload struct {
	Added   []DiffGame `json:"added"`
	Removed []DiffGame `json:"removed"`
	JobID   string     `json:"job_id"`
}

type ImportCompletedPayload struct {
	JobID string `json:"job_id"`
}

type ImportFailedPayload struct {
	JobID  string `json:"job_id"`
	Failed int    `json:"failed"`
	Error  string `json:"error"`
}

type ExportCompletedPayload struct {
	JobID    string `json:"job_id"`
	FilePath string `json:"file_path"`
}

type ExportFailedPayload struct {
	JobID string `json:"job_id"`
	Error string `json:"error"`
}

type BackupCompletedPayload struct {
	BackupID string `json:"backup_id"`
}

type BackupFailedPayload struct {
	Error string `json:"error"`
}

// MaintPayload is shared by admin.maintenance.completed and
// admin.maintenance.failed. The numeric fields are a union over what the
// maintenance jobs report (prune/metadata → count; orphaned-items →
// rescued+failed; stale-jobs → count); all optional. Format renders only
// Action and Error.
type MaintPayload struct {
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
	Count   int    `json:"count,omitempty"`
	Rescued int    `json:"rescued,omitempty"`
	Failed  int    `json:"failed,omitempty"`
}
```

- [ ] **Step 2: Remove the `DiffGame` definition from `formatters.go`**

Delete these lines near the top of `internal/notify/formatters.go` (they now live in `payloads.go`):

```go
// DiffGame is one entry in a sync.diff payload.
type DiffGame struct {
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}
```

- [ ] **Step 3: Verify the package builds**

Run: `go build ./internal/notify/...`
Expected: no output (success). `DiffGame` resolves from `payloads.go`.

- [ ] **Step 4: Commit**

```bash
git add internal/notify/payloads.go internal/notify/formatters.go
git commit -m "refactor(notify): add typed event payload structs"
```

---

## Task 2: `Format` returns decodeErr with safe fallbacks; callers log

Change `Format`'s signature, decode into the named structs, render a safe body on decode failure, and make `failBody`/`maintBody` pure formatters. Update both callers to log, and update existing tests to the new signature. TDD: the new safe-fallback test is written first and fails to compile until the signature changes.

**Files:**
- Modify: `internal/notify/formatters.go`
- Modify: `internal/notify/worker.go:37`
- Modify: `internal/api/events.go:171`
- Test: `internal/notify/formatters_test.go`

- [ ] **Step 1: Write the failing safe-fallback test**

Add to `internal/notify/formatters_test.go`:

```go
func TestFormat_DecodeFailureSafeFallback(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		payload   string
		wantBody  string
	}{
		{"with_errors", TypeSyncCompletedWithErrors, `{"failed":"oops"}`, "Your library sync finished with some failed item(s)."},
		{"needs_review", TypeSyncNeedsReview, `{"count":"oops"}`, "Your library sync has item(s) needing review."},
		{"sync_failed", TypeSyncFailed, `["not","an","object"]`, "Sync failed."},
		{"sync_diff", TypeSyncDiff, `{"added":"nope"}`, "Your game library changed."},
		{"import_failed", TypeImportFailed, `"notobject"`, "Your import failed."},
		{"export_failed", TypeExportFailed, `"notobject"`, "Your export failed."},
		{"backup_failed", TypeAdminBackupFailed, `"notobject"`, "A scheduled backup failed."},
		{"maint_failed", TypeAdminMaintFailed, `"notobject"`, "Maintenance task failed."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, body, err := Format(tc.eventType, json.RawMessage(tc.payload))
			if err == nil {
				t.Fatalf("expected a decode error, got nil")
			}
			if title == "" {
				t.Fatalf("title must not be empty")
			}
			if body != tc.wantBody {
				t.Fatalf("body = %q, want %q", body, tc.wantBody)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails (compile error)**

Run: `go test ./internal/notify/ -run TestFormat_DecodeFailureSafeFallback -v`
Expected: FAIL — build error `assignment mismatch: 3 variables but Format returns 2 values`.

- [ ] **Step 3: Rewrite `Format` and the helpers in `formatters.go`**

Replace the `Format` function and the `failBody`/`maintBody` helpers with:

```go
// Format renders a (title, body) pair for an event type + payload, plus any
// payload-decode error. Unknown types and malformed payloads fall back to a
// generic, never-empty message; on a decode failure the body omits the
// untrusted fields rather than rendering zero-valued data. Callers should log
// a non-nil decodeErr (it signals schema drift or a corrupt stored payload).
func Format(eventType string, payload json.RawMessage) (title, body string, decodeErr error) {
	meta, ok := Meta(eventType)
	label := eventType
	if ok {
		label = meta.Label
	}

	switch eventType {
	case TypeSyncFailed:
		var p SyncFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync failed"
		if decodeErr != nil {
			body = "Sync failed."
		} else {
			body = fmt.Sprintf("Your %s sync failed: %s", fallback(p.Storefront, "library"), fallback(p.Error, "unknown error"))
		}

	case TypeSyncCompleted:
		var p SyncCompletedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync completed"
		body = fmt.Sprintf("Your %s sync completed successfully.", fallback(p.Storefront, "library"))

	case TypeSyncCompletedWithErrors:
		var p SyncCompletedWithErrorsPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync completed with errors"
		if decodeErr != nil {
			body = fmt.Sprintf("Your %s sync finished with some failed item(s).", fallback(p.Storefront, "library"))
		} else {
			body = fmt.Sprintf("Your %s sync finished with %d failed item(s).", fallback(p.Storefront, "library"), p.Failed)
		}

	case TypeSyncNeedsReview:
		var p SyncNeedsReviewPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Sync needs review"
		if decodeErr != nil {
			body = fmt.Sprintf("Your %s sync has item(s) needing review.", fallback(p.Storefront, "library"))
		} else {
			body = fmt.Sprintf("Your %s sync has %d item(s) needing review.", fallback(p.Storefront, "library"), p.Count)
		}

	case TypeSyncDiff:
		var p SyncDiffPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Game library changes"
		if decodeErr != nil {
			body = "Your game library changed."
		} else {
			body = formatDiff(p.Added, p.Removed)
		}

	case TypeImportCompleted:
		title, body = "Import completed", "Your import finished successfully."
	case TypeImportFailed:
		var p ImportFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Import failed"
		body = failBody(p.Error, "Your import failed", decodeErr)
	case TypeExportCompleted:
		title, body = "Export completed", "Your export is ready."
	case TypeExportFailed:
		var p ExportFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Export failed"
		body = failBody(p.Error, "Your export failed", decodeErr)

	case TypeAdminBackupCompleted:
		title, body = "Backup completed", "A scheduled backup completed successfully."
	case TypeAdminBackupFailed:
		var p BackupFailedPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Backup failed"
		body = failBody(p.Error, "A scheduled backup failed", decodeErr)
	case TypeAdminMaintCompleted:
		var p MaintPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Maintenance completed"
		body = maintBody(p.Action, p.Error, "Maintenance task completed", decodeErr)
	case TypeAdminMaintFailed:
		var p MaintPayload
		decodeErr = json.Unmarshal(payload, &p)
		title = "Maintenance failed"
		body = maintBody(p.Action, p.Error, "Maintenance task failed", decodeErr)

	default:
		title = label
		body = "An event occurred: " + eventType
	}

	if title == "" {
		title = label
	}
	if body == "" {
		body = "An event occurred: " + eventType
	}
	return title, body, decodeErr
}
```

Replace the old `failBody` and `maintBody` (which took `payload json.RawMessage`) with these pure formatters:

```go
// failBody renders "<prefix>: <error>", or "<prefix>." when the error is empty
// or the payload failed to decode.
func failBody(errMsg, prefix string, decodeErr error) string {
	if decodeErr != nil || errMsg == "" {
		return prefix + "."
	}
	return prefix + ": " + errMsg
}

// maintBody renders "<prefix> (action) - error.", omitting any part that is
// absent or that could not be decoded.
func maintBody(action, errMsg, prefix string, decodeErr error) string {
	parts := []string{prefix}
	if decodeErr == nil {
		if action != "" {
			parts = append(parts, "("+action+")")
		}
		if errMsg != "" {
			parts = append(parts, "- "+errMsg)
		}
	}
	return strings.Join(parts, " ") + "."
}
```

Leave `formatDiff` and `fallback` unchanged.

- [ ] **Step 4: Update the two callers to log decodeErr**

In `internal/notify/worker.go`, replace line 37 (`title, body := Format(ev.Type, ev.Payload)`) with:

```go
	title, body, derr := Format(ev.Type, ev.Payload)
	if derr != nil {
		slog.Warn("notify: payload decode failed", "event_id", ev.ID, "type", ev.Type, "err", derr)
	}
```

In `internal/api/events.go`, replace line 171 (`title, body := notify.Format(r.Type, r.Payload)`) with:

```go
		title, body, derr := notify.Format(r.Type, r.Payload)
		if derr != nil {
			slog.Warn("notify: payload decode failed", "event_id", r.ID, "type", r.Type, "err", derr)
		}
```

(`events.go` already imports `log/slog` — it uses `slog.Error` at line 156. If a build error reports `slog` unused/missing, add `"log/slog"` to its imports.)

- [ ] **Step 5: Update the existing tests to the 3-value signature**

In `internal/notify/formatters_test.go`, update the four existing `Format(...)` calls:

- `TestFormatSyncFailed`: `title, body := Format(TypeSyncFailed, payload)` → `title, body, _ := Format(TypeSyncFailed, payload)`
- `TestFormatSyncDiff`: `title, body := Format(TypeSyncDiff, payload)` → `title, body, _ := Format(TypeSyncDiff, payload)`
- `TestFormatSyncDiff_EmptyPlatformsOmitsBrackets`: `_, body := Format(TypeSyncDiff, payload)` → `_, body, _ := Format(TypeSyncDiff, payload)`
- `TestFormatUnknownTypeIsSafe`: `title, body := Format("totally.unknown", json.RawMessage(`{}`))` → `title, body, _ := Format("totally.unknown", json.RawMessage(`{}`))`

- [ ] **Step 6: Run the notify tests**

Run: `go test ./internal/notify/ -run TestFormat -v`
Expected: PASS — `TestFormat_DecodeFailureSafeFallback` and all four existing `TestFormat*` tests pass.

- [ ] **Step 7: Verify the whole tree builds**

Run: `go build ./...`
Expected: no output (success) — both callers compile against the new signature.

- [ ] **Step 8: Commit**

```bash
git add internal/notify/formatters.go internal/notify/worker.go internal/api/events.go internal/notify/formatters_test.go
git commit -m "feat(notify): surface and log payload decode errors with safe fallback"
```

---

## Task 3: Widen `EmitParams.Payload` to `any`

Tiny, isolated type change so emit sites can pass typed structs. Backward compatible — `map[string]any` literals still satisfy `any`, so nothing else needs to change yet.

**Files:**
- Modify: `internal/notify/emit.go:21`

- [ ] **Step 1: Change the field type**

In `internal/notify/emit.go`, in the `EmitParams` struct, change:

```go
	Payload     map[string]any
```

to:

```go
	Payload     any
```

The existing nil guard still works (`if p.Payload == nil { p.Payload = map[string]any{} }`) — a nil `any` and a nil map both marshal to `{}`.

- [ ] **Step 2: Verify the whole tree still builds**

Run: `go build ./...`
Expected: no output (success) — all current `Payload: map[string]any{...}` call sites still satisfy `any`.

- [ ] **Step 3: Commit**

```bash
git add internal/notify/emit.go
git commit -m "refactor(notify): allow any typed value as EmitParams.Payload"
```

---

## Task 4: Migrate emit sites to typed payloads

Mechanical: replace every `Payload: map[string]any{...}` at an emit site with the matching typed struct from Task 1, and narrow `emitMaint`. No behavior change (the marshaled JSON is equivalent, except maintenance zero-valued numeric fields are now `omitempty` — see spec). Verified by build + a grep that no untyped emit payloads remain + the existing worker/scheduler tests.

**Files:**
- Modify: `internal/worker/tasks/sync.go` (5 sites: ~340, ~912, ~933, ~939, ~1000)
- Modify: `internal/worker/tasks/import_item.go` (2 sites: ~481, ~492)
- Modify: `internal/worker/tasks/export.go` (2 sites: ~148, ~167)
- Modify: `internal/worker/tasks/metadata_refresh.go` (2 sites: ~128, ~143)
- Modify: `internal/notify/prune.go` (2 sites: ~45, ~54)
- Modify: `internal/scheduler/backup_poll.go` (2 sites: ~47, ~55)
- Modify: `internal/scheduler/scheduler.go` (`emitMaint` definition + `maps` import)
- Modify: `internal/scheduler/orphaned_items.go` (2 sites: ~49, ~76)
- Modify: `internal/scheduler/stale_jobs.go` (3 sites: ~46, ~72, ~81)

- [ ] **Step 1: `sync.go` — convert the five `Payload` literals**

Replace each `Payload: map[string]any{...}` with the typed struct (keep the surrounding `notify.EmitParams{...}` fields and `DedupKey`):

```go
// sync failed
Payload: notify.SyncFailedPayload{Storefront: storefront, Error: msg, JobID: jobID},
// needs review
Payload: notify.SyncNeedsReviewPayload{Storefront: storefront, Count: pendingReviewCount, JobID: jobID},
// completed with errors
Payload: notify.SyncCompletedWithErrorsPayload{Storefront: storefront, Failed: failedCount, JobID: jobID},
// completed
Payload: notify.SyncCompletedPayload{Storefront: storefront, JobID: jobID},
// emitSyncDiff
Payload: notify.SyncDiffPayload{Added: added, Removed: removed, JobID: jobID},
```

For `emitSyncDiff` specifically, the local `added`/`removed` slices are currently
`[]map[string]any` built from `entry := map[string]any{"title": ..., "platforms": ...}`.
Convert them to `[]notify.DiffGame`. Replace the declarations and the loop body:

```go
	added := []notify.DiffGame{}
	removed := []notify.DiffGame{}
	for _, r := range rows {
		platforms := []string{}
		if r.Platforms != "" {
			platforms = strings.Split(r.Platforms, ",")
		}
		entry := notify.DiffGame{Title: r.Title, Platforms: platforms}
		if r.ChangeType == "added" {
			added = append(added, entry)
		} else {
			removed = append(removed, entry)
		}
	}
```

- [ ] **Step 2: `import_item.go` — convert the two `Payload` literals**

```go
// import failed
Payload: notify.ImportFailedPayload{JobID: jobID, Failed: failedCount, Error: fmt.Sprintf("%d item(s) failed to import", failedCount)},
// import completed
Payload: notify.ImportCompletedPayload{JobID: jobID},
```

- [ ] **Step 3: `export.go` — convert the two `Payload` literals**

```go
// export failed
Payload: notify.ExportFailedPayload{JobID: job.ID, Error: errMsg},
// export completed
Payload: notify.ExportCompletedPayload{JobID: job.ID, FilePath: filePath},
```

- [ ] **Step 4: `metadata_refresh.go` — convert the two `Payload` literals**

```go
// maintenance failed
Payload: notify.MaintPayload{Action: "metadata_refresh_dispatch", Error: err.Error()},
// maintenance completed
Payload: notify.MaintPayload{Action: "metadata_refresh_dispatch", Count: len(games)},
```

- [ ] **Step 5: `prune.go` — convert the two `Payload` literals**

This file is in package `notify`, so use the bare type names (no `notify.` prefix):

```go
// maintenance failed
Payload: MaintPayload{Action: "prune_events", Error: err.Error()},
// maintenance completed
Payload: MaintPayload{Action: "prune_events", Count: int(rows)},
```

Note: `rows` is an `int64` from `RowsAffected()`; convert to `int` to match `MaintPayload.Count`.

- [ ] **Step 6: `backup_poll.go` — convert the two `Payload` literals**

```go
// backup failed
Payload: notify.BackupFailedPayload{Error: err.Error()},
// backup completed
Payload: notify.BackupCompletedPayload{BackupID: id},
```

- [ ] **Step 7: Narrow `emitMaint` in `scheduler.go`**

Replace the `emitMaint` definition:

```go
// emitMaint emits an admin.maintenance.{completed,failed} event.
func emitMaint(ctx context.Context, db *bun.DB, failed bool, p notify.MaintPayload) {
	typ := notify.TypeAdminMaintCompleted
	if failed {
		typ = notify.TypeAdminMaintFailed
	}
	notify.Emit(ctx, db, notify.EmitParams{Type: typ, Scope: notify.ScopeAdmin, Payload: p})
}
```

Then remove `"maps"` from the import block (it was only used by the deleted `maps.Copy`).

- [ ] **Step 8: Update `emitMaint` callers in `orphaned_items.go`**

```go
// line ~49
emitMaint(ctx, db, true, notify.MaintPayload{Action: "rescue_orphaned_items", Error: err.Error()})
// line ~76
emitMaint(ctx, db, false, notify.MaintPayload{Action: "rescue_orphaned_items", Rescued: successCount, Failed: failureCount})
```

- [ ] **Step 9: Update `emitMaint` callers in `stale_jobs.go`**

```go
// lines ~46 and ~72 (both failed)
emitMaint(ctx, db, true, notify.MaintPayload{Action: "stale_jobs_cleanup", Error: err.Error()})
// line ~81 (completed)
emitMaint(ctx, db, false, notify.MaintPayload{Action: "stale_jobs_cleanup", Count: int(rows + syncRows)})
```

Note: if `rows`/`syncRows` are `int64`, convert the sum to `int` to match `MaintPayload.Count`; if already `int`, drop the conversion.

- [ ] **Step 10: Verify no untyped emit payloads remain**

Run: `grep -rn "Payload: *map\[string\]any" internal/ --include=*.go | grep -v _test`
Expected: no output. (The only legitimate remaining `map[string]any` is the nil-guard default inside `emit.go`, which is not an `EmitParams` literal — confirm none of the emit call sites match.)

- [ ] **Step 11: Verify the whole tree builds**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 12: Run the affected package tests**

Run: `go test ./internal/notify/... ./internal/scheduler/... ./internal/worker/... -timeout 600s`
Expected: PASS (these require the shared Postgres test container; allow time for startup).

- [ ] **Step 13: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/import_item.go internal/worker/tasks/export.go internal/worker/tasks/metadata_refresh.go internal/notify/prune.go internal/scheduler/backup_poll.go internal/scheduler/scheduler.go internal/scheduler/orphaned_items.go internal/scheduler/stale_jobs.go
git commit -m "refactor(notify): construct typed payloads at all emit sites"
```

---

## Task 5: Registry-driven round-trip golden test

The contract's CI net: every registered event type must have a sample payload and a `Format` case that produces a non-generic, exactly-specified `(title, body)` with no decode error. A new event type with no sample fails the test.

**Files:**
- Test: `internal/notify/formatters_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Add to `internal/notify/formatters_test.go`:

```go
// samplePayloads holds one representative, well-formed payload per registered
// event type. Adding a new event type to the registry without adding a sample
// here fails TestFormat_AllRegisteredTypesRoundTrip.
var samplePayloads = map[string]any{
	TypeSyncCompleted:           SyncCompletedPayload{Storefront: "Steam", JobID: "j1"},
	TypeSyncCompletedWithErrors: SyncCompletedWithErrorsPayload{Storefront: "Steam", Failed: 3, JobID: "j1"},
	TypeSyncFailed:              SyncFailedPayload{Storefront: "Steam", Error: "bad token", JobID: "j1"},
	TypeSyncNeedsReview:         SyncNeedsReviewPayload{Storefront: "Steam", Count: 2, JobID: "j1"},
	TypeSyncDiff:                SyncDiffPayload{Added: []DiffGame{{Title: "Hades", Platforms: []string{"Steam"}}}, JobID: "j1"},
	TypeImportCompleted:         ImportCompletedPayload{JobID: "j1"},
	TypeImportFailed:            ImportFailedPayload{JobID: "j1", Failed: 2, Error: "2 item(s) failed to import"},
	TypeExportCompleted:         ExportCompletedPayload{JobID: "j1", FilePath: "/tmp/export.zip"},
	TypeExportFailed:            ExportFailedPayload{JobID: "j1", Error: "disk full"},
	TypeAdminBackupCompleted:    BackupCompletedPayload{BackupID: "b1"},
	TypeAdminBackupFailed:       BackupFailedPayload{Error: "s3 unreachable"},
	TypeAdminMaintCompleted:     MaintPayload{Action: "prune_events", Count: 5},
	TypeAdminMaintFailed:        MaintPayload{Action: "prune_events", Error: "query failed"},
}

// wantRender is the exact (title, body) each sample must produce. Locks the
// user-facing copy; intentional wording changes update this map alongside
// formatters.go.
var wantRender = map[string]struct{ title, body string }{
	TypeSyncCompleted:           {"Sync completed", "Your Steam sync completed successfully."},
	TypeSyncCompletedWithErrors: {"Sync completed with errors", "Your Steam sync finished with 3 failed item(s)."},
	TypeSyncFailed:              {"Sync failed", "Your Steam sync failed: bad token"},
	TypeSyncNeedsReview:         {"Sync needs review", "Your Steam sync has 2 item(s) needing review."},
	TypeSyncDiff:                {"Game library changes", "Added (1):\n  + Hades [Steam]"},
	TypeImportCompleted:         {"Import completed", "Your import finished successfully."},
	TypeImportFailed:            {"Import failed", "Your import failed: 2 item(s) failed to import"},
	TypeExportCompleted:         {"Export completed", "Your export is ready."},
	TypeExportFailed:            {"Export failed", "Your export failed: disk full"},
	TypeAdminBackupCompleted:    {"Backup completed", "A scheduled backup completed successfully."},
	TypeAdminBackupFailed:       {"Backup failed", "A scheduled backup failed: s3 unreachable"},
	TypeAdminMaintCompleted:     {"Maintenance completed", "Maintenance task completed (prune_events)."},
	TypeAdminMaintFailed:        {"Maintenance failed", "Maintenance task failed (prune_events) - query failed."},
}

func TestFormat_AllRegisteredTypesRoundTrip(t *testing.T) {
	for _, et := range Registry() {
		t.Run(et.Type, func(t *testing.T) {
			sample, ok := samplePayloads[et.Type]
			if !ok {
				t.Fatalf("no sample payload for registered type %q — add one to samplePayloads", et.Type)
			}
			want, ok := wantRender[et.Type]
			if !ok {
				t.Fatalf("no expected render for registered type %q — add one to wantRender", et.Type)
			}
			raw, err := json.Marshal(sample)
			if err != nil {
				t.Fatalf("marshal sample: %v", err)
			}
			title, body, derr := Format(et.Type, raw)
			if derr != nil {
				t.Fatalf("decode error on well-formed payload: %v", derr)
			}
			if body == "An event occurred: "+et.Type {
				t.Fatalf("type %q fell through to the generic body", et.Type)
			}
			if title != want.title || body != want.body {
				t.Fatalf("render mismatch:\n got  title=%q body=%q\n want title=%q body=%q", title, body, want.title, want.body)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/notify/ -run TestFormat_AllRegisteredTypesRoundTrip -v`
Expected: PASS — one subtest per registered type, all green. If a `body`/`title` assertion fails, reconcile the `wantRender` string with the actual `formatters.go` output (the test, not the formatter, is what should match reality here — but verify the copy reads correctly).

- [ ] **Step 3: Run the full notify suite**

Run: `go test ./internal/notify/ -v`
Expected: PASS — all `TestFormat*` tests including the new round-trip and safe-fallback tests.

- [ ] **Step 4: Commit**

```bash
git add internal/notify/formatters_test.go
git commit -m "test(notify): registry-driven payload contract round-trip test"
```

---

## Final verification

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 2: Lint the changed packages**

Run: `golangci-lint run ./internal/notify/... ./internal/scheduler/... ./internal/worker/... ./internal/api/...`
Expected: no findings. Confirm no `//nolint:errcheck` remains in `formatters.go`:
`grep -n "nolint:errcheck" internal/notify/formatters.go` → no output.

- [ ] **Step 3: Full test suite**

Run: `go test ./... -timeout 600s`
Expected: PASS.

- [ ] **Step 4: Open the PR** (only when the user asks)

PR title (drives the squash commit / release-please): `feat(notify): typed payload contract with handled decode errors`

---

## Self-review notes

- **Spec coverage:** Layer 1 → Task 2. Layer 2 (typed structs) → Tasks 1, 3, 4. Layer 3 (registry test) → Task 5. Safe-fallback behavior → Task 2 Step 1/3. Logging at both callers → Task 2 Step 4. `emitMaint` narrowing + `maps` import drop → Task 4 Steps 7-9. All seven `//nolint` removals → Task 2 Step 3 (Format) + Final verification Step 2 (grep gate).
- **Type consistency:** struct names and field names in Tasks 4 and 5 match the definitions in Task 1 (`SyncCompletedWithErrorsPayload.Failed`, `MaintPayload.{Action,Error,Count,Rescued,Failed}`, `SyncDiffPayload.{Added,Removed}`, etc.). `Format`'s 3-value signature is consistent across Tasks 2, 4 (callers), and 5 (test).
- **Confirmed against current source:** `emitSyncDiff`'s `added`/`removed` are `[]map[string]any` today and are converted to `[]notify.DiffGame` in Task 4 Step 1 (explicit loop rewrite included). `RowsAffected()` returns `int64` in `prune.go` and `stale_jobs.go`, so the `int(...)` conversions in Task 4 Steps 5 and 9 are required.
- **Known unknowns to confirm during execution:** exact line numbers drift between this plan and the source — locate emit sites by `notify.EmitParams{` / `Payload:` rather than by line number.
