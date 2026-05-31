# Issue #697 – Per-Route Page Titles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the mobile Firefox bug where tab titles show the full URL by adding `head` functions to every content route so `document.title` is always set programmatically on navigation.

**Architecture:** Add `head: () => ({ meta: [{ title: 'Nexorious' }] })` to the root route as a fallback, render `<HeadContent />` once in `RootComponent`, then add per-route `head` functions with descriptive titles across all 17 content routes. TanStack Router deduplicates head entries and writes the most-specific title to the DOM on every client-side navigation.

**Tech Stack:** TanStack Router v1 (`@tanstack/react-router ^1.163.3`), React 19, TypeScript, Vite

---

### Task 1: Create feature branch

- [ ] **Step 1: Create and switch to the feature branch**

```bash
git checkout -b fix/issue-697-per-route-page-titles
```

---

### Task 2: Update root route with `head` fallback and `HeadContent`

**Files:**
- Modify: `ui/frontend/src/routes/__root.tsx`

- [ ] **Step 1: Update `__root.tsx`**

Replace the entire file with:

```tsx
import { createRootRoute, HeadContent, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider, AuthProvider } from '@/providers';
import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/700.css';
import '@fontsource/geist-mono/400.css';
import '@/styles/globals.css';

export const Route = createRootRoute({
  head: () => ({
    meta: [{ title: 'Nexorious' }],
  }),
  component: RootComponent,
});

function RootComponent() {
  return (
    <QueryProvider>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <AuthProvider>
          <HeadContent />
          <Outlet />
          <Toaster />
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/__root.tsx
git commit -m "fix: add HeadContent and default title to root route"
```

---

### Task 3: Add titles to login and dashboard routes

**Files:**
- Modify: `ui/frontend/src/routes/_public/login.tsx` (line 10)
- Modify: `ui/frontend/src/routes/_authenticated/dashboard.tsx` (line 10)

- [ ] **Step 1: Add `head` to login route**

In `ui/frontend/src/routes/_public/login.tsx`, the route definition at line 10 currently reads:

```tsx
export const Route = createFileRoute('/_public/login')({
  component: LoginPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_public/login')({
  head: () => ({ meta: [{ title: 'Login | Nexorious' }] }),
  component: LoginPage,
});
```

- [ ] **Step 2: Add `head` to dashboard route**

In `ui/frontend/src/routes/_authenticated/dashboard.tsx`, the route definition at line 10 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/dashboard')({
  component: DashboardPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/dashboard')({
  head: () => ({ meta: [{ title: 'Dashboard | Nexorious' }] }),
  component: DashboardPage,
});
```

- [ ] **Step 3: Commit**

```bash
git add ui/frontend/src/routes/_public/login.tsx \
        ui/frontend/src/routes/_authenticated/dashboard.tsx
git commit -m "fix: add page titles to login and dashboard routes"
```

---

### Task 4: Add titles to games routes

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/games/index.tsx` (line 10)
- Modify: `ui/frontend/src/routes/_authenticated/games/add.index.tsx` (line 11)
- Modify: `ui/frontend/src/routes/_authenticated/games/add.confirm.tsx` (line 27)
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.index.tsx` (line 28)
- Modify: `ui/frontend/src/routes/_authenticated/games/$id.edit.tsx` (line 9)

Layout routes `add.tsx` and `$id.tsx` are pure `<Outlet />` pass-throughs — do not modify them.

- [ ] **Step 1: Add `head` to games index route**

In `ui/frontend/src/routes/_authenticated/games/index.tsx`, line 10 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/games/')({
```

Add `head` as the first property in the options object:

```tsx
export const Route = createFileRoute('/_authenticated/games/')({
  head: () => ({ meta: [{ title: 'Library | Nexorious' }] }),
```

- [ ] **Step 2: Add `head` to add game index route**

