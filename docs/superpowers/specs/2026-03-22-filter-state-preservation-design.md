# Filter State Preservation on Back Navigation

**Date:** 2026-03-22
**Status:** Approved

## Problem

When a user navigates from the games list (`/games?q=foo&status=completed&...`) to a game detail page and then back, all active filters are lost. The "Back to Games" buttons in both the game detail and game edit pages navigate to bare `/games`, discarding the URL search params that encode the filter, sort, view, and pagination state.

Affected navigation paths:
- `/games` → click game → `/games/$id` → "Back to Games" → `/games` ❌ (filters lost)
- `/games` → click game → `/games/$id` → Edit → `/games/$id/edit` → Cancel → `/games/$id` → "Back to Games" → `/games` ❌ (filters lost)
- `/games` → click game → `/games/$id` → delete game → `/games` ❌ (filters lost)
- `/games/$id/edit` error state → "Back to Games" → `/games` ❌ (filters lost)

## Design

### Mechanism: Session Storage Return URL

When the user clicks a game from the games list, save the current URL search string to `sessionStorage` before navigating away. All "Back to Games" navigations read from this stored value to reconstruct the correct `/games?...` URL.

**Important:** Session storage is used only to remember the return URL so the correct URL can be constructed. The games list page remains URL-driven — it reads all filter/sort/view/pagination state from the URL search params, not from session storage.

### Storage Key

```
games_list_return_url
```

Stores the raw `window.location.search` value (e.g. `?q=foo&status=completed&sort=title`). When the user is on `/games` with no active filters, `window.location.search` is `''` (empty string) and that empty string is stored. Both a missing key (`null` from `sessionStorage.getItem`) and a stored empty string produce the same return URL of `'/games'` — both correctly mean "no active filters". This is by design.

`window.location.search` is used rather than serializing the router's `search` object because TanStack Router writes filter state directly to the browser URL as standard query string params, so `window.location.search` and the router's view of search params are always in sync for this project.

### Helper

A small inline helper (duplicated in each consuming file — no shared module needed) builds the return URL:

```ts
function getGamesReturnUrl(): string {
  return '/games' + (sessionStorage.getItem('games_list_return_url') ?? '');
}
```

`sessionStorage.getItem` returns `null` when the key is absent and `''` when the key was stored as an empty string. The `??` operator falls back only on `null`, but both cases resolve to `'/games'` (correct behaviour in both cases).

### Changes

#### `frontend/src/routes/_authenticated/games/index.tsx`

In `handleClickGame`, save `window.location.search` before navigating:

```ts
const handleClickGame = (game: UserGame) => {
  sessionStorage.setItem('games_list_return_url', window.location.search);
  navigate({ to: '/games/$id', params: { id: game.id } });
};
```

#### `frontend/src/routes/_authenticated/games/$id.index.tsx`

Add `getGamesReturnUrl` helper. Replace the three `navigate({ to: '/games' })` calls, listed in file order:

1. **`handleDelete`** (fires after game deletion) — navigate to return URL so the user lands back on their filtered list
2. **"Back to Games" button in the error state** — shown when the game cannot be loaded
3. **"Back to Games" button in the normal header** — the primary back navigation

#### `frontend/src/routes/_authenticated/games/$id.edit.tsx`

Add `getGamesReturnUrl` helper. Replace the one `navigate({ to: '/games' })` call in the **error state** "Back to Games" button.

The `game-edit-form.tsx` `handleSave` navigates to `/games/$id` on success (back to the detail page) — this is intentional. The user is then one step from the games list, and the "Back to Games" button in `$id.index.tsx` handles filter restoration from there. `handleSave` does not need to change.

Similarly, the `game-edit-form.tsx` Cancel button navigates to `/games/$id` — also correct and unchanged.

### What Does Not Change

- The games list page — URL is and remains the sole source of truth for filter state
- `game-edit-form.tsx` — Cancel and Save navigation are correct as-is
- No new files, no new hooks, no URL param threading

## Fallback Behaviour

If `games_list_return_url` is not present in session storage (e.g. user navigated directly to the game detail via bookmark or URL bar), "Back to Games" navigates to bare `/games` with no filters applied.

## Testing

### `$id.index.test.tsx` (new file)

Create `frontend/src/routes/_authenticated/games/$id.index.test.tsx`.

Cover:
1. **Normal "Back to Games" button** — with a `games_list_return_url` stored in sessionStorage, clicking navigates to `/games` + the stored search string
2. **Error state "Back to Games" button** — same as above but rendered in the error/not-found state
3. **`handleDelete`** — after deletion, navigate to the return URL
4. **Fallback (no key)** — when sessionStorage has no `games_list_return_url`, all three paths above fall back to bare `/games`

### `$id.edit.test.tsx` (new file)

Create `frontend/src/routes/_authenticated/games/$id.edit.test.tsx`.

Cover:
1. **Error state "Back to Games" button** — with a `games_list_return_url` in sessionStorage, clicking navigates to `/games` + the stored search string
2. **Fallback (no key)** — falls back to bare `/games`
