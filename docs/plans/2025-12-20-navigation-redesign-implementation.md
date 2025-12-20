# Navigation Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the navigation redesign by adding badge indicators, consolidating nav items under a Settings section, and implementing responsive mobile navigation.

**Architecture:** Extract navigation into shared components used by both desktop sidebar and mobile drawer. Use React Query hook `useReviewCountsByType()` for badge counts. Use shadcn Sheet for mobile drawer and Collapsible for expandable sections.

**Tech Stack:** Next.js 16, React 19, shadcn/ui (Sheet, Collapsible), TanStack Query, Tailwind CSS

---

## Prerequisites

Install required shadcn components before starting:

```bash
cd /home/abo/workspace/home/nexorious/frontend
npx shadcn@latest add sheet collapsible
```

---

## Task 1: Create NavBadge Component

**Files:**
- Create: `frontend/src/components/ui/nav-badge.tsx`
- Test: `frontend/src/components/ui/nav-badge.test.tsx`

**Step 1: Write the test file**

```tsx
// frontend/src/components/ui/nav-badge.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavBadge } from './nav-badge';

describe('NavBadge', () => {
  it('renders count when greater than 0', () => {
    render(<NavBadge count={5} />);
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('renders nothing when count is 0', () => {
    const { container } = render(<NavBadge count={0} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when count is negative', () => {
    const { container } = render(<NavBadge count={-1} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('caps display at 99+', () => {
    render(<NavBadge count={150} />);
    expect(screen.getByText('99+')).toBeInTheDocument();
  });

  it('calls onClick when clicked', async () => {
    const user = userEvent.setup();
    const handleClick = vi.fn();
    render(<NavBadge count={5} onClick={handleClick} />);

    await user.click(screen.getByText('5'));
    expect(handleClick).toHaveBeenCalledTimes(1);
  });

  it('stops event propagation on click', async () => {
    const user = userEvent.setup();
    const parentClick = vi.fn();
    const badgeClick = vi.fn();

    render(
      <div onClick={parentClick}>
        <NavBadge count={5} onClick={badgeClick} />
      </div>
    );

    await user.click(screen.getByText('5'));
    expect(badgeClick).toHaveBeenCalled();
    expect(parentClick).not.toHaveBeenCalled();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-badge.test.tsx`

Expected: FAIL with module not found

**Step 3: Write the component**

```tsx
// frontend/src/components/ui/nav-badge.tsx
import { cn } from '@/lib/utils';

interface NavBadgeProps {
  count: number;
  onClick?: (e: React.MouseEvent) => void;
  className?: string;
}

export function NavBadge({ count, onClick, className }: NavBadgeProps) {
  if (count <= 0) return null;

  const displayCount = count > 99 ? '99+' : count.toString();

  const handleClick = (e: React.MouseEvent) => {
    if (onClick) {
      e.stopPropagation();
      e.preventDefault();
      onClick(e);
    }
  };

  return (
    <span
      onClick={handleClick}
      className={cn(
        'inline-flex items-center justify-center min-w-5 h-5 px-1.5 text-xs font-medium rounded-full',
        'bg-destructive text-destructive-foreground',
        onClick && 'cursor-pointer hover:bg-destructive/90',
        className
      )}
    >
      {displayCount}
    </span>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-badge.test.tsx`

Expected: All 6 tests PASS

**Step 5: Add export to components index**

Check if there's a components/ui/index.ts barrel file. If so, add:
```tsx
export { NavBadge } from './nav-badge';
```

**Step 6: Commit**

```bash
git add frontend/src/components/ui/nav-badge.tsx frontend/src/components/ui/nav-badge.test.tsx
git commit -m "feat(nav): add NavBadge component for navigation indicators"
```

---

## Task 2: Create Navigation Item Types and Data

**Files:**
- Create: `frontend/src/components/navigation/types.ts`
- Create: `frontend/src/components/navigation/nav-items.tsx`

**Step 1: Create types file**

```tsx
// frontend/src/components/navigation/types.ts
import type { ReactNode } from 'react';

export interface NavItem {
  href: string;
  label: string;
  icon: ReactNode;
  badge?: number;
  badgeHref?: string; // Where clicking the badge navigates
}

export interface NavSection {
  label: string;
  icon: ReactNode;
  items: NavItem[];
  defaultOpen?: boolean;
}
```

**Step 2: Create nav items hook**

