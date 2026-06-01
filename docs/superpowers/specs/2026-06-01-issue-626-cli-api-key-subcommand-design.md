# Issue #626 — CLI `api-key` subcommand

**Date:** 2026-06-01
**Issue:** #626 — CLI: `nexorious api-key` subcommand for managing API keys
**Depends on:** #625 (server-side API key endpoints — merged), #723 (CLI login/logout/whoami + `cliclient`/`clicfg` — merged)

## Summary

Add a Cobra parent command `api-key` (with a `keys` alias) and three subcommands —
`generate`, `list`, `revoke` — that let a user manage their own API keys from the
command line by talking to a running Nexorious server. The commands authenticate
with the API key already stored in the CLI config by `nexorious login` (sent as
`Authorization: Bearer <key>`), so this is a pure-CLI change: no server or database
changes.

```
nexorious api-key generate --name <label> [--scopes read|write] [--expires-at <RFC3339>]
nexorious api-key list [--json]
nexorious api-key revoke <id-or-name> [--yes]
nexorious keys ...                       # alias for `api-key`
```

## Context

The server endpoints already exist (added with #625, exercised by #723's login bootstrap):

- `GET    /api/auth/api-keys` — list the user's non-revoked keys. Returns an array of
  `{id, name, scopes, last_used_at, created_at, expires_at}`; never returns the raw key.
- `POST   /api/auth/api-keys` — body `{name, scopes?, expires_at?}`; mints a key and
  returns `{id, name, scopes, key, created_at, expires_at}` with the raw `key` shown
  exactly once. `scopes` defaults to `write`, must be `read` or `write`. `expires_at`
  is optional RFC3339.
- `DELETE /api/auth/api-keys/:id` — revokes by id; `204` on success, `404` if not found.

API-key expiry is **enforced** at auth time, not merely stored: the lookup in
`internal/auth/session.go` filters `AND (expires_at IS NULL OR expires_at > now())`,
so an expired key is rejected on the next request.

The CLI plumbing from #723 is reused:

- `internal/clicfg` — local config at `$XDG_CONFIG_HOME/nexorious/config.yaml`, holding a
  `Profile{URL, Username, KeyName, KeyID, Key}`. `Load()`, `Save()`, `CurrentProfile()`,
  `CurrentName()`, `SetProfile()`.
- `internal/cliclient` — thin HTTP client over `/api/auth/*` with the `httpError` helper,
  existing `RevokeAPIKeyWithBearer(key, keyID)`, `Me(key)`, etc.

The server-side `id` is a UUID (`uuid.NewString()`), not the name. Names are plain labels
and are **not** unique — the login bootstrap mints `cli@<hostname>` and re-login revokes
the previous row (leaving a revoked row with the same name behind). Name uniqueness is
therefore not enforced; the CLI resolves names to ids defensively (see `revoke`).

## Commands

### Shared behavior

Every subcommand:

1. `clicfg.Load()` → `CurrentProfile()`. If absent or `Key == ""`, return the error
   `not logged in (run \`nexorious login\` first)` (same wording/pattern as `whoami`).
2. Build `cliclient.New(profile.URL)` and authenticate with `Authorization: Bearer <Key>`.
3. Write output to `cmd.OutOrStdout()`; return errors (cobra prints them; root has
   `SilenceUsage`/`SilenceErrors`).

### `generate`

Flags: `--name` (required), `--scopes` (default `write`), `--expires-at` (optional).

- Validate `--scopes` client-side to `read`/`write` to fail fast (server re-validates).
- `--expires-at` is forwarded verbatim as the request's `expires_at`; the server validates
  RFC3339 and returns `400` on a bad value, which surfaces via `httpError`.
- Before creating, fetch the list; if an **active** key with the same name already exists,
  print a non-fatal warning (`warning: an API key named %q already exists`) to
  `cmd.OutOrStdout()` but proceed — names are not unique.
- On success, print the raw key **exactly once** plus id, name, scopes, and expiry. The raw
  key is **never** written to config (that concern belongs to the login flow).

### `list`

Flag: `--json`.

- Default: an aligned table with columns `ID  NAME  SCOPES  CREATED  LAST USED  EXPIRES`.
  - Timestamps formatted human-readably (local time, e.g. `2006-01-02 15:04`).
  - `last_used_at` / `expires_at` null → `never` and `–` respectively.
  - Empty list prints `No API keys.`
- `--json`: emit the raw server JSON array (pretty-printed) for scripting; no table.
- ID is the first column because `revoke` may need it (ambiguous names fall back to id).

### `revoke <id-or-name>`

Flag: `--yes` / `-y` (skip the self-revoke confirmation prompt).

Resolution (Option A — CLI-side, no server change):

1. Fetch the list of active keys.
2. If the argument exactly matches a key `id`, target that key.
3. Otherwise match active keys by `name`:
   - exactly one → target it,
   - zero → error `no API key with id or name %q`,
   - more than one → error `multiple active keys named %q; revoke by id instead (see \`api-key list\`)`.
4. If the resolved id **equals the stored profile's `KeyID`** (the key this CLI is using):
   - Prompt `Revoke the key this CLI is currently using? This will log you out. [y/N] `
     unless `--yes` is set. A non-`y`/`yes` answer aborts with `aborted`.
   - Revoke via `RevokeAPIKeyWithBearer`.
   - Run the **logout cleanup**: clear `Key`/`KeyID`/`KeyName` on the profile and `Save()`.
     Do **not** issue a second server-side revoke (the key is already revoked; it would 404).
   - Print: `Revoked API key <id>.` then `That was the key this CLI was using — you have
     been logged out of <url>.`
5. Otherwise (any other key): revoke via `RevokeAPIKeyWithBearer`, print `Revoked API key <id>.`

## `cliclient` additions

Mirror the existing style (explicit `http.NewRequest`, `httpError`, `defer Body.Close()`):

- Exported struct:
  ```go
  type APIKey struct {
      ID         string     `json:"id"`
      Name       string     `json:"name"`
      Scopes     string     `json:"scopes"`
      Key        string     `json:"key,omitempty"` // only set on create
      LastUsedAt *time.Time `json:"last_used_at"`
      CreatedAt  time.Time  `json:"created_at"`
      ExpiresAt  *time.Time `json:"expires_at"`
  }
  ```
- `ListAPIKeys(key string) ([]APIKey, error)` — `GET /api/auth/api-keys` with Bearer auth.
- `CreateAPIKeyWithBearer(key, name, scopes string, expiresAt *string) (APIKey, error)` —
  `POST /api/auth/api-keys` with Bearer auth; omits `expires_at` from the body when nil.
  Returns the full response including the raw `Key`. (The existing `CreateAPIKey` uses a
  **session cookie** for the login bootstrap; this is its Bearer-authed sibling. Both are
  kept.)
- Revoke reuses the existing `RevokeAPIKeyWithBearer(key, keyID)` — no new method.

## Shared logout cleanup helper

`runLogout` currently clears `Key`/`KeyID`/`KeyName` and saves inline. Extract that
"clear stored key + save config" step into a small helper (e.g. `clearStoredKey(cfg *clicfg.Config) error`)
and call it from both `runLogout` and the `revoke` self-revoke path, so the logged-out
state is produced in exactly one place. `runLogout` keeps doing the server-side revoke
*before* calling the helper; the self-revoke path has already revoked, so it only calls
the helper.

## Wiring

- New file `cmd/nexorious/api_key.go`:
  - `newAPIKeyCmd()` returns the parent `&cobra.Command{Use: "api-key", Aliases: []string{"keys"}, ...}`
    with `generate`, `list`, `revoke` attached.
  - `runGenerate`, `runListKeys`, `runRevoke` functions.
- `cmd/nexorious/main.go`: `root.AddCommand(newAPIKeyCmd())`.

## Testing

Follow the `login_test.go` / `whoami_test.go` patterns: spin up an `httptest.Server`
that fakes the relevant `/api/auth/*` responses, point a temp `clicfg` at a temp
`XDG_CONFIG_HOME`, and drive commands via `newRootCmd().Execute()` / the `run*` functions
with captured output.

`cmd/nexorious/api_key_test.go`:

- not logged in → error for each subcommand
- `generate` happy path prints the raw key once; includes id/name/scopes/expiry
- `generate` invalid `--scopes` → client-side error, no request made
- `generate` duplicate active name → warning printed, key still created
- `generate --expires-at` forwards the value; server `400` surfaces
- `list` table output (columns, `never`/`–` for nulls, `No API keys.` when empty)
- `list --json` emits the raw array
- `revoke <id>` success
- `revoke <name>` resolves to the single active match
- `revoke <name>` ambiguous → error, no revoke
- `revoke <unknown>` → not-found error
- self-revoke (`id == stored KeyID`) with `--yes` → revokes + clears config + logged-out message
- self-revoke prompt declined → aborts, config unchanged
- server error (500/404) propagates via `httpError`

`internal/cliclient/client_test.go`:

- `ListAPIKeys` decodes the array (httptest)
- `CreateAPIKeyWithBearer` sends `Bearer` auth, omits `expires_at` when nil, returns raw key

## Out of scope

- No server or database changes (no migration, no endpoint changes, no enforced name
  uniqueness — that was considered and deferred; the CLI handles name collisions defensively).
- Slumber: the `api-keys` requests already exist in `slumber.yaml` from #723's bootstrap
  flow. The implementation will verify this and add only any missing `api-keys` requests
  (none expected). No new server routes are introduced.
- Multiple profiles / profile selection flags — `clicfg` supports multiple profiles in its
  schema, but these commands operate on the current profile only, as the other CLI commands do.
