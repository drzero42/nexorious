# Platform/Storefront Mapping UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the frontend UI for mapping Darkadia CSV platform/storefront strings to system entities during import.

**Architecture:** A new `/import/mapping` route that intercepts the import flow when unresolved platform/storefront strings exist. The page fetches platform summary data, displays unresolved strings with dropdowns for selection, and stores mappings in React context for use during finalization.

**Tech Stack:** Next.js 16, React 19, TanStack Query, shadcn/ui components, TypeScript

---

## Existing Infrastructure (No Changes Needed)

The following backend and frontend code already exists and is fully functional:

**Backend:**
- `GET /review/platform-summary?job_id={id}` - Returns unique platform/storefront strings with suggestions
- `POST /review/finalize` - Applies mappings and creates UserGame records

**Frontend API/Hooks:**
- `getPlatformSummary(jobId)` in `frontend/src/api/review.ts`
- `usePlatformSummary(jobId)` in `frontend/src/hooks/use-review.ts`
- `finalizeImport()` in `frontend/src/api/review.ts`
- `useFinalizeImport()` in `frontend/src/hooks/use-review.ts`
- `useAllPlatforms()` and `useAllStorefronts()` in `frontend/src/hooks/use-platforms.ts`

**Types:**
- `PlatformSummaryResponse`, `PlatformMappingSuggestion` in `frontend/src/types/review.ts`
- `Platform`, `Storefront` in `frontend/src/types/platform.ts`

---

## Task 1: Create Import Mapping Context

**Files:**
- Create: `frontend/src/contexts/import-mapping-context.tsx`
- Test: `frontend/src/contexts/import-mapping-context.test.tsx`

**Step 1: Write the failing test**

```typescript
// frontend/src/contexts/import-mapping-context.test.tsx
import { renderHook, act } from '@testing-library/react';
import { ImportMappingProvider, useImportMapping } from './import-mapping-context';

describe('ImportMappingContext', () => {
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <ImportMappingProvider>{children}</ImportMappingProvider>
  );

  it('should provide empty mappings initially', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    expect(result.current.platformMappings).toEqual({});
    expect(result.current.storefrontMappings).toEqual({});
    expect(result.current.jobId).toBeNull();
  });

  it('should set job ID', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setJobId('test-job-123');
    });

    expect(result.current.jobId).toBe('test-job-123');
  });

  it('should set platform mapping', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setPlatformMapping('PC', 'pc-windows');
    });

    expect(result.current.platformMappings).toEqual({ PC: 'pc-windows' });
  });

  it('should set storefront mapping', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setStorefrontMapping('Steam', 'steam');
    });

    expect(result.current.storefrontMappings).toEqual({ Steam: 'steam' });
  });

  it('should clear all mappings', () => {
    const { result } = renderHook(() => useImportMapping(), { wrapper });

    act(() => {
      result.current.setJobId('test-job');
      result.current.setPlatformMapping('PC', 'pc-windows');
      result.current.setStorefrontMapping('Steam', 'steam');
    });

    act(() => {
      result.current.clearMappings();
    });

    expect(result.current.jobId).toBeNull();
    expect(result.current.platformMappings).toEqual({});
    expect(result.current.storefrontMappings).toEqual({});
  });

  it('should throw error when used outside provider', () => {
    // Suppress console.error for this test
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => {
      renderHook(() => useImportMapping());
    }).toThrow('useImportMapping must be used within an ImportMappingProvider');

    consoleSpy.mockRestore();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/contexts/import-mapping-context.test.tsx`
Expected: FAIL with "Cannot find module"

**Step 3: Write minimal implementation**

