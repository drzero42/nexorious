# CLI login: bootstrap API-key auth from username/password (issue #627)

## Summary

Add CLI commands that let a user authenticate against a Nexorious server with
their username and password, exchange those credentials for a named API key, and
store that key in a local config file. Subsequent CLI commands authenticate with
the stored key as a Bearer token — no credentials re-entered.

Three commands:

- `nexorious login` — prompt for server URL, username, password; mint an API key; save it.
- `nexorious logout` — revoke the stored key on the server; remove it from config.
- `nexorious whoami` — call the API with the stored key and print the authenticated user.

## Background

Issue #625 (closed) replaced JWT auth with server-side sessions + API keys:

- `POST /api/auth/login` — body `{username, password}`; on success sets a
  `session_id` HttpOnly cookie and returns the user object.
- `POST /api/auth/api-keys` — body `{name, scopes, expires_at?}`; auth via cookie
  **or** Bearer; returns `{id, name, scopes, key, created_at, expires_at}` where
  `key` is the raw `nxr_<hex>` value, shown exactly once.
- `DELETE /api/auth/api-keys/:id` — revoke; auth via cookie or Bearer.
- `POST /api/auth/logout` — deletes the session backing the request cookie.
- `GET /api/auth/me` — returns the authenticated user.

There is **no** dedicated "exchange password for API key" endpoint. The CLI
therefore reuses the browser session path: log in to obtain a short-lived
session cookie, use that cookie to mint an API key, then tear the session down.

The current CLI (`cmd/nexorious/`) only has server-side subcommands (`serve`,
`migrate`, `version`); there is no existing CLI→API HTTP client.

## Design decisions

| Question | Decision |
|---|---|
| Config location/format | `$XDG_CONFIG_HOME/nexorious/config.yaml` (fallback `~/.config/nexorious/config.yaml`). YAML — already a dependency (`yaml.v3`), matches project convention. |
| Profiles | Single server now, but stored under a profile-ready schema (`current` + `profiles` map) so adding multi-profile later is not a breaking format change. No profile-switching commands yet (YAGNI). |
| Key naming | `cli@<hostname>`. |
| Rotation | On `login`, if the profile already holds a `key_id`, revoke that key server-side **before** minting the new one — no orphaned keys. |
| Credential UX | Interactive prompts for url/username/password; `--url` and `--username` flags override prompts; password read from `NEXORIOUS_PASSWORD` env or a hidden terminal prompt. **No `--password` flag** (avoids leaking into shell history / `ps`). |

## Components

### `internal/clicfg/` — config file load/save/path

Owns the on-disk config. No cobra, no HTTP.

Schema:

```yaml
current: default
profiles:
  default:
    url: http://localhost:8000
    username: alice
    key_name: cli@myhost
    key_id: 7f3c...        # server-side id, for revoke on logout/rotate
    key: nxr_abc123...     # raw bearer token
```

Go types:

```go
type Profile struct {
    URL      string `yaml:"url"`
    Username string `yaml:"username"`
    KeyName  string `yaml:"key_name"`
    KeyID    string `yaml:"key_id"`
    Key      string `yaml:"key"`
}

type Config struct {
    Current  string             `yaml:"current"`
    Profiles map[string]Profile `yaml:"profiles"`
}
```

API:

- `Path() (string, error)` — resolves `$XDG_CONFIG_HOME` then `os.UserHomeDir()/.config`, appends `nexorious/config.yaml`.
- `Load() (*Config, error)` — reads + unmarshals; a missing file returns an empty
  `Config` (with an initialized `Profiles` map), not an error.
- `Save(*Config) error` — creates the dir `0700`, writes the file `0600` (atomic:
  write temp + rename within the config dir).
- Helpers: `CurrentProfile()` / `SetProfile(name, Profile)` convenience on `*Config`,
  defaulting `Current` to `"default"` when empty.

### `internal/cliclient/` — thin API client

