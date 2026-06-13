# Play Planning (backend) — data model, API, filter primitive, completion hook

**Issue:** [#955](https://github.com/drzero42/nexorious/issues/955) — backend half of the Play Planning epic [#939](https://github.com/drzero42/nexorious/issues/939).
**Status:** design approved, ready for an implementation plan.
**Lands first.** The frontend ([#956](https://github.com/drzero42/nexorious/issues/956)) consumes this API and is blocked on it.

## Summary

Pools are user-defined, ordered collections of games a user plans to play — a sibling of
tags that adds *ordering*, *intent to play*, and a per-pool *saved filter* that drives
suggestions. A pool doubles as an **Up Next queue**: a member with a `position` is queued,
a member without one is a **Candidate**. Membership spans owned and wishlisted games. When
a game reaches a finished play-status it silently leaves every pool.

The data model is already decided in the
[data-model design comment on #939](https://github.com/drzero42/nexorious/issues/939#issuecomment-4692552404).
This spec settles the **API and behaviour surface** on top of it.

Two foundational shape decisions were made during the brainstorm:

- **Hybrid endpoint shape.** Pool meta + its (bounded) members are returned inline by
  `GET /api/pools/:id`; the (unbounded, paginated) filtered/suggestion view reuses the
  existing `GET /api/user-games` list endpoint with a `?pool=:id` param. This is the closest
  fit to existing patterns: the list endpoint already does faceted filtering, and tags
  already returns derived membership counts inline.
- **Declarative queue state.** The in-board mutations (promote / demote / reorder) are
  expressed by a single `PUT /api/pools/:id/queue` that takes the full ordered list of
  queued ids. This makes a drag gesture — even "Candidate dropped into the middle of the
  queue" — one atomic, idempotent write, which fits the drag-and-drop board the frontend
  builds.

## Schema

Two tables, two Bun models in `internal/db/models/`, mirroring `Tag`. Migration pair
`internal/db/migrations/20260612000001_create_pools.{up,down}.sql` (confirm the running
number against the latest migration at implementation time).

### `pools` (a sibling of `tags`)

| column | type | notes |
|---|---|---|
| `id` | TEXT PK | uuid, generated in the handler |
| `user_id` | TEXT NOT NULL → `users` | cascade on delete |
| `name` | TEXT NOT NULL | unique per `(user_id, name)`, like tags |
| `color` | TEXT NULL | visual sibling of tags |
| `position` | INTEGER NOT NULL | hand-ordered nav; contiguous, renumbered in a txn on reorder |
| `filter` | JSONB NULL | saved `PoolFilter`; `NULL` = pure manual pool, no suggestions |
| `created_at` / `updated_at` | TIMESTAMPTZ | UTC, set in the handler |

### `pool_games` (membership **and** queue — one table)

| column | type | notes |
|---|---|---|
| `id` | TEXT PK | uuid |
| `pool_id` | TEXT NOT NULL → `pools` | cascade |
| `user_game_id` | TEXT NOT NULL → `user_games` | cascade |
| `position` | INTEGER NULL | `NULL` = **Candidate**; `NOT NULL` = in the **Up Next** queue |
| `created_at` | TIMESTAMPTZ | |

- Unique `(pool_id, user_game_id)` — a game is in a pool at most once.
- Order Up Next by `(position, created_at)`; Candidates by `created_at`. Gaps in
  `position` are tolerated (no unique constraint on it); the next explicit queue write
  renumbers contiguous.

## API surface

All endpoints are user-scoped via the session (like tags); every query filters by the
authenticated `user_id`.

### Pool CRUD + nav reorder

| Method | Path | Behaviour |
|---|---|---|
| `GET` | `/api/pools` | List the user's pools ordered by `position`. Each item: `id, name, color, position, has_filter, queue_count, candidate_count`. Counts are aggregated inline, mirroring how `GET /api/tags` returns `game_count`. |
| `POST` | `/api/pools` | Body `{name, color?, filter?}`. `name` required and unique per user; `position = max(position)+1`; validate filter cards if present (see *Filter validation*). |
| `GET` | `/api/pools/:id` | Pool meta **+ members inline**: `{ ...pool, queue: [card…], candidates: [card…] }`. Members are full game cards reusing the `user_games` list-item response shape (cover, title, platforms, `play_status`, `is_wishlisted` for the buy-first badge). `queue` ordered by `(position, created_at)`, `candidates` by `created_at`. |
| `PUT` | `/api/pools/:id` | Partial update of `name` / `color` / `filter`. Re-validate filter. |
| `DELETE` | `/api/pools/:id` | Delete the pool; `pool_games` cascades. |
| `POST` | `/api/pools/reorder` | Body `{ids:[…]}` → renumber `pools.position` contiguous in a txn. `POST` (not `PUT /:id`-style) avoids the Echo v5 `/:id` vs `/reorder` route-collision gotcha. |

### Membership + queue (declarative)

| Method | Path | Behaviour |
|---|---|---|
| `POST` | `/api/pools/:id/games` | Body `{user_game_id}`. Validate the `user_game` exists and belongs to the user (owned **or** wishlisted) — **pools never create `user_games`**; the off-profile add still goes through library/wishlist first. Insert as a **Candidate** (`position NULL`). **Idempotent**: re-adding an existing member is a `200` no-op (keeps its current state), not a `409`. |
| `DELETE` | `/api/pools/:id/games/:userGameId` | Remove the membership; `404` if not a member. |
| `PUT` | `/api/pools/:id/queue` | Body `{ids:[…ordered]}` = the desired queued set. In a txn: every id **must already be a member** of this pool (else `400` — no silent add); each listed id gets `position = index`; **any member not in the list is demoted to Candidate** (`position NULL`). This single call expresses promote + demote + reorder atomically. |

The promote-appends-at-`max+1` semantic from the issue is satisfied by the client appending
the id to the end of the list it `PUT`s. The declarative model carries a known, accepted
trade-off: a stale client that omits a recently-queued id will demote it — acceptable
because the client sends authoritative board state, and the member-must-exist guard prevents
the inverse footgun (a stale list can never silently *add*).

### Suggestions / filtered view (reuses the list endpoint)

`GET /api/user-games?pool=:id&page=…`:

- The server loads the pool's saved `filter`, applies it (OR of cards, see below)
  `AND play_status NOT IN <finished set>`, and returns the paginated matches (owned +
  wishlist) in the **existing list response shape**.
- Each item carries a new `pool_membership` field: `null | "candidate" | "queued"`. The
  frontend derives a *suggestion* as a match with `pool_membership: null` ("matches this
  pool — add?").
- If the pool's `filter` is `NULL` (pure manual pool) the result is empty.
- **v1:** `?pool` supplies the filter; ad-hoc facet params are **not** merged on top
  (sort and pagination params are still honoured). Merging ad-hoc facets is a possible
  later addition.

## Filter engine

### `ApplyTimeToBeat` primitive

Add `filter.ApplyTimeToBeat(fb *FilterBuilder, min, max *float64)` to `internal/filter/criteria.go`,
mirroring `ApplyRatingMin` / `ApplyRatingMax` but over `games.howlongtobeat_main` (requires
the `games` JOIN, like the genre/theme facets). Rows with a `NULL` `howlongtobeat_main` do
not match a range. Wire `time_to_beat_min` / `time_to_beat_max` query params into
`HandleListUserGames` so the primitive is available in the **normal library**, not only
pools.

Time-to-beat as a *user-driven filter* ("games I can finish in a weekend") is distinct from,
and does not conflict with, the epic's out-of-scope "why now" nudges built *on* HLTB figures.

### OR-of-cards application

The existing `FilterBuilder` ANDs its `where` closures — that is exactly one faceted card.
The pool filter is an ordered *list* of cards evaluated as OR.

```go
type PoolFilter struct {
    Filters []FilterCard `json:"filters"`
}

// FilterCard mirrors the existing library list params:
//   play_status[], genre[], theme[], tag[], platform[], storefront[],
//   rating_min/max, is_loved, game_mode[], player_perspective[], q,
//   time_to_beat_min/max
// (play_status became multi-value in #976; legacy single-string filters still parse.)
type FilterCard struct { /* … */ }
```

- `filter.ApplyPoolFilter(fb, pf)` builds, per card, that card's facet predicates grouped
  with `AND`, then OR's the cards together with an outer `WhereGroup(" OR ", …)`. A game
  matches the pool if it matches **any** card. This is the one thing the faceted engine
  cannot otherwise express (cross-facet disjunction); we get it without a boolean
  expression tree, consistent with the feature's "deliberately narrow and transparent"
  ethos.
- The global `play_status NOT IN (completed, mastered, dominated, dropped)` exclusion is
  applied by the caller **outside** `ApplyPoolFilter` — it is never stored in a card.
- `PoolFilter` unmarshals through the typed struct with **unknown keys rejected**
  ("typed in Go, JSONB at rest"), keeping the JSONB column from degrading into a
  free-for-all blob.

### Filter validation (on pool create/update)

- `filter` omitted / `null` → store `NULL` (pure manual pool, no suggestions).
- `{filters: []}` (empty array) → coerced to `NULL`.
- Each card must set **≥1 facet** — reject an empty card with `400` ("filter card has no
  facets").
- Unknown JSON keys → `400`.

## Completion hook

`usergame.RemoveFromPoolsIfFinished(ctx context.Context, db bun.IDB, userGameID string) error`,
mirroring the existing `ClearWishlistOnAcquire` / `PromoteToInProgressIfPlayed` helpers in
`internal/usergame/`:

- `DELETE FROM pool_games WHERE user_game_id = ?` guarded by the `user_game`'s current
  `play_status` being in the finished set `{completed, mastered, dominated, dropped}`.
  Removes the game from **every** pool at once.
- **Idempotent** — safe to call after any play-status write; it deletes only when the new
  status is finished, so it can be called unconditionally on the write path.
- **No queue renumber on auto-removal.** Gaps are tolerated by the data model; the next
  explicit queue write renumbers. This keeps the helper a single statement.
- **Wiring:** call inside the txn of both play-status write paths in
  `internal/api/user_games.go` — `HandleUpdateUserGame` (single) and `HandleBulkUpdate`
  (bulk) — for each affected `user_game`, at the same site `ClearWishlistOnAcquire` is
  already invoked. An explicit shared helper, **not** a DB trigger, consistent with how the
  wishlist machinery is wired.

### The finished set

`{completed, mastered, dominated, dropped}` drives both auto-removal **and**
suggestion-exclusion identically. `not_started`, `in_progress`, `replay`, and `shelved`
stay eligible (`replay` is active intent; `shelved` is a temporary park). `dropped` is
included in both behaviours deliberately — it is the single strongest "not next" signal, so
it should leave the plan the same way a completion does. Re-add later if you change your
mind.

## Test plan

Following the repo's "test non-trivial / real-bug-catching logic" policy:

- **`ApplyTimeToBeat`** — min-only, max-only, range; `NULL` `howlongtobeat_main` excluded.
- **`ApplyPoolFilter`** — matches any card (OR); a game matching no card is excluded; the
  finished set is excluded by the outer guard; multi-card with array facets.
- **`RemoveFromPoolsIfFinished`** — removes across multiple pools when finished; no-op for
  eligible statuses; idempotent; fires from both the single and bulk update paths.
- **Queue `PUT`** — promote, demote, reorder, and a combined drag in one call; rejects a
  non-member id (`400`); demotes members absent from the list; renumbers contiguous.
- **Add** — rejects a non-existent / other-user `user_game`; idempotent re-add; lands as a
  Candidate.
- **Pool CRUD** — name uniqueness per user; empty-card rejection; empty `filters` → `NULL`;
  `position` appends at `max+1`; reorder renumbers contiguous; delete cascades `pool_games`.
- **Suggestion endpoint** — `pool_membership` flag correctness (`null` / `candidate` /
  `queued`); finished games excluded; `NULL`-filter pool returns empty.

## Out of scope (unchanged from the epic)

No taste/enjoyment predictor, no external data pull, no LLM, no progress-based "why now"
nudges. Suggestions are deterministic and user-defined, built only from data already in the
database. The frontend (per-pool page, nav, add-to-pool entry points) is [#956](https://github.com/drzero42/nexorious/issues/956).
