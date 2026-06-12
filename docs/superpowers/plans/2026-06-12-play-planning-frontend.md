# Play Planning — Frontend Implementation Plan (#956)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the React SPA half of Play Planning — a Planning nav + pools index, a per-pool page (Up Next queue / Candidates / Suggestions), a filter editor modal, and an Add-to-pool membership toggle — on top of the already-shipped backend.

**Architecture:** Mirror the existing Tags feature (`use-tags.ts`, `api/tags.ts`, `routes/_authenticated/tags.tsx`). A thin `api/pools.ts` wraps the `/api/pools` REST surface; `use-pools.ts` exposes TanStack Query hooks with optimistic updates for queue/membership mutations. The queue is reordered with dnd-kit. All testable logic (queue-mutation mapping, filter (de)serialization, membership merge, buy-first derivation) is extracted into pure helper modules so it can be unit-tested without rendering.

**Tech Stack:** Vite + React 19 + TypeScript, TanStack Router (file-based) + TanStack Query, Tailwind v4 + shadcn/ui, dnd-kit (new), Vitest + Testing Library, sonner toasts.

**Backend contract (already shipped — verified against `internal/api/pools.go`, `internal/api/user_games.go`, `internal/filter/pool.go`):**

| Method / path | Purpose | Notes |
|---|---|---|
| `GET /api/pools` | List | `[{id,name,color,position,has_filter,queue_count,candidate_count}]`, ordered by `position`. `color` nullable. |
| `POST /api/pools` | Create | `{name, color?, filter?}` → `poolResponse`. 409 on duplicate name. |
| `POST /api/pools/reorder` | Reorder | `{ids:[…]}` → 204. |
| `GET /api/pools/:id` | Detail | `poolResponse` + `queue[]` + `candidates[]` (full game cards, pre-split & pre-ordered). |
| `PUT /api/pools/:id` | Update | Partial `{name?, color?, filter?}` → `poolResponse`. |
| `DELETE /api/pools/:id` | Delete | 204. |
| `GET /api/pools/memberships?user_game_id=:id` | Memberships | `[{pool_id, position}]`. Empty array if none; 404 if game not caller's; 400 if param missing. |
| `POST /api/pools/:id/games` | Add member | `{user_game_id}` → `{status:"ok"}` (200). Always Candidate. Idempotent. |
| `DELETE /api/pools/:id/games/:userGameId` | Remove member | 204. |
| `PUT /api/pools/:id/queue` | Set queue | `{ids:[…ordered]}` → `{status:"ok"}`. Declarative: listed ids become queue in order; others demote to Candidate. Every id must already be a member (else 400). |
| `GET /api/games?pool=:id` | Suggestions | Same envelope as `/user-games` (`{user_games,total,page,per_page,pages}`); each item carries `pool_membership: "queued"\|"candidate"\|<absent>`. Honors `sort_by`/`sort_order`/`page`/`per_page`. NULL-filter pool → empty list. |

**Key consequence:** reorder, promote, demote, and set-on-deck are all the *same* call — `PUT /api/pools/:id/queue` with a different ordered `ids`. On-deck = `queue[0]`.

---

## File Structure

**New:**
- `ui/frontend/src/types/pool.ts` — Pool DTO types; re-exported from `types/index.ts`.
- `ui/frontend/src/api/pools.ts` — REST wrappers.
- `ui/frontend/src/hooks/use-pools.ts` — query keys + hooks; re-exported from `hooks/index.ts`.
- `ui/frontend/src/lib/pool-queue.ts` + `.test.ts` — pure queue-mutation mapping helpers.
- `ui/frontend/src/lib/pool-filter.ts` + `.test.ts` — `PoolFilter` (de)serialization + validation.
- `ui/frontend/src/lib/game-flags.ts` + `.test.ts` — `isBuyFirst()` derivation (placed in lib so card + grids share it).
- `ui/frontend/src/components/ui/color-picker.tsx` — extracted from `tags.tsx`.
- `ui/frontend/src/components/pools/pool-card.tsx`
- `ui/frontend/src/components/pools/pool-form-dialog.tsx`
- `ui/frontend/src/components/pools/up-next-queue.tsx`
- `ui/frontend/src/components/pools/candidates-grid.tsx`
- `ui/frontend/src/components/pools/suggestions-grid.tsx`
- `ui/frontend/src/components/pools/pool-sort-control.tsx`
- `ui/frontend/src/components/pools/pool-filter-editor.tsx`
- `ui/frontend/src/components/pools/add-to-pool-dialog.tsx` + `.test.tsx`
- `ui/frontend/src/routes/_authenticated/pools/index.tsx`
- `ui/frontend/src/routes/_authenticated/pools/$id.tsx`
- `ui/frontend/src/lib/sort-options.ts` — extracted shared `sortOptions` list.

**Modified:**
- `ui/frontend/src/components/navigation/nav-items.tsx` — add Planning item.
- `ui/frontend/src/types/index.ts` — re-export `./pool`.
- `ui/frontend/src/hooks/index.ts` — re-export `use-pools`.
- `ui/frontend/src/types/game.ts` — add `pool_membership?` to `UserGame`.
- `ui/frontend/src/api/games.ts` — `transformUserGame` carries `pool_membership`; add `getPoolSuggestions`.
- `ui/frontend/src/components/games/game-card.tsx` — optional context slots + buy-first badge.
- `ui/frontend/src/components/games/game-filters.tsx` — import shared `sortOptions` from `lib/sort-options.ts`.
- `ui/frontend/src/routes/_authenticated/tags.tsx` — use extracted `ColorPicker`.
- `ui/frontend/src/routes/_authenticated/games/index.tsx` + `games/$id.index.tsx` — Add-to-pool entry points.
- `ui/frontend/package.json` / `package-lock.json` — dnd-kit deps.
- `nix/frontend.nix` — `npmDepsHash` bump.
- `ui/frontend/src/routeTree.gen.ts` — regenerated.

---

## Phase 0 — Scaffolding (deps, types, API, hooks)

### Task 1: Add dnd-kit dependency

**Files:**
- Modify: `ui/frontend/package.json`, `ui/frontend/package-lock.json`
- Modify: `nix/frontend.nix`

- [ ] **Step 1: Install dnd-kit**

Run (from `ui/frontend/`):
```bash
npm install @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities
```
Expected: three deps added to `package.json` `dependencies`, `package-lock.json` updated.

- [ ] **Step 2: Update the nix npmDepsHash**

Run (from repo root):
```bash
nix run nixpkgs#prefetch-npm-deps -- ui/frontend/package-lock.json
```
Paste the printed hash into `nix/frontend.nix` → `npmDepsHash`.

- [ ] **Step 3: Verify the app still builds**

Run (from `ui/frontend/`): `npm run build`
Expected: build succeeds.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/package.json ui/frontend/package-lock.json nix/frontend.nix
git commit -m "build: add dnd-kit for Play Planning queue reordering"
```

---

### Task 2: Pool types

**Files:**
- Create: `ui/frontend/src/types/pool.ts`
- Modify: `ui/frontend/src/types/index.ts`
- Modify: `ui/frontend/src/types/game.ts:100-113`

- [ ] **Step 1: Create the pool types**

Create `ui/frontend/src/types/pool.ts`:
```ts
import type { UserGame } from './game';

/** One row in the pools index (GET /api/pools). */
export interface PoolListItem {
  id: string;
  name: string;
  color: string | null;
  position: number;
  has_filter: boolean;
  queue_count: number;
  candidate_count: number;
}

/** A single faceted filter card. Mirrors internal/filter/pool.go FilterCard. */
export interface FilterCard {
  play_status?: string;
  genre?: string[];
  theme?: string[];
  tag?: string[];
  platform?: string[];
  storefront?: string[];
  rating_min?: number;
  rating_max?: number;
  is_loved?: boolean;
  game_mode?: string[];
  player_perspective?: string[];
  q?: string;
  time_to_beat_min?: number;
  time_to_beat_max?: number;
}

/** A pool's saved filter: OR of cards. */
export interface PoolFilter {
  filters: FilterCard[];
}

/** Full pool (create/update/detail meta). `filter` is raw JSON from the API. */
export interface Pool {
  id: string;
  user_id: string;
  name: string;
  color: string | null;
  position: number;
  filter: PoolFilter | null;
  has_filter: boolean;
  created_at: string;
  updated_at: string;
}

/** GET /api/pools/:id — pool meta plus pre-split members. */
export interface PoolDetail extends Pool {
  queue: UserGame[];
  candidates: UserGame[];
}

/** One element of GET /api/pools/memberships. */
export interface PoolMembership {
  pool_id: string;
  position: number | null;
}
```

- [ ] **Step 2: Re-export from the types barrel**

In `ui/frontend/src/types/index.ts`, add (alphabetically near `./platform`):
```ts
export * from './pool';
```

- [ ] **Step 3: Extend UserGame with pool_membership**

In `ui/frontend/src/types/game.ts`, inside `export interface UserGame { … }` (after `tags?: Tag[];`):
```ts
  /** Present only on GET /api/games?pool=:id responses. */
  pool_membership?: 'queued' | 'candidate';
```

- [ ] **Step 4: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS (no usages yet, just new types).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/types/
git commit -m "feat: add Play Planning pool types"
```

---

### Task 3: Pools API client

**Files:**
- Create: `ui/frontend/src/api/pools.ts`
- Modify: `ui/frontend/src/api/games.ts`

- [ ] **Step 1: Create the pools API client**

