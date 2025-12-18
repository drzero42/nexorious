# Setup Page Route Guard Integration - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add setup status checking to the RouteGuard so users are redirected to `/setup` when initial admin setup is needed.

**Architecture:** The RouteGuard component will check setup status before authentication. If `needs_setup` is true, redirect to `/setup`. This check happens in the RouteGuard (which wraps protected routes), not in a global layout, matching Next.js patterns. The setup page itself already handles its own setup status check internally.

**Tech Stack:** React 18, Next.js 15 (App Router), TypeScript, Vitest, Testing Library

---

## Summary of Changes

Currently in frontend-next:
- Setup page exists at `src/app/(auth)/setup/page.tsx` with full functionality
- RouteGuard only checks authentication, not setup status
- Protected routes (under `(main)/`) use RouteGuard but don't know about setup

Missing:
1. RouteGuard needs to check setup status before checking authentication
2. If setup is needed, redirect to `/setup` instead of `/login`

---

### Task 1: Add Setup Status API Hook

**Files:**
- Create: `frontend-next/src/hooks/use-setup-status.ts`
- Create: `frontend-next/src/hooks/use-setup-status.test.ts`
- Modify: `frontend-next/src/hooks/index.ts`

**Step 1: Write the failing test**

Create `frontend-next/src/hooks/use-setup-status.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useSetupStatus } from './use-setup-status';

// Mock auth API
const mockCheckSetupStatus = vi.fn();

vi.mock('@/api/auth', () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
}));

describe('useSetupStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns loading state initially', () => {
    mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));

    const { result } = renderHook(() => useSetupStatus());

    expect(result.current.isLoading).toBe(true);
    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('returns needsSetup=true when setup is needed', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it('returns needsSetup=false when setup is complete', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('returns error when API call fails', async () => {
    mockCheckSetupStatus.mockRejectedValue(new Error('Network error'));

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBe('Network error');
  });

  it('returns generic error for non-Error rejections', async () => {
    mockCheckSetupStatus.mockRejectedValue('string error');

    const { result } = renderHook(() => useSetupStatus());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.needsSetup).toBeNull();
    expect(result.current.error).toBe('Failed to check setup status');
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd frontend-next && npm run test -- src/hooks/use-setup-status.test.ts`

Expected: FAIL with "Cannot find module './use-setup-status'"

**Step 3: Write minimal implementation**

Create `frontend-next/src/hooks/use-setup-status.ts`:

```typescript
import { useState, useEffect } from 'react';
import * as authApi from '@/api/auth';

interface UseSetupStatusResult {
  needsSetup: boolean | null;
  isLoading: boolean;
  error: string | null;
}

export function useSetupStatus(): UseSetupStatusResult {
  const [needsSetup, setNeedsSetup] = useState<boolean | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        setNeedsSetup(status.needs_setup);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to check setup status');
      } finally {
        setIsLoading(false);
      }
    };

    checkSetup();
  }, []);

  return { needsSetup, isLoading, error };
}
```

**Step 4: Export the hook**

Modify `frontend-next/src/hooks/index.ts`:

```typescript
export { usePlatforms } from './use-platforms';
export { useTags } from './use-tags';
export { useGames } from './use-games';
export { useSetupStatus } from './use-setup-status';
```

**Step 5: Run test to verify it passes**

Run: `cd frontend-next && npm run test -- src/hooks/use-setup-status.test.ts`

Expected: PASS

**Step 6: Commit**

```bash
git add frontend-next/src/hooks/use-setup-status.ts frontend-next/src/hooks/use-setup-status.test.ts frontend-next/src/hooks/index.ts
git commit -m "feat(frontend-next): add useSetupStatus hook for checking initial setup"
```

---

### Task 2: Update RouteGuard to Check Setup Status

**Files:**
- Modify: `frontend-next/src/components/route-guard.tsx`
- Modify: `frontend-next/src/components/route-guard.test.tsx`

**Step 1: Add tests for setup status checking**

Add new test cases to `frontend-next/src/components/route-guard.test.tsx`. First, read the existing file and add these tests:

