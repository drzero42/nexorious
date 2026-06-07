# Edit-page Platform Row Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix #846 (removing a platform deletes the wrong storefront row) and #847 (storefront change not saved) on the Edit game page by identifying platform rows by stable identity instead of by platform name.

**Architecture:** Frontend-only. Give every `PlatformSelection` a required client-side `key` and an optional server `id`. The per-row list in `PlatformSelector` removes/edits by `key`; a new pure module `platform-reconcile.ts` diffs the original rows against the edited selections by `id` and returns add/remove/update operations that `game-edit-form` dispatches. No backend or migration changes — the API and DB already support multiple storefronts per platform.

**Tech Stack:** React 19 + TypeScript, Vitest + @testing-library/react, TanStack Query hooks. All frontend commands run from `ui/frontend/`.

**Spec:** `docs/superpowers/specs/2026-06-07-edit-page-platform-row-identity-design.md`

---

## File Structure

- `ui/frontend/src/components/ui/platform-selector.tsx` — add identity fields to `PlatformSelection`; stamp keys at creation; key per-row remove/storefront/render. (Modify)
- `ui/frontend/src/components/ui/platform-selector.test.tsx` — update existing literals/assertions for the required `key`; add duplicate-removal test. (Modify)
- `ui/frontend/src/components/games/platform-reconcile.ts` — **new** pure diff function `planPlatformChanges`. (Create)
- `ui/frontend/src/components/games/platform-reconcile.test.ts` — **new** unit tests for the diff. (Create)
- `ui/frontend/src/components/games/game-edit-form.tsx` — seed selections with identity; replace name-based save reconciliation with `planPlatformChanges`. (Modify)
- `ui/frontend/src/components/games/game-edit-form.test.tsx` — restructure hook mocks to stable spies; add removal-save integration test. (Modify)

---

## Task 1: Identity fields on `PlatformSelection` (plumbing, no resolution change)

Introduce `key` (required) and `id` (optional), stamp a key wherever a selection is created (both the full selector and the compact add-flow selector), and switch React render keys to it. Removal/storefront still resolve by name after this task — that is fixed in Task 2. This task keeps both `npm run check` and `npm run test` green.

**Files:**
- Modify: `ui/frontend/src/components/ui/platform-selector.tsx`
- Modify: `ui/frontend/src/components/ui/platform-selector.test.tsx`

- [ ] **Step 1: Extend the type and add a key helper**

In `platform-selector.tsx`, replace the existing `PlatformSelection` interface (currently `{ platform: string; storefront?: string }`) with:

```ts
export interface PlatformSelection {
  /** Stable client-side identity for this selected row. Always present. */
  key: string;
  /** Server UUID. Present only once the row is persisted in the database. */
  id?: string;
  platform: string;
  storefront?: string;
}

/** Generates a stable client-side key for a newly-created selection row. */
function newSelectionKey(): string {
  return typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `sel-${Math.random().toString(36).slice(2)}`;
}
```

- [ ] **Step 2: Stamp a key when the full selector adds a platform**

In `platform-selector.tsx`, in `PlatformSelector`'s `handlePlatformToggle`, the add branch currently is:

```ts
    } else if (!isMaxReached) {
      // Add platform with default storefront if available
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
```

Replace the pushed object literal so it includes a key:

```ts
    } else if (!isMaxReached) {
      // Add platform with default storefront if available
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          key: newSelectionKey(),
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
```

- [ ] **Step 3: Stamp a key when the compact selector adds a platform**

In `platform-selector.tsx`, in `PlatformSelectorCompact`'s `handleToggle`, the add branch currently is:

```ts
    } else {
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
```

Replace the pushed literal with:

```ts
    } else {
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          key: newSelectionKey(),
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
```

- [ ] **Step 4: Switch the full selector's React render keys to `selection.key`**

