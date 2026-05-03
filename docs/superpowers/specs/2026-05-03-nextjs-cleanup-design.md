# Design: Remove Next.js Remnants from Frontend

**Date:** 2026-05-03  
**Status:** Approved

## Background

The frontend was originally written in Next.js and later migrated to Vite + React + TanStack Router. The migration left behind several Next.js-specific artefacts that are either dead code (no-ops in Vite) or stale comments. This cleanup removes them in a single pass.

`next-themes` is intentionally kept — it works correctly in Vite and is not Next.js-specific.

## Scope

All changes are in `frontend/`. No backend changes. No logic changes anywhere — this is pure dead-code and stale-comment removal.

## Changes

### 1. `package.json`

- Rename `"name"` from `"frontend-next"` to `"frontend"`
- Remove `"next": "^16.0.10"` from `dependencies`
- Remove `"eslint-config-next": "^16.0.10"` from `dependencies`
- Run `npm install` to regenerate `package-lock.json`

The ESLint config (`eslint.config.mjs`) does not reference `eslint-config-next`, so no ESLint changes are needed.

### 2. Strip `'use client'` directives (39 files)

Remove the `'use client';` line (and its trailing blank line where present) from every file that has it. The directive is a Next.js React Server Components instruction — it is a no-op in a Vite SPA and should not be present.

Affected files:
- `src/components/dashboard/progress-statistics.tsx`
- `src/components/dashboard/status-progress.tsx`
- `src/components/games/bulk-actions.tsx`
- `src/components/games/game-filters.tsx`
- `src/components/games/game-grid.tsx`
- `src/components/jobs/job-item-card.tsx`
- `src/components/jobs/job-progress-card.tsx`
- `src/components/jobs/recent-activity.tsx`
- `src/components/navigation/nav-items.tsx`
- `src/components/navigation/nav-section.tsx`
- `src/components/sync/epic-auth-dialog.tsx`
- `src/components/sync/epic-connection-card.tsx`
- `src/components/sync/psn-connection-card.tsx`
- `src/components/sync/steam-connection-card.tsx`
- `src/components/ui/accordion.tsx`
- `src/components/ui/alert-dialog.tsx`
- `src/components/ui/avatar.tsx`
- `src/components/ui/checkbox.tsx`
- `src/components/ui/collapsible.tsx`
- `src/components/ui/command.tsx`
- `src/components/ui/dialog.tsx`
- `src/components/ui/dropdown-menu.tsx`
- `src/components/ui/form.tsx`
- `src/components/ui/label.tsx`
- `src/components/ui/multi-select-filter.tsx`
- `src/components/ui/notes-editor.tsx`
- `src/components/ui/platform-selector.tsx`
- `src/components/ui/popover.tsx`
- `src/components/ui/progress.tsx`
- `src/components/ui/scroll-area.tsx`
- `src/components/ui/select.tsx`
- `src/components/ui/separator.tsx`
- `src/components/ui/sheet.tsx`
- `src/components/ui/sonner.tsx`
- `src/components/ui/star-rating.tsx`
- `src/components/ui/tabs.tsx`
- `src/components/ui/tag-selector.tsx`
- `src/components/ui/tooltip.tsx`
- `src/providers/query-provider.tsx`

### 3. Remove dead test mocks (3 files)

The following test files mock Next.js modules that the production components no longer import. The mocks are dead code and must be removed.

**`src/components/games/game-card.test.tsx`**  
Remove the `vi.mock('next/image', ...)` block and its preceding comment. The component renders a plain `<img>` tag, not `next/image`.

**`src/components/games/game-list.test.tsx`**  
Same as above.

**`src/components/navigation/nav-section.test.tsx`**  
Remove the `vi.mock('next/navigation', ...)` block and its preceding comment. The component uses `@tanstack/react-router`, not Next.js navigation.

### 4. Remove stale path-header comments (14 files)

Several files have a first-line comment containing the old file path from when the project lived in a `frontend-next/` or `frontend/` directory. Remove these lines.

Affected files:
- `src/api/sync.test.ts` — `// frontend-next/src/api/sync.test.ts`
- `src/components/navigation/index.ts`
- `src/components/navigation/nav-items.tsx`
- `src/components/navigation/nav-link.test.tsx`
- `src/components/navigation/nav-section.test.tsx`
- `src/components/navigation/nav-section.tsx`
- `src/components/navigation/types.ts`
- `src/components/ui/nav-badge.test.tsx`
- `src/components/ui/nav-badge.tsx`
- `src/lib/env.ts`
- `src/main.tsx`
- `src/routes/__root.tsx`
- `src/routes/_authenticated.tsx`
- `vite.config.ts`

## Verification

After all changes:

```bash
cd frontend
npm install          # regenerate lockfile after package.json changes
npm run check        # tsc --noEmit && eslint — must pass with zero errors
npm run test         # vitest run — all tests must pass
```
