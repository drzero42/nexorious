
import {
  LayoutDashboard,
  Library,
  Plus,
  RefreshCw,
  Tag,
  Users,
  Layers,
  Shield,
  Wrench,
  DatabaseBackup,
} from 'lucide-react';
import type { NavItem, NavSection } from './types';
import { usePendingReviewCount } from '@/hooks/use-jobs';

export function useNavItems() {
  const { data: reviewData } = usePendingReviewCount();
  const pendingReviewCount = reviewData?.pendingReviewCount ?? 0;

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
      href: '/sync',
      label: 'Sync',
      icon: <RefreshCw className="h-4 w-4" />,
      badge: pendingReviewCount,
    },
    {
      href: '/tags',
      label: 'Tags',
      icon: <Tag className="h-4 w-4" />,
    },
  ];

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
      {
        href: '/admin/backups',
        label: 'Backup / Restore',
        icon: <DatabaseBackup className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  return { mainItems, adminSection };
}
