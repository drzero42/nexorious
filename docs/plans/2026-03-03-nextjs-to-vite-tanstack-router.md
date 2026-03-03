# Next.js → Vite + TanStack Router Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace Next.js with Vite + TanStack Router (file-based), serving the built SPA from FastAPI in production.

**Architecture:** Three-phase migration within the existing `frontend/` directory. Phase 1 bootstraps Vite alongside Next.js (both runnable). Phase 2 migrates all page content and components. Phase 3 removes Next.js and updates deployment.

**Tech Stack:** Vite 6, @tanstack/react-router v1, @tanstack/router-plugin (file-based routes), @fontsource/geist-sans + geist-mono, FastAPI StaticFiles catch-all for production SPA serving.

**Design doc:** `docs/plans/2026-03-03-nextjs-to-vite-tanstack-router-design.md`

---

## Phase 1 — Bootstrap Vite + TanStack Router

> After this phase `npm run build` produces `dist/` and `npm run dev` starts the Vite server. The Next.js `app/` directory still exists but is ignored by the new entry point.

### Task 1: Install and remove packages

**Files:**
- Modify: `frontend/package.json`

**Step 1: Install new packages**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm install @tanstack/react-router@^1
npm install --save-dev @tanstack/router-plugin vite
npm install @fontsource/geist-sans @fontsource/geist-mono
```

**Step 2: Remove Next.js packages**

```bash
npm uninstall next eslint-config-next
```

**Step 3: Verify package.json no longer contains `"next"` as a dependency**

```bash
grep '"next"' package.json
```
Expected: no output (or only `@tanstack/react-router` etc.)

---

### Task 2: Create `vite.config.ts`

**Files:**
- Create: `frontend/vite.config.ts`

**Step 1: Create the file**

```typescript
// frontend/vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { TanStackRouterVite } from '@tanstack/router-plugin/vite';
import path from 'path';

export default defineConfig({
  plugins: [
    TanStackRouterVite({ routesDirectory: './src/routes' }),
    react(),
  ],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8000',
      '/static': 'http://localhost:8000',
    },
  },
  build: {
    outDir: 'dist',
  },
});
```

---

### Task 3: Create `index.html`

**Files:**
- Create: `frontend/index.html`

**Step 1: Create the file**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <link rel="icon" type="image/x-icon" href="/favicon.ico" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Nexorious</title>
    <meta name="description" content="Game collection management" />
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

---

### Task 4: Update `tsconfig.json`

**Files:**
- Modify: `frontend/tsconfig.json`

**Step 1: Replace with Vite-compatible config**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": true,
    "skipLibCheck": true,
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "react-jsx",
    "paths": {
      "@/*": ["./src/*"]
    }
  },
  "include": ["src", "vite.config.ts"],
  "exclude": ["node_modules", "dist"]
}
```

Key changes: removed `"incremental"`, removed `plugins: [{name: "next"}]`, removed `.next/**` from `include`, added `vite.config.ts` to `include`.

---

### Task 5: Update `package.json` scripts

**Files:**
- Modify: `frontend/package.json`

**Step 1: Replace the scripts block**

Change the `"scripts"` section to:

```json
"scripts": {
  "dev": "vite",
  "build": "tsc --noEmit && vite build",
  "preview": "vite preview",
  "lint": "eslint .",
  "check": "tsc --noEmit && eslint .",
  "test": "vitest run",
  "test:watch": "vitest",
  "test:coverage": "vitest run --coverage",
  "test:ui": "vitest --ui"
},
```

---

### Task 6: Update `eslint.config.mjs`

**Files:**
- Modify: `frontend/eslint.config.mjs`

**Step 1: Replace with a Next.js-free ESLint config**

```javascript
import { defineConfig, globalIgnores } from 'eslint/config';
import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

export default defineConfig([
  globalIgnores(['dist/**', 'coverage/**']),
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
]);
```

**Step 2: Install the new ESLint packages**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm install --save-dev @eslint/js typescript-eslint eslint-plugin-react-hooks eslint-plugin-react-refresh
```

---

### Task 7: Update `src/lib/env.ts`

**Files:**
- Modify: `frontend/src/lib/env.ts`

**Step 1: Replace the file**

```typescript
// frontend/src/lib/env.ts
export const config = {
  apiUrl: import.meta.env.VITE_API_URL ?? '/api',
  staticUrl: import.meta.env.VITE_STATIC_URL ?? '',
  appName: import.meta.env.VITE_APP_NAME ?? 'Nexorious',
  appVersion: import.meta.env.VITE_APP_VERSION ?? '1.0.0',
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
} as const;
```

**Step 2: Add Vite env type declaration**

Create `frontend/src/vite-env.d.ts`:

```typescript
/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL?: string;
  readonly VITE_STATIC_URL?: string;
  readonly VITE_APP_NAME?: string;
  readonly VITE_APP_VERSION?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
