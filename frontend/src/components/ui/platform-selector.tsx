'use client';

import * as React from 'react';
import { Check, ChevronsUpDown, Monitor, ShoppingBag, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Label } from '@/components/ui/label';
import type { Platform, Storefront } from '@/types';

// ============================================================================
// Types
// ============================================================================

export interface PlatformSelection {
  platform: string;
  storefront?: string;
}

// ============================================================================
// Sub-Components
// ============================================================================

interface PlatformBadgeProps {
  platform: Platform;
  storefront?: Storefront;
  onRemove?: () => void;
  className?: string;
}

function PlatformBadge({
  platform,
  storefront,
  onRemove,
  className,
}: PlatformBadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md bg-secondary px-2.5 py-1 text-sm font-medium text-secondary-foreground',
        className
      )}
    >
      <Monitor className="h-3 w-3" />
      <span className="truncate max-w-[100px]">{platform.display_name}</span>
      {storefront && (
        <>
          <span className="text-muted-foreground">/</span>
          <ShoppingBag className="h-3 w-3 text-muted-foreground" />
          <span className="truncate max-w-[80px] text-muted-foreground">
            {storefront.display_name}
          </span>
        </>
      )}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className="ml-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full hover:bg-muted focus:outline-none focus:ring-1 focus:ring-ring transition-colors"
          aria-label={`Remove ${platform.display_name}`}
        >
          <X className="h-2.5 w-2.5" />
        </button>
      )}
    </span>
  );
}

// ============================================================================
// Storefront Selector (used within PlatformSelector)
// ============================================================================

interface StorefrontSelectorProps {
  storefronts: Storefront[];
  selectedStorefront?: string;
  onStorefrontChange: (storefront: string | undefined) => void;
  disabled?: boolean;
}

