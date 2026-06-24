import { BrandIcon } from '@/components/ui/brand-icon';
import { cn } from '@/lib/utils';
import type { Platform, Storefront } from '@/types';

interface PlatformIconProps {
  platform: Platform;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  decorative?: boolean;
  className?: string;
}

function PlatformIcon({
  platform,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  decorative = false,
  className,
}: PlatformIconProps) {
  return (
    <BrandIcon
      iconUrl={platform.icon_url}
      displayName={platform.display_name}
      size={size}
      showTooltip={showTooltip}
      showLabel={showLabel}
      decorative={decorative}
      className={className}
    />
  );
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

interface StorefrontIconProps {
  storefront: Storefront;
  size?: 'sm' | 'md' | 'lg';
  showTooltip?: boolean;
  showLabel?: boolean;
  decorative?: boolean;
  className?: string;
}

export function StorefrontIcon({
  storefront,
  size = 'md',
  showTooltip = false,
  showLabel = false,
  decorative = false,
  className,
}: StorefrontIconProps) {
  return (
    <BrandIcon
      iconUrl={storefront.icon_url}
      displayName={storefront.display_name}
      size={size}
      showTooltip={showTooltip}
      showLabel={showLabel}
      decorative={decorative}
      className={className}
    />
  );
}

export { PlatformIcon };
