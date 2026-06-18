# `nexctl` Phase 4 — `sync` + `job` Command Groups Design (Epic #1060)

**Status:** Phase 4 of the `nexctl` CLI epic (#1060). Phases 1 (#1081), 2 (#1083), 3 (#1085), and the `game list --pool` follow-up (#1086) are merged. This phase adds the `sync` and `job` command groups in **one PR** (user-chosen scope).

**Builds on:** merged `internal/cliui` (`Prompt`, `ReadPassword`, `Confirm`, `EncodeJSON`, `FirstNonEmpty`), `internal/cliclient` (`doBearer`, `SearchIGDB`, `ImportIGDBGame`, `resolveProfile`/`flagBool` in `cmd/nexctl`), and the render/ref-resolution helpers.

## Problem

`nexctl` can manage the collection, pools, and tags but cannot drive the **storefront sync** subsystem (Steam/PSN/Epic/GOG/Humble) or inspect/operate the **job queue** that sync, import, export, and metadata-refresh run through. This phase makes both manageable from the terminal, including interactive **match review** for sync sources.

## Command surface (this phase)

```
sync  status [storefront]                     [--json|-q]   # no arg: all configs; arg: detailed status
      connect <storefront>                                   # PUT credentials (secret fields prompted or via flags)
      disconnect <storefront>                   [-y]
      config <storefront> [--frequency <freq>]  [--json]     # no flag: show; with flag: set
      run <storefront>                          [--json]     # trigger a sync; prints the job id
      review <storefront>                                    # interactive: walk needs-review external games
      resolve <storefront> <ext-ref> --igdb-id N [--orphan-action remove]
      skip <storefront> <ext-ref>               [-y]
      retry <storefront>                                     # retry this storefront's failed external games
      reset <storefront>                        [-y]         # delete all synced data for the storefront

job   list                                      [--json|-q]  # --status --type --source --limit --page
      show <id>                                 [--json]     # job + item-status breakdown
      retry <id>                                            # retry the job's failed items
      cancel <id>                               [-y]
```

Both groups register on the `nexctl` root.

## Scope decisions (settled with the user)

- **One PR for the whole phase** (sync + job together).
- **Match review is sync-external-games only.** Sync sources review/rematch via `/api/sync/:sf/external-games` + `/api/sync/external-games/:id/rematch` + `/api/sync/ignored/:id`. The generic **import** job-item review (`/api/job-items/:id/resolve|skip`) ships with the import group in **Phase 5**, where the producer side has context. Phase 4 does **not** add `job-items` resolve/skip.
- The epic's `sync setup` verb is folded into `connect` (credentials) + `config` (frequency); there is no separate `setup` command.

## Architecture

Pure REST client over the bearer key (`resolveProfile`). New `internal/cliclient` methods; `cmd/nexctl/{sync,job}*.go` orchestrate them. `nexctl` keeps importing only stdlib + cobra + `clicfg`/`cliclient`/`cliui`/`cliauth` (no server/DB packages) — the compile-time REST boundary verified in earlier phases.

### Storefront resolution (registry-driven, not hardcoded)

The valid storefront slug set is derived at runtime from `GET /api/sync/config` (the server seeds exactly one config row per supported storefront). Helper `resolveStorefront(c, key, arg)` lowercases the arg, checks it against the live config slugs, and errors with the valid list on a miss — no hardcoded slice of slugs. (Per-storefront credential *field names* below are inherently client-known; that is unavoidable and localized to the connect command.)

### Per-storefront connect credentials

`connect <storefront>` builds the right request body from a small in-command credential spec keyed by slug. Secret values are prompted with no echo (`cliui.ReadPassword`) when the flag is omitted and a TTY is present; flags allow non-interactive use.

| storefront | endpoint body fields | flags |
|---|---|---|
| `steam` | `steam_id`, `web_api_key` | `--steam-id`, `--api-key` (secret) |
| `playstation-store` | `npsso_token` | `--npsso` (secret) |
| `epic-games-store` | `auth_code` | `--auth-code` (secret) |
| `gog` | `auth_code` | `--auth-code` (secret) |
| `humble-bundle` | `session_cookie` | `--session-cookie` (secret) |

Connect responses differ per storefront, so the client decodes into `map[string]any` and the command prints the salient confirmation field if present (`steam_username`/`online_id`/`display_name`/`username`/`message`).

### New `cliclient` methods + types

**Sync config / status / trigger / connection:**
- `ListSyncConfigs(key) ([]SyncConfig, error)` — `GET /api/sync/config` (envelope `{configs,total}`).
- `GetSyncConfig(key, sf) (*SyncConfig, error)`, `UpdateSyncConfig(key, sf, frequency string) (*SyncConfig, error)` — `GET`/`PUT /api/sync/config/:sf`.
- `SyncStatus(key, sf) (*SyncStatus, error)` — `GET /api/sync/:sf/status`.
- `TriggerSync(key, sf) (*SyncTriggerResult, error)` — `POST /api/sync/:sf`.
- `ConnectStorefront(key, sf string, body map[string]string) (map[string]any, error)` — `PUT /api/sync/:sf/connection`.
- `DisconnectStorefront(key, sf string) error` — `DELETE /api/sync/:sf/connection`.
- `ResetSyncData(key, sf string) error` — `DELETE /api/sync/:sf/data`.

**Sync external games / review:**
- `ListExternalGames(key, sf string) ([]ExternalGame, error)` — `GET /api/sync/:sf/external-games`.
- `RematchExternalGame(key, id string, igdbID int, orphanAction string) error` — `POST /api/sync/external-games/:id/rematch` (204).
- `RetryFailedExternalGames(key, sf string) error` — `POST /api/sync/:sf/external-games/retry-failed` (204).
- `SkipExternalGame(key, id string) error` — `POST /api/sync/ignored/:id` (204).

**Jobs:**
- `ListJobs(key string, params url.Values) (*JobsPage, error)` — `GET /api/jobs`.
- `GetJob(key, id string) (*Job, error)` — `GET /api/jobs/:id`.
- `GetJobItems(key, id string, params url.Values) (*JobItemsPage, error)` — `GET /api/jobs/:id/items`.
- `RetryFailedJob(key, id string) (*RetryResult, error)` — `POST /api/jobs/:id/retry-failed`.
- `CancelJob(key, id string) error` — `POST /api/jobs/:id/cancel`.

**Types:** `SyncConfig{ID,Storefront,Frequency,LastSyncedAt *string,IsConfigured bool,...}`, `SyncStatus{Storefront,IsSyncing,LastSyncedAt *string,ActiveJobID *string,ExternalGameCount int}`, `SyncTriggerResult{Message,JobID,Storefront,Status}`, `ExternalGame{ID,Storefront,ExternalID,Title,ResolvedIgdbID *int,IgdbTitle *string,IsSkipped,SyncStatus,FailedJobItemID *string,Platforms []string,...}`, `Job{ID,JobType,Source,Status,Priority,TotalItems,ErrorMessage *string,CreatedAt,StartedAt/CompletedAt *string,IsTerminal bool,DurationSeconds *float64,Progress JobProgress}`, `JobProgress{Pending,Processing,Completed,PendingReview,Skipped,Failed,Total,Percent int}`, `JobsPage{Jobs []Job,Total,Page,PerPage,TotalPages int}`, `JobItem{...}` (subset for show), `JobItemsPage{Items []JobItem,Total,Page,PerPage,TotalPages int}`, `RetryResult{Success bool,Message string,RetriedCount int}`.

## Command behaviour

- **`sync status`** — no arg: table of every config (STOREFRONT / CONFIGURED / FREQUENCY / LAST-SYNCED); `--json` raw configs; `-q` bare storefront slugs. With a storefront arg: detailed status (is-syncing, active job id, last sync, external-game count) from `SyncStatus`.
- **`sync connect <sf>`** — resolve sf, gather the per-sf credentials (flags or no-echo prompts), `ConnectStorefront`, print the confirmation field. 400/401 from the server (bad/invalid credentials) surfaces verbatim.
- **`sync disconnect <sf>`** — confirm unless `-y` → `DisconnectStorefront`.
- **`sync config <sf>`** — no `--frequency`: show current config (`GetSyncConfig`). With `--frequency`: `UpdateSyncConfig`, print new value. `--json` for raw.
- **`sync run <sf>`** — `TriggerSync`; prints `started sync (job <id>)`; `--json` raw. 409 (already running) surfaces verbatim.
- **`sync review <sf>`** — interactive only (errors off-TTY with a hint to use `resolve`/`skip`). Lists external games with `sync_status == "needs_review"`. For each: print title; offer **[s]earch IGDB & pick** (reuse `SearchIGDB` by the source title → numbered candidates → `RematchExternalGame(igdb_id)`), **[k]skip** (`SkipExternalGame`), **[n]ext** (leave for later), **[q]uit**. On rematch of the game's last platform the server may demand `orphan_action`; if it 4xx's asking for it, re-issue with `orphan_action=remove` after a confirm.
- **`sync resolve <sf> <ext-ref> --igdb-id N`** — non-interactive `RematchExternalGame`; `--orphan-action remove` forwarded. `<ext-ref>` resolves by external-game id (UUID) or case-insensitive title via `ListExternalGames` (0/1/many → error/use/candidate-error; off-TTY never prompts).
- **`sync skip <sf> <ext-ref>`** — confirm unless `-y` → `SkipExternalGame`.
- **`sync retry <sf>`** — `RetryFailedExternalGames`; prints confirmation.
- **`sync reset <sf>`** — confirm unless `-y` (loud — deletes all synced data) → `ResetSyncData`.
- **`job list`** — table (ID / TYPE / SOURCE / STATUS / ITEMS / CREATED); filters `--status`/`--type`/`--source`, paging `--limit`(→`per_page`)/`--page`; `--json` raw page; `-q` bare ids.
- **`job show <id>`** — meta (type/source/status/priority/created/started/completed/duration/error) + the `progress` breakdown (pending/processing/completed/pending_review/skipped/failed/total/percent). `--json` raw job.
- **`job retry <id>`** — `RetryFailedJob`; prints retried count + message.
- **`job cancel <id>`** — confirm unless `-y` → `CancelJob`. 409 (already terminal) surfaces verbatim.

## Cross-cutting conventions (unchanged)

Human table/detail default; `--json`; `-q` bare ids/slugs. Confirms on destructive/irreversible ops (`disconnect`, `skip`, `reset`, `cancel`) unless `-y`/non-TTY. Secrets prompted with no echo, never accepted positionally. `url.PathEscape` on every id/slug path segment. Void client methods pass `nil` out to `doBearer`.

## Out of scope (later phases)

- Import job-item review (`/api/job-items/:id/resolve|skip`) → **Phase 5** (import/export).
- `import`/`export` (5), `backup`/`admin`/`config` notify (6), packaging (7), `mcp` (8, blocked on #518).
- Per-storefront connection-status GETs (`/api/sync/:sf/connection`) — `sync status` derives configured/last-synced from the config list + `:sf/status`, so the detailed connection GETs are not wired this phase.
- `sync ignored` listing / unskip, external-game pagination — not in the epic's surface; add later if needed.
- A non-interactive bulk `sync review` (the interactive walker + `resolve`/`skip` cover scripted use).