```

---

### Task 8: Update `vitest.config.ts`

**Files:**
- Modify: `frontend/vitest.config.ts`

**Step 1: Change the `exclude` in coverage config**

Replace `.next` with `dist`:

```typescript
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["node_modules", "dist"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      reportsDirectory: "./coverage",
      include: ["src/**/*.{ts,tsx}"],
      exclude: [
        "src/**/*.test.{ts,tsx}",
        "src/test/**",
        "src/**/*.d.ts",
        "src/components/ui/**",
        "src/routeTree.gen.ts",
      ],
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
```

Note: `src/routeTree.gen.ts` is auto-generated by the TanStack Router plugin — exclude it from coverage.

---

### Task 9: Create `src/main.tsx`

**Files:**
- Create: `frontend/src/main.tsx`

**Step 1: Create the file**

```typescript
// frontend/src/main.tsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider, createRouter } from '@tanstack/react-router';
import { routeTree } from './routeTree.gen';

const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>
);
```

Note: `routeTree.gen.ts` is auto-generated by `@tanstack/router-plugin` when you run `vite build` or `vite dev`. It will appear after the first build.

---

### Task 10: Create `src/routes/__root.tsx`

**Files:**
- Create: `frontend/src/routes/__root.tsx`

**Step 1: Create the file**

```typescript
// frontend/src/routes/__root.tsx
import { createRootRoute, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from 'next-themes';
import { Toaster } from '@/components/ui/sonner';
import { QueryProvider, AuthProvider } from '@/providers';
import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/700.css';
import '@fontsource/geist-mono/400.css';
import '@/app/globals.css';

export const Route = createRootRoute({
  component: RootComponent,
});

function RootComponent() {
  return (
    <QueryProvider>
      <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
        <AuthProvider>
          <Outlet />
          <Toaster />
        </AuthProvider>
      </ThemeProvider>
    </QueryProvider>
  );
}
```

Note: `globals.css` is still imported from `src/app/globals.css` here. In Phase 2 it will be moved to `src/styles/globals.css`.

---

### Task 11: Create stub route files

**Files:**
- Create all files listed below with stub content

Create each file with the minimal stub shown. The full content is wired in Phase 2.

**`frontend/src/routes/index.tsx`** — redirects to `/dashboard`:
```typescript
import { createFileRoute, redirect } from '@tanstack/react-router';

export const Route = createFileRoute('/')({
  beforeLoad: () => {
    throw redirect({ to: '/dashboard' });
  },
  component: () => null,
});
```

**`frontend/src/routes/login.tsx`** — stub:
```typescript
import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/login')({
  component: () => <div>Login (migrating...)</div>,
});
```

**`frontend/src/routes/setup.tsx`** — stub:
```typescript
import { createFileRoute } from '@tanstack/react-router';

export const Route = createFileRoute('/setup')({
  component: () => <div>Setup (migrating...)</div>,
});
```

**`frontend/src/routes/_authenticated.tsx`** — pathless auth layout:
```typescript
import { createFileRoute, Outlet } from '@tanstack/react-router';

