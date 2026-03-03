# Design: Migrate Frontend from Next.js to Vite + TanStack Router

**Date:** 2026-03-03
**Status:** Approved

## Motivation

The frontend is a pure client-side SPA — every page uses `"use client"`, zero server features
are in use. Next.js `output: 'export'` fails on dynamic routes (e.g. `games/[id]`) because it
requires `generateStaticParams` at build time. Vite produces a single `index.html` + assets
with no such constraint.

Secondary benefits:
- Eliminate the separate frontend container and Helm service/ingress
- Serve static files directly from the FastAPI backend image
- Eliminate CORS in both development (via Vite proxy) and production (same origin)
- Remove the Next.js middleware deprecation warning

## Scope Summary

- **Routing style:** TanStack Router file-based routing (`@tanstack/router-plugin/vite`)
- **Fonts:** Self-hosted via `@fontsource/geist-sans` and `@fontsource/geist-mono`
- **Production serving:** FastAPI catch-all serves `dist/` (no separate frontend container)
- **Development:** Vite dev server with proxy to FastAPI (HMR, no CORS)
- **Migration strategy:** Phased in-place migration, 3 PRs

## Architecture

### Runtime Topology

**Development:**
```
Browser → Vite dev server (localhost:3000)
            ├── /api/* → proxy → FastAPI (localhost:8000)
            └── /static/* → proxy → FastAPI (localhost:8000)
```

**Production:**
```
Browser → FastAPI (single container)
            ├── /api/* → API routes
            └── /* → dist/index.html (SPA catch-all)
```

### New Dependencies

| Package | Purpose |
|---|---|
| `@tanstack/react-router` | Client-side routing |
| `@tanstack/router-plugin` | Vite plugin for file-based route code generation |
| `@fontsource/geist-sans` | Self-hosted Geist Sans font |
| `@fontsource/geist-mono` | Self-hosted Geist Mono font |
| `vite` | Build tool and dev server |
| `@vitejs/plugin-react` | Already present (devDependency) |

### Removed Dependencies

| Package | Reason |
|---|---|
| `next` | Replaced by Vite |
| `eslint-config-next` | Replaced by standard ESLint config |

### Kept Dependencies (No Changes)

`next-themes` v0.4+ is provider-agnostic and works in Vite without modification. All other
dependencies (TanStack Query, shadcn/ui, Radix UI, React Hook Form, Zod, Vitest, etc.) are
unchanged.

## Routing Structure

```
src/routes/
  __root.tsx              # Root layout: QueryProvider, AuthProvider, ThemeProvider, Toaster
  index.tsx               # / → redirect to /dashboard
  login.tsx               # /login
  setup.tsx               # /setup
  _authenticated.tsx      # Pathless layout route: auth guard (replaces RouteGuard component)
  _authenticated/
    dashboard.tsx         # /dashboard
    games/
      index.tsx           # /games
      add.tsx             # /games/add
      add.confirm.tsx     # /games/add/confirm
      $id.tsx             # /games/$id
      $id.edit.tsx        # /games/$id/edit
    sync/
      index.tsx           # /sync
      $platform.tsx       # /sync/$platform
    jobs/
      index.tsx           # /jobs
      $id.tsx             # /jobs/$id
    admin/
      index.tsx           # /admin
      users/
        index.tsx         # /admin/users
        new.tsx           # /admin/users/new
        $id.tsx           # /admin/users/$id
      platforms.tsx       # /admin/platforms
      maintenance.tsx     # /admin/maintenance
      backups.tsx         # /admin/backups
    review.tsx            # /review
    import-export.tsx     # /import-export
    profile.tsx           # /profile
    tags.tsx              # /tags
```

### Next.js → TanStack Router API Mapping

| Next.js | TanStack Router |
|---|---|
| `useRouter().push('/path')` | `const navigate = useNavigate(); navigate({ to: '/path' })` |
| `useRouter().replace('/path')` | `navigate({ to: '/path', replace: true })` |
| `usePathname()` | `useRouterState({ select: s => s.location.pathname })` |
| `useSearchParams()` | `useSearch({ from: Route.fullPath })` |
| `useParams()` | `useParams({ from: Route.fullPath })` |
| `<Link href="/path">` | `<Link to="/path">` |
| `<Image src=... />` | `<img src=... loading="lazy" />` |
| `router.back()` | `window.history.back()` |

## Environment Variables

All `NEXT_PUBLIC_*` variables become `VITE_*`. In practice, defaults in `lib/env.ts` cover both
dev and prod so explicit env vars are rarely needed:

