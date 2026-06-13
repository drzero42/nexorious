# Game Grid Auto-Fill Columns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the games/wishlist cover grid adapt its column count to the viewport instead of capping at 6 stretching columns (issue #942).

**Architecture:** Replace the Tailwind breakpoint ladder in `GameGrid` with a single CSS auto-fill arbitrary-value class so card width stays in the ~180–220px band at any viewport width. The class lives in one module-level constant used by both the skeleton grid and the real grid. No other component changes.

**Tech Stack:** React 19 + Tailwind CSS v4 (arbitrary value `grid-cols-[...]`), Vitest.

**Spec:** `docs/superpowers/specs/2026-06-12-issue-942-game-grid-auto-fill-design.md`

---

### Task 1: Replace the breakpoint ladder with an auto-fill grid class

Per the project test policy (CLAUDE.md), no new test: this is a pure CSS
class change with no logic or edge cases — `game-grid.test.tsx` has no
assertions on grid classes, so existing tests are unaffected. TDD does not
apply; verification is typecheck + existing tests + visual check.

**Files:**
- Modify: `ui/frontend/src/components/games/game-grid.tsx` (lines 32 and 50)

- [ ] **Step 1: Add the shared grid-class constant**

In `ui/frontend/src/components/games/game-grid.tsx`, below the imports (after line 3), add:

```tsx
const gridClasses =
  'grid grid-cols-[repeat(auto-fill,minmax(min(180px,45%),1fr))] gap-4';
```

Why this value: `auto-fill` adds a column whenever another 180px track fits,
keeping cards ~180–220px wide at any width; `min(180px,45%)` makes the
minimum shrink on narrow containers so phones keep two columns (45% of a
~330px container is ~148px). Tailwind arbitrary values must not contain
spaces — the expression above has none.

- [ ] **Step 2: Use the constant in both grid renders**

Replace line 32 (skeleton grid):

```tsx
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
```

with:

```tsx
      <div className={gridClasses}>
```

Replace line 50 (games grid):

```tsx
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
```

with:

```tsx
    <div className={gridClasses}>
```

After this step the string `grid-cols-2 sm:grid-cols-3` must not appear
anywhere in the file.

- [ ] **Step 3: Run typecheck and the component's tests**

```bash
cd ui/frontend && npm run check && npm run test game-grid.test.tsx
```

Expected: typecheck passes with no errors; all tests in
`game-grid.test.tsx` pass (none assert on grid classes).

- [ ] **Step 4: Verify the compiled CSS contains the auto-fill rule**

```bash
cd ui/frontend && npm run build && grep -c "auto-fill" dist/assets/*.css
```

Expected: build succeeds and grep prints a count ≥ 1, proving Tailwind
generated the arbitrary-value utility (a typo in the bracket expression
would silently produce no CSS rule).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/games/game-grid.tsx
git commit -m "fix: let game grid column count adapt to viewport width"
```

### Task 2: Visual verification at multiple widths

**Files:** none (verification only)

- [ ] **Step 1: Run the app and check /games and /wishlist**

Build and serve (from repo root):

```bash
make && ./nexorious serve
```

Open `/games` in a browser and use devtools responsive mode to check four
widths: ~375px (expect 2 columns), ~1280px (expect ~5 columns), ~1920px
(expect ~8 columns), ~3440px (expect ~17 columns). Cards should stay
roughly the same size (~180–220px wide) across the desktop widths. Spot-check
`/wishlist` once at one desktop width (same component). Also confirm the
loading skeleton briefly shows the same column layout as the loaded grid.

If a local server/database is not available in this environment, report
that visual verification needs the user to check, and list the expected
column counts above for them.
