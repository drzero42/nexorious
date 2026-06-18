# Single-owner user-game mutations (#1056)

Status: **design** · Issue: [#1056](https://github.com/drzero42/nexorious/issues/1056) · Epic: [#1055](https://github.com/drzero42/nexorious/issues/1055) · Date: 2026-06-17

## Context

Epic #1055 ("one canonical path per domain outcome") identified that *acquiring a
game* — insert/merge platform rows, clear the wishlist flag, promote
`not_started → in_progress` when the game has playtime, reconcile tags — is
assembled by hand at 16+ call sites across `internal/api/user_games.go`,
`internal/worker/tasks/sync.go`, `import_item.go`, and `import_pipeline.go`.
Those hand-chained assemblies have already drifted: #1061 (imported played
games stuck at `not_started`) was a real shipping bug caused by import skipping
the promote step that sync ran.

#1061 has since been **point-fixed** (PR #1063 added `PromoteToInProgressIfPlayed`
to both import paths) and #1054's tag-assignment endpoint has **landed**
(PR #1065 added `internal/usergame.ResolveOrCreateTag` / `ReplaceTags`). #1056 is
therefore no longer about *introducing* those behaviours — it is about making
the class of bug **structurally impossible** by giving every mutation outcome a
single owner that runs its full invariant set atomically, so no caller can skip
a step because there is only one path.

## Goals

- One exported operation per user-game mutation outcome in `internal/usergame`,
  each owning its complete invariant/side-effect set and executing it atomically.
- Every in-process caller (REST handlers, sync worker, both import workers)
  routes through these operations; no outcome is reassembled at a call site.
- The loose helpers `ClearWishlistOnAcquire`, `PromoteToInProgressIfPlayed`,
  `RemoveFromPoolsIfFinished` become **unexported** and are called only from
  inside the operations.
- Typed errors at the package boundary; `sql.ErrNoRows` → not-found preserved.
- Deduplicate `isDuplicateKeyError` (copied 4×) into one shared classifier.

## Non-goals

- **Reads / projections** — list, get, stats, filter-options. That is sibling
  issue #1062. Reads are extracted only if they ride along trivially.
- No `Ops`/service struct: operations are free functions (see Decisions).
- No ORM/filter-package rewrite.
- No change to REST routes, request/response JSON shapes, or HTTP status codes
  beyond what the convergence decisions below explicitly call out.

## Decisions (locked in brainstorming)

1. **Free functions, not a service struct.** Every consumer already holds a
   `*bun.DB`; the existing `usergame` helpers are already free functions; the
   codebase's own event (`notify.Emit(ctx, db, …)`) and metric
   (`observability.RecordSyncOutcome(ctx, …)`) APIs are free functions /
   package globals. Operations need only the `db`, so a struct would bundle a
   dependency the codebase deliberately doesn't inject. Public operations take
   `*bun.DB` (concrete, so they own `RunInTx`); the now-internal helpers keep
   `bun.IDB` so they run inside the operation's transaction.

2. **Operations own their transaction.** Each operation opens its own
   `db.RunInTx(...)` and runs its full invariant set atomically. This fixes
   today's non-atomic create path (`user_games.go:477-486` runs the platform
   insert + clear-wishlist + promote on `h.db`, no transaction) and standardizes
   away the two manual `BeginTx`/`Commit` sites (`user_games.go:865`, `1305`).

3. **Full mutation surface, one PR.** All operations below land together; reads
   are untouched.

4. **`IsUniqueViolation` lives in `internal/db`** (neutral home, package `db`),
   not `usergame`, so `pools.go` and `notifications.go` need not depend on
   `usergame`. All four `isDuplicateKeyError` copies
   (`user_games.go`, `tags.go`, `pools.go`, `notifications.go`) are deleted and
   route to it.

## Package shape

`internal/usergame` grows from loose helpers into the operation owner. Public
operations (free functions, `*bun.DB`, own-transaction):

```go
// Acquire-family (insert/merge platforms + clear-wishlist + promote + tags)
func Acquire(ctx, db, AcquireParams) (Result, error)
func AddPlatform(ctx, db, AddPlatformParams) (Result, error)
func AddPlatformBulk(ctx, db, BulkAddPlatformParams) (int, error)
func MoveToLibrary(ctx, db, MoveParams) (Result, error)

// Platform ops
func UpdatePlatform(ctx, db, UpdatePlatformParams) error   // + promote tail
func RemovePlatform(ctx, db, RemovePlatformParams) error
func RemovePlatformBulk(ctx, db, BulkRemovePlatformParams) (int, error)

// Status / fields / progress
func UpdateFields(ctx, db, UpdateFieldsParams) error        // rating/loved/notes/status (+ pools tail)
func SetPlayStatusBulk(ctx, db, BulkStatusParams) (int, error) // + pools tail
func RecordProgress(ctx, db, ProgressParams) error          // hours/progress + promote

// Tags (already exist; promoted to operations)
func ReplaceTags(ctx, db, ...) error                        // full replace (API)
func ResolveOrCreateTag(ctx, db, ...) (string, error)

// Lifecycle
func Delete(ctx, db, DeleteParams) error
func DeleteBulk(ctx, db, BulkDeleteParams) (int, error)
func ClearLibrary(ctx, db, userID string) (int, error)
```

