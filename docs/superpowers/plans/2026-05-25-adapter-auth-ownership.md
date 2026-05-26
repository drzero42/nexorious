# Adapter Auth Ownership Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `EpicClientAdapter` out of the `tasks` package into `services/epic`, and move GOG OAuth token refresh inside `gog.Adapter`, so each storefront adapter owns its full auth lifecycle per `docs/sync.md`.

**Architecture:** Both adapters adopt a callback pattern — the factory decrypts credentials and builds a closure that re-encrypts and persists any updated state; the adapter calls the closure after auth operations. This keeps `services/epic` and `services/gog` free of `bun.DB` and `crypto.Encrypter` imports.

**Tech Stack:** Go, `uptrace/bun`, `github.com/drzero42/nexorious/internal/services/storefrontadapter`

**Spec:** `docs/superpowers/specs/2026-05-25-adapter-auth-ownership-design.md`

---

## Task 1: Create `epic.Adapter` in `services/epic`

**Files:**
- Create: `internal/services/epic/adapter.go`
- Create: `internal/services/epic/adapter_test.go`

---

- [ ] **Step 1: Write the failing tests**

Create `internal/services/epic/adapter_test.go`:

```go
package epic

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeEpicClient satisfies clientInterface without invoking legendary.
type fakeEpicClient struct {
	configured       bool
	restoreErr       error
	getLibraryErr    error
	captureSnapshot  map[string]string
	captureErr       error
	libraryBatches   [][]ExternalGameEntry

	restoreCalled    bool
	getLibraryCalled bool
	captureCalled    bool
	restoredSnapshot map[string]string
}

func (f *fakeEpicClient) Configured() bool { return f.configured }

func (f *fakeEpicClient) RestoreSnapshot(_ string, snapshot map[string]string) error {
	f.restoreCalled = true
	f.restoredSnapshot = snapshot
	return f.restoreErr
}

func (f *fakeEpicClient) GetLibrary(_ context.Context, _ string, onBatch func([]ExternalGameEntry) error) error {
	f.getLibraryCalled = true
	if f.getLibraryErr != nil {
		return f.getLibraryErr
	}
	for _, batch := range f.libraryBatches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeEpicClient) CaptureSnapshot(_ string) (map[string]string, error) {
	f.captureCalled = true
	return f.captureSnapshot, f.captureErr
}

func TestEpicAdapter_NotConfigured_ReturnsError(t *testing.T) {
	fake := &fakeEpicClient{configured: false}
	a := NewAdapter(fake, "user1", map[string]string{"k": "v"}, nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when legendary not configured, got nil")
	}
	if fake.restoreCalled || fake.getLibraryCalled || fake.captureCalled {
		t.Error("expected no client calls when not configured")
	}
}

func TestEpicAdapter_RestoresSnapshotBeforeFetch(t *testing.T) {
	snapshot := map[string]string{"user.json": `{"displayName":"Test"}`}
	fake := &fakeEpicClient{configured: true, captureSnapshot: map[string]string{}}
	a := NewAdapter(fake, "user1", snapshot, nil)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.restoreCalled {
		t.Error("expected RestoreSnapshot to be called")
	}
	if fake.restoredSnapshot["user.json"] != snapshot["user.json"] {
		t.Errorf("snapshot mismatch: got %v, want %v", fake.restoredSnapshot, snapshot)
	}
}

func TestEpicAdapter_PersistsNewSnapshotAfterSuccess(t *testing.T) {
	newSnapshot := map[string]string{"user.json": `{"displayName":"Updated"}`}
	fake := &fakeEpicClient{configured: true, captureSnapshot: newSnapshot}

	var capturedSnapshot map[string]string
	onSnapshot := func(s map[string]string) error {
		capturedSnapshot = s
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSnapshot == nil {
		t.Fatal("expected onSnapshot to be called")
	}
	if capturedSnapshot["user.json"] != newSnapshot["user.json"] {
		t.Errorf("snapshot content mismatch: got %v, want %v", capturedSnapshot, newSnapshot)
	}
}

func TestEpicAdapter_PersistsSnapshotEvenOnFetchError(t *testing.T) {
	newSnapshot := map[string]string{"user.json": `{"displayName":"Updated"}`}
	fetchErr := errors.New("library fetch failed")
	fake := &fakeEpicClient{
		configured:      true,
		getLibraryErr:   fetchErr,
		captureSnapshot: newSnapshot,
	}
	var onSnapshotCalled bool
	onSnapshot := func(map[string]string) error {
		onSnapshotCalled = true
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, fetchErr) {
		t.Errorf("expected fetchErr, got %v", err)
	}
	if !onSnapshotCalled {
		t.Error("expected onSnapshot to be called even on fetch error")
	}
}

func TestEpicAdapter_SkipsPersistWhenSnapshotEmpty(t *testing.T) {
	fake := &fakeEpicClient{configured: true, captureSnapshot: map[string]string{}}
	var onSnapshotCalled bool
	onSnapshot := func(map[string]string) error {
		onSnapshotCalled = true
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if onSnapshotCalled {
		t.Error("expected onSnapshot NOT to be called when captured snapshot is empty")
	}
}

func TestEpicAdapter_MapsEntriesToStorefrontFormat(t *testing.T) {
	fake := &fakeEpicClient{
		configured: true,
		libraryBatches: [][]ExternalGameEntry{
			{{ExternalID: "fortnite", Title: "Fortnite", OwnershipStatus: "owned"}},
		},
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	var received []storefrontadapter.ExternalGameEntry
	if err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		received = append(received, batch...)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(received))
	}
	got := received[0]
	if got.ExternalID != "fortnite" || got.Title != "Fortnite" {
		t.Errorf("unexpected entry: %+v", got)
	}
	if len(got.Platforms) != 1 || got.Platforms[0] != "pc-windows" {
		t.Errorf("expected [pc-windows], got %v", got.Platforms)
	}
	if got.PlaytimeHours != 0 {
		t.Errorf("expected 0 playtime, got %v", got.PlaytimeHours)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/services/epic/... -run TestEpicAdapter -v
```