export const Route = createFileRoute('/_authenticated')({
  component: () => <Outlet />,
});
```

Create stub files for each of the following routes (use the same pattern as `login.tsx` above, adjusting `createFileRoute` path):

- `frontend/src/routes/_authenticated/dashboard.tsx` → path `'/_authenticated/dashboard'`
- `frontend/src/routes/_authenticated/games/index.tsx` → path `'/_authenticated/games/'`
- `frontend/src/routes/_authenticated/games/add.tsx` → path `'/_authenticated/games/add'`
- `frontend/src/routes/_authenticated/games/add.confirm.tsx` → path `'/_authenticated/games/add/confirm'`
- `frontend/src/routes/_authenticated/games/$id.tsx` → path `'/_authenticated/games/$id'`
- `frontend/src/routes/_authenticated/games/$id.edit.tsx` → path `'/_authenticated/games/$id/edit'`
- `frontend/src/routes/_authenticated/sync/index.tsx` → path `'/_authenticated/sync/'`
- `frontend/src/routes/_authenticated/sync/$platform.tsx` → path `'/_authenticated/sync/$platform'`
- `frontend/src/routes/_authenticated/jobs/index.tsx` → path `'/_authenticated/jobs/'`
- `frontend/src/routes/_authenticated/jobs/$id.tsx` → path `'/_authenticated/jobs/$id'`
- `frontend/src/routes/_authenticated/admin/index.tsx` → path `'/_authenticated/admin/'`
- `frontend/src/routes/_authenticated/admin/users/index.tsx` → path `'/_authenticated/admin/users/'`
- `frontend/src/routes/_authenticated/admin/users/new.tsx` → path `'/_authenticated/admin/users/new'`
- `frontend/src/routes/_authenticated/admin/users/$id.tsx` → path `'/_authenticated/admin/users/$id'`
- `frontend/src/routes/_authenticated/admin/platforms.tsx` → path `'/_authenticated/admin/platforms'`
- `frontend/src/routes/_authenticated/admin/maintenance.tsx` → path `'/_authenticated/admin/maintenance'`
- `frontend/src/routes/_authenticated/admin/backups.tsx` → path `'/_authenticated/admin/backups'`
- `frontend/src/routes/_authenticated/review.tsx` → path `'/_authenticated/review'`
- `frontend/src/routes/_authenticated/import-export.tsx` → path `'/_authenticated/import-export'`
- `frontend/src/routes/_authenticated/profile.tsx` → path `'/_authenticated/profile'`
- `frontend/src/routes/_authenticated/tags.tsx` → path `'/_authenticated/tags'`

---

### Task 12: Verify the build

**Step 1: Run `vite build` to generate the route tree and produce `dist/`**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run build
```

Expected: `dist/` directory created, `src/routeTree.gen.ts` generated. If there are TypeScript errors about missing modules (e.g. `@/app/globals.css`), they are expected and will be fixed in Phase 2.

**Step 2: Verify `dist/index.html` exists**

```bash
ls dist/index.html
```

Expected: file present.

**Step 3: Add `routeTree.gen.ts` to `.gitignore`**

Check if `frontend/.gitignore` exists. Add this line:
```
src/routeTree.gen.ts
```

---

### Task 13: Commit Phase 1

```bash
cd /home/abo/workspace/home/nexorious/frontend
git add -A
git commit -m "feat: bootstrap Vite + TanStack Router alongside Next.js"
```

---

## Phase 2 — Migrate Components and Routes

> This phase migrates all content from the Next.js `app/` directory into TanStack Router routes, and replaces all `next/*` imports with TanStack Router + plain HTML equivalents. At the end, `app/` and `middleware.ts` are deleted.

### Task 14: Update test setup to remove Next.js mocks

**Files:**
- Modify: `frontend/src/test/setup.ts`

**Step 1: Remove `vi.mock("next/navigation", ...)` and `vi.mock("next/image", ...)` blocks**

Replace the existing `next/navigation` mock (lines 23–36) and `next/image` mock (lines 38–58) in `src/test/setup.ts` with a TanStack Router mock:

```typescript
// Replace the next/navigation and next/image mock blocks with:

// Mock @tanstack/react-router for tests
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    useParams: () => ({}),
    useSearch: () => ({}),
    useRouterState: vi.fn((opts?: { select?: (s: unknown) => unknown }) => {
      const state = { location: { pathname: '/', search: '', hash: '' } };
      return opts?.select ? opts.select(state) : state;
    }),
    Link: ({ children, to, ...props }: { children: React.ReactNode; to: string; [key: string]: unknown }) => {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const React = require('react');
      return React.createElement('a', { href: to, ...props }, children);
    },
  };
});
```