```tsx
// frontend/src/components/navigation/nav-items.tsx
'use client';

import {
  LayoutDashboard,
  Library,
  Plus,
  ArrowLeftRight,
  RefreshCw,
  Settings,
  Tag,
  ClipboardList,
  User,
  Users,
  Layers,
} from 'lucide-react';
import { useReviewCountsByType } from '@/hooks';
import type { NavItem, NavSection } from './types';

export function useNavItems() {
  const { data: reviewCounts } = useReviewCountsByType();

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
    {
      href: '/import-export',
      label: 'Import / Export',
      icon: <ArrowLeftRight className="h-4 w-4" />,
      badge: reviewCounts?.importPending ?? 0,
      badgeHref: '/review?source=import',
    },
    {
      href: '/sync',
      label: 'Sync',
      icon: <RefreshCw className="h-4 w-4" />,
      badge: reviewCounts?.syncPending ?? 0,
      badgeHref: '/review?source=sync',
    },
  ];

  const settingsSection: NavSection = {
    label: 'Settings',
    icon: <Settings className="h-4 w-4" />,
    items: [
      {
        href: '/tags',
        label: 'Tags',
        icon: <Tag className="h-4 w-4" />,
      },
      {
        href: '/jobs',
        label: 'Jobs',
        icon: <ClipboardList className="h-4 w-4" />,
      },
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
    icon: <Settings className="h-4 w-4" />,
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

  return { mainItems, settingsSection, adminSection };
}
```

**Step 3: Create index file**

```tsx
// frontend/src/components/navigation/index.ts
export { useNavItems } from './nav-items';
export type { NavItem, NavSection } from './types';
```

**Step 4: Commit**

```bash
git add frontend/src/components/navigation/
git commit -m "feat(nav): add navigation types and useNavItems hook"
```

---

## Task 3: Create NavLink Component

**Files:**
- Create: `frontend/src/components/navigation/nav-link.tsx`
- Test: `frontend/src/components/navigation/nav-link.test.tsx`

**Step 1: Write the test file**

```tsx
// frontend/src/components/navigation/nav-link.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavLink } from './nav-link';
import { Library } from 'lucide-react';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: vi.fn(() => '/games'),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));

describe('NavLink', () => {
  const defaultProps = {
    href: '/games',
    label: 'Library',
    icon: <Library className="h-4 w-4" data-testid="icon" />,
  };

  it('renders label and icon', () => {
    render(<NavLink {...defaultProps} />);
    expect(screen.getByText('Library')).toBeInTheDocument();
    expect(screen.getByTestId('icon')).toBeInTheDocument();
  });

  it('renders as a link', () => {
    render(<NavLink {...defaultProps} />);
    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', '/games');
  });

  it('shows badge when count > 0', () => {
    render(<NavLink {...defaultProps} badge={5} />);
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  it('hides badge when count is 0', () => {
    render(<NavLink {...defaultProps} badge={0} />);
    expect(screen.queryByText('0')).not.toBeInTheDocument();
  });

  it('applies active styles when pathname matches href', () => {
    render(<NavLink {...defaultProps} />);
    const link = screen.getByRole('link');
    expect(link).toHaveClass('bg-primary');
  });

  it('calls onNavigate when clicked', async () => {
    const user = userEvent.setup();
    const onNavigate = vi.fn();
    render(<NavLink {...defaultProps} onNavigate={onNavigate} />);

    await user.click(screen.getByRole('link'));
    expect(onNavigate).toHaveBeenCalled();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-link.test.tsx`

Expected: FAIL with module not found

**Step 3: Write the component**

```tsx
// frontend/src/components/navigation/nav-link.tsx
'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import { NavBadge } from '@/components/ui/nav-badge';
import type { NavItem } from './types';

interface NavLinkProps extends NavItem {
  onNavigate?: () => void;
  className?: string;
}

export function NavLink({
  href,
  label,
  icon,
  badge,
  badgeHref,
  onNavigate,
  className,
}: NavLinkProps) {
  const pathname = usePathname();
  const router = useRouter();

  // Check if this link is active
  // Exact match for root paths, prefix match for nested paths
  const isActive =
    href === '/admin'
      ? pathname === '/admin'
      : pathname === href || (pathname.startsWith(href) && href !== '/');

  const handleBadgeClick = (e: React.MouseEvent) => {
    if (badgeHref) {
      e.preventDefault();
      e.stopPropagation();
      router.push(badgeHref);
      onNavigate?.();
    }
  };

  const handleClick = () => {
    onNavigate?.();
  };

  return (
    <Link
      href={href}
      onClick={handleClick}
      className={cn(
        'flex items-center justify-between gap-2 px-3 py-2 rounded-md transition-colors',
        isActive
          ? 'bg-primary text-primary-foreground'
          : 'hover:bg-muted',
        className
      )}
    >
      <span className="flex items-center gap-2">
        {icon}
        <span>{label}</span>
      </span>
      {badge !== undefined && badge > 0 && (
        <NavBadge
          count={badge}
          onClick={badgeHref ? handleBadgeClick : undefined}
        />
      )}
    </Link>
  );
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-link.test.tsx`