Internal (unexported, `bun.IDB`, called only inside the above):
`clearWishlistOnAcquire`, `promoteToInProgressIfPlayed`, `removeFromPoolsIfFinished`.

`Result` carries enough for a worker to emit its job-scoped side-effects:

```go
type Result struct {
    UserGameID      string
    Created         bool                 // user_games row newly inserted
    PlatformChanges []PlatformChange      // per-platform outcome
}
type PlatformChange struct {
    Platform, Storefront string
    Created              bool
    OwnershipUpgraded    bool
    OldOwnership, NewOwnership *string
}
```

## The `Acquire` keystone

`Acquire` is the idempotent upsert all acquire paths route through. It takes a
**mode** to capture the one genuine semantic fork between callers:

```go
type AcquireMode int
const (
    ModeCreate AcquireMode = iota // API create: user_game must NOT exist → ErrConflict if it does
    ModeUpsert                    // sync/import: find-or-create, idempotent, never conflicts
)

type AcquireParams struct {
    UserID    string
    GameID    int32              // resolved internal games.id (callers resolve external→internal as today)
    Mode      AcquireMode
    Platforms []PlatformInput    // platform, storefront, hours, ownership, is_available, acquired_date, external_game_id
    Tags      []TagInput         // optional; additive-merge (see convergence #3)
    TagMode   TagMode            // Merge (sync/import) | Replace (unused here; ReplaceTags is the explicit op)
}
```

Behaviour, all inside one `RunInTx`:

1. **user_games row.** `ModeUpsert`: `INSERT … ON CONFLICT (user_id, game_id) DO
   UPDATE SET updated_at = now() RETURNING id, (xmax = 0) AS is_new`. `ModeCreate`:
   plain insert; a unique violation on `(user_id, game_id)` → `ErrConflict`
   ("game already in collection").