Add the React import at the top of setup.ts if not already present (it's already importing from vitest).

**Step 2: Run tests to see the baseline failure count**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm test 2>&1 | tail -20
```

This will fail for tests that still import from `next/navigation` or `next/link` directly. That's expected — we'll fix those in subsequent tasks.

---

### Task 15: Migrate `src/providers/auth-provider.tsx`

**Files:**
- Modify: `frontend/src/providers/auth-provider.tsx`

**Step 1: Replace `useRouter` with `useNavigate`**

Change:
```typescript
import { useRouter } from 'next/navigation';
```
To:
```typescript
import { useNavigate } from '@tanstack/react-router';
```

Change:
```typescript
const router = useRouter();
```
To:
```typescript
const navigate = useNavigate();
```

Change (in the `logout` callback):
```typescript
router.push('/login');
```
To:
```typescript
navigate({ to: '/login' });
```

Remove the `'use client';` directive at the top of the file (Vite doesn't use this).

**Step 2: Run auth provider tests**

```bash
npm test auth-provider
```

Expected: pass.

---

### Task 16: Migrate `src/components/route-guard.tsx`

**Files:**
- Modify: `frontend/src/components/route-guard.tsx`

**Step 1: Replace `useRouter` with `useNavigate`**

Change:
```typescript
import { useRouter } from 'next/navigation';
```
To:
```typescript
import { useNavigate } from '@tanstack/react-router';
```

Change:
```typescript
const router = useRouter();
```
To:
```typescript
const navigate = useNavigate();
```

Change:
```typescript
router.replace('/setup');
```
To:
```typescript
navigate({ to: '/setup', replace: true });
```

Change:
```typescript
router.replace('/login');
```
To:
```typescript
navigate({ to: '/login', replace: true });
```

Remove `'use client';` directive.

**Step 2: Run route guard tests**

```bash
npm test route-guard
```

Expected: pass.

---

### Task 17: Migrate `src/components/navigation/nav-link.tsx`

**Files:**
- Modify: `frontend/src/components/navigation/nav-link.tsx`

**Step 1: Replace `next/link` and `next/navigation`**

Change imports from:
```typescript
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
```
To:
```typescript
import { Link, useNavigate, useRouterState } from '@tanstack/react-router';
```

Change:
```typescript
const pathname = usePathname();
const router = useRouter();
```
To:
```typescript
const pathname = useRouterState({ select: (s) => s.location.pathname });
const navigate = useNavigate();
```

Change:
```typescript
router.push(badgeHref);
```
To:
```typescript
navigate({ to: badgeHref });
```

Change:
```typescript
<Link href={href} ...>
```
To:
```typescript
<Link to={href} ...>
```

Remove `'use client';` directive.

**Step 2: Run navigation tests**

```bash
npm test nav-link nav-section
```

Expected: pass.

---

### Task 18: Migrate `src/components/navigation/sidebar.tsx` and `mobile-nav.tsx`

**Files:**
- Modify: `frontend/src/components/navigation/sidebar.tsx`
- Modify: `frontend/src/components/navigation/mobile-nav.tsx`

**Step 1: In both files, replace `next/navigation` imports**

For `sidebar.tsx` — check what it imports. If it uses `useRouter` or `usePathname`, replace with TanStack Router equivalents (same pattern as nav-link.tsx above). If it only uses `Link`, replace `import Link from 'next/link'` with `import { Link } from '@tanstack/react-router'` and `href=` with `to=`.

For `mobile-nav.tsx` — same pattern.

Remove `'use client';` directives.

**Step 2: Run navigation tests**

```bash
npm test sidebar mobile-nav
```

Expected: pass (or no tests exist for these files — that's fine).

---

### Task 19: Migrate `src/components/ui/platform-icon.tsx`

**Files:**
- Modify: `frontend/src/components/ui/platform-icon.tsx`

**Step 1: Replace `next/image` with `<img>`**

Change:
```typescript
import Image from 'next/image';
```
Remove this import entirely.

Find the `<Image ...>` usage and replace with:
```typescript
<img
  src={...}
  alt={...}
  loading="lazy"
  className={...}
  width={...}
  height={...}
/>
```

Remove `'use client';` directive.

---

### Task 20: Migrate game components

**Files:**
- Modify: `frontend/src/components/games/game-card.tsx`
- Modify: `frontend/src/components/games/game-list.tsx`
- Modify: `frontend/src/components/games/game-edit-form.tsx`
- Modify: `frontend/src/components/games/igdb-search.tsx`

For each file:
- Replace `import Image from 'next/image'` → remove, use `<img loading="lazy">` instead
- Replace `import Link from 'next/link'` → `import { Link } from '@tanstack/react-router'`
- Replace `import { useRouter } from 'next/navigation'` → `import { useNavigate } from '@tanstack/react-router'`
- Replace `href=` on `<Link>` → `to=`
- Replace `router.push(...)` → `navigate({ to: ... })`
- Replace `router.replace(...)` → `navigate({ to: ..., replace: true })`
- Remove all `'use client';` directives

**Step 2: Run game component tests**

```bash
npm test game-card game-list game-edit-form
```

Expected: pass.

---

### Task 21: Migrate remaining components

**Files:**
- Modify: `frontend/src/components/sync/recent-activity.tsx`
- Modify: `frontend/src/components/sync/sync-service-card.tsx`
- Modify: `frontend/src/components/dashboard/CurrentlyPlayingSection.tsx`
- Modify: `frontend/src/components/jobs/job-card.tsx`
- Modify: `frontend/src/components/jobs/job-items-details.tsx`

For each file, apply the same pattern:
- Replace `next/link` → TanStack Router `Link` with `to=` instead of `href=`
- Replace `next/navigation` hooks → TanStack Router equivalents
- Replace `next/image` → `<img loading="lazy">`
- Remove `'use client';`

**Step 2: Run their tests**

```bash
npm test sync-service-card CurrentlyPlayingSection job-card
```

Expected: pass.

---

### Task 22: Update `__root.tsx` to use moved globals.css

**Files:**
- Create: `frontend/src/styles/globals.css` (move from `src/app/globals.css`)
- Modify: `frontend/src/routes/__root.tsx`

**Step 1: Move globals.css**

```bash
mv /home/abo/workspace/home/nexorious/frontend/src/app/globals.css \
   /home/abo/workspace/home/nexorious/frontend/src/styles/globals.css
```

**Step 2: Update the import in `__root.tsx`**

Change:
```typescript
import '@/app/globals.css';
```
To:
```typescript
import '@/styles/globals.css';
```

---

### Task 23: Wire up `_authenticated.tsx` with the layout

**Files:**
- Modify: `frontend/src/routes/_authenticated.tsx`

**Step 1: Replace stub with full layout**

```typescript
// frontend/src/routes/_authenticated.tsx
import { createFileRoute, Outlet } from '@tanstack/react-router';
import { RouteGuard } from '@/components/route-guard';
import { Sidebar, MobileNav } from '@/components/navigation';

export const Route = createFileRoute('/_authenticated')({
  component: AuthenticatedLayout,
});

function AuthenticatedLayout() {
  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        <MobileNav />
        <Sidebar />
        <main className="flex-1 p-6 overflow-auto md:ml-64">
          <Outlet />
        </main>
      </div>
    </RouteGuard>
  );
}
```

---

### Task 24: Migrate auth page routes (login and setup)

**Files:**
- Modify: `frontend/src/routes/login.tsx`
- Modify: `frontend/src/routes/setup.tsx`
- Source: `frontend/src/app/(auth)/login/page.tsx`
- Source: `frontend/src/app/(auth)/setup/page.tsx`

**Step 1: For `login.tsx`**

Read `src/app/(auth)/login/page.tsx`. Copy the component function into `src/routes/login.tsx`:

```typescript
// frontend/src/routes/login.tsx
import { createFileRoute } from '@tanstack/react-router';
// [copy all imports from src/app/(auth)/login/page.tsx except next/* imports]
// Replace 'next/navigation' imports with '@tanstack/react-router' equivalents

