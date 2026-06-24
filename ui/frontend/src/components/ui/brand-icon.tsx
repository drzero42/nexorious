import * as React from 'react';
import { useTheme } from 'next-themes';
import { config } from '@/lib/env';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

export interface BrandIconProps {
  /** Bare stored icon path (e.g. "/logos/platforms/pc/pc-icon-light.svg"); staticUrl is prefixed here. */
  iconUrl?: string;
  displayName: string;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  /**
   * Mark the icon as decorative so screen readers ignore it. Use when a visible
   * text label is adjacent (the label carries the meaning); otherwise the icon's
   * accessible name doubles up with the label (e.g. "PC PC"). The `showLabel`
   * path is always decorative since it renders its own label.
   */
  decorative?: boolean;
  className?: string;
}

const brandIconSizeClasses: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'h-4 w-4',
  md: 'h-5 w-5',
  lg: 'h-6 w-6',
};

const LIGHT_TOKEN = '-icon-light.svg';
const DARK_TOKEN = '-icon-dark.svg';

export function BrandIcon({
  iconUrl,
  displayName,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  decorative = false,
  className,
}: BrandIconProps) {
  const { resolvedTheme } = useTheme();

  // The showLabel path renders its own adjacent label, so the icon is always
  // decorative there; otherwise honour the explicit prop.
  const hideFromScreenReaders = decorative || showLabel;

  const basePath = iconUrl ? `${config.staticUrl}${iconUrl}` : null;
  const themedPath =
    basePath && resolvedTheme === 'dark' && basePath.includes(LIGHT_TOKEN)
      ? basePath.replace(LIGHT_TOKEN, DARK_TOKEN)
      : basePath;

  const [src, setSrc] = React.useState<string | null>(themedPath);
  const [failed, setFailed] = React.useState(false);

  // Reset during render when the themed path changes (theme toggle, new icon).
  // This is the React-idiomatic "reset state on prop change" pattern — calling
  // setState in the render body (not in an effect) schedules a synchronous
  // re-render before the browser paints, avoiding the double-paint of useEffect.
  const prevThemedPath = React.useRef(themedPath);
  if (prevThemedPath.current !== themedPath) {
    prevThemedPath.current = themedPath;
    setSrc(themedPath);
    setFailed(false);
  }

  const handleError = () => {
    if (src && basePath && src !== basePath) {
      // Dark variant missing — fall back to the stored (light) asset.
      setSrc(basePath);
    } else {
      // No usable image — show the badge.
      setFailed(true);
    }
  };

  const showImage = src != null && !failed;

  const icon = showImage ? (
    <img
      src={src}
      alt={hideFromScreenReaders ? '' : displayName}
      width={24}
      height={24}
      className={cn(brandIconSizeClasses[size], 'object-contain', className)}
      loading="lazy"
      onError={handleError}
    />
  ) : (
    <span
      aria-hidden={hideFromScreenReaders || undefined}
      className={cn('text-muted-foreground', brandIconSizeClasses[size], className)}
    >
      {displayName.charAt(0)}
    </span>
  );

  const content = showLabel ? (
    <span className="inline-flex items-center gap-1.5">
      {icon}
      <span className="text-sm text-muted-foreground">{displayName}</span>
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
            <p>{displayName}</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return content;
}