Expected: compilation error — `NewAdapter` undefined.

- [ ] **Step 3: Create the implementation**

Create `internal/services/epic/adapter.go`:

```go
package epic

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// clientInterface is the subset of *Client that Adapter depends on.
// Tests inject a fake without invoking the real legendary subprocess.
type clientInterface interface {
	Configured() bool
	RestoreSnapshot(userID string, snapshot map[string]string) error
	GetLibrary(ctx context.Context, userID string, onBatch func([]ExternalGameEntry) error) error
	CaptureSnapshot(userID string) (map[string]string, error)
}

// Adapter implements storefrontadapter.Adapter for the Epic Games Store via
// the Legendary CLI. It restores a session snapshot before fetching and
// captures the updated snapshot afterward, delegating persistence to onSnapshot.
type Adapter struct {
	client     clientInterface
	userID     string
	snapshot   map[string]string
	onSnapshot func(map[string]string) error
}

// NewAdapter returns a storefrontadapter.Adapter for Epic Games Store.
// snapshot is the decrypted legendary state loaded from user_sync_configs.
// onSnapshot is called with the updated snapshot after CaptureSnapshot; the
// factory wires the DB write here. onSnapshot may be nil.
func NewAdapter(
	client clientInterface,
	userID string,
	snapshot map[string]string,
	onSnapshot func(map[string]string) error,
) storefrontadapter.Adapter {
	return &Adapter{
		client:     client,
		userID:     userID,
		snapshot:   snapshot,
		onSnapshot: onSnapshot,
	}
}

// GetLibrary implements storefrontadapter.Adapter.
func (a *Adapter) GetLibrary(ctx context.Context, _ int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	if !a.client.Configured() {
		return fmt.Errorf("epic: legendary not configured (LEGENDARY_WORK_DIR unset)")
	}

	if err := a.client.RestoreSnapshot(a.userID, a.snapshot); err != nil {
		return fmt.Errorf("epic: restore snapshot: %w", err)
	}

	fetchErr := a.client.GetLibrary(ctx, a.userID, func(batch []ExternalGameEntry) error {
		mapped := make([]storefrontadapter.ExternalGameEntry, 0, len(batch))
		for _, e := range batch {
			mapped = append(mapped, storefrontadapter.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   0,
				Platforms:       []string{"pc-windows"},
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  false,
			})
		}
		return onBatch(mapped)
	})

	// Capture updated snapshot regardless of fetch error.
	newSnapshot, captureErr := a.client.CaptureSnapshot(a.userID)
	if captureErr != nil {
		slog.Error("epic: capture snapshot failed", "user_id", a.userID, "err", captureErr)
	} else if len(newSnapshot) > 0 && a.onSnapshot != nil {
		if err := a.onSnapshot(newSnapshot); err != nil {
			slog.Error("epic: persist updated snapshot failed", "user_id", a.userID, "err", err)
		}
	}

	return fetchErr
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/services/epic/... -run TestEpicAdapter -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Verify `*Client` satisfies `clientInterface`**

```bash
go build ./internal/services/epic/...
```

Expected: builds cleanly (confirms `*epic.Client` satisfies `clientInterface` — otherwise the compiler would complain when the factory passes a `*Client` as `clientInterface` in Task 2).

- [ ] **Step 6: Commit**

```bash
git add internal/services/epic/adapter.go internal/services/epic/adapter_test.go
git commit -m "feat(sync): add epic.Adapter in services/epic with callback pattern"
```

---

## Task 2: Wire `epic.Adapter` into the factory; remove `EpicClientAdapter` from `tasks`

**Files:**
- Modify: `cmd/nexorious/serve.go` (epic factory case)
- Modify: `internal/worker/tasks/sync.go` (remove `EpicClientAdapter`, `epicSubprocessClient`, `epicsvc` import)
- Modify: `internal/worker/tasks/sync_test.go` (remove `TestEpicClientAdapter_*` tests and helpers)

---

- [ ] **Step 1: Update the epic factory case in `serve.go`**

In `cmd/nexorious/serve.go`, find the `buildAdapterFactory` function. Replace the `case "epic":` block (currently ~4 lines returning `&tasks.EpicClientAdapter{...}`) with:

```go
case "epic":
    if cfg.StorefrontCredentials == nil {
        return nil, tasks.ErrCredentials
    }
    plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        slog.Warn("adapter factory: epic decrypt failed", "user_id", cfg.UserID, "err", err)
        return nil, fmt.Errorf("%w: epic decrypt failed", tasks.ErrCredentials)
    }
    var snapshot map[string]string
    if err := json.Unmarshal(plain, &snapshot); err != nil {
        return nil, tasks.ErrCredentials
    }
    onSnapshot := func(s map[string]string) error {
        newJSON, _ := json.Marshal(s)
        enc, encErr := encrypter.Encrypt(newJSON)
        if encErr != nil {
            return encErr
        }
        _, dbErr := db.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
            enc, cfg.UserID,
        ).Exec(context.Background())
        return dbErr
    }
    return epicsvc.NewAdapter(epicClient, cfg.UserID, snapshot, onSnapshot), nil
