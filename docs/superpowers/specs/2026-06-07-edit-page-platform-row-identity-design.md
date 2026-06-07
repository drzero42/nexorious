# Edit-page platform row identity — design

**Date:** 2026-06-07
**Issues:** #846 (removing a platform removes the wrong storefront), #847 (storefront change not saved)
**Out of scope / related:** #848 (manual duplicate-platform add — deferred), #849 (acquired-date never persisted — separate)

## Problem

On the Edit game page, platform associations are managed through the
`PlatformSelector` component and reconciled on save by **platform name**. But the
domain allows multiple associations per platform name, distinguished by
storefront and identified by a row UUID. The database enforces this:

```sql
CREATE UNIQUE INDEX user_game_platforms_uniq
    ON user_game_platforms (user_game_id, platform, storefront) NULLS NOT DISTINCT;
```

So `(pc, steam)` and `(pc, NULL)` are two distinct, valid rows — routinely
created by sync/import. The frontend collapses a game's platforms into a
name-keyed list, and both bugs fall out of that single flaw.

### #846 — removing a platform removes the wrong storefront

When `game.platforms` has two rows with the same name, the edit form's
`selectedPlatforms` holds two entries that are indistinguishable (same name, no
identity). Consequences chain:

1. Both render with the same React key `key={selection.platform}`
   (`platform-selector.tsx`) — the list cannot tell them apart.
2. `handleRemovePlatform(name)` filters out **all** entries with that name — one
   click removes both from the UI model.
3. On save, `getPlatformAssociationId(name)` resolves to
   `game.platforms.find(p => p.platform === name)?.id` — the **first** row with
   that name — so the DELETE targets the first row regardless of which one the
   user clicked. The row the user wanted to keep may be the one deleted.

### #847 — storefront change not saved

The edited storefront lives only in `selectedPlatforms[i].storefront`
(`handleStorefrontChange` in `platform-selector.tsx`). `handleSave`
(`game-edit-form.tsx`) consumes `selectedPlatforms` **solely** to compute
adds/removes by name. A storefront-only edit leaves the name unchanged, so the
row is neither added nor removed. The one path that could update it — the
per-row update loop keyed by `p.id` — (a) does not include storefront in its
`needsUpdate` check, and (b) sends `storefront: originalPlatform.storefront`,
the original DB value. The changed storefront is therefore never read by any
save path.

(Backend is correct: `HandleUpdatePlatform` persists storefront at
`user_games.go:1057-1059` and handles the unique-constraint conflict. No backend
change is needed.)

`handleStorefrontChange` is also name-keyed, so on a duplicated platform it would
change the storefront on **all** same-name rows — a second facet of the same
root cause.

## Root cause

`PlatformSelector`'s per-row operations and `game-edit-form`'s save
reconciliation identify platform rows by **platform name**, while the true row
identity is the DB UUID (`id`) — or, for an unsaved row, a stable client-side
key. Name is not unique, so removal, storefront editing, and save reconciliation
all resolve to "the first/all rows with this name."

## Approach

Surgical, frontend-only. Give selection rows a stable identity and reconcile by
it. **Keep the existing layout** (the dropdown checklist and the separate
per-platform details section stay where they are). **Leave the dropdown's
add/toggle semantics untouched** — that affordance is what #848 will replace, so
reworking it now would be throwaway work.

### Identity model

A platform selection carries two distinct, principled notions of identity, and
we apply them **uniformly across every consumer of the shape** (both the edit
flow and the add-game flow). This removes the prior smell where the same shape
meant different things to different callers.

```ts
export interface PlatformSelection {
  key: string;        // stable client-side identity — REQUIRED, present the moment the row exists
  id?: string;        // server UUID — OPTIONAL, present only once persisted
  platform: string;
  storefront?: string;
}
```

- **`key` — required.** "Which row is this, within the current editing session."
  A row always knows who it is the moment it appears on screen, regardless of
  flow. Every point that creates a selection stamps a fresh `key` (e.g.
  `crypto.randomUUID()`); when seeding from saved data, the `key` is set from the
  row id. Because it is always present, the per-row list keys off it directly —
  no defensive fallback needed.
- **`id` — optional.** "Does this row correspond to something already saved in
  the database." A freshly-added row genuinely has none, so optionality here is
  accurate modelling, not debt. Save reconciliation uses presence of `id` to
  distinguish persisted rows from new ones.

