# Credential Encryption at Rest (Issue #599)

**Date:** 2026-05-22
**Status:** Draft
**Blocks:** #514 (notifications — will reuse `internal/crypto` and `DB_ENCRYPTION_KEY`)

## Background

`internal/config/config.go` documents `SecretKey` as used for "JWT signing and credential encryption," but no encryption code exists. All storefront credentials (`storefront_credentials`, `epic_legendary_state`) are stored as plaintext JSON. This issue adds real at-rest encryption.

## Goals

- Encrypt all storefront credentials at rest using AES-256-GCM.
- Introduce a separate `DB_ENCRYPTION_KEY` env var scoped to DB encryption (one key, one purpose).
- Leave `SECRET_KEY` scoped to JWT signing only — update the misleading comment.
- No schema changes — the encryption layer is purely application-side.
- Issue #514 (notification channel URLs) will reuse the same primitive.

## Out of Scope

- Key rotation (`OLD_DB_ENCRYPTION_KEY`).
- Encrypting non-credential fields (`preferences`, etc.).
- Migration consolidation.

## Design

### Key Management

Two separate required env vars:

| Var | Purpose | Generation |
|---|---|---|
| `SECRET_KEY` | JWT signing | `openssl rand -hex 32` |
| `DB_ENCRYPTION_KEY` | At-rest credential encryption | `openssl rand -base64 32` |

Rationale: different blast radius, different rotation cadence. A compromised JWT key enables token forgery; a compromised DB key exposes credentials from any backup. Keeping them separate lets each be rotated independently.

### `internal/crypto` Package

New package. Single exported type:

```go
type Encrypter struct { key [32]byte }

// NewEncrypter validates that rawKey is at least 32 bytes (security floor on
// raw entropy), derives a 32-byte AES-256 key via SHA-256(rawKey), and returns
// the Encrypter.
func NewEncrypter(rawKey string) (*Encrypter, error)

// Encrypt returns "enc:v1:" + base64(nonce || ciphertext || GCM tag).
// Uses AES-256-GCM with a random 12-byte nonce per call.
func (e *Encrypter) Encrypt(plaintext []byte) (string, error)

// Decrypt strips the "enc:v1:" prefix (errors if absent), base64-decodes,
// splits out the 12-byte nonce, and decrypts. Returns error on wrong key,
// tampered data, or missing prefix.
func (e *Encrypter) Decrypt(ciphertext string) ([]byte, error)
```

The `enc:v1:` sentinel prefix lets callers detect unencrypted rows defensively.

**Tests** (`internal/crypto/crypto_test.go`):
- Round-trip: `Decrypt(Encrypt(plain)) == plain`
- Tamper detection: modifying ciphertext bytes → error
- Wrong key: different `Encrypter` → error
- Missing prefix → error
- Empty input → error

### Config

`internal/config/config.go`:
- Add `DBEncryptionKey string \`env:"DB_ENCRYPTION_KEY,required"\`` in the Security section.
- Update `SecretKey` comment to `// SecretKey is used for JWT signing.`

### Startup Wiring

`cmd/nexorious/serve.go`, immediately after config load:

```go
encrypter, err := crypto.NewEncrypter(cfg.DBEncryptionKey)
if err != nil {
    slog.Error("invalid DB_ENCRYPTION_KEY", "err", err)
    os.Exit(1)
}
```

The `encrypter` is passed into:
- `api.NewSyncHandler(db, riverClient, steam, psn, epic, gog, encrypter)` — stored as `encrypter *crypto.Encrypter` field
- `&tasks.DispatchSyncWorker{..., Encrypter: encrypter}`
- `&tasks.ProcessSyncItemWorker{..., Encrypter: encrypter}`