```typescript
// Add to existing imports
const mockCheckSetupStatus = vi.fn();

// Add to existing vi.mock('@/api/auth', ...) or create new mock:
vi.mock('@/api/auth', () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
}));

// Add new describe block for setup status tests:
describe('setup status checks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
  });

  it('redirects to /setup when setup is needed', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    mockUseAuth.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
      login: vi.fn(),
      logout: vi.fn(),
      error: null,
      clearError: vi.fn(),
    });

    render(
      <RouteGuard>
        <div>Protected Content</div>
      </RouteGuard>
    );

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/setup');
    });
  });

  it('does not render children when setup is needed', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    mockUseAuth.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
      login: vi.fn(),
      logout: vi.fn(),
      error: null,
      clearError: vi.fn(),
    });

    render(
      <RouteGuard>
        <div>Protected Content</div>
      </RouteGuard>
    );

    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/setup');
    });

    expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
  });

  it('proceeds to auth check when setup is not needed', async () => {
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
    mockUseAuth.mockReturnValue({
      isAuthenticated: true,
      isLoading: false,
      user: { id: '1', username: 'test', isAdmin: false },
      login: vi.fn(),
      logout: vi.fn(),
      error: null,
      clearError: vi.fn(),
    });

    render(
      <RouteGuard>
        <div>Protected Content</div>
      </RouteGuard>
    );

    await waitFor(() => {
      expect(screen.getByText('Protected Content')).toBeInTheDocument();
    });

    expect(mockReplace).not.toHaveBeenCalled();
  });

  it('shows loading spinner while checking setup status', () => {
    mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));
    mockUseAuth.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
      login: vi.fn(),
      logout: vi.fn(),
      error: null,
      clearError: vi.fn(),
    });

    const { container } = render(
      <RouteGuard>
        <div>Protected Content</div>
      </RouteGuard>
    );

    expect(container.querySelector('.animate-spin')).toBeInTheDocument();
    expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
  });

  it('continues to login redirect if setup check fails', async () => {
    mockCheckSetupStatus.mockRejectedValue(new Error('Network error'));
    mockUseAuth.mockReturnValue({
      isAuthenticated: false,
      isLoading: false,
      user: null,
      login: vi.fn(),
      logout: vi.fn(),
      error: null,
      clearError: vi.fn(),
    });

    render(
      <RouteGuard>
        <div>Protected Content</div>
      </RouteGuard>
    );

    // Should fall through to auth check and redirect to login
    await waitFor(() => {
      expect(mockReplace).toHaveBeenCalledWith('/login');
    });
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `cd frontend-next && npm run test -- src/components/route-guard.test.tsx`

Expected: FAIL - setup-related tests will fail

**Step 3: Update RouteGuard implementation**

Replace `frontend-next/src/components/route-guard.tsx`:

```typescript
'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/providers';
import * as authApi from '@/api/auth';

interface RouteGuardProps {
  children: React.ReactNode;
}

export function RouteGuard({ children }: RouteGuardProps) {
  const router = useRouter();
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const [isCheckingSetup, setIsCheckingSetup] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);

  // Check setup status on mount
  useEffect(() => {
    const checkSetup = async () => {
      try {
        const status = await authApi.checkSetupStatus();
        if (status.needs_setup) {
          setNeedsSetup(true);
          router.replace('/setup');
          return;
        }
      } catch {
        // If setup check fails, continue to auth check
        // (we'll redirect to login if not authenticated)
      } finally {
        setIsCheckingSetup(false);
      }
    };

    checkSetup();
  }, [router]);

  // Handle auth redirect after setup check completes
  useEffect(() => {
    if (!isCheckingSetup && !needsSetup && !authLoading && !isAuthenticated) {
      router.replace('/login');
    }
  }, [isCheckingSetup, needsSetup, authLoading, isAuthenticated, router]);

  // Show loading spinner while checking setup or auth
  if (isCheckingSetup || authLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="animate-spin h-8 w-8 border-4 border-primary border-t-transparent rounded-full" />
      </div>
    );
  }

  // Don't render anything while redirecting to setup
  if (needsSetup) {
    return null;
  }

  // Don't render anything while redirecting to login
  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
```

**Step 4: Run tests to verify they pass**

Run: `cd frontend-next && npm run test -- src/components/route-guard.test.tsx`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend-next/src/components/route-guard.tsx frontend-next/src/components/route-guard.test.tsx
git commit -m "feat(frontend-next): add setup status check to RouteGuard"
```

---

### Task 3: Update RouteGuard Tests for Comprehensive Coverage

**Files:**
- Modify: `frontend-next/src/components/route-guard.test.tsx`

The existing test file needs to be updated to properly mock the new setup status check. This task ensures all existing tests still pass with the new behavior.

**Step 1: Read existing test file and understand current structure**