In `platform-selector.tsx` there are three `key={selection.platform}` render keys in `PlatformSelector` (two trigger-badge maps and the per-row selection list). Change all three to `key={selection.key}`. They are inside:
1. The trigger badge map when `selectedPlatformObjects.length <= 2` (`<PlatformBadge key={selection.platform} ... />`).
2. The trigger badge map inside the `slice(0, 1)` branch (`<PlatformBadge key={selection.platform} ... />`).
3. The per-row selected list (`<div key={selection.platform} className="flex items-center gap-3 ...">`).

Each becomes `key={selection.key}`.

- [ ] **Step 5: Add a literal helper and update existing test literals/assertions**

In `platform-selector.test.tsx`, add this helper just below the imports (after the `import type { Platform, Storefront } ...` line):

```ts
// Helper: build a PlatformSelection with the now-required `key`.
const sel = (platform: string, storefront?: string, key = `k-${platform}`): PlatformSelection => ({
  key,
  platform,
  storefront,
});
```

Then replace each `PlatformSelection[]` literal with `sel(...)` calls. Apply every replacement below exactly:

| Old | New |
|---|---|
| `[{ platform: 'pc', storefront: 'steam' }]` | `[sel('pc', 'steam')]` |
| `[{ platform: 'pc' }, { platform: 'ps5' }, { platform: 'xbox' }]` | `[sel('pc'), sel('ps5'), sel('xbox')]` |
| `[{ platform: 'pc', storefront: 'steam' }, { platform: 'ps5' }]` | `[sel('pc', 'steam'), sel('ps5')]` |
| `[{ platform: 'pc' }, { platform: 'ps5' }]` | `[sel('pc'), sel('ps5')]` |
| `[{ platform: 'pc' }]` | `[sel('pc')]` |
| `[{ platform: 'xbox' }]` | `[sel('xbox')]` |

(`[{ platform: 'pc', storefront: 'steam' }]` appears several times — replace every occurrence with `[sel('pc', 'steam')]`.)

Then update the two assertions that check an **added** row (they now receive a generated `key`), changing the exact-object match to `expect.objectContaining`:

- In `'calls onChange when platform is selected'`:

```ts
    expect(handleChange).toHaveBeenCalledWith([
      expect.objectContaining({ platform: 'pc', storefront: 'steam' }),
    ]);
```

- In `'calls onChange when checkbox is toggled'` (the `PlatformSelectorCompact` describe):

```ts
    expect(handleChange).toHaveBeenCalledWith([
      expect.objectContaining({ platform: 'pc', storefront: 'steam' }),
    ]);
```

Leave the `toHaveBeenCalledWith([])` assertions (deselect / clear / uncheck) unchanged.

- [ ] **Step 6: Run type-check and the selector tests**

Run: `npm run check`
Expected: PASS (no type errors).

Run: `npm run test platform-selector.test.tsx`
Expected: PASS (all existing selector tests green).

- [ ] **Step 7: Commit**

```bash
git add ui/frontend/src/components/ui/platform-selector.tsx ui/frontend/src/components/ui/platform-selector.test.tsx
git commit -m "refactor: add stable identity to PlatformSelection rows"
```

---

## Task 2: Per-row remove and storefront resolve by `key` (#846 UI layer)

Make the per-row X button and storefront dropdown act on the single targeted row by `key` instead of by platform name. This fixes the UI half of #846 (and the duplicate-storefront-cross-edit facet of #847). The storefront save path is verified by Task 3's pure tests; this task's failing test covers removal.

**Files:**
- Modify: `ui/frontend/src/components/ui/platform-selector.tsx`
- Modify: `ui/frontend/src/components/ui/platform-selector.test.tsx`

- [ ] **Step 1: Write the failing removal test**

In `platform-selector.test.tsx`, inside the `describe('PlatformSelector', ...)` block, add:

```ts
  it('removes only the targeted row when two rows share a platform (#846)', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const selected: PlatformSelection[] = [
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
      { key: 'k-2', platform: 'pc', storefront: undefined },
    ];

    render(
      <PlatformSelector
        selectedPlatforms={selected}
        availablePlatforms={mockPlatforms}
        onChange={handleChange}
      />,
    );

    const removeButtons = screen.getAllByRole('button', { name: /remove pc/i });
    expect(removeButtons).toHaveLength(2);
    await user.click(removeButtons[1]); // remove the second PC row (no storefront)

    expect(handleChange).toHaveBeenCalledWith([
      { key: 'k-1', platform: 'pc', storefront: 'steam' },
    ]);
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test platform-selector.test.tsx`
Expected: FAIL — the current name-based `handleRemovePlatform` removes **all** `pc` rows, so `onChange` is called with `[]` instead of the single remaining row.

- [ ] **Step 3: Make remove and storefront resolve by `key`**

In `platform-selector.tsx`, change the two handlers in `PlatformSelector` from name-based to key-based.

`handleStorefrontChange` becomes:

```ts
  const handleStorefrontChange = (key: string, storefront: string | undefined) => {
    if (disabled) return;

    onChange(selectedPlatforms.map((s) => (s.key === key ? { ...s, storefront } : s)));
  };
```

`handleRemovePlatform` becomes:

```ts
  const handleRemovePlatform = (key: string) => {
    if (disabled) return;
    onChange(selectedPlatforms.filter((s) => s.key !== key));
  };
```

Then update the two call sites in the per-row selected list JSX to pass `selection.key`:

- The storefront selector callback:

```ts
                    <StorefrontSelector
                      storefronts={storefronts}
                      selectedStorefront={selection.storefront}
                      onStorefrontChange={(storefront) =>
                        handleStorefrontChange(selection.key, storefront)
                      }
                      disabled={disabled}
                    />
```

- The remove button:

```ts
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRemovePlatform(selection.key)}
                  disabled={disabled}
                  className="flex-shrink-0 h-8 w-8 p-0"
                >
```