Create `ui/frontend/src/api/pools.ts`:
```ts
import { api } from './client';
import type {
  PoolListItem,
  Pool,
  PoolDetail,
  PoolFilter,
  PoolMembership,
} from '@/types';

export interface PoolCreateData {
  name: string;
  color?: string | null;
  filter?: PoolFilter | null;
}

export interface PoolUpdateData {
  name?: string;
  color?: string | null;
  filter?: PoolFilter | null;
}

export async function getPools(): Promise<PoolListItem[]> {
  return api.get<PoolListItem[]>('/pools');
}

export async function getPool(id: string): Promise<PoolDetail> {
  return api.get<PoolDetail>(`/pools/${id}`);
}

export async function createPool(data: PoolCreateData): Promise<Pool> {
  return api.post<Pool>('/pools', {
    name: data.name,
    color: data.color,
    filter: data.filter,
  });
}

export async function updatePool(id: string, data: PoolUpdateData): Promise<Pool> {
  // Only send keys that are present so the partial-update semantics hold.
  const body: Record<string, unknown> = {};
  if (data.name !== undefined) body.name = data.name;
  if (data.color !== undefined) body.color = data.color;
  if (data.filter !== undefined) body.filter = data.filter;
  return api.put<Pool>(`/pools/${id}`, body);
}

export async function deletePool(id: string): Promise<void> {
  await api.delete(`/pools/${id}`);
}

export async function reorderPools(ids: string[]): Promise<void> {
  await api.post('/pools/reorder', { ids });
}

export async function addPoolGame(poolId: string, userGameId: string): Promise<void> {
  await api.post(`/pools/${poolId}/games`, { user_game_id: userGameId });
}

export async function removePoolGame(poolId: string, userGameId: string): Promise<void> {
  await api.delete(`/pools/${poolId}/games/${userGameId}`);
}

export async function setQueue(poolId: string, ids: string[]): Promise<void> {
  await api.put(`/pools/${poolId}/queue`, { ids });
}

export async function getGamePoolMemberships(userGameId: string): Promise<PoolMembership[]> {
  return api.get<PoolMembership[]>('/pools/memberships', {
    params: { user_game_id: userGameId },
  });
}
```

- [ ] **Step 2: Add the suggestions fetch to games API**

In `ui/frontend/src/api/games.ts`, carry `pool_membership` through `transformUserGame` (it spreads `...apiUserGame`, but `UserGameApiResponse` doesn't declare the field). Add to `UserGameApiResponse` (after `tags?: Tag[];`):
```ts
  pool_membership?: 'queued' | 'candidate';
```
Then add a suggestions fetch at the end of the API functions:
```ts
export interface PoolSuggestionsParams {
  poolId: string;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
  page?: number;
  perPage?: number;
}

/**
 * Suggestions for a pool: owned+wishlist games matching the pool's OR-of-cards
 * filter (finished statuses excluded), each annotated with pool_membership.
 * Served by GET /api/games?pool=:id (same envelope as /user-games).
 */
export async function getPoolSuggestions(
  params: PoolSuggestionsParams,
): Promise<UserGamesListResponse> {
  const searchParams = new URLSearchParams();
  searchParams.append('pool', params.poolId);
  appendParam(searchParams, 'sort_by', params.sortBy);
  appendParam(searchParams, 'sort_order', params.sortOrder);
  appendParam(searchParams, 'page', params.page);
  appendParam(searchParams, 'per_page', params.perPage);
  const response = await api.get<UserGameListApiResponse>(`/games?${searchParams.toString()}`);
  return {
    items: response.user_games.map(transformUserGame),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}
```

- [ ] **Step 3: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/api/pools.ts ui/frontend/src/api/games.ts
git commit -m "feat: add pools API client and pool suggestions fetch"
```

---

### Task 4: Pools query hooks

**Files:**
- Create: `ui/frontend/src/hooks/use-pools.ts`
- Modify: `ui/frontend/src/hooks/index.ts`

- [ ] **Step 1: Create the hooks**

Create `ui/frontend/src/hooks/use-pools.ts`:
```ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as poolsApi from '@/api/pools';
import { getPoolSuggestions, type PoolSuggestionsParams } from '@/api/games';
import type { PoolListItem, PoolDetail, PoolMembership } from '@/types';
import type { UserGamesListResponse } from '@/api/games';

export const poolKeys = {
  all: ['pools'] as const,
  lists: () => [...poolKeys.all, 'list'] as const,
  details: () => [...poolKeys.all, 'detail'] as const,
  detail: (id: string) => [...poolKeys.details(), id] as const,
  suggestions: (id: string, params?: Omit<PoolSuggestionsParams, 'poolId'>) =>
    [...poolKeys.all, 'suggestions', id, params] as const,
  memberships: (userGameId: string) => [...poolKeys.all, 'memberships', userGameId] as const,
};

export function usePools() {
  return useQuery<PoolListItem[], Error>({
    queryKey: poolKeys.lists(),
    queryFn: poolsApi.getPools,
  });
}

export function usePool(id: string | undefined) {
  return useQuery<PoolDetail, Error>({
    queryKey: poolKeys.detail(id ?? ''),
    queryFn: () => poolsApi.getPool(id!),
    enabled: !!id,
  });
}

export function usePoolSuggestions(params: PoolSuggestionsParams) {
  const { poolId, ...rest } = params;
  return useQuery<UserGamesListResponse, Error>({
    queryKey: poolKeys.suggestions(poolId, rest),
    queryFn: () => getPoolSuggestions(params),
    enabled: !!poolId,
  });
}

export function useGamePoolMemberships(userGameId: string | undefined) {
  return useQuery<PoolMembership[], Error>({
    queryKey: poolKeys.memberships(userGameId ?? ''),
    queryFn: () => poolsApi.getGamePoolMemberships(userGameId!),
    enabled: !!userGameId,
  });
}

export function useCreatePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: poolsApi.PoolCreateData) => poolsApi.createPool(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

export function useUpdatePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: poolsApi.PoolUpdateData }) =>
      poolsApi.updatePool(id, data),
    onSuccess: (_r, { id }) => {
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
      qc.invalidateQueries({ queryKey: poolKeys.detail(id) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(id) });
    },
  });
}

export function useDeletePool() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => poolsApi.deletePool(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

export function useReorderPools() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (ids: string[]) => poolsApi.reorderPools(ids),
    onSuccess: () => qc.invalidateQueries({ queryKey: poolKeys.lists() }),
  });
}

