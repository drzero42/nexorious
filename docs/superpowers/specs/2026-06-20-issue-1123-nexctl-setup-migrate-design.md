# `nexctl setup` + `nexctl migrate` — design

Issue: [#1123](https://github.com/drzero42/nexorious/issues/1123) — *feat(nexctl): add a
setup command with full parity to the web setup UI (create-admin + backup restore)*.

## Problem

A fresh Nexorious instance can be bootstrapped two ways through the web UI: create the
first admin user, **or** restore from an existing backup (disaster recovery). Neither path
is fully reachable from the CLI:

- `nexorious setup` (server binary) creates the first admin and auto-runs pending
  migrations, with an optional `--login`. It has **no restore**.
- `nexctl` has **no setup command at all**. `nexctl backup restore` deliberately targets
  the authenticated `/api/admin/backups/*` endpoints, which require users to exist, so it
  cannot serve a pre-bootstrap restore.

So the headless/automated disaster-recovery restore is only achievable through the browser.

### Architectural observation (drives the scope)

`cmd/nexorious/setup.go` is a **pure REST/HTTP client** — it never touches the database. It
drives `/health`, `/api/auth/setup/admin`, `/api/migrate/run`, and `/api/migrate/status`
over HTTP via `internal/cliclient`, and reuses `internal/cliauth` for `--login`. The only
"database" mentions in the file are error-message strings. That is exactly `nexctl`'s job;
the command is misplaced in the server binary.

Contrast `nexorious migrate` (`cmd/nexorious/migrate.go`): it connects **directly** to
`DATABASE_URL` via Bun and runs the migrator against the DB. That is a legitimate
server-binary, initContainer-style command and **stays put**.

The one reason the issue originally proposed keeping `nexorious setup` in place is that the
container ships **only** `nexorious` (`Dockerfile` builds and copies `/app/nexorious`;
`nexctl` has no container image and is not in the NixOS module by default). `nexorious
setup` exists to be run via `docker exec` / `kubectl exec` into a fresh instance, where
`nexorious` is on hand but `nexctl` is not. We resolve this by **shipping `nexctl` in the
container image** so the in-container bootstrap path keeps working.

## Goals

1. Give `nexctl` a `setup` command group with full parity to the web setup UI:
   create-admin, list on-disk backups, restore from upload, restore from a named on-disk
   backup.
2. Give `nexctl` a `migrate` command (and `migrate status`) that drives the server's
   migration endpoints — the CLI equivalent of the web `/migrate` "Run migrations" button.
3. **Relocate** the REST-client `setup` command out of `nexorious` into `nexctl`, removing
   it from the server binary.
4. Ship `nexctl` in the container image so headless `kubectl exec … nexctl setup` works.

### Non-goals

- No change to `nexorious migrate` (DB-direct; stays in the server binary).
- No deprecation shim or alias for the removed `nexorious setup` — clean breaking removal
  (solo user; no external dependents).
- No NixOS-module change: bare-metal/systemd operators already add `nexctl` to
  `environment.systemPackages` opt-in; the container image is what the bootstrap path needs.

## Setup-zone & migration-zone endpoints

All pre-bootstrap, **unauthenticated**, gated by app state / "no users exist":

| Endpoint | Method | Body | cliclient |
|---|---|---|---|
| `/api/auth/setup/admin` | POST | `{username,password}` | `SetupAdmin` (exists) |
| `/api/auth/setup/backups` | GET | — | **new** `SetupListBackups` |
| `/api/auth/setup/restore` | POST | multipart `file` | **new** `SetupRestoreUpload` |
| `/api/auth/setup/restore/disk` | POST | `{filename}` | **new** `SetupRestoreFromDisk` |
| `/api/migrate/run` | POST | — | `RunMigrations` (exists) |
| `/api/migrate/status` | GET | — | `MigrationStatus` (exists) |
| `/health` | GET | — | `Health` (exists) |

## Command surface

```
nexctl setup admin [--url U] [--username U] [--password-stdin] [--login] [--profile P]
        # health preflight → auto-run + await pending migrations → POST /api/auth/setup/admin
        # interactive: prompt username + password (twice, no-echo); --password-stdin reads one line
        # --login: log in with the same credentials and store an API key in the profile

nexctl setup backups [--url U] [--json | -q]
        # GET /api/auth/setup/backups → table: FILENAME  SIZE  MODIFIED  RESTORABLE  REASON
        # --json: raw entries; -q: bare filenames

nexctl setup restore --file PATH [--url U] [-y]
        # multipart upload → POST /api/auth/setup/restore
nexctl setup restore <name>      [--url U] [-y]
        # POST /api/auth/setup/restore/disk {"filename": name}
        # exactly one of <name> or --file (mirrors `backup restore [<id>] | --file`)
        # loud destructive confirm, bypassable with -y

nexctl migrate [--url U]
        # POST /api/migrate/run, then poll /api/migrate/status to ready/failed (timeout)
        # prints "No pending migrations." when already ready

nexctl migrate status [--url U] [--json]
        # GET /api/migrate/status → state / pending_count / detail
```

### URL & profile resolution

Setup and migrate commands are **unauthenticated**, so they must not require a stored API
key (unlike `resolveProfile`). A shared helper resolves the target URL:

```
resolveServerURL(cmd) =
    --url flag           (if set)
    else current profile's stored URL   (if a profile exists; no key needed)
    else cliauth.DefaultServerURL
```

`setup admin --login` stores the minted API key into the profile named by `--profile` (or
the current profile), reusing `cliauth.LoginAndStoreKey`.

### Confirmations

- `setup restore` (both forms): loud destructive confirm consistent with `backup restore`
  ("restore will overwrite the database…"), bypassable with the persistent `-y`.
- `setup admin` and `migrate`: no destructive confirm (creation / forward-only migration).

## Shared-code extraction

The orchestration currently private to `cmd/nexorious/setup.go` moves to shared `internal/`
packages so both binaries (during the transition) and both new `nexctl` commands consume
one implementation and cannot drift:

- **Into `internal/cliauth`** (already owns `LoginAndStoreKey` + `DefaultServerURL`, already
  shared by both binaries): the migrate/setup orchestration — `preflight` (health → branch
  on state), `runMigrateAndWait` (POST run + poll status with timeout), `migrationFailedErr`,
  and the `SetupResult` → user-message/error mapping (`reportSetupResult`). Exported with
  clear names; the polling interval/timeout become package constants or parameters.
- **Password prompting via `internal/cliui`** (`ReadPassword`, already used by `nexctl
  account login`): interactive double-prompt-with-confirm and the `--password-stdin`
  single-line read. The pure match/mismatch confirm logic stays unit-testable without a TTY.

`cmd/nexorious` no longer references this orchestration after `setup` is removed. `nexorious
migrate` is untouched (it does not use these helpers).

## cliclient additions

Three new methods on `*cliclient.Client`, all **unauthenticated**:

```go
// GET /api/auth/setup/backups
func (c *Client) SetupListBackups() ([]SetupBackupEntry, error)

// POST /api/auth/setup/restore  (multipart field "file", streamed)
func (c *Client) SetupRestoreUpload(filename string, body io.Reader) error

// POST /api/auth/setup/restore/disk  {"filename": ...}
func (c *Client) SetupRestoreFromDisk(filename string) error
```

`SetupBackupEntry` mirrors the server DTO (`HandleSetupListBackups`): `filename`,
`size_bytes`, `mtime`, `restorable`, `reason`, and an optional `manifest`
(`created_at`, `app_version`, `migration_version`, `backup_type`, `stats{users,games,tags}`).

To avoid sending a bogus `Authorization: Bearer ` header to the no-auth endpoints,
`doBearer` and `doBearerMultipart` are adjusted to **omit the `Authorization` header when
`key == ""`**. The new setup methods pass `key=""`. (`SetupAdmin`/`Health`/`RunMigrations`/
`MigrationStatus` already hand-craft requests with no auth header and are unchanged.)

## Container packaging

The released image (`runtime-ci` target) draws prebuilt per-arch binaries from the
`binaries` named context (`./ci-binaries`), which **already** contains `nexctl-linux-${arch}`
(`release-artifacts.yaml` builds it). The source-build image (`runtime` target) compiles
from source.

- `Dockerfile` `go-build` stage: add `go build … -o /out/nexctl ./cmd/nexctl`.
- `Dockerfile` `runtime` target: `COPY --from=go-build … /out/nexctl /usr/local/bin/nexctl`.
- `Dockerfile` `runtime-ci` target: `COPY --from=binaries …chmod=0755
  nexctl-linux-${TARGETARCH} /usr/local/bin/nexctl`.
- Placing `nexctl` on `PATH` (`/usr/local/bin`) lets `kubectl exec … nexctl setup …` work
  without an absolute path. The `nexorious` entrypoint/CMD is unchanged.
- **No change** to `release-artifacts.yaml` (already builds nexctl per-arch and uploads the
  `ci-binaries` artifact).

## Removal & docs

- Delete `cmd/nexorious/setup.go` and `cmd/nexorious/setup_cmd_test.go`; remove
  `newSetupCmd()` from `cmd/nexorious/main.go`. Run `make deadcode` to reconcile any newly
  orphaned exported symbols against the diff (`promptSecret` stays — `reset_password.go`
  still uses it).
- `docs/admin-guide.md` (lines ~252 and ~358): repoint bootstrap instructions from
  `nexorious setup` to `nexctl setup` (and mention `nexctl` is available in the container).
- `docs/user-guide.md` nexctl section: document `setup` (admin/backups/restore) and
  `migrate`.
- `CLAUDE.md`: update the `cmd/nexorious/`, `cmd/nexctl/`, and `internal/cliauth/` bullets
  and the two "setup-zone restore … belongs to `nexorious setup`" notes to reflect the new
  `nexctl setup` home.

## Testing

- **cliclient** (`httptest`): `SetupListBackups` (decode entries incl. manifest),
  `SetupRestoreUpload` (multipart field + streaming), `SetupRestoreFromDisk` (JSON body);
  for each, the success path, an error path (e.g. 403 "users exist", 503 "psql unavailable",
  400 incompatible), and an assertion that **no `Authorization` header is sent**.
- **cmd/nexctl** (`httptest` stub, porting the patterns from the existing
  `cmd/nexorious/setup_cmd_test.go`): `setup admin` success / already-setup 403 / migrate
  redirect / `--password-stdin` / `--login`; `setup backups` table + `--json` + `-q`; `setup
  restore` upload vs disk mutual-exclusion, confirm vs `-y`; `migrate` run (202/400/409
  tolerated → ready) and `migrate status`.
- **cliauth**: unit-test the extracted `runMigrateAndWait` state machine and the
  `reportSetupResult` mapping without a live server.

## Risks / notes

- **Breaking removal of `nexorious setup`.** Scripts calling `nexorious setup` must switch to
  `nexctl setup admin`. Acceptable per the project's no-back-compat stance; flagged in the
  changelog via the `feat:`/`feat!` commit and the docs update. (This is a `feat`, not a
  `feat!` — the *capability* is preserved, only the binary that hosts it changes; the docs
  update is the migration note.)
- The shared `internal/cliauth` orchestration must keep the same migration-failed handling
  (surface the failure detail, do not silently retry a previously-failed migration).