export const Route = createFileRoute('/login')({
  component: LoginPage,
});

// [paste the component function here, renamed to LoginPage]
// Replace useRouter() with useNavigate()
// Replace router.push/replace with navigate({to: ...})
```

**Step 2: For `setup.tsx`**

Same pattern — copy from `src/app/(auth)/setup/page.tsx`.

**Step 3: Run auth page tests**

```bash
npm test login setup
```

Expected: pass (update test mocks if they still reference `next/navigation` directly).

---

### Task 25: Migrate dashboard, games, and games/add routes

**Files:**
- Modify: `frontend/src/routes/_authenticated/dashboard.tsx`
- Modify: `frontend/src/routes/_authenticated/games/index.tsx`
- Modify: `frontend/src/routes/_authenticated/games/add.tsx`
- Modify: `frontend/src/routes/_authenticated/games/add.confirm.tsx`
- Sources: corresponding `src/app/(main)/` page files

**Step 1: For each route file, copy the component from the corresponding `app/` page**

Pattern:
```typescript
import { createFileRoute } from '@tanstack/react-router';
// [imports from original page.tsx, with next/* replaced]

export const Route = createFileRoute('/_authenticated/dashboard')({
  component: DashboardPage,
});

// [paste component, renamed]
```

Replace any `useRouter`, `usePathname`, `useParams`, `useSearchParams` from `next/navigation` with:
- `useNavigate` (for `useRouter`)
- `useRouterState({ select: s => s.location.pathname })` (for `usePathname`)
- `useParams({ from: Route.fullPath })` (for `useParams`)
- `useSearch({ from: Route.fullPath })` (for `useSearchParams`)

**Step 2: Run tests**

```bash
npm test dashboard games/page
```

---

### Task 26: Migrate games detail and edit routes

**Files:**
- Modify: `frontend/src/routes/_authenticated/games/$id.tsx`
- Modify: `frontend/src/routes/_authenticated/games/$id.edit.tsx`
- Sources: `src/app/(main)/games/[id]/page.tsx`, `src/app/(main)/games/[id]/edit/page.tsx`

**Step 1: Migrate `$id.tsx`**

The `[id]` param becomes `$id`. Access it with:
```typescript
const { id } = Route.useParams();
```

Replace `notFound()` from `next/navigation` with a conditional render:
```typescript
// Instead of notFound(), render a not-found UI:
if (!data) return <div>Game not found</div>;
```

**Step 2: Run tests**

```bash
npm test "games/\[id\]"
```

---

### Task 27: Migrate sync, jobs, admin, and remaining routes

**Files:**
- Modify all remaining stub route files in `src/routes/_authenticated/`
- Sources: corresponding `src/app/(main)/` page files

Apply the same migration pattern for:
- `sync/index.tsx` ← `app/(main)/sync/page.tsx`
- `sync/$platform.tsx` ← `app/(main)/sync/[platform]/page.tsx` (replace `notFound()` with inline not-found render)
- `jobs/index.tsx` ← `app/(main)/jobs/page.tsx`
- `jobs/$id.tsx` ← `app/(main)/jobs/[id]/page.tsx`
- `admin/index.tsx` ← `app/(main)/admin/page.tsx`
- `admin/users/index.tsx` ← `app/(main)/admin/users/page.tsx`
- `admin/users/new.tsx` ← `app/(main)/admin/users/new/page.tsx`
- `admin/users/$id.tsx` ← `app/(main)/admin/users/[id]/page.tsx`
- `admin/platforms.tsx` ← `app/(main)/admin/platforms/page.tsx`
- `admin/maintenance.tsx` ← `app/(main)/admin/maintenance/page.tsx`
- `admin/backups.tsx` ← `app/(main)/admin/backups/page.tsx`
- `review.tsx` ← `app/(main)/review/page.tsx`
- `import-export.tsx` ← `app/(main)/import-export/page.tsx`
- `profile.tsx` ← `app/(main)/profile/page.tsx`
- `tags.tsx` ← `app/(main)/tags/page.tsx`

For `sync/$platform.tsx`, the `[platform]` param becomes `$platform`. Replace `notFound()` with:
```typescript
if (!SUPPORTED_SYNC_PLATFORMS.includes(platform as SyncPlatform)) {
  return <div className="p-6">Platform not found</div>;
}
```

**Step 2: Run all tests**

```bash
npm test
```

Fix any remaining test failures that reference `next/navigation` or `next/link` mocks.

---

### Task 28: Update individual test files that mock `next/navigation` directly

**Files:**
- All `*.test.tsx` files that contain `vi.mock('next/navigation', ...)` or `vi.mock('next/link', ...)`

**Step 1: Find files with per-file next/* mocks**

```bash
grep -rl "vi.mock.*next/" /home/abo/workspace/home/nexorious/frontend/src --include="*.test.tsx" --include="*.test.ts"
```

**Step 2: Remove the per-file mocks**

For each file found, remove the entire `vi.mock('next/navigation', ...)` and `vi.mock('next/link', ...)` blocks. The global mock in `src/test/setup.ts` (added in Task 14) now handles this.

**Step 3: Run all tests**

```bash
npm run test
```

Expected: all pass.

---

### Task 29: Delete Next.js artifacts

**Files:**
- Delete: `frontend/src/app/` (entire directory)
- Delete: `frontend/src/middleware.ts`
- Delete: `frontend/next-env.d.ts`
- Delete: `frontend/next.config.ts`
- Delete: `frontend/postcss.config.mjs` (only if Tailwind v4 uses vite plugin instead; check first)

**Step 1: Check if `postcss.config.mjs` is needed by Vite**

Tailwind CSS v4 uses the `@tailwindcss/vite` plugin, not PostCSS, when used with Vite. Check `globals.css` — if it uses `@import "tailwindcss"`, then PostCSS config is no longer needed. If it uses `@tailwind base; @tailwind components; @tailwind utilities;`, PostCSS is still needed.

```bash
head -5 /home/abo/workspace/home/nexorious/frontend/src/styles/globals.css
```

If the CSS uses `@import "tailwindcss"` (Tailwind v4 style): install `@tailwindcss/vite` and add it to `vite.config.ts`, then delete `postcss.config.mjs`. If it uses `@tailwind` directives (v3 style): keep `postcss.config.mjs`.

**Step 2: Delete Next.js files**

```bash
rm -rf /home/abo/workspace/home/nexorious/frontend/src/app
rm /home/abo/workspace/home/nexorious/frontend/src/middleware.ts
rm /home/abo/workspace/home/nexorious/frontend/next-env.d.ts
rm /home/abo/workspace/home/nexorious/frontend/next.config.ts
```

**Step 3: Run the full check**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
npm run test
```

