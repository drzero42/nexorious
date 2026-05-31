# Issue #646: Platform Label in Edit Form Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show "Platform (Storefront)" labels (e.g. "Windows (GOG)") in the edit-game form's per-platform cards and in the detail page's playtime breakdown, so entries for the same storefront on different platforms are distinguishable.

**Architecture:** Add a pure `formatPlatformLabel` helper to `game-utils.ts`, cover it with unit tests, then replace the two broken fallback chains (edit form + playtime breakdown) with calls to the helper.

**Tech Stack:** TypeScript, Vitest, React

---

### Task 1: Add `formatPlatformLabel` helper (TDD)

**Files:**
- Modify: `ui/frontend/src/lib/game-utils.ts`
- Modify: `ui/frontend/src/lib/game-utils.test.ts`

- [ ] **Step 1: Write the failing tests**

Open `ui/frontend/src/lib/game-utils.test.ts` and append:

```ts
describe('formatPlatformLabel', () => {
  it('returns "Platform (Storefront)" when both details are present', () => {
    expect(
      formatPlatformLabel({
        platform: 'windows',
        storefront: 'gog',
        platform_details: { display_name: 'Windows' },
        storefront_details: { display_name: 'GOG' },
      }),
    ).toBe('Windows (GOG)');
  });

  it('falls back to raw names when details are absent', () => {
    expect(
      formatPlatformLabel({
        platform: 'windows',
        storefront: 'gog',
        platform_details: null,
        storefront_details: null,
      }),
    ).toBe('windows (gog)');
  });

  it('shows only storefront when platform is missing', () => {
    expect(
      formatPlatformLabel({
        platform: null,
        storefront: null,
        platform_details: null,
        storefront_details: { display_name: 'Steam' },
      }),
    ).toBe('Steam');
  });

  it('shows only platform when storefront is missing', () => {
    expect(
      formatPlatformLabel({
        platform: 'linux',
        storefront: null,
        platform_details: { display_name: 'Linux PC' },
        storefront_details: null,
      }),
    ).toBe('Linux PC');
  });

  it('returns "Unknown" when everything is absent', () => {
    expect(
      formatPlatformLabel({
        platform: null,
        storefront: null,
        platform_details: null,
        storefront_details: null,
      }),
    ).toBe('Unknown');
  });
});
```

- [ ] **Step 2: Add the import to the test file**

The test file already imports from `./game-utils`; update the import line to include `formatPlatformLabel`:

```ts
import { describe, it, expect } from 'vitest';
import { formatTtb, formatIgdbRating, formatHoursPlayed, formatPlatformLabel } from './game-utils';
```

- [ ] **Step 3: Run to confirm it fails**

```bash
cd ui/frontend && npm run test game-utils.test.ts
```

Expected: 5 new test cases fail with "formatPlatformLabel is not a function" (or similar import error).

- [ ] **Step 4: Implement the helper**

In `ui/frontend/src/lib/game-utils.ts`, append after `formatIgdbRating`:

```ts
export function formatPlatformLabel(p: {
  platform?: string | null;
  storefront?: string | null;
  platform_details?: { display_name: string } | null;
  storefront_details?: { display_name: string } | null;
}): string {
  const platform = p.platform_details?.display_name || p.platform;
  const storefront = p.storefront_details?.display_name || p.storefront;
  if (platform && storefront) return `${platform} (${storefront})`;
  return platform || storefront || 'Unknown';
}
```

- [ ] **Step 5: Run to confirm all tests pass**

```bash
cd ui/frontend && npm run test game-utils.test.ts
```

Expected: all tests in the file pass (3 existing `describe` blocks + the new one).

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/lib/game-utils.ts ui/frontend/src/lib/game-utils.test.ts
git commit -m "feat: add formatPlatformLabel helper to game-utils"
```

---

### Task 2: Apply helper in the edit-game form

**Files:**
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx`

- [ ] **Step 1: Update the import**

Find the existing import in `game-edit-form.tsx`:

```ts
import { formatHoursPlayed } from '@/lib/game-utils';
```

Replace with:

```ts
import { formatHoursPlayed, formatPlatformLabel } from '@/lib/game-utils';
```

- [ ] **Step 2: Replace the broken label computation**

Find these lines (around line 418):

```tsx
const platformName =
  p.storefront_details?.display_name ||
  p.storefront ||
  p.platform_details?.display_name ||
  p.platform ||
  'Unknown';
```

Replace with:

```tsx
const platformName = formatPlatformLabel(p);
```

- [ ] **Step 3: Run the frontend tests**

```bash
cd ui/frontend && npm run test game-edit-form.test.tsx
```

Expected: all existing tests pass.

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/components/games/game-edit-form.tsx
git commit -m "fix: show platform name in edit form storefront cards"
```

---

### Task 3: Apply helper in the detail page playtime breakdown

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx`

- [ ] **Step 1: Update the import**

Find the existing import in `$id.index.tsx`:

```ts
import { formatIgdbRating, formatHoursPlayed, formatTtb } from '@/lib/game-utils';
```

Replace with:

```ts
import { formatIgdbRating, formatHoursPlayed, formatTtb, formatPlatformLabel } from '@/lib/game-utils';
```

- [ ] **Step 2: Replace the broken label in the playtime breakdown**

Find these lines (around line 404):

```tsx
<span>
  {p.storefront_details?.display_name ||
    p.storefront ||
    p.platform_details?.display_name ||
    p.platform ||
    'Unknown'}
</span>
```

Replace with:

```tsx
<span>{formatPlatformLabel(p)}</span>
```

- [ ] **Step 3: Run the frontend tests**

```bash
cd ui/frontend && npm run test '$id.index.tsx'
```

Expected: all existing tests pass.

- [ ] **Step 4: Type-check the whole frontend**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add "ui/frontend/src/routes/_authenticated/games/\$id.index.tsx"
git commit -m "fix: show platform name in detail page playtime breakdown"
```