Expected: All tests PASS

**Step 5: Add export to index**

```tsx
// Update frontend/src/components/navigation/index.ts
export { useNavItems } from './nav-items';
export { NavLink } from './nav-link';
export type { NavItem, NavSection } from './types';
```

**Step 6: Commit**

```bash
git add frontend/src/components/navigation/
git commit -m "feat(nav): add NavLink component with badge support"
```

---

## Task 4: Create NavSection Component (Collapsible)

**Files:**
- Create: `frontend/src/components/navigation/nav-section.tsx`
- Test: `frontend/src/components/navigation/nav-section.test.tsx`

**Step 1: Write the test file**

```tsx
// frontend/src/components/navigation/nav-section.test.tsx
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { NavSectionCollapsible } from './nav-section';
import { Settings, Tag, User } from 'lucide-react';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: vi.fn(() => '/other'),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));

describe('NavSectionCollapsible', () => {
  const defaultProps = {
    label: 'Settings',
    icon: <Settings className="h-4 w-4" data-testid="section-icon" />,
    items: [
      { href: '/tags', label: 'Tags', icon: <Tag className="h-4 w-4" /> },
      { href: '/profile', label: 'Profile', icon: <User className="h-4 w-4" /> },
    ],
  };

  it('renders section label', () => {
    render(<NavSectionCollapsible {...defaultProps} />);
    expect(screen.getByText('Settings')).toBeInTheDocument();
  });

  it('is collapsed by default', () => {
    render(<NavSectionCollapsible {...defaultProps} />);
    expect(screen.queryByText('Tags')).not.toBeInTheDocument();
  });

  it('expands when clicked', async () => {
    const user = userEvent.setup();
    render(<NavSectionCollapsible {...defaultProps} />);

    await user.click(screen.getByText('Settings'));
    expect(screen.getByText('Tags')).toBeInTheDocument();
    expect(screen.getByText('Profile')).toBeInTheDocument();
  });

  it('collapses when clicked again', async () => {
    const user = userEvent.setup();
    render(<NavSectionCollapsible {...defaultProps} />);

    await user.click(screen.getByText('Settings'));
    expect(screen.getByText('Tags')).toBeInTheDocument();

    await user.click(screen.getByText('Settings'));
    expect(screen.queryByText('Tags')).not.toBeInTheDocument();
  });

  it('respects defaultOpen prop', () => {
    render(<NavSectionCollapsible {...defaultProps} defaultOpen={true} />);
    expect(screen.getByText('Tags')).toBeInTheDocument();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-section.test.tsx`

Expected: FAIL with module not found

**Step 3: Write the component**

```tsx
// frontend/src/components/navigation/nav-section.tsx
'use client';

import { useState } from 'react';
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
  onNavigate,
}: NavSectionCollapsibleProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

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

**Step 4: Run test to verify it passes**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test -- nav-section.test.tsx`

Expected: All tests PASS

**Step 5: Add export to index**

```tsx
// Update frontend/src/components/navigation/index.ts
export { useNavItems } from './nav-items';
export { NavLink } from './nav-link';
export { NavSectionCollapsible } from './nav-section';
export type { NavItem, NavSection } from './types';
```

**Step 6: Commit**

```bash
git add frontend/src/components/navigation/
git commit -m "feat(nav): add collapsible NavSection component"
```

---

## Task 5: Create Desktop Sidebar Component

**Files:**
- Create: `frontend/src/components/navigation/sidebar.tsx`

**Step 1: Write the Sidebar component**