```typescript
// frontend/src/contexts/import-mapping-context.tsx
'use client';

import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

interface ImportMappingContextValue {
  jobId: string | null;
  platformMappings: Record<string, string>;
  storefrontMappings: Record<string, string>;
  setJobId: (jobId: string | null) => void;
  setPlatformMapping: (original: string, resolvedId: string) => void;
  setStorefrontMapping: (original: string, resolvedId: string) => void;
  clearMappings: () => void;
}

const ImportMappingContext = createContext<ImportMappingContextValue | null>(null);

export function ImportMappingProvider({ children }: { children: ReactNode }) {
  const [jobId, setJobId] = useState<string | null>(null);
  const [platformMappings, setPlatformMappings] = useState<Record<string, string>>({});
  const [storefrontMappings, setStorefrontMappings] = useState<Record<string, string>>({});

  const setPlatformMapping = useCallback((original: string, resolvedId: string) => {
    setPlatformMappings((prev) => ({ ...prev, [original]: resolvedId }));
  }, []);

  const setStorefrontMapping = useCallback((original: string, resolvedId: string) => {
    setStorefrontMappings((prev) => ({ ...prev, [original]: resolvedId }));
  }, []);

  const clearMappings = useCallback(() => {
    setJobId(null);
    setPlatformMappings({});
    setStorefrontMappings({});
  }, []);

  return (
    <ImportMappingContext.Provider
      value={{
        jobId,
        platformMappings,
        storefrontMappings,
        setJobId,
        setPlatformMapping,
        setStorefrontMapping,
        clearMappings,
      }}
    >
      {children}
    </ImportMappingContext.Provider>
  );
}

export function useImportMapping(): ImportMappingContextValue {
  const context = useContext(ImportMappingContext);
  if (!context) {
    throw new Error('useImportMapping must be used within an ImportMappingProvider');
  }
  return context;
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/contexts/import-mapping-context.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/contexts/import-mapping-context.tsx frontend/src/contexts/import-mapping-context.test.tsx
git commit -m "$(cat <<'EOF'
feat(import): add ImportMappingContext for platform/storefront mappings

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Create MappingSection Component

**Files:**
- Create: `frontend/src/components/import/mapping-section.tsx`
- Test: `frontend/src/components/import/mapping-section.test.tsx`

**Step 1: Write the failing test**

```typescript
// frontend/src/components/import/mapping-section.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MappingSection } from './mapping-section';
import type { PlatformMappingSuggestion } from '@/types';