```typescript
// lib/env.ts (updated)
export const config = {
  apiUrl: import.meta.env.VITE_API_URL ?? '/api',
  staticUrl: import.meta.env.VITE_STATIC_URL ?? '',
  appName: import.meta.env.VITE_APP_NAME ?? 'Nexorious',
  appVersion: import.meta.env.VITE_APP_VERSION ?? '1.0.0',
} as const;
```

`import.meta.env.DEV` replaces `process.env.NODE_ENV === 'development'`.

## Vite Configuration

```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { TanStackRouterVite } from '@tanstack/router-plugin/vite';
import path from 'path';

export default defineConfig({
  plugins: [TanStackRouterVite(), react()],
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

## Deployment Changes

### FastAPI catch-all (backend/app/main.py)

After all API routers are registered, mount the SPA:

```python
from fastapi.staticfiles import StaticFiles
import os

dist_dir = os.path.join(os.path.dirname(__file__), '..', 'dist')
if os.path.isdir(dist_dir):
    app.mount("/", StaticFiles(directory=dist_dir, html=True), name="spa")
```

The `html=True` flag returns `index.html` for any unrecognised path.

### Backend Dockerfile (multi-stage)

```dockerfile
# Stage 1: Build frontend
FROM node:22-alpine AS frontend-build
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Backend (existing)
FROM python:3.13-slim AS backend
# ... existing backend build steps ...
COPY --from=frontend-build /frontend/dist /app/dist
```

### docker-compose (dev)

Frontend service updated:
- Remove `INTERNAL_API_URL` (server-side middleware env var, no longer needed)
- Replace `NEXT_PUBLIC_*` with `VITE_*` (or omit — defaults in `lib/env.ts` work via proxy)
- Remove `/app/.next` volume mount
- Command remains `npm run dev` (now runs Vite instead of Next.js)

### Helm Chart

Per IDEAS.md, remove frontend `controller`, `service`, and `ingress` entries from `values.yaml`.
The backend image now bundles the compiled static files.

## Migration Phases

### Phase 1 — Bootstrap Vite + TanStack Router
**Branch:** `feat/vite-bootstrap`

- Install Vite, `@tanstack/react-router`, `@tanstack/router-plugin`, fontsource packages
- Remove `next`, `eslint-config-next`
- Create `vite.config.ts`, `index.html`, `src/main.tsx`
- Create the `src/routes/` tree skeleton (stub components, no content yet)
- Update `tsconfig.json` (remove Next.js paths, add Vite types)
- Update `package.json` scripts (`dev`, `build`, `check`)
- Update `eslint.config.mjs` (remove `eslint-config-next`)
- Update `lib/env.ts` (swap `NEXT_PUBLIC_` → `import.meta.env.VITE_`)
- Verify `npm run build` produces a `dist/` directory
- Verify `npm run dev` starts Vite dev server

### Phase 2 — Migrate Components and Routes
**Branch:** `feat/vite-migrate-routes`

- Replace all `next/navigation` imports with TanStack Router equivalents (~60 files)
- Replace all `next/link` imports with TanStack Router `<Link>`
- Replace all `next/image` with `<img loading="lazy">`
- Delete `middleware.ts`
- Migrate `AuthProvider`: replace `useRouter` with `useNavigate`
- Migrate `RouteGuard`: convert to `_authenticated` layout route
- Populate all route files with their page components
- Update all tests that mock `next/navigation` or `next/router`
- Run `npm run check` and `npm run test` — all must pass

### Phase 3 — Remove Next.js, Update Deployment
**Branch:** `feat/vite-deployment`

- Update `backend/Dockerfile` to multi-stage build
- Add FastAPI SPA catch-all in `backend/app/main.py`
- Update `docker-compose.yml` (remove `INTERNAL_API_URL`, update frontend env vars)
- Update Helm `values.yaml` (remove frontend controller/service/ingress)
- Update `CLAUDE.md` quick reference (commands and URLs)
- Update frontend `Dockerfile` (runs Vite dev server)
- Verify end-to-end: `npm run build` → FastAPI serves `dist/` → SPA works

## Testing Strategy

- Vitest configuration remains unchanged (already uses `@vitejs/plugin-react` internally)
- Tests that mock `next/navigation` (useRouter, usePathname, etc.) are updated to mock
  TanStack Router equivalents
- The `src/test/setup.ts` file removes Next.js mock imports
- Coverage thresholds remain: >70% frontend

## Out of Scope

- No changes to backend API or database schema
- No changes to shadcn/ui component code (only their consumers)
- No performance tuning or bundle optimisation beyond what Vite provides by default
- `next-themes` kept as-is (works with Vite)
