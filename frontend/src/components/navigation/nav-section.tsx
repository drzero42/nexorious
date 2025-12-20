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
  needsAttention = false,
  onNavigate,
}: NavSectionCollapsibleProps) {
  // Track if the user has manually closed the section
  const [userClosed, setUserClosed] = useState(false);

  // Open if: needsAttention is true (and user hasn't closed), OR defaultOpen, OR user opened
  // The key insight: needsAttention=true should force open unless user explicitly closed
  const isOpen = needsAttention ? !userClosed : defaultOpen;
  const [manualOpen, setManualOpen] = useState<boolean | null>(null);

  // Final open state: manual override takes precedence, then computed state
  const finalIsOpen = manualOpen !== null ? manualOpen : isOpen;

  const handleOpenChange = (open: boolean) => {
    setManualOpen(open);
    if (!open && needsAttention) {
      setUserClosed(true);
    }
  };

  return (
    <Collapsible open={finalIsOpen} onOpenChange={handleOpenChange}>
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
