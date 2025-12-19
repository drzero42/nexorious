# Frontend Anti-Pattern Refactoring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor frontend code to eliminate anti-patterns in state management, TypeScript types, component size, logging, and reactivity patterns.

**Architecture:** Systematic cleanup across the Svelte frontend, addressing each anti-pattern category independently. Changes are isolated and can be done in any order.

**Tech Stack:** SvelteKit, TypeScript, Svelte 5 runes

**Related Issues:** nexorious-548 (epic), nexorious-bji, nexorious-qpk, nexorious-9xc, nexorious-ayw, nexorious-d8e, nexorious-idn, nexorious-ife, nexorious-60w, nexorious-lgs, nexorious-5hn

---

## Task 1: Remove/Wrap Console.log Statements (nexorious-bji)

**Files:** Multiple files with 107 instances

**Step 1: Find all console.log statements**

```bash
cd /home/abo/workspace/home/nexorious/frontend && grep -rn "console.log" src/ --include="*.ts" --include="*.svelte" | wc -l
```

**Step 2: Create a logger utility if not exists**

Create `frontend/src/lib/utils/logger.ts`:

```typescript
const isDev = import.meta.env.DEV;

export const logger = {
  debug: (...args: unknown[]) => {
    if (isDev) console.log('[DEBUG]', ...args);
  },
  info: (...args: unknown[]) => {
    if (isDev) console.info('[INFO]', ...args);
  },
  warn: (...args: unknown[]) => {
    console.warn('[WARN]', ...args);
  },
  error: (...args: unknown[]) => {
    console.error('[ERROR]', ...args);
  },
};
```

**Step 3: Replace console.log calls**

For each file:
- Remove debug-only console.log statements
- Replace important logs with `logger.info()` or `logger.debug()`
- Keep `console.error()` and `console.warn()` or replace with `logger.error()`/`logger.warn()`

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 2: Break Down Large Components (nexorious-qpk)

**Files:** 5+ components over 500 lines

**Step 1: Identify large components**

```bash
find frontend/src -name "*.svelte" -exec wc -l {} + | sort -rn | head -20
```

**Step 2: Refactor pattern**

For each large component:
1. Identify logically separate sections (e.g., header, filters, list, modals)
2. Extract into child components with clear props interfaces
3. Keep state management in parent, pass handlers as props
4. Use composition over monolithic structures

**Example breakdown for a typical large component:**
- `GameList.svelte` (800 lines) → Split into:
  - `GameListHeader.svelte` (filters, search, view toggle)
  - `GameListGrid.svelte` (grid view rendering)
  - `GameListTable.svelte` (table view rendering)
  - `GameListPagination.svelte` (pagination controls)

**Step 3: Update imports in parent components**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 3: Replace TypeScript `any` Types (nexorious-9xc)

**Files:** Multiple files with `any` types

**Step 1: Find all any types**

```bash
cd /home/abo/workspace/home/nexorious/frontend && grep -rn ": any" src/ --include="*.ts" --include="*.svelte" | grep -v node_modules
```

**Step 2: Replace with proper interfaces**

For each `any`:
1. Analyze the actual data structure being used
2. Create or use existing interface
3. Replace `any` with specific type

**Common replacements:**
- API responses → Use types from `src/lib/types/`
- Event handlers → `MouseEvent`, `KeyboardEvent`, etc.
- Unknown objects → `Record<string, unknown>` or specific interface
- Catch blocks → `unknown` then narrow with type guards

**Example:**
```typescript
// Before
const handleClick = (e: any) => { ... }

// After
const handleClick = (e: MouseEvent) => { ... }
```

**Verification:**
```bash
npm run check
```

---

## Task 4: Consolidate Store Creation Patterns (nexorious-ayw)

**Files:** Store files in `frontend/src/lib/stores/`

**Step 1: Audit current store patterns**

```bash
grep -rn "writable\|readable\|derived" frontend/src/lib/stores/
```

**Step 2: Create shared store utility**

Create `frontend/src/lib/stores/utils/create-store.ts`:

```typescript
import { writable, type Writable } from 'svelte/store';

interface StoreOptions<T> {
  persist?: boolean;
  key?: string;
}

export function createStore<T>(
  initialValue: T,
  options: StoreOptions<T> = {}
): Writable<T> {
  const { persist = false, key } = options;

  // Load from localStorage if persist enabled
  let value = initialValue;
  if (persist && key && typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem(key);
    if (stored) {
      try {
        value = JSON.parse(stored);
      } catch {
        // Use default if parsing fails
      }
    }
  }

  const store = writable<T>(value);

  // Save to localStorage on changes if persist enabled
  if (persist && key) {
    store.subscribe((v) => {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(key, JSON.stringify(v));
      }
    });
  }

  return store;
}
```

