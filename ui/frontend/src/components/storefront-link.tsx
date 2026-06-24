import { StorefrontIcon } from '@/components/ui/platform-icon';
import type { Storefront } from '@/types';

interface StorefrontLabelProps {
  storefront: Storefront;
  storeUrl?: string;
}

export function StorefrontLabel({ storefront, storeUrl }: StorefrontLabelProps) {
  const inner = (
    <span className="inline-flex items-center gap-1">
      <StorefrontIcon storefront={storefront} size="sm" decorative />
      {storefront.display_name}
    </span>
  );

  if (storeUrl) {
    return (
      <a
        href={storeUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center text-sm text-muted-foreground underline-offset-2 hover:underline"
      >
        {inner}
      </a>
    );
  }
  return <span className="text-sm text-muted-foreground">{inner}</span>;
}
