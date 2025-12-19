# Frontend Test Anti-Pattern Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Clean up anti-patterns in frontend tests to improve reliability, maintainability, and debugging experience.

**Architecture:** Systematic cleanup across test files, addressing P2 issues first (test reliability), then P3 issues (maintainability).

**Tech Stack:** Vitest, @testing-library/svelte, userEvent

**Related Issues:** nexorious-j3x (epic), nexorious-70i, nexorious-7z6, nexorious-3ne, nexorious-mdp, nexorious-7ju, nexorious-6t9, nexorious-71i, nexorious-ofl, nexorious-f9s, nexorious-0kh, nexorious-f21, nexorious-z9m

---

## Priority 2 Tasks (Test Reliability)

### Task 1: Fix Store Mocking by Mutating .value Directly (nexorious-70i)

**Problem:** Tests mock stores by directly mutating `.value` instead of using proper Svelte store patterns.

**Files:** Multiple test files

**Step 1: Find problematic patterns**

```bash
grep -rn "\.value\s*=" frontend/src --include="*.test.ts"
```

**Step 2: Refactor to proper store mocking**

```typescript
// Before (anti-pattern)
vi.mock('$lib/stores/auth', () => ({
  authStore: { value: { user: mockUser } }
}));

// After (proper pattern)
vi.mock('$lib/stores/auth', () => {
  const { writable } = require('svelte/store');
  return {
    authStore: writable({ user: mockUser })
  };
});
```

Or use `svelte/store`'s `get` function with writable stores:

```typescript
import { writable } from 'svelte/store';

const mockAuthStore = writable({ user: mockUser });

vi.mock('$lib/stores/auth', () => ({
  authStore: mockAuthStore
}));

// Update in test
mockAuthStore.set({ user: differentUser });
```

**Verification:**
```bash
npm run test
```

---

### Task 2: Fix Tests Swallowing Errors with try/catch (nexorious-7z6)

**Problem:** Tests use try/catch but don't properly assert on errors.

**Files:** Multiple test files

**Step 1: Find problematic patterns**

```bash
grep -rn "try {" frontend/src --include="*.test.ts" -A5 | grep -v "expect"
```

**Step 2: Refactor to use expect assertions**

```typescript
// Before (anti-pattern)
test('handles error', async () => {
  try {
    await someFunction();
  } catch (e) {
    // Silently swallowed
  }
});

// After (proper assertion)
test('handles error', async () => {
  await expect(someFunction()).rejects.toThrow('Expected error message');
});

// Or for testing error handling behavior
test('shows error message on failure', async () => {
  server.use(
    http.get('/api/data', () => HttpResponse.json({ error: 'Failed' }, { status: 500 }))
  );

  render(MyComponent);
  await waitFor(() => {
    expect(screen.getByText('Failed to load data')).toBeInTheDocument();
  });
});
```

**Verification:**
```bash
npm run test
```

---

### Task 3: Fix Store Tests with Dynamic Imports (nexorious-3ne)

**Problem:** Store tests dynamically import modules, causing test isolation issues.

**Files:** Store test files in `frontend/src/lib/stores/**/*.test.ts`

**Step 1: Find dynamic imports**

```bash
grep -rn "await import\|dynamic import" frontend/src/lib/stores --include="*.test.ts"
```

**Step 2: Use static imports with vi.resetModules()**

```typescript
// Before (anti-pattern)
test('store behavior', async () => {
  const { myStore } = await import('./myStore');
  // Tests may share state between tests
});

// After (proper isolation)
import { beforeEach, vi } from 'vitest';

beforeEach(() => {
  vi.resetModules();
});

// Then import at top level
import { myStore } from './myStore';

test('store behavior', () => {
  // Fresh module state for each test
});
```

**Verification:**
```bash
npm run test
```

---

### Task 4: Fix Global console.error Suppression (nexorious-mdp)

**Problem:** Tests suppress console.error globally, hiding real errors.

**Files:** Test setup files and individual tests

**Step 1: Find global suppression**

```bash
grep -rn "console.error\s*=" frontend/src --include="*.test.ts"
grep -rn "vi.spyOn.*console.*error" frontend/src --include="*.test.ts"
```