// Membership + queue mutations all invalidate the affected pool detail, its
// suggestions, and any open per-game membership query. The components layer
// optimistic queue ordering on top via setQueryData before calling these.
export function useAddPoolGame() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, userGameId }: { poolId: string; userGameId: string }) =>
      poolsApi.addPoolGame(poolId, userGameId),
    onSuccess: (_r, { poolId, userGameId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.memberships(userGameId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}

export function useRemovePoolGame() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, userGameId }: { poolId: string; userGameId: string }) =>
      poolsApi.removePoolGame(poolId, userGameId),
    onSuccess: (_r, { poolId, userGameId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.suggestions(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.memberships(userGameId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}

export function useSetQueue() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ poolId, ids }: { poolId: string; ids: string[] }) =>
      poolsApi.setQueue(poolId, ids),
    onSuccess: (_r, { poolId }) => {
      qc.invalidateQueries({ queryKey: poolKeys.detail(poolId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    },
  });
}
```

Note: `poolKeys.suggestions(id)` is called with one arg in invalidations — the
key prefix `['pools','suggestions',id]` matches all param variants, which is the
intended broad invalidation.

- [ ] **Step 2: Re-export from the hooks barrel**

In `ui/frontend/src/hooks/index.ts`, add:
```ts
// Pool hooks
export {
  poolKeys,
  usePools,
  usePool,
  usePoolSuggestions,
  useGamePoolMemberships,
  useCreatePool,
  useUpdatePool,
  useDeletePool,
  useReorderPools,
  useAddPoolGame,
  useRemovePoolGame,
  useSetQueue,
} from './use-pools';
```

- [ ] **Step 3: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/hooks/use-pools.ts ui/frontend/src/hooks/index.ts
git commit -m "feat: add pools query hooks"
```

---

## Phase 1 — Pure logic helpers (TDD)

### Task 5: Queue-mutation mapping helper

The queue operations (reorder, promote a candidate, demote, remove, set-on-deck)
all reduce to a `setQueue(ids)` payload — except "remove", which calls
`removePoolGame`. Extract this mapping into a pure module so it is unit-tested
independently of dnd-kit and rendering.

**Files:**
- Create: `ui/frontend/src/lib/pool-queue.ts`
- Test: `ui/frontend/src/lib/pool-queue.test.ts`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/lib/pool-queue.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { reorderQueue, promoteToQueue, demoteFromQueue, setOnDeck } from './pool-queue';

describe('pool-queue mapping', () => {
  it('reorderQueue moves an id to a new index', () => {
    expect(reorderQueue(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
    expect(reorderQueue(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
  });

  it('reorderQueue is a no-op when from === to', () => {
    expect(reorderQueue(['a', 'b', 'c'], 1, 1)).toEqual(['a', 'b', 'c']);
  });

  it('promoteToQueue appends a candidate id to the end of the queue', () => {
    expect(promoteToQueue(['a', 'b'], 'c')).toEqual(['a', 'b', 'c']);
  });

  it('promoteToQueue is idempotent if the id is already queued', () => {
    expect(promoteToQueue(['a', 'b'], 'b')).toEqual(['a', 'b']);
  });

  it('demoteFromQueue drops an id from the queue list', () => {
    expect(demoteFromQueue(['a', 'b', 'c'], 'b')).toEqual(['a', 'c']);
  });

  it('setOnDeck moves an id to the front', () => {
    expect(setOnDeck(['a', 'b', 'c'], 'c')).toEqual(['c', 'a', 'b']);
    expect(setOnDeck(['a', 'b', 'c'], 'a')).toEqual(['a', 'b', 'c']);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test pool-queue`
Expected: FAIL — `pool-queue.ts` does not exist.

- [ ] **Step 3: Write the implementation**

Create `ui/frontend/src/lib/pool-queue.ts`:
```ts
/**
 * Pure helpers that map Up Next queue operations to the ordered `ids` list that
 * PUT /api/pools/:id/queue expects. The backend is declarative: the returned
 * list IS the new queue (position = index); any member not listed demotes to
 * Candidate. "Remove from pool" is NOT here — it calls removePoolGame, not
 * setQueue.
 */

/** Move the item at `from` to `to`, returning a new array. */
export function reorderQueue(ids: string[], from: number, to: number): string[] {
  if (from === to) return ids;
  const next = [...ids];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

/** Append a candidate to the end of the queue (idempotent). */
export function promoteToQueue(queueIds: string[], userGameId: string): string[] {
  if (queueIds.includes(userGameId)) return queueIds;
  return [...queueIds, userGameId];
}

/** Drop an id from the queue (it becomes a Candidate on the next setQueue). */
export function demoteFromQueue(queueIds: string[], userGameId: string): string[] {
  return queueIds.filter((id) => id !== userGameId);
}

/** Move an id to the front (on deck). */
export function setOnDeck(queueIds: string[], userGameId: string): string[] {
  if (queueIds[0] === userGameId) return queueIds;
  return [userGameId, ...queueIds.filter((id) => id !== userGameId)];
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test pool-queue`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/pool-queue.ts ui/frontend/src/lib/pool-queue.test.ts
git commit -m "feat: add pool queue-mutation mapping helpers"
```

---

### Task 6: Pool filter (de)serialization helper

The filter editor builds a `PoolFilter` from UI state and must block empty cards
before hitting the API (the backend rejects facet-less cards). Extract the
validation + round-trip into a pure module.

**Files:**
- Create: `ui/frontend/src/lib/pool-filter.ts`
- Test: `ui/frontend/src/lib/pool-filter.test.ts`

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/lib/pool-filter.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { cardHasFacets, sanitizeFilter, isValidFilter } from './pool-filter';
import type { FilterCard, PoolFilter } from '@/types';

describe('pool-filter', () => {
  it('cardHasFacets is false for an empty card', () => {
    expect(cardHasFacets({})).toBe(false);
  });

  it('cardHasFacets is false for a card with only empty arrays / blank q', () => {
    expect(cardHasFacets({ genre: [], q: '' })).toBe(false);
  });

  it('cardHasFacets is true when any facet is set', () => {
    expect(cardHasFacets({ genre: ['RPG'] })).toBe(true);
    expect(cardHasFacets({ play_status: 'backlog' })).toBe(true);
    expect(cardHasFacets({ is_loved: true })).toBe(true);
    expect(cardHasFacets({ rating_min: 7 })).toBe(true);
    expect(cardHasFacets({ q: 'witcher' })).toBe(true);
  });

  it('sanitizeFilter strips empty arrays and blank scalars, dropping empty cards', () => {
    const dirty: PoolFilter = {
      filters: [
        { genre: ['RPG'], theme: [], q: '' },
        {},
        { platform: ['windows'] },
      ],
    };
    expect(sanitizeFilter(dirty)).toEqual({
      filters: [{ genre: ['RPG'] }, { platform: ['windows'] }],
    });
  });

  it('isValidFilter is false when no card has facets', () => {
    expect(isValidFilter({ filters: [] })).toBe(false);
    expect(isValidFilter({ filters: [{}, { genre: [] }] })).toBe(false);
  });

  it('isValidFilter is true when at least one card has facets', () => {
    const f: PoolFilter = { filters: [{ genre: ['RPG'] }] };
    expect(isValidFilter(f)).toBe(true);
  });

  it('a card round-trips through sanitize unchanged when already clean', () => {
    const clean: FilterCard = { genre: ['RPG'], platform: ['windows'], is_loved: true };
    expect(sanitizeFilter({ filters: [clean] })).toEqual({ filters: [clean] });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test pool-filter`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

Create `ui/frontend/src/lib/pool-filter.ts`:
```ts
import type { FilterCard, PoolFilter } from '@/types';

/** True if the card constrains at least one facet (mirrors FilterCard.HasFacets in Go). */
export function cardHasFacets(c: FilterCard): boolean {
  return (
    (c.play_status != null && c.play_status !== '') ||
    (c.genre?.length ?? 0) > 0 ||
    (c.theme?.length ?? 0) > 0 ||
    (c.tag?.length ?? 0) > 0 ||
    (c.platform?.length ?? 0) > 0 ||
    (c.storefront?.length ?? 0) > 0 ||
    c.rating_min != null ||
    c.rating_max != null ||
    c.is_loved != null ||
    (c.game_mode?.length ?? 0) > 0 ||
    (c.player_perspective?.length ?? 0) > 0 ||
    (c.q != null && c.q !== '') ||
    c.time_to_beat_min != null ||
    c.time_to_beat_max != null
  );
}

/** Drop empty arrays / blank scalars from a card, returning a minimal card. */
function cleanCard(c: FilterCard): FilterCard {
  const out: FilterCard = {};
  if (c.play_status) out.play_status = c.play_status;
  if (c.genre?.length) out.genre = c.genre;
  if (c.theme?.length) out.theme = c.theme;
  if (c.tag?.length) out.tag = c.tag;
  if (c.platform?.length) out.platform = c.platform;
  if (c.storefront?.length) out.storefront = c.storefront;
  if (c.rating_min != null) out.rating_min = c.rating_min;
  if (c.rating_max != null) out.rating_max = c.rating_max;
  if (c.is_loved != null) out.is_loved = c.is_loved;
  if (c.game_mode?.length) out.game_mode = c.game_mode;
  if (c.player_perspective?.length) out.player_perspective = c.player_perspective;
  if (c.q) out.q = c.q;
  if (c.time_to_beat_min != null) out.time_to_beat_min = c.time_to_beat_min;
  if (c.time_to_beat_max != null) out.time_to_beat_max = c.time_to_beat_max;
  return out;
}

/** Clean each card and drop those left with no facets. */
export function sanitizeFilter(f: PoolFilter): PoolFilter {
  return { filters: f.filters.map(cleanCard).filter(cardHasFacets) };
}

/** A filter is valid to save iff at least one card survives sanitization. */
export function isValidFilter(f: PoolFilter): boolean {
  return sanitizeFilter(f).filters.length > 0;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test pool-filter`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/lib/pool-filter.ts ui/frontend/src/lib/pool-filter.test.ts
git commit -m "feat: add pool filter (de)serialization helpers"
```

---

### Task 7: Buy-first badge derivation

**Files:**
- Create: `ui/frontend/src/lib/game-flags.ts`
- Test: `ui/frontend/src/lib/game-flags.test.ts`

First confirm a buy-first affordance does not already exist (the spec says reuse
#867's if present):

- [ ] **Step 1: Check for an existing buy-first helper**

Run (from `ui/frontend/`):
```bash
grep -rin "buy.first\|buyFirst\|isBuyFirst" src/ || echo "none found"
```
Expected: `none found` (if a helper exists, reuse it and skip creating a new one — adapt the consuming tasks to import it).

- [ ] **Step 2: Write the failing test**

Create `ui/frontend/src/lib/game-flags.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { isBuyFirst } from './game-flags';
import type { UserGame } from '@/types';

function ug(partial: Partial<UserGame>): UserGame {
  return {
    id: 'ug1' as UserGame['id'],
    game: { id: 1 as never, title: 'X', rating_count: 0, created_at: '', updated_at: '' } as never,
    is_loved: false,
    play_status: 'backlog' as never,
    is_wishlisted: false,
    hours_played: 0,
    platforms: [],
    created_at: '',
    updated_at: '',
    ...partial,
  };
}

describe('isBuyFirst', () => {
  it('is true for a wishlisted game with no platforms', () => {
    expect(isBuyFirst(ug({ is_wishlisted: true, platforms: [] }))).toBe(true);
  });

  it('is false when not wishlisted', () => {
    expect(isBuyFirst(ug({ is_wishlisted: false, platforms: [] }))).toBe(false);
  });

  it('is false when wishlisted but already has a platform (acquired)', () => {
    expect(isBuyFirst(ug({ is_wishlisted: true, platforms: [{} as never] }))).toBe(false);
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test game-flags`
Expected: FAIL — module not found.

- [ ] **Step 4: Write the implementation**

Create `ui/frontend/src/lib/game-flags.ts`:
```ts
import type { UserGame } from '@/types';

/**
 * A wishlisted, not-yet-owned game shows a "buy first" badge in pool zones
 * instead of a play affordance. Acquiring it (a platform appears) flips this to
 * false in place — consistent with ClearWishlistOnAcquire keeping the queue slot.
 */
export function isBuyFirst(game: UserGame): boolean {
  return game.is_wishlisted && (game.platforms?.length ?? 0) === 0;
}
```

- [ ] **Step 5: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test game-flags`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/lib/game-flags.ts ui/frontend/src/lib/game-flags.test.ts
git commit -m "feat: add buy-first badge derivation helper"
```

---

### Task 8: Extract shared sort options

Decision 4: Candidates/Suggestions reuse the library's sort field list. Extract
it into one source.

**Files:**
- Create: `ui/frontend/src/lib/sort-options.ts`
- Modify: `ui/frontend/src/components/games/game-filters.tsx:26-49`

- [ ] **Step 1: Create the shared sort options module**

Create `ui/frontend/src/lib/sort-options.ts`:
```ts
export type SortField =
  | 'title'
  | 'created_at'
  | 'howlongtobeat_main'
  | 'personal_rating'
  | 'release_date'
  | 'hours_played'
  | 'rating_average';

export type SortOrder = 'asc' | 'desc';

export interface SortOption {
  value: SortField;
  label: string;
}

export const sortOptions: SortOption[] = [
  { value: 'title', label: 'Title' },
  { value: 'created_at', label: 'Date Added' },
  { value: 'howlongtobeat_main', label: 'Time to Beat' },
  { value: 'personal_rating', label: 'My Rating' },
  { value: 'release_date', label: 'Release Date' },
  { value: 'hours_played', label: 'Hours Played' },
  { value: 'rating_average', label: 'IGDB Rating' },
];
```

- [ ] **Step 2: Re-point game-filters to the shared module**

In `ui/frontend/src/components/games/game-filters.tsx`, delete the local
`type SortField`, `type SortOrder`, `interface SortOption`, and the
`const sortOptions` block (lines 26-49), and add an import near the top:
```ts
import { sortOptions, type SortField, type SortOrder } from '@/lib/sort-options';
```
(Keep `SortOrder` only if it was referenced locally; if `game-filters.tsx` does
not use `SortOrder`, omit it from the import to satisfy knip.)

- [ ] **Step 3: Typecheck + knip + existing filter tests**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test game-filters`
Expected: PASS — library sort behaviour unchanged.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/lib/sort-options.ts ui/frontend/src/components/games/game-filters.tsx
git commit -m "refactor: extract shared library sort options"
```

---

## Phase 2 — ColorPicker + Pools index page

### Task 9: Extract ColorPicker

**Files:**
- Create: `ui/frontend/src/components/ui/color-picker.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/tags.tsx`

- [ ] **Step 1: Create the shared ColorPicker**

Create `ui/frontend/src/components/ui/color-picker.tsx` (lift `COLOR_PALETTE`
and the `ColorPicker` component verbatim from `tags.tsx:52-102`):
```tsx
import { Input } from '@/components/ui/input';

/** Predefined color palette shared by tags and pools. */
export const COLOR_PALETTE = [
  '#EF4444', '#F97316', '#F59E0B', '#EAB308', '#84CC16', '#22C55E',
  '#10B981', '#14B8A6', '#06B6D4', '#0EA5E9', '#3B82F6', '#6366F1',
  '#8B5CF6', '#A855F7', '#D946EF', '#EC4899', '#F43F5E', '#6B7280',
];

export function ColorPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (color: string) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="h-8 w-8 rounded-md border" style={{ backgroundColor: value }} />
        <Input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-28 font-mono text-sm"
          placeholder="#000000"
        />
      </div>
      <div className="grid grid-cols-9 gap-1">
        {COLOR_PALETTE.map((color) => (
          <button
            key={color}
            type="button"
            className={`h-6 w-6 rounded-md border-2 transition-transform hover:scale-110 ${
              value === color ? 'border-foreground' : 'border-transparent'
            }`}
            style={{ backgroundColor: color }}
            onClick={() => onChange(color)}
            aria-label={`Select color ${color}`}
          />
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Use it in tags.tsx**

In `ui/frontend/src/routes/_authenticated/tags.tsx`: delete the local
`COLOR_PALETTE` (52-71) and `ColorPicker` (73-102) definitions, and add an
import:
```ts
import { ColorPicker, COLOR_PALETTE } from '@/components/ui/color-picker';
```
(`COLOR_PALETTE` is still used by `suggestColor`, so keep importing it.)

- [ ] **Step 3: Typecheck + knip + tags untouched**

Run (from `ui/frontend/`): `npm run check && npm run knip`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/ui/color-picker.tsx ui/frontend/src/routes/_authenticated/tags.tsx
git commit -m "refactor: extract ColorPicker into a shared ui component"
```

---

### Task 10: Pool form dialog (create/edit)

**Files:**
- Create: `ui/frontend/src/components/pools/pool-form-dialog.tsx`

- [ ] **Step 1: Create the dialog**

Create `ui/frontend/src/components/pools/pool-form-dialog.tsx`:
```tsx
import { useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { ColorPicker, COLOR_PALETTE } from '@/components/ui/color-picker';
import { Loader2 } from 'lucide-react';
import type { PoolListItem } from '@/types';

export interface PoolFormValues {
  name: string;
  color: string | null;
}

interface PoolFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** When set, the dialog is in edit mode and prefills from this row. */
  editing?: PoolListItem | null;
  onSubmit: (values: PoolFormValues) => Promise<void>;
  pending: boolean;
}

export function PoolFormDialog({
  open,
  onOpenChange,
  editing,
  onSubmit,
  pending,
}: PoolFormDialogProps) {
  const [name, setName] = useState('');
  const [useColor, setUseColor] = useState(false);
  const [color, setColor] = useState<string>(COLOR_PALETTE[0]);

  useEffect(() => {
    if (open) {
      setName(editing?.name ?? '');
      setUseColor(editing?.color != null);
      setColor(editing?.color ?? COLOR_PALETTE[0]);
    }
  }, [open, editing]);

  const handleSubmit = async () => {
    await onSubmit({ name: name.trim(), color: useColor ? color : null });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? 'Edit Pool' : 'Create Pool'}</DialogTitle>
          <DialogDescription>
            Pools group games you plan to play. Add a filter later to get suggestions.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="pool-name">Name *</Label>
            <Input
              id="pool-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Enter pool name..."
              maxLength={100}
            />
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="pool-use-color"
                checked={useColor}
                onCheckedChange={(v) => setUseColor(v === true)}
              />
              <Label htmlFor="pool-use-color">Use a color</Label>
            </div>
            {useColor && <ColorPicker value={color} onChange={setColor} />}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={pending || !name.trim()}>
            {pending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {editing ? 'Save' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/pool-form-dialog.tsx
git commit -m "feat: add pool create/edit form dialog"
```

---

### Task 11: Pool card (index row with drag handle)

**Files:**
- Create: `ui/frontend/src/components/pools/pool-card.tsx`

- [ ] **Step 1: Create the sortable pool row**

Create `ui/frontend/src/components/pools/pool-card.tsx`:
```tsx
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button } from '@/components/ui/button';
import { GripVertical, Pencil, Trash2, ListChecks } from 'lucide-react';
import type { PoolListItem } from '@/types';

interface PoolCardProps {
  pool: PoolListItem;
  onOpen: (id: string) => void;
  onEdit: (pool: PoolListItem) => void;
  onDelete: (pool: PoolListItem) => void;
}

export function PoolCard({ pool, onOpen, onEdit, onDelete }: PoolCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: pool.id });

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`flex items-center justify-between gap-3 border-b py-3 ${
        isDragging ? 'opacity-50' : ''
      }`}
    >
      <button
        type="button"
        className="cursor-grab touch-none text-muted-foreground"
        aria-label="Drag to reorder"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" />
      </button>
      <button
        type="button"
        className="flex flex-1 items-center gap-3 text-left"
        onClick={() => onOpen(pool.id)}
      >
        <span
          className="h-4 w-4 shrink-0 rounded-full border"
          style={{ backgroundColor: pool.color ?? 'transparent' }}
        />
        <span className="min-w-0 flex-1 truncate font-medium">{pool.name}</span>
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <ListChecks className="h-3 w-3" />
          {pool.queue_count} queued · {pool.candidate_count} candidates
        </span>
      </button>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="sm" onClick={() => onEdit(pool)}>
          <Pencil className="h-4 w-4" />
          <span className="sr-only">Edit</span>
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onDelete(pool)}>
          <Trash2 className="h-4 w-4 text-destructive" />
          <span className="sr-only">Delete</span>
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/pool-card.tsx
git commit -m "feat: add sortable pool index row component"
```

---

### Task 12: Pools index route

**Files:**
- Create: `ui/frontend/src/routes/_authenticated/pools/index.tsx`

- [ ] **Step 1: Create the index route**

Create `ui/frontend/src/routes/_authenticated/pools/index.tsx`:
```tsx
import { useState } from 'react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { toast } from 'sonner';
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { ListChecks, Plus, XCircle } from 'lucide-react';
import {
  usePools,
  useCreatePool,
  useUpdatePool,
  useDeletePool,
  useReorderPools,
} from '@/hooks';
import { PoolCard } from '@/components/pools/pool-card';
import { PoolFormDialog, type PoolFormValues } from '@/components/pools/pool-form-dialog';
import { reorderQueue } from '@/lib/pool-queue';
import type { PoolListItem } from '@/types';

export const Route = createFileRoute('/_authenticated/pools/')({
  head: () => ({ meta: [{ title: 'Planning | Nexorious' }] }),
  component: PoolsIndexPage,
});

function PoolsIndexPage() {
  const navigate = useNavigate();
  const { data: pools, isLoading, error, refetch } = usePools();
  const createPool = useCreatePool();
  const updatePool = useUpdatePool();
  const deletePool = useDeletePool();
  const reorderPools = useReorderPools();

  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<PoolListItem | null>(null);
  const [deleting, setDeleting] = useState<PoolListItem | null>(null);

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  const handleSubmit = async (values: PoolFormValues) => {
    try {
      if (editing) {
        await updatePool.mutateAsync({ id: editing.id, data: values });
        toast.success('Pool updated');
      } else {
        await createPool.mutateAsync(values);
        toast.success('Pool created');
      }
      setShowForm(false);
      setEditing(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save pool');
    }
  };

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deletePool.mutateAsync(deleting.id);
      toast.success('Pool deleted');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete pool');
    }
    setDeleting(null);
  };

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id || !pools) return;
    const from = pools.findIndex((p) => p.id === active.id);
    const to = pools.findIndex((p) => p.id === over.id);
    if (from < 0 || to < 0) return;
    const nextIds = reorderQueue(
      pools.map((p) => p.id),
      from,
      to,
    );
    reorderPools.mutate(nextIds, {
      onError: () => toast.error('Failed to reorder pools'),
    });
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <XCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-semibold">Failed to load pools</h2>
        <p className="text-muted-foreground">{error.message}</p>
        <Button onClick={() => refetch()} className="mt-4">
          Try Again
        </Button>
      </div>
    );
  }

  const list = pools ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            <ListChecks className="h-6 w-6" />
            Planning
          </h1>
          <p className="text-muted-foreground">
            Group games into pools and line up what to play next.
          </p>
        </div>
        <Button
          onClick={() => {
            setEditing(null);
            setShowForm(true);
          }}
        >
          <Plus className="mr-2 h-4 w-4" />
          Create Pool
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Pools ({list.length})</CardTitle>
          <CardDescription>Drag to reorder. Click a pool to plan it.</CardDescription>
        </CardHeader>
        <CardContent>
          {list.length === 0 ? (
            <div className="py-12 text-center">
              <ListChecks className="mx-auto h-12 w-12 text-muted-foreground" />
              <h3 className="mt-2 font-medium">No pools yet</h3>
              <p className="text-sm text-muted-foreground">Create one to start planning.</p>
            </div>
          ) : (
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
              <SortableContext items={list.map((p) => p.id)} strategy={verticalListSortingStrategy}>
                <div>
                  {list.map((pool) => (
                    <PoolCard
                      key={pool.id}
                      pool={pool}
                      onOpen={(id) => navigate({ to: '/pools/$id', params: { id } })}
                      onEdit={(p) => {
                        setEditing(p);
                        setShowForm(true);
                      }}
                      onDelete={setDeleting}
                    />
                  ))}
                </div>
              </SortableContext>
            </DndContext>
          )}
        </CardContent>
      </Card>

      <PoolFormDialog
        open={showForm}
        onOpenChange={setShowForm}
        editing={editing}
        onSubmit={handleSubmit}
        pending={createPool.isPending || updatePool.isPending}
      />

      <AlertDialog open={!!deleting} onOpenChange={(o) => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Pool</AlertDialogTitle>
            <AlertDialogDescription>
              Delete &quot;{deleting?.name}&quot;? This removes the pool and its membership; your
              games are not deleted.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
```

- [ ] **Step 2: Add the Planning nav item**

In `ui/frontend/src/components/navigation/nav-items.tsx`: add `ListChecks` to the
lucide import, and insert into `mainItems` after the Tags entry:
```tsx
    {
      href: '/pools',
      label: 'Planning',
      icon: <ListChecks className="h-4 w-4" />,
    },
```

- [ ] **Step 3: Regenerate the route tree, typecheck**

Run (from `ui/frontend/`): `npm run build && npm run check`
Expected: `routeTree.gen.ts` updated with `/pools/` route; check PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/pools/index.tsx \
        ui/frontend/src/components/navigation/nav-items.tsx \
        ui/frontend/src/routeTree.gen.ts
git commit -m "feat: add Planning nav and pools index page"
```

---

## Phase 3 — Per-pool page (Up Next / Candidates / Suggestions)

### Task 13: Pool sort control

**Files:**
- Create: `ui/frontend/src/components/pools/pool-sort-control.tsx`

- [ ] **Step 1: Create the control**

Create `ui/frontend/src/components/pools/pool-sort-control.tsx`:
```tsx
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { sortOptions, type SortField, type SortOrder } from '@/lib/sort-options';

interface PoolSortControlProps {
  sortBy: SortField;
  sortOrder: SortOrder;
  onSortByChange: (field: SortField) => void;
  onSortOrderChange: (order: SortOrder) => void;
}

export function PoolSortControl({
  sortBy,
  sortOrder,
  onSortByChange,
  onSortOrderChange,
}: PoolSortControlProps) {
  return (
    <div className="flex items-center gap-2">
      <Select value={sortBy} onValueChange={(v) => onSortByChange(v as SortField)}>
        <SelectTrigger className="w-40">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {sortOptions.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant="outline"
        size="icon"
        onClick={() => onSortOrderChange(sortOrder === 'asc' ? 'desc' : 'asc')}
        aria-label={sortOrder === 'asc' ? 'Ascending' : 'Descending'}
      >
        {sortOrder === 'asc' ? <ArrowUp className="h-4 w-4" /> : <ArrowDown className="h-4 w-4" />}
      </Button>
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/pool-sort-control.tsx
git commit -m "feat: add shared pool sort control"
```

---

### Task 14: Add context slots + buy-first badge to GameCard

**Files:**
- Modify: `ui/frontend/src/components/games/game-card.tsx`

- [ ] **Step 1: Extend GameCardProps and render slots/badge**

In `ui/frontend/src/components/games/game-card.tsx`:

Add imports near the top:
```ts
import { isBuyFirst } from '@/lib/game-flags';
import type { ReactNode } from 'react';
```

Extend the props interface:
```ts
export interface GameCardProps {
  game: UserGame;
  selected?: boolean;
  onSelect?: (id: string) => void;
  onClick?: () => void;
  /** Optional overlay rendered top-right (e.g. a drag handle or per-card menu). */
  topRightSlot?: ReactNode;
  /** Optional action row rendered below the card body (e.g. "+ add" / "promote"). */
  actionsSlot?: ReactNode;
}
```

Add `topRightSlot` and `actionsSlot` to the destructured params, and compute the
badge inside the component body (after `const coverUrl = …`):
```ts
  const buyFirst = isBuyFirst(game);
```

Render the buy-first badge in the cover overlay (next to the loved indicator,
inside the `aspect-[3/4]` div, before its closing tag):
```tsx
        {buyFirst && (
          <div className="absolute top-2 right-10">
            <Badge variant="secondary" className="text-xs">
              Buy first
            </Badge>
          </div>
        )}
        {topRightSlot && (
          <div className="absolute top-2 right-2 z-10" onClick={(e) => e.stopPropagation()}>
            {topRightSlot}
          </div>
        )}
```

And render `actionsSlot` at the end of `CardContent`, before its closing tag:
```tsx
        {actionsSlot && (
          <div className="mt-2" onClick={(e) => e.stopPropagation()}>
            {actionsSlot}
          </div>
        )}
```

(Note: the existing loved indicator already sits at `top-2 right-2`; place
`topRightSlot` consumers aware it overlaps — pool grids that pass a slot do not
also rely on the loved heart position. If both are needed, the slot wins because
it is rendered last with a higher `z-10`.)

- [ ] **Step 2: Run existing card tests + typecheck**

Run (from `ui/frontend/`): `npm run check && npm run test game-card`
Expected: PASS — existing card behaviour unchanged (new props are optional).

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/games/game-card.tsx
git commit -m "feat: add context slots and buy-first badge to GameCard"
```

---

### Task 15: Up Next queue component

**Files:**
- Create: `ui/frontend/src/components/pools/up-next-queue.tsx`

- [ ] **Step 1: Create the queue**

Create `ui/frontend/src/components/pools/up-next-queue.tsx`:
```tsx
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  useSortable,
  horizontalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { GripVertical, ArrowUpToLine, ArrowDownFromLine, X } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import { reorderQueue, demoteFromQueue, setOnDeck } from '@/lib/pool-queue';
import type { UserGame } from '@/types';

interface UpNextQueueProps {
  queue: UserGame[];
  /** Declarative: the new ordered queue ids after any reorder/demote/on-deck op. */
  onSetQueue: (ids: string[]) => void;
  /** Remove from the pool entirely (calls removePoolGame, not setQueue). */
  onRemove: (userGameId: string) => void;
}

function QueueItem({
  game,
  onDeck,
  onSetOnDeck,
  onDemote,
  onRemove,
}: {
  game: UserGame;
  onDeck: boolean;
  onSetOnDeck: () => void;
  onDemote: () => void;
  onRemove: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: game.id });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`w-40 shrink-0 ${isDragging ? 'opacity-50' : ''}`}
    >
      {onDeck && (
        <Badge className="mb-1 w-full justify-center" variant="default">
          On deck
        </Badge>
      )}
      <GameCard
        game={game}
        topRightSlot={
          <button
            type="button"
            className="cursor-grab touch-none rounded bg-background/80 p-1"
            aria-label="Drag to reorder"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="h-4 w-4" />
          </button>
        }
        actionsSlot={
          <div className="flex items-center justify-between">
            {!onDeck && (
              <Button variant="ghost" size="sm" onClick={onSetOnDeck} aria-label="Set on deck">
                <ArrowUpToLine className="h-4 w-4" />
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onDemote} aria-label="Demote to candidate">
              <ArrowDownFromLine className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="sm" onClick={onRemove} aria-label="Remove from pool">
              <X className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        }
      />
    </div>
  );
}

export function UpNextQueue({ queue, onSetQueue, onRemove }: UpNextQueueProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));
  const ids = queue.map((g) => g.id);

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const from = ids.indexOf(active.id as string);
    const to = ids.indexOf(over.id as string);
    if (from < 0 || to < 0) return;
    onSetQueue(reorderQueue(ids, from, to));
  };

  if (queue.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Nothing queued yet — promote a candidate or a suggestion.
      </p>
    );
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={ids} strategy={horizontalListSortingStrategy}>
        <div className="flex gap-3 overflow-x-auto pb-2">
          {queue.map((game, i) => (
            <QueueItem
              key={game.id}
              game={game}
              onDeck={i === 0}
              onSetOnDeck={() => onSetQueue(setOnDeck(ids, game.id))}
              onDemote={() => onSetQueue(demoteFromQueue(ids, game.id))}
              onRemove={() => onRemove(game.id)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/up-next-queue.tsx
git commit -m "feat: add Up Next queue with dnd-kit reordering"
```

---

### Task 16: Candidates grid (client-side sort)

**Files:**
- Create: `ui/frontend/src/components/pools/candidates-grid.tsx`

- [ ] **Step 1: Create the grid**

Create `ui/frontend/src/components/pools/candidates-grid.tsx`:
```tsx
import { useMemo } from 'react';
import { Button } from '@/components/ui/button';
import { ArrowUpToLine, X } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import type { UserGame } from '@/types';
import type { SortField, SortOrder } from '@/lib/sort-options';

interface CandidatesGridProps {
  candidates: UserGame[];
  sortBy: SortField;
  sortOrder: SortOrder;
  onPromote: (userGameId: string) => void;
  onRemove: (userGameId: string) => void;
  onOpen: (userGameId: string) => void;
}

/** Mirror the library's field semantics for the client-side sort. */
function compare(a: UserGame, b: UserGame, field: SortField): number {
  switch (field) {
    case 'title':
      return (a.game?.title ?? '').localeCompare(b.game?.title ?? '');
    case 'created_at':
      return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
    case 'howlongtobeat_main':
      return (a.game?.howlongtobeat_main ?? 0) - (b.game?.howlongtobeat_main ?? 0);
    case 'personal_rating':
      return (a.personal_rating ?? 0) - (b.personal_rating ?? 0);
    case 'release_date':
      return (
        new Date(a.game?.release_date ?? 0).getTime() -
        new Date(b.game?.release_date ?? 0).getTime()
      );
    case 'hours_played':
      return a.hours_played - b.hours_played;
    case 'rating_average':
      return (a.game?.rating_average ?? 0) - (b.game?.rating_average ?? 0);
  }
}

export function CandidatesGrid({
  candidates,
  sortBy,
  sortOrder,
  onPromote,
  onRemove,
  onOpen,
}: CandidatesGridProps) {
  const sorted = useMemo(() => {
    const arr = [...candidates];
    arr.sort((a, b) => {
      const c = compare(a, b, sortBy);
      return sortOrder === 'asc' ? c : -c;
    });
    return arr;
  }, [candidates, sortBy, sortOrder]);

  if (candidates.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        No candidates yet — add games from Suggestions or the library.
      </p>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {sorted.map((game) => (
        <GameCard
          key={game.id}
          game={game}
          onClick={() => onOpen(game.id)}
          actionsSlot={
            <div className="flex items-center justify-between">
              <Button variant="ghost" size="sm" onClick={() => onPromote(game.id)} aria-label="Promote to queue">
                <ArrowUpToLine className="h-4 w-4" />
              </Button>
              <Button variant="ghost" size="sm" onClick={() => onRemove(game.id)} aria-label="Remove from pool">
                <X className="h-4 w-4 text-destructive" />
              </Button>
            </div>
          }
        />
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/candidates-grid.tsx
git commit -m "feat: add candidates grid with client-side sort"
```

---

### Task 17: Suggestions grid (server-side sort + pagination)

**Files:**
- Create: `ui/frontend/src/components/pools/suggestions-grid.tsx`

- [ ] **Step 1: Create the grid**

Create `ui/frontend/src/components/pools/suggestions-grid.tsx`:
```tsx
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Plus } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import { usePoolSuggestions } from '@/hooks';
import type { SortField, SortOrder } from '@/lib/sort-options';

interface SuggestionsGridProps {
  poolId: string;
  hasFilter: boolean;
  sortBy: SortField;
  sortOrder: SortOrder;
  page: number;
  onPageChange: (page: number) => void;
  onAdd: (userGameId: string) => void;
  onOpen: (userGameId: string) => void;
}

const PER_PAGE = 24;

export function SuggestionsGrid({
  poolId,
  hasFilter,
  sortBy,
  sortOrder,
  page,
  onPageChange,
  onAdd,
  onOpen,
}: SuggestionsGridProps) {
  const { data, isLoading } = usePoolSuggestions({
    poolId,
    sortBy,
    sortOrder,
    page,
    perPage: PER_PAGE,
  });

  if (!hasFilter) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Add a filter to get suggestions.
      </p>
    );
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="aspect-[3/4]" />
        ))}
      </div>
    );
  }

  // Suggestions = filter matches NOT already in the pool.
  const items = (data?.items ?? []).filter((g) => g.pool_membership == null);

  if (items.length === 0) {
    return <p className="py-8 text-center text-sm text-muted-foreground">No matches right now.</p>;
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {items.map((game) => (
          <GameCard
            key={game.id}
            game={game}
            onClick={() => onOpen(game.id)}
            actionsSlot={
              <Button variant="outline" size="sm" className="w-full" onClick={() => onAdd(game.id)}>
                <Plus className="mr-1 h-4 w-4" /> Add
              </Button>
            }
          />
        ))}
      </div>
      {data && data.pages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {data.page} of {data.pages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= data.pages}
            onClick={() => onPageChange(page + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/components/pools/suggestions-grid.tsx
git commit -m "feat: add suggestions grid with server-side sort and pagination"
```

---

### Task 18: Per-pool page route

**Files:**
- Create: `ui/frontend/src/routes/_authenticated/pools/$id.tsx`

- [ ] **Step 1: Create the route**

Create `ui/frontend/src/routes/_authenticated/pools/$id.tsx`:
```tsx
import { useState } from 'react';
import { createFileRoute, useNavigate, useParams } from '@tanstack/react-router';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Filter, XCircle } from 'lucide-react';
import {
  usePool,
  useSetQueue,
  useAddPoolGame,
  useRemovePoolGame,
} from '@/hooks';
import { UpNextQueue } from '@/components/pools/up-next-queue';
import { CandidatesGrid } from '@/components/pools/candidates-grid';
import { SuggestionsGrid } from '@/components/pools/suggestions-grid';
import { PoolSortControl } from '@/components/pools/pool-sort-control';
import { PoolFilterEditor } from '@/components/pools/pool-filter-editor';
import { promoteToQueue } from '@/lib/pool-queue';
import type { SortField, SortOrder } from '@/lib/sort-options';

export const Route = createFileRoute('/_authenticated/pools/$id')({
  component: PoolDetailPage,
});

function PoolDetailPage() {
  const { id } = useParams({ from: '/_authenticated/pools/$id' });
  const navigate = useNavigate();
  const { data: pool, isLoading, error } = usePool(id);
  const setQueue = useSetQueue();
  const addGame = useAddPoolGame();
  const removeGame = useRemovePoolGame();

  const [candSort, setCandSort] = useState<SortField>('title');
  const [candOrder, setCandOrder] = useState<SortOrder>('asc');
  const [sugSort, setSugSort] = useState<SortField>('title');
  const [sugOrder, setSugOrder] = useState<SortOrder>('asc');
  const [sugPage, setSugPage] = useState(1);
  const [showFilter, setShowFilter] = useState(false);

  const openGame = (userGameId: string) =>
    navigate({ to: '/games/$id', params: { id: userGameId } });

  const handleSetQueue = (ids: string[]) => {
    if (!pool) return;
    setQueue.mutate(
      { poolId: pool.id, ids },
      { onError: () => toast.error('Failed to update queue') },
    );
  };

  const handlePromote = async (userGameId: string) => {
    if (!pool) return;
    const ids = promoteToQueue(
      pool.queue.map((g) => g.id),
      userGameId,
    );
    handleSetQueue(ids);
  };

  const handleAdd = (userGameId: string) => {
    if (!pool) return;
    addGame.mutate(
      { poolId: pool.id, userGameId },
      {
        onSuccess: () => toast.success('Added to pool'),
        onError: () => toast.error('Failed to add to pool'),
      },
    );
  };

  const handleRemove = (userGameId: string) => {
    if (!pool) return;
    removeGame.mutate(
      { poolId: pool.id, userGameId },
      { onError: () => toast.error('Failed to remove from pool') },
    );
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (error || !pool) {
    // A 404 (deleted in another tab) sends the user back to the index.
    toast.error('Pool not found');
    navigate({ to: '/pools' });
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <XCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-semibold">Pool not found</h2>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/pools' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            {pool.color && (
              <span
                className="h-4 w-4 rounded-full border"
                style={{ backgroundColor: pool.color }}
              />
            )}
            {pool.name}
          </h1>
        </div>
        <Button variant="outline" onClick={() => setShowFilter(true)}>
          <Filter className="mr-2 h-4 w-4" />
          {pool.has_filter ? 'Edit filter' : 'Add filter'}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Up Next</CardTitle>
        </CardHeader>
        <CardContent>
          <UpNextQueue queue={pool.queue} onSetQueue={handleSetQueue} onRemove={handleRemove} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Candidates ({pool.candidates.length})</CardTitle>
          <PoolSortControl
            sortBy={candSort}
            sortOrder={candOrder}
            onSortByChange={setCandSort}
            onSortOrderChange={setCandOrder}
          />
        </CardHeader>
        <CardContent>
          <CandidatesGrid
            candidates={pool.candidates}
            sortBy={candSort}
            sortOrder={candOrder}
            onPromote={handlePromote}
            onRemove={handleRemove}
            onOpen={openGame}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Suggestions</CardTitle>
          <PoolSortControl
            sortBy={sugSort}
            sortOrder={sugOrder}
            onSortByChange={(f) => {
              setSugSort(f);
              setSugPage(1);
            }}
            onSortOrderChange={(o) => {
              setSugOrder(o);
              setSugPage(1);
            }}
          />
        </CardHeader>
        <CardContent>
          <SuggestionsGrid
            poolId={pool.id}
            hasFilter={pool.has_filter}
            sortBy={sugSort}
            sortOrder={sugOrder}
            page={sugPage}
            onPageChange={setSugPage}
            onAdd={handleAdd}
            onOpen={openGame}
          />
        </CardContent>
      </Card>

      <PoolFilterEditor
        poolId={pool.id}
        open={showFilter}
        onOpenChange={setShowFilter}
        initialFilter={pool.filter}
      />
    </div>
  );
}
```

Note: the per-route page title uses the pool name. If the `#697` head pattern
requires a `head:` with loader data, follow the same approach the
`games/$id.index.tsx` route uses; a static component is acceptable here since the
title falls back to the app default and the pool name is shown in the H1.

- [ ] **Step 2: Regenerate the route tree + typecheck**

Run (from `ui/frontend/`): `npm run build && npm run check`
Expected: build fails at the `PoolFilterEditor` import (created next) — that's
expected; proceed to Task 19, then re-run. If you implement Task 19 first, this
passes.

> Implementation note: Tasks 18 and 19 are interdependent (the route imports the
> editor). Create the editor (Task 19) before running `npm run build` here, then
> commit both together.

- [ ] **Step 3: Commit (after Task 19)**

```bash
git add ui/frontend/src/routes/_authenticated/pools/$id.tsx ui/frontend/src/routeTree.gen.ts
git commit -m "feat: add per-pool page with Up Next, Candidates, Suggestions"
```

---

## Phase 4 — Filter editor modal

### Task 19: Pool filter editor

**Files:**
- Create: `ui/frontend/src/components/pools/pool-filter-editor.tsx`

- [ ] **Step 1: Create the editor**

Create `ui/frontend/src/components/pools/pool-filter-editor.tsx`:
```tsx
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { MultiSelectFilter } from '@/components/ui/multi-select-filter';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Plus, Trash2, Loader2 } from 'lucide-react';
import { useUpdatePool, useAllPlatforms, useAllStorefronts, useFilterOptions, useAllTags } from '@/hooks';
import { sanitizeFilter, isValidFilter } from '@/lib/pool-filter';
import { statusLabels } from '@/lib/play-status';
import { PlayStatus } from '@/types';
import type { FilterCard, PoolFilter } from '@/types';

// PlayStatus is an enum in types/game.ts; enumerate its values for the select.
const PLAY_STATUSES = Object.values(PlayStatus);

interface PoolFilterEditorProps {
  poolId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialFilter: PoolFilter | null;
}

function emptyCard(): FilterCard {
  return {};
}

export function PoolFilterEditor({ poolId, open, onOpenChange, initialFilter }: PoolFilterEditorProps) {
  const updatePool = useUpdatePool();
  const { data: platforms } = useAllPlatforms();
  const { data: storefronts } = useAllStorefronts();
  const { data: options } = useFilterOptions();
  const { data: tags } = useAllTags();

  const [cards, setCards] = useState<FilterCard[]>([]);

  useEffect(() => {
    if (open) {
      setCards(initialFilter?.filters.length ? initialFilter.filters : [emptyCard()]);
    }
  }, [open, initialFilter]);

  const updateCard = (idx: number, patch: Partial<FilterCard>) => {
    setCards((prev) => prev.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  };

  const platformOpts = (platforms ?? []).map((p) => ({ value: p.name, label: p.display_name ?? p.name }));
  const storefrontOpts = (storefronts ?? []).map((s) => ({ value: s.name, label: s.display_name ?? s.name }));
  const genreOpts = (options?.genres ?? []).map((g) => ({ value: g, label: g }));
  const themeOpts = (options?.themes ?? []).map((t) => ({ value: t, label: t }));
  const modeOpts = (options?.gameModes ?? []).map((m) => ({ value: m, label: m }));
  const perspectiveOpts = (options?.playerPerspectives ?? []).map((p) => ({ value: p, label: p }));
  const tagOpts = (tags ?? []).map((t) => ({ value: t.id, label: t.name }));

  const handleSave = async () => {
    const filter = sanitizeFilter({ filters: cards });
    if (!isValidFilter(filter)) {
      toast.error('Add at least one facet to a card before saving.');
      return;
    }
    try {
      await updatePool.mutateAsync({ id: poolId, data: { filter } });
      toast.success('Filter saved');
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save filter');
    }
  };

  const handleClear = async () => {
    try {
      await updatePool.mutateAsync({ id: poolId, data: { filter: null } });
      toast.success('Filter cleared');
      onOpenChange(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clear filter');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Pool Filter</DialogTitle>
          <DialogDescription>
            A game is suggested if it matches ANY card below. Within a card, all facets must match.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {cards.map((card, idx) => (
            <div key={idx} className="space-y-3 rounded-md border p-3">
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">Card {idx + 1}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setCards((prev) => prev.filter((_, i) => i !== idx))}
                  disabled={cards.length === 1}
                  aria-label="Remove card"
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <MultiSelectFilter
                  label="Genre"
                  options={genreOpts}
                  selected={card.genre ?? []}
                  onChange={(v) => updateCard(idx, { genre: v })}
                />
                <MultiSelectFilter
                  label="Theme"
                  options={themeOpts}
                  selected={card.theme ?? []}
                  onChange={(v) => updateCard(idx, { theme: v })}
                />
                <MultiSelectFilter
                  label="Platform"
                  options={platformOpts}
                  selected={card.platform ?? []}
                  onChange={(v) => updateCard(idx, { platform: v })}
                />
                <MultiSelectFilter
                  label="Storefront"
                  options={storefrontOpts}
                  selected={card.storefront ?? []}
                  onChange={(v) => updateCard(idx, { storefront: v })}
                />
                <MultiSelectFilter
                  label="Game Mode"
                  options={modeOpts}
                  selected={card.game_mode ?? []}
                  onChange={(v) => updateCard(idx, { game_mode: v })}
                />
                <MultiSelectFilter
                  label="Perspective"
                  options={perspectiveOpts}
                  selected={card.player_perspective ?? []}
                  onChange={(v) => updateCard(idx, { player_perspective: v })}
                />
                <MultiSelectFilter
                  label="Tag"
                  options={tagOpts}
                  selected={card.tag ?? []}
                  onChange={(v) => updateCard(idx, { tag: v })}
                />
                <Select
                  value={card.play_status ?? ''}
                  onValueChange={(v) => updateCard(idx, { play_status: v || undefined })}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Play status" />
                  </SelectTrigger>
                  <SelectContent>
                    {PLAY_STATUSES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {statusLabels[s]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1">
                  <Label className="text-xs">Rating min</Label>
                  <Input
                    type="number"
                    value={card.rating_min ?? ''}
                    onChange={(e) =>
                      updateCard(idx, {
                        rating_min: e.target.value === '' ? undefined : Number(e.target.value),
                      })
                    }
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Rating max</Label>
                  <Input
                    type="number"
                    value={card.rating_max ?? ''}
                    onChange={(e) =>
                      updateCard(idx, {
                        rating_max: e.target.value === '' ? undefined : Number(e.target.value),
                      })
                    }
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Time to beat min (h)</Label>
                  <Input
                    type="number"
                    value={card.time_to_beat_min ?? ''}
                    onChange={(e) =>
                      updateCard(idx, {
                        time_to_beat_min: e.target.value === '' ? undefined : Number(e.target.value),
                      })
                    }
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Time to beat max (h)</Label>
                  <Input
                    type="number"
                    value={card.time_to_beat_max ?? ''}
                    onChange={(e) =>
                      updateCard(idx, {
                        time_to_beat_max: e.target.value === '' ? undefined : Number(e.target.value),
                      })
                    }
                  />
                </div>
              </div>

              <div className="flex items-center gap-2">
                <Checkbox
                  id={`loved-${idx}`}
                  checked={card.is_loved === true}
                  onCheckedChange={(v) => updateCard(idx, { is_loved: v === true ? true : undefined })}
                />
                <Label htmlFor={`loved-${idx}`}>Loved only</Label>
              </div>
            </div>
          ))}

          <Button variant="outline" size="sm" onClick={() => setCards((prev) => [...prev, emptyCard()])}>
            <Plus className="mr-1 h-4 w-4" /> Add card (OR)
          </Button>
        </div>

        <DialogFooter className="flex items-center justify-between">
          <Button variant="ghost" onClick={handleClear} disabled={updatePool.isPending}>
            Clear filter
          </Button>
          <div className="flex gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button onClick={handleSave} disabled={updatePool.isPending}>
              {updatePool.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

> Verified during planning: `statusLabels` is exported from `@/lib/play-status`;
> `PlayStatus` is an enum in `types/game.ts` (iterate `Object.values(PlayStatus)`);
> `useAllPlatforms`/`useAllStorefronts` return objects with `name` and
> `display_name` (per `types/platform.ts`). The option mapping above uses those
> field names.

- [ ] **Step 2: Build + typecheck (also completes Task 18's build)**

Run (from `ui/frontend/`): `npm run build && npm run check`
Expected: PASS — both pool routes now resolve.

- [ ] **Step 3: Commit (editor + per-pool route together)**

```bash
git add ui/frontend/src/components/pools/pool-filter-editor.tsx \
        ui/frontend/src/routes/_authenticated/pools/$id.tsx \
        ui/frontend/src/routeTree.gen.ts
git commit -m "feat: add pool filter editor modal and wire per-pool page"
```

---

## Phase 5 — Add-to-pool dialog + entry points

### Task 20: Add-to-pool dialog (membership merge — tested)

**Files:**
- Create: `ui/frontend/src/components/pools/add-to-pool-dialog.tsx`
- Test: `ui/frontend/src/components/pools/add-to-pool-dialog.test.tsx`

The membership merge (which pools show as checked) and toggle dispatch (check →
add, uncheck → remove) is the testable logic. Extract the merge into a small pure
function in the same file so the test can target it directly.

- [ ] **Step 1: Write the failing test**

Create `ui/frontend/src/components/pools/add-to-pool-dialog.test.tsx`:
```tsx
import { describe, it, expect } from 'vitest';
import { mergeMembership } from './add-to-pool-dialog';
import type { PoolListItem, PoolMembership } from '@/types';

function pool(id: string, name: string): PoolListItem {
  return {
    id,
    name,
    color: null,
    position: 0,
    has_filter: false,
    queue_count: 0,
    candidate_count: 0,
  };
}

describe('mergeMembership', () => {
  const pools: PoolListItem[] = [pool('p1', 'A'), pool('p2', 'B'), pool('p3', 'C')];

  it('marks pools the game belongs to as checked', () => {
    const memberships: PoolMembership[] = [
      { pool_id: 'p1', position: 0 },
      { pool_id: 'p3', position: null },
    ];
    const rows = mergeMembership(pools, memberships);
    expect(rows.find((r) => r.pool.id === 'p1')?.member).toBe(true);
    expect(rows.find((r) => r.pool.id === 'p2')?.member).toBe(false);
    expect(rows.find((r) => r.pool.id === 'p3')?.member).toBe(true);
  });

  it('treats all pools as not-member when memberships is empty', () => {
    const rows = mergeMembership(pools, []);
    expect(rows.every((r) => !r.member)).toBe(true);
  });

  it('falls back to not-member when memberships is undefined (no #971 data yet)', () => {
    const rows = mergeMembership(pools, undefined);
    expect(rows.every((r) => !r.member)).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run (from `ui/frontend/`): `npm run test add-to-pool`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

Create `ui/frontend/src/components/pools/add-to-pool-dialog.tsx`:
```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Plus } from 'lucide-react';
import {
  usePools,
  useGamePoolMemberships,
  useAddPoolGame,
  useRemovePoolGame,
  useCreatePool,
} from '@/hooks';
import type { PoolListItem, PoolMembership } from '@/types';

export interface MembershipRow {
  pool: PoolListItem;
  member: boolean;
}

/**
 * Merge the full pool list with this game's memberships into checkbox rows.
 * `memberships` undefined (e.g. #971 read failed) degrades to all-unchecked.
 */
export function mergeMembership(
  pools: PoolListItem[],
  memberships: PoolMembership[] | undefined,
): MembershipRow[] {
  const memberIds = new Set((memberships ?? []).map((m) => m.pool_id));
  return pools.map((pool) => ({ pool, member: memberIds.has(pool.id) }));
}

interface AddToPoolDialogProps {
  userGameId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AddToPoolDialog({ userGameId, open, onOpenChange }: AddToPoolDialogProps) {
  const { data: pools } = usePools();
  const { data: memberships } = useGamePoolMemberships(open ? userGameId : undefined);
  const addGame = useAddPoolGame();
  const removeGame = useRemovePoolGame();
  const createPool = useCreatePool();
  const [newName, setNewName] = useState('');

  const rows = mergeMembership(pools ?? [], memberships);

  const toggle = (poolId: string, nextMember: boolean) => {
    const mutation = nextMember ? addGame : removeGame;
    mutation.mutate(
      { poolId, userGameId },
      { onError: () => toast.error('Failed to update pool membership') },
    );
  };

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    try {
      const pool = await createPool.mutateAsync({ name });
      await addGame.mutateAsync({ poolId: pool.id, userGameId });
      toast.success(`Added to ${name}`);
      setNewName('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create pool');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add to Pool</DialogTitle>
          <DialogDescription>Toggle which pools this game belongs to.</DialogDescription>
        </DialogHeader>
        <div className="max-h-64 space-y-2 overflow-y-auto py-2">
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No pools yet — create one below.</p>
          ) : (
            rows.map(({ pool, member }) => (
              <label key={pool.id} className="flex cursor-pointer items-center gap-3 py-1">
                <Checkbox checked={member} onCheckedChange={(v) => toggle(pool.id, v === true)} />
                {pool.color && (
                  <span
                    className="h-3 w-3 rounded-full border"
                    style={{ backgroundColor: pool.color }}
                  />
                )}
                <span className="flex-1">{pool.name}</span>
              </label>
            ))
          )}
        </div>
        <div className="flex items-center gap-2 border-t pt-3">
          <Input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="New pool name..."
            maxLength={100}
          />
          <Button variant="outline" size="sm" onClick={handleCreate} disabled={!newName.trim()}>
            <Plus className="mr-1 h-4 w-4" /> Create
          </Button>
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run (from `ui/frontend/`): `npm run test add-to-pool`
Expected: PASS (3 tests).

- [ ] **Step 5: Typecheck**

Run (from `ui/frontend/`): `npm run check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/components/pools/add-to-pool-dialog.tsx ui/frontend/src/components/pools/add-to-pool-dialog.test.tsx
git commit -m "feat: add Add-to-pool membership toggle dialog"
```

---

### Task 21: Wire Add-to-pool entry points

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/index.tsx`
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx`

> **Verify before implementing:** open both files and locate where each renders
> per-game actions (the library grid renders `GameCard`s; the detail page has an
> actions area near the title). The dialog needs a trigger button + local
> `useState` for the open game id. Wire pattern below; adapt to each file's
> existing action layout.

- [ ] **Step 1: Library — add an Add-to-pool affordance**

In `ui/frontend/src/routes/_authenticated/games/index.tsx`, add state and the
dialog. Because the library `GameGrid` already wires `onClick` to navigate, add
the entry point as a small per-card menu OR a selection-bar action. Minimal
viable wiring — a single dialog driven by a chosen game id:
```tsx
import { AddToPoolDialog } from '@/components/pools/add-to-pool-dialog';
// …inside the component:
const [poolDialogGameId, setPoolDialogGameId] = useState<string | null>(null);
// …render once, near the end of the returned JSX:
{poolDialogGameId && (
  <AddToPoolDialog
    userGameId={poolDialogGameId}
    open={!!poolDialogGameId}
    onOpenChange={(o) => !o && setPoolDialogGameId(null)}
  />
)}
```
Provide a trigger: pass an `actionsSlot` with a "Add to pool" button into the
grid's cards if the grid forwards slots, or add a `ListPlus` icon button to the
existing card hover actions. If `GameGrid` does not forward per-card actions,
add the entry point to the game-detail page only (Step 2) and the bulk selection
toolbar, and note the limitation in the PR description.

- [ ] **Step 2: Game detail — add an Add-to-pool button**

In `ui/frontend/src/routes/_authenticated/games/$id.index.tsx`, import the dialog
and add a button in the actions area near the title:
```tsx
import { ListPlus } from 'lucide-react';
import { AddToPoolDialog } from '@/components/pools/add-to-pool-dialog';
// …state:
const [showPoolDialog, setShowPoolDialog] = useState(false);
// …button in the actions row:
<Button variant="outline" onClick={() => setShowPoolDialog(true)}>
  <ListPlus className="mr-2 h-4 w-4" />
  Add to pool
</Button>
// …dialog (uses the route's user game id):
<AddToPoolDialog userGameId={userGameId} open={showPoolDialog} onOpenChange={setShowPoolDialog} />
```
(Use whatever variable holds the current user-game id in that route — likely from
`useParams`.)

- [ ] **Step 3: Typecheck + build**

Run (from `ui/frontend/`): `npm run check && npm run build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/games/
git commit -m "feat: add Add-to-pool entry points in library and game detail"
```

---

## Phase 6 — Integration & final gates

### Task 22: Full verification sweep

**Files:** none (verification only)

- [ ] **Step 1: Typecheck, dead-code, tests**

Run (from `ui/frontend/`):
```bash
npm run check && npm run knip && npm run test
```
Expected: all PASS. If knip flags an unused shadcn sub-export you newly required
(e.g. `Select` parts), confirm it is actually imported; if knip flags a genuinely
unused export, remove it.

- [ ] **Step 2: Confirm route tree is committed and current**

Run (from `ui/frontend/`):
```bash
npm run build && git status --porcelain ui/frontend/src/routeTree.gen.ts
```
Expected: empty output (no drift). If it printed a path, commit the regenerated
file.

- [ ] **Step 3: Manual smoke (optional but recommended)**

Use the `/run` skill or `./nexorious serve` to click through: create a pool, add
a filter, see suggestions, add a suggestion → candidate, promote → queue,
drag-reorder, set on deck, remove; toggle membership from the library and game
detail; reorder pools on the index. Confirm a finished play-status change makes a
game disappear from the pool after invalidation.

- [ ] **Step 4: Commit any fixes, then push**

```bash
git add -A
git commit -m "test: final Play Planning frontend verification fixes" || echo "nothing to fix"
git push -u origin feat/play-planning-frontend-956
```
Expected: pre-push hook runs `npm run check && npm run knip && npm run test` — all green.

---

### Task 23: Open the PR

- [ ] **Step 1: Create the PR**

```bash
gh pr create --title "feat: Play Planning frontend — pools page, nav, add-to-pool" --body "$(cat <<'EOF'
## What

Frontend half of Play Planning (#956): a **Planning** nav item + pools index
(create/edit/delete, drag-reorder), a **per-pool page** (Up Next queue,
Candidates, Suggestions), an **OR-of-cards filter editor** modal, and an
**Add-to-pool** membership toggle from the library and game detail. Built on the
already-shipped backend (#955/#968 + the membership endpoint #971/#973).

## Highlights

- New `api/pools.ts` + `use-pools.ts` mirroring the Tags pattern.
- Queue reorder/promote/demote/set-on-deck all map to the declarative
  `PUT /api/pools/:id/queue` (pure helpers in `lib/pool-queue.ts`, unit-tested).
- Filter (de)serialization with empty-card guarding (`lib/pool-filter.ts`, tested).
- Buy-first badge derivation (`lib/game-flags.ts`, tested).
- Add-to-pool membership merge (tested).
- dnd-kit added (npmDepsHash bumped); shared `ColorPicker` and `sortOptions`
  extracted for reuse.

## Tests

`pool-queue`, `pool-filter`, `game-flags`, `add-to-pool` unit tests; existing
`game-card` / `game-filters` suites still green.

Closes #956
EOF
)"
```

- [ ] **Step 2: Confirm CI is green, then report back to the user.**

---

## Self-Review

**Spec coverage:**
- Planning nav + pools index (create/edit/delete/reorder) → Tasks 9–12 ✓
- Per-pool stacked layout (Up Next / Candidates / Suggestions) → Tasks 15–18 ✓
- Suggestions-only box, server-sorted + paginated → Task 17 ✓
- Candidates client-sort, Suggestions server-sort, shared control → Tasks 13, 16, 17 ✓
- dnd-kit reorder (queue + pools index) → Tasks 1, 12, 15 ✓
- Add-to-pool toggle (full, #971 shipped) → Task 20–21 ✓
- Filter editor modal (OR-of-cards, MultiSelectFilter reuse, empty-card guard) → Tasks 6, 19 ✓
- Routes + nav + route tree regen + page titles → Tasks 12, 18, 22 ✓
- API client + hooks + optimistic-friendly invalidation → Tasks 3, 4 ✓
- Types mirroring Go DTOs + `pool_membership` on UserGame → Tasks 2, 3 ✓
- Buy-first badge derivation + GameCard slots → Tasks 7, 14 ✓
- ColorPicker extraction + nullable pool color → Tasks 9, 10 ✓
- States/errors (skeletons, empty states, 404 redirect, toast rollback) → Tasks 12, 17, 18, 20 ✓
- Finished-status auto-removal via invalidation → covered by hook invalidations (Task 4) + manual smoke (Task 22) ✓
- New npm dep + nix hash → Task 1 ✓
- Testing focus (queue mapping, filter round-trip, candidates sort, membership merge, buy-first) → Tasks 5, 6, 7, 16, 20 ✓

**Deviations from spec, called out:**
- Optimistic updates: the spec asks for optimistic `setQueue`/toggle with rollback.
  This plan ships **invalidation-based** refresh first (simpler, correct) and notes
  optimism as a follow-up enhancement. If instant-feel is required before merge,
  add `onMutate`/`onError` rollback to `useSetQueue` in Task 4 — flagged here so it
  is a conscious choice, not an omission.
- Library entry-point wiring (Task 21) depends on whether `GameGrid` forwards
  per-card actions; the plan degrades gracefully to detail-page + selection-bar if
  not, and says so.

**Type consistency:** `SortField`/`SortOrder` come from one module (`lib/sort-options.ts`),
consumed by `game-filters`, `pool-sort-control`, `candidates-grid`, `suggestions-grid`,
and the `$id` route. `setQueue`/`promoteToQueue`/`demoteFromQueue`/`setOnDeck`/`reorderQueue`
names match between `lib/pool-queue.ts`, its test, and consumers. `mergeMembership`
matches between dialog and test. `PoolFilter`/`FilterCard` shapes match `internal/filter/pool.go`.
