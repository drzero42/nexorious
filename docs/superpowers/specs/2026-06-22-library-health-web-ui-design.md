# Library Health page (web UI) — design

**Issue:** [#1145](https://github.com/drzero42/nexorious/issues/1145) — the web-UI child of the
Library Smells epic [#1143](https://github.com/drzero42/nexorious/issues/1143).
**Status:** design approved, awaiting implementation plan.
**Depends on:** [#1144](https://github.com/drzero42/nexorious/issues/1144) — the detection engine +
REST API backbone, **already landed** (`internal/librarysmells`, `internal/api/library_smells.go`,
routed at `/api/library/smells`). This issue is frontend-only and consumes that API unchanged.

## Summary

A "Library Health" page in the React SPA that surfaces the 10 library-smell checks grouped by their
two severity tiers. Each check is an expandable section showing its flagged-game count and the
flagged games; auto-fixable checks offer per-game and "apply to all" one-click fixes, manual checks
deep-link to the game's edit view, and every check supports per-item dismiss with a per-section
restore view.

No backend changes. The page is a pure consumer of the existing REST API.

## API consumed (already landed, unchanged)

Base group `/api/library/smells` (auth-gated, user-scoped). All listing endpoints share the
`{items, total, page, per_page, pages}` envelope (`page` 1-indexed, `per_page` 1–200, default 25).

| Method | Path | Returns |
|---|---|---|
| `GET` | `/api/library/smells` | `SmellSummaryItem[]` = `{id, title, description, tier, auto_fixable, count}` for all 10 checks (count is post-ignore). |
| `GET` | `/api/library/smells/:checkID` | Paginated `FlaggedItem[]`. 404 on unknown slug. |
| `POST` | `/api/library/smells/:checkID/apply` | Body `{user_game_ids}` (≤200). 422 if not auto-fixable. Returns `{applied, skipped}`. Re-validates server-side (TOCTOU-safe). |
| `POST` | `/api/library/smells/:checkID/ignore` | Body `{user_game_ids}` (≤200). Returns `{ignored}`. |
| `DELETE` | `/api/library/smells/:checkID/ignore` | Body `{user_game_ids}` (≤200). Returns `{restored}`. |
| `GET` | `/api/library/smells/:checkID/ignored` | Paginated dismissed items `{user_game_id, title, created_at}`. |

`FlaggedItem` = `{user_game_id, game_id, title, cover_art_url?, suggested_status?, detail?}`
(context fields omitempty; only the ones a given check needs are set). **One item per game** — the
platform-level checks dedupe across a game's platform rows (see *Refinement* below), so a check never
counts the same game twice.

**The 10 checks** (slug → tier, auto-fix):

- Tier `inconsistency`: `storefront-less-platform` (deep-link), `orphan-game` (deep-link),
  `wishlisted-yet-owned` (**auto-fix** → clear wishlist), `missing-ownership-status` (deep-link),
  `impossible-acquired-date` (deep-link; `detail` text), `invalid-storefront-for-platform`
  (deep-link).
- Tier `nudge`: `beat-but-not-marked` (**auto-fix** → completed), `played-but-not-started`
  (**auto-fix** → in_progress), `in-progress-untouched` (**auto-fix** → not_started),
  `unrated-after-finishing` (deep-link).

Auto-fixable set: `wishlisted-yet-owned`, `beat-but-not-marked`, `played-but-not-started`,
`in-progress-untouched`. Tier display order: **Inconsistencies first, then Nudges** (registry
order within each tier).

## Route, nav & page shell

- Route file `ui/frontend/src/routes/_authenticated/library-health.tsx` → path `/library-health`.
  `routeTree.gen.ts` regenerated via `npm run build` and committed alongside.
- Nav entry in `components/navigation/nav-items.tsx`: label **"Library Health"**, a Lucide icon
  (e.g. `Stethoscope`). Dynamic badge = total **Tier-1 (inconsistency)** flagged count (sum of
  `count` over inconsistency checks), surfaced via the existing `useNavItems()` mechanism; the badge
  hides at zero. (Nudges are not "wrong", so they do not drive the badge.)
- Shell follows the existing authenticated-page pattern: `<div className="space-y-6">`, an `<h1>`
  header with a one-line subtitle and a manual **Refresh** button, `Suspense` + `Skeleton` fallback,
  and the shared error-state block with a Retry button.

## Data layer

- `ui/frontend/src/api/library-health.ts` — typed functions over the shared `api.*` client:
  `getSmellSummary()`, `getSmellItems(checkID, {page, perPage})`, `getIgnoredItems(checkID,
  {page, perPage})`, `applySmell(checkID, ids)`, `ignoreSmell(checkID, ids)`,
  `restoreSmell(checkID, ids)`. Response types mirror the API verbatim (snake_case fields):
  `SmellSummaryItem`, `FlaggedItem`, `IgnoredItem`, and the paginated envelopes.
- `ui/frontend/src/hooks/use-library-health.ts` — a `smellKeys` query-key factory and:
  - `useSmellSummary()` — drives the whole page.
  - `useSmellItems(checkID, params, { enabled })` — lazily fetched when a check is expanded.
  - `useIgnoredItems(checkID, params, { enabled })` — lazily fetched when the dismissed toggle opens.
  - `useApplySmell()`, `useIgnoreSmell()`, `useRestoreSmell()` — mutations that on success
    invalidate `smellKeys.summary()` plus the affected check's list and ignored keys.
  - Exported from `ui/frontend/src/hooks/index.ts`.

`smellKeys` shape (mirrors `gameKeys`):

```ts
const smellKeys = {
  all: ['librarySmells'] as const,
  summary: () => [...smellKeys.all, 'summary'] as const,
  list: (checkID: string, params?: PageParams) => [...smellKeys.all, 'list', checkID, params] as const,
  ignored: (checkID: string, params?: PageParams) => [...smellKeys.all, 'ignored', checkID, params] as const,
};
```

## Page structure

- One `useSmellSummary()` call drives the layout. Checks are partitioned by tier into two labeled
  blocks: **Inconsistencies** then **Nudges**, each showing how many checks are affected.
- Each check renders as an **Accordion** item (shadcn `accordion`):
  - Header: title, a tier Badge, an "Auto-fix" Badge when `auto_fixable`, and the flagged `count`.
  - Zero-count checks render instead as a **muted, non-expandable "All clear ✓" row** in their tier
    (no accordion trigger, no fetch).
- Expanding a check lazily fetches its listing (`useSmellItems`, `enabled` on open). Flagged items
  render in a compact **Table**:
  - Cover thumbnail + title, the title linking to `/games/$id/edit`.
  - Check context column: just `detail` when present (e.g. the impossible-acquired-date reason).
    No per-platform context — see *Refinement*.
  - A per-row actions cell (see Interactions).
  - A leading Checkbox per row for multi-select is **out of scope** for v1 — actions are per-row
    plus a single section-level "apply to all". (Revisit if needed.)
  - Pagination control (shadcn `pagination`) when `pages > 1`; default 25/page.
- A small **"Show dismissed (N)"** toggle per check section reveals that check's ignored items
  (`useIgnoredItems`, `enabled` on toggle) in a simple list with a **Restore** button each. `N` comes
  from the ignored listing's `total` (fetched when toggled).

## Interactions

- **Auto-fixable checks** — per-row **"Apply"** button (direct; a single low-risk reversible-by-edit
  mutation, no confirm) and a section-level **"Apply to all (N)"** button that opens an
  **AlertDialog** confirm (destructive `AlertDialogAction` styling per project convention) before
  bulk-applying. Bulk apply collects every flagged `user_game_id` for the check (fetch at
  `per_page=200`; loop pages only if `total > 200`) and POSTs in chunks of ≤200 to respect the API
  cap. On success: toast `applied`/`skipped` and invalidate. (Solo user / small libraries → in
  practice one page, one call.)
- **Manual (deep-link) checks** — per-row **"Fix"** that navigates to `/games/$id/edit`. The user
  fixes the game (all its platforms) there; the edit page is not modified.
- **Ignore** — per-row **"Ignore"** on every check (reversible via restore, so no confirm). Removes
  the row from the active listing and decrements the summary count on invalidation.
- **Restore** — in the per-section dismissed sub-view; re-runs detection so the item reappears in the
  active listing if it still flags.

## States

- **Loading:** Skeleton roughly matching the tier/accordion layout.
- **Error:** shared error block (icon + message + Retry calling `refetch`).
- **Empty / all-clear:** when every check has `count === 0`, a celebratory empty state ("Your library
  is in great shape 🎉") shown above the all-clear check rows.

## Testing

Vitest + Testing Library component tests for the high-value logic (mock the hooks / `api` layer per
existing frontend test patterns — e.g. `game-card.test.tsx`):

- Tier grouping renders Inconsistencies before Nudges; a zero-count check renders as a
  non-expandable "All clear" row (no table, no fetch).
- An auto-fixable check shows per-row "Apply" + section "Apply to all"; a manual check shows "Fix"
  (deep-link) and no "Apply".
- "Apply to all" opens the confirm dialog and fires the apply mutation with the flagged ids only on
  confirm; cancel fires nothing.
- Ignore → the row leaves the active listing; the dismissed toggle lists it; Restore calls the
  restore mutation.
- "Fix" / title link targets `/games/$id/edit` with the correct id.
- A check renders one row per game even when a game has multiple offending platform rows
  (dedupe regression test in `internal/librarysmells`).

## Refinement (post-implementation)

Two behaviours were adjusted after first use, against the originally-merged engine (#1144):

- **Dedupe to one item per game.** The four platform-level checks (`storefront-less-platform`,
  `missing-ownership-status`, `impossible-acquired-date`, `invalid-storefront-for-platform`)
  originally emitted one `FlaggedItem` **per offending `user_game_platforms` row**, so a game with
  several offending platforms was counted and listed multiple times (e.g. 2819 findings over 1823
  games). They now select from `user_games` with an `EXISTS` over the platform rows, yielding one
  item per game; the count is "distinct flagged games". All four are deep-link only, so no per-row
  fix is lost — "Fix" opens the game's edit page where every platform is fixed together.
- **Dropped per-platform context.** `platform_row_id`, `platform`, `storefront`, and
  `suggested_storefront` were removed from `FlaggedItem` (Go + TS). The suggestion ("Suggested:
  Steam") rendered without the platform name and — since the edit form is not pre-filled — added no
  value. Rows now show just the game, plus `detail` where a check sets it (the
  impossible-acquired-date reason). `suggested_status` is retained on the wire (unused by the UI).
- **On-open re-run.** Because a fix made via the "Fix" deep-link mutates the games cache (not the
  smells cache) and queries have a 5-min `staleTime`, the page invalidates the whole `smellKeys`
  tree on mount and from the Refresh button, so returning from an edit reflects the fix.

## Out of scope

- The `nexctl doctor` CLI + MCP tools are [#1146](https://github.com/drzero42/nexorious/issues/1146),
  a separate child. (The detector dedupe in *Refinement* is the only backend change here; the rest is
  frontend.)
- Editing the game edit page to pre-fill a suggested value (the suggestion was dropped entirely — see
  *Refinement*).
- Multi-select checkboxes on flagged rows (per-row + apply-all cover v1).
- A cross-check "fix everything" — apply is always scoped to a single check (epic rule).
