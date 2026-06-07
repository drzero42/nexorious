# Allow duplicate platforms on a game (#848)

**Status:** design
**Issue:** #848
**Builds on:** #851 (platform-row identity model) — see
`memory/project_edit_page_platform_identity.md`

## Problem

A game can hold multiple associations of one platform distinguished by
storefront. The DB enforces uniqueness on `(user_game_id, platform, storefront)`
with `NULLS NOT DISTINCT`, so e.g. `PC (Windows)` + Steam *and* `PC (Windows)` +
Epic coexist. Imports and sync create these freely; the backend supports them.

The frontend cannot. Both platform selectors identify a platform by **name**, so
there is no affordance to add a second copy. On the edit page the dropdown is a
*toggle* — clicking an already-selected platform *removes* it. A user who owns a
game on Steam and Epic, having added `PC (Windows)` + Steam, has no way to add
the Epic copy.

This is frontend-only. #851 already established the `key`/`id` identity model on
`PlatformSelection` and the `planPlatformChanges` reconcile (selections without
an `id` are adds). This work replaces the name-based *add* affordance #851
deliberately left in place, on that same foundation. `planPlatformChanges`'s
diff-by-`id` logic and the identity scheme are not changed.

## Scope

In scope:

1. **Edit page** (`PlatformSelector`): row-based editor that allows adding a
   second copy of a platform with a different storefront.
2. **Add wizard** (`PlatformSelectorCompact`): allow adding a second storefront
   copy of a checked platform, preserving the existing IGDB-suggestion layout.
3. **Edit page**: ownership / hours / acquired inputs for *newly-added* rows
   (today they only exist for already-persisted rows), so a new copy can be
   detailed before its first save.

Out of scope:

- **`acquired_date` persistence** is broken in the backend for *all* rows
  (create and update) — tracked as **#849**. This work renders and sends the
  acquired input for new rows so it works the moment #849 lands, but the value
  will not persist until then. Behavior is unchanged from the existing
  persisted-row card, which already silently drops it.
- The add wizard has no ownership/hours/acquired inputs at all; this work does
  not add them there. The compact variant only gains duplicate support.

## Design

### Shared core: `platform-options.ts` (new, pure, unit-tested)

A platform has **N + 1 storefront "slots"**: its N storefronts plus the
`undefined` ("No storefront") slot. Duplicates are made *structurally
impossible* by never offering a slot already taken by a sibling row of the same
platform — so the backend 409 path is never exercised from the UI.

```ts
type Row = Pick<PlatformSelection, 'key' | 'platform' | 'storefront'>;

// Storefront values (incl. undefined) used by OTHER rows of `platformName`.
function usedStorefronts(rows: Row[], platformName: string, exceptKey?: string): Set<string | undefined>;

// Selectable storefronts for one row: the platform's storefronts minus slots
// used by siblings. The row's OWN current value always remains selectable.
function availableStorefronts(platform: Platform, rows: Row[], currentRowKey: string): Storefront[];

// Whether every slot of `platform` is already taken (→ not a valid add target).
function isPlatformExhausted(platform: Platform, rows: Row[]): boolean;

// Slot to auto-assign when a platform is added/selected: default_storefront if
// free, else first free storefront, else undefined ("No storefront").
function firstFreeStorefront(platform: Platform, rows: Row[]): string | undefined;
```

`undefined` (No storefront) is treated as a normal slot, so a platform with no
storefronts (1 slot) allows exactly one row; a platform with 2 storefronts
allows up to 3 rows (Steam, Epic, No storefront).

### `PlatformSelector` (edit page) — row-based editor

Retire the toggle-checklist. The component renders a list of selected rows; each
row is:

