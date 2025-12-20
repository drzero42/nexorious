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
