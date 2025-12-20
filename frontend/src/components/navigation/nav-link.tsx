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
