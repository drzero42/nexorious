import * as React from 'react';
import { Check, ChevronsUpDown, Monitor, Plus, X } from 'lucide-react';
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
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Label } from '@/components/ui/label';
import {
  availableStorefronts,
  firstFreeStorefront,
  isPlatformExhausted,
  usedStorefronts,
} from '@/components/ui/platform-options';
import type { Platform, Storefront } from '@/types';

// ============================================================================
// Types
// ============================================================================

export interface PlatformSelection {
  /** Stable client-side identity for this selected row. Always present. */
  key: string;
  /** Server UUID. Present only once the row is persisted in the database. */
  id?: string;
  platform: string;
  storefront?: string;
}

/** Generates a stable client-side key for a newly-created selection row. */
function newSelectionKey(): string {
  return typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `sel-${Math.random().toString(36).slice(2)}`;
}

/** Human label for a row's storefront, falling back to the raw name. */
function storefrontLabel(platform: Platform | undefined, storefront?: string): string | undefined {
  if (!storefront) return undefined;
  return platform?.storefronts?.find((s) => s.name === storefront)?.display_name ?? storefront;
}

/** Accessible label for a row's remove control. */
function removeLabel(platform: Platform | undefined, storefront?: string): string {
  const name = platform?.display_name ?? 'platform';
  const sf = storefrontLabel(platform, storefront);
  return sf ? `Remove ${name} / ${sf}` : `Remove ${name}`;
}

// ============================================================================
// Storefront Selector (shared)
// ============================================================================

interface StorefrontSelectorProps {
  /** Storefronts selectable in this row (already constrained to free slots). */
  storefronts: Storefront[];
  selectedStorefront?: string;
  onStorefrontChange: (storefront: string | undefined) => void;
  /** Whether the "No storefront" option is offered. */
  allowNone?: boolean;
  disabled?: boolean;
}