function StorefrontSelector({
  storefronts,
  selectedStorefront,
  onStorefrontChange,
  disabled = false,
}: StorefrontSelectorProps) {
  if (storefronts.length === 0) {
    return null;
  }

  return (
    <Select
      value={selectedStorefront ?? 'none'}
      onValueChange={(value) =>
        onStorefrontChange(value === 'none' ? undefined : value)
      }
      disabled={disabled}
    >
      <SelectTrigger className="h-8 text-xs">
        <SelectValue placeholder="Select storefront" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="none">No storefront</SelectItem>
        {storefronts.map((storefront) => (
          <SelectItem key={storefront.name} value={storefront.name}>
            {storefront.display_name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

// ============================================================================
// Main Component
// ============================================================================

export interface PlatformSelectorProps {
  /** Currently selected platforms with optional storefronts */
  selectedPlatforms: PlatformSelection[];
  /** Available platforms to choose from */
  availablePlatforms: Platform[];
  /** Callback when selection changes */
  onChange: (selections: PlatformSelection[]) => void;
  /** Whether the selector is disabled */
  disabled?: boolean;
  /** Placeholder text when no platforms are selected */
  placeholder?: string;
  /** Maximum number of platforms that can be selected */
  maxSelection?: number;
  /** Additional class name */
  className?: string;
  /** ID for accessibility */
  id?: string;
}

export function PlatformSelector({
  selectedPlatforms,
  availablePlatforms,
  onChange,
  disabled = false,
  placeholder = 'Select platforms...',
  maxSelection,
  className,
  id,
}: PlatformSelectorProps) {
  const [open, setOpen] = React.useState(false);
  const [searchQuery, setSearchQuery] = React.useState('');

  // Get selected platform objects with their storefronts
  const selectedPlatformObjects = React.useMemo(() => {
    return selectedPlatforms.map((selection) => {
      const platform = availablePlatforms.find(
        (p) => p.name === selection.platform
      );
      const storefront = platform?.storefronts?.find(
        (s) => s.name === selection.storefront
      );
      return { selection, platform, storefront };
    });
  }, [availablePlatforms, selectedPlatforms]);

  // Filter available platforms based on search
  const filteredPlatforms = React.useMemo(() => {
    if (!searchQuery.trim()) return availablePlatforms;

    const query = searchQuery.toLowerCase();
    return availablePlatforms.filter(
      (platform) =>
        platform.display_name.toLowerCase().includes(query) ||
        platform.name.toLowerCase().includes(query)
    );
  }, [availablePlatforms, searchQuery]);

  // Check if we've reached max selection
  const isMaxReached =
    maxSelection !== undefined && selectedPlatforms.length >= maxSelection;

  const handlePlatformToggle = (platformName: string) => {
    if (disabled) return;

    const existingIndex = selectedPlatforms.findIndex(
      (s) => s.platform === platformName
    );

    if (existingIndex !== -1) {
      // Remove platform
      onChange(selectedPlatforms.filter((s) => s.platform !== platformName));
    } else if (!isMaxReached) {
      // Add platform with default storefront if available
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
  };

  const handleStorefrontChange = (
    platformName: string,
    storefront: string | undefined
  ) => {
    if (disabled) return;

    onChange(
      selectedPlatforms.map((s) =>
        s.platform === platformName ? { ...s, storefront } : s
      )
    );
  };

  const handleRemovePlatform = (platformName: string) => {
    if (disabled) return;
    onChange(selectedPlatforms.filter((s) => s.platform !== platformName));
  };

  const handleClearAll = () => {
    if (disabled) return;
    onChange([]);
  };

  return (
    <div className={cn('space-y-3', className)} id={id}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label="Select platforms"
            disabled={disabled}
            className={cn(
              'w-full justify-between min-h-[40px] h-auto py-2',
              !selectedPlatforms.length && 'text-muted-foreground'
            )}
          >
            <div className="flex flex-wrap gap-1 items-center">
              {selectedPlatformObjects.length > 0 ? (
                selectedPlatformObjects.length <= 2 ? (
                  selectedPlatformObjects.map(
                    ({ selection, platform, storefront }) =>
                      platform && (
                        <PlatformBadge
                          key={selection.platform}
                          platform={platform}
                          storefront={storefront}
                        />
                      )
                  )
                ) : (
                  <>
                    {selectedPlatformObjects.slice(0, 1).map(
                      ({ selection, platform, storefront }) =>
                        platform && (
                          <PlatformBadge
                            key={selection.platform}
                            platform={platform}
                            storefront={storefront}
                          />
                        )
                    )}
                    <Badge variant="secondary" className="text-xs">
                      +{selectedPlatformObjects.length - 1} more
                    </Badge>
                  </>
                )
              ) : (
                <span className="flex items-center gap-2">
                  <Monitor className="h-4 w-4" />
                  {placeholder}
                </span>
              )}
            </div>
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[320px] p-0" align="start">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Search platforms..."
              value={searchQuery}
              onValueChange={setSearchQuery}
            />
            <CommandList>
              {/* Quick Actions */}
              <CommandGroup>
                <div className="flex items-center justify-between px-2 py-1.5 text-xs text-muted-foreground">
                  <span>
                    {selectedPlatforms.length} of {filteredPlatforms.length}{' '}
                    selected
                    {maxSelection && ` (max ${maxSelection})`}
                  </span>
                  <button
                    type="button"
                    onClick={handleClearAll}
                    disabled={disabled || selectedPlatforms.length === 0}
                    className="text-primary hover:text-primary/80 disabled:text-muted-foreground disabled:cursor-not-allowed"
                  >
                    Clear
                  </button>
                </div>
              </CommandGroup>

              {/* Platform list */}
              {filteredPlatforms.length === 0 ? (
                <CommandEmpty>
                  {availablePlatforms.length === 0 ? (
                    <div className="flex flex-col items-center gap-2 py-4">
                      <Monitor className="h-8 w-8 text-muted-foreground/50" />
                      <p className="text-sm text-muted-foreground">
                        No platforms available
                      </p>
                    </div>
                  ) : (
                    <div className="flex flex-col items-center gap-2 py-4">
                      <p className="text-sm text-muted-foreground">
                        No platforms matching &quot;{searchQuery}&quot;
                      </p>
                    </div>
                  )}
                </CommandEmpty>
              ) : (
                <ScrollArea className="max-h-[250px]">
                  <CommandGroup heading="Platforms">
                    {filteredPlatforms.map((platform) => {
                      const isSelected = selectedPlatforms.some(
                        (s) => s.platform === platform.name
                      );
                      const isDisabledItem = !isSelected && isMaxReached;

                      return (
                        <CommandItem
                          key={platform.name}
                          value={platform.name}
                          onSelect={() => handlePlatformToggle(platform.name)}
                          disabled={isDisabledItem}
                          className={cn(
                            'cursor-pointer',
                            isDisabledItem && 'opacity-50 cursor-not-allowed'
                          )}
                        >
                          <div className="flex items-center gap-2 flex-1 min-w-0">
                            <div
                              className={cn(
                                'flex h-4 w-4 items-center justify-center rounded border',
                                isSelected
                                  ? 'bg-primary border-primary text-primary-foreground'
                                  : 'border-muted-foreground/25'
                              )}
                            >
                              {isSelected && <Check className="h-3 w-3" />}
                            </div>
                            <Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                            <span className="truncate flex-1">
                              {platform.display_name}
                            </span>
                            {platform.storefronts &&
                              platform.storefronts.length > 0 && (
                                <span className="text-xs text-muted-foreground flex-shrink-0">
                                  {platform.storefronts.length} store
                                  {platform.storefronts.length !== 1 ? 's' : ''}
                                </span>
                              )}
                          </div>
                        </CommandItem>
                      );
                    })}
                  </CommandGroup>
                </ScrollArea>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      {/* Selected platforms with storefront selection */}
      {selectedPlatformObjects.length > 0 && (
        <div className="space-y-2">
          {selectedPlatformObjects.map(({ selection, platform }) => {
            if (!platform) return null;
            const storefronts = platform.storefronts ?? [];

            return (
              <div
                key={selection.platform}
                className="flex items-center gap-3 p-3 rounded-lg border bg-card"
              >
                <div className="flex items-center gap-2 flex-1 min-w-0">
                  <Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                  <span className="font-medium truncate">
                    {platform.display_name}
                  </span>
                </div>
                {storefronts.length > 0 && (
                  <div className="flex-shrink-0 w-36">
                    <StorefrontSelector
                      storefronts={storefronts}
                      selectedStorefront={selection.storefront}
                      onStorefrontChange={(storefront) =>
                        handleStorefrontChange(
                          selection.platform,
                          storefront
                        )
                      }
                      disabled={disabled}
                    />
                  </div>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleRemovePlatform(selection.platform)}
                  disabled={disabled}
                  className="flex-shrink-0 h-8 w-8 p-0"
                >
                  <X className="h-4 w-4" />
                  <span className="sr-only">
                    Remove {platform.display_name}
                  </span>
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ============================================================================
// Compact Variant (for forms with limited space)
// ============================================================================

export interface PlatformSelectorCompactProps {
  /** Currently selected platforms with optional storefronts */
  selectedPlatforms: PlatformSelection[];
  /** Available platforms to choose from */
  availablePlatforms: Platform[];
  /** Callback when selection changes */
  onChange: (selections: PlatformSelection[]) => void;
  /** Whether the selector is disabled */
  disabled?: boolean;
  /** Additional class name */
  className?: string;
}

export function PlatformSelectorCompact({
  selectedPlatforms,
  availablePlatforms,
  onChange,
  disabled = false,
  className,
}: PlatformSelectorCompactProps) {
  const handleToggle = (platformName: string) => {
    if (disabled) return;

    const existingIndex = selectedPlatforms.findIndex(
      (s) => s.platform === platformName
    );

    if (existingIndex !== -1) {
      onChange(selectedPlatforms.filter((s) => s.platform !== platformName));
    } else {
      const platform = availablePlatforms.find((p) => p.name === platformName);
      const defaultStorefront = platform?.default_storefront;
      onChange([
        ...selectedPlatforms,
        {
          platform: platformName,
          storefront: defaultStorefront,
        },
      ]);
    }
  };

  const handleStorefrontChange = (
    platformName: string,
    storefront: string | undefined
  ) => {
    if (disabled) return;

    onChange(
      selectedPlatforms.map((s) =>
        s.platform === platformName ? { ...s, storefront } : s
      )
    );
  };

  if (availablePlatforms.length === 0) {
    return (
      <div className={cn('text-center py-8 text-muted-foreground', className)}>
        <Monitor className="w-12 h-12 mx-auto mb-4 text-muted-foreground/50" />
        <p className="text-sm">No platforms available</p>
      </div>
    );
  }

  return (
    <div className={cn('space-y-2', className)}>
      {availablePlatforms.map((platform) => {
        const selection = selectedPlatforms.find(
          (s) => s.platform === platform.name
        );
        const isSelected = !!selection;
        const storefronts = platform.storefronts ?? [];

        return (
          <div
            key={platform.name}
            className={cn(
              'rounded-lg border p-3 transition-colors',
              isSelected
                ? 'bg-primary/5 border-primary/20'
                : 'bg-background border-border hover:bg-accent/50',
              disabled && 'opacity-50'
            )}
          >
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-input text-primary focus:ring-primary"
                checked={isSelected}
                onChange={() => handleToggle(platform.name)}
                disabled={disabled}
              />
              <Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0" />
              <span className="font-medium flex-1">{platform.display_name}</span>
            </label>
            {isSelected && storefronts.length > 0 && (
              <div className="mt-2 ml-7 flex items-center gap-2">
                <Label className="text-xs text-muted-foreground">
                  Storefront:
                </Label>
                <StorefrontSelector
                  storefronts={storefronts}
                  selectedStorefront={selection?.storefront}
                  onStorefrontChange={(storefront) =>
                    handleStorefrontChange(platform.name, storefront)
                  }
                  disabled={disabled}
                />
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
