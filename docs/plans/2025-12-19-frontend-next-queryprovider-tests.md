# QueryProvider Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add comprehensive tests for the QueryProvider component in frontend-next.

**Architecture:** Test the QueryProvider wrapper component that provides TanStack Query context to the application.

**Tech Stack:** React 19, TanStack Query v5, Vitest, @testing-library/react

**Related Issues:** nexorious-4z7f

---

## Overview

The `QueryProvider` component (`frontend-next/src/providers/query-provider.tsx`) wraps the application with TanStack Query's `QueryClientProvider`. Tests should verify:
1. Provider renders children correctly
2. QueryClient is configured properly
3. Children can access query context

---

## Task 1: Create QueryProvider Tests

**Files:**
- Create: `frontend-next/src/providers/__tests__/query-provider.test.tsx`

**Step 1: Set up test file**

```typescript
import { describe, it, expect } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { useQuery } from '@tanstack/react-query';
import { QueryProvider } from '../query-provider';

describe('QueryProvider', () => {
  it('renders children correctly', () => {
    render(
      <QueryProvider>
        <div data-testid="child">Child Content</div>
      </QueryProvider>
    );

    expect(screen.getByTestId('child')).toBeInTheDocument();
    expect(screen.getByText('Child Content')).toBeInTheDocument();
  });

  it('provides query context to children', async () => {
    function TestComponent() {
      const { data, isLoading } = useQuery({
        queryKey: ['test'],
        queryFn: async () => 'test data',
      });

      if (isLoading) return <div>Loading...</div>;
      return <div data-testid="data">{data}</div>;
    }

    render(
      <QueryProvider>
        <TestComponent />
      </QueryProvider>
    );

    // Initially shows loading
    expect(screen.getByText('Loading...')).toBeInTheDocument();

    // Eventually shows data
    await waitFor(() => {
      expect(screen.getByTestId('data')).toHaveTextContent('test data');
    });
  });

  it('allows multiple components to share query cache', async () => {
    let fetchCount = 0;

    function ComponentA() {
      const { data } = useQuery({
        queryKey: ['shared'],
        queryFn: async () => {
          fetchCount++;
          return 'shared data';
        },
        staleTime: 10000,
      });
      return <div data-testid="component-a">{data}</div>;
    }

    function ComponentB() {
      const { data } = useQuery({
        queryKey: ['shared'],
        queryFn: async () => {
          fetchCount++;
          return 'shared data';
        },
        staleTime: 10000,
      });
      return <div data-testid="component-b">{data}</div>;
    }

    render(
      <QueryProvider>
        <ComponentA />
        <ComponentB />
      </QueryProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('component-a')).toHaveTextContent('shared data');
      expect(screen.getByTestId('component-b')).toHaveTextContent('shared data');
    });

    // Should only fetch once due to shared cache
    expect(fetchCount).toBe(1);
  });

  it('handles query errors gracefully', async () => {
    function TestComponent() {
      const { error, isError } = useQuery({
        queryKey: ['error-test'],
        queryFn: async () => {
          throw new Error('Test error');
        },
        retry: false,
      });

      if (isError) return <div data-testid="error">{error.message}</div>;
      return <div>Loading...</div>;
    }

    render(
      <QueryProvider>
        <TestComponent />
      </QueryProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('error')).toHaveTextContent('Test error');
    });
  });
});
```

**Verification:**
```bash
cd /home/abo/workspace/home/nexorious/frontend-next && npm run test -- src/providers/__tests__/query-provider.test.tsx
```

---

## Acceptance Criteria

- [ ] Test file created at `frontend-next/src/providers/__tests__/query-provider.test.tsx`
- [ ] Tests verify children render correctly
- [ ] Tests verify query context is available
- [ ] Tests verify cache sharing works
- [ ] Tests verify error handling
- [ ] All tests pass: `npm run test`
