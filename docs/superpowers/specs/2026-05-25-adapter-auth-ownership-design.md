# Adapter Auth Ownership Design

**Date:** 2026-05-25
**Branch:** issue-608-normalise-external-games

## Background

After the unified `StorefrontAdapter` refactor landed (see `2026-05-24-sync-spec-compliance-design.md`),
two places still violate the spec's adapter responsibility rule:

> *Each adapter lives in its own `services/` package and is responsible for all authentication
> mechanics (token refresh, CLI state management, credential expiry detection).*

1. **EpicClientAdapter is in the `tasks` package.** It holds `*bun.DB` and `*crypto.Encrypter`
   references and directly reads/writes `user_sync_configs`. It should live in `services/epic`.

2. **GOG token refresh is in the factory.** `buildAdapterFactory` in `serve.go` calls
   `gogClient.RefreshToken`, stores the result back to the DB, and hands a pre-refreshed
   `accessToken` to `gog.Adapter`. The adapter owns no auth mechanics; they live outside it.

---

## Design

Both adapters are refactored to use a **callback pattern**: the factory decrypts the stored
credentials, builds a closure that re-encrypts and persists any updated credentials, and passes
that closure to the adapter constructor. The adapter calls the closure after any credential
lifecycle operation. This keeps `services/gog` and `services/epic` free of `bun.DB` and
`crypto.Encrypter` imports while satisfying the spec's responsibility assignment.

---

## Section 1 — Epic adapter moves to `services/epic`

### New file: `internal/services/epic/adapter.go`

```go
// clientInterface is the subset of *Client that Adapter depends on.
// Tests can inject a fake without invoking the real legendary subprocess.
type clientInterface interface {
    Configured() bool
    RestoreSnapshot(userID string, snapshot map[string]string) error
    GetLibrary(ctx context.Context, userID string, onBatch func([]ExternalGameEntry) error) error
    CaptureSnapshot(userID string) (map[string]string, error)
}

type Adapter struct {
    client     clientInterface
    userID     string
    snapshot   map[string]string
    onSnapshot func(map[string]string) error
}

// NewAdapter returns a storefrontadapter.Adapter for Epic Games Store.
// snapshot is the decrypted legendary state loaded from user_sync_configs.
// onSnapshot is called after CaptureSnapshot; the factory wires the DB write here.
func NewAdapter(
    client clientInterface,
    userID string,
    snapshot map[string]string,
    onSnapshot func(map[string]string) error,
) storefrontadapter.Adapter
```

`GetLibrary` is the current `EpicClientAdapter.GetLibrary` logic moved verbatim:
1. Check `Configured()` — return error (not `ErrCredentials`) if legendary is not set up
2. Restore snapshot to disk
3. Call `client.GetLibrary`, mapping `epic.ExternalGameEntry` → `storefrontadapter.ExternalGameEntry`
   with `Platforms: []string{"pc-windows"}` and `PlaytimeHours: 0`
4. After `GetLibrary` returns (regardless of error), call `CaptureSnapshot`; if a non-empty
   snapshot is returned, call `onSnapshot` with it — log errors but do not abort

### Changes to `internal/worker/tasks/sync.go`

- Remove `EpicClientAdapter` struct, its `GetLibrary` method, and the `epicSubprocessClient` interface
- Remove the `epicsvc` import

### Changes to `buildAdapterFactory` in `cmd/nexorious/serve.go`

The `epic` case changes from constructing an `EpicClientAdapter` to:

```go
case "epic":
    if cfg.StorefrontCredentials == nil {
        return nil, tasks.ErrCredentials
    }
    plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        return nil, fmt.Errorf("%w: epic decrypt failed", tasks.ErrCredentials)
    }
    var snapshot map[string]string
    if err := json.Unmarshal(plain, &snapshot); err != nil {
        return nil, tasks.ErrCredentials
    }
    onSnapshot := func(s map[string]string) error {
        newJSON, _ := json.Marshal(s)
        enc, err := encrypter.Encrypt(newJSON)
        if err != nil {
            return err
        }
        _, err = db.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now()
             WHERE user_id = ? AND storefront = 'epic'`,
            enc, cfg.UserID,
        ).Exec(context.Background())
        return err
    }
    return epicsvc.NewAdapter(epicClient, cfg.UserID, snapshot, onSnapshot), nil
