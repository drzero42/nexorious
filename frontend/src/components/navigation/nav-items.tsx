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
    {
      href: '/review',
      label: 'Review',
      icon: <ClipboardCheck className="h-4 w-4" />,
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

  return { mainItems, settingsSection, adminSection };
}
