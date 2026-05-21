# JobPriority Widen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the frontend's one-member `JobPriority` enum with a string-literal union (`'low' | 'normal' | 'high'`) so the type matches the backend's actual priority domain.

**Architecture:** Single file is the source of truth (`ui/frontend/src/types/jobs.ts`); two test files reference the old `JobPriority.HIGH` symbol and switch to the literal `'high'`. The boundary cast in `ui/frontend/src/api/jobs.ts` stays as `as JobPriority` — unchanged in form, but now widened by the new union. No production `.tsx` reads `.priority`, so no component code changes.

**Tech Stack:** TypeScript 5, Vitest, knip; project uses standard `tsconfig` (no `verbatimModuleSyntax`).

**Spec:** [docs/superpowers/specs/2026-05-21-issue-542-jobpriority-widen-design.md](../specs/2026-05-21-issue-542-jobpriority-widen-design.md)

**Branch:** `542-jobpriority-widen` (already created; spec commit landed there).

---

## Task 1: Widen JobPriority to string union and update call sites

**Files:**
- Modify: `ui/frontend/src/types/jobs.ts:45-47` (the enum block)
- Modify: `ui/frontend/src/api/jobs.test.ts:4` (import) and `ui/frontend/src/api/jobs.test.ts:70` (assertion)
- Modify: `ui/frontend/src/components/jobs/job-card.test.tsx:6` (import) and `ui/frontend/src/components/jobs/job-card.test.tsx:15` (mock data)

Not touched (intentional, verify it remains untouched):
- `ui/frontend/src/api/jobs.ts:12,175` — already correct; the `import type` block stays as-is, the `as JobPriority` cast stays as-is.
- `ui/frontend/src/types/index.ts` — `export *` already forwards type aliases.

> **Working directory note:** `npm` commands run from `ui/frontend/`; `git` commands run from the repo root. The plan calls this out explicitly only where it matters.

- [ ] **Step 1: Capture baseline — confirm current build is green**

```bash
cd ui/frontend
npm run check && npm run knip && npm run test -- --run
```
Expected: all three commands exit 0. Capture this so you know the baseline before changing anything. If any of these is already failing on `main`, stop and surface it to the user — the change in this plan won't be the cause.

- [ ] **Step 2: Replace the enum with a string-literal union in `types/jobs.ts`**

In [ui/frontend/src/types/jobs.ts](../../ui/frontend/src/types/jobs.ts), replace lines 45-47:

```ts
export enum JobPriority {
  HIGH = 'high',
}
```

with:

```ts
export type JobPriority = 'low' | 'normal' | 'high';
```

Do not change anything else in the file. The `Job.priority: JobPriority` field declaration at line 74 remains the same — the type name is unchanged, only its shape is.

- [ ] **Step 3: Run typecheck — expect failures in the two test files**

