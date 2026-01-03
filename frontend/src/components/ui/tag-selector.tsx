'use client';

import * as React from 'react';
import { Check, ChevronsUpDown, Plus, Search, Tag as TagIcon, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import type { Tag } from '@/types';

// ============================================================================
// Color Utilities
// ============================================================================

/**
 * Calculate appropriate text color based on background luminance.
 */
function getTextColor(hexColor: string): string {
  const hex = hexColor.replace('#', '');
  const r = parseInt(hex.substring(0, 2), 16);
  const g = parseInt(hex.substring(2, 4), 16);
  const b = parseInt(hex.substring(4, 6), 16);

  // Calculate relative luminance
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;

  return luminance > 0.5 ? '#000000' : '#FFFFFF';
}

// ============================================================================
// Sub-Components
// ============================================================================

interface TagBadgeProps {
  tag: Tag;
  onRemove?: () => void;
  size?: 'sm' | 'md';
  className?: string;
}

function TagBadge({ tag, onRemove, size = 'md', className }: TagBadgeProps) {
  const textColor = getTextColor(tag.color);
  const sizeClasses = size === 'sm' ? 'text-xs px-2 py-0.5' : 'text-sm px-2.5 py-1';

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full font-medium transition-colors',
        sizeClasses,
        className
      )}
      style={{ backgroundColor: tag.color, color: textColor }}
    >
      <span className="truncate max-w-[120px]">{tag.name}</span>
      {tag.game_count !== undefined && tag.game_count > 0 && (
        <span className="opacity-75 text-xs">({tag.game_count})</span>
      )}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRemove();
          }}
          className="ml-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full hover:bg-white/20 focus:outline-none focus:ring-1 focus:ring-white/50 transition-colors"
          aria-label={`Remove tag ${tag.name}`}
        >
          <X className="h-2.5 w-2.5" />
        </button>
      )}
    </span>
  );
}

// ============================================================================
// Main Component
// ============================================================================

export interface TagSelectorProps {
  /** Currently selected tag IDs */
  selectedTagIds: string[];
  /** Available tags to choose from */
  availableTags: Tag[];
  /** Callback when selection changes */
  onChange: (selectedTagIds: string[]) => void;
  /** Callback when user wants to create a new tag */
  onCreateTag?: (name: string) => void;
  /** Whether the selector is disabled */
  disabled?: boolean;
  /** Placeholder text when no tags are selected */
  placeholder?: string;
  /** Whether to allow creating new tags inline */
  allowCreate?: boolean;
  /** Whether to show game counts on tags */
  showGameCounts?: boolean;
  /** Maximum number of tags that can be selected */
  maxSelection?: number;
  /** Additional class name */
  className?: string;
  /** ID for accessibility */
  id?: string;
}