(The same `encrypter` instance will be reused by the notifications handler in #514.)

### Write Paths

All credential-write sites in `internal/api/sync.go` follow this pattern:

```go
jsonBytes, err := json.Marshal(creds)
// handle err
ciphertext, err := h.encrypter.Encrypt(jsonBytes)
// handle err
// store ciphertext string
```

Affected sites:
- Steam verify (`~line 491`)
- PSN configure (`~line 554`)
- Epic connect (`~line 667`) — both `storefront_credentials` and `epic_legendary_state`
- GOG connect (`~line 1128`)

The PSN token-refresh write in `internal/worker/tasks/sync.go` (`~line 386`) also encrypts.
The GOG token-refresh write in `internal/worker/tasks/sync.go` (`~line 503`) also encrypts.
The Epic `epic_legendary_state` update in `internal/worker/tasks/sync.go` (`~line 107`) also encrypts.

### Read Paths

All credential-read sites call `Decrypt` before `json.Unmarshal`:

```go
plaintext, err := h.encrypter.Decrypt(*row.StorefrontCredentials)
if err != nil {
    slog.Warn("credentials decrypt failed; clearing", "user_id", uid, "storefront", sf, "err", err)
    _, _ = db.NewRaw(
        `UPDATE user_sync_configs SET storefront_credentials = NULL, updated_at = now() WHERE user_id = ? AND storefront = ?`,
        uid, sf,
    ).Exec(ctx)
    // return "not configured" response (same as NULL credentials)
    return c.JSON(http.StatusOK, notConfiguredResponse)
}
if err := json.Unmarshal(plaintext, &creds); err != nil { ... }
```

Affected sites in `internal/api/sync.go`:
- Steam status (`~line 598`)
- PSN status (`~line 742`)
- GOG status (`~line 1184`)

Affected sites in `internal/worker/tasks/sync.go`:
- `DispatchSyncWorker.Work` credential read (`~line 185`) — all storefronts
- PSN-specific creds read (`~line 296`)
- GOG-specific creds read (`~line 487`)
- Epic `epic_legendary_state` read (`~line 82`)

On decrypt failure: NULL the credential column, log a warning, treat as "not configured." This handles the wrong-key-after-restore scenario gracefully without any schema changes.

### Test Strategy

Sync tests (`internal/worker/tasks/sync_test.go`, `internal/api/sync_test.go`) that insert credentials directly into the DB must produce ciphertext fixtures. A fixed test key constant is defined in the test package:

```go
const testEncryptionKey = "test-db-encryption-key-32-bytes!!"

// in TestMain or test helpers:
enc, _ := crypto.NewEncrypter(testEncryptionKey)
credsCiphertext, _ := enc.Encrypt([]byte(`{"api_key":"test-key","steam_id":"123"}`))
// insert credsCiphertext into storefront_credentials
```

Tests assert on decrypted values, not raw DB bytes (except tests that specifically verify the DB-layer behavior like token refresh writes).

### Operator-Facing Updates

Every surface that documents or wires `SECRET_KEY` gets a parallel entry for `DB_ENCRYPTION_KEY`:

| File | Change |
|---|---|
| `.env.example` | Add `DB_ENCRYPTION_KEY=your-db-encryption-key-here` with generation hint |
| `deploy/docker/.env.example` | Same |
| `deploy/docker/docker-compose.yml` | Add `DB_ENCRYPTION_KEY: ${DB_ENCRYPTION_KEY:?DB_ENCRYPTION_KEY is required}` |
| `devenv.nix` | Add dev-only placeholder mirroring existing `SECRET_KEY` line |
| `deploy/helm/values.yaml` | Add `nexorious.dbEncryptionKey` / `nexorious.dbEncryptionKeyFrom`; add `DB_ENCRYPTION_KEY` to both env blocks (~L281 and ~L361) |
| `deploy/helm/templates/_helpers.tpl` | Clone `secretKey` placeholder check and `secretKeyFrom` validation; add `nexorious.dbEncryptionKeySecretName` and `nexorious.dbEncryptionKeySecretKey` helpers |
| `deploy/helm/templates/credentials-secret.yaml` | Add `DB_ENCRYPTION_KEY` entry to `stringData` |
| `deploy/helm/templates/NOTES.txt` | Add default-placeholder warning parallel to `secretKey` warning |
| `nix/module.nix` | Add `DB_ENCRYPTION_KEY` to header docstring and `environmentFile` docs |
| `README.md` | Add to env-var checklist and example block |
| `CLAUDE.md` | Add to initial-setup snippet |

## Testing

- `internal/crypto/crypto_test.go`: unit tests for the crypto primitive (round-trip, tamper, wrong key, bad prefix)
- Existing sync tests: updated to use `crypto.NewEncrypter(testEncryptionKey)` to produce fixture ciphertext
- `go test ./...` must pass; `npm run check && npm run knip && npm run test` unchanged (no frontend changes in this issue)
