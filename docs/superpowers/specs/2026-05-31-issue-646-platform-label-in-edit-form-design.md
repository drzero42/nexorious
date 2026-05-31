# Issue #646: Show Platform in Edit Game Dialog Storefront Cards

## Problem

When editing a game, the "Platforms & Ownership" section renders one card per
`UserGamePlatform` entry. The card header label is computed with a storefront-first
fallback chain that produces only the storefront name (e.g. "GOG"). If a game is owned
on GOG for both Windows and Linux, both cards display "GOG" with no way to tell them
apart. The same storefront-only label appears in the playtime breakdown on the detail page.

## Root Cause

Two locations use a storefront-first fallback that never combines both dimensions:

```ts
// game-edit-form.tsx:418  and  $id.index.tsx:405
p.storefront_details?.display_name || p.storefront ||
p.platform_details?.display_name  || p.platform  || 'Unknown'
```

The data needed to disambiguate is already present: `platform_details.display_name` and
`storefront_details.display_name` are populated by the backend via
`Relation("PlatformRecord").Relation("StorefrontRecord")` and mapped to `UserGamePlatform`
on the frontend.

## Design

### Shared helper

Add `formatPlatformLabel` to `ui/frontend/src/lib/game-utils.ts`:

```ts
export function formatPlatformLabel(p: {
  platform?: string | null;
  storefront?: string | null;
  platform_details?: { display_name: string } | null;
  storefront_details?: { display_name: string } | null;
}): string {
  const platform  = p.platform_details?.display_name  || p.platform;
  const storefront = p.storefront_details?.display_name || p.storefront;
  if (platform && storefront) return `${platform} (${storefront})`;
  return platform || storefront || 'Unknown';
}
```

Format: `"Windows (GOG)"` when both are known; graceful single-value fallback otherwise.

The structural parameter type (not `UserGamePlatform` directly) keeps the helper usable
outside the game context if needed and avoids a circular import.

### Affected locations

| File | Location | Change |
|---|---|---|
| `ui/frontend/src/components/games/game-edit-form.tsx` | line ~418 | Replace 5-line fallback chain with `formatPlatformLabel(p)` |
| `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` | line ~405 | Replace 5-line fallback chain with `formatPlatformLabel(p)` |

The third label site in `$id.index.tsx` (lines 297-303, the Platforms & Ownership list in
the main game info card) already renders platform and storefront as separate styled
elements and is visually correct — it is left unchanged.

### No backend changes

All required data is already returned by the API. This is a frontend-only fix.

## Testing

- Existing `game-edit-form.test.tsx` and `$id.index.tsx` tests cover the render path;
  no new test cases required (the label logic is a pure function with no branching
  security or invariant concerns per the testing policy).
- Manual verification: a game owned on two platforms via the same storefront (e.g. GOG
  Windows + GOG Linux) should now show distinct card headers in the edit form and
  distinct rows in the playtime breakdown.
