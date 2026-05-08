# Misconfiguration Gate — Design Spec

## Scope

Detect missing or invalid IGDB credentials at startup and block access to the application until the issue is resolved. This is a Phase 1 gap — the spec calls for a misconfiguration gate but the current implementation marks `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` as `required`, crashing the binary on startup if they're absent.

### What changes

- `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` become optional (empty string default)
- A startup validation step checks presence **and** validity (Twitch token fetch)
- A new middleware gate redirects all routes to `/misconfigured` when misconfigurations exist
- `/misconfigured` renders an HTML page showing what's wrong and how to fix it
- `/health` reports misconfiguration status

### Out of scope

- Generic misconfiguration framework (only IGDB for now)
- Auto-refresh on the misconfigured page — missing env vars or invalid credentials require a restart to fix, so polling for recovery is pointless

---

## Startup Validation

### Location

In `main.go`, after config is loaded and before building the Echo server. The validation result (a `[]string` of human-readable problem descriptions) is passed to `api.New(...)`.

### Checks

Two-stage validation:

1. **Presence** — are `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` set and non-empty?
   - If either is missing: `"IGDB_CLIENT_ID is not set"` / `"IGDB_CLIENT_SECRET is not set"` added to the list. Skip stage 2.

2. **Validity** — if both are present, attempt a Twitch OAuth token fetch via `igdb.AuthManager.GetAccessToken(ctx)`.
   - The call uses a 10-second timeout context so it doesn't hang startup.
   - On error: `"IGDB credentials are invalid: <error detail>"` added to the list.
   - This catches bad client ID/secret, revoked credentials, and network issues. A network failure is treated as a misconfiguration — the user can restart once connectivity is restored.

### Code sketch

```go
// in main.go, after config load
var misconfigurations []string

if cfg.IGDBClientID == "" {
    misconfigurations = append(misconfigurations, "IGDB_CLIENT_ID is not set")
}
if cfg.IGDBClientSecret == "" {
    misconfigurations = append(misconfigurations, "IGDB_CLIENT_SECRET is not set")
}

if len(misconfigurations) == 0 {
    // Credentials present — verify they work
    validateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    igdbClient = igdb.NewClient(cfg)
    if _, err := igdbClient.Auth.GetAccessToken(validateCtx); err != nil {
        misconfigurations = append(misconfigurations,
            fmt.Sprintf("IGDB credentials are invalid: %v", err))
    }
}

if len(misconfigurations) > 0 {
    slog.Warn("misconfiguration detected — app will block until resolved",
        "issues", misconfigurations)
}
```

Note: `igdbClient.Auth` is currently unexported. The implementation will need to either export `Auth` or add a `ValidateCredentials(ctx) error` method on `Client`. The plan should decide which.

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

## Middleware Gate

### Position

Between Gate 1 (DB unavailable) and Gate 2 (migrations pending). Gate order:

1. DB unavailable → redirect to `/db-error`
2. **Misconfigured → redirect to `/misconfigured`** ← new
3. Migrations pending → redirect to `/migrate`
4. Setup required → redirect to `/setup`

### Behavior

The gate receives the `[]string` of misconfigurations (via closure or a holder struct). If the list is non-empty, all requests are redirected to `/misconfigured` except:

- `/misconfigured` (the page itself)
- `/health` (monitoring must always work)

Unlike the DB-error and migration gates, this gate is **static** — the misconfigurations list is computed once at startup and never changes. There is no recovery path without a restart.

### Code sketch

```go
// In api.New(), between Gate 1 and Gate 2
e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c *echo.Context) error {
        if len(misconfigurations) > 0 {
            path := c.Request().URL.Path
            if path == "/misconfigured" || path == "/health" {
                return next(c)
            }
            return c.Redirect(http.StatusFound, "/misconfigured")
        }
        return next(c)
    }
})
```

---

## `/misconfigured` Page

### Pattern

Same as the db-error page: an `html/template` file in `ui/misconfigured/`, parsed via `template.ParseFS`, with dynamic data rendered server-side.

### Files

- `ui/misconfigured/index.html` — Go `html/template` with `{{range .Misconfigurations}}` to render the problem list
- `ui/ui.go` — new embed: `//go:embed misconfigured` var `MisconfiguredBox embed.FS`

### Handler