```tsx
// frontend/src/components/navigation/sidebar.tsx
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
  const { mainItems, settingsSection, adminSection } = useNavItems();

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

        {/* Settings section */}
        <div className="mt-6">
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

**Step 2: Add export to index**

```tsx
// Update frontend/src/components/navigation/index.ts
export { useNavItems } from './nav-items';
export { NavLink } from './nav-link';
export { NavSectionCollapsible } from './nav-section';
export { Sidebar } from './sidebar';
export type { NavItem, NavSection } from './types';
```

**Step 3: Commit**

```bash
git add frontend/src/components/navigation/
git commit -m "feat(nav): add desktop Sidebar component"
```

---

## Task 6: Create Mobile Navigation Component

**Files:**
- Create: `frontend/src/components/navigation/mobile-nav.tsx`

**Step 1: Write the MobileNav component**

```tsx
// frontend/src/components/navigation/mobile-nav.tsx
'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Menu, LogOut, User } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { useAuth } from '@/providers';
import { useNavItems, NavLink, NavSectionCollapsible } from './index';

export function MobileNav() {
  const [open, setOpen] = useState(false);
  const { user, logout } = useAuth();
  const { mainItems, settingsSection, adminSection } = useNavItems();

  const handleNavigate = () => {
    setOpen(false);
  };

  const handleLogout = () => {
    setOpen(false);
    logout();
  };

  return (
    <div className="flex md:hidden items-center justify-between p-4 border-b bg-card">
      {/* Hamburger and Logo */}
      <div className="flex items-center gap-3">
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon">
              <Menu className="h-5 w-5" />
              <span className="sr-only">Toggle menu</span>
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-72 p-0">
            <SheetHeader className="p-4 border-b">
              <SheetTitle>
                <Link href="/games" onClick={handleNavigate}>
                  Nexorious
                </Link>
              </SheetTitle>
            </SheetHeader>

            <nav className="flex-1 p-4 overflow-y-auto">
              {/* Main navigation items */}
              <ul className="space-y-1">
                {mainItems.map((item) => (
                  <li key={item.href}>
                    <NavLink {...item} onNavigate={handleNavigate} />
                  </li>
                ))}
              </ul>

              {/* Settings section */}
              <div className="mt-6">
                <NavSectionCollapsible
                  {...settingsSection}
                  onNavigate={handleNavigate}
                />
              </div>

              {/* Admin section (admin only) */}
              {user?.isAdmin && (
                <div className="mt-4">
                  <NavSectionCollapsible
                    {...adminSection}
                    onNavigate={handleNavigate}
                  />
                </div>
              )}

              {/* Account section */}
              <div className="mt-6 pt-4 border-t">
                <p className="px-3 mb-2 text-xs font-semibold uppercase text-muted-foreground">
                  Account
                </p>
                <ul className="space-y-1">
                  <li>
                    <Link
                      href="/profile"
                      onClick={handleNavigate}
                      className="flex items-center gap-2 px-3 py-2 rounded-md hover:bg-muted transition-colors"
                    >
                      <Avatar className="h-6 w-6">
                        <AvatarFallback className="text-xs">
                          {user?.username?.charAt(0).toUpperCase()}
                        </AvatarFallback>
                      </Avatar>
                      <span>{user?.username}</span>
                    </Link>
                  </li>
                  <li>
                    <button
                      onClick={handleLogout}
                      className="flex w-full items-center gap-2 px-3 py-2 rounded-md hover:bg-muted transition-colors text-left"
                    >
                      <LogOut className="h-4 w-4" />
                      <span>Sign out</span>
                    </button>
                  </li>
                </ul>
              </div>
            </nav>
          </SheetContent>
        </Sheet>

        <Link href="/games" className="font-bold text-lg">
          Nexorious
        </Link>
      </div>

      {/* Avatar on right */}
      <Link href="/profile">
        <Avatar className="h-8 w-8">
          <AvatarFallback>
            {user?.username?.charAt(0).toUpperCase()}
          </AvatarFallback>
        </Avatar>
      </Link>
    </div>
  );
}
```

**Step 2: Add export to index**

```tsx
// Update frontend/src/components/navigation/index.ts
export { useNavItems } from './nav-items';
export { NavLink } from './nav-link';
export { NavSectionCollapsible } from './nav-section';
export { Sidebar } from './sidebar';
export { MobileNav } from './mobile-nav';
export type { NavItem, NavSection } from './types';
```

**Step 3: Commit**

```bash
git add frontend/src/components/navigation/
git commit -m "feat(nav): add MobileNav component with Sheet drawer"
```

---

## Task 7: Update Main Layout

**Files:**
- Modify: `frontend/src/app/(main)/layout.tsx`

**Step 1: Replace the layout with new navigation components**

Replace the entire file content:

```tsx
// frontend/src/app/(main)/layout.tsx
'use client';

