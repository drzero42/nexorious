# Play Planning — Frontend Design (#956)

**Status:** Approved (design); implementation pending.
**Epic:** #939. **Backend:** #955 (PR #968, landed) + #971 (per-game membership endpoint, pending).
**This issue:** #956 — per-pool page, Planning nav, add-to-pool entry points.

## Summary

The frontend half of Play Planning. Backend #955 shipped the full `/api/pools`
surface, the OR-of-cards filter engine, the `?pool=:id` suggestions query, and
the finished-status auto-removal hook (see
[play-planning-backend-design.md](2026-06-12-play-planning-backend-design.md)). This spec covers the React SPA:
a **Planning** section with a pools index, a **per-pool page** (Up Next queue,
Candidates, Suggestions), an **Add-to-pool** membership dialog, and a **filter
editor**. No new backend behavior is designed here except the dependency on
**#971** (per-game pool membership read endpoint) called out in §7.

All semantics — data model, OR-of-cards filter, finished set
`{completed, mastered, dominated, dropped}`, wishlist-inclusive membership,
`ClearWishlistOnAcquire` keeping a game's queue slot on acquisition — are decided
in the [#939 data-model comment](https://github.com/drzero42/nexorious/issues/939#issuecomment-4692552404)
and realized in the landed backend. The frontend reflects them; it does not
re-decide them.

## Backend contract (already shipped — what the frontend builds on)

Verified against `internal/api/pools.go`, `internal/api/user_games.go`,
`internal/filter/pool.go` (PR #968):

| Method / path | Purpose | Notes |
|---|---|---|
| `GET /api/pools` | List | `[{id,name,color,position,has_filter,queue_count,candidate_count}]`, ordered by `position`. `color` is **nullable**. |
| `POST /api/pools` | Create | `{name, color?, filter?}` → pool. |
| `POST /api/pools/reorder` | Reorder | `{ids:[…ordered]}` → 204. Renumbers `position` contiguously. |
| `GET /api/pools/:id` | Detail | pool + pre-split `queue[]` and `candidates[]` (full game cards, already ordered: queue by `(position, created_at)`, candidates by `created_at`). |
| `PUT /api/pools/:id` | Update | Partial `{name?, color?, filter?}` (absent key = unchanged). |
| `DELETE /api/pools/:id` | Delete | 204. Cascades `pool_games`. |
| `POST /api/pools/:id/games` | Add member | `{user_game_id}` → **always lands as Candidate** (position NULL). Idempotent (200 no-op if already a member). |
| `DELETE /api/pools/:id/games/:userGameId` | Remove member | 204. Removes from pool entirely (queue or candidate). |
| `PUT /api/pools/:id/queue` | Set queue | `{ids:[…ordered]}` — **declarative**: listed ids become the queue in order (`position = index`); every other member demotes to Candidate. Every listed id must already be a member (else 400). |
| `GET /api/games?pool=:id` | Suggestions / filtered library | Owned+wishlist games matching the pool's OR-of-cards filter, **finished statuses excluded**, each annotated `pool_membership: "queued" \| "candidate" \| <absent>`. Honors `sort_by`/`sort_order` + pagination. A pool with a NULL filter returns an empty list. Ad-hoc facet params are **not** merged when `pool=` is set. |

**Key consequence:** "reorder", "promote a candidate", "demote", and "set on
deck" are all the *same* call — `PUT /api/pools/:id/queue` with a different
ordered `ids` list. The on-deck game is simply `queue[0]`. "Add" is always a
two-step path to the queue (POST → Candidate, then PUT queue to promote), which
the UI can chain when the user drags a suggestion straight into Up Next.

## Decisions

1. **Nav + management — index page (mirrors `/tags`).** A `Planning` nav item
   opens `/pools`; create/rename/recolor/delete/reorder live there. Rejected:
   expandable nav section / hybrid (more nav surface to build and keep in sync;
   the index mirrors an existing, well-understood page).
2. **Per-pool layout — stacked.** Up Next, then Candidates, then Suggestions,
   each full-width. Rejected: two-box side-by-side (cramped on mobile) and tabbed
   lower box (hides the candidates↔suggestions relationship and needs a Tabs
   component we don't have).
3. **Suggestions box shows suggestions only** (filter-matches not yet in the
   pool), not the full annotated library. Cleaner "matches this pool — add?"
   surface.
4. **Candidates and Suggestions are sortable** with the same field/direction
   control as the library. Suggestions sort **server-side** (params on
   `?pool=:id`, since it is paginated); Candidates sort **client-side** (returned
   whole in the detail payload). Up Next is manually ordered and not sortable.
5. **Reorder uses dnd-kit** (`@dnd-kit/core` + `@dnd-kit/sortable` +
   `@dnd-kit/utilities`) — no DnD library exists in the repo today. Used for the
   Up Next queue and the pools-index reorder.
6. **Add-to-pool is a membership toggle**, consuming the upcoming **#971**
   endpoint. Fallback: add-only if #971 is not yet merged (§7).
7. **Filter editor is a modal**, a stacked list of OR-cards. Rejected: dedicated
   sub-page (extra route; the modal matches the create/edit-pool dialogs).

## Routes (TanStack Router, file-based)

New files under `ui/frontend/src/routes/_authenticated/`:

- `pools/index.tsx` — pools index (list, create/edit/delete, drag-reorder).
- `pools/$id.tsx` — per-pool page (Up Next / Candidates / Suggestions). No nested
  child routes (the filter editor is a modal), so a single leaf route suffices.

Add the `Planning` item to `components/navigation/nav-items.tsx` (`mainItems`,
near Wishlist/Tags). Per-route page titles via the existing `#697` pattern
("Planning" for the index, the pool name for the detail page). Run
`npm run build` to regenerate and commit `routeTree.gen.ts`.

## API client & hooks

**`src/api/pools.ts`** (new) — thin wrappers over `api.get/post/put/delete`:
`getPools()`, `getPool(id)`, `createPool(data)`, `updatePool(id, data)`,
`deletePool(id)`, `reorderPools(ids)`, `addPoolGame(poolId, userGameId)`,
`removePoolGame(poolId, userGameId)`, `setQueue(poolId, ids)`, and
`getGamePoolMemberships(userGameId)` (#971).

**`src/hooks/use-pools.ts`** (new) — query keys + hooks mirroring `use-tags.ts`:

```ts
poolKeys = {
  all: ['pools'],
  lists: () => [...all, 'list'],
  detail: (id) => [...all, 'detail', id],
  memberships: (userGameId) => [...all, 'memberships', userGameId], // #971
}
```

- `usePools()`, `usePool(id)` (queries).
- `useCreatePool/useUpdatePool/useDeletePool/useReorderPools` → invalidate
  `lists()`.
- `useAddPoolGame/useRemovePoolGame/useSetQueue` → invalidate the affected
  `detail(id)`, the suggestions query for that pool, and any open
  `memberships(...)`. **Optimistic updates** for `useSetQueue` (queue reorder must
  feel instant) and for add/remove toggles; rollback on error with a sonner toast.

Extend the existing user-games hook/api to pass `pool` and any `sort_by`/
`sort_order` for the suggestions query, and to surface the optional
`pool_membership` field.

## Types

`src/types/` additions (mirror the Go DTOs verified above):

```ts
interface PoolListItem { id; name; color: string | null; position; has_filter; queue_count; candidate_count }
interface Pool { id; user_id; name; color: string | null; position; filter: PoolFilter | null; has_filter; created_at; updated_at }
interface PoolDetail extends Pool { queue: UserGame[]; candidates: UserGame[] }
interface FilterCard { play_status?; genre?; theme?; tag?; platform?; storefront?; rating_min?; rating_max?; is_loved?; game_mode?; player_perspective?; q?; time_to_beat_min?; time_to_beat_max? }
interface PoolFilter { filters: FilterCard[] }
interface PoolMembership { pool_id: string; position: number | null } // #971
```

Extend `UserGame` with `pool_membership?: 'queued' | 'candidate'`.

## Components

Under `src/components/pools/` unless noted:

- **`pool-card.tsx`** — index row: color dot, name, `queue/candidate` counts,
  overflow menu (edit/delete), drag handle.
- **`pool-form-dialog.tsx`** — create/edit (name + color). Extract the Tags page's
  `ColorPicker` into `src/components/ui/color-picker.tsx` and reuse it here and in
  `tags.tsx` (small refactor; the color field is shared design language between
  tags and pools). Pool color is optional (nullable) — allow "no color".
- **Delete confirm** — reuse `AlertDialog`.
- **`up-next-queue.tsx`** — dnd-kit `SortableContext` over `queue[]`; first card
  carries an "On deck" marker; drop → `setQueue` with the new order. Accepts drops
  from Candidates (promote). Per-card actions: demote to candidate, remove from
  pool.
- **`candidates-grid.tsx`** — sortable (client-side) grid; per-card "promote to
  queue" and "remove".
- **`suggestions-grid.tsx`** — server-sorted, paginated grid from
  `GET /api/games?pool=:id`; per-card "+ add" (→ `addPoolGame`). Filtered to
  `pool_membership` absent (suggestions only).
- **`pool-sort-control.tsx`** — shared field/direction control; reuse the library
  `sortOptions` (extract the list so both consume one source). Drives Candidates
  (client) and Suggestions (server).
- **`pool-filter-editor.tsx`** — modal; ordered list of cards (add/remove),
  each card a facet set assembled from `MultiSelectFilter` + the existing option
  hooks (`useAllPlatforms`, `useAllStorefronts`, `useFilterOptions`,
  `useAllTags`), plus selects for play_status/loved and number inputs for rating
  and time-to-beat ranges. "Matches ANY card" helper text. Serializes to
  `PoolFilter`; an empty card is invalid (backend rejects facet-less cards). Save
  → `updatePool({filter})`.
- **`add-to-pool-dialog.tsx`** — opened from a library `GameCard` action and the
  game-detail view. Merges `usePools()` with `useGamePoolMemberships(userGameId)`
  (#971) to render checkboxes reflecting current membership; check →
  `addPoolGame`, uncheck → `removePoolGame`; "+ new pool" inline. Fallback (no
  #971): render add-only (no check-state, no remove).

**`GameCard` reuse / additions** (`src/components/games/game-card.tsx`):
- **Buy-first badge** — derived `is_wishlisted && platforms.length === 0`; shown
  on wishlisted-unowned cards in pool zones (and reused if a wishlist buy-first
  affordance already exists from #867 — check before adding a second one).
- Context affordances (drag handle, "+ add", per-card menu) passed in as
  props/slots so the base card stays generic; don't fork the component.

## States & errors

Loading skeletons for index and per-pool zones. Empty states: index ("No pools
yet — create one"); empty pool ("Nothing here yet — add games from Suggestions or
the library"); empty Suggestions when the pool has no filter ("Add a filter to
get suggestions") vs. filter present but zero matches ("No matches right now").
Mutation failures surface a sonner toast and roll back optimistic state. A pool
that 404s (deleted in another tab) redirects to the index with a toast.

Finished-status auto-removal is silent and backend-driven: after a play-status
change, invalidate affected pool details/suggestions so removed games disappear
without a prompt (consistent with the decided semantics).

## Dependency on #971

The Add-to-pool **toggle** needs a per-game membership read endpoint
(`GET /api/games/:userGameId/pools` or `GET /api/pools/memberships?user_game_id=:id`)
returning `[{pool_id, position}]` — see #971. It is the only backend gap; add and
remove already exist. If #971 has not merged when frontend work begins, ship the
dialog **add-only** and swap in the toggle behind the same component once the
endpoint lands. Everything else in this spec depends only on the already-shipped
backend.

## New npm dependency

`@dnd-kit/core`, `@dnd-kit/sortable`, `@dnd-kit/utilities`. Adding them changes
`ui/frontend/package-lock.json` → update `npmDepsHash` in `nix/frontend.nix`
(`nix run nixpkgs#prefetch-npm-deps -- ui/frontend/package-lock.json`).

## Testing

Per the project policy (test non-trivial logic, not wrappers), focus
frontend tests (`vitest` + Testing Library) on:
- The queue-mutation mapping: reorder / promote / demote / set-on-deck all produce
  the correct `setQueue` `ids` payload, and "remove" calls `removePoolGame` (not
  `setQueue`).
- `PoolFilter` (de)serialization: cards round-trip; an empty/facet-less card is
  blocked client-side before hitting the API.
- Client-side Candidates sort matches the library's field semantics.
- `add-to-pool-dialog` membership merge: checkbox state derives correctly from
  `usePools()` × memberships; toggling fires add vs. remove.
- Buy-first badge derivation (`is_wishlisted && no platforms`).

Skip tests for thin card/layout wrappers and pure presentational components.

## Out of scope (unchanged from the epic)

No taste/enjoyment predictor, no external data, no LLM, no progress-based "why
now" nudges. Suggestions are exactly the deterministic OR-of-cards filter the
backend already computes.
