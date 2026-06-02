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
nexorious setup [--username U] [--url URL] [--password-stdin]
```

- `--username` — optional; prompted interactively when stdin is a TTY.
  **Required** when `--password-stdin` is used.
- `--url` — server base URL. Defaults to `http://localhost:8000` (the same
  fixed default `login` uses — `defaultServerURL` in `cmd/nexorious/login.go`).
  No `PORT`/config derivation: KISS. If the server listens elsewhere, the
  operator passes `--url`.
- `--password-stdin` — read the password from stdin instead of prompting.
  The password is **never** accepted as a flag value.

### Password source (explicit, modelled on `docker login`)

TTY detection is **not** used to silently slurp stdin. The source is explicit:

- `--password-stdin` set → read one line from stdin, trim, no confirmation
  prompt. `--username` is required (fail fast if absent).
- else, stdin is a TTY → hidden entry via `golang.org/x/term` + a confirmation
  prompt; mismatch is an error and **no** request is sent.
- else (no `--password-stdin`, non-TTY) → error:
  `no password: pass --password-stdin to read it from stdin, or run interactively`.

## Architecture

`setup` is a **pure HTTP client** — it never opens the database, so it does not
use `loadEnvAndConfig` / `openBunDB`.

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

### `cmd/nexorious/setup.go`

- `newSetupCmd()` registers flags and wires `RunE: runSetup`.
- `runSetup`:
  1. Resolve base URL: `--url` → else `defaultServerURL`.
  2. **Preflight** `client.Health()`. Connection error → abort
     "could not reach server". `status != "ok"` → abort with the state,
     **before** reading any credentials.
  3. Resolve username (flag → TTY prompt; required under `--password-stdin`).
  4. Resolve password (per the rules above). Reuses the existing
     `promptSecret` helper from `reset_password.go` for the TTY path.
  5. `client.SetupAdmin(...)` and map the outcome (table below).
- Registered via `root.AddCommand(newSetupCmd())` in `main.go`.

## Outcome → output / exit-code mapping

| Result | Output (to stdout/err) | Exit |
|---|---|---|
| `201` | `Admin user "<username>" created.` | 0 |
| `403` | `setup already complete; an admin user already exists` | 1 |
| `400` | the server's validation message (e.g. password ≥ 8) | 1 |
| `302` → `Location: /migrate` | `migrations are pending — run "nexorious migrate" first` | 1 |
| `302` → `Location: /db-error` | `database is unavailable` | 1 |
| connection error | `could not reach server at <url> — is it running?` | 1 |

The preflight in step 2 normally catches the `302`/unreachable cases first; the
`302` rows are the defensive fallback if state changes between preflight and the
POST.

## Operational note

The server must already be running, migrated, and in `Ready` + `NeedsSetup`
state for the endpoint to be reachable. Intended workflow is
`docker exec` / `kubectl exec` into the running container (or running alongside
a live server on the host) — same pattern as `reset-password`.

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
  The existing `cmd/nexorious/setup_test.go` DB harness is **not** reused for
  this command (it's an HTTP client, not a DB writer).
- **`internal/cliclient/client_test.go`** — unit tests for `Health` and
  `SetupAdmin`, including that a `302` is returned as an observable response
  (redirect not followed) with its `Location`.

## Supporting changes

- `DEV.md` — add a `setup` row to the CLI Subcommands table.
- `slumber.yaml` — the `create_admin` request (`POST /api/auth/setup/admin`)
  already exists and is complete; no change. No new route is added.
- `login` — **unchanged.** It already defaults to `http://localhost:8000`;
  `setup` reuses that same constant, which is the entirety of the "align them"
  request.

## Out of scope

- No `PORT`/config-based URL derivation (KISS).
- No new HTTP route — the endpoint already exists.
- No changes to `reset-password`.
