import {
  LayoutDashboard,
  Library,
  Plus,
  Heart,
  RefreshCw,
  Tag,
  ListChecks,
  ArrowLeftRight,
  Users,
  Shield,
  Wrench,
  DatabaseBackup,
  Activity,
  HelpCircle,
  BookOpen,
} from 'lucide-react';
import type { NavItem, NavSection } from './types';
import { usePendingReviewCount } from '@/hooks/use-jobs';
import { useImportSources } from '@/hooks';

export function useNavItems() {
  const { data: reviewData } = usePendingReviewCount();
  const { data: importSources } = useImportSources();
  // Split the pending-review backlog by surface: registry-backed import sources
  // are reviewed on the Import/Export page, every other source (the sync
  // storefronts) on the Sync page.
  const importReviewCount = (importSources ?? []).reduce(
    (sum, s) => sum + (reviewData?.countsBySource?.[s.slug] ?? 0),
    0,
  );
  const syncReviewCount = Math.max(0, (reviewData?.pendingReviewCount ?? 0) - importReviewCount);

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
      href: '/wishlist',
      label: 'Wishlist',
      icon: <Heart className="h-4 w-4" />,
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
      badge: syncReviewCount,
    },
    {
      href: '/tags',
      label: 'Tags',
      icon: <Tag className="h-4 w-4" />,
    },
    {
      href: '/pools',
      label: 'Planning',
      icon: <ListChecks className="h-4 w-4" />,
    },
    {
      href: '/import-export',
      label: 'Import / Export',
      icon: <ArrowLeftRight className="h-4 w-4" />,
      badge: importReviewCount,
    },
    {
      href: '/help/user-guide',
      label: 'Help',
      icon: <HelpCircle className="h-4 w-4" />,
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
        href: '/admin/activity',
        label: 'Activity',
        icon: <Activity className="h-4 w-4" />,
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
      {
        href: '/help/admin-guide',
        label: 'Admin Guide',
        icon: <BookOpen className="h-4 w-4" />,
      },
    ],
    defaultOpen: false,
  };

  return { mainItems, adminSection };
}