**Step 2: Use scoped suppression**

```typescript
// Before (anti-pattern - global suppression)
beforeAll(() => {
  console.error = vi.fn();
});

// After (scoped suppression)
test('handles error gracefully', () => {
  const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

  // Test code that triggers expected error
  render(ComponentThatThrows);

  expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('expected error'));
  consoleSpy.mockRestore();
});
```

**Verification:**
```bash
npm run test
```

---

## Priority 3 Tasks (Maintainability)

### Task 5: Reduce Excessive CSS Class Testing (nexorious-7ju)

**Files:** `frontend/src/lib/components/ToastContainer.test.ts`

**Problem:** Tests check exact CSS classes instead of user-visible behavior.

**Step 1: Identify CSS class assertions**

```bash
grep -rn "toHaveClass\|className" frontend/src --include="*.test.ts" | head -30
```

**Step 2: Replace with behavior/accessibility tests**

```typescript
// Before (anti-pattern)
expect(toast).toHaveClass('toast-success bg-green-500 text-white');

// After (behavior-focused)
expect(screen.getByRole('alert')).toHaveAccessibleName(/success/i);
expect(toast).toBeVisible();
```

**Verification:**
```bash
npm run test
```

---

### Task 6: Fix getAllByText Usage (nexorious-6t9)

**Problem:** Using `getAllByText` for assertions indicates unclear component structure.

**Files:** Multiple test files

**Step 1: Find getAllByText usage**

```bash
grep -rn "getAllByText\|queryAllByText" frontend/src --include="*.test.ts"
```

**Step 2: Use more specific queries**

```typescript
// Before (anti-pattern)
const items = screen.getAllByText(/game/i);
expect(items).toHaveLength(3);

// After (specific queries)
const gameCards = screen.getAllByRole('article', { name: /game card/i });
expect(gameCards).toHaveLength(3);

// Or use within() for scoped queries
const list = screen.getByRole('list');
const items = within(list).getAllByRole('listitem');
expect(items).toHaveLength(3);
```

**Verification:**
```bash
npm run test
```

---

### Task 7: Remove Duplicate Test Coverage (nexorious-71i)

**Files:** Multiple test files with overlapping coverage

**Step 1: Audit test coverage**

```bash
npm run test:coverage
```

**Step 2: Identify and consolidate duplicates**

Look for:
- Same component tested in multiple files
- Identical test cases with different descriptions
- Integration tests duplicating unit test coverage

**Step 3: Remove lower-quality duplicates**

Keep the test that:
- Has better assertions
- Tests more edge cases
- Is more maintainable

**Verification:**
```bash
npm run test
npm run test:coverage
```

---

### Task 8: Replace fireEvent with userEvent (nexorious-ofl)

**Problem:** Using `fireEvent` instead of `userEvent` for user interactions.

**Files:** Multiple test files

**Step 1: Find fireEvent usage**

```bash
grep -rn "fireEvent\." frontend/src --include="*.test.ts"
```

**Step 2: Replace with userEvent**

```typescript
// Before (anti-pattern)
import { fireEvent } from '@testing-library/svelte';

fireEvent.click(button);
fireEvent.input(textbox, { target: { value: 'test' } });

// After (user-centric)
import { userEvent } from '@testing-library/user-event';

const user = userEvent.setup();
await user.click(button);
await user.type(textbox, 'test');
```

**Note:** `userEvent` better simulates real user behavior (focus, blur, keyboard events).

**Verification:**
```bash
npm run test
```

---

### Task 9: Replace container.querySelector (nexorious-f9s)

**Problem:** Using `container.querySelector` instead of testing-library queries.

**Files:** Multiple test files

**Step 1: Find querySelector usage**

```bash
grep -rn "container.querySelector\|container.querySelectorAll" frontend/src --include="*.test.ts"
```

**Step 2: Replace with testing-library queries**

```typescript
// Before (anti-pattern)
const { container } = render(MyComponent);
const button = container.querySelector('button.submit');

// After (accessible query)
render(MyComponent);
const button = screen.getByRole('button', { name: /submit/i });
```

