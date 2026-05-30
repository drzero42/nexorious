# Credential Encryption at Rest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Encrypt all storefront credentials at rest with AES-256-GCM using a new `DB_ENCRYPTION_KEY` env var, keeping `SECRET_KEY` scoped to JWT signing only.

**Architecture:** A new `internal/crypto` package provides an `Encrypter` struct wired via dependency injection into `SyncHandler`, `DispatchSyncWorker`, and `EpicClientAdapter`. Write paths encrypt before persisting; read paths decrypt before unmarshal, clearing credentials on decrypt failure. The `epic_legendary_state` JSONB column stores ciphertext as a JSON string (e.g. `"enc:v1:base64..."`) to avoid a schema change.

**Tech Stack:** Go stdlib `crypto/aes`, `crypto/cipher`, `crypto/rand`, `crypto/sha256`, `encoding/base64`; Bun ORM; Echo v5; existing testcontainers test infrastructure.

---

## File Structure

**New:**
- `internal/crypto/crypto.go` — `Encrypter` struct, `NewEncrypter`, `Encrypt`, `Decrypt`
- `internal/crypto/crypto_test.go` — unit tests for the crypto primitive

**Modified:**
- `internal/config/config.go` — add `DBEncryptionKey` field, fix `SecretKey` comment
- `internal/api/router.go` — add `encrypter *crypto.Encrypter` to `New()` and `registerRoutes()`, pass to `NewSyncHandler`
- `internal/api/sync.go` — add `encrypter` to `SyncHandler` struct + `NewSyncHandler`; encrypt on write, decrypt on read
- `internal/api/auth_test.go` — add `DBEncryptionKey` to `testCfg()`
- `internal/api/main_test.go` — add `testEncrypter` package-level var
- `internal/api/sync_test.go` — update `newSyncTestApp` to pass encrypter; add encrypt/decrypt fixture tests
- `internal/api/router_test.go` — update all `api.New(...)` call sites to pass encrypter
- `internal/api/games_test.go` — update `api.New(...)` call site to pass encrypter
- `internal/worker/tasks/sync.go` — add `Encrypter` to `DispatchSyncWorker` + `EpicClientAdapter`; encrypt/decrypt all credential operations
- `internal/worker/tasks/main_test.go` — add `testEncrypter` package-level var
- `internal/worker/tasks/sync_test.go` — encrypt all credential fixtures; update worker construction
- `cmd/nexorious/serve.go` — construct encrypter at startup (fail fast), pass to `api.New` and workers
- `.env.example` — add `DB_ENCRYPTION_KEY`
- `deploy/docker/.env.example` — add `DB_ENCRYPTION_KEY`
- `deploy/docker/docker-compose.yml` — add `DB_ENCRYPTION_KEY` env var
- `devenv.nix` — add dev-only `DB_ENCRYPTION_KEY` placeholder
- `deploy/helm/values.yaml` — add `dbEncryptionKey` / `dbEncryptionKeyFrom` config
- `deploy/helm/templates/_helpers.tpl` — clone `secretKey` machinery for `dbEncryptionKey`
- `deploy/helm/templates/credentials-secret.yaml` — add `DB_ENCRYPTION_KEY` entry
- `deploy/helm/templates/NOTES.txt` — add default-placeholder warning
- `nix/module.nix` — add `DB_ENCRYPTION_KEY` to docs
- `README.md` — add `DB_ENCRYPTION_KEY` to env-var checklist and example
- `CLAUDE.md` — add `DB_ENCRYPTION_KEY` to initial-setup snippet

---

## Task 1: `internal/crypto` package

**Files:**
- Create: `internal/crypto/crypto.go`
- Create: `internal/crypto/crypto_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/crypto/crypto_test.go`:

```go
package crypto_test

import (
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/crypto"
)

const testKey = "test-db-encryption-key-32-bytes!!"

func mustEncrypter(t *testing.T, key string) *crypto.Encrypter {
	t.Helper()
	enc, err := crypto.NewEncrypter(key)
	if err != nil {
		t.Fatalf("NewEncrypter: %v", err)
	}
	return enc
}

func TestEncrypter_RoundTrip(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	plain := []byte(`{"web_api_key":"abc","steam_id":"123"}`)
	ciphertext, err := enc.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ciphertext, "enc:v1:") {
		t.Fatalf("expected enc:v1: prefix, got %q", ciphertext[:min(len(ciphertext), 20)])
	}
	got, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestEncrypter_UniqueNonces(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	plain := []byte("same plaintext")
	c1, _ := enc.Encrypt(plain)
	c2, _ := enc.Encrypt(plain)
	if c1 == c2 {
		t.Fatal("two encryptions of the same plaintext must produce different ciphertexts")
	}
}

func TestEncrypter_TamperDetection(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	ciphertext, _ := enc.Encrypt([]byte("sensitive"))
	// flip the last byte of the base64 payload
	b := []byte(ciphertext)
	b[len(b)-1] ^= 0xFF
	_, err := enc.Decrypt(string(b))
	if err == nil {
		t.Fatal("expected error on tampered ciphertext, got nil")
	}
}

func TestEncrypter_WrongKey(t *testing.T) {
	enc1 := mustEncrypter(t, testKey)
	enc2 := mustEncrypter(t, "different-db-encryption-key-32b!")
	ciphertext, _ := enc1.Encrypt([]byte("secret"))
	_, err := enc2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key, got nil")
	}
}

func TestEncrypter_MissingPrefix(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	_, err := enc.Decrypt("plain json, no prefix")
	if err == nil {
		t.Fatal("expected error for missing enc:v1: prefix, got nil")
	}
}

func TestEncrypter_EmptyPlaintext(t *testing.T) {
	enc := mustEncrypter(t, testKey)
	_, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
}

func TestNewEncrypter_ShortKey(t *testing.T) {
	_, err := crypto.NewEncrypter("tooshort")
	if err == nil {
		t.Fatal("expected error for key shorter than 32 bytes, got nil")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Run tests — expect compile failure (package doesn't exist yet)**

```bash
go test ./internal/crypto/... 2>&1 | head -5
```

Expected: `no Go files in .../internal/crypto` or similar.

- [ ] **Step 3: Implement `internal/crypto/crypto.go`**

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const prefix = "enc:v1:"

// Encrypter performs AES-256-GCM encryption and decryption.
type Encrypter struct {
	key [32]byte
}

// NewEncrypter validates that rawKey is at least 32 bytes (security floor on
// raw entropy), derives a 32-byte AES-256 key via SHA-256(rawKey), and returns
// the Encrypter.
func NewEncrypter(rawKey string) (*Encrypter, error) {
	if len(rawKey) < 32 {
		return nil, fmt.Errorf("crypto: DB_ENCRYPTION_KEY must be at least 32 bytes, got %d", len(rawKey))
	}
	key := sha256.Sum256([]byte(rawKey))
	return &Encrypter{key: key}, nil
}

// Encrypt returns "enc:v1:" + base64(nonce || ciphertext || GCM tag).
// Uses AES-256-GCM with a random 12-byte nonce per call.
func (e *Encrypter) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize()) // 12 bytes
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce.
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt strips the "enc:v1:" prefix, base64-decodes, splits out the 12-byte
// nonce, and decrypts. Returns error on wrong key, tampered data, or missing prefix.
func (e *Encrypter) Decrypt(ciphertext string) ([]byte, error) {
	if !strings.HasPrefix(ciphertext, prefix) {
		return nil, fmt.Errorf("crypto: missing enc:v1: prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext[len(prefix):])
	if err != nil {
		return nil, fmt.Errorf("crypto: base64 decode: %w", err)
	}
	block, err := aes.NewCipher(e.key[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plain, nil
}
```

