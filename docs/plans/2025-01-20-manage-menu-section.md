# Manage Menu Section Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a new collapsible "Manage" menu section that groups Import/Export, Sync, Review, Jobs, and Tags, with auto-expand behavior when items need user attention.

**Architecture:** Add a new `manageSection` to navigation config, enhance `NavSection` type with `needsAttention` property, create a `useJobsSummary` hook for badge counts, and update `NavSectionCollapsible` to auto-expand based on attention state.

**Tech Stack:** React, TypeScript, TanStack Query, Lucide icons

---

## Task 1: Add JobsSummary type

**Files:**
- Modify: `frontend/src/types/jobs.ts`

**Step 1: Add the JobsSummary interface**

Add at the end of the Interfaces section (after `JobConfirmResponse`):

```typescript
export interface JobsSummary {
  runningCount: number;
  failedCount: number;
}
```

**Step 2: Export the new type**

Verify the type is exported (it will be automatically since it's in the types file).

**Step 3: Commit**

```bash
git add frontend/src/types/jobs.ts
git commit -m "feat(types): add JobsSummary interface"
```

---

## Task 2: Add getJobsSummary API function

**Files:**
- Modify: `frontend/src/api/jobs.ts`

**Step 1: Add the API response type**

Add after the existing API response interfaces:

```typescript
interface JobsSummaryApiResponse {
  running_count: number;
  failed_count: number;
}
```

**Step 2: Add the API function**

Add at the end of the API Functions section:

```typescript
/**
 * Get summary counts for jobs (running and failed).
 * This is a lightweight endpoint for sidebar badge display.
 */
export async function getJobsSummary(): Promise<JobsSummary> {
  const response = await api.get<JobsSummaryApiResponse>('/jobs/summary');
  return {
    runningCount: response.running_count,
    failedCount: response.failed_count,
  };
}
```

**Step 3: Add the import**

Add `JobsSummary` to the imports from `@/types`:

```typescript
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobConfirmResponse,
  JobsSummary,  // Add this
  JobType,
  JobSource,
  JobStatus,
  JobPriority,
} from '@/types';
```

**Step 4: Commit**

```bash
git add frontend/src/api/jobs.ts
git commit -m "feat(api): add getJobsSummary function"
```

---

## Task 3: Create useJobsSummary hook

**Files:**
- Modify: `frontend/src/hooks/use-jobs.ts`
- Modify: `frontend/src/hooks/index.ts`

**Step 1: Add the hook to use-jobs.ts**

Add after the existing query hooks (after `useJob`):

```typescript
/**
 * Hook to fetch job summary counts for sidebar badge.
 * Returns counts of running and failed jobs.
 */
export function useJobsSummary() {
  return useQuery({
    queryKey: [...jobsKeys.all, 'summary'] as const,
    queryFn: () => jobsApi.getJobsSummary(),
    refetchInterval: 10000, // Poll every 10 seconds for badge updates
  });
}
```

**Step 2: Export from index.ts**

Add `useJobsSummary` to the exports in `frontend/src/hooks/index.ts`:

```typescript
// Jobs hooks
export {
  jobsKeys,
  useJobs,
  useJob,
  useJobsSummary,  // Add this
  useCancelJob,
  useDeleteJob,
  useConfirmJob,
} from './use-jobs';
```

**Step 3: Commit**

```bash
git add frontend/src/hooks/use-jobs.ts frontend/src/hooks/index.ts
git commit -m "feat(hooks): add useJobsSummary hook"
```

---

## Task 4: Update NavSection type with needsAttention

**Files:**
- Modify: `frontend/src/components/navigation/types.ts`

**Step 1: Add needsAttention property**

Update the `NavSection` interface:

```typescript
export interface NavSection {
  label: string;
  icon: ReactNode;
  items: NavItem[];
  defaultOpen?: boolean;
  needsAttention?: boolean;  // Add this - when true, section auto-expands
}
```

**Step 2: Commit**

```bash
git add frontend/src/components/navigation/types.ts
git commit -m "feat(nav): add needsAttention property to NavSection type"
```

---

## Task 5: Update NavSectionCollapsible to support needsAttention

**Files:**
- Modify: `frontend/src/components/navigation/nav-section.tsx`

**Step 1: Update the component**

Replace the entire file content:

```typescript
// frontend/src/components/navigation/nav-section.tsx
'use client';

import { useState, useEffect } from 'react';
import { ChevronDown } from 'lucide-react';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { cn } from '@/lib/utils';
import { NavLink } from './nav-link';
import type { NavSection } from './types';

interface NavSectionCollapsibleProps extends NavSection {
  onNavigate?: () => void;
}

export function NavSectionCollapsible({
  label,
  icon,
  items,
  defaultOpen = false,
  needsAttention = false,
  onNavigate,
}: NavSectionCollapsibleProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen || needsAttention);

  // Auto-expand when needsAttention becomes true
  useEffect(() => {
    if (needsAttention) {
      setIsOpen(true);
    }
  }, [needsAttention]);

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <CollapsibleTrigger className="flex w-full items-center justify-between px-3 py-2 rounded-md hover:bg-muted transition-colors">
        <span className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          {icon}
          <span>{label}</span>
        </span>
        <ChevronDown
          className={cn(
            'h-4 w-4 text-muted-foreground transition-transform',
            isOpen && 'rotate-180'
          )}
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <ul className="mt-1 ml-4 space-y-1 border-l pl-2">
          {items.map((item) => (
            <li key={item.href}>
              <NavLink {...item} onNavigate={onNavigate} />
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  );
}
```

**Step 2: Commit**

```bash
git add frontend/src/components/navigation/nav-section.tsx
git commit -m "feat(nav): support needsAttention auto-expand in NavSectionCollapsible"
```

---

## Task 6: Restructure navigation items

**Files:**
- Modify: `frontend/src/components/navigation/nav-items.tsx`

**Step 1: Update imports**

Replace the imports:

```typescript
// frontend/src/components/navigation/nav-items.tsx
'use client';

import {
  LayoutDashboard,
  Library,
  Plus,
  ArrowLeftRight,
  RefreshCw,
  ClipboardCheck,
  Settings,
  Tag,
  ClipboardList,
  User,
  Users,
  Layers,
  Shield,
  Boxes,
} from 'lucide-react';
import { useReviewSummary, useJobsSummary } from '@/hooks';
import type { NavItem, NavSection } from './types';
```

**Step 2: Update the useNavItems hook**

Replace the entire hook:

```typescript
export function useNavItems() {
  const { data: reviewSummary } = useReviewSummary();
  const { data: jobsSummary } = useJobsSummary();

  const pendingReviews = reviewSummary?.totalPending ?? 0;
  const runningJobs = jobsSummary?.runningCount ?? 0;
  const failedJobs = jobsSummary?.failedCount ?? 0;
  const jobsBadgeCount = runningJobs + failedJobs;

  // Items needing attention trigger auto-expand
  const manageNeedsAttention = pendingReviews > 0 || failedJobs > 0;

  const mainItems: NavItem[] = [
    {
      href: '/dashboard',
      label: 'Dashboard',
      icon: <LayoutDashboard className="h-4 w-4" />,
    },
    {
      href: '/games',
      label: 'Library',
      icon: <Library className="h-4 w-4" />,
    },
    {
      href: '/games/add',
      label: 'Add Game',
      icon: <Plus className="h-4 w-4" />,
    },
  ];

  const manageSection: NavSection = {
    label: 'Manage',
    icon: <Boxes className="h-4 w-4" />,
    items: [
      {
        href: '/import-export',
        label: 'Import / Export',
        icon: <ArrowLeftRight className="h-4 w-4" />,
      },
      {
        href: '/sync',
        label: 'Sync',
        icon: <RefreshCw className="h-4 w-4" />,
      },
      {
        href: '/review',
        label: 'Review',
        icon: <ClipboardCheck className="h-4 w-4" />,
        badge: pendingReviews,
      },
      {
        href: '/jobs',
        label: 'Jobs',
        icon: <ClipboardList className="h-4 w-4" />,
        badge: jobsBadgeCount,
      },
      {
        href: '/tags',
        label: 'Tags',
        icon: <Tag className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
    needsAttention: manageNeedsAttention,
  };

  const settingsSection: NavSection = {
    label: 'Settings',
    icon: <Settings className="h-4 w-4" />,
    items: [
      {
        href: '/profile',
        label: 'Profile',
        icon: <User className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  const adminSection: NavSection = {
    label: 'Administration',
    icon: <Shield className="h-4 w-4" />,
    items: [
      {
        href: '/admin',
        label: 'Admin Dashboard',
        icon: <LayoutDashboard className="h-4 w-4" />,
      },
      {
        href: '/admin/users',
        label: 'User Management',
        icon: <Users className="h-4 w-4" />,
      },
      {
        href: '/admin/platforms',
        label: 'Platforms',
        icon: <Layers className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  return { mainItems, manageSection, settingsSection, adminSection };
}
```

**Step 3: Commit**

```bash
git add frontend/src/components/navigation/nav-items.tsx
git commit -m "feat(nav): restructure navigation with Manage section"
```

---

## Task 7: Update Sidebar component

**Files:**
- Modify: `frontend/src/components/navigation/sidebar.tsx`

**Step 1: Update the component**

Replace the entire file:

```typescript
'use client';

import Link from 'next/link';
import { LogOut, User, ChevronDown } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useAuth } from '@/providers';
import { useNavItems, NavLink, NavSectionCollapsible } from './index';

export function Sidebar() {
  const { user, logout } = useAuth();
  const { mainItems, manageSection, settingsSection, adminSection } = useNavItems();

  return (
    <aside className="hidden md:flex w-64 bg-card border-r flex-col h-screen">
      {/* Logo */}
      <div className="p-4 border-b">
        <Link href="/games" className="block">
          <h1 className="text-xl font-bold">Nexorious</h1>
        </Link>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-4 overflow-y-auto">
        {/* Main navigation items */}
        <ul className="space-y-1">
          {mainItems.map((item) => (
            <li key={item.href}>
              <NavLink {...item} />
            </li>
          ))}
        </ul>

        {/* Manage section */}
        <div className="mt-6">
          <NavSectionCollapsible {...manageSection} />
        </div>

        {/* Settings section */}
        <div className="mt-4">
          <NavSectionCollapsible {...settingsSection} />
        </div>

        {/* Admin section (admin only) */}
        {user?.isAdmin && (
          <div className="mt-4">
            <NavSectionCollapsible {...adminSection} />
          </div>
        )}
      </nav>

      {/* User menu at bottom */}
      <div className="p-4 border-t">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" className="w-full justify-between">
              <span className="flex items-center gap-2">
                <User className="h-4 w-4" />
                <span className="truncate">{user?.username}</span>
              </span>
              <ChevronDown className="h-4 w-4 opacity-50" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-56">
            <DropdownMenuItem asChild className="cursor-pointer">
              <Link href="/profile">
                <User className="mr-2 h-4 w-4" />
                <span>Profile</span>
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={logout} className="cursor-pointer">
              <LogOut className="mr-2 h-4 w-4" />
              <span>Log out</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </aside>
  );
}
```

**Step 2: Commit**

```bash
git add frontend/src/components/navigation/sidebar.tsx
git commit -m "feat(nav): add Manage section to Sidebar"
```

---

## Task 8: Update MobileNav component

**Files:**
- Modify: `frontend/src/components/navigation/mobile-nav.tsx`

**Step 1: Update the component**

Update the destructuring and add manageSection:

Find:
```typescript
const { mainItems, settingsSection, adminSection } = useNavItems();
```

Replace with:
```typescript
const { mainItems, manageSection, settingsSection, adminSection } = useNavItems();
```

**Step 2: Add Manage section to the nav**

Find the Settings section block:
```typescript
{/* Settings section */}
<div className="mt-6">
  <NavSectionCollapsible
    {...settingsSection}
    onNavigate={handleNavigate}
  />
</div>
```

Replace with:
```typescript
{/* Manage section */}
<div className="mt-6">
  <NavSectionCollapsible
    {...manageSection}
    onNavigate={handleNavigate}
  />
</div>

{/* Settings section */}
<div className="mt-4">
  <NavSectionCollapsible
    {...settingsSection}
    onNavigate={handleNavigate}
  />
</div>
```

**Step 3: Commit**

```bash
git add frontend/src/components/navigation/mobile-nav.tsx
git commit -m "feat(nav): add Manage section to MobileNav"
```

---

## Task 9: Add backend jobs summary endpoint

**Files:**
- Modify: `backend/app/api/import_api/core.py`

**Step 1: Add the response schema**

Add after the existing response schemas:

```python
class JobsSummaryResponse(BaseModel):
    """Response for job summary counts."""
    running_count: int
    failed_count: int
```

**Step 2: Add the endpoint**

Add after the existing `/jobs` endpoint (before `/jobs/{job_id}`):

```python
@router.get("/jobs/summary", response_model=JobsSummaryResponse)
async def get_jobs_summary(
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
) -> JobsSummaryResponse:
    """Get summary counts of running and failed jobs for the current user."""
    from sqlalchemy import func, and_
    from app.models import ImportJob, JobStatus

    # Count running jobs (processing, finalizing)
    running_result = await db.execute(
        select(func.count()).select_from(ImportJob).where(
            and_(
                ImportJob.user_id == current_user.id,
                ImportJob.status.in_([JobStatus.PROCESSING, JobStatus.FINALIZING])
            )
        )
    )
    running_count = running_result.scalar() or 0

    # Count failed jobs
    failed_result = await db.execute(
        select(func.count()).select_from(ImportJob).where(
            and_(
                ImportJob.user_id == current_user.id,
                ImportJob.status == JobStatus.FAILED
            )
        )
    )
    failed_count = failed_result.scalar() or 0

    return JobsSummaryResponse(
        running_count=running_count,
        failed_count=failed_count
    )
```

**Step 3: Commit**

```bash
git add backend/app/api/import_api/core.py
git commit -m "feat(api): add /jobs/summary endpoint"
```

---

## Task 10: Update tests for nav-section

**Files:**
- Modify: `frontend/src/components/navigation/nav-section.test.tsx`

**Step 1: Read and update the existing test file**

First read the file to understand its structure, then add tests for the `needsAttention` prop:

```typescript
describe('needsAttention behavior', () => {
  it('should auto-expand when needsAttention is true', () => {
    render(
      <NavSectionCollapsible
        label="Manage"
        icon={<span>icon</span>}
        items={[{ href: '/test', label: 'Test', icon: <span>i</span> }]}
        defaultOpen={false}
        needsAttention={true}
      />
    );

    // Content should be visible because needsAttention overrides defaultOpen
    expect(screen.getByText('Test')).toBeVisible();
  });

  it('should stay collapsed when needsAttention is false and defaultOpen is false', () => {
    render(
      <NavSectionCollapsible
        label="Manage"
        icon={<span>icon</span>}
        items={[{ href: '/test', label: 'Test', icon: <span>i</span> }]}
        defaultOpen={false}
        needsAttention={false}
      />
    );

    // Content should not be visible
    expect(screen.queryByText('Test')).not.toBeInTheDocument();
  });
});
```

**Step 2: Commit**

```bash
git add frontend/src/components/navigation/nav-section.test.tsx
git commit -m "test(nav): add tests for needsAttention behavior"
```

---

## Task 11: Run type checks and tests

**Step 1: Run frontend type check**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check
```

Expected: No TypeScript errors

**Step 2: Run frontend tests**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run test
```

Expected: All tests pass

**Step 3: Run backend type check**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pyrefly check
```

Expected: No type errors

**Step 4: Run backend tests**

```bash
cd /home/abo/workspace/home/nexorious/backend && uv run pytest
```

Expected: All tests pass

**Step 5: Commit any fixes if needed**

---

## Task 12: Manual testing

**Step 1: Start the development servers**

```bash
# Terminal 1 - Backend
cd /home/abo/workspace/home/nexorious/backend && uv run python -m app.main

# Terminal 2 - Frontend
cd /home/abo/workspace/home/nexorious/frontend && npm run dev
```

**Step 2: Test scenarios**

1. Verify "Manage" section appears collapsed by default (when no pending reviews or failed jobs)
2. Verify "Manage" section contains: Import/Export, Sync, Review, Jobs, Tags
3. Verify "Settings" section only contains: Profile
4. Create a failed job and verify:
   - Jobs badge shows count
   - Manage section auto-expands
5. Add pending review items and verify:
   - Review badge shows count
   - Manage section auto-expands (if not already)
6. Verify running jobs show in badge but don't trigger auto-expand
7. Test mobile nav has same behavior

**Step 3: Final commit**

```bash
git add -A
git commit -m "feat(nav): complete Manage menu section implementation"
```
