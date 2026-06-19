# `nexctl` Phase 6 — `backup` + `admin` + `config` Command Groups Design (Epic #1060)

**Status:** Phase 6 of the `nexctl` CLI epic (#1060). Phases 1 (#1081), 2 (#1083), 3 (#1085), the `game list --pool` follow-up (#1086), 4 (#1087), and 5 (#1088) are merged. This phase adds the `backup`, `admin`, and `config` (incl. nested `config notify`) command groups in **one PR** (user-chosen scope).

**Builds on:** merged `internal/cliui` (`Prompt`, `ReadPassword`, `Confirm`, `EncodeJSON`), `internal/cliclient` (`doBearer`, `doBearerMultipart`, `DownloadExport`-style streaming), and the established `cmd/nexctl` idioms (`resolveProfile`, `flagBool`, `interactive`, tabwriter tables, persistent `-y`).

## Problem

`nexctl` can manage a user's collection, sync, jobs, and import/export, but cannot drive **operator** concerns (database backups, admin user management, the destructive data reset) or **per-user configuration** (deal region, notification delivery channels + event subscriptions). All of these are browser-only today. This phase closes that gap, completing the "browser-equivalent over the same REST API" goal for everything except `mcp` (Phase 8).

## Command surface (this phase)

```
backup  list                                                  [--json|-q]
        create                                                            # synchronous; prints backup id
        rm <id>                                               [-y]
        download <id> [--out FILE|-]
        restore <id>                                          [-y]        # destructive + async (confirm:true)
        restore --file <path>                                 [-y]        # destructive + async (multipart upload)
        schedule                                              [--json]    # show backup config
        schedule set [--frequency manual|daily|weekly] [--time HH:MM] [--day 0-6]
                     [--retention-mode days|count] [--retention-value N]

admin   user list                                             [--json|-q]
        user show <id>                                        [--json]
        user create <username> [--admin]                                  # password prompted no-echo (--password to script)
        user enable <id>                                                  # is_active=true
        user disable <id>                                     [-y]        # is_active=false (drops sessions)
        user set-admin <id> [--revoke]                                    # is_admin=true / false
        user passwd <id>                                                  # reset password, prompted no-echo
        user rm <id>                                          [-y]        # prints deletion-impact, then confirms
        reset                                                 [-y]        # obliterate ALL user data (loud)

config  get                                                   [--json]    # user settings (deal_region)
        set --deal-region <region>
        notify channel list                                   [--json|-q]
        notify channel create <name> [--url URL]                          # URL is secret; prompted no-echo if omitted
        notify channel edit <id> [--name N] [--url URL]
        notify channel rm <id>                                [-y]
        notify channel test <id>                                          # send a test notification
        notify test-url [--url URL]                                       # test an arbitrary Shoutrrr URL
        notify sub list                                       [--json|-q] # the user's subscribed event types
        notify sub set <event-type>...                                    # replace the subscription set
        notify sub reset                                                  # back to defaults
        notify events                                         [--json|-q] # available event types (id/scope/category/label)
```

`backup`, `admin`, `config` register on the `nexctl` root. `admin user` and `config notify`/`config notify channel`/`config notify sub` are nested subgroups.

## Scope decisions (settled with the user)

- **One PR for the whole phase** (backup + admin + config together).
- **Notifications nest under `config notify`** (not a top-level `notify` group). `config` is the single user-configuration umbrella: `config get/set` for settings, `config notify …` for delivery channels + subscriptions.
- **Setup-zone restore is out of scope.** `/api/auth/setup/{backups,restore,restore/disk}` only work when **no users exist** (pre-bootstrap), so an authenticated `nexctl` key can never use them — that flow belongs to `nexorious setup`. `nexctl` only wires the authenticated `/api/admin/backups/*` restore paths.
- **Admin commands require an admin user's key.** API keys do not carry admin scope; admin is derived from the key's owning user's `is_admin`. A non-admin key gets a 403, surfaced verbatim. No client-side admin gate.

## Architecture

Pure REST client over the bearer key (`resolveProfile`). New `internal/cliclient` methods grouped by area; `cmd/nexctl/{backup,admin,config}*.go` orchestrate them. `nexctl` keeps importing only stdlib + cobra + `internal/cli*` — the compile-time REST boundary verified in earlier phases. No server/DB packages; notification/backup model shapes are mirrored as local client structs.

### New `cliclient` methods + types

**Backup** (`internal/cliclient/backup.go`) — all admin-gated server-side:
- `GetBackupConfig(key) (*BackupConfig, error)` / `UpdateBackupConfig(key, BackupConfig) (*BackupConfig, error)` — `GET`/`PUT /api/admin/backups/config`.
- `ListBackups(key) ([]Backup, error)` — `GET /api/admin/backups` (envelope `{backups,total}`).
- `CreateBackup(key) (*CreateBackupResult, error)` — `POST /api/admin/backups` (synchronous; `{backup_id,message}`; 409 if in progress, 503 if pg_dump missing).
- `DeleteBackup(key, id) error` — `DELETE …/:id` (204).
- `DownloadBackup(key, id string, w io.Writer) error` — `GET …/:id/download` (streams tar.gz; no decode).
- `RestoreBackup(key, id string) error` — `POST …/:id/restore` body `{"confirm":true}` (destructive/async; `{success,message}`; 409/503).
- `RestoreBackupUpload(key, filename string, data []byte) error` — `POST …/restore/upload` multipart `file` (2 GB cap).
- Types: `BackupConfig{Schedule,ScheduleTime string,ScheduleDay int,RetentionMode string,RetentionValue int,UpdatedAt string}`, `Backup{ID,CreatedAt,BackupType string,SizeBytes int64,Stats struct{Users,Games,Tags int}}`, `CreateBackupResult{BackupID,Message string}`.

**Admin** (`internal/cliclient/admin.go`):
- `CreateUser(key, username, password string, isAdmin bool) (*AdminUser, error)` — `POST /api/auth/admin/users` (201).
- `ListUsers(key) ([]AdminUser, error)` — `GET …/users` (bare array, newest first).
- `GetUser(key, id) (*AdminUser, error)` — `GET …/users/:id`.
- `UpdateUser(key, id string, fields map[string]any) (*AdminUser, error)` — `PUT …/users/:id` (partial: username/is_active/is_admin; pointer/omit semantics via the map). Backs `enable`/`disable`/`set-admin`.
- `ResetUserPassword(key, id, newPassword string) error` — `PUT …/users/:id/password`.
- `GetDeletionImpact(key, id) (*DeletionImpact, error)` — `GET …/users/:id/deletion-impact`.
- `DeleteUser(key, id) error` — `DELETE …/users/:id` (200 + message).
- `AdminReset(key) (int, error)` — `POST /api/auth/admin/reset` → `{deleted}` count.
- Types: `AdminUser{ID,Username string,IsActive,IsAdmin bool,CreatedAt,UpdatedAt string}`, `DeletionImpact{UserID,Username string,TotalGames,TotalTags,TotalImportJobs,TotalExportJobs,TotalSyncJobs,TotalSyncConfigs,TotalSessions int,Warning string}`.

**Settings + notifications** (`internal/cliclient/config.go`):
- `GetSettings(key) (*Settings, error)` — `GET /api/settings`. `UpdateSettings(key string, fields map[string]any) (*Settings, error)` — `PATCH /api/settings` (422 on invalid region). Type `Settings{DealRegion string}`.
- `ListNotifyChannels(key) ([]NotifyChannel, error)` — `GET /api/notifications/channels` (no URL ever returned). `CreateNotifyChannel(key, name, url string) (*NotifyChannel, error)` — `POST …/channels` (201; 409 dup name). `UpdateNotifyChannel(key, id string, fields map[string]any) (*NotifyChannel, error)` — `PATCH …/channels/:id`. `DeleteNotifyChannel(key, id) error` — `DELETE` (204). `TestNotifyChannel(key, id) error` — `POST …/channels/:id/test` (204; 502 on send failure). `TestNotifyURL(key, url string) error` — `POST /api/notifications/test` (204; 400/502).
- `ListEventTypes(key) ([]EventType, error)` — `GET …/event-types`. `GetSubscriptions(key) ([]string, error)` — `GET …/subscriptions` (envelope `{event_types}`). `PutSubscriptions(key, types []string) ([]string, error)` — `PUT …/subscriptions` (400 on unknown/admin-scoped type). `ResetSubscriptions(key) ([]string, error)` — `POST …/subscriptions/reset`.
- Types: `NotifyChannel{ID,Name,CreatedAt string}` (deliberately no URL — encrypted at rest, never returned), `EventType{Type,Scope,Category,Label string,DefaultOn bool}`.

## Command behaviour (highlights)

- **`backup create`** — synchronous; prints `created backup <id>`. 409 ("already in progress") / 503 ("pg_dump unavailable") surface verbatim.
- **`backup download <id>`** — streams the tar.gz to `--out` (path / `-` stdout / default `backup-<id>.tar.gz`), mirroring `export`.
- **`backup restore <id>` / `restore --file <path>`** — **loud destructive confirm** (states it closes the DB, clears all sessions, and forces re-login) unless `-y`. The client always sends `{"confirm":true}`; the CLI gate is the user confirmation. 409/503 surface verbatim. (Exactly one of `<id>` or `--file` — error if both/neither.)
- **`backup schedule` / `schedule set`** — show vs update `BackupConfig`; only `--changed` flags are sent (the `Changed()` pattern); server validates time/day/retention.
- **`admin user create <username>`** — password via `--password` or no-echo prompt (`cliui.ReadPassword`), never positional. Prints the created user row.
- **`admin user disable/enable/set-admin`** — thin `UpdateUser` calls. `disable` confirms (drops the user's sessions). The server refuses self-deactivation / self-demotion (400) — surfaced verbatim.
- **`admin user rm <id>`** — first `GetDeletionImpact` and print the cascade counts + warning, then confirm (unless `-y`), then `DeleteUser`. Server refuses self-deletion (400).
- **`admin reset`** — the loudest confirm in the CLI (obliterates all user data, preserves admins + catalog); requires `-y` or an interactive yes. Prints `deleted <n> games`.
- **`config get` / `set --deal-region`** — show / PATCH user settings. 422 (invalid region) surfaces verbatim.
- **`config notify channel create <name>`** — URL is a secret (Shoutrrr tokens); `--url` or no-echo prompt. List/edit never display the URL (server never returns it). `channel test <id>` and `test-url` print success or the 502 error.
- **`config notify sub set <event-type>...`** — replace the subscription set; `sub reset` restores defaults; `notify events` lists valid types (so the user knows what to pass). Unknown/admin-scoped types 400 → surfaced.

## Cross-cutting conventions (unchanged)

Human table/detail default; `--json`; `-q` bare ids/slugs. Confirms on destructive ops (`backup rm`/`restore`, `user disable`/`rm`, `admin reset`, `config notify channel rm`) unless `-y`/non-TTY. Secrets (user passwords, notification URLs) prompted no-echo, never positional. `url.PathEscape` on every id path segment. Void client methods pass `nil` out. Multipart restore uses `doBearerMultipart`. Streaming download builds the request directly (no decode), like `DownloadExport`.

## Out of scope (later phases / API limits)

- **Setup-zone restore** (`/api/auth/setup/*`) — pre-bootstrap only; belongs to `nexorious setup`.
- **`admin user` rename** — the PUT supports `username`, but renaming is rare and omitted this phase (addable later cleanly; no back-compat concern).
- **Notification inbox/feed** — per the epic, the in-app notification feed does not fit the CLI; only delivery channels + subscriptions are wired.
- `mcp` (Phase 8, blocked on #518). Packaging of `nexctl` (Phase 7).