- [ ] **Step 4: Run tests — expect all pass**

```bash
go test ./internal/crypto/... -v
```

Expected: all 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/
git commit -m "feat: add internal/crypto AES-256-GCM encrypter"
```

---

## Task 2: Config + startup wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `cmd/nexorious/serve.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/sync.go` (struct + constructor only)
- Modify: `internal/worker/tasks/sync.go` (struct fields only)
- Modify: `internal/api/auth_test.go`
- Modify: `internal/api/main_test.go`
- Modify: `internal/api/router_test.go`
- Modify: `internal/api/games_test.go`
- Modify: `internal/api/sync_test.go` (helper signature only)
- Modify: `internal/worker/tasks/main_test.go`

- [ ] **Step 1: Add `DBEncryptionKey` to config and fix `SecretKey` comment**

In `internal/config/config.go`, replace:

```go
// SecretKey is used for JWT signing and credential encryption.
SecretKey string `env:"SECRET_KEY,required"`
```

with:

```go
// SecretKey is used for JWT signing.
SecretKey string `env:"SECRET_KEY,required"`

// DBEncryptionKey is used for at-rest encryption of storefront credentials.
// Generate with: openssl rand -base64 32
DBEncryptionKey string `env:"DB_ENCRYPTION_KEY,required"`
```

- [ ] **Step 2: Construct encrypter in `serve.go` and pass to `api.New`**

In `cmd/nexorious/serve.go`, add the import `"github.com/drzero42/nexorious/internal/crypto"` and insert immediately after `cfg` is loaded (before the database setup):

```go
encrypter, err := crypto.NewEncrypter(cfg.DBEncryptionKey)
if err != nil {
    slog.Error("invalid DB_ENCRYPTION_KEY", "err", err)
    os.Exit(1)
}
```

Then on the `api.New(...)` call (line ~312), add `encrypter` as the second argument:

```go
e := api.New(encrypter, cfg, migrator, db, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, riverClient)
```

- [ ] **Step 3: Update `api.New` and `registerRoutes` signatures in `internal/api/router.go`**

Change line 35 from:

```go
func New(cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
```

to:

```go
func New(encrypter *crypto.Encrypter, cfg *config.Config, migrator *migrate.Migrator, db *bun.DB, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient ...*river.Client[pgx.Tx]) *echo.Echo {
```

Add `"github.com/drzero42/nexorious/internal/crypto"` to the imports in `router.go`.

Inside `New`, find the call to `registerRoutes` (line ~122) and thread `encrypter` through:

```go
registerRoutes(encrypter, e, cfg, mh, db, migrator, resolvedDatabaseURL, igdbClient, backupSvc, restoreCallbacks, rc)
```

Change line 127 from:

```go
func registerRoutes(e *echo.Echo, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient *river.Client[pgx.Tx]) {
```

to:

```go
func registerRoutes(encrypter *crypto.Encrypter, e *echo.Echo, cfg *config.Config, mh *migrate.Handler, db *bun.DB, migrator *migrate.Migrator, resolvedDatabaseURL string, igdbClient *igdb.Client, backupSvc *backup.Service, restoreCallbacks *RestoreCallbacks, riverClient *river.Client[pgx.Tx]) {
```

Inside `registerRoutes`, find line ~315 where `NewSyncHandler` is called and add `encrypter`:

```go
synch := NewSyncHandler(encrypter, db, riverClient, &steamClientAdapter{c: steamSvc}, &psnClientAdapter{c: psnSvc}, &epicClientAdapter{c: epicSvc}, &gogClientAdapter{c: gogSvc})
```

- [ ] **Step 4: Add `encrypter` field to `SyncHandler` and update `NewSyncHandler`**

In `internal/api/sync.go`, change the `SyncHandler` struct (line ~171) to:

```go
type SyncHandler struct {
	encrypter   *crypto.Encrypter
	db          *bun.DB
	riverClient *river.Client[pgx.Tx]
	steamClient SteamClient
	psnClient   PSNClient
	epicClient  EpicClient
	gogClient   GOGClient
}
```

Add `"github.com/drzero42/nexorious/internal/crypto"` to the imports in `sync.go`.

Change `NewSyncHandler` (line ~181) to:

```go
func NewSyncHandler(encrypter *crypto.Encrypter, db *bun.DB, riverClient *river.Client[pgx.Tx], steam SteamClient, psn PSNClient, epic EpicClient, gog GOGClient) *SyncHandler {
	return &SyncHandler{encrypter: encrypter, db: db, riverClient: riverClient, steamClient: steam, psnClient: psn, epicClient: epic, gogClient: gog}
}
```

- [ ] **Step 5: Add `Encrypter` field to `DispatchSyncWorker` and `EpicClientAdapter`**

In `internal/worker/tasks/sync.go`, add `"github.com/drzero42/nexorious/internal/crypto"` to imports.

Change `DispatchSyncWorker` struct (line ~140):

```go
type DispatchSyncWorker struct {
	river.WorkerDefaults[DispatchSyncArgs]
	DB          *bun.DB
	Encrypter   *crypto.Encrypter
	Steam       SteamLibraryAdapter
	PSN         PSNLibraryAdapter
	Epic        EpicLibraryAdapter
	GOG         GOGLibraryAdapter
	RiverClient *river.Client[pgx.Tx]
}
```

Change `EpicClientAdapter` struct (line ~69):

```go
type EpicClientAdapter struct {
	Client    *epicsvc.Client
	DB        *bun.DB
	Encrypter *crypto.Encrypter
}
```

- [ ] **Step 6: Wire encrypter into workers in `serve.go`**

Change the `dispatchSyncWorker` construction (line ~165):

```go
dispatchSyncWorker := &tasks.DispatchSyncWorker{
    DB:        db,
    Encrypter: encrypter,
    Steam:     steamsvc.NewClient(),
    PSN:       psnsvc.NewClient(),
    Epic:      &tasks.EpicClientAdapter{Client: epicsvc.NewClient(cfg.LegendaryWorkDir), DB: db, Encrypter: encrypter},
    GOG:       gogsvc.NewClient(),
}
```

Also update the re-creation of workers inside the dynamic re-wiring block (line ~242):

```go
newDispatchSync := &tasks.DispatchSyncWorker{
    DB:        newDB,
    Encrypter: encrypter,
    Steam:     steamsvc.NewClient(),
    PSN:       psnsvc.NewClient(),
    Epic:      &tasks.EpicClientAdapter{Client: epicsvc.NewClient(cfg.LegendaryWorkDir), DB: newDB, Encrypter: encrypter},
    GOG:       gogsvc.NewClient(),
}
```

- [ ] **Step 7: Fix test call sites — add `DBEncryptionKey` to `testCfg()` and add `testEncrypter` vars**

In `internal/api/auth_test.go`, update `testCfg()`:

```go
func testCfg() *config.Config {
    return &config.Config{
        SecretKey:                "test-secret-key-at-least-32-bytes!",
        DBEncryptionKey:          "test-db-encryption-key-32-bytes!!",
        AccessTokenExpireMinutes: 15,
        RefreshTokenExpireDays:   30,
        Port:                     8000,
    }
}
```

In `internal/api/main_test.go`, add after the `var testConnStr string` declaration:

```go
// testEncrypter is the shared Encrypter for all api_test tests.
var testEncrypter *crypto.Encrypter
```

Add to the `TestMain` function, immediately before `os.Exit(m.Run())`:

```go
var encErr error
testEncrypter, encErr = crypto.NewEncrypter(testCfg().DBEncryptionKey)
if encErr != nil {
    panic("test encrypter: " + encErr.Error())
}
```

Add the import `"github.com/drzero42/nexorious/internal/crypto"` to `main_test.go`.

- [ ] **Step 8: Update all `api.New(...)` call sites in test files to pass `testEncrypter`**

In `internal/api/router_test.go`, every call like:

```go
e := api.New(testCfg(), m, nil, "", nil, nil, nil)
```

becomes:

```go
e := api.New(testEncrypter, testCfg(), m, nil, "", nil, nil, nil)
```

Do this for all occurrences (there are ~15). Use a search-and-replace but verify the file compiles afterwards.

In `internal/api/games_test.go`, update the one occurrence:

```go
return api.New(testEncrypter, cfg, m, db, "", igdbClient, nil, nil)
```

- [ ] **Step 9: Update `newSyncTestApp` in `internal/api/sync_test.go`**

Change the helper:

```go
func newSyncTestApp(t *testing.T, db *bun.DB, steam api.SteamClient, psn api.PSNClient) interface {
    ServeHTTP(http.ResponseWriter, *http.Request)
} {
    t.Helper()
    cfg := testCfg()
    e := echo.New()
    ah := api.NewAuthHandler(testDB, cfg)
    e.POST("/api/auth/login", ah.HandleLogin)
    synch := api.NewSyncHandler(testEncrypter, db, nil, steam, psn, (api.EpicClient)(nil), (api.GOGClient)(nil))
    g := e.Group("/api/sync", auth.JWTMiddleware(cfg.SecretKey, db))
    synch.RegisterRoutes(g)
    return e
}
```

- [ ] **Step 10: Add `testEncrypter` to worker tasks test package**

In `internal/worker/tasks/main_test.go`, add after `var testConnStr string`:

```go
// testEncrypter is the shared Encrypter for all tasks_test tests.
var testEncrypter *crypto.Encrypter
```

Add to `TestMain`, immediately before `os.Exit(m.Run())`:

```go
var encErr error
testEncrypter, encErr = crypto.NewEncrypter("test-db-encryption-key-32-bytes!!")
if encErr != nil {
    panic("test encrypter: " + encErr.Error())
}
```

Add `"github.com/drzero42/nexorious/internal/crypto"` to the imports.

- [ ] **Step 11: Verify the build compiles**

```bash
go build ./...
```

Expected: no errors. (Tests may still fail since credentials aren't encrypted yet — that's fine.)

- [ ] **Step 12: Commit**

```bash
git add internal/config/config.go internal/api/router.go internal/api/sync.go \
    internal/worker/tasks/sync.go cmd/nexorious/serve.go \
    internal/api/auth_test.go internal/api/main_test.go internal/api/router_test.go \
    internal/api/games_test.go internal/api/sync_test.go \
    internal/worker/tasks/main_test.go
git commit -m "feat: wire DB_ENCRYPTION_KEY config and Encrypter into sync handler and workers"
```

---

## Task 3: Encrypt credential write paths in API handlers

**Files:**
- Modify: `internal/api/sync.go` (write paths: Steam verify, PSN configure, Epic connect, GOG connect)
- Modify: `internal/api/sync_test.go` (add tests verifying ciphertext is stored)

- [ ] **Step 1: Write a failing test that verifies Steam verify stores ciphertext**

Add to `internal/api/sync_test.go`, after the existing Steam verify tests:

```go
func TestSteamVerify_StoresCiphertext(t *testing.T) {
    truncateAllTables(t)
    e := newSyncTestApp(t, testDB, &stubSteamClient{
        summary: &api.SteamPlayerSummary{
            SteamID: "76561198000000001", PersonaName: "TestUser",
            CommunityVisibilityState: 3,
        },
    }, &stubPSNClient{})
    _, token := setupTagUser(t, testDB, e, "steam-cipher-1")

    rec := postJSONAuth(t, e, "/api/sync/steam/verify", map[string]any{
        "web_api_key": "AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH",
        "steam_id":    "76561198000000001",
    }, token)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }

    // The raw DB value must be ciphertext, not plaintext JSON.
    var raw string
    if err := testDB.NewRaw(
        `SELECT storefront_credentials FROM user_sync_configs WHERE storefront = 'steam'`,
    ).Scan(context.Background(), &raw); err != nil {
        t.Fatalf("scan storefront_credentials: %v", err)
    }
    if !strings.HasPrefix(raw, "enc:v1:") {
        t.Fatalf("expected ciphertext with enc:v1: prefix, got %q", raw[:min(len(raw), 30)])
    }
}
```

Add `"strings"` and `"context"` to the imports in `sync_test.go` if not already present. Add the `min` helper if not already present:

```go
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

- [ ] **Step 2: Run the test — expect failure**

```bash
go test ./internal/api/... -run TestSteamVerify_StoresCiphertext -v
```

Expected: FAIL — the DB stores plaintext JSON, not `enc:v1:...`.

- [ ] **Step 3: Encrypt in `HandleSteamVerify`**

In `internal/api/sync.go`, find the Steam verify write (around line 484):

```go
credsJSON, _ := json.Marshal(creds)
credsStr := string(credsJSON)
```

Replace with:

```go
credsJSON, _ := json.Marshal(creds)
credsStr, err := h.encrypter.Encrypt(credsJSON)
if err != nil {
    slog.Error("sync: encrypt steam credentials failed", "err", err, "user_id", userID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt credentials")
}
```

- [ ] **Step 4: Run the test — expect pass**

```bash
go test ./internal/api/... -run TestSteamVerify_StoresCiphertext -v
```

Expected: PASS.

- [ ] **Step 5: Encrypt in `HandlePSNConfigure`**

In `internal/api/sync.go`, find the PSN configure write (around line 551):

```go
credsJSON, _ := json.Marshal(creds)
credsStr := string(credsJSON)
```

Replace with:

```go
credsJSON, _ := json.Marshal(creds)
credsStr, err := h.encrypter.Encrypt(credsJSON)
if err != nil {
    slog.Error("psn: encrypt credentials failed", "err", err, "user_id", userID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt credentials")
}
```

- [ ] **Step 6: Encrypt in `HandleEpicConnect`**

In `internal/api/sync.go`, find the Epic connect write (around line 655). The current code:

```go
snapshotJSON, _ := json.Marshal(snapshot)
creds := map[string]string{
    "display_name": info.DisplayName,
    "account_id":   info.AccountID,
}
credsJSON, _ := json.Marshal(creds)
credsStr := string(credsJSON)
```

Replace with:

```go
// Encrypt the display_name/account_id credentials.
creds := map[string]string{
    "display_name": info.DisplayName,
    "account_id":   info.AccountID,
}
credsJSON, _ := json.Marshal(creds)
credsStr, err := h.encrypter.Encrypt(credsJSON)
if err != nil {
    slog.Error("epic: encrypt credentials failed", "err", err, "user_id", userID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt credentials")
}

// Encrypt the legendary state snapshot.
// Stored as a JSON string in the JSONB column: "enc:v1:base64..."
snapshotPlain, _ := json.Marshal(snapshot)
snapshotCiphertext, err := h.encrypter.Encrypt(snapshotPlain)
if err != nil {
    slog.Error("epic: encrypt legendary state failed", "err", err, "user_id", userID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt epic state")
}
snapshotJSON, _ := json.Marshal(snapshotCiphertext) // wrap as JSON string for JSONB column
```

Then update the model assignment to use `snapshotJSON` (unchanged field name, but now holds a JSON-encoded ciphertext string):

```go
row := &models.UserSyncConfig{
    ...
    StorefrontCredentials: &credsStr,
    EpicLegendaryState:    snapshotJSON,
    ...
}
```

- [ ] **Step 7: Encrypt in `HandleGOGConnect`**

In `internal/api/sync.go`, find the GOG connect write (around line 1118):

```go
credsJSON, _ := json.Marshal(creds)
credsStr := string(credsJSON)
```

Replace with:

```go
credsJSON, _ := json.Marshal(creds)
credsStr, err := h.encrypter.Encrypt(credsJSON)
if err != nil {
    slog.Error("gog: encrypt credentials failed", "err", err, "user_id", userID)
    return echo.NewHTTPError(http.StatusInternalServerError, "failed to encrypt credentials")
}
```

- [ ] **Step 8: Run all API tests**

```bash
go test ./internal/api/... -timeout 120s -v 2>&1 | tail -30
```

Expected: all tests pass. The `TestSteamVerify_StoresCiphertext` passes; existing tests pass because they don't check raw DB values.

- [ ] **Step 9: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: encrypt storefront credentials on write in API handlers"
```

---

## Task 4: Decrypt credential read paths in API handlers

**Files:**
- Modify: `internal/api/sync.go` (read paths: PSN status, Epic connection, GOG status)
- Modify: `internal/api/sync_test.go` (add decrypt round-trip tests)

- [ ] **Step 1: Write a failing test for PSN status decrypt**

Add to `internal/api/sync_test.go`:

```go
func TestPSNStatus_DecryptsCredentials(t *testing.T) {
    truncateAllTables(t)
    e := newSyncTestApp(t, testDB, &stubSteamClient{}, &stubPSNClient{})
    _, token := setupTagUser(t, testDB, e, "psn-decrypt-1")

    // Seed encrypted credentials directly.
    creds := map[string]any{
        "npsso_token":      "a" + strings.Repeat("b", 63),
        "online_id":        "TestUser",
        "account_id":       "acc-123",
        "region":           "eu",
        "is_verified":      true,
        "token_expired_at": nil,
    }
    credsJSON, _ := json.Marshal(creds)
    ciphertext, _ := testEncrypter.Encrypt(credsJSON)
    userID := userIDFromToken(t, token)
    _, _ = testDB.ExecContext(context.Background(),
        `INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials, created_at, updated_at)
         VALUES (gen_random_uuid(), ?, 'psn', 'manual', ?, now(), now())`,
        userID, ciphertext,
    )

    rec := getAuth(t, e, "/api/sync/psn/connection", token)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
    }
    var resp map[string]any
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if resp["online_id"] != "TestUser" {
        t.Fatalf("expected online_id=TestUser, got %v", resp["online_id"])
    }
}

// userIDFromToken extracts the user UUID from a JWT token via the auth endpoint.
func userIDFromToken(t *testing.T, token string) string {
    t.Helper()
    // The user_id is the subject in the JWT. Query the users table instead.
    var id string
    if err := testDB.NewRaw(`SELECT id FROM users ORDER BY created_at DESC LIMIT 1`).Scan(context.Background(), &id); err != nil {
        t.Fatalf("get user id: %v", err)
    }
    return id
}
```

- [ ] **Step 2: Run the test — expect failure**

```bash
go test ./internal/api/... -run TestPSNStatus_DecryptsCredentials -v
```

Expected: FAIL — `json.Unmarshal` of ciphertext fails, returns 500.

- [ ] **Step 3: Decrypt in `HandleGetPSNStatus`**

In `internal/api/sync.go`, find the PSN status read (around line 587):

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, psnStatusResponse{})
}

var creds struct { ... }
if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
    slog.Error("psn: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

Replace with:

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, psnStatusResponse{})
}

plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
if err != nil {
    slog.Warn("psn: credentials decrypt failed; clearing", "user_id", userID, "err", err)
    _, _ = h.db.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
        userID,
    ).Exec(context.Background())
    return c.JSON(http.StatusOK, psnStatusResponse{})
}

var creds struct {
    OnlineID       string     `json:"online_id"`
    AccountID      string     `json:"account_id"`
    Region         string     `json:"region"`
    IsVerified     bool       `json:"is_verified"`
    TokenExpiredAt *time.Time `json:"token_expired_at"`
}
if err := json.Unmarshal(plainCreds, &creds); err != nil {
    slog.Error("psn: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

- [ ] **Step 4: Decrypt in `HandleGetEpicConnection`**

In `internal/api/sync.go`, find the Epic connection read (around line 734):

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "disabled": false})
}

var creds struct { ... }
if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
    slog.Error("epic: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

Replace with:

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "disabled": false})
}

plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
if err != nil {
    slog.Warn("epic: credentials decrypt failed; clearing", "user_id", userID, "err", err)
    _, _ = h.db.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = NULL, epic_legendary_state = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
        userID,
    ).Exec(context.Background())
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "disabled": false})
}

var creds struct {
    DisplayName string `json:"display_name"`
    AccountID   string `json:"account_id"`
}
if err := json.Unmarshal(plainCreds, &creds); err != nil {
    slog.Error("epic: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

- [ ] **Step 5: Decrypt in `HandleGOGStatus` (the GOG connection handler)**

In `internal/api/sync.go`, find the GOG status read (around line 1176):

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "auth_url": authURL})
}

var creds struct { ... }
if err := json.Unmarshal([]byte(*row.StorefrontCredentials), &creds); err != nil {
    slog.Error("gog: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

Replace with:

```go
if row.StorefrontCredentials == nil {
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "auth_url": authURL})
}

