# Vite chunk-size warning — design

## Context

When `make frontend` (or `npm run build` from `ui/frontend/`) runs, Vite emits:

```
(!) Some chunks are larger than 500 kB after minification.
```

Current build output:

| Asset                | Raw       | Gzip     |
|----------------------|-----------|----------|
| `assets/index-*.js`  | 1320 kB   | 385 kB   |
| `assets/index-*.css` | 91 kB     | 27 kB    |

The bundle is monolithic — no route-level lazy loading, no `manualChunks` vendor splitting.

## Goal

Silence the warning without restructuring the bundle.

## Non-goals

- Reducing initial JS payload (385 kB gzipped is acceptable for a self-hosted, Go-binary-embedded SPA on a private network)
- Route-level lazy loading or `manualChunks` configuration
- Any change to dependencies or build pipeline

## Rationale

The warning's default threshold (500 kB) targets public-internet apps where time-to-interactive matters across uncached visitors. This project is self-hosted: users install the binary, open the SPA once per session, and pay the initial-load cost from a local-network connection. Aggressive code-splitting carries refactor cost (touching every route file for TanStack Router's lazy convention) without a meaningful payoff in this deployment model.

We still want the *function* of the warning — flagging unexpected bundle growth — just at a threshold matched to this app's actual size.

## Design

Add `chunkSizeWarningLimit: 1500` to the `build` block in `ui/frontend/vite.config.ts`:

```ts
build: {
  outDir: 'dist',
  chunkSizeWarningLimit: 1500,
},
```

### Threshold choice: 1500 kB

- Current main chunk is 1320 kB → fits with ~180 kB of headroom.
- Headroom absorbs organic growth (new routes, additional shadcn components, dependency updates).
- A regression that pushes the bundle past 1500 kB — e.g. an accidental import that drags in a 1 MB+ dependency — still trips the warning. We keep the early-warning function; we just calibrate it to this app.

## Verification

From `ui/frontend/`:

```bash
npm run build
```

Expected:
- Exit code 0
- No `(!) Some chunks are larger than 500 kB` warning
- Bundle size unchanged (within ±10 kB of current 1320 kB main chunk)

No new tests required — this is a build-config tweak with no runtime behaviour change.

## Out of scope (future work, not part of this change)

If initial load *does* become a concern later, the natural next steps are:

1. Route-level lazy loading via TanStack Router's `.lazy.tsx` file convention (per-route chunks, on-demand load).
2. `manualChunks` for vendor splitting (better cache hit rate across deploys).
3. Audit large dependencies (TipTap, Radix bundle) for tree-shaking opportunities.

None of these are required to address the current warning.
