# Remove "Coming Soon" Placeholder Messages Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the "Database Cleanup" card and its surrounding grid wrapper from the admin Maintenance page, leaving no "coming soon" placeholders.

**Architecture:** Pure frontend deletion — no new logic, no new files, no backend changes. All edits are in a single file. No TDD loop needed: this removes existing UI rather than adding behavior; the existing test suite will confirm nothing is broken.

**Tech Stack:** React 19, TypeScript, TanStack Router, Tailwind CSS v4, lucide-react

**Spec:** `docs/superpowers/specs/2026-03-17-remove-coming-soon-placeholders-design.md`

---

## Files

| Action | Path |
|--------|------|
| Modify | `frontend/src/routes/_authenticated/admin/maintenance.tsx` |

---

### Task 0: Create feature branch

- [ ] **Step 1: Create and switch to a feature branch**

```bash
git checkout -b feat/remove-coming-soon-placeholders
```

---

### Task 1: Remove `Trash2` from the lucide-react import

**Files:**
- Modify: `frontend/src/routes/_authenticated/admin/maintenance.tsx:12-19`

`Trash2` is used only in the Database Cleanup card. Remove it from the import before deleting the card so the diff stays clean and tooling doesn't complain about a dangling reference.

- [ ] **Step 1: Remove `Trash2` from the import block**

Change lines 12–19 from:

```tsx
import {
  Package,
  Trash2,
  RefreshCw,
  Loader2,
  CheckCircle,
  RotateCcw,
} from 'lucide-react';
```

To:

```tsx
import {
  Package,
  RefreshCw,
  Loader2,
  CheckCircle,
  RotateCcw,
} from 'lucide-react';
```

---

### Task 2: Update `MaintenancePageSkeleton` — remove grid and second placeholder

**Files:**
- Modify: `frontend/src/routes/_authenticated/admin/maintenance.tsx:34-37`

The skeleton mirrors the live layout. It currently has a 2-column grid with two `h-64` placeholders. After removing the Database Cleanup card the live layout is a single full-width card, so the skeleton should match.

- [ ] **Step 1: Replace the grid div with a single skeleton**

Change lines 34–37 from:

```tsx
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
      </div>
```

To:

```tsx
      <Skeleton className="h-64" />
```

---

### Task 3: Remove the grid wrapper and Database Cleanup card from `MaintenancePage`

**Files:**
- Modify: `frontend/src/routes/_authenticated/admin/maintenance.tsx:152-240`

The 2-column grid (lines 152–240) wraps both the Seed Data card and the Database Cleanup card. Remove the grid wrapper `<div>` and the entire Database Cleanup `<Card>` block. The Seed Data card renders directly into the `space-y-6` container, becoming full-width.

- [ ] **Step 1: Replace the grid wrapper block**

Change lines 152–240 from:

```tsx
      {/* Seed Data and Cleanup in 2-column grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Seed Data Section */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Package className="h-5 w-5" />
              Seed Data
            </CardTitle>
            <CardDescription>
              Load official platforms, storefronts, and default mappings
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {seedResult && (
              <Alert className="border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-200">
                <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
                <AlertTitle>Success</AlertTitle>
                <AlertDescription>
                  {seedResult.message}
                  {seedResult.totalChanges > 0 && (
                    <ul className="mt-2 list-inside list-disc text-sm">
                      <li>{seedResult.platformsAdded} platforms</li>
                      <li>{seedResult.storefrontsAdded} storefronts</li>
                      <li>{seedResult.mappingsCreated} mappings</li>
                    </ul>
                  )}
                </AlertDescription>
              </Alert>
            )}
            <p className="text-sm text-muted-foreground">
              This operation is idempotent and safe to run multiple times. Existing data will be
              preserved.
            </p>
            <Button onClick={handleLoadSeedData} disabled={isSeedLoading} className="w-full">
              {isSeedLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Loading...
                </>
              ) : (
                <>
                  <Package className="mr-2 h-4 w-4" />
                  Load Seed Data
                </>
              )}
            </Button>
          </CardContent>
        </Card>

        {/* Database Cleanup Section */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Trash2 className="h-5 w-5" />
              Database Cleanup
            </CardTitle>
            <CardDescription>Remove orphaned data and expired records</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Orphaned Files</p>
                  <p className="text-sm text-muted-foreground">
                    Remove cover art not linked to any game
                  </p>
                </div>
                <Button variant="outline" size="sm" disabled>
                  Coming Soon
                </Button>
              </div>
            </div>
            <div className="rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium">Expired Jobs</p>
                  <p className="text-sm text-muted-foreground">
                    Clean up job data older than 7 days
                  </p>
                </div>
                <Button variant="outline" size="sm" disabled>
                  Coming Soon
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
```

To:

```tsx
      {/* Seed Data Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Package className="h-5 w-5" />
            Seed Data
          </CardTitle>
          <CardDescription>
            Load official platforms, storefronts, and default mappings
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {seedResult && (
            <Alert className="border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-200">
              <CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
              <AlertTitle>Success</AlertTitle>
              <AlertDescription>
                {seedResult.message}
                {seedResult.totalChanges > 0 && (
                  <ul className="mt-2 list-inside list-disc text-sm">
                    <li>{seedResult.platformsAdded} platforms</li>
                    <li>{seedResult.storefrontsAdded} storefronts</li>
                    <li>{seedResult.mappingsCreated} mappings</li>
                  </ul>
                )}
              </AlertDescription>
            </Alert>
          )}
          <p className="text-sm text-muted-foreground">
            This operation is idempotent and safe to run multiple times. Existing data will be
            preserved.
          </p>
          <Button onClick={handleLoadSeedData} disabled={isSeedLoading} className="w-full">
            {isSeedLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Loading...
              </>
            ) : (
              <>
                <Package className="mr-2 h-4 w-4" />
                Load Seed Data
              </>
            )}
          </Button>
        </CardContent>
      </Card>
```

---

### Task 4: Verify and commit

**Files:**
- No new files

- [ ] **Step 1: Run type check and lint**

```bash
cd frontend && npm run check
```

Expected: zero errors, zero warnings.

- [ ] **Step 2: Run tests**

```bash
cd frontend && npm run test
```

Expected: all tests pass (no tests cover the deleted UI).

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/_authenticated/admin/maintenance.tsx
git commit -m "feat: remove coming soon placeholder messages from maintenance page"
```

- [ ] **Step 4: Open a PR**

```bash
gh pr create \
  --title "feat: remove coming soon placeholder messages" \
  --body "Removes the disabled Database Cleanup card (Orphaned Files + Expired Jobs) from the admin Maintenance page. The actual cleanup functionality is a separate Medium-priority roadmap item. Also updates the page skeleton to match the new single-card layout.

## Changes
- Delete Database Cleanup card and its 2-column grid wrapper
- Seed Data card is now full-width
- Remove unused \`Trash2\` lucide-react import
- Update \`MaintenancePageSkeleton\` to match new layout

Closes: Remove all 'coming soon' placeholder messages (High)"
```

- [ ] **Step 5: Review PR diff, then merge**

```bash
gh pr view --web   # review the diff in browser
gh pr merge --squash --delete-branch
```
