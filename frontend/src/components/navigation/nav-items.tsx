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
  User,
  Users,
  Layers,
  Shield,
  Boxes,
  Wrench,
} from 'lucide-react';
import { usePendingReviewCount, useJobsSummary } from '@/hooks';
import type { NavItem, NavSection } from './types';

export function useNavItems() {
  const { data: pendingReviewData } = usePendingReviewCount();
  const { data: jobsSummary } = useJobsSummary();

  const pendingReviews = pendingReviewData?.pendingReviewCount ?? 0;
  const failedJobs = jobsSummary?.failedCount ?? 0;

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
      {
        href: '/admin/maintenance',
        label: 'Maintenance',
        icon: <Wrench className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  return { mainItems, manageSection, settingsSection, adminSection };
}