```

Also change the anonymous function parameter from `ctx context.Context` to `_ context.Context` since `ctx` is no longer used in the factory body:

```go
return func(_ context.Context, storefront string, cfg models.UserSyncConfig) (tasks.StorefrontAdapter, error) {
```

- [ ] **Step 2: Remove `EpicClientAdapter` and `epicSubprocessClient` from `tasks/sync.go`**

Delete the following blocks entirely from `internal/worker/tasks/sync.go`:

1. The `epicSubprocessClient` interface (lines beginning with `// epicSubprocessClient is the subset...` through the closing `}`)
2. The `// EpicClientAdapter implements StorefrontAdapter...` comment block
3. The `type EpicClientAdapter struct { ... }` struct
4. The `func (a *EpicClientAdapter) GetLibrary(...)` method and its entire body

Also remove the import line:
```go
epicsvc "github.com/drzero42/nexorious/internal/services/epic"
```

**Verify the remaining imports are still valid** — `context`, `encoding/json`, and `log/slog` are all still used elsewhere in the file.

- [ ] **Step 3: Remove Epic adapter tests from `tasks/sync_test.go`**

Delete from `internal/worker/tasks/sync_test.go`:

1. The `// EpicClientAdapter — DB↔disk snapshot round-trip` section header comment
2. `type fakeEpicSubprocessClient struct { ... }` and all its methods (`Configured`, `RestoreSnapshot`, `GetLibrary`, `CaptureSnapshot`)
3. `func seedEpicConfig(...)` helper
4. `func readEpicSnapshot(...)` helper
5. All six test functions: `TestEpicClientAdapter_NotConfigured_ReturnsErrorWithoutTouchingDB`, `TestEpicClientAdapter_NoSnapshotInDB_ReturnsError`, `TestEpicClientAdapter_RestoresSnapshotFromDB`, `TestEpicClientAdapter_PersistsNewSnapshotAfterSuccess`, `TestEpicClientAdapter_PersistsSnapshotEvenOnFetchError`, `TestEpicClientAdapter_SkipsPersistWhenSnapshotEmpty`

Also remove the import line:
```go
epicsvc "github.com/drzero42/nexorious/internal/services/epic"
```

Update the stale comment in the `DispatchSyncWorker` test that references `EpicClientAdapter` (around line 946). Change:
```go
// Epic games are mapped by the EpicClientAdapter to pc-windows.
```
to:
```go
// Epic games are always mapped to pc-windows by the Epic adapter.
```

- [ ] **Step 4: Build the project**

```bash
make build
```

Expected: clean build with no errors.

- [ ] **Step 5: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass. The six removed `TestEpicClientAdapter_*` tests are gone; new `TestEpicAdapter_*` tests in `services/epic` pass.

- [ ] **Step 6: Run linter**

```bash
golangci-lint run
```

Expected: zero errors.

- [ ] **Step 7: Commit**

```bash
git add cmd/nexorious/serve.go \
        internal/worker/tasks/sync.go \
        internal/worker/tasks/sync_test.go
git commit -m "refactor(sync): move EpicClientAdapter to services/epic; wire callback in factory"
```

---

## Task 3: Update `gog.Adapter` to own token refresh

**Files:**
- Modify: `internal/services/gog/adapter.go`
- Create: `internal/services/gog/adapter_test.go`

---

- [ ] **Step 1: Write the failing tests**

Create `internal/services/gog/adapter_test.go`:

```go
package gog

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeGOGClient satisfies clientInterface without making real HTTP calls.
type fakeGOGClient struct {
	refreshResult *TokenResponse
	refreshErr    error
	libraryErr    error
	libraryEntries []ExternalGameEntry

	refreshCalled      bool
	getLibraryCalled   bool
	refreshInput       string
	getLibraryAccessToken string
}

func (f *fakeGOGClient) RefreshToken(_ context.Context, refreshToken string) (*TokenResponse, error) {
	f.refreshCalled = true
	f.refreshInput = refreshToken
	return f.refreshResult, f.refreshErr
}

func (f *fakeGOGClient) GetLibrary(_ context.Context, accessToken string, _ int, onBatch func([]ExternalGameEntry) error) error {
	f.getLibraryCalled = true
	f.getLibraryAccessToken = accessToken
	if f.libraryErr != nil {
		return f.libraryErr
	}
	if len(f.libraryEntries) > 0 {
		return onBatch(f.libraryEntries)
	}
	return nil
}

func TestGOGAdapter_RefreshesTokenBeforeFetch(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	a := NewAdapter(fake, "old-refresh", nil)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.refreshCalled {
		t.Error("expected RefreshToken to be called")
	}
	if fake.refreshInput != "old-refresh" {
		t.Errorf("expected refresh input 'old-refresh', got %q", fake.refreshInput)
	}
	if fake.getLibraryAccessToken != "new-access" {
		t.Errorf("expected GetLibrary called with 'new-access', got %q", fake.getLibraryAccessToken)
	}
}

func TestGOGAdapter_CallsOnNewTokensAfterRefresh(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	var gotAccess, gotRefresh string
	onNewTokens := func(access, refresh string) error {
		gotAccess = access
		gotRefresh = refresh
		return nil
	}
	a := NewAdapter(fake, "old-refresh", onNewTokens)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAccess != "new-access" || gotRefresh != "new-refresh" {
		t.Errorf("onNewTokens got (%q, %q), want ('new-access', 'new-refresh')", gotAccess, gotRefresh)
	}
}

func TestGOGAdapter_AuthExpiredReturnsCredentialsError(t *testing.T) {
	fake := &fakeGOGClient{refreshErr: ErrGOGAuthExpired}
	a := NewAdapter(fake, "expired-refresh", nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected ErrCredentials, got %v", err)
	}
	if fake.getLibraryCalled {
		t.Error("expected GetLibrary not to be called on auth failure")
	}
}

func TestGOGAdapter_TransientRefreshErrorPropagates(t *testing.T) {
	transientErr := errors.New("connection refused")
	fake := &fakeGOGClient{refreshErr: transientErr}
	a := NewAdapter(fake, "old-refresh", nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, transientErr) {
		t.Errorf("expected transient error to propagate, got %v", err)
	}
	if errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Error("expected non-credentials error for transient failure")
	}
}

func TestGOGAdapter_OnNewTokensFailureDoesNotAbortFetch(t *testing.T) {
	fake := &fakeGOGClient{
		refreshResult: &TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"},
	}
	onNewTokens := func(_, _ string) error {
		return errors.New("db write failed")
	}
	a := NewAdapter(fake, "old-refresh", onNewTokens)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("expected no error when persist fails, got %v", err)
	}
	if !fake.getLibraryCalled {
		t.Error("expected GetLibrary to still be called after persist failure")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/services/gog/... -run TestGOGAdapter -v
```

Expected: compilation error — `NewAdapter` has wrong signature (currently takes `*Client, string`).

- [ ] **Step 3: Update `gog/adapter.go`**

Replace the entire content of `internal/services/gog/adapter.go` with:

```go
package gog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// clientInterface is the subset of *Client that Adapter depends on.
// Tests inject a fake without making real HTTP calls.
type clientInterface interface {
	RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)
	GetLibrary(ctx context.Context, accessToken string, batchSize int, onBatch func([]ExternalGameEntry) error) error
}

// Adapter wraps a Client and implements storefrontadapter.Adapter.
// It refreshes the OAuth2 access token before each fetch, delegating
// token persistence to the onNewTokens callback.
type Adapter struct {
	client       clientInterface
	refreshToken string
	onNewTokens  func(accessToken, refreshToken string) error
}

// NewAdapter returns a storefrontadapter.Adapter for GOG.
// refreshToken is the OAuth2 refresh token loaded from user_sync_configs.
// onNewTokens is called with the new access/refresh pair after a successful
// token refresh; the factory wires the DB write here. onNewTokens may be nil.
func NewAdapter(
	client clientInterface,
	refreshToken string,
	onNewTokens func(accessToken, refreshToken string) error,
) storefrontadapter.Adapter {
	return &Adapter{client: client, refreshToken: refreshToken, onNewTokens: onNewTokens}
}

// GetLibrary implements storefrontadapter.Adapter.
func (a *Adapter) GetLibrary(ctx context.Context, batchSize int, onBatch func([]storefrontadapter.ExternalGameEntry) error) error {
	tok, err := a.client.RefreshToken(ctx, a.refreshToken)
	if errors.Is(err, ErrGOGAuthExpired) {
		return fmt.Errorf("%w: gog token refresh failed", storefrontadapter.ErrCredentials)
	}
	if err != nil {
		return fmt.Errorf("gog: token refresh: %w", err)
	}

	if a.onNewTokens != nil {
		if err := a.onNewTokens(tok.AccessToken, tok.RefreshToken); err != nil {
			slog.Error("gog: persist refreshed tokens failed", "err", err)
		}
	}

	return a.client.GetLibrary(ctx, tok.AccessToken, batchSize, func(entries []ExternalGameEntry) error {
		mapped := make([]storefrontadapter.ExternalGameEntry, 0, len(entries))
		for _, e := range entries {
			mapped = append(mapped, storefrontadapter.ExternalGameEntry{
				ExternalID:      e.ExternalID,
				Title:           e.Title,
				PlaytimeHours:   float64(e.PlaytimeHours),
				Platforms:       e.Platforms,
				OwnershipStatus: e.OwnershipStatus,
				IsSubscription:  e.IsSubscription,
			})
		}
		return onBatch(mapped)
	})
}
```

- [ ] **Step 4: Run the GOG adapter tests**

```bash
go test ./internal/services/gog/... -run TestGOGAdapter -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Run the full GOG test suite to ensure no regressions**

```bash
go test ./internal/services/gog/... -v
```

Expected: all tests pass (auth_test.go and library_test.go still pass).

- [ ] **Step 6: Commit the adapter**

```bash
git add internal/services/gog/adapter.go internal/services/gog/adapter_test.go
git commit -m "feat(sync): gog.Adapter owns OAuth token refresh via callback"
```

---

## Task 4: Wire updated `gog.Adapter` into the factory; final verification

**Files:**
- Modify: `cmd/nexorious/serve.go` (gog factory case)

---

- [ ] **Step 1: Update the gog factory case in `serve.go`**

In `cmd/nexorious/serve.go`, find the `case "gog":` block inside `buildAdapterFactory`. Replace the entire block (from `case "gog":` through `return gogsvc.NewAdapter(gogClient, newTok.AccessToken), nil`) with:

```go
case "gog":
    if cfg.StorefrontCredentials == nil {
        return nil, tasks.ErrCredentials
    }
    plain, err := encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        slog.Warn("adapter factory: gog decrypt failed", "user_id", cfg.UserID, "err", err)
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
        newCredsJSON, _ := json.Marshal(creds)
        enc, encErr := encrypter.Encrypt(newCredsJSON)
        if encErr != nil {
            return encErr
        }
        _, dbErr := db.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
            enc, cfg.UserID,
        ).Exec(context.Background())
        return dbErr
    }
    return gogsvc.NewAdapter(gogsvc.NewClient(), creds.RefreshToken, onNewTokens), nil
