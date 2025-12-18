'use client';

import * as React from 'react';
import * as ProgressPrimitive from '@radix-ui/react-progress';
import { cn } from '@/lib/utils';
import { PlayStatus } from '@/types';

const statusColors: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'bg-gray-400',
  [PlayStatus.IN_PROGRESS]: 'bg-blue-500',
  [PlayStatus.COMPLETED]: 'bg-green-500',
  [PlayStatus.MASTERED]: 'bg-purple-500',
  [PlayStatus.DOMINATED]: 'bg-yellow-500',
  [PlayStatus.SHELVED]: 'bg-orange-500',
  [PlayStatus.DROPPED]: 'bg-red-500',
  [PlayStatus.REPLAY]: 'bg-cyan-500',
};

const statusLabels: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'Not Started',
  [PlayStatus.IN_PROGRESS]: 'In Progress',
  [PlayStatus.COMPLETED]: 'Completed',
  [PlayStatus.MASTERED]: 'Mastered',
  [PlayStatus.DOMINATED]: 'Dominated',
  [PlayStatus.SHELVED]: 'Shelved',
  [PlayStatus.DROPPED]: 'Dropped',
  [PlayStatus.REPLAY]: 'Replay',
};

const statusIcons: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: '⏸️',
  [PlayStatus.IN_PROGRESS]: '🎮',
  [PlayStatus.COMPLETED]: '✅',
  [PlayStatus.MASTERED]: '🏆',
  [PlayStatus.DOMINATED]: '👑',
  [PlayStatus.SHELVED]: '📚',
  [PlayStatus.DROPPED]: '❌',
  [PlayStatus.REPLAY]: '🔄',
};

const statusDescriptions: Record<PlayStatus, string> = {
  [PlayStatus.NOT_STARTED]: 'Games waiting to be played',
  [PlayStatus.IN_PROGRESS]: 'Currently playing',
  [PlayStatus.COMPLETED]: 'Main story finished',
  [PlayStatus.MASTERED]: 'All major content done',
  [PlayStatus.DOMINATED]: '100% completion',
  [PlayStatus.SHELVED]: 'On hold for later',
  [PlayStatus.DROPPED]: 'No longer playing',
  [PlayStatus.REPLAY]: 'Playing again',
};

interface StatusProgressProps {
  status: PlayStatus;
  count: number;
  total: number;
  showDescription?: boolean;
  className?: string;
}

export function StatusProgress({
  status,
  count,
  total,
  showDescription = false,
  className,
}: StatusProgressProps) {
  const percentage = total > 0 ? (count / total) * 100 : 0;
  const colorClass = statusColors[status];
  const label = statusLabels[status];
  const icon = statusIcons[status];
  const description = statusDescriptions[status];

  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-lg" role="img" aria-label={label}>
            {icon}
          </span>
          <div>
            <span className="font-medium text-foreground">{label}</span>
            <span className="ml-2 text-sm text-muted-foreground">
              ({count} {count === 1 ? 'game' : 'games'})
            </span>
          </div>
        </div>
        <span className="text-sm text-muted-foreground">
          {percentage.toFixed(1)}%
        </span>
      </div>
      <ProgressPrimitive.Root
        className="relative h-2 w-full overflow-hidden rounded-full bg-secondary"
        value={percentage}
      >
        <ProgressPrimitive.Indicator
          className={cn('h-full transition-all', colorClass)}
          style={{ width: `${percentage}%` }}
        />
      </ProgressPrimitive.Root>
      {showDescription && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
    </div>
  );
}

export { statusColors, statusLabels, statusIcons, statusDescriptions };