plainCreds, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
if err != nil {
    slog.Warn("gog: credentials decrypt failed; clearing", "user_id", userID, "err", err)
    _, _ = h.db.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
        userID,
    ).Exec(context.Background())
    return c.JSON(http.StatusOK, map[string]any{"connected": false, "auth_url": authURL})
}

var creds struct {
    Username string `json:"username"`
    UserID   string `json:"user_id"`
}
if err := json.Unmarshal(plainCreds, &creds); err != nil {
    slog.Error("gog: stored credentials are corrupted", "err", err)
    return echo.NewHTTPError(http.StatusInternalServerError, "stored credentials are corrupted")
}
```

- [ ] **Step 6: Run all API tests**

```bash
go test ./internal/api/... -timeout 120s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/api/sync.go internal/api/sync_test.go
git commit -m "feat: decrypt storefront credentials on read in API handlers"
```

---

## Task 5: Encrypt/decrypt credential operations in workers

**Files:**
- Modify: `internal/worker/tasks/sync.go` (all read + write sites)
- Modify: `internal/worker/tasks/sync_test.go` (encrypt all credential fixtures, update worker construction)

- [ ] **Step 1: Update worker construction in `sync_test.go` to pass `testEncrypter`**

In `internal/worker/tasks/sync_test.go`, every `DispatchSyncWorker` construction such as:

```go
w := &tasks.DispatchSyncWorker{DB: testDB, Steam: &fakeSteamAdapter{}, RiverClient: nil}
```

becomes:

```go
w := &tasks.DispatchSyncWorker{DB: testDB, Encrypter: testEncrypter, Steam: &fakeSteamAdapter{}, RiverClient: nil}
```

Do this for all occurrences throughout the file (search for `DispatchSyncWorker{`).

- [ ] **Step 2: Encrypt all `storefront_credentials` fixtures in `sync_test.go`**

Every test that inserts raw JSON into `storefront_credentials` must encrypt it first. For example, replace:

```go
_, _ = testDB.ExecContext(ctx,
    `INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
    uuid.NewString(), userID, `{"web_api_key":"test-key","steam_id":"76561198000000001"}`,
)
```

with:

```go
steamCreds, _ := testEncrypter.Encrypt([]byte(`{"web_api_key":"test-key","steam_id":"76561198000000001"}`))
_, _ = testDB.ExecContext(ctx,
    `INSERT INTO user_sync_configs (id, user_id, storefront, frequency, storefront_credentials) VALUES (?, ?, 'steam', 'daily', ?)`,
    uuid.NewString(), userID, steamCreds,
)
```

Apply the same pattern for every PSN, GOG, and Epic credential insertion in the file. For PSN:

```go
psnCreds, _ := testEncrypter.Encrypt([]byte(`{"npsso_token":"...","is_verified":true,"online_id":"test","account_id":"acc","region":"eu"}`))
```

For GOG:

```go
gogCreds, _ := testEncrypter.Encrypt([]byte(`{"access_token":"tok","refresh_token":"ref","user_id":"u1","username":"testuser"}`))
```

For Epic `epic_legendary_state` — it's stored as a JSON string in a JSONB column:

```go
epicPlain, _ := json.Marshal(map[string]string{"access_token": "tok", "refresh_token": "ref"})
epicCipher, _ := testEncrypter.Encrypt(epicPlain)
epicJSON, _ := json.Marshal(epicCipher) // wrap as JSON string for JSONB
_, _ = testDB.ExecContext(ctx,
    `INSERT INTO user_sync_configs (id, user_id, storefront, frequency, epic_legendary_state, created_at, updated_at) VALUES (?, ?, 'epic', 'manual', ?, now(), now())`,
    uuid.NewString(), userID, string(epicJSON),
)
```

Also update any tests that read raw `storefront_credentials` back from the DB and check its value (e.g., the PSN token-refresh tests that do `Scan(ctx, &rawCreds)` — those tests should decrypt before asserting):

```go
var rawCreds string
_ = testDB.NewRaw(`SELECT storefront_credentials FROM user_sync_configs WHERE id = ?`, configID).Scan(ctx, &rawCreds)
plainCreds, _ := testEncrypter.Decrypt(rawCreds)
var creds map[string]any
_ = json.Unmarshal(plainCreds, &creds)
// now assert on creds fields
```

- [ ] **Step 3: Run worker tests — expect failures from undecrypted reads**

```bash
go test ./internal/worker/tasks/... -run TestDispatchSync -timeout 120s -v 2>&1 | grep -E "FAIL|PASS|panic" | head -20
```

Expected: failures because the workers still call `json.Unmarshal` on ciphertext directly.

- [ ] **Step 4: Decrypt `storefront_credentials` in `DispatchSyncWorker.Work` — steam case**

In `internal/worker/tasks/sync.go`, find the steam case (around line 181). The current code:

```go
case "steam":
    var creds struct {
        WebAPIKey string `json:"web_api_key"`
        SteamID   string `json:"steam_id"`
    }
    if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
        return nil
    }
```

Replace with:

```go
case "steam":
    plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        slog.Warn("dispatch_sync: steam credentials decrypt failed; clearing", "user_id", p.UserID, "err", err)
        _, _ = w.DB.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'steam'`,
            p.UserID,
        ).Exec(context.Background())
        failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
        return nil
    }
    var creds struct {
        WebAPIKey string `json:"web_api_key"`
        SteamID   string `json:"steam_id"`
    }
    if err := json.Unmarshal(plainCreds, &creds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid steam credentials")
        return nil
    }