In `ui/frontend/src/routes/_authenticated/games/add.index.tsx`, the route definition at line 11 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/games/add/')({
  component: AddGamePage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/games/add/')({
  head: () => ({ meta: [{ title: 'Add Game | Nexorious' }] }),
  component: AddGamePage,
});
```

- [ ] **Step 3: Add `head` to add game confirm route**

In `ui/frontend/src/routes/_authenticated/games/add.confirm.tsx`, the route definition at line 27 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/games/add/confirm')({
  component: AddGameConfirmPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/games/add/confirm')({
  head: () => ({ meta: [{ title: 'Add Game | Nexorious' }] }),
  component: AddGameConfirmPage,
});
```

- [ ] **Step 4: Add `head` to game detail route**

In `ui/frontend/src/routes/_authenticated/games/$id.index.tsx`, the route definition at line 28 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/games/$id/')({
  component: GameDetailPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/games/$id/')({
  head: () => ({ meta: [{ title: 'Game Details | Nexorious' }] }),
  component: GameDetailPage,
});
```

- [ ] **Step 5: Add `head` to game edit route**

In `ui/frontend/src/routes/_authenticated/games/$id.edit.tsx`, the route definition at line 9 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/games/$id/edit')({
  component: GameEditPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/games/$id/edit')({
  head: () => ({ meta: [{ title: 'Edit Game | Nexorious' }] }),
  component: GameEditPage,
});
```

- [ ] **Step 6: Commit**

```bash
git add "ui/frontend/src/routes/_authenticated/games/index.tsx" \
        "ui/frontend/src/routes/_authenticated/games/add.index.tsx" \
        "ui/frontend/src/routes/_authenticated/games/add.confirm.tsx" \
        "ui/frontend/src/routes/_authenticated/games/\$id.index.tsx" \
        "ui/frontend/src/routes/_authenticated/games/\$id.edit.tsx"
git commit -m "fix: add page titles to games routes"
```

---

### Task 5: Add titles to sync routes

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/index.tsx` (line 21)
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx` (line 65)

- [ ] **Step 1: Add `head` to sync index route**

In `ui/frontend/src/routes/_authenticated/sync/index.tsx`, the route definition at line 21 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/sync/')({
  component: SyncPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/sync/')({
  head: () => ({ meta: [{ title: 'Sync | Nexorious' }] }),
  component: SyncPage,
});
```

- [ ] **Step 2: Add `head` to sync storefront route**

In `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`, the route definition at line 65 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/sync/$storefront')({
  component: StorefrontSyncPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/sync/$storefront')({
  head: ({ params }) => {
    const name = params.storefront.charAt(0).toUpperCase() + params.storefront.slice(1);
    return { meta: [{ title: `${name} Sync | Nexorious` }] };
  },
  component: StorefrontSyncPage,
});
```

This produces titles like "Steam Sync | Nexorious", "Psn Sync | Nexorious", etc. (`params.storefront` values are `steam`, `psn`, `gog`, `epic`.)

- [ ] **Step 3: Commit**

```bash
git add "ui/frontend/src/routes/_authenticated/sync/index.tsx" \
        "ui/frontend/src/routes/_authenticated/sync/\$storefront.tsx"
git commit -m "fix: add page titles to sync routes"
```

---

### Task 6: Add titles to remaining authenticated routes

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/import-export.tsx` (line 34)
- Modify: `ui/frontend/src/routes/_authenticated/tags.tsx` (line 43)
- Modify: `ui/frontend/src/routes/_authenticated/profile.tsx` (line 26)

- [ ] **Step 1: Add `head` to import-export route**

In `ui/frontend/src/routes/_authenticated/import-export.tsx`, the route definition at line 34 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/import-export')({
  component: ImportExportPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/import-export')({
  head: () => ({ meta: [{ title: 'Import & Export | Nexorious' }] }),
  component: ImportExportPage,
});
```

- [ ] **Step 2: Add `head` to tags route**

In `ui/frontend/src/routes/_authenticated/tags.tsx`, the route definition at line 43 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/tags')({
  component: TagsPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/tags')({
  head: () => ({ meta: [{ title: 'Tags | Nexorious' }] }),
  component: TagsPage,
});
```

- [ ] **Step 3: Add `head` to profile route**

In `ui/frontend/src/routes/_authenticated/profile.tsx`, the route definition at line 26 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/profile')({
  component: ProfilePage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/profile')({
  head: () => ({ meta: [{ title: 'Profile | Nexorious' }] }),
  component: ProfilePage,
});
```