(Leave `PlatformSelectorCompact` unchanged — it renders one row per available platform and cannot produce duplicates, so its name-based matching stays correct.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm run test platform-selector.test.tsx`
Expected: PASS (including the new #846 test and all prior tests).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/ui/platform-selector.tsx ui/frontend/src/components/ui/platform-selector.test.tsx
git commit -m "fix: remove and edit platform rows by identity, not name (#846)"
```

---

## Task 3: Pure reconciliation module (#846 + #847 logic)

Create a pure function that diffs original DB rows against the edited selections by `id`, returning the add/remove/update operations to dispatch on save. This is where both bugs are conclusively fixed and tested with plain data (no UI).

**Files:**
- Create: `ui/frontend/src/components/games/platform-reconcile.ts`
- Create: `ui/frontend/src/components/games/platform-reconcile.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `ui/frontend/src/components/games/platform-reconcile.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { planPlatformChanges, type PlatformDetailState } from './platform-reconcile';
import { OwnershipStatus } from '@/types';
import type { UserGamePlatform } from '@/types';

const orig = (
  id: string,
  platform: string,
  storefront: string | undefined,
  extra: Partial<UserGamePlatform> = {},
): UserGamePlatform => ({
  id,
  platform,
  storefront,
  is_available: true,
  hours_played: 0,
  ownership_status: OwnershipStatus.OWNED,
  created_at: '2024-01-01T00:00:00Z',
  ...extra,
});

const detail = (
  hoursPlayed = 0,
  ownershipStatus = OwnershipStatus.OWNED,
  acquiredDate = '',
): PlatformDetailState => ({ hoursPlayed, ownershipStatus, acquiredDate });

describe('planPlatformChanges', () => {
  it('emits an update carrying the new storefront when it changes (#847)', () => {
    const original = [orig('ugp-1', 'pc', 'steam', { hours_played: 10, acquired_date: '2024-01-15' })];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'epic' }];
    const details = { 'ugp-1': detail(10, OwnershipStatus.OWNED, '2024-01-15') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.adds).toEqual([]);
    expect(cs.removes).toEqual([]);
    expect(cs.updates).toEqual([
      {
        id: 'ugp-1',
        platform: 'pc',
        storefront: 'epic',
        hoursPlayed: 10,
        ownershipStatus: OwnershipStatus.OWNED,
        acquiredDate: '2024-01-15',
      },
    ]);
  });

  it('removes the correct row id and leaves the sibling when a duplicate is deleted (#846)', () => {
    const original = [orig('ugp-1', 'pc', 'steam'), orig('ugp-2', 'pc', undefined)];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(0), 'ugp-2': detail(0) };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.removes).toEqual([{ id: 'ugp-2' }]);
    expect(cs.adds).toEqual([]);
    expect(cs.updates).toEqual([]);
  });

  it('treats a selection without an id as an add', () => {
    const selections = [{ key: 'new-1', platform: 'ps5', storefront: 'psn' }];

    const cs = planPlatformChanges([], selections, {});

    expect(cs.adds).toEqual([{ platform: 'ps5', storefront: 'psn' }]);
    expect(cs.removes).toEqual([]);
    expect(cs.updates).toEqual([]);
  });

  it('emits nothing when there are no changes', () => {
    const original = [orig('ugp-1', 'pc', 'steam', { hours_played: 10, acquired_date: '2024-01-15' })];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(10, OwnershipStatus.OWNED, '2024-01-15') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs).toEqual({ adds: [], removes: [], updates: [] });
  });

  it('emits an update when ownership or hours change', () => {
    const original = [orig('ugp-1', 'pc', 'steam', { hours_played: 5 })];
    const selections = [{ key: 'ugp-1', id: 'ugp-1', platform: 'pc', storefront: 'steam' }];
    const details = { 'ugp-1': detail(12, OwnershipStatus.BORROWED, '') };

    const cs = planPlatformChanges(original, selections, details);

    expect(cs.updates).toEqual([
      {
        id: 'ugp-1',
        platform: 'pc',
        storefront: 'steam',
        hoursPlayed: 12,
        ownershipStatus: OwnershipStatus.BORROWED,
      },
    ]);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run test platform-reconcile.test.ts`
Expected: FAIL — module `./platform-reconcile` does not exist yet.

- [ ] **Step 3: Implement the pure module**

Create `ui/frontend/src/components/games/platform-reconcile.ts`:

```ts
import type { UserGamePlatform } from '@/types';
import { OwnershipStatus } from '@/types';
import type { PlatformSelection } from '@/components/ui/platform-selector';

/** Per-row ownership/playtime state as edited in the form, keyed by row id. */
export interface PlatformDetailState {
  hoursPlayed: number;
  ownershipStatus: OwnershipStatus;
  acquiredDate: string; // '' when none
}

interface PlatformChangeSet {
  adds: { platform: string; storefront?: string }[];
  removes: { id: string }[];
  updates: {
    id: string;
    platform: string;
    storefront?: string;
    hoursPlayed: number;
    ownershipStatus: OwnershipStatus;
    acquiredDate?: string;
  }[];
}

/**
 * Diffs the persisted platform rows against the edited selections by row id.
 * - selections without an `id` are adds
 * - original rows whose id is no longer selected are removes
 * - selected rows whose storefront (from the selection) or ownership/date/hours
 *   (from `details`) changed are updates
 */
export function planPlatformChanges(
  original: UserGamePlatform[],
  selections: PlatformSelection[],
  details: Record<string, PlatformDetailState>,
): PlatformChangeSet {
  const currentIds = new Set(selections.map((s) => s.id).filter((id): id is string => !!id));

  const adds = selections
    .filter((s) => !s.id)
    .map((s) => ({ platform: s.platform, storefront: s.storefront }));

  const removes = original.filter((o) => !currentIds.has(o.id)).map((o) => ({ id: o.id }));

  const updates: PlatformChangeSet['updates'] = [];
  for (const s of selections) {
    if (!s.id) continue;
    const o = original.find((p) => p.id === s.id);
    if (!o) continue;

    const d = details[s.id] ?? {
      hoursPlayed: o.hours_played,
      ownershipStatus: o.ownership_status,
      acquiredDate: o.acquired_date ?? '',
    };

    const changed =
      (o.storefront ?? '') !== (s.storefront ?? '') ||
      o.hours_played !== d.hoursPlayed ||
      o.ownership_status !== d.ownershipStatus ||
      (o.acquired_date ?? '') !== d.acquiredDate;

    if (changed) {
      updates.push({
        id: s.id,
        platform: o.platform ?? '',
        storefront: s.storefront,
        hoursPlayed: d.hoursPlayed,
        ownershipStatus: d.ownershipStatus,
        acquiredDate: d.acquiredDate || undefined,
      });
    }
  }

  return { adds, removes, updates };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run test platform-reconcile.test.ts`
Expected: PASS (all five cases).

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/components/games/platform-reconcile.ts ui/frontend/src/components/games/platform-reconcile.test.ts
git commit -m "feat: pure reconciliation of platform rows by identity (#846, #847)"
```

---

## Task 4: Wire reconciliation into the edit form (#846 + #847 end-to-end)

Seed the form's selections with identity, build the per-row detail map, and replace the name-based save logic with `planPlatformChanges`. Restructure the test's hook mocks into stable spies and add an integration test that proves removal deletes the correct row id.

**Files:**
- Modify: `ui/frontend/src/components/games/game-edit-form.tsx`
- Modify: `ui/frontend/src/components/games/game-edit-form.test.tsx`

- [ ] **Step 1: Restructure the hook mocks and write the failing integration test**

In `game-edit-form.test.tsx`, replace the entire existing `vi.mock('@/hooks', ...)` block with stable, resettable spies plus controllable platform data:

```ts
const hooks = vi.hoisted(() => ({
  updateGame: vi.fn(),
  addPlatform: vi.fn(),
  removePlatform: vi.fn(),
  updatePlatform: vi.fn(),
  assignTags: vi.fn(),
  removeTags: vi.fn(),
  createOrGetTag: vi.fn(),
}));

const state = vi.hoisted(() => ({ platforms: [] as unknown[] }));

vi.mock('@/hooks', () => ({
  useUpdateUserGame: () => ({ mutateAsync: hooks.updateGame }),
  useAddPlatformToUserGame: () => ({ mutateAsync: hooks.addPlatform }),
  useRemovePlatformFromUserGame: () => ({ mutateAsync: hooks.removePlatform }),
  useUpdatePlatformAssociation: () => ({ mutateAsync: hooks.updatePlatform }),
  useAssignTagsToGame: () => ({ mutateAsync: hooks.assignTags }),
  useRemoveTagsFromGame: () => ({ mutateAsync: hooks.removeTags }),
  useAllPlatforms: () => ({ data: state.platforms, isLoading: false }),
  useAllTags: () => ({ data: [], isLoading: false }),
  useCreateOrGetTag: () => ({ mutateAsync: hooks.createOrGetTag }),
  useSyncConfig: () => ({ data: null }),
}));
```

Add `waitFor` to the testing imports and define a PC platform fixture. At the top of the file, change the import line to:

```ts
import { render, screen, waitFor } from '@/test/test-utils';
```

Add this fixture near `mockGame` (after the `mockGame` definition):

```ts
const mockPlatformsData = [
  {
    name: 'pc',
    display_name: 'PC',
    is_active: true,
    source: 'official',
    default_storefront: 'steam',
    storefronts: [
      {
        name: 'steam',
        display_name: 'Steam',
        is_active: true,
        source: 'official',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];
```

Update `beforeEach` to reset spies and platform data:

```ts
  beforeEach(() => {
    vi.clearAllMocks();
    mockNavigate.mockReset();
    Object.values(hooks).forEach((fn) => fn.mockResolvedValue({}));
    state.platforms = [];
  });
```

Then add the integration test inside `describe('GameEditForm', ...)`:

```ts
  it('deletes the correct row id when a platform is removed, then saved (#846)', async () => {
    const user = userEvent.setup();
    state.platforms = mockPlatformsData; // so the selected PC row + its remove button render

    render(<GameEditForm game={mockGame} />);

    await user.click(screen.getByRole('button', { name: /remove pc/i }));
    await user.click(screen.getAllByRole('button', { name: /save changes/i })[0]);

    await waitFor(() =>
      expect(hooks.removePlatform).toHaveBeenCalledWith({
        userGameId: mockGame.id,
        platformAssociationId: 'ugp-1',
      }),
    );
    expect(hooks.addPlatform).not.toHaveBeenCalled();
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test game-edit-form.test.tsx`
Expected: FAIL — with the current name-based save, `getPlatformAssociationId` still resolves by name; more importantly the mocks were just restructured, so this confirms the new wiring is not present yet. (If it errors on `state`/`hooks` references, that confirms the implementation in Step 3 is still required.)

- [ ] **Step 3: Seed identity and replace the save reconciliation**

In `game-edit-form.tsx`:

(a) Update the import from React to drop the now-unused `useCallback` (it is only used by the helper removed below). Current line:

```ts
import { useState, useCallback, useMemo } from 'react';
```

becomes:

```ts
import { useState, useMemo } from 'react';
```

(b) Add the reconcile import alongside the other local imports (near the `formatHoursPlayed` import):

```ts
import { planPlatformChanges, type PlatformDetailState } from './platform-reconcile';
```

(c) Seed `selectedPlatforms` with identity. Replace:

```ts
  const [selectedPlatforms, setSelectedPlatforms] = useState<PlatformSelection[]>(
    game.platforms
      .filter((p) => p.platform)
      .map((p) => ({
        platform: p.platform!,
        storefront: p.storefront,
      })),
  );
```

with:

```ts
  const [selectedPlatforms, setSelectedPlatforms] = useState<PlatformSelection[]>(
    game.platforms
      .filter((p) => p.platform)
      .map((p) => ({
        key: p.id,
        id: p.id,
        platform: p.platform!,
        storefront: p.storefront,
      })),
  );
```

(d) Remove the now-unused `originalPlatformNames` memo and `getPlatformAssociationId` callback. Delete this block:

```ts
  const originalPlatformNames = useMemo(
    () => game.platforms.map((p) => p.platform).filter(Boolean) as string[],
    [game.platforms],
  );

  // Get platform association ID by platform name
  const getPlatformAssociationId = useCallback(
    (platformName: string): string | undefined => {
      const assoc = game.platforms.find((p) => p.platform === platformName);
      return assoc?.id;
    },
    [game.platforms],
  );
```

(e) Replace the platform reconciliation in `handleSave`. Delete the existing "2. Handle platform changes" and "3. Update platform playtimes and ownership" blocks (from `const currentPlatformNames = ...` through the end of the `for (const [platformId, data] of Object.entries(platformOwnership))` loop) and put in their place:

```ts
      // 2. Reconcile platform associations by row identity.
      const details: Record<string, PlatformDetailState> = {};
      for (const p of game.platforms) {
        const ownership = platformOwnership[p.id];
        details[p.id] = {
          hoursPlayed: platformPlaytimes[p.id] ?? p.hours_played,
          ownershipStatus: ownership?.ownershipStatus ?? p.ownership_status,
          acquiredDate: ownership?.acquiredDate ?? p.acquired_date ?? '',
        };
      }

      const { adds, removes, updates } = planPlatformChanges(
        game.platforms,
        selectedPlatforms,
        details,
      );

      for (const add of adds) {
        await addPlatform.mutateAsync({
          userGameId: game.id,
          data: { platform: add.platform, storefront: add.storefront },
        });
      }

      for (const remove of removes) {
        await removePlatform.mutateAsync({
          userGameId: game.id,
          platformAssociationId: remove.id,
        });
      }

      for (const update of updates) {
        await updatePlatformAssoc.mutateAsync({
          userGameId: game.id,
          platformAssociationId: update.id,
          data: {
            platform: update.platform,
            storefront: update.storefront,
            hoursPlayed: update.hoursPlayed,
            ownershipStatus: update.ownershipStatus,
            acquiredDate: update.acquiredDate,
          },
        });
      }
```

Keep the existing "1. Update basic game properties" (`updateGame.mutateAsync(...)`) block before this, and the "4. Handle tag changes" block after it (renumbering of the comment is optional).

- [ ] **Step 4: Run the edit-form tests to verify they pass**

Run: `npm run test game-edit-form.test.tsx`
Expected: PASS (the new #846 integration test plus all existing tests).

- [ ] **Step 5: Type-check**

Run: `npm run check`
Expected: PASS — no unused `useCallback`/`originalPlatformNames`, no type errors.

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/components/games/game-edit-form.tsx ui/frontend/src/components/games/game-edit-form.test.tsx
git commit -m "fix: save platform storefront changes and remove correct row (#846, #847)"
```

---

## Task 5: Full verification

Run the complete frontend gate the pre-push hook enforces, then a manual smoke check.

**Files:** none (verification only)

- [ ] **Step 1: Type-check, dead-code, tests, build**

From `ui/frontend/`:

Run: `npm run check`
Expected: PASS.

Run: `npm run knip`
Expected: PASS — no unused files/exports. (`platform-reconcile.ts` is imported by `game-edit-form.tsx`; `PlatformDetailState` is used there; `planPlatformChanges` is used there and in its test.)

Run: `npm run test`
Expected: PASS — whole suite green.

Run: `npm run build`
Expected: PASS — SPA builds, `routeTree.gen.ts` unchanged (no routes added).

- [ ] **Step 2: Manual smoke check (record results)**

Build and run the app (`make && ./nexorious serve`) and, on a game that has two associations of the same platform with different storefronts (create via sync/import, or temporarily insert two rows):

1. Change a storefront on one existing row and Save → reopen the game; the new storefront persists (#847).
2. Remove the specific row you click (e.g. the no-storefront one) and Save → only that row is gone; its sibling remains (#846).
3. Confirm a single-row game still adds/edits/removes normally, and the add-game flow still selects platforms and storefronts as before.

- [ ] **Step 3: Finalize**

Use the `superpowers:finishing-a-development-branch` skill to open the PR (title `fix: …` per Conventional Commits, referencing #846 and #847).

---

## Self-Review

**Spec coverage:**
- Identity model (`key` required, `id` optional, uniform across consumers) → Task 1 (type + key stamping in both `PlatformSelector` and `PlatformSelectorCompact`).
- `PlatformSelector` per-row remove/storefront/render by identity → Task 1 (render keys) + Task 2 (handlers).
- Add-game flow mechanical key-stamping, no UX change → Task 1 Step 3 (compact selector), verified by Task 5 manual check and existing compact tests.
- `game-edit-form` seeding with identity + id-based save diff (POST/DELETE/merged PUT) + removal of `getPlatformAssociationId` → Task 4.
- Backend/schema untouched → no backend tasks present. ✓
- Tests called for in the spec: duplicate-row remove (Task 2 + Task 4 integration), storefront edit issues update with new storefront (Task 3), add-flow regression (Task 5 + retained compact tests). ✓

**Placeholder scan:** No TBD/TODO; every code step shows full code; the test-literal updates are given as an explicit replacement table. ✓

**Type consistency:** `PlatformSelection { key: string; id?: string; platform; storefront? }` is used identically across Tasks 1–4. `planPlatformChanges(original, selections, details)` signature and the `PlatformDetailState` shape match between Task 3 (definition/tests) and Task 4 (caller). Hook mutation argument shapes (`{ userGameId, data }`, `{ userGameId, platformAssociationId }`, `{ userGameId, platformAssociationId, data }`) match the existing hook usage in `game-edit-form.tsx`. ✓