Read `frontend-next/src/components/route-guard.test.tsx` to understand current mock setup.

**Step 2: Update test file with proper mocks**

The test file needs:
1. Mock for `@/api/auth` with `checkSetupStatus`
2. All existing tests should mock `checkSetupStatus` to return `{ needs_setup: false }` by default
3. New tests for setup-specific behavior

Full test file structure:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { RouteGuard } from './route-guard';

// Mock next/navigation
const mockReplace = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({
    replace: mockReplace,
    push: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Mock auth provider
const mockUseAuth = vi.fn();

vi.mock('@/providers', () => ({
  useAuth: () => mockUseAuth(),
}));

// Mock auth API
const mockCheckSetupStatus = vi.fn();

vi.mock('@/api/auth', () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
}));

describe('RouteGuard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: setup complete
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
  });

  describe('loading states', () => {
    it('shows loading spinner while checking setup status', () => {
      mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      const { container } = render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      expect(container.querySelector('.animate-spin')).toBeInTheDocument();
      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
    });

    it('shows loading spinner while auth is loading', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: true,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      const { container } = render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(container.querySelector('.animate-spin')).toBeInTheDocument();
      });
      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
    });
  });

  describe('setup status checks', () => {
    it('redirects to /setup when setup is needed', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/setup');
      });
    });

    it('does not render children when setup is needed', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/setup');
      });

      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
    });

    it('continues to login redirect if setup check fails', async () => {
      mockCheckSetupStatus.mockRejectedValue(new Error('Network error'));
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/login');
      });
    });
  });

  describe('authentication checks', () => {
    it('renders children when authenticated and setup complete', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isAuthenticated: true,
        isLoading: false,
        user: { id: '1', username: 'test', isAdmin: false },
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(screen.getByText('Protected Content')).toBeInTheDocument();
      });

      expect(mockReplace).not.toHaveBeenCalled();
    });

    it('redirects to /login when not authenticated and setup complete', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/login');
      });
    });

    it('does not render children when not authenticated', async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/login');
      });

      expect(screen.queryByText('Protected Content')).not.toBeInTheDocument();
    });
  });
});
```

**Step 3: Run all RouteGuard tests**

Run: `cd frontend-next && npm run test -- src/components/route-guard.test.tsx`

Expected: PASS

**Step 4: Commit**

```bash
git add frontend-next/src/components/route-guard.test.tsx
git commit -m "test(frontend-next): update RouteGuard tests for setup status checks"
```

---

### Task 4: Run Full Test Suite and Type Check

**Files:**
- None (verification only)

**Step 1: Run type check**

Run: `cd frontend-next && npm run check`

Expected: PASS with no type errors

**Step 2: Run full test suite**

Run: `cd frontend-next && npm run test`

Expected: All tests pass

**Step 3: Verify setup flow manually (optional)**

1. Start backend without any users: `cd backend && uv run alembic downgrade base && uv run alembic upgrade head`
2. Start frontend-next: `cd frontend-next && npm run dev`
3. Navigate to `http://localhost:3000/games` (a protected route)
4. Verify redirect to `/setup`
5. Complete setup, verify redirect to `/login`
6. Login, verify access to `/games`

**Step 4: Final commit if any fixes needed**

```bash
git add .
git commit -m "fix(frontend-next): address any issues from full test suite"
```

---

### Task 5: Create Beads Issue for Tracking

**Step 1: Create the beads issue**

Run:
```bash
bd create --title="Add setup status check to RouteGuard in frontend-next" --type=feature --priority=2
```

**Step 2: Close the issue when complete**

```bash
bd close <issue-id>
bd sync
```

---

## Verification Checklist

- [ ] `useSetupStatus` hook created with tests
- [ ] RouteGuard checks setup status before auth
- [ ] RouteGuard redirects to `/setup` when `needs_setup: true`
- [ ] RouteGuard falls through to login redirect when setup check fails
- [ ] All existing RouteGuard tests still pass
- [ ] Type check passes
- [ ] Full test suite passes
- [ ] Manual verification of setup flow works

---

## Notes

- The setup page (`/setup`) already handles its own setup status check and redirects to `/login` if setup is not needed
- The login page does NOT need setup checking because users can't login if there are no users (the backend will return 401)
- Setup status check only needs to happen in the RouteGuard (protected routes), not in public routes like `/login` or `/setup`
- The hook `useSetupStatus` is created for potential future use but the RouteGuard uses the API directly to avoid an extra state layer
