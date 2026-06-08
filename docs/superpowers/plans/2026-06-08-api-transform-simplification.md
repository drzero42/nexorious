# API Response Transform Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Camp A API transforms (snake_case → snake_case) spread-based or identity-deleted so new fields stop being silently dropped, and de-duplicate the shared platform/tag transforms.

**Architecture:** Transforms that do real work (branded-id casts, nested transforms, defensive defaults) become spread + override; pure-identity transforms are deleted and responses typed directly as their domain types. `platforms.ts` becomes the single home for `transformPlatform`/`transformStorefront`; `games.ts` imports them.

**Tech Stack:** TypeScript, Vitest, MSW (`msw` `http`/`HttpResponse`) for HTTP mocking. Tests live beside source as `*.test.ts` under `ui/frontend/src/api/`. Run from `ui/frontend/`.

**Spec:** `docs/superpowers/specs/2026-06-08-api-transform-simplification-design.md`

**Working directory for all commands:** `ui/frontend/`

---

## File Structure

- `ui/frontend/src/api/platforms.ts` — export `transformPlatform`, `transformStorefront`, `PlatformApiResponse`, `StorefrontApiResponse` (canonical home).
- `ui/frontend/src/api/tags.ts` — delete `transformTag` + `TagApiResponse`; type responses as `Tag`.
- `ui/frontend/src/api/import-export.ts` — delete both transforms + their `*ApiResponse` interfaces; type responses as domain types.
- `ui/frontend/src/api/games.ts` — delete duplicate transforms/interfaces, import canonical ones from `./platforms`, rewrite work-doing transforms as spread + override.
- Tests added to the matching `*.test.ts` files.

A note on the boundary tests: every current hand-listed transform drops fields it does not enumerate, so a "an unexpected field survives" assertion **fails RED before the refactor** and passes GREEN after. To read an undeclared field off a typed result in a test, cast through `Record<string, unknown>`.

---

## Task 1: `platforms.ts` — export canonical transforms, prove fields survive

**Files:**
- Modify: `ui/frontend/src/api/platforms.ts`
- Test: `ui/frontend/src/api/platforms.test.ts`

- [ ] **Step 1: Write the failing test**

