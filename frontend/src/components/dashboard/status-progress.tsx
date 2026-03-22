'use client';

import * as React from 'react';
import * as ProgressPrimitive from '@radix-ui/react-progress';
import { cn } from '@/lib/utils';
import { PlayStatus } from '@/types';
import { statusColors, statusLabels, statusIcons, statusDescriptions } from './status-progress-data';

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