```go
type MisconfiguredHandler struct {
    misconfigurations []string
    tmpl              *template.Template
}

func NewMisconfiguredHandler(misconfigurations []string) *MisconfiguredHandler {
    tmpl := template.Must(template.ParseFS(ui.MisconfiguredBox, "misconfigured/index.html"))
    return &MisconfiguredHandler{misconfigurations: misconfigurations, tmpl: tmpl}
}

func (h *MisconfiguredHandler) HandleMisconfigured(c *echo.Context) error {
    data := struct {
        Misconfigurations []string
    }{
        Misconfigurations: h.misconfigurations,
    }
    c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
    return h.tmpl.Execute(c.Response(), data)
}
```

### Page content

The HTML template displays:

- A heading: "Nexorious — Configuration Required"
- The list of detected problems (rendered from `{{range .Misconfigurations}}`)
- A "How to fix" section with instructions:
  1. Register at https://dev.twitch.tv/console
  2. Create an application to get a Client ID and Client Secret
  3. Set `IGDB_CLIENT_ID` and `IGDB_CLIENT_SECRET` environment variables
  4. Restart nexorious
- A note that IGDB credentials are required for game search, import, and metadata features

Styled inline (no external CSS), visually consistent with the db-error and migration pages. No auto-refresh — a restart is required to clear this state.

### Route registration

```go
// In registerRoutes(), before the migration routes
if len(misconfigurations) > 0 {
    mch := NewMisconfiguredHandler(misconfigurations)
    e.GET("/misconfigured", mch.HandleMisconfigured)
}
```

The route is only registered when misconfigurations exist.

---

## `/health` Update

When misconfigurations exist, the health endpoint reports them:

```json
{"status": "misconfigured", "misconfigurations": ["IGDB_CLIENT_ID is not set"]}
```

When everything is fine, unchanged: `{"status": "ok"}`.

The health handler needs access to the misconfigurations list. Pass it via the same mechanism as the middleware (closure or struct field in the route registration).

---

## `api.New()` Signature Change

`api.New` receives the misconfigurations list as a new parameter:

```go
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB,
    resolvedDatabaseURL string, igdbClient *igdb.Client,
    misconfigurations []string) *echo.Echo
```

This is passed through to `registerRoutes` for the gate middleware, the `/misconfigured` handler, and the `/health` handler.

---

## Testing

### Config tests (`config_test.go`)

- `TestLoad_Defaults` — verify `config.Load()` succeeds without `IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET` (both default to empty string)
- Update existing required-env-var test — remove IGDB vars from the "required" set

### Router integration tests (`router_test.go`)

- **Gate active**: pass `misconfigurations: []string{"IGDB_CLIENT_ID is not set"}` to `api.New()`; verify `GET /` redirects to `/misconfigured`; verify `GET /misconfigured` returns 200 with HTML containing the problem text; verify `GET /health` returns 200 (not redirected)
- **Gate inactive**: pass `misconfigurations: nil` to `api.New()`; verify `GET /` does not redirect to `/misconfigured`

### Health endpoint test

- With misconfigurations: verify response is `{"status": "misconfigured", "misconfigurations": [...]}`
- Without misconfigurations: verify response is `{"status": "ok"}` (existing test, ensure it still passes)

### IGDB credential validation (main.go)

This is startup logic in `main.go` and not easily unit-testable. The validation logic itself is covered by existing `AuthManager` tests (token fetch success/failure). No new tests needed for the validation step — the router tests cover the gate behavior given a misconfigurations list.

---

## Checklist

- [ ] Make `IGDB_CLIENT_ID` / `IGDB_CLIENT_SECRET` optional in config struct
- [ ] Update config tests
- [ ] Add IGDB credential validation in `main.go` (presence + Twitch token fetch)
- [ ] Add `misconfigurations []string` parameter to `api.New()` and `registerRoutes()`
- [ ] Add misconfiguration middleware gate (between Gate 1 and Gate 2)
- [ ] Create `ui/misconfigured/index.html` template
- [ ] Add `MisconfiguredBox embed.FS` to `ui/ui.go`
- [ ] Add `MisconfiguredHandler` with `HandleMisconfigured`
- [ ] Register `/misconfigured` route
- [ ] Update `/health` handler to report misconfigurations
- [ ] Add router integration tests for the gate
- [ ] Verify all existing tests still pass