```

- [ ] **Step 2: Build the project**

```bash
make build
```

Expected: clean build with no errors.

- [ ] **Step 3: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass.

- [ ] **Step 4: Run linter**

```bash
golangci-lint run
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/nexorious/serve.go
git commit -m "refactor(sync): wire gog.Adapter token refresh callback in factory"
```

---

## Self-Review Checklist

After writing this plan, verifying against the spec:

**Spec coverage:**
- ✅ `epic.Adapter` in `services/epic` with `clientInterface` (Spec Section 1)
- ✅ `onSnapshot` callback pattern (Spec Section 1)
- ✅ Factory decrypts snapshot + builds closure (Spec Section 1, serve.go changes)
- ✅ `EpicClientAdapter` removed from `tasks/sync.go` (Spec Section 1)
- ✅ Epic adapter tests migrated, no DB dependency (Spec Section 1 / Testing)
- ✅ `gog.Adapter` owns `RefreshToken` call (Spec Section 2)
- ✅ `ErrGOGAuthExpired` → `ErrCredentials` (Spec Section 2)
- ✅ `onNewTokens` callback — persist failure does not abort fetch (Spec Section 2)
- ✅ GOG factory case simplified (Spec Section 2, serve.go changes)
- ✅ GOG adapter tests: refresh, onNewTokens, auth expiry, transient error, persist failure (Spec Testing section)

**Type consistency:**
- `NewAdapter` in `epic/adapter.go` takes `clientInterface` — factory passes `*epic.Client` which satisfies the interface ✓
- `NewAdapter` in `gog/adapter.go` takes `clientInterface` — factory passes `*gog.Client` which satisfies the interface (`RefreshToken` and `GetLibrary` both implemented) ✓
- `onSnapshot func(map[string]string) error` matches the closure type in serve.go ✓
- `onNewTokens func(accessToken, refreshToken string) error` matches the closure type in serve.go ✓