Expected: zero TypeScript errors, all tests pass.

**Step 4: Run build**

```bash
npm run build
```

Expected: clean build, `dist/` produced.

---

### Task 30: Commit Phase 2

```bash
cd /home/abo/workspace/home/nexorious/frontend
git add -A
git commit -m "feat: migrate all routes and components from Next.js to Vite + TanStack Router"
```

---

## Phase 3 — Remove Next.js, Update Deployment

> Wire the built `dist/` into FastAPI, update docker-compose and Helm chart, update the backend Dockerfile to multi-stage, and clean up CLAUDE.md.

### Task 31: Add SPA catch-all to FastAPI

**Files:**
- Modify: `backend/app/main.py`

**Step 1: Remove the root JSON endpoint**

Find and remove:
```python
@app.get("/")
async def root():
    """Root endpoint with basic app information"""
    return {
        "message": f"Welcome to {settings.app_name}",
        ...
    }
```

The SPA's `index.html` will handle `/` from now on.

**Step 2: Add SPA catch-all at the end of the file**

After all `app.mount()` calls for `/static/cover_art` and `/static/logos`, add:

```python
# Mount SPA — must be last, catches all non-API paths
_dist_dir = os.path.join(os.path.dirname(__file__), '..', 'dist')
if os.path.isdir(_dist_dir):
    app.mount("/", StaticFiles(directory=_dist_dir, html=True), name="spa")
```

