# `nexorious setup` CLI command — design

**Issue:** #728 — Add command to run admin setup
**Date:** 2026-06-02

## Goal

Add a `nexorious setup` cobra subcommand that drives the **existing**
`POST /api/auth/setup/admin` HTTP endpoint to create the first admin user,
as an alternative to the web setup wizard. The CLI is a thin client: the
server handler stays the single source of truth for validation (username ≥ 3,
password ≥ 8), bcrypt hashing, and the first-user-only idempotency guard
(serializable transaction).

This deliberately differs from `reset-password` (#727/#744), which connects
directly to Postgres. Setup must go over HTTP so the server's serializable
"users is empty" check remains authoritative.

## Command

```
nexorious setup [--username U] [--url URL] [--password-stdin] [--login]
```

- `--username` — optional; prompted interactively when stdin is a TTY.
  **Required** when `--password-stdin` is used.
- `--url` — server base URL. Defaults to `http://localhost:8000` (the same
  fixed default `login` uses — `defaultServerURL` in `cmd/nexorious/login.go`).
  No `PORT`/config derivation: KISS. If the server listens elsewhere, the
  operator passes `--url`.
- `--password-stdin` — read the password from stdin instead of prompting.
  The password is **never** accepted as a flag value.
- `--login` — after the admin is created, log in with the same credentials and
  store an API key in the CLI config, so subsequent commands are ready to use.
  See "Log in after setup" below.

Pending migrations are applied automatically (no flag) — see "Run migrations
first" below.

### Password source (explicit, modelled on `docker login`)

TTY detection is **not** used to silently slurp stdin. The source is explicit:

- `--password-stdin` set → read one line from stdin, trim, no confirmation
  prompt. `--username` is required (fail fast if absent).
- else, stdin is a TTY → hidden entry via `golang.org/x/term` + a confirmation
  prompt; mismatch is an error and **no** request is sent.
- else (no `--password-stdin`, non-TTY) → error:
  `no password: pass --password-stdin to read it from stdin, or run interactively`.

## Run migrations first (automatic)

Brings up a brand-new instance with one command: start the server (which sits
in `needs_migration`, redirecting the UI to `/migrate`), then `nexorious setup`
to migrate **and** create the admin.

There is **no `--migrate` flag**. `setup` only ever creates the *first* admin
(the server's setup endpoint 403s once an admin exists), so it is inherently a
fresh-instance command — and a fresh instance always has pending migrations.
Requiring an opt-in flag would mean every first run hits a "pass --migrate"
detour for zero safety benefit: there is nothing on an empty DB to protect.
So `setup` applies pending migrations itself. The one exception is a migration
that previously *failed* — see the state table below.

This is driven over HTTP, not DB-direct, and that is a hard constraint — not a
preference:

- The `migrate` subcommand (`runMigrate`) creates its **own** throwaway
  `migrate.NewMigrator(db)`, migrates, and exits. Correct for an
  init-container where no server is running.
- `setup` POSTs to a **running** server, which caches its migration state in
  memory. The server's background poller (`migrator.go`) only re-determines
  state when recovering from a DB outage — never in the steady
  "DB reachable, migrations pending" case. So a DB-direct migration would leave
  the running server stuck in `needs_migration`, and Gate 2 would redirect the
  subsequent `/api/auth/setup/admin` to `/migrate` indefinitely.

`POST /api/migrate/run` is **not** a separate migration mechanism: its handler
calls the identical `migrator.RunMigrations(ctx)` that `runMigrate` calls. The
only difference is that it runs on the **server's own** migrator, so the
server's state actually transitions to `ready`. That is the only way the
running server learns migrations are done.

### Migrate-first flow

After the `GET /health` preflight, branch on the reported `status`:

| `status` | behavior |
|---|---|
| `ok` / `ready` | proceed to admin setup |
| `needs_migration` | `POST /api/migrate/run`, then poll until `ready`, then proceed |
| `migrating` | poll until `ready` (the `run` POST returns 409, which is tolerated), then proceed |
| `migration_failed` | abort: `migrations previously failed: <detail> — resolve the underlying problem (check the server logs) before retrying` |
| `db_unavailable` | abort: `database is unavailable` |
| any other | abort: `server is not ready (status: <status>)` |

`migration_failed` is the **one state setup will not auto-resolve.** Re-running
a migration that already failed almost never fixes it — the failure is usually
a bad statement or data that conflicts with the schema change, and a retry just
hits the same error. So instead of silently re-running, `setup` surfaces the
failure detail (fetched from `GET /api/migrate/status`) and aborts, leaving the
operator to investigate. Once they have actually fixed the underlying problem,
re-running `setup` (or `nexorious migrate`) is their deliberate next step.

When migrating:
1. `POST /api/migrate/run`. Treat `202 {"status":"migration started"}` as
   success; treat `400 {"error":"already up to date"}` as "already migrated"
   (proceed); `409` "in progress" → fall through to polling.
2. Poll `GET /api/migrate/status` (e.g. every ~1s) printing progress, until
   `state == "ready"` → proceed; `state == "migration_failed"` → abort and
   surface the status `error`. A bounded timeout guards against hanging.
3. Proceed to credential resolution + admin setup.

(The migration SSE stream `/api/migrate/progress` is **not** consumed — polling
`/api/migrate/status` is simpler and sufficient for a CLI.)

## Log in after setup (`--login`)

An admin running `setup` from the CLI will usually want to run *other* CLI
commands next, which need a stored API key (`nexorious login`). `--login` folds
that step in: after a successful admin creation, it reuses the username and
password just supplied to run the existing login bootstrap (log in → mint key →
store in the CLI config), so the admin is ready to go in one command.

- **Opt-in, default off.** It writes to the local CLI config — a side effect —
  and `setup` is often run via `docker/kubectl exec` in an ephemeral container
  where a stored key is pointless. So it is only done when asked.
- **Only on a fresh `201`.** On `403` (admin exists) or any other non-success,
  `setup` aborts as usual and the login step never runs; the operator uses
  `nexorious login` directly. `setup --login` is *not* a setup-or-login hybrid.
- **Login-step failure is scoped.** The admin creation is irreversible and has
  already printed its success line. If the login step then fails (e.g. the
  config file can't be written), the command returns an error prefixed
  `admin created, but --login failed (run "nexorious login")` and exits
  non-zero — signaling the operator must **not** re-run `setup` (which would
  now `403`), only `nexorious login`.
- **No re-prompt.** The password is already in memory from either source
  (`--password-stdin` line or the interactive confirm), so `--login` needs no
  additional input.

The login bootstrap is the same logic `nexorious login` runs after it resolves
its url/username/password. That tail (revoke any previously stored key → mint a
fresh key → drop the bootstrap session → save the profile → print) is extracted
into a shared `loginAndStoreKey` helper, called by both `login` and
`setup --login` with the same `cliclient.Client` instance. No new `cliclient`
methods are needed — `Login`, `CreateAPIKey`, `Logout`, and
`RevokeAPIKeyWithCookie` already exist.

## Architecture

`setup` is a **pure HTTP client** — it never opens the database, so it does not
use `loadEnvAndConfig` / `openBunDB`. The automatic migrate step is likewise
HTTP-driven (see above), preserving this property.

### `internal/cliclient` additions

Two new methods on the existing `Client`:

- `Health() (status string, err error)` — `GET /health`; decodes
  `{"status": "..."}`. Returns the `status` string (`"ok"` when ready,
  otherwise the app-state name).
- `SetupAdmin(username, password string) (*SetupResult, error)` —
  `POST /api/auth/setup/admin` with a JSON body `{username, password}`.

To observe redirects (gates 1–3 reply `302`), the client used for
`SetupAdmin` sets:

```go
CheckRedirect: func(*http.Request, []*http.Request) error {
    return http.ErrUseLastResponse
}
```

so a `302` is returned as a response rather than followed. The `201` body
(the me-response) and any session cookie are ignored — the command only needs
to know it succeeded.

`SetupResult` (or an equivalent typed return) carries enough to map the
outcome: the status code and, for a `302`, the `Location` header.

For the automatic migrate step, two more methods:

- `RunMigrations() error` — `POST /api/migrate/run`. `202` → success;
  `400 "already up to date"` → treated as success (nil); other codes →
  decoded error.
- `MigrationStatus() (state, detail string, err error)` — `GET /api/migrate/status`;
  decodes `{"state": "...", "error": "...", "pending_count": N}` and returns the
  `state` plus any failure `detail`.

### `cmd/nexorious/setup.go`

- `newSetupCmd()` registers flags and wires `RunE: runSetup`.
- `runSetup`:
  1. Resolve base URL: `--url` → else `defaultServerURL`.
  2. **Preflight** `client.Health()`. Connection error → abort
     "could not reach server". Then branch on `status`:
     - `db_unavailable` → abort.
     - `needs_migration` / `migrating` → run the migrate-first flow (above)
       and continue once `ready`.
     - `migration_failed` → abort, surfacing the status detail (do **not**
       retry).
     - `ready` → continue.
     All of this happens **before** reading any credentials.
  3. Resolve username (flag → TTY prompt; required under `--password-stdin`).
  4. Resolve password (per the rules above). Reuses the existing
     `promptSecret` helper from `reset_password.go` for the TTY path.
  5. `client.SetupAdmin(...)` and map the outcome (table below).
  6. If `--login` and step 5 succeeded (`201`): load the CLI config and call the
     shared `loginAndStoreKey` helper with the same `client`, `url`, `username`,
     and `password`. A failure here is wrapped with the `admin created, but
     --login failed` prefix (see "Log in after setup").
- Registered via `root.AddCommand(newSetupCmd())` in `main.go`.

## Outcome → output / exit-code mapping

| Result | Output (to stdout/err) | Exit |
|---|---|---|
| `201` | `Admin user "<username>" created.` | 0 |
| `403` | `setup already complete; an admin user already exists` | 1 |
| `400` | the server's validation message (e.g. password ≥ 8) | 1 |
| `302` → `Location: /migrate` | `migrations are pending — run "nexorious migrate" first` (defensive fallback; preflight normally migrates first) | 1 |
| `302` → `Location: /db-error` | `database is unavailable` | 1 |
| connection error | `could not reach server at <url> — is it running?` | 1 |

The preflight in step 2 normally catches the `302`/unreachable cases first; the
`302` rows are the defensive fallback if state changes between preflight and the
POST.

## Operational note

The server must already be running and reachable. It may still be in
`needs_migration` — `setup` migrates it first automatically. Intended
workflow is `docker exec` / `kubectl exec` into the running container (or
running alongside a live server on the host) — same pattern as
`reset-password`.

## Error handling

- Errors are returned from `RunE`; `main()` prints `error: <msg>` and exits 1
  (existing behaviour). Domain messages above are returned as plain errors.
- The `cliclient` methods wrap transport/decoder failures with context
  (`fmt.Errorf("...: %w", err)`), matching the existing `Login`/`CreateAPIKey`
  style and the `httpError` decoder for `{"message": ...}` bodies.

## Testing (httptest, all branches)

- **`cmd/nexorious/setup_test.go`** — table-driven against an
  `httptest.Server` stubbing `/health` and `/api/auth/setup/admin`:
  - `201` success (happy path)
  - `403` already-complete
  - `400` validation message surfaced
  - `302` → `/migrate`
  - `302` → `/db-error`
  - unhealthy preflight (`status != "ok"`) aborts before POST
  - connection refused (server closed) → reach-server error
  - `--password-stdin` piped path (no confirmation)
  - password-mismatch path errors without sending a request
  - missing `--username` under `--password-stdin` → error
  - auto-migrate happy path: `needs_migration` → run → poll → `ready` → `201`
  - auto-migrate where the run ends in `migration_failed` → abort (detail surfaced)
  - `migrating` preflight → poll until `ready` → `201` (no double run)
  - `migration_failed` preflight → abort surfacing the status detail, **without**
    re-running migrations (credentials never read)
  - `--login` happy path: `201` admin → login bootstrap → minted key stored in an
    isolated (`XDG_CONFIG_HOME`) config; both the admin-created and "Logged in"
    lines printed
  - `--login` where the login step fails after the `201`: admin-created line still
    printed, returned error carries the `admin created, but --login failed` prefix
  The existing `cmd/nexorious/setup_test.go` DB harness is **not** reused for
  this command (it's an HTTP client, not a DB writer).
- **`internal/cliclient/client_test.go`** — unit tests for `Health`,
  `SetupAdmin` (including that a `302` is returned as an observable response
  with its `Location`, redirect not followed), `RunMigrations` (202 success,
  400 "already up to date" → nil), and `MigrationStatus`.

## Supporting changes

- `DEV.md` — add a `setup` row to the CLI Subcommands table.
- `slumber.yaml` — the `create_admin` (`POST /api/auth/setup/admin`),
  `migration_run` (`POST /api/migrate/run`), and `migration_status`
  (`GET /api/migrate/status`) requests all already exist; no change. No new
  route is added.
- `login` — **unchanged.** It already defaults to `http://localhost:8000`;
  `setup` reuses that same constant, which is the entirety of the "align them"
  request.

## Out of scope

- No `PORT`/config-based URL derivation (KISS).
- No new HTTP route — the endpoint already exists.
- No changes to `reset-password`.