function StorefrontSelector({
  storefronts,
  selectedStorefront,
  onStorefrontChange,
  allowNone = true,
  disabled = false,
}: StorefrontSelectorProps) {
  // Keep the current value showable even when its slot would otherwise be hidden.
  const showNone = allowNone || selectedStorefront == null;

  return (
    <Select
      value={selectedStorefront ?? 'none'}
      onValueChange={(value) => onStorefrontChange(value === 'none' ? undefined : value)}
      disabled={disabled}
    >
      <SelectTrigger className="h-8 text-xs">
        <SelectValue placeholder="Select storefront" />
      </SelectTrigger>
      <SelectContent>
        {showNone && <SelectItem value="none">No storefront</SelectItem>}
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
// PlatformSelector (row-based editor) — edit page
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
  /** Placeholder text for an empty platform picker */
  placeholder?: string;
  /** Additional class name */
  className?: string;
  /** ID for accessibility */
  id?: string;
}

interface PlatformRowEditorProps {
  selection: PlatformSelection;
  allRows: PlatformSelection[];
  availablePlatforms: Platform[];
  placeholder: string;
  disabled: boolean;
  onPlatformChange: (key: string, platformName: string) => void;
  onStorefrontChange: (key: string, storefront: string | undefined) => void;
  onRemove: (key: string) => void;
}

function PlatformRowEditor({
  selection,
  allRows,
  availablePlatforms,
  placeholder,
  disabled,
  onPlatformChange,
  onStorefrontChange,
  onRemove,
}: PlatformRowEditorProps) {
  const [open, setOpen] = React.useState(false);
  const [searchQuery, setSearchQuery] = React.useState('');

  const platform = availablePlatforms.find((p) => p.name === selection.platform);
  const otherRows = React.useMemo(
    () => allRows.filter((r) => r.key !== selection.key),
    [allRows, selection.key],
  );

  const filteredPlatforms = React.useMemo(() => {
    if (!searchQuery.trim()) return availablePlatforms;
    const query = searchQuery.toLowerCase();
    return availablePlatforms.filter(
      (p) => p.display_name.toLowerCase().includes(query) || p.name.toLowerCase().includes(query),
    );
  }, [availablePlatforms, searchQuery]);

  const hasStorefronts = (platform?.storefronts?.length ?? 0) > 0;

  return (
    <div className="flex items-center gap-3 p-3 rounded-lg border bg-card">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label={platform ? `Change platform: ${platform.display_name}` : placeholder}
            disabled={disabled}
            className={cn('flex-1 justify-between min-w-0', !platform && 'text-muted-foreground')}
          >
            <span className="flex items-center gap-2 min-w-0">
              <Monitor className="h-4 w-4 shrink-0" />
              <span className="truncate">{platform?.display_name ?? placeholder}</span>
            </span>
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[280px] p-0" align="start">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Search platforms..."
              value={searchQuery}
              onValueChange={setSearchQuery}
            />
            <CommandList>
              {filteredPlatforms.length === 0 ? (
                <CommandEmpty>No platforms found</CommandEmpty>
              ) : (
                <CommandGroup heading="Platforms">
                  {filteredPlatforms.map((p) => {
                    const isCurrent = p.name === selection.platform;
                    const itemDisabled = !isCurrent && isPlatformExhausted(p, otherRows);
                    return (
                      <CommandItem
                        key={p.name}
                        value={p.name}
                        disabled={itemDisabled}
                        onSelect={() => {
                          onPlatformChange(selection.key, p.name);
                          setOpen(false);
                        }}
                        className={cn('cursor-pointer', itemDisabled && 'cursor-not-allowed')}
                      >
                        <Check
                          className={cn('mr-2 h-4 w-4', isCurrent ? 'opacity-100' : 'opacity-0')}
                        />
                        <Monitor className="mr-2 h-4 w-4 text-muted-foreground" />
                        <span className="truncate">{p.display_name}</span>
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      {hasStorefronts && platform && (
        <div className="w-40 shrink-0">
          <StorefrontSelector
            storefronts={availableStorefronts(platform, allRows, selection.key)}
            allowNone={!usedStorefronts(allRows, platform.name, selection.key).has(undefined)}
            selectedStorefront={selection.storefront}
            onStorefrontChange={(sf) => onStorefrontChange(selection.key, sf)}
            disabled={disabled}
          />
        </div>
      )}

      <Button
        variant="ghost"
        size="sm"
        onClick={() => onRemove(selection.key)}
        disabled={disabled}
        className="shrink-0 h-8 w-8 p-0"
      >
        <X className="h-4 w-4" />
        <span className="sr-only">{removeLabel(platform, selection.storefront)}</span>
      </Button>
    </div>
  );
}

export function PlatformSelector({
  selectedPlatforms,
  availablePlatforms,
  onChange,
  disabled = false,
  placeholder = 'Select platform...',
  className,
  id,
}: PlatformSelectorProps) {
  const handleAddRow = () => {
    if (disabled) return;
    onChange([
      ...selectedPlatforms,
      { key: newSelectionKey(), platform: '', storefront: undefined },
    ]);
  };

  const handlePlatformChange = (key: string, platformName: string) => {
    if (disabled) return;
    const others = selectedPlatforms.filter((s) => s.key !== key);
    const platform = availablePlatforms.find((p) => p.name === platformName);
    const storefront = platform ? firstFreeStorefront(platform, others) : undefined;
    onChange(
      selectedPlatforms.map((s) =>
        s.key === key ? { ...s, platform: platformName, storefront } : s,
      ),
    );
  };

  const handleStorefrontChange = (key: string, storefront: string | undefined) => {
    if (disabled) return;
    onChange(selectedPlatforms.map((s) => (s.key === key ? { ...s, storefront } : s)));
  };

  const handleRemove = (key: string) => {
    if (disabled) return;
    onChange(selectedPlatforms.filter((s) => s.key !== key));
  };

  const allExhausted =
    availablePlatforms.length > 0 &&
    availablePlatforms.every((p) => isPlatformExhausted(p, selectedPlatforms));

  return (
    <div className={cn('space-y-3', className)} id={id}>
      {selectedPlatforms.length > 0 && (
        <div className="space-y-2">
          {selectedPlatforms.map((selection) => (
            <PlatformRowEditor
              key={selection.key}
              selection={selection}
              allRows={selectedPlatforms}
              availablePlatforms={availablePlatforms}
              placeholder={placeholder}
              disabled={disabled}
              onPlatformChange={handlePlatformChange}
              onStorefrontChange={handleStorefrontChange}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}

      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={handleAddRow}
        disabled={disabled || availablePlatforms.length === 0 || allExhausted}
      >
        <Plus className="mr-2 h-4 w-4" />
        Add platform
      </Button>
    </div>
  );
}

// ============================================================================
// PlatformSelectorCompact (add wizard)
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
  const handleToggle = (platform: Platform) => {
    if (disabled) return;
    const rows = selectedPlatforms.filter((s) => s.platform === platform.name);
    if (rows.length > 0) {
      // Uncheck: drop every copy of this platform.
      onChange(selectedPlatforms.filter((s) => s.platform !== platform.name));
    } else {
      onChange([
        ...selectedPlatforms,
        {
          key: newSelectionKey(),
          platform: platform.name,
          storefront: firstFreeStorefront(platform, selectedPlatforms),
        },
      ]);
    }
  };

  const handleAddStorefront = (platform: Platform) => {
    if (disabled) return;
    onChange([
      ...selectedPlatforms,
      {
        key: newSelectionKey(),
        platform: platform.name,
        storefront: firstFreeStorefront(platform, selectedPlatforms),
      },
    ]);
  };

  const handleStorefrontChange = (key: string, storefront: string | undefined) => {
    if (disabled) return;
    onChange(selectedPlatforms.map((s) => (s.key === key ? { ...s, storefront } : s)));
  };

  const handleRemoveRow = (key: string) => {
    if (disabled) return;
    onChange(selectedPlatforms.filter((s) => s.key !== key));
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
        const rows = selectedPlatforms.filter((s) => s.platform === platform.name);
        const isSelected = rows.length > 0;
        const storefronts = platform.storefronts ?? [];
        const exhausted = isPlatformExhausted(platform, selectedPlatforms);

        return (
          <div
            key={platform.name}
            className={cn(
              'rounded-lg border p-3 transition-colors',
              isSelected
                ? 'bg-primary/5 border-primary/20'
                : 'bg-background border-border hover:bg-accent/50',
              disabled && 'opacity-50',
            )}
          >
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-input text-primary focus:ring-primary"
                checked={isSelected}
                onChange={() => handleToggle(platform)}
                disabled={disabled}
              />
              <Monitor className="h-4 w-4 text-muted-foreground flex-shrink-0" />
              <span className="font-medium flex-1">{platform.display_name}</span>
            </label>

            {isSelected && storefronts.length > 0 && (
              <div className="mt-2 ml-7 space-y-2">
                {rows.map((row) => (
                  <div key={row.key} className="flex items-center gap-2">
                    <Label className="text-xs text-muted-foreground">Storefront:</Label>
                    <StorefrontSelector
                      storefronts={availableStorefronts(platform, selectedPlatforms, row.key)}
                      allowNone={
                        !usedStorefronts(selectedPlatforms, platform.name, row.key).has(undefined)
                      }
                      selectedStorefront={row.storefront}
                      onStorefrontChange={(sf) => handleStorefrontChange(row.key, sf)}
                      disabled={disabled}
                    />
                    {rows.length > 1 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemoveRow(row.key)}
                        disabled={disabled}
                        className="h-7 w-7 p-0"
                      >
                        <X className="h-3.5 w-3.5" />
                        <span className="sr-only">{removeLabel(platform, row.storefront)}</span>
                      </Button>
                    )}
                  </div>
                ))}
                {!exhausted && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => handleAddStorefront(platform)}
                    disabled={disabled}
                    className="h-7 text-xs text-primary"
                  >
                    <Plus className="mr-1 h-3.5 w-3.5" />
                    Add another storefront
                  </Button>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