```

- [ ] **Step 5: Decrypt `storefront_credentials` in the PSN case**

In `internal/worker/tasks/sync.go`, find the PSN case credential read (around line 291):

```go
case "psn":
    var psnCreds struct {
        NpssoToken string `json:"npsso_token"`
        IsVerified bool   `json:"is_verified"`
    }
    if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &psnCreds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
        return nil
    }
```

Replace with:

```go
case "psn":
    plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        slog.Warn("dispatch_sync: psn credentials decrypt failed; clearing", "user_id", p.UserID, "err", err)
        _, _ = w.DB.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
            p.UserID,
        ).Exec(context.Background())
        failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
        return nil
    }
    var psnCreds struct {
        NpssoToken string `json:"npsso_token"`
        IsVerified bool   `json:"is_verified"`
    }
    if err := json.Unmarshal(plainCreds, &psnCreds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid psn credentials")
        return nil
    }
```

- [ ] **Step 6: Encrypt the PSN token-refresh write**

In `internal/worker/tasks/sync.go`, find the PSN token-expired write (around line 375):

```go
newCreds := map[string]any{
    "npsso_token":      psnCreds.NpssoToken,
    "is_verified":      false,
    "token_expired_at": expiredAt,
}
if b, merr := json.Marshal(newCreds); merr == nil {
    s := string(b)
    if _, err := w.DB.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
        s, p.UserID,
    ).Exec(context.Background()); err != nil {
        slog.Error("dispatch_sync: persist expired psn token failed", "err", err, "job_id", p.JobID)
    }
}
```

Replace with:

```go
newCreds := map[string]any{
    "npsso_token":      psnCreds.NpssoToken,
    "is_verified":      false,
    "token_expired_at": expiredAt,
}
if b, merr := json.Marshal(newCreds); merr == nil {
    if enc, encErr := w.Encrypter.Encrypt(b); encErr == nil {
        if _, err := w.DB.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'psn'`,
            enc, p.UserID,
        ).Exec(context.Background()); err != nil {
            slog.Error("dispatch_sync: persist expired psn token failed", "err", err, "job_id", p.JobID)
        }
    } else {
        slog.Error("dispatch_sync: encrypt expired psn token failed", "err", encErr, "job_id", p.JobID)
    }
}
```

- [ ] **Step 7: Decrypt `storefront_credentials` in the GOG case and encrypt the token-refresh write**

In `internal/worker/tasks/sync.go`, find the GOG case (around line 475). Replace the existing credential read:

```go
case "gog":
    ...
    var creds struct { ... }
    if err := json.Unmarshal([]byte(*cfg.StorefrontCredentials), &creds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid gog credentials")
        return nil
    }
```

with:

```go
case "gog":
    if w.GOG == nil {
        failSyncJob(ctx, w.DB, p.JobID, "GOG sync not available")
        return nil
    }

    plainCreds, err := w.Encrypter.Decrypt(*cfg.StorefrontCredentials)
    if err != nil {
        slog.Warn("dispatch_sync: gog credentials decrypt failed; clearing", "user_id", p.UserID, "err", err)
        _, _ = w.DB.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
            p.UserID,
        ).Exec(context.Background())
        failSyncJob(ctx, w.DB, p.JobID, "credentials decrypt failed")
        return nil
    }
    var creds struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        UserID       string `json:"user_id"`
        Username     string `json:"username"`
    }
    if err := json.Unmarshal(plainCreds, &creds); err != nil {
        failSyncJob(ctx, w.DB, p.JobID, "invalid gog credentials")
        return nil
    }
```

Then replace the GOG token-refresh write (around line 503):

```go
if newCredsJSON, merr := json.Marshal(creds); merr == nil {
    if _, err := w.DB.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
        string(newCredsJSON), p.UserID,
    ).Exec(context.Background()); err != nil {
        slog.Error("dispatch_sync: persist refreshed gog token failed", "err", err, "job_id", p.JobID)
    }
}
```

with:

```go
if newCredsJSON, merr := json.Marshal(creds); merr == nil {
    if enc, encErr := w.Encrypter.Encrypt(newCredsJSON); encErr == nil {
        if _, err := w.DB.NewRaw(
            `UPDATE user_sync_configs SET storefront_credentials = ?, updated_at = now() WHERE user_id = ? AND storefront = 'gog'`,
            enc, p.UserID,
        ).Exec(context.Background()); err != nil {
            slog.Error("dispatch_sync: persist refreshed gog token failed", "err", err, "job_id", p.JobID)
        }
    } else {
        slog.Error("dispatch_sync: encrypt refreshed gog token failed", "err", encErr, "job_id", p.JobID)
    }
}
```

- [ ] **Step 8: Decrypt and encrypt `epic_legendary_state` in `EpicClientAdapter`**

In `internal/worker/tasks/sync.go`, find `EpicClientAdapter.GetLibrary` (around line 75). Replace the state read:

```go
var snapshotJSON []byte
if err := a.DB.NewRaw(
    `SELECT epic_legendary_state FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
    userID,
).Scan(ctx, &snapshotJSON); err != nil || len(snapshotJSON) == 0 {
    return fmt.Errorf("epic: no legendary state found for user (not connected)")
}
var snapshot map[string]string
if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
    return fmt.Errorf("epic: unmarshal legendary state: %w", err)
}
```

with:

```go
var rawStateJSON []byte
if err := a.DB.NewRaw(
    `SELECT epic_legendary_state FROM user_sync_configs WHERE user_id = ? AND storefront = 'epic'`,
    userID,
).Scan(ctx, &rawStateJSON); err != nil || len(rawStateJSON) == 0 {
    return fmt.Errorf("epic: no legendary state found for user (not connected)")
}
// rawStateJSON is a JSONB column storing a JSON string: "enc:v1:base64..."
var ciphertextStr string
if err := json.Unmarshal(rawStateJSON, &ciphertextStr); err != nil {
    return fmt.Errorf("epic: unmarshal legendary state wrapper: %w", err)
}
plainState, err := a.Encrypter.Decrypt(ciphertextStr)
if err != nil {
    slog.Warn("epic: legendary state decrypt failed; clearing", "user_id", userID, "err", err)
    _, _ = a.DB.NewRaw(
        `UPDATE user_sync_configs SET epic_legendary_state = NULL, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
        userID,
    ).Exec(context.Background())
    return fmt.Errorf("epic: legendary state decrypt failed (credentials cleared)")
}
var snapshot map[string]string
if err := json.Unmarshal(plainState, &snapshot); err != nil {
    return fmt.Errorf("epic: unmarshal legendary state: %w", err)
}
```

Then replace the state write (around line 107):

```go
newJSON, _ := json.Marshal(newSnapshot)
if _, err := a.DB.NewRaw(
    `UPDATE user_sync_configs SET epic_legendary_state = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
    string(newJSON), userID,
).Exec(context.Background()); err != nil {
    slog.Error("epic: persist updated snapshot failed", "user_id", userID, "err", err)
}
```

with:

```go
newPlainJSON, _ := json.Marshal(newSnapshot)
newCiphertext, encErr := a.Encrypter.Encrypt(newPlainJSON)
if encErr != nil {
    slog.Error("epic: encrypt updated snapshot failed", "user_id", userID, "err", encErr)
} else {
    // Wrap ciphertext as a JSON string for storage in the JSONB column.
    newStateJSON, _ := json.Marshal(newCiphertext)
    if _, err := a.DB.NewRaw(
        `UPDATE user_sync_configs SET epic_legendary_state = ?, updated_at = now() WHERE user_id = ? AND storefront = 'epic'`,
        string(newStateJSON), userID,
    ).Exec(context.Background()); err != nil {
        slog.Error("epic: persist updated snapshot failed", "user_id", userID, "err", err)
    }
}
```

- [ ] **Step 9: Run all worker tests**

```bash
go test ./internal/worker/tasks/... -timeout 300s -v 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 10: Run the full test suite**

```bash
go test -timeout 600s ./...
```

Expected: all tests pass. Zero failures.

- [ ] **Step 11: Commit**

```bash
git add internal/worker/tasks/sync.go internal/worker/tasks/sync_test.go internal/worker/tasks/main_test.go
git commit -m "feat: encrypt/decrypt storefront credentials in sync workers"
```

---

## Task 6: Operator documentation updates

**Files:** `.env.example`, `deploy/docker/.env.example`, `deploy/docker/docker-compose.yml`, `devenv.nix`, `deploy/helm/values.yaml`, `deploy/helm/templates/_helpers.tpl`, `deploy/helm/templates/credentials-secret.yaml`, `deploy/helm/templates/NOTES.txt`, `nix/module.nix`, `README.md`, `CLAUDE.md`

- [ ] **Step 1: Update `.env.example`**

After the `SECRET_KEY` line, add:

```
# DB encryption key — generate: openssl rand -base64 32
DB_ENCRYPTION_KEY=your-db-encryption-key-here
```

- [ ] **Step 2: Update `deploy/docker/.env.example`**

Same addition after the `SECRET_KEY` line.

- [ ] **Step 3: Update `deploy/docker/docker-compose.yml`**

After `SECRET_KEY: ${SECRET_KEY:?SECRET_KEY is required}` (line ~44), add:

```yaml
      DB_ENCRYPTION_KEY: ${DB_ENCRYPTION_KEY:?DB_ENCRYPTION_KEY is required}
```

- [ ] **Step 4: Update `devenv.nix`**

After `SECRET_KEY = "dev-only-insecure-secret-do-not-use-in-production";` (line ~8), add:

```nix
DB_ENCRYPTION_KEY = "dev-only-insecure-db-key-do-not-use-in-production";
```

- [ ] **Step 5: Update Helm `values.yaml`**

After the `secretKey: "change-me-in-production"` entry (line ~26), add:

```yaml
  dbEncryptionKey: "change-me-in-production"

  # Supply DB_ENCRYPTION_KEY from an existing Secret instead of inline.
  # If set, dbEncryptionKey is ignored.
  dbEncryptionKeyFrom:
    name: ""
    key: ""
```

In the deployment env block (~L281), after the `SECRET_KEY` block, add:

```yaml
          DB_ENCRYPTION_KEY:
            valueFrom:
              secretKeyRef:
                name: '{{ include "nexorious.dbEncryptionKeySecretName" . }}'
                key: '{{ include "nexorious.dbEncryptionKeySecretKey" . }}'
```

Apply the same addition in the job pod env block (~L361).

- [ ] **Step 6: Update Helm `_helpers.tpl`**

In `nexorious.validateValues`, add `dbEncryptionKeyFrom` to the `$fromFields` list:

```
  (dict "label" "dbEncryptionKeyFrom" "from" .Values.nexorious.dbEncryptionKeyFrom)
```

After the `$secretKeyFromConfigured` check block, add:

```
{{- $dbEncKeyFromConfigured := not (empty (dig "name" "" (default dict .Values.nexorious.dbEncryptionKeyFrom))) -}}
{{- if not $dbEncKeyFromConfigured -}}
  {{- if eq .Values.nexorious.dbEncryptionKey "change-me-in-production" }}
    {{- fail "nexorious.dbEncryptionKey must be set to a secure random value" }}
  {{- end }}
{{- end }}
```

At the end of the file, add the two new helper definitions (parallel to the existing `nexorious.secretKeySecretName` / `nexorious.secretKeySecretKey` helpers at ~L209):

```
{{- define "nexorious.dbEncryptionKeySecretName" -}}
{{- if and .Values.nexorious.dbEncryptionKeyFrom .Values.nexorious.dbEncryptionKeyFrom.name -}}
{{- .Values.nexorious.dbEncryptionKeyFrom.name -}}
{{- else -}}
{{ include "nexorious.fullname" . }}-credentials
{{- end -}}
{{- end }}

{{- define "nexorious.dbEncryptionKeySecretKey" -}}
{{- if and .Values.nexorious.dbEncryptionKeyFrom .Values.nexorious.dbEncryptionKeyFrom.key -}}
{{- .Values.nexorious.dbEncryptionKeyFrom.key -}}
{{- else -}}
DB_ENCRYPTION_KEY
{{- end -}}
{{- end }}
```

Also update the `credentialsSecretNeeded` helper to include `dbEncryptionKey`:

```
{{- define "nexorious.credentialsSecretNeeded" -}}
{{- $secretKeyNeeded := not (and .Values.nexorious.secretKeyFrom .Values.nexorious.secretKeyFrom.name) -}}
{{- $dbEncKeyNeeded := not (and .Values.nexorious.dbEncryptionKeyFrom .Values.nexorious.dbEncryptionKeyFrom.name) -}}
... (update the logic to OR the two conditions)
```

Check the existing helper definition and mirror the pattern exactly.

- [ ] **Step 7: Update Helm `credentials-secret.yaml`**

In the `stringData` block, after the `SECRET_KEY` entry (line ~27), add:

```yaml
{{- if not $dbEncryptionKeyFrom.name }}
  DB_ENCRYPTION_KEY: {{ .Values.nexorious.dbEncryptionKey | quote }}
{{- end }}
```

Look at the file structure first and mirror the existing `SECRET_KEY` conditional pattern exactly.

- [ ] **Step 8: Update Helm `NOTES.txt`**

After the `{{- if eq .Values.nexorious.secretKey "change-me-in-production" }}` warning block, add:

```
{{- if eq .Values.nexorious.dbEncryptionKey "change-me-in-production" }}

WARNING: nexorious.dbEncryptionKey is set to the default placeholder value.
   Set it to a secure random value:
   --set nexorious.dbEncryptionKey=$(openssl rand -base64 32)
{{- end }}
```

- [ ] **Step 9: Update `nix/module.nix`**

In the header docstring (line ~18), after the `SECRET_KEY` reference, add:
```
#   DB_ENCRYPTION_KEY=<random-secret-used-for-at-rest-credential-encryption>
```

In the `environmentFile` documentation (line ~132), after the `SECRET_KEY` description, add:
```
        - `DB_ENCRYPTION_KEY` — random secret for at-rest credential encryption.
          Generate with `openssl rand -base64 32`.
```

- [ ] **Step 10: Update `README.md`**

Find line ~69 (the `cp .env.example .env` note) and update the comment:

```
cp .env.example .env   # fill in SECRET_KEY, DB_ENCRYPTION_KEY, IGDB_CLIENT_ID, IGDB_CLIENT_SECRET, POSTGRES_PASSWORD
```

Find the env-var example block (~line 94):

```
SECRET_KEY=your-secret-key-here           # generate: openssl rand -hex 32
```

Add after it:

```
DB_ENCRYPTION_KEY=your-db-encryption-key  # generate: openssl rand -base64 32
```

Find the checklist (~line 109):

```
- [ ] `SECRET_KEY` set to a cryptographically random value
```

Add after it:

```
- [ ] `DB_ENCRYPTION_KEY` set to a cryptographically random value
```

- [ ] **Step 11: Update `CLAUDE.md`**

Find the initial-setup snippet (around line 54):

```bash
export SECRET_KEY="<random-secret>"   # required; used for JWT signing and encryption
```

Replace with:

```bash
export SECRET_KEY="<random-secret>"         # required; used for JWT signing
export DB_ENCRYPTION_KEY="<random-secret>"  # required; generate: openssl rand -base64 32
```

- [ ] **Step 12: Final test run and lint**

```bash
go test -timeout 600s ./...
golangci-lint run
```

Expected: all tests pass, zero lint errors.

- [ ] **Step 13: Commit**

```bash
git add .env.example deploy/docker/.env.example deploy/docker/docker-compose.yml \
    devenv.nix deploy/helm/ nix/module.nix README.md CLAUDE.md
git commit -m "docs: add DB_ENCRYPTION_KEY to all operator-facing surfaces"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| `internal/crypto` package (AES-256-GCM, `enc:v1:` prefix, tests) | Task 1 |
| `DBEncryptionKey` config field, `SecretKey` comment fix | Task 2 |
| Fail-fast startup validation | Task 2 (serve.go) |
| `Encrypter` in `SyncHandler`, `DispatchSyncWorker`, `EpicClientAdapter` | Task 2 |
| Steam verify write path encrypts | Task 3 |
| PSN configure write path encrypts | Task 3 |
| Epic connect write path encrypts (both fields) | Task 3 |
| GOG connect write path encrypts | Task 3 |
| PSN status read path decrypts, clears on failure | Task 4 |
| Epic connection read path decrypts, clears on failure | Task 4 |
| GOG status read path decrypts, clears on failure | Task 4 |
| DispatchSyncWorker steam/psn/gog credential decrypts | Task 5 |
| PSN token-refresh write encrypts | Task 5 |
| GOG token-refresh write encrypts | Task 5 |
| EpicClientAdapter legendary state read decrypts (JSON-string wrapper) | Task 5 |
| EpicClientAdapter legendary state write encrypts (JSON-string wrapper) | Task 5 |
| Test fixtures use real encrypter with fixed test key | Tasks 3, 4, 5 |
| Operator docs: .env.example, docker-compose, devenv, Helm, Nix, README, CLAUDE.md | Task 6 |
