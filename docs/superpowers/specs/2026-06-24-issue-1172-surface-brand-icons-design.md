# Surface platform & storefront icons across the UI — Design

**Issue:** #1172
**Status:** Settled (came out of a brainstorm with a visual companion; decisions in the issue are locked). This spec records those decisions against the actual code so the plan can be written against verified anchors.
**Related:** #1171 (theme enablement — landed), #1173 (sourcing the missing icon assets — independent).

## Problem

Storefront icons (`icon_url`) render **only** in the sync section today. Platform icons render on game cards/lists via `PlatformIcon`/`PlatformIconList`, but game-detail rows, the add/edit selectors, and the filter facets are all text-only — even though `platform_details` and `storefront_details` (each carrying `icon_url`) are already present in the data. The platform icon component also hardcodes a generic `Monitor` glyph in the selectors instead of the real per-platform asset.

## Goals

- A single shared icon primitive (`BrandIcon`) that both platforms and storefronts render through, so the two never drift.
- Surface platform **and** storefront icons in: game-detail "Platforms & Ownership" rows, the storefront selector + platform picker, and the Platforms/Storefronts filter facets.
- Theme-aware icon variant selection (light/dark) with a graceful fallback when a dark asset is missing.
- Missing icon → existing first-letter badge fallback (real assets are sourced in #1173).

## Non-goals (explicitly decided in the issue)

- Storefront icons on game **cards** and the game **list** (kept platform-only to avoid clutter).
- Stats page (no frontend stats page exists).
- Sync section (already renders storefront icons).
- No backend or type changes — `Platform.icon_url` and `Storefront.icon_url` already exist (`src/types/platform.ts`), and `storefront_details?: Storefront` is already on the game-platform type (`src/types/game.ts`).

## Verified code anchors

| Area | File | Current state |
|---|---|---|
| Platform icon | `src/components/ui/platform-icon.tsx` | `PlatformIcon` (img → first-letter fallback → optional tooltip/label) + `PlatformIconList` wrapper. `iconUrl = config.staticUrl + platform.icon_url`. |
| Storefront label | `src/components/storefront-link.tsx` | `StorefrontLabel({displayName, storeUrl})` renders `(displayName)` text, optionally wrapped in an external `<a>`. |
| Game-detail rows | `src/routes/_authenticated/games/$id.index.tsx:483–516` | Left: `display_name` + `<StorefrontLabel>`. Right: achievements / ownership badge / acquired date. |
| Storefront selector | `src/components/ui/platform-selector.tsx:77–106` | Plain `<Select>` of `storefront.display_name`. |
| Platform picker | `src/components/ui/platform-selector.tsx` | Combobox (Popover+Command) trigger L182, command item L217, compact checkbox row L432, empty-state L399 — all use hardcoded `Monitor`. |
| Filter facets | `src/components/games/game-filters.tsx:73–81, 258, 298` | `platformOptions`/`storefrontOptions` are `{value,label}`; rendered via `MultiSelectFilter`. |
| Facet component | `src/components/ui/multi-select-filter.tsx` | `MultiSelectOption = {value,label}`; renders a checkbox + `<span>{label}</span>` per option. |
| Theme | `next-themes` `useTheme()` available (landed in #1171); `resolvedTheme` is `'light' | 'dark'`. |

## Design

### 1. `BrandIcon` primitive

Extract the inner render of today's `PlatformIcon` into a generic `BrandIcon` in `src/components/ui/brand-icon.tsx`. It is **brand-neutral** — it knows nothing about "platform" vs "storefront", only `iconUrl` + `displayName`.

```ts
interface BrandIconProps {
  iconUrl?: string;        // bare stored path, e.g. "/logos/storefronts/steam/steam-icon-light.svg"
  displayName: string;     // for alt text + first-letter fallback
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  className?: string;
}
```

Behavior:
- **Theme-aware variant:** read `resolvedTheme` from `useTheme()`. When it is `'dark'`, swap `-icon-light.svg` → `-icon-dark.svg` in the resolved URL. (Only swaps when the `-icon-light.svg` token is present; otherwise the URL is used as-is.)
- **onError fallback:** if the (possibly dark-swapped) image fails to load, fall back to the **originally-stored** filename (the light/base asset) — covers entries that have no dark variant yet. A second failure (no image at all) falls through to the first-letter badge via the same `onError` path.
- **Missing `iconUrl` → first-letter badge** (existing `PlatformIcon` fallback: `displayName.charAt(0)` in a muted span).
- `showLabel` / `showTooltip` keep the existing `PlatformIcon` semantics (label = icon + name inline; tooltip = name on hover when not labelled).
- URL construction (`config.staticUrl + path`) stays the caller's concern? No — `BrandIcon` takes the **bare stored path** and prefixes `config.staticUrl` itself, so every call site is consistent. (Today only `PlatformIcon` does this; centralizing it removes the chance of a future caller forgetting.)

> Theme-variant note: today the stored asset is the bare filename and the URL is `/logos/storefronts/<name>/<file>` (or `platforms/`). The dark variant is the same path with the `-icon-light` → `-icon-dark` token swapped. Until #1173 lands real dark assets, the `onError` fallback to the stored (light) filename keeps rendering correct under a dark theme.

### 2. `PlatformIcon` / `PlatformIconList` — rewrap, keep public API

`PlatformIcon({platform, size, showTooltip, showLabel, className})` keeps its signature and now renders `<BrandIcon iconUrl={platform.icon_url} displayName={platform.display_name} .../>`. `PlatformIconList` is unchanged in API; it keeps filtering on `platform_details`.

### 3. `StorefrontIcon` / `StorefrontIconList` — new thin wrappers

New wrappers (co-located with `PlatformIcon`, or a sibling `storefront-icon.tsx` — plan decides) mirroring the platform API:
- `StorefrontIcon({storefront: Storefront, size, showTooltip, showLabel, className})` → `<BrandIcon iconUrl={storefront.icon_url} displayName={storefront.display_name} .../>`.
- `StorefrontIconList({storefronts: Array<{storefront_details?: Storefront}>, ...})` mirroring `PlatformIconList` (filters on `storefront_details`).

### 4. Game-detail "Platforms & Ownership" rows (layout option A)

Left side becomes: `‹platform-icon› Platform name   ‹storefront-icon› Storefront name`.
- Platform name gets a leading `StorefrontIcon`/`PlatformIcon` (`size="sm"`).
- The storefront keeps its **display name and store-URL link**, but the parenthesised `(name)` styling is replaced by `‹storefront-icon› name`. This means `StorefrontLabel` gains an `icon`/`storefront` input so it can render the leading icon while preserving the optional `<a href={storeUrl}>`. Right side (achievements / ownership / acquired date) is untouched.

### 5. Selectors

- **Storefront selector** (`StorefrontSelector`, platform-selector.tsx): replace the plain `<Select>` with a searchable **Popover+Command combobox matching the platform picker** — `‹icon› name` in the trigger and in each list item, plus the "No storefront" option. Used in both the row editor and the compact wizard (both already pass `storefronts: Storefront[]`, which carry `icon_url`).
- **Platform picker:** swap the hardcoded `Monitor` for each platform's real icon via `PlatformIcon`/`BrandIcon` in the combobox trigger (L182), command items (L217), and the compact checkbox rows (L432). Keep `Monitor` only as the **empty-state placeholder** (L399) and as the trigger glyph when no platform is selected yet.

### 6. Filter facets

Add an **optional leading icon** to the shared `MultiSelectFilter`: extend `MultiSelectOption` with `icon?: React.ReactNode`, rendered before the label when present (the existing layout already has a flex row, so the icon slots in before `<span>{label}</span>`). In `game-filters.tsx`, build `platformOptions`/`storefrontOptions` with `icon: <PlatformIcon .../>` / `icon: <StorefrontIcon .../>`. The synthetic `"Unknown"` option carries no icon.

## Testing

- **`BrandIcon`** (`brand-icon.test.tsx`): renders `<img>` when `iconUrl` present; first-letter badge when absent; selects the `-icon-dark.svg` variant under a dark `resolvedTheme` (mock `useTheme`); `onError` falls back to the stored filename, then to the badge.
- **`StorefrontIcon`/`StorefrontIconList`**: parity with the platform behavior (present / fallback), mirroring the existing platform expectations.
- **Game-detail rows**: render shows platform + storefront icon inline with names, and the store-URL `<a>` is preserved.
- **Storefront combobox**: renders icons in items and selects a value (calls `onStorefrontChange`).
- **Filter facet**: an option with an `icon` renders it next to the label (extend `multi-select-filter.test.tsx`).

Tests mock `next-themes` `useTheme` the same way `theme-sync.test.tsx` does (`vi.mock('next-themes', …)`).

## Risks / notes

- `resolvedTheme` is `undefined` on the very first render before hydration; treat `undefined`/anything-not-`'dark'` as light (no swap) — safe default.
- Centralizing the `config.staticUrl` prefix inside `BrandIcon` changes `PlatformIcon` to pass the bare path; verify no other caller double-prefixes.
- Keep Echo/asset-path convention: icons live at `/logos/storefronts/<name>/<file>` and `/logos/platforms/<name>/<file>` (served from the SPA dist, not `/static/logos/`).
