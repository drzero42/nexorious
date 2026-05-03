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
  needsAttention?: boolean;  // Add this - when true, section auto-expands
}