export function TagSelector({
  selectedTagIds,
  availableTags,
  onChange,
  onCreateTag,
  disabled = false,
  placeholder = 'Select tags...',
  allowCreate = false,
  showGameCounts = true,
  maxSelection,
  className,
  id,
}: TagSelectorProps) {
  const [open, setOpen] = React.useState(false);
  const [searchQuery, setSearchQuery] = React.useState('');

  // Get selected tag objects
  const selectedTags = React.useMemo(() => {
    return availableTags.filter((tag) => selectedTagIds.includes(tag.id));
  }, [availableTags, selectedTagIds]);

  // Filter available tags based on search
  const filteredTags = React.useMemo(() => {
    if (!searchQuery.trim()) return availableTags;

    const query = searchQuery.toLowerCase();
    return availableTags.filter(
      (tag) =>
        tag.name.toLowerCase().includes(query) ||
        tag.description?.toLowerCase().includes(query)
    );
  }, [availableTags, searchQuery]);

  // Check if search query matches any existing tag
  const canCreateNewTag = React.useMemo(() => {
    if (!allowCreate || !searchQuery.trim()) return false;

    const normalizedQuery = searchQuery.trim().toLowerCase();
    return !availableTags.some(
      (tag) => tag.name.toLowerCase() === normalizedQuery
    );
  }, [allowCreate, availableTags, searchQuery]);

  // Check if we've reached max selection
  const isMaxReached = maxSelection !== undefined && selectedTagIds.length >= maxSelection;

  const handleTagToggle = (tagId: string) => {
    if (disabled) return;

    if (selectedTagIds.includes(tagId)) {
      onChange(selectedTagIds.filter((id) => id !== tagId));
    } else if (!isMaxReached) {
      onChange([...selectedTagIds, tagId]);
    }
  };

  const handleRemoveTag = (tagId: string) => {
    if (disabled) return;
    onChange(selectedTagIds.filter((id) => id !== tagId));
  };

  const handleCreateTag = () => {
    if (disabled || !canCreateNewTag || !onCreateTag) return;

    onCreateTag(searchQuery.trim());
    setSearchQuery('');
  };

  const handleSelectAll = () => {
    if (disabled) return;

    const allFilteredIds = filteredTags.map((tag) => tag.id);
    const idsToAdd = maxSelection
      ? allFilteredIds.slice(0, maxSelection - selectedTagIds.length)
      : allFilteredIds;

    const newIds = [...new Set([...selectedTagIds, ...idsToAdd])];
    onChange(maxSelection ? newIds.slice(0, maxSelection) : newIds);
  };

  const handleClearAll = () => {
    if (disabled) return;
    onChange([]);
  };

  return (
    <div className={cn('space-y-2', className)} id={id}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label="Select tags"
            disabled={disabled}
            className={cn(
              'w-full justify-between min-h-[40px] h-auto py-2',
              !selectedTags.length && 'text-muted-foreground'
            )}
          >
            <div className="flex flex-wrap gap-1 items-center">
              {selectedTags.length > 0 ? (
                selectedTags.length <= 3 ? (
                  selectedTags.map((tag) => (
                    <TagBadge
                      key={tag.id}
                      tag={tag}
                      size="sm"
                    />
                  ))
                ) : (
                  <>
                    {selectedTags.slice(0, 2).map((tag) => (
                      <TagBadge
                        key={tag.id}
                        tag={tag}
                        size="sm"
                      />
                    ))}
                    <Badge variant="secondary" className="text-xs">
                      +{selectedTags.length - 2} more
                    </Badge>
                  </>
                )
              ) : (
                <span className="flex items-center gap-2">
                  <TagIcon className="h-4 w-4" />
                  {placeholder}
                </span>
              )}
            </div>
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[300px] p-0" align="start">
          <Command shouldFilter={false}>
            <CommandInput
              placeholder="Search tags..."
              value={searchQuery}
              onValueChange={setSearchQuery}
            />
            <CommandList>
              {/* Quick Actions */}
              <CommandGroup>
                <div className="flex items-center justify-between px-2 py-1.5 text-xs text-muted-foreground">
                  <span>
                    {selectedTagIds.length} of {filteredTags.length} selected
                    {maxSelection && ` (max ${maxSelection})`}
                  </span>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={handleSelectAll}
                      disabled={disabled || isMaxReached}
                      className="text-primary hover:text-primary/80 disabled:text-muted-foreground disabled:cursor-not-allowed"
                    >
                      All
                    </button>
                    <span className="text-muted-foreground/50">|</span>
                    <button
                      type="button"
                      onClick={handleClearAll}
                      disabled={disabled || selectedTagIds.length === 0}
                      className="text-primary hover:text-primary/80 disabled:text-muted-foreground disabled:cursor-not-allowed"
                    >
                      None
                    </button>
                  </div>
                </div>
              </CommandGroup>

              <CommandSeparator />

              {/* Create new tag option */}
              {canCreateNewTag && onCreateTag && (
                <>
                  <CommandGroup heading="Create new">
                    <CommandItem
                      onSelect={handleCreateTag}
                      className="cursor-pointer"
                    >
                      <Plus className="mr-2 h-4 w-4" />
                      Create &quot;{searchQuery.trim()}&quot;
                    </CommandItem>
                  </CommandGroup>
                  <CommandSeparator />
                </>
              )}

              {/* Tag list */}
              {filteredTags.length === 0 ? (
                <CommandEmpty>
                  {availableTags.length === 0 ? (
                    <div className="flex flex-col items-center gap-2 py-4">
                      <TagIcon className="h-8 w-8 text-muted-foreground/50" />
                      <p className="text-sm text-muted-foreground">No tags available</p>
                      {allowCreate && (
                        <p className="text-xs text-muted-foreground">
                          Type to create a new tag
                        </p>
                      )}
                    </div>
                  ) : (
                    <div className="flex flex-col items-center gap-2 py-4">
                      <Search className="h-6 w-6 text-muted-foreground/50" />
                      <p className="text-sm text-muted-foreground">
                        No tags matching &quot;{searchQuery}&quot;
                      </p>
                    </div>
                  )}
                </CommandEmpty>
              ) : (
                <ScrollArea className="max-h-[200px]">
                  <CommandGroup heading="Tags">
                    {filteredTags.map((tag) => {
                      const isSelected = selectedTagIds.includes(tag.id);
                      const isDisabledItem = !isSelected && isMaxReached;

                      return (
                        <CommandItem
                          key={tag.id}
                          value={tag.id}
                          onSelect={() => handleTagToggle(tag.id)}
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
                            <div
                              className="h-3 w-3 rounded-full border border-muted-foreground/25 flex-shrink-0"
                              style={{ backgroundColor: tag.color }}
                            />
                            <span className="truncate flex-1">{tag.name}</span>
                            {showGameCounts && tag.game_count !== undefined && tag.game_count > 0 && (
                              <span className="text-xs text-muted-foreground flex-shrink-0">
                                {tag.game_count}
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

      {/* Selected tags display (when closed) */}
      {selectedTags.length > 3 && !open && (
        <div className="flex flex-wrap gap-1">
          {selectedTags.map((tag) => (
            <TagBadge
              key={tag.id}
              tag={{ ...tag, game_count: showGameCounts ? tag.game_count : undefined }}
              size="sm"
              onRemove={disabled ? undefined : () => handleRemoveTag(tag.id)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ============================================================================
// Compact Variant
// ============================================================================

export interface TagSelectorCompactProps {
  /** Currently selected tag IDs */
  selectedTagIds: string[];
  /** Available tags to choose from */
  availableTags: Tag[];
  /** Callback when selection changes */
  onChange: (selectedTagIds: string[]) => void;
  /** Whether the selector is disabled */
  disabled?: boolean;
  /** Additional class name */
  className?: string;
}

/**
 * A more compact tag selector that shows tags as a checkbox list.
 * Useful for forms or sidebars where space is limited.
 */
export function TagSelectorCompact({
  selectedTagIds,
  availableTags,
  onChange,
  disabled = false,
  className,
}: TagSelectorCompactProps) {
  const [searchQuery, setSearchQuery] = React.useState('');

  // Filter available tags based on search
  const filteredTags = React.useMemo(() => {
    if (!searchQuery.trim()) return availableTags;

    const query = searchQuery.toLowerCase();
    return availableTags.filter(
      (tag) =>
        tag.name.toLowerCase().includes(query) ||
        tag.description?.toLowerCase().includes(query)
    );
  }, [availableTags, searchQuery]);

  // Separate selected and unselected
  const selectedTags = filteredTags.filter((tag) => selectedTagIds.includes(tag.id));
  const unselectedTags = filteredTags.filter((tag) => !selectedTagIds.includes(tag.id));

  const handleToggle = (tagId: string) => {
    if (disabled) return;

    if (selectedTagIds.includes(tagId)) {
      onChange(selectedTagIds.filter((id) => id !== tagId));
    } else {
      onChange([...selectedTagIds, tagId]);
    }
  };

  const handleSelectAll = () => {
    if (disabled) return;
    const allIds = filteredTags.map((tag) => tag.id);
    onChange([...new Set([...selectedTagIds, ...allIds])]);
  };

  const handleClearAll = () => {
    if (disabled) return;
    onChange([]);
  };

  if (availableTags.length === 0) {
    return (
      <div className={cn('text-center py-8 text-muted-foreground', className)}>
        <TagIcon className="w-12 h-12 mx-auto mb-4 text-muted-foreground/50" />
        <p className="text-sm">No tags available</p>
        <p className="text-xs mt-1">Create some tags first to use them here.</p>
      </div>
    );
  }

  return (
    <div className={cn('space-y-4', className)}>
      {/* Search and controls */}
      <div className="space-y-3">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            type="text"
            className={cn(
              'flex h-10 w-full rounded-md border border-input bg-background pl-9 pr-3 py-2 text-sm',
              'ring-offset-background placeholder:text-muted-foreground',
              'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
              'disabled:cursor-not-allowed disabled:opacity-50'
            )}
            placeholder="Search tags..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            disabled={disabled}
          />
        </div>

        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground">
            {selectedTags.length} of {filteredTags.length} selected
          </span>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleSelectAll}
              disabled={disabled}
              className="text-xs text-primary hover:text-primary/80 font-medium disabled:text-muted-foreground disabled:cursor-not-allowed"
            >
              Select All
            </button>
            <span className="text-muted-foreground/50">|</span>
            <button
              type="button"
              onClick={handleClearAll}
              disabled={disabled || selectedTagIds.length === 0}
              className="text-xs text-primary hover:text-primary/80 font-medium disabled:text-muted-foreground disabled:cursor-not-allowed"
            >
              Clear
            </button>
          </div>
        </div>
      </div>

      {filteredTags.length === 0 ? (
        <div className="text-center py-8 text-muted-foreground">
          <Search className="w-8 h-8 mx-auto mb-2 text-muted-foreground/50" />
          <p className="text-sm">No tags matching &quot;{searchQuery}&quot;</p>
        </div>
      ) : (
        <ScrollArea className="max-h-96">
          <div className="space-y-4">
            {/* Selected tags */}
            {selectedTags.length > 0 && (
              <div>
                <h4 className="text-sm font-medium mb-2 flex items-center gap-2">
                  <Check className="w-4 h-4 text-green-600" />
                  Selected ({selectedTags.length})
                </h4>
                <div className="space-y-2">
                  {selectedTags.map((tag) => (
                    <TagListItem
                      key={tag.id}
                      tag={tag}
                      isSelected
                      disabled={disabled}
                      onToggle={() => handleToggle(tag.id)}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Available tags */}
            {unselectedTags.length > 0 && (
              <div>
                <h4 className="text-sm font-medium mb-2 flex items-center gap-2">
                  <TagIcon className="w-4 h-4 text-muted-foreground" />
                  Available ({unselectedTags.length})
                </h4>
                <div className="space-y-2">
                  {unselectedTags.map((tag) => (
                    <TagListItem
                      key={tag.id}
                      tag={tag}
                      isSelected={false}
                      disabled={disabled}
                      onToggle={() => handleToggle(tag.id)}
                    />
                  ))}
                </div>
              </div>
            )}
          </div>
        </ScrollArea>
      )}
    </div>
  );
}

interface TagListItemProps {
  tag: Tag;
  isSelected: boolean;
  disabled: boolean;
  onToggle: () => void;
}

function TagListItem({ tag, isSelected, disabled, onToggle }: TagListItemProps) {
  return (
    <label
      className={cn(
        'flex items-center gap-3 p-2 rounded-lg border transition-colors cursor-pointer',
        isSelected
          ? 'bg-green-50 border-green-200 dark:bg-green-950/20 dark:border-green-800'
          : 'bg-background border-border hover:bg-accent',
        disabled && 'opacity-50 cursor-not-allowed'
      )}
    >
      <input
        type="checkbox"
        className="h-4 w-4 rounded border-input text-primary focus:ring-primary"
        checked={isSelected}
        onChange={onToggle}
        disabled={disabled}
      />
      <div className="flex-1 flex items-center gap-2 min-w-0">
        <div
          className="w-3 h-3 rounded-full border border-muted-foreground/25 flex-shrink-0"
          style={{ backgroundColor: tag.color }}
        />
        <div className="min-w-0 flex-1">
          <div className="font-medium text-sm truncate">{tag.name}</div>
          {tag.description && (
            <div className="text-xs text-muted-foreground truncate">
              {tag.description}
            </div>
          )}
        </div>
      </div>
      {tag.game_count !== undefined && tag.game_count > 0 && (
        <span className="text-xs text-muted-foreground flex-shrink-0">
          {tag.game_count} game{tag.game_count !== 1 ? 's' : ''}
        </span>
      )}
    </label>
  );
}
