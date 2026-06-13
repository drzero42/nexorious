# Game grid auto-fill columns — design (issue #942)

## Problem

The cover-art grid on `/games` (and `/wishlist`, which renders the same
component) caps at 6 columns via the Tailwind ladder
`grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6`
in `ui/frontend/src/components/games/game-grid.tsx`. The ladder ends at `xl:`
(≥1280px) and each track is `minmax(0, 1fr)`, so beyond 1280px the cards
stretch with the viewport: ~150px-wide cards at 1280px grow to ~510px on a
3440px ultrawide. Card size is a direct function of monitor width, which it
should not be.

All claims in the issue were verified against the code: the column cap
(`game-grid.tsx` lines 32 and 50), the stretching fractional tracks, the
unconstrained `<main>` in `ui/frontend/src/routes/_authenticated.tsx`, and the
`aspect-[3/4]` cover in `game-card.tsx`.

## Decision

Replace the breakpoint ladder with a CSS auto-fill grid so the **column count
adapts to the viewport** while the **card width stays in a narrow band**:

```
grid-cols-[repeat(auto-fill,minmax(min(180px,45%),1fr))]
```

- `auto-fill` + `minmax(180px, 1fr)`: the browser adds a column as soon as
  another 180px track fits, so card width stays in roughly the 180–220px band
  regardless of viewport or sidebar state. No breakpoints to maintain.
- `min(180px, 45%)`: on narrow containers (phones, ~330px content width) 45%
  evaluates below 180px, guaranteeing two columns — matching today's mobile
  `grid-cols-2` look. On desktop widths `min()` resolves to 180px.
- Full content width remains in use (no `max-w-*` cap) — on an ultrawide the
  user gets more columns of normal-sized covers, not larger covers.

Resulting column counts: ~2 on phones (unchanged), ~5 at 1280px, ~8 at
1920px, ~17 at 3440px.

### Alternatives rejected

- **Extend the breakpoint ladder** (`2xl:grid-cols-8` + custom ultrawide
  breakpoints): more code, card size still steps between breakpoints, and it
  breaks again on the next wider monitor.
- **Cap content width** (`max-w-*` on the layout or grid): wastes the screen
  the user paid for; ruled out in favour of using the full width.

## Changes

Single file: `ui/frontend/src/components/games/game-grid.tsx`.

1. Extract a module-level constant holding the shared grid class string
   (auto-fill class + `gap-4`).
2. Use it in **both** grid renders — the loading-skeleton grid (line 32) and
   the games grid (line 50) — so loading and loaded layouts match and cannot
   drift.

No changes to `game-card.tsx` (the card already fills its grid cell with an
`aspect-[3/4]` cover) or `_authenticated.tsx` (the unconstrained `<main>` is
now intentional).

## Testing & verification

Pure CSS layout change — no logic, branches, or edge cases — so per the
project test policy no new test is added. The DOM structure is unchanged, so
any existing render tests keep passing. Verification is visual: build the
frontend and check `/games` (and `/wishlist`) at a phone width, ~1280px,
~1920px, and an ultrawide width, confirming card width stays roughly
180–220px while the column count adapts.