import { RouteGuard } from '@/components';
import { Sidebar, MobileNav } from '@/components/navigation';

export default function MainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <RouteGuard>
      <div className="flex min-h-screen flex-col md:flex-row">
        {/* Mobile header */}
        <MobileNav />

        {/* Desktop sidebar */}
        <Sidebar />

        {/* Main content */}
        <main className="flex-1 p-6 overflow-auto">{children}</main>
      </div>
    </RouteGuard>
  );
}
```

**Step 2: Verify the app runs**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run dev`

Check in browser:
- Desktop (>768px): Sidebar visible on left, no hamburger
- Mobile (<768px): Hamburger in header, sidebar hidden, sheet opens on click

**Step 3: Run type check**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run check`

Expected: No TypeScript errors

**Step 4: Run all tests**

Run: `cd /home/abo/workspace/home/nexorious/frontend && npm run test`

Expected: All tests pass

**Step 5: Commit**

```bash
git add frontend/src/app/\(main\)/layout.tsx
git commit -m "feat(nav): integrate new navigation components into main layout"
```

---

## Task 8: Add Navigation Components Export to Main Components Index

**Files:**
- Modify: `frontend/src/components/index.ts` (if exists)

**Step 1: Check if components barrel exists and add navigation export**

If `frontend/src/components/index.ts` exists, add:

```tsx
export * from './navigation';
```

**Step 2: Commit (if changes made)**

```bash
git add frontend/src/components/index.ts
git commit -m "chore: export navigation components from components barrel"
```

---

## Task 9: Final Verification and Cleanup

**Step 1: Run full test suite**

```bash
cd /home/abo/workspace/home/nexorious/frontend && npm run check && npm run test
```

Expected: All checks and tests pass

**Step 2: Manual testing checklist**

Test the following in browser:

Desktop (>768px):
- [ ] Sidebar visible, hamburger hidden
- [ ] Dashboard, Library, Add Game, Import/Export, Sync links work
- [ ] Import/Export shows badge when importPending > 0
- [ ] Sync shows badge when syncPending > 0
- [ ] Clicking badge navigates to `/review?source=import` or `/review?source=sync`
- [ ] Settings section collapses/expands
- [ ] Tags, Jobs, Profile accessible under Settings
- [ ] Admin section visible for admin users
- [ ] User dropdown at bottom works (Profile, Log out)

Mobile (<768px):
- [ ] Hamburger visible, sidebar hidden
- [ ] Hamburger opens sheet from left
- [ ] All nav items accessible in sheet
- [ ] Clicking nav item closes sheet
- [ ] Badges visible on Import/Export and Sync
- [ ] Settings and Admin sections expand/collapse
- [ ] Account section at bottom with avatar and sign out
- [ ] Avatar in header links to profile

**Step 3: Commit final state**

If any fixes were needed:

```bash
git add -A
git commit -m "fix(nav): address issues found in manual testing"
```

---

## Summary

After completing all tasks, the navigation will have:

1. **Badge indicators** on Import/Export and Sync showing pending review counts
2. **Collapsible Settings section** containing Tags, Jobs, and Profile
3. **Responsive mobile navigation** with hamburger menu and slide-out sheet
4. **Shared navigation components** used by both desktop and mobile

Files created:
- `frontend/src/components/ui/nav-badge.tsx`
- `frontend/src/components/ui/nav-badge.test.tsx`
- `frontend/src/components/navigation/types.ts`
- `frontend/src/components/navigation/nav-items.tsx`
- `frontend/src/components/navigation/nav-link.tsx`
- `frontend/src/components/navigation/nav-link.test.tsx`
- `frontend/src/components/navigation/nav-section.tsx`
- `frontend/src/components/navigation/nav-section.test.tsx`
- `frontend/src/components/navigation/sidebar.tsx`
- `frontend/src/components/navigation/mobile-nav.tsx`
- `frontend/src/components/navigation/index.ts`

Files modified:
- `frontend/src/app/(main)/layout.tsx`