```

### Test migration

`TestEpicClientAdapter_*` tests (currently in `tasks/sync_test.go`) move to a new
`internal/services/epic/adapter_test.go`. They no longer need a live database: the `onSnapshot`
callback is an in-memory function that records whether it was called and what it received.
The `fakeEpicSubprocessClient` type moves with them.

---

## Section 2 — GOG adapter owns token refresh

### Updated `internal/services/gog/adapter.go`

```go
type Adapter struct {
    client       *Client
    refreshToken string
    onNewTokens  func(accessToken, refreshToken string) error
}

// NewAdapter returns a storefrontadapter.Adapter for GOG.
// refreshToken is the OAuth2 refresh token loaded from user_sync_configs.
// onNewTokens is called after a successful token refresh; the factory wires the DB write here.
func NewAdapter(
    client *Client,
    refreshToken string,
    onNewTokens func(accessToken, refreshToken string) error,
) storefrontadapter.Adapter
```

`GetLibrary` logic:
1. Call `client.RefreshToken(ctx, a.refreshToken)`
   - If `errors.Is(err, ErrGOGAuthExpired)`: return `fmt.Errorf("%w: gog token refresh", storefrontadapter.ErrCredentials)`
   - On other errors: return the error as-is (transient — River will retry)
2. Call `a.onNewTokens(tok.AccessToken, tok.RefreshToken)` — log error but continue
3. Call `client.GetLibrary(ctx, tok.AccessToken, batchSize, onBatch)` (existing logic unchanged)

### Changes to `buildAdapterFactory` in `cmd/nexorious/serve.go`

The `gog` case changes from refreshing the token directly to building the callback:

```go
case "gog":
    if cfg.StorefrontCredentials == nil {
        return nil, tasks.ErrCredentials
    }
    plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        return nil, tasks.ErrCredentials
    }
    var creds struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        UserID       string `json:"user_id"`
        Username     string `json:"username"`
    }
    if err := json.Unmarshal(plain, &creds); err != nil {
        return nil, tasks.ErrCredentials
    }
    onNewTokens := func(accessToken, refreshToken string) error {
        creds.AccessToken = accessToken
        creds.RefreshToken = refreshToken
        newJSON, _ := json.Marshal(creds)
        enc, err := encrypter.Encrypt(newJSON)
        if err != nil {
            return err
        }
        _, err = db.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now()
             WHERE user_id = ? AND storefront = 'gog'`,
            enc, cfg.UserID,
        ).Exec(context.Background())
        return err
    }
    return gogsvc.NewAdapter(gogsvc.NewClient(), creds.RefreshToken, onNewTokens), nil
```

The token-refresh loop that previously existed in this case (lines 443–461) is removed entirely.

---

## Files touched

| File | Change |
|---|---|
| `internal/services/epic/adapter.go` | New — `epic.Adapter` + `clientInterface` |
| `internal/services/epic/adapter_test.go` | New — migrated `TestEpicClientAdapter_*` tests, no DB dependency |
| `internal/services/gog/adapter.go` | Update struct + `GetLibrary` to own token refresh |
| `internal/worker/tasks/sync.go` | Remove `EpicClientAdapter`, `epicSubprocessClient`, `epicsvc` import |
| `internal/worker/tasks/sync_test.go` | Remove `TestEpicClientAdapter_*` tests |
| `cmd/nexorious/serve.go` | Update `epic` and `gog` factory cases |

---

## Testing

- **Epic adapter tests** (`adapter_test.go`): cover `NotConfigured`, `NoSnapshotInDB`, `RestoresSnapshot`,
  `PersistsNewSnapshot`, `PersistsSnapshotEvenOnFetchError`, `SkipsPersistWhenEmpty` — all existing
  cases, rewritten to use an in-memory `onSnapshot` callback instead of `testDB`.
- **GOG adapter tests**: add cases for successful token refresh (asserts `onNewTokens` is called with
  the new tokens), token refresh failure returning `ErrCredentials`, and transient refresh failure
  propagating the error.
- No changes to `DispatchSyncWorker` tests — the adapter interface is unchanged.
