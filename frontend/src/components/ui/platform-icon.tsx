'use client';

import Image from 'next/image';
import { config } from '@/lib/env';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import type { Platform } from '@/types';

export interface PlatformIconProps {
  platform: Platform;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  className?: string;
}

const sizeClasses = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-6 w-6',
};

export function PlatformIcon({
  platform,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  className,
}: PlatformIconProps) {
  const iconUrl = platform.icon_url
    ? `${config.staticUrl}${platform.icon_url}`
    : null;

  const icon = iconUrl ? (
    <Image
      src={iconUrl}
      alt={platform.display_name}
      width={24}
      height={24}
      className={cn(sizeClasses[size], 'object-contain', className)}
    />
  ) : (
    <span className={cn('text-muted-foreground', sizeClasses[size], className)}>
      {platform.display_name.charAt(0)}
    </span>
  );

  const content = showLabel ? (
    <span className="inline-flex items-center gap-1.5">
      {icon}
      <span className="text-sm text-muted-foreground">{platform.display_name}</span>
    </span>
  ) : (
    icon
  );

  if (showTooltip && !showLabel) {
    return (
      <TooltipProvider delayDuration={300}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">{content}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{platform.display_name}</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return content;
}

export interface PlatformIconListProps {
  platforms: Array<{ platform_details?: Platform; platform?: string }>;
  size?: 'sm' | 'md' | 'lg';
  showTooltips?: boolean;
  showLabels?: boolean;
  className?: string;
}

export function PlatformIconList({
  platforms,
  size = 'md',
  showTooltips = false,
  showLabels = false,
  className,
}: PlatformIconListProps) {
  const validPlatforms = platforms.filter((p) => p.platform_details);

  if (validPlatforms.length === 0) {
    return <span className="text-sm text-muted-foreground">-</span>;
  }

  return (
    <span className={cn('inline-flex items-center gap-1.5 flex-wrap', className)}>
      {validPlatforms.map((p, index) => (
        <PlatformIcon
          key={p.platform_details!.name + index}
          platform={p.platform_details!}
          size={size}
          showTooltip={showTooltips}
          showLabel={showLabels}
        />
      ))}
    </span>
  );
}