The guard `if os.path.isdir(_dist_dir)` means the backend still works without the `dist/` directory (e.g. during development without a frontend build).

**Step 3: Run backend tests**

```bash
cd /home/abo/workspace/home/nexorious/backend
uv run pytest app/tests/ -v 2>&1 | tail -20
```

Expected: all pass (backend tests are not affected by frontend changes).

**Step 4: Verify type check**

```bash
uv run pyrefly check
```

Expected: zero errors.

---

### Task 32: Update backend `Dockerfile` to multi-stage build

**Files:**
- Modify: `backend/Dockerfile`

**Step 1: Add the frontend build stage at the top**

Prepend this stage before the existing content:

```dockerfile
# Stage 1: Build frontend
FROM node:22-alpine AS frontend-build
WORKDIR /frontend
COPY ../frontend/package*.json ./
RUN npm ci
COPY ../frontend/ .
RUN npm run build
```

Wait — Docker COPY can't use `../` paths. The Dockerfile is at `backend/Dockerfile` and the frontend is at `frontend/`. The build context must include both. The easiest approach: update the docker-compose `build` context to the repo root and use a single combined Dockerfile at the repo root, OR keep the `backend/Dockerfile` but adjust the build context in docker-compose to `.` (the repo root).

**Recommended approach:** Use repo-root context in docker-compose for the api service, and use a new `Dockerfile` at the repo root:

Create `Dockerfile` at repo root:

```dockerfile
# Stage 1: Build frontend
FROM node:22-alpine AS frontend-build
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Backend (copy from existing backend/Dockerfile)
FROM ghcr.io/astral-sh/uv:python3.13-bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    gnupg curl ca-certificates \
    && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends postgresql-client-16 \
    && rm -rf /var/lib/apt/lists/*

ENV UV_COMPILE_BYTECODE=1
ENV UV_LINK_MODE=copy

COPY backend/pyproject.toml backend/uv.lock ./
RUN uv sync --frozen --no-install-project

COPY backend/ .
RUN uv sync --frozen

# Copy built frontend SPA
COPY --from=frontend-build /frontend/dist ./dist

CMD ["uv", "run", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
```

**Step 2: Update `docker-compose.yml` api build context**

Change the api service `build` from:
```yaml
build: ./backend
```
To:
```yaml
build:
  context: .
  dockerfile: Dockerfile
```

The `backend/Dockerfile` is kept unchanged for use as a dev-only backend container if needed.

**Step 3: Verify docker-compose build**

```bash
podman-compose build api
```

Expected: successful build with the frontend included in the image.

---

### Task 33: Update `docker-compose.yml` frontend service

**Files:**
- Modify: `docker-compose.yml`

**Step 1: Update the frontend service**

Replace the existing `frontend` service with:

```yaml
  frontend:
    build: ./frontend
    ports:
      - "3000:3000"
    environment:
      # Optional: override API URL (defaults to /api via Vite proxy)
      # VITE_API_URL: http://localhost:8000/api
    volumes:
      - ./frontend:/app:Z
      - /app/node_modules
    depends_on:
      - api
```

