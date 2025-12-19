# Frontend Cleanup Tasks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove unused code, files, and cleanup minor issues in the frontend codebase.

**Architecture:** Simple deletions and minor fixes. Each task is independent and can be done in any order.

**Tech Stack:** SvelteKit, TypeScript

**Related Issues:** nexorious-e80, nexorious-sh6, nexorious-5oo, nexorious-857, nexorious-b3h, nexorious-dr3, nexorious-chc

---

## Task 1: Remove Unused import-helpers Functions (nexorious-e80)

**Files:**
- Modify: `frontend/src/lib/utils/import-helpers.ts`
- Delete: `frontend/src/lib/examples/steam-refactor-example.svelte`

**Step 1: Verify functions are unused**

```bash
cd /home/abo/workspace/home/nexorious/frontend
grep -rn "filterActionsForContext\|createSteamConfigFields" src/ --include="*.ts" --include="*.svelte" | grep -v import-helpers.ts
```

Expected: Only steam-refactor-example.svelte uses these functions.

**Step 2: Remove functions from import-helpers.ts**

Delete lines 157-170 (`filterActionsForContext`) and lines 198-217 (`createSteamConfigFields`).

**Step 3: Delete the example file**

```bash
rm frontend/src/lib/examples/steam-refactor-example.svelte
```

**Step 4: Update index.ts exports if needed**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 2: Remove Unused createCompactPlatformDisplay (nexorious-sh6)

**Files:**
- Modify: `frontend/src/lib/utils/platform-utils.ts` (lines 56-68)

**Step 1: Verify function is unused**

```bash
cd /home/abo/workspace/home/nexorious/frontend
grep -rn "createCompactPlatformDisplay" src/ --include="*.ts" --include="*.svelte" | grep -v platform-utils.ts
```

Expected: No matches.

**Step 2: Remove the function**

Delete the `createCompactPlatformDisplay` function (lines 56-68).

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 3: Remove Unused steam-utils.ts File (nexorious-5oo)

**Files:**
- Delete: `frontend/src/lib/utils/steam-utils.ts`
- Modify: `frontend/src/lib/utils/index.ts` (remove export if present)

**Step 1: Verify all functions are unused**

```bash
cd /home/abo/workspace/home/nexorious/frontend
grep -rn "isPCWindowsPlatformActive\|isSteamStorefrontActive\|isSteamGamesUserPreferenceEnabled\|isSteamGamesAvailable\|getSteamGamesUnavailableReason" src/ --include="*.ts" --include="*.svelte" | grep -v steam-utils.ts
```

Expected: No matches (backend handles this via `/api/import/sources/steam/availability`).

**Step 2: Delete the file**

```bash
rm frontend/src/lib/utils/steam-utils.ts
```

**Step 3: Remove from index.ts if exported**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 4: Remove Unused Components (nexorious-857)

**Files:**
- Delete: `frontend/src/lib/components/RouteGuardNew.svelte`
- Delete: `frontend/src/lib/components/ErrorBoundary.svelte`
- Delete: `frontend/src/lib/components/Portal.svelte`
- Modify: `frontend/src/lib/components/index.ts` (remove exports)

**Step 1: Verify components are unused**

```bash
cd /home/abo/workspace/home/nexorious/frontend
grep -rn "RouteGuardNew\|ErrorBoundary\|Portal" src/ --include="*.ts" --include="*.svelte" | grep -v "index.ts\|RouteGuardNew.svelte\|ErrorBoundary.svelte\|Portal.svelte"
```

Expected: No matches (only RouteGuard is used, not RouteGuardNew).

**Step 2: Delete the component files**

```bash
rm frontend/src/lib/components/RouteGuardNew.svelte
rm frontend/src/lib/components/ErrorBoundary.svelte
rm frontend/src/lib/components/Portal.svelte
```

**Step 3: Remove exports from index.ts**

Remove lines 2-3 and 14 (or wherever these are exported).

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 5: Remove Commented-Out Code Blocks (nexorious-b3h)

**Files:** Multiple frontend files

**Step 1: Find commented-out code**

```bash
cd /home/abo/workspace/home/nexorious/frontend
grep -rn "// TODO\|// FIXME\|// HACK\|/\*\s*$" src/ --include="*.ts" --include="*.svelte" | head -30
```

Also look for multi-line comments that contain code:
```bash
grep -rn "<!--.*-->" src/ --include="*.svelte" -A5 | head -50
```

**Step 2: Evaluate each commented block**

For each block:
- If it's a valid TODO with clear next steps → Keep
- If it's dead code with no clear purpose → Delete
- If it's commented-out implementation → Delete (git history preserves it)

**Step 3: Remove dead commented code**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 6: Clean Up Empty handleTagChange (nexorious-dr3)

**Files:**
- Modify: `frontend/src/lib/components/GameTagEditor.svelte`

**Step 1: Find the empty handler**

```bash
grep -rn "handleTagChange" frontend/src --include="*.svelte" -A5
```

**Step 2: Either implement or remove**

If the handler is empty and not needed:
- Remove the function
- Remove the event binding that calls it

If it should have logic:
- Implement the required functionality

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 7: Update Frontend Documentation Comments (nexorious-chc)

**Files:**
- Modify: `frontend/src/lib/types/platform-resolution.ts` (line 5)
- Modify: `frontend/src/lib/types/darkadia.ts` (line 3)

**Step 1: Update platform-resolution.ts**

Change line 5 from:
```typescript
* from backend/app/api/schemas/platform.py
```
to:
```typescript
* from backend/app/schemas/platform.py
```

**Step 2: Update darkadia.ts**

Change line 3 from:
```typescript
* Matches backend API schemas in backend/app/api/schemas/darkadia.py
```
to:
```typescript
* Matches backend schemas in backend/app/schemas/darkadia.py
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Acceptance Criteria

- [ ] `filterActionsForContext` and `createSteamConfigFields` removed from import-helpers.ts
- [ ] `steam-refactor-example.svelte` deleted
- [ ] `createCompactPlatformDisplay` removed from platform-utils.ts
- [ ] `steam-utils.ts` deleted
- [ ] Unused components (RouteGuardNew, ErrorBoundary, Portal) deleted
- [ ] Component index.ts exports updated
- [ ] Dead commented code removed
- [ ] Empty handleTagChange cleaned up
- [ ] Documentation comments updated with correct paths
- [ ] `npm run check` passes
- [ ] `npm run test` passes
