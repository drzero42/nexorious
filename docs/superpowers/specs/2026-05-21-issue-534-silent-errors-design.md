# Stop silently swallowing errors (issue #534)

Date: 2026-05-21
Tracking issue: [#534](https://github.com/drzero42/nexorious/issues/534)
Milestone: 0.1.0

## Background

Issue #534 documented ~100 sites in the Go codebase where errors are discarded via `_ =` or `_, _ =` assignments. Standard `defer`-based cleanup is acceptable, but a significant number of these sites are genuine defects that mask crashes, auth failures, DB write failures, and silent job-queue drops.

The issue was filed against an older commit. A fresh audit on 2026-05-21 shows the original enumeration is stale: a handful of sites have already been fixed (e.g. `internal/services/igdb/igdb.go:419` now returns the error from `http.NewRequestWithContext`), and a larger number of new sites have been introduced by work landed since (epic sync, job-item retry endpoints, scheduler periodic jobs).

This spec replaces the issue's enumeration with a fresh inventory, defines a uniform classification scheme, and decomposes the work into five child issues so each tier can be reviewed and merged independently.

## Goals

1. Eliminate silent error discards in the categories Sev 1–4 defined below.
2. Make the remaining acceptable cases explicit and enforceable via lint.
3. Prevent regression by enabling `errcheck` in golangci-lint with a documented allowlist.
4. Keep PRs small enough to review without losing context — one severity tier per PR.

## Non-goals

- Refactoring the error-handling architecture (no new error types, no result-monad wrappers).
- Returning typed errors from packages that currently return plain `error`.
- Restructuring control flow beyond what is needed to surface the error correctly.

## Categorization criteria

Every `_ =` / `_, _ =` site falls into exactly one of these buckets. The criteria are applied uniformly during the re-audit.

| Severity | Criteria |
|---|---|
| **Sev 1** | Failure can cause a panic, hang, or persistent bad state with no recovery path. |
| **Sev 2** | Failure silently weakens auth/credential handling — empty token or zero-value credentials proceed into downstream calls. |
| **Sev 3** | State-mutating DB write or job-queue insert where failure means the system silently diverges from the operator's or user's intent. |
| **Sev 4** | SELECTs feeding response data, restore/cleanup operations, partial-data losses — failure produces a wrong but non-fatal result. |
| **Acceptable** | `defer Close/Rollback`, SSE/progress-stream `fmt.Fprintf`, fire-and-forget temp file cleanup, drain `io.Copy(io.Discard, body)`, hash-computation `io.Copy`, `riverClient.Stop()` on shutdown, intentionally optional `c.Bind` of body, cosmetic stats queries. |

## Fix conventions

The fix verb depends on the severity and the call-site context.

### Logging format

```go
slog.Error("<domain>: <action> failed", "err", err, "<context_key>", value)
```

Match the existing convention in `internal/api/sync.go:551,660,1071`. Always include `err` and at least one identifier (`job_id`, `job_item_id`, `user_id`).

### Handler vs worker default

- **API handlers** (`internal/api/*.go`): `if _, err := ...; err != nil { slog.Error(...); return echo.NewHTTPError(http.StatusInternalServerError, "<short user-facing message>") }`. The 500 response is what makes the failure visible to the user.
- **Background workers** (`internal/worker/tasks/*.go`, `internal/scheduler/*.go`): log-only. There is no caller to return to, so structured logs are the only signal.

### River insert exception

For River inserts on the critical path of a job (a dispatch worker enqueuing child items), log-only is not enough — the parent job sits "running" indefinitely if the insert fails. In those cases:

| Context | Treatment |
|---|---|
| Dispatch worker enqueuing child items | Log + mark the corresponding `job_items` row `status='failed'` with `last_error = "failed to enqueue: <err>"`. Auto-retry picks it up. |
| Periodic-job scheduler inserts | Log-only. Cron tick re-fires on the next interval; transient failures self-heal. |
| Handler-initiated retry inserts | Handler returns 500; user retries. |

### Transactional sequences

A subset of Sev 3 sites do `UPDATE x` then `INSERT INTO river_job`. If the insert fails after the update committed, state is inconsistent. Where this matters (the main dispatch paths in `worker/tasks/sync.go`), wrap the pair in a Bun transaction *or* re-order so the insert happens before the update. The inventory marks these sites with `txn` so they get extra attention during PR 3.

## Inventory

The re-audit ran on commit `447e3daa` (2026-05-21). Acceptable patterns are listed for completeness so the errcheck allowlist (PR 5) can be verified against them.

### Sev 1 — crash / silent bad state

| Site | Current | Fix |
|---|---|---|
| `internal/migrate/handler.go:69,74` | Goroutine swallows `RunMigrations` and `InitNeedsSetup` errors; never transitions on failure | Add `AppStateMigrationFailed` to the FSM, log via `slog.Error`, transition to failed; surface via existing `/api/migrate/status` payload |

(The IGDB `http.NewRequestWithContext` site listed in the original issue is already fixed at `internal/services/igdb/igdb.go:428`; no action needed.)

### Sev 2 — silent auth / credential failures

| Site | Current | Fix |
|---|---|---|
| `internal/api/sync.go:589` (PSN) | `_ = json.Unmarshal([]byte(*row.StorefrontCredentials), &creds)` | Log + 500 ("stored credentials are corrupted") |
| `internal/api/sync.go:722` (Epic) | Same | Same |
| `internal/api/sync.go:1122` (GOG) | Same | Same |
| `internal/services/psn/client.go:368` | `accessToken, _ = psnClient.AccessToken()` | Change helper signature to `(string, error)`; propagate to caller |
| `internal/api/auth.go:227` | `_ = c.Bind(&req)` in auth handler | `if err := c.Bind(&req); err != nil { return echo.NewHTTPError(http.StatusBadRequest, "invalid request body") }` |

### Sev 3 — DB writes and River inserts

#### `internal/api/sync.go` (handler — default: log + 500)

| Line | Site | Notes |
|---|---|---|
| 393 | `riverClient.Insert(DispatchSync...)` | Handler retry; 500 |
| 491 | `db.NewInsert().Model(row)` | 500 |
| 504 | `db.NewRaw(...).Exec` | 500 |
| 605 | `db.NewRaw(...).Exec` | 500 |
| 678 | `db.NewRaw(...).Exec` (Epic clear creds) | 500 |
| 768 | `db.NewRaw(...).Exec` | 500 |
| 791 | `db.NewRaw(...).Exec` | 500 |
| 810 | `db.NewRaw(...).Exec` | 500 |
| 816 | `riverClient.Insert(ProcessSyncItem...)` | 500 |
| 891 | `db.NewRaw(...).Exec` | 500 |
| 895 | `db.NewRaw(...).Exec` | 500 |
| 978 | `db.NewRaw(DELETE user_game_platforms).Exec` | 500 |
| 982 | `db.NewRaw(DELETE user_games).Exec` | 500 |
| 987 | `db.NewRaw(INSERT games).Exec` | 500 |
| 993 | `db.NewRaw(UPDATE external_games).Exec` | 500 |
| 1025 | `riverClient.Insert(ProcessSyncItem...)` | 500 |
| 1086 | `db.NewRaw(...).Exec` | 500 |
| 683 | `epicClient.Cleanup(ctx, userID)` | Log-only (best-effort cleanup; no user-facing failure) |

#### `internal/api/jobs.go` (handler — default: log + 500)

| Line | Site | Notes |
|---|---|---|
| 541 | `db.NewRaw(...).Exec` | 500 |
| 634 | `db.NewRaw(...).Exec` | 500 |

#### `internal/api/job_items.go` (handler — default: log + 500)

| Line | Site | Notes |
|---|---|---|
| 100, 105, 117, 123, 128, 195 | `db.NewRaw(...).Exec` (retry/dispatch state changes) | 500 |
| 111 | `db.NewSelect()...Scan` (siblings lookup) | 500 |

#### `internal/worker/tasks/sync.go` (worker — default: log only)

| Lines | Notes |
|---|---|
| 155, 198, 252, 259, 270, 375, 490, 585, 590, 599, 610, 693, 697, 716, 720, 764, 768, 809, 826, 855, 871, 877, 1018, 1036, 1046, 1068 | Log-only |
| 277 | `riverClient.Insert(ProcessSyncItem)` — dispatch critical path: log + mark `job_items.failed` |

Mark lines 270 + 277 and 716 + 720 with `txn`: each pair updates DB state and enqueues a River row; review whether the update should be deferred until after the insert succeeds.

#### `internal/worker/tasks/metadata_refresh.go` (worker)

| Line | Site | Notes |
|---|---|---|
| 136 | `riverClient.Insert(MetadataRefreshItem)` | Dispatch critical path: log + mark `job_items.failed` |
| 298 | `db.NewRaw(...).Exec` | Log-only |

#### `internal/scheduler/` (worker — default: log only)

| Site | Notes |
|---|---|
| `scheduler.go:185` (`riverClient.Insert DispatchSync`) | Log-only (cron self-heals) |
| `scheduler.go:317` (`db.NewRaw(...).Exec`) | Log-only |
| `backup_poll.go:52` (`db.NewRaw(...).Exec`) | Log-only |

### Sev 4 — medium concern / per-site judgement

| Site | Current | Fix |
|---|---|---|
| `internal/api/user_games.go:947` | `_ = h.db.NewSelect().Model(plat)...` (response data) | Log + 500 |
| `internal/api/user_games.go:1042` | Same | Log + 500 |
| `internal/api/import.go:131` | `_ = json.Unmarshal(raw, &gameFields)` per-record | Log at WARN with record id; skip the record; surface skip count in import job progress |
| `internal/backup/service.go:834` | `_ = RunPsqlCommand(conn, terminateCmd)` (restore) | Capture err; abort restore with wrapped error |
| `internal/backup/service.go:835` | `_ = RunPsqlCommand(conn, "DROP SCHEMA...; CREATE SCHEMA...")` (restore) | Capture err; abort restore with wrapped error |
| `internal/api/sync.go:968` | `_ = h.db.NewRaw(SELECT COUNT...).Scan(ctx, &otherCount)` — feeds orphan-action decision | Log + 500; the count drives a 409 response so silent failure here mis-routes the orphan logic |
| `internal/api/jobs.go:43` | `_ = h.db.NewRaw(...status counts).Scan(ctx, &counts)` (response data) | Log + 500 |

### Acceptable (no fix, included in errcheck allowlist or per-site `//nolint:errcheck` with comment)

| Site | Reason |
|---|---|
| `cmd/nexorious/serve.go:234` (`riverClient.Stop`) | Graceful-shutdown best-effort; nowhere to surface |
| `cmd/nexorious/version.go:16`, `migrate/handler.go:101,105`, `migrate/migrator.go:225` (`fmt.Fprint(f|ln)`) | Writing to stdout / SSE response — recovery is impossible |
| `internal/api/backup.go:308,381`, `internal/api/export.go:150`, `internal/backup/service.go:675,848,850` (`os.Remove`/`os.RemoveAll`/`copyDir` cleanup) | Best-effort temp-file cleanup |
| `internal/backup/service.go:97,98,99,102` (stats `QueryRowContext.Scan`) | Cosmetic stats; zero-value display is acceptable. Add comment in the fix PR. |
| `internal/backup/service.go:427` (`filepath.WalkDir`) | Callback signature returns err via parameter; the outer return is the walk's own status |
| `internal/backup/service.go:436` (`io.Copy(h, f)` for hashing) | Reclassify after review — failures should propagate. Investigate during PR 4; move to a fixed bucket if needed. |
| `internal/backup/service.go:677` (`_ = manifest`) | Unused-variable suppression, not error. Remove the variable if truly unused (PR 4). |
| `internal/services/igdb/igdb.go:475` (`io.Copy(io.Discard, body)`) | Drain to allow connection reuse |
| `internal/api/job_items.go:165` (`_ = c.Bind(&body) // optional body`) | Comment is explicit; keep but consider wrapping the comment in a `//nolint:errcheck` if errcheck flags it |
| `defer ...Close()` / `defer ...Rollback()` (numerous) | Standard cleanup; ignored by errcheck via exclusion |

## Child issues

Five child issues, filed against milestone 0.1.0, all linked from #534's body as a checklist. #534 stays open until all five close. Each child issue's body links back to the relevant section of this spec.

| # | Title (proposed) | Scope | Tests |
|---|---|---|---|
| **A** | `fix: surface migration failures via app state (issue #534 sev 1)` | `migrate/handler.go` + new `AppStateMigrationFailed` + middleware + status payload + UI surface | Migration-failure path; UI status |
| **B** | `fix: stop swallowing auth and credential errors (issue #534 sev 2)` | Three credential `Unmarshal` sites in `sync.go`, PSN access token, `auth.go:227` `c.Bind` | Auth `Bind` 400; credential 500 |
| **C** | `fix: log + surface DB and job-queue write failures (issue #534 sev 3)` | All Sev 3 rows in the inventory | One representative handler test (river insert failure → 500) |
| **D** | `fix: stop silently producing wrong data (issue #534 sev 4)` | `user_games.go` SELECTs, `import.go` per-record unmarshal, `backup/service.go` restore commands, `sync.go:968`, `jobs.go:43` | Import skip-record test; backup restore abort test |
| **E** | `chore: enable errcheck lint with documented allowlist (issue #534)` | Edit `.golangci.yml`; CI gate flips on; allowlist matches the "Acceptable" inventory rows | `golangci-lint run` reports zero `errcheck` findings |

Each child issue is independently mergeable. They must merge in the order **A → B → C → D → E**: PR E (lint) must run after the others land so it can validate zero findings.

## Testing strategy

| Tier | New tests |
|---|---|
| A | Force `RunMigrations` to error in a unit test of `HandleRun`; assert state transitions to `AppStateMigrationFailed` and status payload contains the error message |
| B | Auth handler with malformed JSON body → 400; sync handler with corrupted stored credentials → 500 |
| C | One representative test: sync handler where River insert fails returns 500. Most of PR C is mechanical observability; exhaustive testing of each site is not warranted under the project's testing policy |
| D | Import job with malformed game record skips it and records the skip count; backup restore where `DROP SCHEMA` fails aborts the restore with the wrapped error |
| E | None — CI failure is the test |

Mechanical sites (~80% of the diff) don't get tests. This matches the project's testing policy: write tests for security-sensitive logic, non-obvious invariants, and real-bug-found-and-fixed. Logging a previously-swallowed error meets none of those.

## Risks

- **Behavior change in handlers:** PRs B–D convert previously-200 responses into 400/500 on failure paths. Frontend code that doesn't expect those status codes may need a small update. Each PR's verification checklist should include manual smoke-testing of the touched flows.
- **Transactional inconsistency surfaced:** PR C may reveal pre-existing inconsistencies where the update half of an `UPDATE then INSERT` pair commits but the insert fails. Sites flagged `txn` in the inventory get extra review; if changing semantics is needed, scope it into PR C or split a follow-up.
- **errcheck false positives in PR E:** the allowlist is derived from this inventory. Anything errcheck flags after PRs A–D land that isn't in the allowlist means the inventory missed it. The fix is to either add it to the appropriate-severity PR before PR E lands, or — if it is genuinely acceptable — to add it to the allowlist with a justification comment in the lint config diff.

## Out of scope

- Adding context fields to existing `slog.Error` calls that aren't part of this audit.
- Adopting structured error types (e.g. `errors.New(... )` + sentinel checks) beyond what's needed for the specific fixes.
- Touching test files — `_ =` in tests is fine and not part of this audit.