```bash
npm run check
```
Expected: TypeScript errors in `api/jobs.test.ts` and `components/jobs/job-card.test.tsx` complaining that `JobPriority.HIGH` does not exist on a string-union type. This is the *intended* signal — it proves the call sites needed the widening to be type-aware. If `check` passes with no errors, something is wrong (maybe the file wasn't saved); stop and investigate.

- [ ] **Step 4: Update `api/jobs.test.ts`**

In [ui/frontend/src/api/jobs.test.ts](../../ui/frontend/src/api/jobs.test.ts):

Replace line 4:
```ts
import { JobType, JobSource, JobStatus, JobPriority } from '@/types';
```
with:
```ts
import { JobType, JobSource, JobStatus } from '@/types';
```

(Remove `JobPriority` from the import — after the next edit, the file no longer references the symbol. `JobType`, `JobSource`, `JobStatus` are still enums and stay value imports.)

Replace line 70:
```ts
      expect(result.jobs[0].priority).toBe(JobPriority.HIGH);
```
with:
```ts
      expect(result.jobs[0].priority).toBe('high');
```

- [ ] **Step 5: Update `job-card.test.tsx`**

In [ui/frontend/src/components/jobs/job-card.test.tsx](../../ui/frontend/src/components/jobs/job-card.test.tsx):

Replace line 6:
```ts
import { JobType, JobSource, JobStatus, JobPriority } from '@/types';
```
with:
```ts
import { JobType, JobSource, JobStatus } from '@/types';
```

Replace line 15 (inside the `mockJob` literal):
```ts
  priority: JobPriority.HIGH,
```
with:
```ts
  priority: 'high',
```

The surrounding `const mockJob: Job = { ... }` type annotation keeps `priority` constrained to the union, so the literal `'high'` is checked.

- [ ] **Step 6: Run typecheck — expect clean pass**

```bash
npm run check
```
Expected: exit 0, no errors. If errors remain, re-read the error and find the missed call site (use `grep -rn 'JobPriority' ui/frontend/src` from the repo root to confirm only the union-type declaration in `types/jobs.ts` and the `import type` + `as JobPriority` in `api/jobs.ts` remain).

- [ ] **Step 7: Run knip — expect no findings**

```bash
npm run knip
```
Expected: exit 0, no unused-exports findings related to `JobPriority`. A string-union export does not have "members" for knip to flag.

- [ ] **Step 8: Run the frontend test suite**

```bash
npm run test -- --run
```
Expected: all tests pass. `--run` flag forces a one-shot run (Vitest defaults to watch mode in some configurations).

- [ ] **Step 9: Sanity check — confirm only the expected symbols remain**

```bash
grep -rn 'JobPriority' ui/frontend/src
```
Expected output (exact lines may shift by one or two):
- `ui/frontend/src/types/jobs.ts: export type JobPriority = 'low' | 'normal' | 'high';`
- `ui/frontend/src/types/jobs.ts:   priority: JobPriority;` (the `Job` field)
- `ui/frontend/src/api/jobs.ts:   JobPriority,` (inside the existing `import type` block)
- `ui/frontend/src/api/jobs.ts:   priority: apiJob.priority as JobPriority,`

No other references should appear. In particular, no `JobPriority.HIGH` anywhere.

- [ ] **Step 10: Commit**

From the repo root:
```bash
git add ui/frontend/src/types/jobs.ts \
        ui/frontend/src/api/jobs.test.ts \
        ui/frontend/src/components/jobs/job-card.test.tsx
git commit -m "$(cat <<'EOF'
fix(frontend): widen JobPriority to match backend domain

Closes #542.

The frontend JobPriority enum had only HIGH after the knip cleanup in
#539, but the backend writes 'low', 'normal', and 'high' (scheduled
syncs use 'low'). The unchecked 'as JobPriority' cast in transformJob
hid the divergence at runtime, leaving any future consumer of
job.priority typed against a single-value enum.

Replace the enum with a string-literal union so all three backend
values are part of the type. Update the two test references from
JobPriority.HIGH to the literal 'high'. The 'as JobPriority' cast
stays in place, matching the sibling jobType/source/status casts.
EOF
)"
```

(The Conventional Commit prefix `fix:` triggers a patch bump in release-please per the CLAUDE.md release-process rules. The heredoc form is required by CLAUDE.md to preserve formatting of multi-line commit messages.)

---

## Out of Scope (do NOT include in this PR)

- Runtime narrowing/validation of `priority` (or `jobType`/`source`/`status`) inside `transformJob`.
- Knip carve-out config changes — none required for a string union.
- Adding any UI that displays priority.
- Promoting `JobType`/`JobSource`/`JobStatus`/`JobItemStatus` to string unions.

If any of these come up during implementation, file a separate issue rather than expanding the PR.

## Post-implementation

After Task 1 lands locally:
- Push the branch: `git push -u origin 542-jobpriority-widen`
- Open a PR with title `fix(frontend): widen JobPriority to match backend domain` (matches the commit; squash-merge will use it).
- Do not self-merge; CLAUDE.md mandates the user explicitly approve PR merges.
