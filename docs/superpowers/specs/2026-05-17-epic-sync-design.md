# Epic Games Store Sync Design

**Date:** 2026-05-17  
**Status:** Draft

## Problem

The Go backend has no Epic Games Store sync. The previous Python backend implemented this via the `legendary` Python library, which is not usable from Go. There is no official Epic API for listing a user's owned games, and implementing the undocumented API directly would require maintaining brittle protocol knowledge. Epic does not offer OAuth redirect URIs to non-partner applications.

## Goal

Implement Epic Games Store library sync using the Legendary CLI as a subprocess. Legendary owns all Epic protocol knowledge — authentication, token refresh, library listing, and API compatibility. The Go backend acts purely as an orchestrator: it invokes legendary with an isolated config directory, captures whatever state legendary writes, persists it in the database, and restores it before each subsequent operation.

Epic sync is disabled (with a clear error) when the required `LEGENDARY_WORK_DIR` environment variable is not configured.

## Approach

Legendary is called as a subprocess with `XDG_CONFIG_HOME` set to a per-user working directory under `LEGENDARY_WORK_DIR`. Legendary writes its config and metadata files there. After each operation, the Go backend snapshots those files and stores them in the database. Before each subsequent operation, the snapshot is restored to the per-user directory. This makes legendary's state portable across pod restarts without requiring persistent volume storage for config data.

The metadata cache (`metadata/`) is included in the snapshot because without it legendary re-fetches metadata for every owned game from Epic's catalog API on each sync — potentially hundreds of HTTP calls. At ~1.4 MB for a typical library of ~500 games, this is well within PostgreSQL's storage comfort zone.

## Configuration

### New environment variable: `LEGENDARY_WORK_DIR`

Type: string (absolute path). If empty, Epic sync is disabled:
- `GET /api/sync/epic/connection` returns `{"connected": false, "disabled": true, "reason": "legendary_not_configured"}`
- `POST /api/sync/epic/connect` returns 503 with a descriptive error
- Attempting to run an Epic sync job logs a clear error and marks the job failed immediately

The directory must be writable. In Kubernetes this is an `emptyDir` volume. In devenv it is set in `enterShell`.

### devenv.nix changes

Two additions:
1. `legendary-gl` added to `packages`
2. `export LEGENDARY_WORK_DIR="$DEVENV_ROOT/.legendary-work"` added to `enterShell`

The `.legendary-work/` directory is gitignored.

### Kubernetes Helm chart

An `emptyDir` volume is added to the nexorious controller and mounted at `/legendary-work`. The `LEGENDARY_WORK_DIR` env var is set to `/legendary-work` in the main container. The migrate initContainer does not need it.

## Per-user working directory

For a user with ID `{user_id}`, legendary's config directory is:

```
{LEGENDARY_WORK_DIR}/{user_id}/legendary/
```

This is achieved by setting `XDG_CONFIG_HOME={LEGENDARY_WORK_DIR}/{user_id}` in the subprocess environment. Legendary appends `/legendary/` itself.

The directory is created on first use (connect) and removed on disconnect.

## State snapshot

After each legendary invocation the Go backend snapshots the legendary config dir. The snapshot is a JSON map of relative path → file content (as a string, since all legendary files are JSON or INI text):

```json
{
  "user.json": "{...}",
  "config.ini": "[Legendary]\n...",
  "version.json": "{...}",
  "metadata/GameName.json": "{...}",
  ...
}
```

Files excluded from the snapshot:
- `*.lock` — transient lock files
- `tmp/` — legendary's own temp dir
- `manifests/` — download manifests, irrelevant for library listing

The snapshot is stored as a `jsonb` column `epic_legendary_state` on the `user_sync_config` row for the epic platform. The existing `platform_credentials` column is unused for Epic (credentials are embedded in `user.json` within the snapshot).

### Restore

Before running any legendary subprocess, the Go backend:
1. Creates `{LEGENDARY_WORK_DIR}/{user_id}/legendary/` (and any `metadata/` subdirs) if they don't exist
2. Writes each file from the snapshot to its relative path under that directory
3. Runs legendary

After legendary exits:
1. Reads all files from the directory (applying the same exclusion rules)
2. Updates the snapshot in the DB

## New service package: `internal/services/epic`

**File:** `internal/services/epic/client.go`

```go
type Client struct {
    workDir string // LEGENDARY_WORK_DIR value
}

func NewClient(workDir string) *Client

// Authenticate runs `legendary auth --code <code>` in a fresh per-user dir.
// Returns the account display name and account ID extracted from user.json.
func (c *Client) Authenticate(ctx context.Context, userID, authCode string) (*EpicAccountInfo, error)

// GetAccountInfo reads user.json from the per-user dir and extracts account info.
// Does not invoke legendary.
func (c *Client) GetAccountInfo(ctx context.Context, userID string) (*EpicAccountInfo, error)

// GetLibrary runs `legendary list --json` with the per-user dir as XDG_CONFIG_HOME.
// Streams parsed entries to the onBatch callback (same pattern as PSN).
func (c *Client) GetLibrary(ctx context.Context, userID string, onBatch func([]ExternalLibraryEntry) error) error

// Cleanup removes the per-user working directory.
func (c *Client) Cleanup(ctx context.Context, userID string) error

type EpicAccountInfo struct {
    DisplayName string
    AccountID   string
}

type ExternalLibraryEntry struct {
    ExternalID      string // legendary app_name
    Title           string
    Namespace       string
    CatalogItemID   string
    OwnershipStatus string // always "owned" for epic
}
```

### State helpers (unexported)