A `Client` holding a base URL and an `*http.Client`. No cobra, no config knowledge.
Each method maps to one endpoint and returns typed results or a wrapped error.
Non-2xx responses are decoded into a useful error (status + server message).

```go
type Client struct { baseURL string; hc *http.Client }
func New(baseURL string) *Client

// Login posts credentials and returns the raw session_id cookie *value*
// (read directly from the response, not via a cookie jar — so a Secure-flagged
// cookie over http://localhost is still usable for the follow-up calls).
func (c *Client) Login(username, password string) (sessionID string, err error)

func (c *Client) CreateAPIKey(sessionID, name string) (key, id string, err error)
func (c *Client) RevokeAPIKeyWithCookie(sessionID, keyID string) error // rotation
func (c *Client) RevokeAPIKeyWithBearer(key, keyID string) error       // logout
func (c *Client) Logout(sessionID string) error                        // drop throwaway session
func (c *Client) Me(key string) (username string, err error)           // whoami
```

The session cookie is attached to follow-up requests explicitly as a
`Cookie: session_id=<value>` header; Bearer calls set `Authorization: Bearer <key>`.

### `cmd/nexorious/login.go`, `logout.go`, `whoami.go` — cobra wiring

Glue: parse flags, run interactive prompts, call `cliclient`, persist via `clicfg`.

`login` flow:

1. Resolve `url` (flag → existing profile → prompt, default `http://localhost:8000`).
2. Resolve `username` (flag → existing profile → prompt).
3. Resolve password: `NEXORIOUS_PASSWORD` env, else hidden prompt via
   `golang.org/x/term` `ReadPassword` (falls back to plain read if stdin is not a TTY).
4. `Login` → session cookie.
5. If the saved profile already has a `key_id`, `RevokeAPIKeyWithCookie` it (best-effort: log a warning on failure, continue).
6. `CreateAPIKey(cookie, "cli@"+hostname)` → raw key + id.
7. `Logout(cookie)` (best-effort).
8. Save the profile (`0600`). Print success, the server url, the key name, and a masked key.

`logout` flow:

1. Load current profile; error if no stored key.
2. `RevokeAPIKeyWithBearer(key, key_id)` — failure (key already gone, server
   unreachable) is a **warning**, not fatal.
3. Clear the profile's credential fields (`key`, `key_id`, `key_name`) but keep
   `url` and `username` so a later `login` can pre-fill them; `Save`.
4. Print confirmation.

`whoami` flow: load current profile → `Me(key)` → print `username @ url`. This is
the first command that consumes the stored key, so it doubles as a bootstrap
smoke test. A 401 prints a clear "not logged in / key revoked" message.

## Dependencies

- New direct dependency: `golang.org/x/term` (hidden password input). Small,
  well-maintained `golang.org/x` package.
- `gopkg.in/yaml.v3` is promoted from indirect to direct.

## Testing

- **`clicfg`**: save→load round-trip; XDG vs `~/.config` fallback (temp
  `XDG_CONFIG_HOME` / `HOME`); file mode is `0600` and dir `0700`; missing file
  yields empty config, not error; atomic-write leaves no temp file behind.
- **`cliclient`**: each method against an `httptest.Server` — success paths plus
  error/401 paths; `Login` correctly extracts the `session_id` cookie value;
  follow-up calls send the cookie / bearer header the handler expects.
- **cmd**: light coverage of flag/prompt resolution precedence (flag > config >
  prompt) where it can be exercised without a live server. The end-to-end mint
  flow is covered by the `cliclient` tests; cobra glue stays thin.
- No new server routes → no `slumber.yaml` changes.

## Out of scope (YAGNI)

- Multi-profile switching commands (`--profile`, `profile use`, listing).
- API-key scope / expiry flags on `login` (always `write`, no expiry).
- A config-edit command.
- Mobile/MCP bootstrap (separate concerns).

## File mode rationale

The config file stores a live Bearer token. It is written `0600` and its
directory `0700` so other local users cannot read the credential.
