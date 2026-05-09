# IGDB Optional Credentials — Design Spec

## Scope

Make `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` optional so the app runs normally without them. Only IGDB-dependent endpoints are gated — the rest of the app (browsing the local collection, managing user games, auth, platforms, tags) works fine regardless of IGDB configuration.

This is a Phase 1 gap. The current implementation marks both env vars as `required`, crashing the binary on startup if they're absent.

### What changes

- `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` become optional (empty string default)
- A startup validation step checks presence **and** validity (Twitch token probe)
- IGDB-dependent handlers return `503 Service Unavailable` when IGDB is not configured
- `/health` reports `igdb_configured: true/false`

### What does NOT change

- No middleware gate — the app is never blocked
- No `/misconfigured` page — not needed since non-IGDB features work fine
- No `AppStateMisconfigured` — this is not a state machine concern

---

## Config Changes

In `internal/config/config.go`:

```go
// Before (crashes on startup if missing)
IGDBClientID     string `env:"IGDB_CLIENT_ID,required"`
IGDBClientSecret string `env:"IGDB_CLIENT_SECRET,required"`

// After (empty string default)
IGDBClientID     string `env:"IGDB_CLIENT_ID"`
IGDBClientSecret string `env:"IGDB_CLIENT_SECRET"`
```

The `config_test.go` tests for required env vars must be updated — `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` are no longer required for `config.Load()` to succeed.

---

## Startup Validation

### Location

In `main.go`, after config is loaded and before building the Echo server.

### Two-stage check

1. **Presence** — are `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` set and non-empty?
   - If either is missing: log a warning (`"IGDB credentials not configured — game search, import, and metadata features will be unavailable"`), set `igdbClient = nil`, skip stage 2.

2. **Validity** — if both are present, create the IGDB client and attempt a Twitch OAuth token fetch via `GetAccessToken(ctx)` with a 10-second timeout context.
   - **Success** — credentials are valid, `igdbClient` is ready. Log an info message: `"IGDB credentials validated successfully"`.
   - **Auth failure** (HTTP 400/401/403 from Twitch) — credentials are present but wrong. Log a warning with the specific error (e.g. `"IGDB credentials invalid: Twitch returned status 403 — disabling IGDB features"`), set `igdbClient = nil`.
   - **Network/transient error** (timeout, DNS failure) — do **not** treat as invalid. Log a WARN (`"IGDB credential probe failed (network/transient) — IGDB client kept"`) and keep the IGDB client — the per-request auth will surface the problem if it persists.

### Code sketch

```go
// in main.go, after config load
var igdbClient *igdb.Client

if cfg.IGDBClientID == "" || cfg.IGDBClientSecret == "" {
    slog.Warn("IGDB credentials not configured — game search, import, and metadata features will be unavailable")
} else {
    igdbClient = igdb.NewClient(cfg)
    validateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    _, err := igdbClient.ValidateCredentials(validateCtx)
    cancel()
    if err != nil {
        if igdb.IsAuthError(err) {
            slog.Warn("IGDB credentials are invalid — disabling IGDB features", "err", err)
            igdbClient = nil
        } else {
            slog.Warn("IGDB credential probe failed (network/transient) — IGDB client kept", "err", err)
        }
    }
}
```

Note: The IGDB client needs a public method to validate credentials and a way to distinguish auth errors from network errors. Options:
- Add `ValidateCredentials(ctx) (string, error)` on `Client` that delegates to `AuthManager.GetAccessToken(ctx)`
- Add `IsAuthError(err) bool` helper that checks if the error wraps `ErrTwitchAuth` with an HTTP status in the 4xx range

The implementation plan should decide the exact API.

---

## IGDB Endpoint Gating

### Which endpoints

All handlers that call the IGDB client:
- `POST /api/games/search/igdb` (`HandleSearchIGDB`)
- `GET /api/games/igdb/:igdb_id` (`HandleGetIGDBGame`)
- `POST /api/games/igdb-import` (`HandleImportFromIGDB`)
- Future: sync dispatch, metadata refresh (Phase 3+)

### How

The `GamesHandler` already holds an `igdb *igdb.Client` field. When `igdbClient` is `nil` (not configured), the handler methods that need it return early:

```go
func (h *GamesHandler) HandleSearchIGDB(c *echo.Context) error {
    if h.igdb == nil {
        return c.JSON(http.StatusServiceUnavailable, map[string]string{
            "error":  "IGDB is not configured",
            "detail": "Set IGDB_CLIENT_ID and IGDB_CLIENT_SECRET environment variables and restart.",
        })
    }
    // ... existing logic
}
```

This is a simple nil check at the top of each IGDB handler — no middleware needed.

### Non-IGDB endpoints unaffected

These work normally regardless of IGDB configuration:
- `GET /api/games` — lists games from the local DB
- `GET /api/games/:id` — gets a game from the local DB
- All user-games, platforms, tags, auth endpoints

---

## `/health` Update

Add `igdb_configured` to the health response:

```json
{"status": "ok", "igdb_configured": true}
```

When IGDB is not configured:

```json
{"status": "ok", "igdb_configured": false}
```

The health handler needs to know whether `igdbClient` is nil. Pass this as a boolean to the route registration.

---

## `api.New()` Signature

`igdbClient` is already a parameter. When it's `nil`, the games handler and health endpoint behave accordingly. No new parameters needed — the existing `igdbClient *igdb.Client` being nil is the signal.

---

## Testing

### Config tests (`config_test.go`)

- Verify `config.Load()` succeeds without `IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET` (both default to empty string)
- Update existing required-env-var test — remove IGDB vars from the "required" set

### Games handler tests (`games_test.go`)

- **IGDB not configured**: create `GamesHandler` with `igdb: nil`; verify `POST /api/games/search/igdb` returns 503 with the expected error body; same for `GET /api/games/igdb/:igdb_id` and `POST /api/games/igdb-import`
- **Non-IGDB endpoints still work**: verify `GET /api/games` and `GET /api/games/:id` work normally when `igdb` is nil

### Health endpoint test

- With `igdbClient != nil`: verify response includes `"igdb_configured": true`
- With `igdbClient == nil`: verify response includes `"igdb_configured": false`

### IGDB credential validation

Startup logic in `main.go` — not directly unit-testable. The validation itself is covered by existing `AuthManager` tests. The handler-level nil checks are covered by the games handler tests above.

---

## Checklist

- [ ] Make `IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET` optional in config struct
- [ ] Update config tests
- [ ] Add IGDB credential validation in `main.go` (presence check + Twitch token probe)
- [ ] Add `ValidateCredentials` method (or equivalent) to IGDB client
- [ ] Add `IsAuthError` helper to distinguish auth failures from network errors
- [ ] Add nil-check guard to `HandleSearchIGDB`, `HandleGetIGDBGame`, `HandleImportFromIGDB`
- [ ] Update `/health` handler to include `igdb_configured` boolean
- [ ] Add games handler tests for IGDB-not-configured case
- [ ] Add health endpoint tests for `igdb_configured` field
- [ ] Verify all existing tests still pass