describe('MappingSection', () => {
  const mockItems: PlatformMappingSuggestion[] = [
    { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    { original: 'PS4', count: 8, suggestedId: null, suggestedName: null },
  ];

  const mockOptions = [
    { id: 'pc-windows', display_name: 'PC (Windows)' },
    { id: 'pc-linux', display_name: 'PC (Linux)' },
    { id: 'playstation-4', display_name: 'PlayStation 4' },
  ];

  it('should render section title and count', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('(1 unresolved)')).toBeInTheDocument();
  });

  it('should not render when no unresolved items', () => {
    const resolvedItems: PlatformMappingSuggestion[] = [
      { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    ];

    const { container } = render(
      <MappingSection
        title="Platforms"
        items={resolvedItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('should display unresolved items with dropdowns', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={vi.fn()}
      />
    );

    // Only unresolved item (PS4) should be shown
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
    expect(screen.getByText('8 games')).toBeInTheDocument();
    expect(screen.queryByText('"PC"')).not.toBeInTheDocument();
  });

  it('should call onMappingChange when selection is made', async () => {
    const user = userEvent.setup();
    const onMappingChange = vi.fn();

    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{}}
        onMappingChange={onMappingChange}
      />
    );

    // Click the select trigger
    await user.click(screen.getByRole('combobox'));

    // Select an option
    await user.click(screen.getByText('PlayStation 4'));

    expect(onMappingChange).toHaveBeenCalledWith('PS4', 'playstation-4');
  });

  it('should show selected value in dropdown', () => {
    render(
      <MappingSection
        title="Platforms"
        items={mockItems}
        options={mockOptions}
        mappings={{ PS4: 'playstation-4' }}
        onMappingChange={vi.fn()}
      />
    );

    expect(screen.getByText('PlayStation 4')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/components/import/mapping-section.test.tsx`
Expected: FAIL with "Cannot find module"

**Step 3: Write minimal implementation**

```typescript
// frontend/src/components/import/mapping-section.tsx
'use client';

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { PlatformMappingSuggestion } from '@/types';

interface MappingSectionProps {
  title: string;
  items: PlatformMappingSuggestion[];
  options: { id: string; display_name: string }[];
  mappings: Record<string, string>;
  onMappingChange: (original: string, resolvedId: string) => void;
}

export function MappingSection({
  title,
  items,
  options,
  mappings,
  onMappingChange,
}: MappingSectionProps) {
  // Filter to only show unresolved items (no suggestedId)
  const unresolvedItems = items.filter((item) => !item.suggestedId);

  // Don't render if no unresolved items
  if (unresolvedItems.length === 0) {
    return null;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h3 className="text-lg font-semibold">{title}</h3>
        <span className="text-sm text-muted-foreground">
          ({unresolvedItems.length} unresolved)
        </span>
      </div>

      <div className="space-y-3">
        {unresolvedItems.map((item) => (
          <div
            key={item.original}
            className="flex items-center justify-between gap-4 rounded-lg border p-4"
          >
            <div className="flex-1">
              <div className="font-medium">&quot;{item.original}&quot;</div>
              <div className="text-sm text-muted-foreground">
                {item.count} {item.count === 1 ? 'game' : 'games'}
              </div>
            </div>

            <Select
              value={mappings[item.original] || ''}
              onValueChange={(value) => onMappingChange(item.original, value)}
            >
              <SelectTrigger className="w-[250px]">
                <SelectValue placeholder={`Select ${title.toLowerCase().slice(0, -1)}`} />
              </SelectTrigger>
              <SelectContent>
                {options.map((option) => (
                  <SelectItem key={option.id} value={option.id}>
                    {option.display_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ))}
      </div>
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/components/import/mapping-section.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/components/import/mapping-section.tsx frontend/src/components/import/mapping-section.test.tsx
git commit -m "$(cat <<'EOF'
feat(import): add MappingSection component for platform/storefront selection

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Create Mapping Page

**Files:**
- Create: `frontend/src/app/(main)/import/mapping/page.tsx`
- Test: `frontend/src/app/(main)/import/mapping/page.test.tsx`

**Step 1: Write the failing test**

```typescript
// frontend/src/app/(main)/import/mapping/page.test.tsx
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useRouter, useSearchParams } from 'next/navigation';
import MappingPage from './page';
import { usePlatformSummary, useAllPlatforms, useAllStorefronts } from '@/hooks';
import { ImportMappingProvider } from '@/contexts/import-mapping-context';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useRouter: vi.fn(),
  useSearchParams: vi.fn(),
}));

// Mock hooks
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual('@/hooks');
  return {
    ...actual,
    usePlatformSummary: vi.fn(),
    useAllPlatforms: vi.fn(),
    useAllStorefronts: vi.fn(),
  };
});

const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
};

const mockPlatformSummary = {
  platforms: [
    { original: 'PC', count: 15, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' },
    { original: 'PS4', count: 8, suggestedId: null, suggestedName: null },
  ],
  storefronts: [
    { original: 'Steam', count: 10, suggestedId: 'steam', suggestedName: 'Steam' },
    { original: 'Epic', count: 5, suggestedId: null, suggestedName: null },
  ],
  allResolved: false,
};

const mockPlatforms = [
  { id: 'pc-windows', display_name: 'PC (Windows)' },
  { id: 'playstation-4', display_name: 'PlayStation 4' },
];

const mockStorefronts = [
  { id: 'steam', display_name: 'Steam' },
  { id: 'epic-games-store', display_name: 'Epic Games Store' },
];

describe('MappingPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (useRouter as ReturnType<typeof vi.fn>).mockReturnValue(mockRouter);
    (useSearchParams as ReturnType<typeof vi.fn>).mockReturnValue(
      new URLSearchParams('job_id=test-job-123')
    );
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockPlatformSummary,
      isLoading: false,
      error: null,
    });
    (useAllPlatforms as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockPlatforms,
      isLoading: false,
    });
    (useAllStorefronts as ReturnType<typeof vi.fn>).mockReturnValue({
      data: mockStorefronts,
      isLoading: false,
    });
  });

  const renderWithProvider = () => {
    return render(
      <ImportMappingProvider>
        <MappingPage />
      </ImportMappingProvider>
    );
  };

  it('should redirect to review when all resolved', async () => {
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { ...mockPlatformSummary, allResolved: true },
      isLoading: false,
      error: null,
    });

    renderWithProvider();

    await waitFor(() => {
      expect(mockRouter.replace).toHaveBeenCalledWith('/review?job_id=test-job-123');
    });
  });

  it('should display page title and description', () => {
    renderWithProvider();

    expect(screen.getByText('Platform & Storefront Mapping')).toBeInTheDocument();
    expect(
      screen.getByText(/Some values from your CSV need to be mapped/)
    ).toBeInTheDocument();
  });

  it('should display unresolved platform section', () => {
    renderWithProvider();

    expect(screen.getByText('Platforms')).toBeInTheDocument();
    expect(screen.getByText('"PS4"')).toBeInTheDocument();
  });

  it('should display unresolved storefront section', () => {
    renderWithProvider();

    expect(screen.getByText('Storefronts')).toBeInTheDocument();
    expect(screen.getByText('"Epic"')).toBeInTheDocument();
  });

  it('should disable continue button when not all mapped', () => {
    renderWithProvider();

    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    expect(continueButton).toBeDisabled();
  });

  it('should enable continue button when all mapped', async () => {
    const user = userEvent.setup();
    renderWithProvider();

    // Select platform mapping
    const platformSelect = screen.getAllByRole('combobox')[0];
    await user.click(platformSelect);
    await user.click(screen.getByText('PlayStation 4'));

    // Select storefront mapping
    const storefrontSelect = screen.getAllByRole('combobox')[1];
    await user.click(storefrontSelect);
    await user.click(screen.getByText('Epic Games Store'));

    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    expect(continueButton).toBeEnabled();
  });

  it('should navigate to review on continue', async () => {
    const user = userEvent.setup();
    renderWithProvider();

    // Select platform mapping
    const platformSelect = screen.getAllByRole('combobox')[0];
    await user.click(platformSelect);
    await user.click(screen.getByText('PlayStation 4'));

    // Select storefront mapping
    const storefrontSelect = screen.getAllByRole('combobox')[1];
    await user.click(storefrontSelect);
    await user.click(screen.getByText('Epic Games Store'));

    // Click continue
    const continueButton = screen.getByRole('button', { name: /continue to review/i });
    await user.click(continueButton);

    expect(mockRouter.push).toHaveBeenCalledWith('/review?job_id=test-job-123');
  });

  it('should show loading state', () => {
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: null,
      isLoading: true,
      error: null,
    });

    renderWithProvider();

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('should show error state', () => {
    (usePlatformSummary as ReturnType<typeof vi.fn>).mockReturnValue({
      data: null,
      isLoading: false,
      error: new Error('Failed to load'),
    });

    renderWithProvider();

    expect(screen.getByText(/error/i)).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/app/\\(main\\)/import/mapping/page.test.tsx`
Expected: FAIL with "Cannot find module"

**Step 3: Write minimal implementation**

```typescript
// frontend/src/app/(main)/import/mapping/page.tsx
'use client';

import { useEffect, useMemo } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Skeleton } from '@/components/ui/skeleton';
import { AlertCircle, ArrowRight, Loader2 } from 'lucide-react';
import { MappingSection } from '@/components/import/mapping-section';
import { useImportMapping } from '@/contexts/import-mapping-context';
import { usePlatformSummary, useAllPlatforms, useAllStorefronts } from '@/hooks';

export default function MappingPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const jobId = searchParams.get('job_id');

  const {
    platformMappings,
    storefrontMappings,
    setJobId,
    setPlatformMapping,
    setStorefrontMapping,
  } = useImportMapping();

  const { data: summary, isLoading: summaryLoading, error: summaryError } = usePlatformSummary(jobId);
  const { data: platforms, isLoading: platformsLoading } = useAllPlatforms({ activeOnly: true });
  const { data: storefronts, isLoading: storefrontsLoading } = useAllStorefronts({ activeOnly: true });

  const isLoading = summaryLoading || platformsLoading || storefrontsLoading;

  // Set job ID in context when page loads
  useEffect(() => {
    if (jobId) {
      setJobId(jobId);
    }
  }, [jobId, setJobId]);

  // Redirect to review if all resolved
  useEffect(() => {
    if (summary?.allResolved && jobId) {
      router.replace(`/review?job_id=${jobId}`);
    }
  }, [summary?.allResolved, jobId, router]);

  // Get unresolved items
  const unresolvedPlatforms = useMemo(
    () => summary?.platforms.filter((p) => !p.suggestedId) || [],
    [summary?.platforms]
  );
  const unresolvedStorefronts = useMemo(
    () => summary?.storefronts.filter((s) => !s.suggestedId) || [],
    [summary?.storefronts]
  );

  // Check if all unresolved items have mappings
  const allMapped = useMemo(() => {
    const platformsMapped = unresolvedPlatforms.every(
      (p) => platformMappings[p.original]
    );
    const storefrontsMapped = unresolvedStorefronts.every(
      (s) => storefrontMappings[s.original]
    );
    return platformsMapped && storefrontsMapped;
  }, [unresolvedPlatforms, unresolvedStorefronts, platformMappings, storefrontMappings]);

  const handleContinue = () => {
    if (jobId) {
      router.push(`/review?job_id=${jobId}`);
    }
  };

  if (!jobId) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>No job ID provided. Please start an import first.</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div>
          <Skeleton className="mb-2 h-8 w-64" />
          <Skeleton className="h-4 w-96" />
        </div>
        <Card>
          <CardContent className="space-y-4 p-6">
            <Skeleton className="h-6 w-32" />
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      </div>
    );
  }

  if (summaryError) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>Error</AlertTitle>
        <AlertDescription>
          {summaryError instanceof Error ? summaryError.message : 'Failed to load platform summary'}
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <nav className="mb-2 flex items-center text-sm text-muted-foreground">
          <Link href="/dashboard" className="hover:text-foreground">
            Dashboard
          </Link>
          <span className="mx-2">/</span>
          <Link href="/import-export" className="hover:text-foreground">
            Import / Export
          </Link>
          <span className="mx-2">/</span>
          <span className="text-foreground">Platform Mapping</span>
        </nav>
        <h1 className="text-2xl font-bold">Platform & Storefront Mapping</h1>
        <p className="text-muted-foreground">
          Some values from your CSV need to be mapped to our system. Please select the correct
          mapping for each unrecognized value below.
        </p>
      </div>

      {/* Mapping Sections */}
      <Card>
        <CardHeader>
          <CardTitle>Unresolved Mappings</CardTitle>
        </CardHeader>
        <CardContent className="space-y-8">
          {summary && platforms && (
            <MappingSection
              title="Platforms"
              items={summary.platforms}
              options={platforms.map((p) => ({ id: p.id, display_name: p.display_name }))}
              mappings={platformMappings}
              onMappingChange={setPlatformMapping}
            />
          )}

          {summary && storefronts && (
            <MappingSection
              title="Storefronts"
              items={summary.storefronts}
              options={storefronts.map((s) => ({ id: s.id, display_name: s.display_name }))}
              mappings={storefrontMappings}
              onMappingChange={setStorefrontMapping}
            />
          )}

          {unresolvedPlatforms.length === 0 && unresolvedStorefronts.length === 0 && (
            <p className="text-center text-muted-foreground">
              All platforms and storefronts have been automatically matched.
            </p>
          )}
        </CardContent>
      </Card>

      {/* Continue Button */}
      <div className="flex justify-end">
        <Button onClick={handleContinue} disabled={!allMapped} size="lg">
          Continue to Review
          <ArrowRight className="ml-2 h-4 w-4" />
        </Button>
      </div>

      {!allMapped && (
        <p className="text-right text-sm text-muted-foreground">
          Please map all unresolved values before continuing.
        </p>
      )}
    </div>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/app/\\(main\\)/import/mapping/page.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/app/\\(main\\)/import/mapping/page.tsx frontend/src/app/\\(main\\)/import/mapping/page.test.tsx
git commit -m "$(cat <<'EOF'
feat(import): add platform/storefront mapping page

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add Provider to Layout

**Files:**
- Modify: `frontend/src/app/(main)/layout.tsx`

**Step 1: Read existing layout file**

Run: Read `frontend/src/app/(main)/layout.tsx`

**Step 2: Add ImportMappingProvider wrap**

Wrap the children with `ImportMappingProvider`:

```typescript
import { ImportMappingProvider } from '@/contexts/import-mapping-context';

// In the return statement, wrap children:
<ImportMappingProvider>
  {children}
</ImportMappingProvider>
```

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/app/\\(main\\)/layout.tsx
git commit -m "$(cat <<'EOF'
feat(import): add ImportMappingProvider to main layout

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Update Import Page Navigation

**Files:**
- Modify: `frontend/src/app/(main)/import-export/page.tsx:206`

**Step 1: Update Darkadia import navigation**

Change the router.push after successful Darkadia import to navigate to mapping page instead of jobs page:

```typescript
// Before (line 206):
router.push(`/jobs/${result.job_id}`);

// After:
if (source === ImportSource.DARKADIA) {
  router.push(`/import/mapping?job_id=${result.job_id}`);
} else {
  router.push(`/jobs/${result.job_id}`);
}
```

**Step 2: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS

**Step 3: Run existing tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- src/app/\\(main\\)/import-export/page.test.tsx`
Expected: PASS (update test if needed)

**Step 4: Commit**

```bash
git add frontend/src/app/\\(main\\)/import-export/page.tsx
git commit -m "$(cat <<'EOF'
feat(import): route Darkadia imports to mapping page

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Update Review Page to Use Mappings for Finalization

**Files:**
- Modify: `frontend/src/app/(main)/review/page.tsx`

**Step 1: Import and use context**

Add import:
```typescript
import { useImportMapping } from '@/contexts/import-mapping-context';
```

In the component:
```typescript
const { platformMappings, storefrontMappings, jobId: contextJobId, clearMappings } = useImportMapping();
```

**Step 2: Add finalization functionality**

Add a "Finalize Import" button that appears when:
1. There's a job_id in the URL or context
2. There are platform/storefront mappings in context

When clicked, call `useFinalizeImport` with the mappings.

**Step 3: Run type check and tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test -- src/app/\\(main\\)/review/page.test.tsx`
Expected: PASS

**Step 4: Commit**

```bash
git add frontend/src/app/\\(main\\)/review/page.tsx
git commit -m "$(cat <<'EOF'
feat(import): integrate finalization with platform mappings in review page

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Export Context from Contexts Index

**Files:**
- Create or Modify: `frontend/src/contexts/index.ts`

**Step 1: Create/update index file**

```typescript
// frontend/src/contexts/index.ts
export { ImportMappingProvider, useImportMapping } from './import-mapping-context';
```

**Step 2: Update imports in components to use index**

Update any imports of the context to use `@/contexts` instead of full path.

**Step 3: Commit**

```bash
git add frontend/src/contexts/index.ts
git commit -m "$(cat <<'EOF'
chore(import): export context from index file

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Run Full Test Suite and Type Checks

**Step 1: Run all frontend checks**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`
Expected: PASS with 0 TypeScript errors

**Step 2: Run all frontend tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`
Expected: PASS with >70% coverage

**Step 3: Fix any issues**

If tests fail, fix the issues and create additional commits.

**Step 4: Final commit for any fixes**

```bash
git add -A
git commit -m "$(cat <<'EOF'
fix(import): address test and type check issues

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

This plan implements the platform/storefront mapping UI feature with:

1. **ImportMappingContext** - React context to store user's mapping selections
2. **MappingSection** - Reusable component for displaying/selecting mappings
3. **MappingPage** - New page at `/import/mapping` for the mapping UI
4. **Navigation updates** - Route Darkadia imports through mapping page
5. **Review integration** - Use stored mappings during finalization

All backend APIs are already implemented (`GET /review/platform-summary`, `POST /review/finalize`).
All frontend API functions and hooks are already implemented (`usePlatformSummary`, `useFinalizeImport`).