- a **searchable platform picker** (Popover + Command, reusing the current
  search building blocks) whose options disable platforms where
  `isPlatformExhausted` is true (the row's own platform stays selectable);
- a **storefront `Select`** whose options come from `availableStorefronts`;
- a **remove** button (per-row, by `key`).

A **"+ Add platform"** button appends a blank row (`key` stamped via
`newSelectionKey()`, no `id`). Selecting a platform in a row auto-assigns
`firstFreeStorefront`. Rows remain keyed by `key`; adds are selections without
an `id`, consumed unchanged by `planPlatformChanges`.

`maxSelection` is removed — no caller passes it (verified: only `tag-selector`,
a separate component, uses its own `maxSelection`), and it conflicts with the
slot model.

### `PlatformSelectorCompact` (add wizard) — checkbox model + extra storefronts

Keep the checkbox-per-platform list (the add wizard renders it three times with
the same `selectedPlatforms`/`onChange` over different `availablePlatforms`
subsets — IGDB-suggested, "other", all — so the partitioned suggestion UX must
survive). Changes:

- when a platform is checked, alongside its storefront selector show a small
  **"+ add another storefront"** affordance that appends another row of the same
  platform, with its storefront defaulted via `firstFreeStorefront` and
  constrained via `availableStorefronts`; hidden once the platform is exhausted;
- each extra row gets its own storefront `Select` (constrained) and a remove
  control;
- unchecking a platform removes **all** rows of that platform (consistent with
  today's name-based filter).

No ownership/hours/acquired inputs are added here.

### New-row details on the edit page (`game-edit-form.tsx`)

Today the per-platform details section (ownership / acquired / hours) maps over
`game.platforms` — the **persisted** rows — so an added row has no detail
inputs until save + reload. Change it to drive off `selectedPlatforms`:

- re-key `platformPlaytimes` and `platformOwnership` from row `id` to row `key`
  (persisted rows already use `key === id`, so existing entries map cleanly);
- render one detail card per `selectedPlatforms` row, labelled from the
  `platforms` lookup (platform + storefront display names); new rows default to
  ownership `OWNED`, hours `0`, acquired `''`;
- Steam-sync hours-lock (`isSteamSynced`) keys off the row's storefront as
  today.

This keeps `PlatformSelector` responsible only for platform+storefront identity
(storefront control stays there); ownership/hours/acquired stay in the detail
cards. The two-section layout (selector above, detail cards below) is unchanged
— it already shows each platform in both sections today; this only extends the
detail cards to cover not-yet-persisted rows.

### Plumbing

- **`platform-reconcile.ts`** — `adds` carry `hoursPlayed` / `ownershipStatus` /
  `acquiredDate` from `details`; the `details` lookup uses the row `key` for
  both adds and updates (was `id`). Diff-by-`id` for removes/updates is
  unchanged.
- **`game-edit-form.tsx`** `handleSave` — build `details` keyed by `key`; pass
  add detail fields to `addPlatform.mutateAsync`.
- **`api/games.ts`** `addPlatformToUserGame` — add `ownership_status` and
  `acquired_date` to the request body (currently omitted). `UserGamePlatformData`
  already carries these fields.

No backend changes. `HandleCreatePlatform` already reads `OwnershipStatus` and
`HoursPlayed`; `acquired_date` remains dropped pending #849.

## Testing (TDD)

- `platform-options.test.ts` (new): slot math, sibling exclusion, own-value
  retained, exhaustion (0-storefront and N-storefront platforms), default-slot
  selection.
- `platform-reconcile.test.ts`: adds carry detail fields; key-based lookup;
  existing remove/update diffs still hold.
- `games.test.ts`: `addPlatformToUserGame` sends `ownership_status` and
  `acquired_date`.
- `platform-selector.test.tsx` (rewrite): row-based `PlatformSelector` — add
  row, change platform, constrained storefronts, exhaustion disables platform,
  remove by row; compact "+ add another storefront" add/constrain/remove,
  uncheck removes all copies.

## Acceptance

- On the edit page a user can add `PC (Windows)` + Steam and `PC (Windows)` +
  Epic to the same game and save successfully; both persist and reload.
- The UI never lets two rows resolve to the same `(platform, storefront)`.
- A newly-added row's ownership and hours can be set before the first save and
  persist. Acquired date is editable and sent, persisting once #849 lands.
- The add wizard can register two storefront copies of one platform.
