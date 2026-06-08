# API response transform simplification (#860)

## Problem

Every `ui/frontend/src/api/*.ts` module converts raw backend JSON into domain
objects through hand-written `transform*` functions that enumerate each field.
Adding or changing a field means editing it in three places — the
`*ApiResponse` interface, the domain type, and the transform — and forgetting
the transform step silently drops the field at runtime with no error.

This is not hypothetical. It caused the regression found while implementing #856
(PR #859): the backend returned `user_game_id`, but `transformIGDBGameCandidate`
did not copy it, so it vanished before reaching the UI.

## Scope

This work addresses **Camp A only** — the modules whose domain types use
**snake_case keys identical to the API response**:

- `games.ts`
- `tags.ts`
- `platforms.ts`
- `import-export.ts`

**Camp B is explicitly out of scope.** The camelCase modules (`jobs.ts`,
`sync.ts`, `auth.ts`, `events.ts`, `admin.ts`, `backup.ts`) rename
snake_case → camelCase, so spread cannot apply and the rename is doing real
work. There, a forgotten field surfaces as a TypeScript error on the camelCase
consumer (the field is *missing* from the camelCase type), so the silent-drop
risk is already low. Leaving them untouched keeps this PR focused on the actual
bug class.

## Verified facts

- `Tag` (`@/types/game.ts`) is **structurally identical** to `TagApiResponse` —
  same fields, same optionality. `transformTag` is a pure identity copy.
- `Platform` / `Storefront` (`@/types/platform.ts`) are **structurally
  identical** to the `PlatformApiResponse` / `StorefrontApiResponse` defined in
  `platforms.ts` (including `igdb_platform_id`).
- `games.ts` defines its **own** `PlatformApiResponse` that **omits
  `igdb_platform_id`** and makes `is_active`/`source`/`created_at`/`updated_at`
  optional. Its local `transformPlatform` therefore **drops `igdb_platform_id`**
  on nested `platform_details` — a latent instance of this very bug.
- `transformImportJobResponse` / `transformExportJobResponse` are pure identity
  copies.
- **No Camp A transform is exported or imported by any other file.** The whole
  refactor is localized to these four modules — no consumer-side churn.

## Approach

Transforms that do *real work* (branded-id casts, nested transforms, defensive
defaults) become **spread + override**. Transforms that are *pure identity* are
**deleted entirely** — with no transform, there is nothing to forget, which is
the strongest form of "structurally impossible to silently drop a field".

### `tags.ts`

- Delete `transformTag`.
- Drop the `TagApiResponse` interface.
- Type `/tags` responses directly as `Tag` / `Tag[]`; the api functions return
  the response as-is.

### `import-export.ts`

- Delete `transformImportJobResponse` and `transformExportJobResponse`.
- Drop `ImportJobApiResponse` / `ExportJobApiResponse`.
- Type the upload/post responses directly as `ImportJobCreatedResponse` /
  `ExportJobCreatedResponse`.

### `platforms.ts` — canonical home

- Becomes the **single source** for `transformPlatform` / `transformStorefront`
  and the shared `PlatformApiResponse` / `StorefrontApiResponse` types. Export
  all four.
- Keep the defensive `?? false` / `?? ''` defaults — they protect against
  partial nested payloads and adding them at top level (where fields are
  already required) is harmless.

### `games.ts`

- Delete the duplicate `transformTag` / `transformPlatform` /
  `transformStorefront` and the duplicate `PlatformApiResponse` /
  `StorefrontApiResponse` / `TagApiResponse` interfaces.
- Import the canonical `transformPlatform` / `transformStorefront` and their
  response types from `./platforms`; import `Tag` for nested tag typing. (No
  import cycle — `platforms.ts` does not import `games.ts`.)
- Rewrite the work-doing transforms as spread + override:

  ```ts
  function transformGame(api: GameApiResponse): Game {
    return { ...api, id: api.id as GameId };
  }

  function transformIGDBGameCandidate(
    api: IGDBGameCandidateApiResponse,
  ): IGDBGameCandidate {
    return { ...api, igdb_id: api.igdb_id as GameId };
  }

  function transformUserGamePlatform(
    api: UserGamePlatformApiResponse,
  ): UserGamePlatform {
    return {
      ...api,
      platform_details: api.platform_details
        ? transformPlatform(api.platform_details)
        : undefined,
      storefront_details: api.storefront_details
        ? transformStorefront(api.storefront_details)
        : undefined,
    };
  }

  function transformUserGame(api: UserGameApiResponse): UserGame {
    return {
      ...api,
      id: api.id as UserGameId,
      game: transformGame(api.game),
      platforms: (api.platforms ?? []).map(transformUserGamePlatform),
      tags: api.tags, // Tag ≡ TagApiResponse — flows through unchanged
    };
  }
  ```

- `UserGamePlatformApiResponse.platform_details` is retyped to the canonical
  `PlatformApiResponse`, which **fixes the dropped `igdb_platform_id`** as a
  side effect.

## Behavioral notes

- Spread copies every field present on the response object. Because the old
  hand-written transforms already copied every declared field, output for
  existing (well-formed) responses is unchanged. The new behavior is purely
  additive: undeclared/new fields now survive instead of being dropped.
- The one intentional fix is `igdb_platform_id` now surviving on nested
  `platform_details`.

## Testing

- Add **one boundary test per Camp A module** asserting that an unexpected/new
  field on the API response survives to the domain object. For the
  delete-transform modules (`tags`, `import-export`) the test guards against a
  future lossy transform being reintroduced; for `games` it exercises
  `transformGame` / `transformUserGame` / `transformIGDBGameCandidate` /
  `transformUserGamePlatform` and `platforms` exercises the spread path.
- Existing api test suites must stay green. `toEqual` assertions on fixtures
  that carry only declared fields remain valid; any that drift get updated.
- Run the affected suites directly (`npm run test games.test.ts`, etc.); the
  full gate runs on `git push`.

## Acceptance criteria (from #860)

- [x] Adding a plain field to a snake_case API response requires no transform
      edit. (Achieved via spread / direct typing.)
- [x] A decision for the Camp B camelCase files: **leave as-is**, documented
      above.
- [x] Existing API test suites stay green; a per-module boundary test asserts a
      new field survives.
- [x] Duplicate transforms de-duplicated: `transformTag`,
      `transformPlatform`, `transformStorefront` now have a single home.