```go
func (c *Client) restoreSnapshot(userID string, snapshot map[string]string) error
func (c *Client) captureSnapshot(userID string) (map[string]string, error)
func (c *Client) runLegendary(ctx context.Context, userID string, args ...string) ([]byte, error)
```

`runLegendary` sets `XDG_CONFIG_HOME`, captures stdout and stderr, and returns an error on non-zero exit. It does not restore or capture snapshots — callers do that.

## DB schema changes

One new column on `user_sync_config`:

```sql
ALTER TABLE user_sync_config
    ADD COLUMN epic_legendary_state jsonb;
```

Migration file: `internal/db/migrations/20260517000001_epic_legendary_state.up.sql`

`platform_credentials` for the epic platform row contains the auth code used during initial connect (for audit), or is left empty — the real credentials live inside the snapshot's `user.json`.

## Auth flow (connect)

The user connects their Epic account via the existing sync connection UI pattern:

1. `GET /api/sync/epic/auth-url` — returns the auth URL the user should visit:
   ```
   https://www.epicgames.com/id/api/redirect?clientId=34a02cf8f4414e29b15921876da36f9a&responseType=code
   ```
2. User visits the URL in their browser, logs in, and copies the `authorizationCode` value from the JSON response page.
3. `POST /api/sync/epic/connect` with body `{"auth_code": "<code>"}`:
   - Calls `client.Authenticate(ctx, userID, authCode)`:
     - Creates a fresh per-user working dir
     - Runs `legendary auth --code <code>`
     - Reads `user.json` to extract `displayName` and `account_id`
   - Captures the snapshot and stores it in `epic_legendary_state`
   - Creates/updates the `user_sync_config` row with `platform = "epic"`
   - Returns `{"display_name": "...", "account_id": "..."}`
4. `POST /api/sync/epic/disconnect`:
   - Deletes the `user_sync_config` row (or clears `epic_legendary_state`)
   - Calls `client.Cleanup` to remove the working dir

## Sync flow

`DispatchSyncWorker` gains an `Epic EpicLibraryAdapter` field alongside the existing `Steam` and `PSN` fields.

The Epic sync branch in `Work` calls `w.Epic.GetLibrary(ctx, userID, batchCallback)` and then continues with the existing dispatch logic (upsert `external_games`, dispatch `ProcessSyncItemArgs` jobs). The adapter is responsible for snapshot management — `DispatchSyncWorker` does not touch the filesystem.

### `EpicLibraryAdapter` interface

```go
type EpicLibraryAdapter interface {
    GetLibrary(
        ctx     context.Context,
        userID  string,
        onBatch func([]epicsvc.ExternalLibraryEntry) error,
    ) error
}
```

The concrete implementation `EpicClientAdapter` (in `internal/worker/tasks/sync.go` or a nearby file) holds a `*epic.Client` and a `*bun.DB`. Its `GetLibrary`:

1. Loads `epic_legendary_state` from `user_sync_config` for the given user
2. Calls `client.restoreSnapshot(userID, snapshot)` to write files to disk
3. Calls `client.GetLibrary(ctx, userID, onBatch)` — invokes `legendary list --json` and streams parsed entries to the callback
4. Calls `client.captureSnapshot(userID)` to read files back from disk
5. Persists the updated snapshot to `epic_legendary_state` in the DB
6. Returns any error from step 3

## Error handling

| Scenario | Behaviour |
|---|---|
| `LEGENDARY_WORK_DIR` not set | Connect returns 503; sync job marked failed immediately with message "Epic sync not configured (LEGENDARY_WORK_DIR unset)" |
| `legendary` binary not in PATH | Connect returns 503; sync job marked failed with message "legendary not found in PATH" |
| `legendary auth` exits non-zero | Connect returns 400 with legendary's stderr as error detail |
| `legendary list` exits non-zero | Sync job marked failed; legendary stderr logged and stored as job error message |
| `user.json` missing after auth | Connect returns 500; treat as auth failure |
| Snapshot restore fails (disk full, permission denied) | Sync job marked failed with OS error detail |

## `legendary list` output format

Legendary outputs a JSON array when called with `--json`. Each element has at minimum:

```json
{
  "app_name": "AppNameString",
  "app_title": "Human Readable Title",
  "namespace": "catalogNamespace",
  "catalog_item_id": "hexstring"
}
```

`ExternalID` in the `ExternalLibraryEntry` is set to `app_name`. This is the stable identifier Epic uses for a game. `Title` is `app_title`.

Free games (Epic's weekly giveaways) and non-game content appear in the output. DLC filtering — legendary may include a field such as `main_game_appname` for DLC entries — should be confirmed against real `legendary list --json` output during implementation and the filter applied accordingly. Entries identified as DLC are skipped.

## Testing

The `EpicLibraryAdapter` interface is mockable in the same way as `PSNLibraryAdapter`. Tests for `DispatchSyncWorker` with Epic storefront use a mock that calls `onBatch` once with a fixed set of entries.

The `epic.Client` itself is not unit-tested at the subprocess level — verifying legendary subprocess invocation is an integration concern. The snapshot capture/restore helpers (`restoreSnapshot`, `captureSnapshot`) are tested with a real temporary directory.

## Out of scope

- `legendary list --force-refresh` flag — the default behaviour (using cached metadata where available) is sufficient
- Playtime data — legendary does not provide playtime; this field will be 0 for Epic games
- DLC as separate entries — filtered out at the listing stage
- Unreal Engine Marketplace content (`--include-ue`) — not included
- Multi-user concurrency hardening — per-user directories provide natural isolation; no additional locking needed at this stage