- [ ] **Step 4: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/import-export.tsx \
        ui/frontend/src/routes/_authenticated/tags.tsx \
        ui/frontend/src/routes/_authenticated/profile.tsx
git commit -m "fix: add page titles to import-export, tags, and profile routes"
```

---

### Task 7: Add titles to admin routes

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/admin/index.tsx` (line 13)
- Modify: `ui/frontend/src/routes/_authenticated/admin/backups.tsx` (line 63)
- Modify: `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx` (line 26)
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/index.tsx` (line 30)
- Modify: `ui/frontend/src/routes/_authenticated/admin/users/new.tsx` (line 15)

- [ ] **Step 1: Add `head` to admin index route**

In `ui/frontend/src/routes/_authenticated/admin/index.tsx`, the route definition at line 13 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/admin/')({
  component: AdminDashboardPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/admin/')({
  head: () => ({ meta: [{ title: 'Admin | Nexorious' }] }),
  component: AdminDashboardPage,
});
```

- [ ] **Step 2: Add `head` to admin backups route**

In `ui/frontend/src/routes/_authenticated/admin/backups.tsx`, the route definition at line 63 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/admin/backups')({
  component: BackupsPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/admin/backups')({
  head: () => ({ meta: [{ title: 'Backups | Nexorious' }] }),
  component: BackupsPage,
});
```

- [ ] **Step 3: Add `head` to admin maintenance route**

In `ui/frontend/src/routes/_authenticated/admin/maintenance.tsx`, the route definition at line 26 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/admin/maintenance')({
  component: MaintenancePage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/admin/maintenance')({
  head: () => ({ meta: [{ title: 'Maintenance | Nexorious' }] }),
  component: MaintenancePage,
});
```

- [ ] **Step 4: Add `head` to admin users index route**

In `ui/frontend/src/routes/_authenticated/admin/users/index.tsx`, the route definition at line 30 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/admin/users/')({
  component: UsersPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/admin/users/')({
  head: () => ({ meta: [{ title: 'Users | Nexorious' }] }),
  component: UsersPage,
});
```

- [ ] **Step 5: Add `head` to admin new user route**

In `ui/frontend/src/routes/_authenticated/admin/users/new.tsx`, the route definition at line 15 currently reads:

```tsx
export const Route = createFileRoute('/_authenticated/admin/users/new')({
  component: CreateUserPage,
});
```

Change it to:

```tsx
export const Route = createFileRoute('/_authenticated/admin/users/new')({
  head: () => ({ meta: [{ title: 'New User | Nexorious' }] }),
  component: CreateUserPage,
});
```

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/admin/index.tsx \
        ui/frontend/src/routes/_authenticated/admin/backups.tsx \
        ui/frontend/src/routes/_authenticated/admin/maintenance.tsx \
        ui/frontend/src/routes/_authenticated/admin/users/index.tsx \
        ui/frontend/src/routes/_authenticated/admin/users/new.tsx
git commit -m "fix: add page titles to admin routes"
```

---

### Task 8: Final build and type verification

- [ ] **Step 1: Run TypeScript check**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 2: Run frontend tests**

```bash
cd ui/frontend && npm run test
```

Expected: all tests pass (no tests check `document.title`; this is a sanity check that nothing regressed).

- [ ] **Step 3: Build the frontend**

```bash
make frontend
```

Expected: build completes with no errors.

- [ ] **Step 4: Open a PR**

```bash
gh pr create \
  --title "fix: set per-route page titles to fix mobile Firefox tab title bug" \
  --body "Closes #697

Mobile Firefox reverts the tab title to the URL on client-side navigation when \`document.title\` is never set programmatically. This PR uses TanStack Router's \`head\` + \`HeadContent\` mechanism to set a descriptive title on every route navigation.

Changes:
- Add \`HeadContent\` and a default \`'Nexorious'\` title to the root route
- Add per-route \`head\` functions to all 17 content routes
- Dynamic storefront title derived from route params (e.g. \"Steam Sync | Nexorious\")"
```
