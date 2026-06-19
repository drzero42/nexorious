# One canonical way to read/project a user-game (#1062)

Status: **design** · Issue: [#1062](https://github.com/drzero42/nexorious/issues/1062) · Sibling of (closed) epic [#1055](https://github.com/drzero42/nexorious/issues/1055) · Date: 2026-06-19

## Problem

The project's "one canonical way to achieve an outcome" principle, applied to **reads**: a user-game (with its platforms, storefront records, tags, and external-game links) is loaded with a hand-copied relation block at many call sites instead of one shared definition.

The relation triple

```go
Relation("Game").
Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
    return q.Relation("PlatformRecord").Relation("StorefrontRecord")
}).
Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
    return q.Relation("Tag")
})
```

is copy-pasted **~8× in `internal/api/user_games.go`** (list-by-ids, get-detail, and the post-mutation re-reads in create / update-fields / replace-tags / update-progress / move-to-library) and **once in `internal/api/pools.go`** (`loadUserGameCards`). The DTO projection (`toUserGameWithPlatformsResponse`) is already shared between those two handlers, so the divergence is in the **query**, not the projection.

The copies have already drifted: `HandleGetUserGame` loads an extra `Relation("ExternalGame")` on platforms (for storefront deep-links) and filters by `user_id`; the five post-mutation re-reads omit both. That is precisely the accidental divergence this issue exists to remove.

## Scope

**In scope:** card/detail reads in the `internal/api` package — `user_games.go` and `pools.go`.

**Out of scope (separate concerns, confirmed with the issue author's framing):**

- Collection stats (`HandleCollectionStats`) and filter-option facets (`HandleFilterOptions`, `HandleListGenres`) — aggregate query shapes that do not share the relation/projection seam.
- `internal/worker/tasks/export.go` — loads the relations but projects into the import/export **v2 file contract** DTO, a deliberately different shape; converging it would couple the file format to the API projection.
- Mutation logic (owned by the landed #1056 / `internal/usergame`), the filter package, and any ORM rewrite.

## Home

The helpers live in the **`internal/api`** package (new file `internal/api/user_game_read.go`). Every in-scope consumer (`user_games.go`, `pools.go`) is already in that package. This matches the epic's reframing — "REST is the only in-process consumer" — and its explicit non-goal of speculative cross-package extraction for consumers that are not in-process. The mutation owner `internal/usergame` is **not** extended for reads: doing so would add a package boundary for a dedup whose only consumers live in `internal/api` today.

## Design

A new file `internal/api/user_game_read.go` with three package-level (free, `*bun.DB`-taking) helpers:

### 1. Relation decorator

```go
// withUserGameRelations applies the canonical set of relations for projecting a
// user-game into a card/detail response: the game, its platforms (with platform,
// storefront, and external-game records), and its tags.
func withUserGameRelations(q *bun.SelectQuery) *bun.SelectQuery {
    return q.
        Relation("Game").
        Relation("Platforms", func(q *bun.SelectQuery) *bun.SelectQuery {
            return q.Relation("PlatformRecord").Relation("StorefrontRecord").Relation("ExternalGame")
        }).
        Relation("Tags", func(q *bun.SelectQuery) *bun.SelectQuery {
            return q.Relation("Tag")
        })
}
```

`ExternalGame` is part of the **one** canonical relation set — loaded everywhere (decision below).

### 2. Single-row detail loader

```go
// LoadUserGameDetail loads a single user-game owned by userID with the canonical
// relation set. Returns sql.ErrNoRows when the game does not exist or is not the
// caller's.
func LoadUserGameDetail(ctx context.Context, db *bun.DB, userGameID, userID string) (*models.UserGame, error)
```

Always scoped by `user_id`, folding `HandleGetUserGame`'s ownership filter into the canonical path. Replaces all six single-id query blocks:

- `HandleGetUserGame` (read)
- `HandleCreateUserGame` re-read (note: this site currently re-selects by the freshly-created id; it has `userID` in scope)
- `HandleUpdateUserGame` re-read
- `HandleReplaceTags` re-read
- `HandleUpdateProgress` re-read
- `HandleMoveToLibrary` re-read

The five post-mutation callers already verified ownership through the mutation; scoping the re-read by `user_id` is harmless (the row is owned) and removes the divergence. Callers map `sql.ErrNoRows` to their existing 404 / 500 handling (`errors.Is(err, sql.ErrNoRows)`).

### 3. By-id-list card loader

```go
// LoadUserGameCardsByIDs loads user-games for the given ids with the canonical
// relation set, for list/card projections. Order is not guaranteed; callers that
// need a specific order re-apply it (HandleListUserGames) or key by id (pools).
func LoadUserGameCardsByIDs(ctx context.Context, db *bun.DB, ids []string) ([]models.UserGame, error)
```

Used by:

- `HandleListUserGames` (line ~324) — re-applies its sort on the returned slice, exactly as today.
- `pools.go` `loadUserGameCards` — becomes a thin wrapper that calls this and builds its `map[string]userGameWithPlatformsResponse` via the existing `toUserGameWithPlatformsResponse`.

### Projection unchanged

`toUserGameWithPlatformsResponse` (and `toUserGamePlatformResponse`) stay exactly as-is. They are already the shared projection seam; this change only converges the **query** feeding them.

## Deliberate convergence (behaviour change)

Per the epic's policy that converging a divergence means picking the correct behaviour and calling it out:

`toUserGamePlatformResponse` reads `ugp.ExternalGame` to populate the platform's **`store_url`** (the storefront deep-link, `json:"store_url,omitempty"`). Because only detail-GET loaded `ExternalGame`, **list cards, pool cards, and the five mutation responses currently omit `store_url`** — their `ExternalGame` is nil so the `if` is skipped. Converging to one loader that always loads `ExternalGame` means **those responses now carry the `store_url` deep-link too**, matching detail-GET. This is the correct behaviour: it removes a latent inconsistency where the library grid / post-mutation payloads lacked the storefront links the detail view had.

It is additive — the response gains an `omitempty` field, nothing is removed or changed. No back-compat concern (single user, no external client pinned to the payload shape). There is **no multi-user data-isolation impact**: `ExternalGame` is reachable only through a platform row of a user-game the caller owns, and every loader is `user_id`-scoped. The only cost is one extra relation fetch on the list/card query, independent of user count and accepted here for a single canonical loader.

Detail-GET behaviour is unchanged.

## Testing

- Existing `internal/api/user_games_test.go` and the pool tests cover the endpoints behaviourally and must stay green (they will now also see `ExternalGame` populated where it was previously absent — assert additively, do not assert it is nil).
- Add a focused test for the canonical loader(s):
  - `LoadUserGameDetail` returns all four relation groups (Game, Platforms with PlatformRecord/StorefrontRecord/ExternalGame, Tags with Tag) populated.
  - `LoadUserGameDetail` returns `sql.ErrNoRows` for another user's game id (ownership scoping).
  - `LoadUserGameCardsByIDs` returns the same relation set for a set of ids.
- Use the shared `testDB` + `truncateAllTables(t)` pattern; seed platforms/storefronts with their **seeded** names (e.g. `pc-windows`, `steam`) per the FK constraint.

## Dead-code check

This refactor removes call-site query blocks and may orphan nothing exported, but it deletes code paths and refactors within a package boundary — run `make deadcode` afterward and reconcile any new entries against the diff.

## Non-goals

- Not a mutation refactor (#1056, landed).
- Not stats/facet/export convergence.
- No ORM / filter-package rewrite.
