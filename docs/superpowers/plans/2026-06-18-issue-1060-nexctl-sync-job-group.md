# `nexctl` Phase 4 — `sync` + `job` Command Groups Plan (Epic #1060)

Spec: docs/superpowers/specs/2026-06-18-issue-1060-nexctl-sync-job-group-design.md
Branch: `issue-1060-nexctl-sync-job-group`

Subagent-driven: each task is implemented (TDD), verified + committed by the controller, then independently reviewed (sonnet). A whole-branch review (opus) gates the merge. **Carry-over note from Phases 1–3:** background implementers often halt after ~10 tool calls mid-task — verify partial state, finish the mechanical remainder, then always run the independent review (the quality gate holds regardless of who types the code).

The cliclient layer (T1–T3) lands first so the command layer (T4–T9) compiles against real methods. Each task ends green (`go test ./...`), lint-clean, deadcode-clean.

---

### T1 — cliclient sync config/status/trigger/connection methods + types
- **Types:** `SyncConfig`, `SyncStatus`, `SyncTriggerResult`.
- **Methods:** `ListSyncConfigs`, `GetSyncConfig`, `UpdateSyncConfig`, `SyncStatus`, `TriggerSync`, `ConnectStorefront`, `DisconnectStorefront`, `ResetSyncData`.
- **Tests** (`internal/cliclient/sync_test.go`): httptest server asserts method+path+body; decode of the `{configs,total}` envelope; `ConnectStorefront` round-trips an arbitrary body and decodes `map[string]any`; `DisconnectStorefront`/`ResetSyncData` send DELETE and tolerate 204.
- Commit: `feat: add nexctl cliclient sync config/status/connection methods`

### T2 — cliclient sync external-games + review methods + `ExternalGame` type
- **Type:** `ExternalGame`.
- **Methods:** `ListExternalGames`, `RematchExternalGame` (body `{igdb_id, orphan_action}`, 204), `RetryFailedExternalGames` (204), `SkipExternalGame` (204).
- **Tests** (`internal/cliclient/sync_external_test.go`): path with `url.PathEscape(id)`; rematch body shape incl. omitted-vs-present `orphan_action`; 204 handling.
- Commit: `feat: add nexctl cliclient sync external-games methods`

### T3 — cliclient job methods + types
- **Types:** `Job`, `JobProgress`, `JobsPage`, `JobItem` (subset), `JobItemsPage`, `RetryResult`.
- **Methods:** `ListJobs(params)`, `GetJob`, `GetJobItems(params)`, `RetryFailedJob`, `CancelJob`.
- **Tests** (`internal/cliclient/jobs_test.go`): query passthrough; page envelope decode; `RetryFailedJob` decodes `{success,message,retried_count}`; `CancelJob` POST.
- Commit: `feat: add nexctl cliclient job methods`

### T4 — nexctl `sync` parent + `resolveStorefront` + `sync status`
- `newSyncCmd()` registers all sync subcommands. `resolveStorefront(c,key,arg)` validates against `ListSyncConfigs` slugs (registry-driven; errors with the valid list).
- `sync status`: no arg → config table / `--json` / `-q` slugs; arg → detailed `SyncStatus`.
- **Tests** (`cmd/nexctl/sync_test.go`): config-list table; detailed status; unknown storefront → friendly error listing valid slugs.
- Commit: `feat: add nexctl sync status command + storefront resolution`

### T5 — nexctl `sync connect` / `sync disconnect`
- Per-storefront credential spec (slug → fields/flags/secret). Secret values prompted no-echo when flag omitted + TTY; flags for non-interactive. `connect` PUTs and prints the confirmation field; `disconnect` confirms unless `-y`.
- **Tests** (`cmd/nexctl/sync_connect_test.go`): steam connect with `--steam-id`/`--api-key` sends correct body + prints username; psn `--npsso`; disconnect `-y` sends DELETE; off-TTY connect with missing secret flag errors (no hang).
- Commit: `feat: add nexctl sync connect/disconnect commands`

### T6 — nexctl `sync config` / `sync run` / `sync reset`
- `config <sf>`: show when no `--frequency`, else `UpdateSyncConfig`. `run <sf>`: `TriggerSync`, print job id (`--json` raw). `reset <sf>`: loud confirm unless `-y` → `ResetSyncData`.
- **Tests** (`cmd/nexctl/sync_run_test.go`): config show + set; run prints job id; reset `-y` sends DELETE; reset without `-y` off-TTY aborts.
- Commit: `feat: add nexctl sync config/run/reset commands`

### T7 — nexctl `sync review` / `sync resolve` / `sync skip` / `sync retry`
- `resolveExternalRef(cmd,c,key,sf,ref)` (UUID or case-insensitive title via `ListExternalGames`, TTY-pick/off-TTY candidate-error). `review`: interactive walker (search IGDB → pick → rematch / skip / next / quit; orphan-action confirm path). `resolve`: non-interactive rematch (`--igdb-id`, `--orphan-action`). `skip`: confirm unless `-y`. `retry`: `RetryFailedExternalGames`.
- **Tests** (`cmd/nexctl/sync_review_test.go`): resolve by id sends rematch body; resolve by title resolves then rematches; skip `-y`; retry posts; review errors off-TTY with a hint.
- Commit: `feat: add nexctl sync review/resolve/skip/retry commands`

### T8 — nexctl `job` parent + `job list` + `job show`
- `newJobCmd()`. `list`: filters `--status/--type/--source`, paging `--limit`(→per_page)/`--page`; table / `--json` / `-q` ids. `show <id>`: meta + progress breakdown / `--json`.
- **Tests** (`cmd/nexctl/job_test.go`): list table + query passthrough; `-q` ids; show renders progress.
- Commit: `feat: add nexctl job list/show commands`

### T9 — nexctl `job retry` + `job cancel`
- `retry <id>`: `RetryFailedJob`, print count+message. `cancel <id>`: confirm unless `-y` → `CancelJob`.
- **Tests** (`cmd/nexctl/job_mutate_test.go`): retry prints count; cancel `-y` posts; cancel without `-y` off-TTY aborts.
- Commit: `feat: add nexctl job retry/cancel commands`

### T10 — wiring, docs, deadcode
- Register `newSyncCmd()`/`newJobCmd()` on root in `main.go`; add `sync`/`job` to `main_test.go` want-map. Update `CLAUDE.md` `cmd/nexctl/` bullet. `make build` (both binaries); `make deadcode` reconcile; `tag/sync/job --help` list subcommands.
- Commit: `docs: nexctl sync/job groups (Phase 4 wiring + CLAUDE.md)`

---

## Verification gate (whole-branch, opus)

- Import boundary: `nexctl` imports only stdlib + cobra + `clicfg`/`cliclient`/`cliui`/`cliauth`.
- Client contracts match `internal/api/{sync,jobs,job_items}.go` (paths, methods, bodies, envelopes).
- Storefront set is registry-driven (no hardcoded slug slice); credential field map localized to connect.
- Secrets never accepted positionally / never echoed; off-TTY never hangs on a prompt.
- Destructive ops (`disconnect`/`skip`/`reset`/`cancel`) confirm unless `-y`; `reset` is loud.
- `sync review` is interactive-only and degrades to a clear off-TTY error pointing at `resolve`/`skip`.
- Full `go test ./...` green; lint clean; deadcode clean.