2. **Platform merge** (canonical = sync's rules, see convergence #1): for each
   input, upsert by `(user_game_id, platform, storefront)`:
   - absent → insert (`is_available`, `hours_played`, `ownership_status`,
     `external_game_id`, `acquired_date`).
   - present → single `UPDATE`: `hours_played = max(existing, new)`,
     `ownership_status` = higher of the two by `ownershipRank`, always backfill
     `external_game_id`. Record `OwnershipUpgraded` + old/new in the `Result`.
3. **Invariant tail:** `clearWishlistOnAcquire`, `promoteToInProgressIfPlayed`,
   and (if `Tags` supplied) tag reconciliation in `TagMode`.
4. Return `Result`.

Folding the promote into step 3 is what makes #1061 structurally impossible —
there is no acquire path that omits it. The #1061 point-fix lines in
`import_item.go` and `import_pipeline.go` are deleted along with the rest of the
hand-chaining.

## Convergence decisions (behaviour changes — flagged for review per #1055)

Each is a place where the existing paths disagree and we pick one canonical
behaviour. **These are the items most worth scrutinizing during spec review.**

1. **Platform merge becomes the canonical insert-or-update for the upsert paths.**
   Today: sync merges (max-hours, ownership-rank upgrade, `external_game_id`
   backfill); **import skips** an existing `(platform, storefront)` pair entirely
   (`import_item.go:278`). Under `ModeUpsert`, import re-import will now **update**
   an existing platform's hours/ownership upward instead of skipping. This is
   strictly more correct (re-import reflects new playtime) and is a no-op when the
   source values don't exceed stored ones. *Behaviour change: re-import updates
   existing platform rows.*

2. **`ModeCreate` keeps strict conflict semantics; `ModeUpsert` is idempotent.**
   The API create/add-platform 409 ("already in collection" / duplicate
   platform) is intentional UX for an explicit user action and is preserved via
   `ModeCreate`. Sync/import use `ModeUpsert` and never 409. *No behaviour change
   for the API; this just makes the existing fork explicit.*

3. **Tags merge additively on acquire; full-replace stays the dedicated op.**
   Import merges tags additively (never deletes tags from other sources;
   `import_item.go:322-343`). The API tags endpoint (`ReplaceTags`) is an explicit
   full reconcile. These are different *intents*, not a divergence to collapse:
   `Acquire` uses `TagMode = Merge`; `ReplaceTags` remains the explicit replace
   operation. *No behaviour change.*

4. **Acquire invariant set is atomic.** API create and create-platform run
   clear-wishlist/promote on `h.db` outside any transaction today; sync/import run
   them on `w.DB` outside a transaction. All become atomic within the operation's
   `RunInTx`. *Behaviour change: partial-failure no longer leaves an incoherent
   row (e.g. platforms inserted but wishlist not cleared).*

## Transaction & atomicity model

Operations own `RunInTx`. **Job-scoped side-effects stay at the worker
boundary** because they require a `job_id` the operation does not have:

- The `changes('added' | 'already_in_library' | 'status_changed')` rows
  (`sync.go:812`, `857`, `865`) and per-item `notify.Emit` are written by the
  **worker after `Acquire` returns**, driven by `Result.Created` /
  `Result.PlatformChanges` (which carry old/new ownership for `status_changed`).
  This is not a regression: those rows are written on `w.DB` outside any
  transaction today, and the existing "write after platforms confirmed" ordering
  is preserved by writing them after the operation commits.

## Error model

`internal/usergame` defines sentinels:

```go
var ErrNotFound   = errors.New("user game not found")
var ErrConflict   = errors.New("conflict")        // already in collection / duplicate platform
var ErrValidation = errors.New("validation")      // wrap detail with %w + fmt.Errorf
```

Operations wrap with `%w`; `sql.ErrNoRows` maps to `ErrNotFound`; a
`db.IsUniqueViolation(err)` maps to `ErrConflict`. The package stays echo-free.

- **API handlers** gain a small mapper `httpError(err) *echo.HTTPError`:
  `ErrNotFound`→404, `ErrConflict`→409, `ErrValidation`→400, else 500 (+ the
  existing `slog.ErrorContext` on the 500 branch). Existing per-endpoint status
  codes and messages are preserved.
- **Workers** map operation errors to their existing `markItemFailed` / log path.

## Call-site routing

| Current site | → Operation |
|---|---|
| `HandleCreateUserGame` | `Acquire(ModeCreate)` |
| sync `UserGameWorker`, `import_item`, `import_pipeline` | `Acquire(ModeUpsert)` |
| `HandleCreatePlatform` / `HandleBulkAddPlatforms` | `AddPlatform` / `AddPlatformBulk` |
| `HandleUpdatePlatform` / `HandleDeletePlatform` / `HandleBulkRemovePlatforms` | `UpdatePlatform` / `RemovePlatform` / `RemovePlatformBulk` |
| `HandleUpdateUserGame` / `HandleBulkUpdate` | `UpdateFields` / `SetPlayStatusBulk` |
| `HandleUpdateProgress` | `RecordProgress` |
| `HandleMoveToLibrary` | `MoveToLibrary` |
| `HandleReplaceTags` | `ReplaceTags` |
| `HandleDeleteUserGame` / `HandleBulkDelete` / `HandleClearLibrary` | `Delete` / `DeleteBulk` / `ClearLibrary` |

Read handlers (`HandleListUserGames`, `HandleGetUserGame`, `HandleListPlatforms`,
`HandleListUserGameIDs`, `HandleListGenres`, `HandleFilterOptions`,
`HandleCollectionStats`) are **not touched** (#1062).

`AddPlatform` / `MoveToLibrary` reuse the merge + invariant tail of `Acquire`
against an existing user_game (they assert ownership/wishlist state first and
return `ErrNotFound` / `ErrValidation` as appropriate).

## Testing

Per project policy (shared `testDB`, `truncateAllTables(t)` per test), the
safety net moves to operation-level tests in `internal/usergame`:

- `Acquire` idempotency (running `ModeUpsert` twice is stable).
- promote-if-played fires on **every** acquire path (regression lock for #1061).
- wishlist clears **atomically** with the platform insert.
- platform merge: max-hours, ownership-rank upgrade, `external_game_id` backfill.
- `ModeCreate` returns `ErrConflict` on an existing game; `ModeUpsert` does not.
- `removeFromPoolsIfFinished` fires on finishing status writes.
- ownership scoping: an operation never mutates another user's row.

Meaningful existing handler tests migrate down to operation tests; handler tests
retain a thin layer asserting the `httpError` status mapping. Workers keep
coverage that the change-row / notify side-effects still fire off `Result`.

## Rollout

One PR. Suggested internal execution order (not separate PRs): shared
`db.IsUniqueViolation` + sentinel errors → `Acquire` + its merge → route the four
acquire call sites → platform/status/progress/delete/bulk operations → delete the
four `isDuplicateKeyError` copies and the now-unused exported helpers → run
`make deadcode` to confirm no orphaned exports.
