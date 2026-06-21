# Remember filters and sorting in the library view

**Issue:** [#1129](https://github.com/drzero42/nexorious/issues/1129)
**Date:** 2026-06-21
**Status:** Approved design

## Problem

The library view (`/games`) keeps its entire state — filters, sort field/order,
view mode, per-page, search query, and page — in URL search params. Nothing is
persisted. Navigating to `/games` fresh (e.g. via the sidebar "Library" link,
which is a plain `<Link to="/games">` with no search params) starts from
defaults every time: `sort=title/asc`, no filters, grid view, 50 per page. A
user who always browses with a particular filter/sort has to re-apply it on
every visit.

Separately, the existing "Clear filters" button is a `variant="ghost"` control
that visually blends into the page — on a narrow screen (Firefox on Android) it
is easy to miss entirely.

## Goal

Remember the **entire** library view state across sessions and restore it when
the user returns to `/games` with no explicit params. The original issue asked
to exclude the search query, but during brainstorming the requirement was
updated: search query and page number should also be remembered. So there is no
carve-out — the whole view state is persisted.

Also make the existing "Clear" button easier to spot.

### Non-goals

- No server-side persistence. Preferences live in browser `localStorage`
  (per-browser, not synced across devices, not in backups). This was an explicit
  choice to avoid backend/migration work.
- No new "reset" button. The existing "Clear" button already does what was
  wanted; it just needed to be more visible.

## Approach

Mirror the URL search-param object to `localStorage`. The page already keeps
100% of its state in URL search params, so persistence is a matter of writing
that object on change and restoring it on a fresh landing. Deep/shared links
with explicit params continue to win — we only hydrate from storage when the
URL has no params.

## Design

### 1. New helper: `ui/frontend/src/lib/library-prefs.ts`

A small, testable module that owns all `localStorage` access:

- `const KEY = 'nexorious:library-view:v1'` — versioned key so a future change
  to the persisted shape can be ignored safely rather than mis-parsed.
- `saveLibraryPrefs(search: Record<string, string | string[]>): void` —
  `JSON.stringify` and write. Wrapped in try/catch so a quota or serialization
  error never breaks navigation.
- `loadLibraryPrefs(): Record<string, string | string[]> | null` — read and
  `JSON.parse` inside try/catch; return `null` on missing or corrupt data.

### 2. Write on every change

In `GamesPageContent.updateParams` (`ui/frontend/src/routes/_authenticated/games/index.tsx`)
— the single chokepoint through which every URL mutation flows — call
`saveLibraryPrefs(params)` after the new `params` object is computed (alongside
the existing `navigate(...)`).

Because filters, sort, view mode, per-page, search, and page all flow through
`updateParams`, the whole view state is mirrored automatically. This includes
the **Clear** action (it calls `onFiltersChange` → `handleFiltersChange` →
`updateParams`), so clearing writes the cleared state and it sticks on the next
visit.

### 3. Hydrate on a fresh landing

A `useEffect` that runs once on mount:

- If the current `search` object has **no keys** (empty params) **and**
  `loadLibraryPrefs()` returns a non-null saved object, call
  `navigate({ to: '/games', search: saved, replace: true })`.
- The "empty params" guard means a deep/shared link such as `?status=playing`
  is never overridden — explicit params win.
- `replace: true` avoids adding a history entry for the hydration redirect.

The existing effect that resets `page` when it exceeds the available page count
(`index.tsx:192`) already protects against a stale saved page pointing past the
end of a now-smaller library, so no extra guard is needed for that case.

### 4. Clear button visibility

In `ui/frontend/src/components/games/game-filters.tsx` (the Clear button,
currently around line 286), change `variant="ghost"` to `variant="outline"` so
it has a visible border/background like the adjacent "More filters" button,
instead of blending into the page. No layout or behavior change — only
visibility.

### 5. Interaction with existing sessionStorage

This is independent of the existing
`sessionStorage('games_list_return_url')` mechanism, which restores state for
the per-tab back-navigation after viewing a game's detail page. The two coexist:
`sessionStorage` handles in-session round-trips; the new `localStorage` handles
cross-session restore.

## Testing

- Unit-test `library-prefs.ts`: round-trip save → load; `loadLibraryPrefs()`
  returns `null` when the key is absent; `loadLibraryPrefs()` returns `null`
  (no throw) on corrupt JSON.
- A focused render test of the library page: landing with empty params hydrates
  the URL from a saved object; landing with non-empty params does **not**
  hydrate (deep link wins).
- The existing `game-filters.test.tsx` continues to cover the Clear button; the
  variant change needs no new behavioral test.

No backend changes and no database migration.

## Trade-offs

- Remembering search + page means returning after a long gap can open the
  library filtered to an old search term and/or on a deep page. This is the
  desired behavior per the updated requirement; the now-more-visible **Clear**
  button is the escape hatch.
- `localStorage` is per-browser: preferences do not follow the user to another
  device or browser, and are not captured by backup/export. Accepted as the
  cost of avoiding backend work.
