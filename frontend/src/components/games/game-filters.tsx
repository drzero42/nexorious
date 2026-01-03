'use client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { MultiSelectFilter } from '@/components/ui/multi-select-filter';
import { useAllPlatforms, useAllStorefronts, useUserGameGenres, useAllTags } from '@/hooks';
import { PlayStatus } from '@/types';
import { ArrowDownAZ, ArrowUpAZ, ArrowDown, ArrowUp, Grid, List, X } from 'lucide-react';

type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date';
type SortOrder = 'asc' | 'desc';

interface SortOption {
  value: SortField;
  label: string;
}

const sortOptions: SortOption[] = [
  { value: 'title', label: 'Title' },
  { value: 'created_at', label: 'Date Added' },
  { value: 'howlongtobeat_main', label: 'Time to Beat' },
  { value: 'personal_rating', label: 'My Rating' },
  { value: 'release_date', label: 'Release Date' },
];

export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    platformId?: string;           // Keep for backwards compat (but will migrate to platforms)
    platforms?: string[];          // New: multi-select
    storefronts?: string[];        // New
    genres?: string[];             // New
    tags?: string[];               // New
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
  sortBy: SortField;
  sortOrder: SortOrder;
  onSortByChange: (sortBy: SortField) => void;
  onSortOrderToggle: () => void;
}

const statusOptions: { value: PlayStatus; label: string }[] = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.DROPPED, label: 'Dropped' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

export function GameFilters({
  filters,
  onFiltersChange,
  viewMode,
  onViewModeChange,
  sortBy,
  sortOrder,
  onSortByChange,
  onSortOrderToggle,
}: GameFiltersProps) {
  const { data: platforms } = useAllPlatforms();
  const { data: storefronts } = useAllStorefronts();
  const { data: genres } = useUserGameGenres();
  const { data: tags } = useAllTags();

  // Convert data to MultiSelectFilter options
  const platformOptions = platforms?.map((p) => ({ value: p.name, label: p.display_name })) ?? [];
  const storefrontOptions = storefronts?.map((s) => ({ value: s.name, label: s.display_name })) ?? [];
  const genreOptions = genres?.map((g) => ({ value: g, label: g })) ?? [];
  const tagOptions = tags?.map((t) => ({ value: t.name, label: t.name })) ?? [];

  const hasActiveFilters =
    filters.search ||
    filters.status ||
    filters.platformId ||
    (filters.platforms && filters.platforms.length > 0) ||
    (filters.storefronts && filters.storefronts.length > 0) ||
    (filters.genres && filters.genres.length > 0) ||
    (filters.tags && filters.tags.length > 0);

  const clearFilters = () => {
    onFiltersChange({
      search: '',
      status: undefined,
      platformId: undefined,
      platforms: [],
      storefronts: [],
      genres: [],
      tags: [],
    });
  };

  return (
    <div className="flex flex-wrap gap-4 items-center">
      {/* Search */}
      <Input
        type="search"
        placeholder="Search games..."
        value={filters.search}
        onChange={(e) => onFiltersChange({ ...filters, search: e.target.value })}
        className="w-64"
      />

      {/* Status filter */}
      <Select
        value={filters.status ?? 'all'}
        onValueChange={(value) =>
          onFiltersChange({
            ...filters,
            status: value === 'all' ? undefined : (value as PlayStatus),
          })
        }
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Status" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Statuses</SelectItem>
          {statusOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Platform filter (multi-select) */}
      <MultiSelectFilter
        label="Platforms"
        options={platformOptions}
        selected={filters.platforms ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, platforms: selected })}
      />

      {/* Storefront filter (multi-select) */}
      <MultiSelectFilter
        label="Storefronts"
        options={storefrontOptions}
        selected={filters.storefronts ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, storefronts: selected })}
      />

      {/* Genre filter (multi-select) */}
      <MultiSelectFilter
        label="Genres"
        options={genreOptions}
        selected={filters.genres ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, genres: selected })}
      />

      {/* Tags filter (multi-select) */}
      <MultiSelectFilter
        label="Tags"
        options={tagOptions}
        selected={filters.tags ?? []}
        onChange={(selected) => onFiltersChange({ ...filters, tags: selected })}
      />

      {/* Sort dropdown */}
      <Select
        value={sortBy}
        onValueChange={(value) => onSortByChange(value as SortField)}
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Sort by" />
        </SelectTrigger>
        <SelectContent>
          {sortOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {/* Sort direction toggle */}
      <Button
        variant="outline"
        size="icon"
        onClick={onSortOrderToggle}
        title={sortOrder === 'asc' ? 'Ascending' : 'Descending'}
      >
        {sortBy === 'title' ? (
          sortOrder === 'asc' ? (
            <ArrowDownAZ className="h-4 w-4" />
          ) : (
            <ArrowUpAZ className="h-4 w-4" />
          )
        ) : sortOrder === 'asc' ? (
          <ArrowUp className="h-4 w-4" />
        ) : (
          <ArrowDown className="h-4 w-4" />
        )}
      </Button>

      {/* Clear filters */}
      {hasActiveFilters && (
        <Button variant="ghost" size="sm" onClick={clearFilters}>
          <X className="h-4 w-4 mr-1" />
          Clear
        </Button>
      )}

      {/* Spacer */}
      <div className="flex-1" />

      {/* View toggle */}
      <div className="flex border rounded-md">
        <Button
          variant={viewMode === 'grid' ? 'secondary' : 'ghost'}
          size="sm"
          onClick={() => onViewModeChange('grid')}
        >
          <Grid className="h-4 w-4" />
        </Button>
        <Button
          variant={viewMode === 'list' ? 'secondary' : 'ghost'}
          size="sm"
          onClick={() => onViewModeChange('list')}
        >
          <List className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