Making `key` required brings the add-game flow into scope (see below) for a
small, mechanical change: it must stamp a `key` wherever it creates a selection.
This is type-driven and carries **no behaviour or UX change** in that flow.

### `PlatformSelector` changes (per-row list only)

- Render the selected-rows list with `key={selection.key}` instead of
  `key={selection.platform}` — removes the duplicate-key collision.
- `handleRemovePlatform(key)` removes the single row whose `key` matches.
- `handleStorefrontChange(key, storefront)` updates the single matching row.
- `handlePlatformToggle` (the dropdown) is unchanged **except** that the add
  branch assigns a fresh `key` to each new row. No UX/behavior change; its
  uncheck still removes all rows of a name (revisited under #848).

### Add-game flow changes (`add.confirm.tsx`, `PlatformSelectorCompact`)

Mechanical only, to honour the now-required `key`:

- Wherever a selection is created (e.g. when a platform checkbox is toggled on),
  stamp a fresh `key`.
- Internal lookups that currently match a selection by `platform` name can stay
  as they are — `PlatformSelectorCompact` renders one row per *available*
  platform and cannot produce duplicates, so name matching remains unambiguous
  there. The `key` is carried for shape consistency, not yet used to disambiguate.
- No UX or behaviour change. Duplicate-platform adds in this flow remain #848.

### `game-edit-form.tsx` changes

- Seed `selectedPlatforms` from `game.platforms` as
  `{ key: p.id, id: p.id, platform: p.platform!, storefront: p.storefront }`.
- Rewrite the platform reconciliation in `handleSave` to a targeted diff by
  `id`:
  - **Add:** selections with no `id` → `POST` `{ platform, storefront }`.
  - **Remove:** original row ids (from `game.platforms`) not present among the
    current selections' ids → `DELETE` by that `id`.
  - **Update:** for each row present in both (matched by `id`), if the storefront
    (from the selection) **or** the ownership / acquired-date / hours (from the
    existing per-row detail state) changed, issue **one merged `PUT`** for that
    row. This folds the old separate storefront and ownership/hours update paths
    into a single call per changed row.
- Remove the name-based `getPlatformAssociationId` helper.

The per-row details section (ownership / acquired date / hours), already keyed by
`p.id`, is unchanged in layout and continues to feed the merged update.

## Data flow (after)

1. Load: `game.platforms` → `selectedPlatforms` with `key=id=p.id`, plus the
   existing `platformOwnership` / `platformPlaytimes` keyed by `p.id`.
2. Edit: per-row X / storefront dropdown mutate the matching row by `key`;
   ownership/date/hours mutate detail state by `id`.
3. Save: reconcile by `id` → POST (adds) / DELETE (removes) / merged PUT
   (changed existing rows).

## Error handling

No new error surfaces. Existing mutation error handling in `handleSave`
(try/catch → toast) is retained. The backend already returns `409` on a
unique-constraint conflict; that path is unchanged and surfaces through the
existing toast.

## Testing

- `platform-selector.test.tsx`
  - Removing one of two same-name rows (different storefronts) removes only the
    targeted row and leaves its sibling.
  - Changing the storefront on one of two same-name rows updates only that row.
- `game-edit-form.test.tsx`
  - Editing the storefront of an existing row issues a `PUT` carrying the new
    storefront (#847).
  - Removing one of two same-name rows issues a `DELETE` for the correct `id`
    and leaves the sibling (#846).
- Add-game flow: existing `add.confirm` behaviour is unchanged. Confirm the
  current tests still pass (selecting/deselecting a platform, picking a
  storefront, and submitting); no new behaviour to assert beyond that the
  `key`-stamping does not regress selection.

## Scope guardrails

- Frontend only. No backend or migration changes — the API and schema already
  support everything described.
- In scope, deliberately: a uniform identity model across **both** the edit flow
  and the add-game flow. The add flow's inclusion is limited to mechanical
  `key`-stamping with no behaviour change — done to keep the shared shape
  consistent and avoid leaving debt, not to add features.
- Out of scope: dropdown add/toggle semantics, consolidating the two UI sections,
  and manual duplicate-platform adds in either flow (all → #848); acquired-date
  persistence (→ #849).