Remove:
- `NEXT_PUBLIC_API_URL`
- `NEXT_PUBLIC_STATIC_URL`
- `INTERNAL_API_URL`
- `/app/.next` volume mount

**Step 2: Update the `frontend/Dockerfile`**

Replace the comment and CMD to reflect Vite:

```dockerfile
# frontend/Dockerfile
FROM docker.io/node:22-bookworm-slim

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

EXPOSE 3000

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

The `--host 0.0.0.0` flag is required so Vite's dev server is accessible from outside the container.

---

### Task 34: Update Helm `values.yaml`

**Files:**
- Modify: `deploy/helm/values.yaml`

**Step 1: Remove the frontend controller, service, and ingress**

Find and remove (or comment out with a note) any `controllers.frontend`, `service.frontend`, and `ingress.frontend` blocks in `values.yaml`.

Per the design doc, the frontend is now bundled into the backend image and served via FastAPI's `/` catch-all. No separate Kubernetes deployment is needed.

**Step 2: Lint the Helm chart**

```bash
helm lint --strict deploy/helm/ \
  --set nexorious.secretKey=x \
  --set nexorious.internalApiKey=x \
  --set nexorious.postgresql.password=x
```

Expected: no errors or warnings.

---

### Task 35: Update `CLAUDE.md`

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Update frontend commands in the Quick Reference table**

Change:
```markdown
| Start development server | `uv run python -m app.main` | `npm run dev` |
| Build                    |                             | `npm run build` |
```

The commands remain the same (`npm run dev`, `npm run build`) — but update the Important URLs section:

Remove `Frontend Dev: http://localhost:3000` from the URLs table OR keep it to indicate the Vite dev server port.

**Step 2: Update the "Project Structure" section**

Remove references to Next.js App Router conventions (`app/`, `page.tsx`, Next.js-specific concepts). Replace with:

```
- `frontend/` - Vite + React SPA with TanStack Router, Tailwind CSS, shadcn/ui, TanStack Query
  - `src/routes/` - TanStack Router file-based routes
  - `src/components/` - Reusable React components
  - ...
```

---

### Task 36: Final verification

**Step 1: Full backend test suite**

```bash
cd /home/abo/workspace/home/nexorious/backend
uv run pytest --cov=app --cov-report=term-missing 2>&1 | tail -20
```

Expected: all pass, >80% coverage.

**Step 2: Full frontend test suite**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run check
npm run test -- --coverage 2>&1 | tail -20
```

Expected: all pass, >70% coverage, zero TypeScript errors.

**Step 3: Production build**

```bash
cd /home/abo/workspace/home/nexorious/frontend
npm run build
ls dist/
```

Expected: `index.html` and `assets/` present in `dist/`.

**Step 4: Smoke test with FastAPI serving the SPA**

```bash
cd /home/abo/workspace/home/nexorious/backend
# Start the backend with the dist directory available
uv run uvicorn app.main:app --port 8000 &
sleep 2
curl -s http://localhost:8000/ | grep -c "Nexorious"
# Should return 1 (index.html served)
curl -s http://localhost:8000/games | grep -c "Nexorious"
# Should return 1 (SPA catch-all serving index.html)
curl -s http://localhost:8000/api/health | grep -c "healthy"
# Should return 1 (API still working)
kill %1
```

---

### Task 37: Commit Phase 3

```bash
git add -A
git commit -m "feat: remove Next.js, serve SPA from FastAPI, update deployment"
```

---

## Reference: Next.js → TanStack Router Quick Cheat Sheet

| Pattern | Next.js | TanStack Router |
|---|---|---|
| Navigate programmatically | `const r = useRouter(); r.push('/path')` | `const n = useNavigate(); n({ to: '/path' })` |
| Navigate with replace | `r.replace('/path')` | `n({ to: '/path', replace: true })` |
| Current pathname | `usePathname()` | `useRouterState({ select: s => s.location.pathname })` |
| URL params | `useParams().id` | `Route.useParams().id` or `useParams({ from: Route.fullPath }).id` |
| Search params | `useSearchParams().get('q')` | `Route.useSearch().q` |
| Link component | `<Link href="/path">` | `<Link to="/path">` |
| Image | `<Image src=... fill />` | `<img src=... loading="lazy" />` |
| 404 | `notFound()` | Render a not-found component inline |
| Env vars | `process.env.NEXT_PUBLIC_X` | `import.meta.env.VITE_X` |
| Dev/prod flag | `process.env.NODE_ENV === 'development'` | `import.meta.env.DEV` |
