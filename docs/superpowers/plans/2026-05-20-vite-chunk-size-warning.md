# Vite chunk-size warning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Silence Vite's "chunks larger than 500 kB" build warning by raising `chunkSizeWarningLimit` to 1500 kB, calibrated to the current bundle size with headroom.

**Architecture:** Single-field addition to the existing `build` block in `ui/frontend/vite.config.ts`. No runtime behaviour change, no bundle restructuring, no new files.

**Tech Stack:** Vite 8, TypeScript

**Spec:** [docs/superpowers/specs/2026-05-20-vite-chunk-size-warning-design.md](../specs/2026-05-20-vite-chunk-size-warning-design.md)

---

## File Structure

Only one file is touched:

- **Modify:** `ui/frontend/vite.config.ts` — add `chunkSizeWarningLimit: 1500` inside the existing `build: { outDir: 'dist' }` object.

No new files. No test files (build-config change with no runtime behaviour).

---

## Task 1: Raise the Vite chunk-size warning threshold

**Files:**
- Modify: `ui/frontend/vite.config.ts:26-28`

- [ ] **Step 1: Confirm baseline — the warning currently fires**

Run (from `ui/frontend/`):
```bash
npm run build 2>&1 | tail -15
```

Expected output includes:
```
dist/assets/index-*.js  ~1,320 kB │ gzip: ~385 kB
(!) Some chunks are larger than 500 kB after minification.
```

If the warning is *not* present, the bundle has changed significantly since the spec was written — re-evaluate the threshold before continuing.

- [ ] **Step 2: Edit `ui/frontend/vite.config.ts`**

Current `build` block (lines 26-28):
```ts
  build: {
    outDir: 'dist',
  },
```

Replace with:
```ts
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
  },
```

That is the only change in this file. Do not touch anything else (plugins, server, resolve, etc.).

- [ ] **Step 3: Run the build to verify the warning is gone**

Run (from `ui/frontend/`):
```bash
npm run build 2>&1 | tail -15
```

Expected:
- Exit code 0
- Bundle size line still shows `dist/assets/index-*.js  ~1,320 kB │ gzip: ~385 kB` (within ±10 kB)
- **No** `(!) Some chunks are larger than 500 kB` line
- No new warnings introduced

If the main chunk has grown to >1500 kB, the warning will still fire — in that case, stop and discuss raising the threshold further rather than blindly bumping it.

- [ ] **Step 4: Run the frontend quality gates**

Per `CLAUDE.md` Quality Gates, run from `ui/frontend/`:
```bash
npm run check && npm run knip && npm run test
```

Expected: all three exit 0. None of them should be affected by this change — this is a safety check that we haven't accidentally edited something else.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/vite.config.ts
git commit -m "$(cat <<'EOF'
build(frontend): raise Vite chunkSizeWarningLimit to 1500 kB

Current main chunk is ~1320 kB; the default 500 kB threshold fires on
every build. Calibrate the warning to this app's actual size with
~180 kB headroom for organic growth, keeping the early-warning
function for regressions (e.g. accidental large imports).

See docs/superpowers/specs/2026-05-20-vite-chunk-size-warning-design.md
EOF
)"
```

---

## Self-Review

**Spec coverage:**
- Spec "Design" (add `chunkSizeWarningLimit: 1500`) → Task 1 Step 2 ✓
- Spec "Verification" (`npm run build` clean, bundle size unchanged) → Task 1 Step 3 ✓
- Spec "Non-goals" (no bundle restructuring, no script changes) → Step 2 explicitly limits scope to that one field ✓
- Spec rationale on threshold = 1500 with headroom → captured in commit message ✓

**Placeholder scan:** No TBDs, no "handle edge cases", no "similar to above". All commands and code are concrete. ✓

**Type consistency:** N/A — no types introduced.

No gaps.

---

## Execution

Plan complete and saved to `docs/superpowers/plans/2026-05-20-vite-chunk-size-warning.md`.

This is a single 5-step task. Subagent-driven execution would be overkill; recommend inline execution in the current session.