Add this block inside the top-level `describe('platforms.ts', ...)` in `ui/frontend/src/api/platforms.test.ts` (place it after the existing imports/setup; reuse the file's existing `API_URL`, `server`, `http`, `HttpResponse`):

```ts
describe('transform boundary', () => {
  it('passes an unexpected new field through getPlatform untouched', async () => {
    server.use(
      http.get(`${API_URL}/platforms/pc`, () =>
        HttpResponse.json({
          name: 'pc',
          display_name: 'PC',
          is_active: true,
          source: 'official',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
          brand_new_field: 'survives',
        }),
      ),
    );

    const result = await getPlatform('pc');

    expect((result as Record<string, unknown>).brand_new_field).toBe('survives');
  });

  it('passes an unexpected new field through getStorefront untouched', async () => {
    server.use(
      http.get(`${API_URL}/platforms/storefronts/steam`, () =>
        HttpResponse.json({
          name: 'steam',
          display_name: 'Steam',
          is_active: true,
          source: 'official',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
          brand_new_field: 'survives',
        }),
      ),
    );

    const result = await getStorefront('steam');

    expect((result as Record<string, unknown>).brand_new_field).toBe('survives');
  });
});
```

If `getPlatform`/`getStorefront` are not already imported at the top of the test file, add them to the existing import from `'./platforms'`.

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test platforms.test.ts`
Expected: FAIL — `brand_new_field` is `undefined` because the current hand-listed `transformPlatform`/`transformStorefront` only copy enumerated fields.

- [ ] **Step 3: Rewrite the transforms as spread + override and export them**

In `ui/frontend/src/api/platforms.ts`, replace the `transformStorefront` and `transformPlatform` function definitions (currently lines ~90–116) with:

```ts
export function transformStorefront(apiStorefront: StorefrontApiResponse): Storefront {
  return {
    ...apiStorefront,
    is_active: apiStorefront.is_active ?? false,
    source: apiStorefront.source ?? '',
    created_at: apiStorefront.created_at ?? '',
    updated_at: apiStorefront.updated_at ?? '',
  };
}

export function transformPlatform(apiPlatform: PlatformApiResponse): Platform {
  return {
    ...apiPlatform,
    is_active: apiPlatform.is_active ?? false,
    source: apiPlatform.source ?? '',
    storefronts: apiPlatform.storefronts?.map(transformStorefront),
    created_at: apiPlatform.created_at ?? '',
    updated_at: apiPlatform.updated_at ?? '',
  };
}
```

- [ ] **Step 4: Make the response types a shared, exported superset**

Still in `platforms.ts`, change the `PlatformApiResponse` and `StorefrontApiResponse` interfaces (lines ~8–30) to be `export`ed and to make the defaulted fields optional (so they are a safe superset that also covers the partial nested payloads `games.ts` defends against):

```ts
export interface PlatformApiResponse {
  name: string;
  display_name: string;
  icon_url?: string;
  igdb_platform_id?: number | null;
  is_active?: boolean;
  source?: string;
  default_storefront?: string;
  storefronts?: StorefrontApiResponse[];
  created_at?: string;
  updated_at?: string;
}

export interface StorefrontApiResponse {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
  is_active?: boolean;
  source?: string;
  created_at?: string;
  updated_at?: string;
}
```

- [ ] **Step 5: Run tests + typecheck to verify GREEN**

Run: `npm run test platforms.test.ts`
Expected: PASS (all platforms tests, including the two new ones).

Run: `npm run check`
Expected: no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/platforms.ts ui/frontend/src/api/platforms.test.ts
git commit -m "refactor(frontend): spread-based platform/storefront transforms, export as canonical"
```

---

## Task 2: `tags.ts` — delete identity transform, type as `Tag` directly

**Files:**
- Modify: `ui/frontend/src/api/tags.ts`
- Test: `ui/frontend/src/api/tags.test.ts`

- [ ] **Step 1: Write the failing test**

Add inside the top-level `describe` in `ui/frontend/src/api/tags.test.ts` (reuse the file's `API_URL`/`server`/`http`/`HttpResponse`; import `getTag` from `'./tags'` if not already imported):

```ts
describe('transform boundary', () => {
  it('passes an unexpected new field through getTag untouched', async () => {
    server.use(
      http.get(`${API_URL}/tags/tag-1`, () =>
        HttpResponse.json({
          id: 'tag-1',
          user_id: 'user-1',
          name: 'RPG',
          color: '#ff0000',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
          brand_new_field: 'survives',
        }),
      ),
    );

    const result = await getTag('tag-1');

    expect((result as Record<string, unknown>).brand_new_field).toBe('survives');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test tags.test.ts`
Expected: FAIL — `brand_new_field` is `undefined` because `transformTag` only copies enumerated fields.

- [ ] **Step 3: Delete `transformTag` and the `TagApiResponse` interface**

In `ui/frontend/src/api/tags.ts`:
- Delete the `TagApiResponse` interface (lines ~8–17).
- Delete the entire `transformTag` function (lines ~91–102) and its `Transformation Functions` comment banner.

- [ ] **Step 4: Type the API calls directly as `Tag` and drop the `.map(transformTag)` / `transformTag(...)` calls**

Update each call site in `tags.ts` so responses are typed as `Tag` and returned as-is:

```ts
// getTags
const response = await api.get<Tag[]>('/tags', { params: queryParams });
return {
  tags: response,
  total: response.length,
  page: 1,
  perPage: response.length,
  totalPages: 1,
};

// getAllTags
const response = await api.get<Tag[]>('/tags');
return response;

// getTag
const response = await api.get<Tag>(`/tags/${id}`);
return response;

// createTag
const response = await api.post<Tag>('/tags', {
  name: data.name,
  color: data.color,
  description: data.description,
});
return response;

// createOrGetTag — the response shape is { tag: Tag; created: boolean }
const response = await api.post<{ tag: Tag; created: boolean }>(
  '/tags/create-or-get',
  undefined,
  { params: queryParams },
);
return { tag: response.tag, created: response.created };

// updateTag
const response = await api.put<Tag>(`/tags/${id}`, requestBody);
return response;
```

Replace the `TagCreateOrGetApiResponse` usage in `createOrGetTag` with the inline `{ tag: Tag; created: boolean }` shown above, and delete the now-unused `TagCreateOrGetApiResponse` interface (lines ~19–22). Leave the `TagAssign*`/`TagRemove*` camelCase mappers untouched — those are Camp B-style renames and out of scope.

- [ ] **Step 5: Run tests + typecheck to verify GREEN**

Run: `npm run test tags.test.ts`
Expected: PASS (all tags tests, including the new one).

Run: `npm run check`
Expected: no TypeScript errors and no unused-symbol errors. If `knip` later flags a now-unused import of `Tag`, keep it — `Tag` is used in the typed calls above.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/tags.ts ui/frontend/src/api/tags.test.ts
git commit -m "refactor(frontend): drop identity transformTag, type tag responses as Tag"
```

---

## Task 3: `import-export.ts` — delete identity transforms

**Files:**
- Modify: `ui/frontend/src/api/import-export.ts`
- Test: `ui/frontend/src/api/import-export.test.ts`

- [ ] **Step 1: Write the failing test**

Add inside the top-level `describe` in `ui/frontend/src/api/import-export.test.ts` (reuse the file's existing `API_URL`/`server`/`http`/`HttpResponse`; the export functions hit `POST /export/json`). First inspect the file to confirm the mock helper names, then add:

```ts
describe('transform boundary', () => {
  it('passes an unexpected new field through exportCollectionJson untouched', async () => {
    server.use(
      http.post(`${API_URL}/export/json`, () =>
        HttpResponse.json({
          job_id: 'job-1',
          status: 'pending',
          message: 'started',
          estimated_items: 5,
          brand_new_field: 'survives',
        }),
      ),
    );

    const result = await exportCollectionJson();

    expect((result as Record<string, unknown>).brand_new_field).toBe('survives');
  });
});
```

Import `exportCollectionJson` from `'./import-export'` if not already imported.

- [ ] **Step 2: Run test to verify it fails**

Run: `npm run test import-export.test.ts`
Expected: FAIL — `brand_new_field` is `undefined`.

- [ ] **Step 3: Delete both transforms and their `*ApiResponse` interfaces**

In `ui/frontend/src/api/import-export.ts`:
- Delete the `ImportJobApiResponse` and `ExportJobApiResponse` interfaces (lines ~8–21).
- Delete `transformImportJobResponse` and `transformExportJobResponse` (lines ~27–44) and the `Transformation Functions` banner.

- [ ] **Step 4: Type the calls directly as the domain types**

Update each call site to type the response as the domain type and return it directly:

```ts
// importNexoriousJson
const response = await apiUploadFile<ImportJobCreatedResponse>('/import/nexorious', file);
return response;

// importDarkadiaCsv
const response = await apiUploadFile<ImportJobCreatedResponse>('/import/darkadia', file);
return response;

// exportCollectionJson
const response = await api.post<ExportJobCreatedResponse>('/export/json');
return response;

// exportCollectionCsv
const response = await api.post<ExportJobCreatedResponse>('/export/csv');
return response;
```

`ImportJobCreatedResponse` / `ExportJobCreatedResponse` are already imported from `@/types` at the top of the file — keep that import.

- [ ] **Step 5: Run tests + typecheck to verify GREEN**

Run: `npm run test import-export.test.ts`
Expected: PASS.

Run: `npm run check`
Expected: no TypeScript errors.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/import-export.ts ui/frontend/src/api/import-export.test.ts
git commit -m "refactor(frontend): drop identity import/export transforms, type responses directly"
```

---

## Task 4: `games.ts` — import canonical transforms, spread the rest, fix `igdb_platform_id`

**Files:**
- Modify: `ui/frontend/src/api/games.ts`
- Test: `ui/frontend/src/api/games.test.ts`

- [ ] **Step 1: Write the failing tests**

Add inside the top-level `describe('games.ts', ...)` in `ui/frontend/src/api/games.test.ts` (reuse existing `API_URL`/`server`/`http`/`HttpResponse`; `searchIGDB` and `getUserGame` are already imported):

```ts
describe('transform boundary', () => {
  it('keeps an unexpected new field on IGDB search results', async () => {
    server.use(
      http.post(`${API_URL}/games/search/igdb`, () =>
        HttpResponse.json({
          games: [
            {
              igdb_id: 42,
              title: 'Surprise',
              platforms: [],
              brand_new_field: 'survives',
            },
          ],
          total: 1,
        }),
      ),
    );

    const result = await searchIGDB('surprise');

    expect((result[0] as Record<string, unknown>).brand_new_field).toBe('survives');
  });

  it('keeps igdb_platform_id on a nested platform_details object', async () => {
    server.use(
      http.get(`${API_URL}/user-games/ug-1`, () =>
        HttpResponse.json({
          id: 'ug-1',
          game: {
            id: 1,
            title: 'Test',
            rating_count: 0,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
          is_loved: false,
          play_status: PlayStatus.PLAYING,
          hours_played: 0,
          platforms: [
            {
              id: 'p-1',
              platform: 'pc',
              is_available: true,
              hours_played: 0,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
              platform_details: {
                name: 'pc',
                display_name: 'PC',
                igdb_platform_id: 6,
                is_active: true,
                source: 'official',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
            },
          ],
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
        }),
      ),
    );

    const result = await getUserGame('ug-1');

    expect(result.platforms[0].platform_details?.igdb_platform_id).toBe(6);
  });
});
```

`PlayStatus` and `OwnershipStatus` are already imported at the top of `games.test.ts`. Confirm the single-user-game endpoint path against the existing `getUserGame` test in the file and match it (`/user-games/:id`); adjust the URL above if the existing test uses a different path.

- [ ] **Step 2: Run tests to verify they fail**

Run: `npm run test games.test.ts`
Expected: FAIL on both new tests — `brand_new_field` is dropped by `transformIGDBGameCandidate`, and `igdb_platform_id` is dropped because the local `PlatformApiResponse` omits it.

- [ ] **Step 3: Delete the duplicate interfaces and transforms; import the canonical ones**

In `ui/frontend/src/api/games.ts`:
- Delete the local `PlatformApiResponse` interface (lines ~50–60), the local `StorefrontApiResponse` interface (lines ~62–71), and the local `TagApiResponse` interface (lines ~87–96).
- Delete the local `transformPlatform` (lines ~223–235), `transformStorefront` (lines ~237–248), and `transformTag` (lines ~270–281) functions.
- Update the import block at the top to pull the canonical symbols from `./platforms`. Replace the existing `import type { Platform, Storefront } from '@/types/platform';` line with:

```ts
import {
  transformPlatform,
  transformStorefront,
  type PlatformApiResponse,
  type StorefrontApiResponse,
} from './platforms';
```

`Tag` is already imported from `@/types` — keep it; it now types the `tags` passthrough.

- [ ] **Step 4: Rewrite the remaining transforms as spread + override**

Replace `transformUserGamePlatform`, `transformGame`, `transformUserGame`, and `transformIGDBGameCandidate` in `games.ts` with:

```ts
function transformUserGamePlatform(apiPlatform: UserGamePlatformApiResponse): UserGamePlatform {
  return {
    ...apiPlatform,
    platform_details: apiPlatform.platform_details
      ? transformPlatform(apiPlatform.platform_details)
      : undefined,
    storefront_details: apiPlatform.storefront_details
      ? transformStorefront(apiPlatform.storefront_details)
      : undefined,
  };
}

function transformGame(apiGame: GameApiResponse): Game {
  return { ...apiGame, id: apiGame.id as GameId };
}

function transformUserGame(apiUserGame: UserGameApiResponse): UserGame {
  return {
    ...apiUserGame,
    id: apiUserGame.id as UserGameId,
    game: transformGame(apiUserGame.game),
    platforms: (apiUserGame.platforms ?? []).map(transformUserGamePlatform),
    tags: apiUserGame.tags,
  };
}

function transformIGDBGameCandidate(apiCandidate: IGDBGameCandidateApiResponse): IGDBGameCandidate {
  return { ...apiCandidate, igdb_id: apiCandidate.igdb_id as GameId };
}
```

The `UserGamePlatformApiResponse` interface still references `PlatformApiResponse`/`StorefrontApiResponse` for `platform_details`/`storefront_details` — those names now resolve to the imported canonical types, so no change is needed there. The `UserGameApiResponse.tags` field still references `TagApiResponse`; change that one field to `tags?: Tag[];` (since the local `TagApiResponse` is gone and `Tag` ≡ the response shape).

- [ ] **Step 5: Run tests + typecheck to verify GREEN**

Run: `npm run test games.test.ts`
Expected: PASS — both new tests pass; all existing tests stay green (existing fixtures carry only declared fields, so spread output matches prior shape).

Run: `npm run check`
Expected: no TypeScript errors.

Run: `npm run knip`
Expected: no new findings (all imported symbols are used).

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/games.ts ui/frontend/src/api/games.test.ts
git commit -m "refactor(frontend): spread games transforms, reuse canonical platform/tag transforms"
```

---

## Task 5: Final verification

- [ ] **Step 1: Run the full frontend gate**

Run (from `ui/frontend/`): `npm run check && npm run knip && npm run test`
Expected: all pass. This is the same gate the pre-push hook enforces.

- [ ] **Step 2: Confirm no Camp A transform survives as a hand-listed enumerator**

Run (from repo root): `grep -rn "transformTag\|transformImportJobResponse\|transformExportJobResponse" ui/frontend/src`
Expected: no matches (all three deleted).

Run: `grep -rn "function transformPlatform\|function transformStorefront" ui/frontend/src`
Expected: exactly one match each, both in `api/platforms.ts`.

- [ ] **Step 3: Push and open the PR** (only when the user asks — see CLAUDE.md branch workflow)

PR title (Conventional Commits, drives release-please): `refactor(frontend): simplify API response transforms to stop silently dropping fields`

PR body must include:
```
Closes #860
```
and a short note that Camp B was intentionally left as-is per the design doc, and that the change incidentally fixes `igdb_platform_id` being dropped on nested `platform_details`.

---

## Self-Review notes

- **Spec coverage:** snake_case files spread/identity-typed (Tasks 1–4); Camp B decision documented + untouched (spec + Task 5 grep); existing suites green + per-module boundary test (Tasks 1–4 Step 1); duplicates removed (Task 4 Step 3, Task 5 Step 2). The `igdb_platform_id` fix has a dedicated test (Task 4 Step 1).
- **Type consistency:** `transformPlatform`/`transformStorefront` exported from `platforms.ts` (Task 1) are the exact names imported in `games.ts` (Task 4). `PlatformApiResponse`/`StorefrontApiResponse` are exported from `platforms.ts` and consumed by `games.ts`'s `UserGamePlatformApiResponse`.
- **No placeholders:** every code/edit step shows the actual code.
