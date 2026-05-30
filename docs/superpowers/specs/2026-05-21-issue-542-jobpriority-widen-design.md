# Widen JobPriority on the frontend to match backend domain

**Issue:** [#542](https://github.com/drzero42/nexorious/issues/542)
**Date:** 2026-05-21
**Milestone:** 0.1.0

## Problem

After the knip-driven cleanup in #539, the frontend `JobPriority` enum has only one member:

```ts
// ui/frontend/src/types/jobs.ts:45-47
export enum JobPriority {
  HIGH = 'high',
}
```

The backend domain has three values — `internal/db/models/jobs.go:44-46`:

```go
JobPriorityLow    = "low"
JobPriorityNormal = "normal"
JobPriorityHigh   = "high"
```

…and live code writes the non-`high` values: `internal/scheduler/scheduler.go:160` inserts scheduled syncs at `priority='low'`.

The boundary cast in [ui/frontend/src/api/jobs.ts:175](../../ui/frontend/src/api/jobs.ts#L175) is `priority: apiJob.priority as JobPriority`. `as` is unchecked at runtime, so the field flows in with values the compiler claims are impossible. The bug is currently invisible because no React component reads or branches on `job.priority` — `grep -rn '\.priority' ui/frontend/src --include='*.tsx'` returns zero usages. The day a feature adds a priority badge, filter, or sort, the compiler will allow code that assumes priority is always `'high'`, and the bug becomes real.

## Goal

Make the frontend type for `JobPriority` reflect the actual domain (`low` | `normal` | `high`), so future code that reads `job.priority` is type-checked against the real value set.

## Change

Replace the enum with a string-literal union in [ui/frontend/src/types/jobs.ts](../../ui/frontend/src/types/jobs.ts):

```ts
export type JobPriority = 'low' | 'normal' | 'high';
```

The boundary cast in [ui/frontend/src/api/jobs.ts:175](../../ui/frontend/src/api/jobs.ts#L175) stays `priority: apiJob.priority as JobPriority` — same shape as the sibling `jobType`/`source`/`status` casts in the same function.

### Why string union, not enum or const-object

- **No knip carve-out needed.** Knip flags unused enum *members*; it does not track "unused" inside a string union, so all three literals coexist without an `ignoreExports` entry. Knip carve-outs add config friction that string unions sidestep.
- **No runtime overhead.** No emitted JS object for the type, no name-to-value table to maintain.
- **Forces literal usage at call sites.** `JobPriority.HIGH` becomes `'high'`, which is what the JSON wire format already carries — one less mental hop.

### Why no runtime validator

The bug being fixed is *type-level only*: the compiler is wrong about which values `priority` can hold. Widening the union to match the backend resolves that. A runtime validator would defend against backend/frontend drift, which is a different concern, and adding one only on `priority` while leaving sibling `as` casts unchecked would create asymmetric handling for no specific reason. If runtime narrowing later becomes useful, it deserves its own change and should be applied uniformly to all four fields.

### Call-site fallout

Two test files reference `JobPriority.HIGH` and must change to the literal `'high'`:

- [ui/frontend/src/api/jobs.test.ts:70](../../ui/frontend/src/api/jobs.test.ts#L70)
- [ui/frontend/src/components/jobs/job-card.test.tsx:15](../../ui/frontend/src/components/jobs/job-card.test.tsx#L15)

After the change, `JobPriority` is type-only. Existing imports that bring in `JobPriority` alongside enum values (e.g. `JobType`) need no structural change — `JobPriority` just narrows to a type-only binding within the same `import { ... }` line. If the project's tsconfig enables `verbatimModuleSyntax`, the implementer should mark it inline as `import { type JobPriority, ... }` or move it to a sibling `import type` line; otherwise no syntactic change is required.

The barrel re-export from [ui/frontend/src/types/index.ts](../../ui/frontend/src/types/index.ts) uses `export *`, which forwards type aliases the same way it forwarded the enum type — no change required there.

No production `.tsx` reads `.priority`, so no component code changes.

### Tests

No new tests. The bug is type-level: TypeScript's check is the verification. The existing `transformJob` test in `api/jobs.test.ts` continues to cover the happy path; only the literal value in the assertion changes.

## Out of scope

- Adding runtime validation to `transformJob` for any of the four boundary casts.
- Adding a knip carve-out (string unions don't need one).
- Adding UI that displays priority.
- Promoting other boundary enums (`JobType`, `JobSource`, `JobStatus`, `JobItemStatus`) to string unions — they each still have multiple members and an active reason to be enums.

## Risk

Effectively none. The change tightens the type; runtime behavior is unaffected. The compiler will catch any forgotten `JobPriority.HIGH` references via `npm run check`.

## Verification

From `ui/frontend/`:

- `npm run check` — zero TypeScript errors
- `npm run knip` — zero findings
- `npm run test` — all tests pass

## References

- Issue: [#542](https://github.com/drzero42/nexorious/issues/542)
- Preceding cleanup that left the single-member enum: #539
- Backend constants: `internal/db/models/jobs.go:42-47`
- Backend writer that uses non-`high` priority: `internal/scheduler/scheduler.go:160`
