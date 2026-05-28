# Loved filter — design

**Issue:** [#600 "Missing filter: loved"](https://github.com/Nexorious/nexorious/issues/600)
**Date:** 2026-05-28
**Scope:** Frontend-only

## Problem

The games library page lets users filter by play status, ownership, platforms, storefronts, genres, game modes, themes, perspectives, and tags — but not by whether a game is marked as "loved". The `is_loved` field is fully wired through the backend and API client; only the UI control and URL plumbing are missing.

## Current state

The backend and API client already support filtering by `is_loved`:

- `internal/filter/criteria.go` — `ApplyIsLoved(fb, *bool)` filters on `ug.is_loved`.
- `internal/api/user_games.go` — reads `?is_loved=true|false` and calls `ApplyIsLoved` in both `HandleListUserGames` and `HandleFilterOptions`.
- `ui/frontend/src/api/games.ts` — `GetUserGamesParams.isLoved?: boolean` exists and is serialized to `?is_loved=true|false` via `appendParam`.

The gap is purely frontend.

## Design

### Control

A three-option `<Select>` mirroring the existing Play Status / Ownership Status controls:

| `<Select>` value | `isLoved` | Label        |
| ---------------- | --------- | ------------ |
| `"all"`          | `undefined` | "All games"   |
| `"true"`         | `true`    | "Loved only" |
| `"false"`        | `false`   | "Not loved"  |

### Placement

Primary filters row, after **Ownership Status** and before **Platforms**. The issue argues "loved" is a first-class personal attribute and belongs alongside the other status filters — this matches that intent.

Width: `w-36`. The labels are short; the existing Play Status (`w-40`) and Ownership (`w-44`) values inform the choice.

### URL parameter

- `?loved=true` → `isLoved: true`
- `?loved=false` → `isLoved: false`
- absent or any other value → `isLoved: undefined`

Consistent with how existing filters use shortened URL names (`ownership` for `ownershipStatus`, `platform` for `platforms`, etc.). The API client converts back to `is_loved` when calling the backend, which is already implemented.

## Files to change

| File | Change |
|---|---|
| `ui/frontend/src/components/games/game-filters.tsx` | Add `isLoved?: boolean` to `GameFiltersProps['filters']`; render the `<Select>` in the primary row; update `hasActiveFilters` to include `filters.isLoved !== undefined`; update `clearFilters` to reset `isLoved: undefined`. |
| `ui/frontend/src/routes/_authenticated/games/index.tsx` | Parse `?loved` URL param in the `filters` memo; include `isLoved` in the inline `handleFiltersChange` type and map it to `loved` in `updateParams`; include `isLoved` in `filterFields` so it flows through to `useUserGames` / `useUserGameIds`. |

## Out of scope

- Backend changes (already done).
- API client changes (already done).
- Tests. The change is thin UI wiring with no non-trivial logic; per project policy ("Do NOT write a test when the function is a thin wrapper… only verifies that calling the function returns what it computes"), no new tests are warranted. The backend filter that does the work is already in place.
- A "loved" column in the list view, or sorting by loved status. Not requested by the issue.

## Acceptance criteria

1. Selecting "Loved only" or "Not loved" filters the games grid/list to match.
2. The selection persists across page reloads via the `?loved` URL parameter.
3. "Clear" resets the loved filter alongside the others.
4. The active-filter indicator behaves correctly when only the loved filter is set.
5. `npm run check` and `npm run knip` pass; the existing frontend test suite still passes.