**Step 3: Migrate existing stores to use shared utility**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 5: Use Existing ApiService Instead of Duplicated apiCall (nexorious-d8e)

**Files:** Components using custom fetch instead of ApiService

**Step 1: Find duplicated API call patterns**

```bash
grep -rn "fetch\(" frontend/src --include="*.svelte" --include="*.ts" | grep -v ApiService | grep -v node_modules
```

**Step 2: Migrate to ApiService**

Replace raw `fetch()` calls with `ApiService` methods:

```typescript
// Before
const response = await fetch('/api/games', {
  headers: { Authorization: `Bearer ${token}` }
});
const data = await response.json();

// After
import { ApiService } from '$lib/services/api';
const data = await ApiService.get('/api/games');
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 6: Extract Duplicated Component Utilities (nexorious-idn)

**Files:** Multiple components with similar utility functions

**Step 1: Identify duplicated utilities**

Look for repeated patterns like:
- Date formatting
- Number formatting
- String truncation
- Validation helpers

**Step 2: Extract to shared utilities**

Add to `frontend/src/lib/utils/`:
- `format.ts` - Formatting utilities
- `validation.ts` - Validation helpers

**Step 3: Update components to use shared utilities**

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 7: Fix Manual Reactivity Hacks (nexorious-ife)

**Files:** Components with Set/Map reassignment for reactivity

**Step 1: Find reactivity hacks**

```bash
grep -rn "= new Set\|= new Map" frontend/src --include="*.svelte" -A2 -B2
```

Look for patterns like:
```typescript
// Anti-pattern
mySet = new Set(mySet);
myMap = new Map(myMap);
```

**Step 2: Replace with Svelte 5 runes**

Use `$state` with proper mutation tracking:

```typescript
// Before (hack)
let items = new Set<string>();
items.add('item');
items = new Set(items); // Force reactivity

// After (Svelte 5 runes)
let items = $state(new Set<string>());
items.add('item'); // Reactivity handled automatically
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 8: Improve Timer/Interval Cleanup (nexorious-60w)

**Files:** Components using setTimeout/setInterval

**Step 1: Find timer usage**

```bash
grep -rn "setTimeout\|setInterval" frontend/src --include="*.svelte"
```

**Step 2: Ensure proper cleanup**

For each timer, ensure cleanup in `onDestroy` or return from `$effect`:

```typescript
// Before (potential leak)
let timer: number;
onMount(() => {
  timer = setInterval(() => { ... }, 1000);
});

// After (proper cleanup)
$effect(() => {
  const timer = setInterval(() => { ... }, 1000);
  return () => clearInterval(timer);
});
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 9: Refactor Event Listener Cleanup (nexorious-lgs)

**Files:** Components with manual event listeners

**Step 1: Find event listener patterns**

```bash
grep -rn "addEventListener\|removeEventListener" frontend/src --include="*.svelte"
```

**Step 2: Use effect.pre() for cleanup**

```typescript
// Before
onMount(() => {
  window.addEventListener('resize', handleResize);
  return () => window.removeEventListener('resize', handleResize);
});

// After (Svelte 5)
$effect.pre(() => {
  window.addEventListener('resize', handleResize);
  return () => window.removeEventListener('resize', handleResize);
});
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Task 10: Replace Hardcoded Timeout Values (nexorious-5hn)

**Files:** Components with magic numbers for timeouts

**Step 1: Find hardcoded timeouts**

```bash
grep -rn "setTimeout.*[0-9]\{3,\}\|setInterval.*[0-9]\{3,\}" frontend/src --include="*.svelte" --include="*.ts"
```

**Step 2: Create constants file**

Add to `frontend/src/lib/constants/timing.ts`:

```typescript
export const TIMING = {
  DEBOUNCE_MS: 300,
  TOAST_DURATION_MS: 5000,
  ANIMATION_DURATION_MS: 200,
  POLLING_INTERVAL_MS: 30000,
  API_TIMEOUT_MS: 10000,
} as const;
```

**Step 3: Replace magic numbers with constants**

```typescript
// Before
setTimeout(doSomething, 300);

// After
import { TIMING } from '$lib/constants/timing';
setTimeout(doSomething, TIMING.DEBOUNCE_MS);
```

**Verification:**
```bash
npm run check
npm run test
```

---

## Acceptance Criteria

- [ ] No console.log statements (or wrapped in dev-only logger)
- [ ] No components over 500 lines
- [ ] No `any` types in TypeScript
- [ ] Consistent store creation pattern
- [ ] No duplicated API call patterns
- [ ] Shared utility functions extracted
- [ ] No Set/Map reassignment hacks
- [ ] All timers properly cleaned up
- [ ] Event listeners use effect.pre() pattern
- [ ] No hardcoded timeout values
- [ ] `npm run check` passes
- [ ] `npm run test` passes