**Common replacements:**
- `querySelector('button')` → `getByRole('button')`
- `querySelector('input')` → `getByRole('textbox')` or `getByLabelText()`
- `querySelector('.error')` → `getByRole('alert')` or `getByText(/error/i)`

**Verification:**
```bash
npm run test
```

---

### Task 10: Simplify Complex beforeEach Mocking (nexorious-0kh)

**Problem:** Excessive beforeEach setup with many mocks makes tests hard to understand.

**Files:** Multiple test files

**Step 1: Find complex beforeEach blocks**

```bash
grep -rn "beforeEach" frontend/src --include="*.test.ts" -A30 | head -100
```

**Step 2: Refactor to test-specific setup**

```typescript
// Before (anti-pattern)
beforeEach(() => {
  vi.mock('$lib/stores/auth', () => ({ ... }));
  vi.mock('$lib/stores/games', () => ({ ... }));
  vi.mock('$lib/services/api', () => ({ ... }));
  vi.mock('$lib/utils/format', () => ({ ... }));
  // 20 more mocks...
});

// After (focused setup)
// Use factory functions for common setups
function setupAuthenticatedUser(user = mockUser) {
  vi.mocked(useAuth).mockReturnValue({ user, isAuthenticated: true });
}

function setupApiMock(handlers: HttpHandler[]) {
  server.use(...handlers);
}

test('shows user games', async () => {
  setupAuthenticatedUser();
  setupApiMock([
    http.get('/api/games', () => HttpResponse.json(mockGames))
  ]);

  render(GameList);
  // assertions...
});
```

**Verification:**
```bash
npm run test
```

---

### Task 11: Add Missing Assertions (nexorious-f21)

**Problem:** Tests verify 'should not throw' but have no assertions.

**Files:** Multiple test files

**Step 1: Find assertion-less tests**

```bash
grep -rn "test\|it\(" frontend/src --include="*.test.ts" -A10 | grep -B10 "});" | grep -v "expect"
```

**Step 2: Add meaningful assertions**

```typescript
// Before (anti-pattern)
test('renders without crashing', () => {
  render(MyComponent);
  // No assertions!
});

// After (meaningful assertion)
test('renders component content', () => {
  render(MyComponent);
  expect(screen.getByRole('heading', { name: /title/i })).toBeInTheDocument();
});
```

**Verification:**
```bash
npm run test
```

---

### Task 12: Reduce toMatchObject/toEqual Overuse (nexorious-z9m)

**Problem:** Tests use excessive snapshot-style matching instead of specific assertions.

**Files:** Multiple test files

**Step 1: Find excessive object matching**

```bash
grep -rn "toMatchObject\|toEqual" frontend/src --include="*.test.ts" | head -30
```

**Step 2: Replace with specific assertions**

```typescript
// Before (anti-pattern)
expect(result).toMatchObject({
  id: expect.any(String),
  name: 'Test',
  createdAt: expect.any(Date),
  updatedAt: expect.any(Date),
  // 20 more properties...
});

// After (focused assertions)
expect(result.name).toBe('Test');
expect(result.id).toBeDefined();
// Only assert on properties relevant to the test
```

**Verification:**
```bash
npm run test
```

---

## Acceptance Criteria

### P2 Issues (All must be resolved)
- [ ] No store mocking by mutating `.value` directly
- [ ] No try/catch blocks without assertions
- [ ] No dynamic imports causing isolation issues
- [ ] No global console.error suppression

### P3 Issues (At least 50% resolved)
- [ ] Reduced CSS class testing in ToastContainer
- [ ] No getAllByText for assertions where specific queries work
- [ ] Duplicate test coverage removed
- [ ] fireEvent replaced with userEvent
- [ ] container.querySelector replaced with testing-library queries
- [ ] Complex beforeEach simplified
- [ ] Missing assertions added
- [ ] toMatchObject/toEqual overuse reduced

### Quality Gates
- [ ] `npm run test` passes
- [ ] Test coverage maintained or improved
- [ ] No new anti-patterns introduced
