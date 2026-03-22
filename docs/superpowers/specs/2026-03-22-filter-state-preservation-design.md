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

Stores the raw `window.location.search` value (e.g. `?q=foo&status=completed&sort=title`). An empty string or missing key means "no active filters".

### Helper

A small inline helper builds the return URL:

```ts
function getGamesReturnUrl(): string {
  return '/games' + (sessionStorage.getItem('games_list_return_url') ?? '');
}
```

### Changes

#### `frontend/src/routes/_authenticated/games/index.tsx`

In `handleClickGame`, save the current search string before navigating:

```ts
const handleClickGame = (game: UserGame) => {
  sessionStorage.setItem('games_list_return_url', window.location.search);
  navigate({ to: '/games/$id', params: { id: game.id } });
};
```

#### `frontend/src/routes/_authenticated/games/$id.index.tsx`

Add `getGamesReturnUrl` helper. Replace all three `navigate({ to: '/games' })` calls:

1. "Back to Games" button (normal state)
2. "Back to Games" button (error state)
3. `handleDelete` — navigate to return URL after deletion

#### `frontend/src/routes/_authenticated/games/$id.edit.tsx`

Add `getGamesReturnUrl` helper. Replace the one `navigate({ to: '/games' })` call in the error state.

### What Does Not Change

- The games list page — URL is and remains the sole source of truth for filter state
- `game-edit-form.tsx` Cancel button — correctly navigates back to the game detail page (`/games/$id`); filter restoration is handled from there
- No new files, no new hooks, no URL param threading

## Fallback Behaviour

If `games_list_return_url` is not present in session storage (e.g. user navigated directly to the game detail via bookmark or URL bar), "Back to Games" navigates to bare `/games` with no filters applied.

## Testing

Add a test to the game detail component verifying that "Back to Games" navigates to the stored sessionStorage URL rather than bare `/games` when a return URL is present.
