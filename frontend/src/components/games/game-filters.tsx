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
import { useAllPlatforms } from '@/hooks';
import { PlayStatus } from '@/types';
import { Grid, List, X } from 'lucide-react';

export interface GameFiltersProps {
  filters: {
    search: string;
    status?: PlayStatus;
    platformId?: string;
  };
  onFiltersChange: (filters: GameFiltersProps['filters']) => void;
  viewMode: 'grid' | 'list';
  onViewModeChange: (mode: 'grid' | 'list') => void;
}

const statusOptions: { value: PlayStatus; label: string }[] = [
  { value: PlayStatus.NOT_STARTED, label: 'Not Started' },
  { value: PlayStatus.IN_PROGRESS, label: 'In Progress' },
  { value: PlayStatus.COMPLETED, label: 'Completed' },
  { value: PlayStatus.MASTERED, label: 'Mastered' },
  { value: PlayStatus.DOMINATED, label: 'Dominated' },
  { value: PlayStatus.SHELVED, label: 'Shelved' },
  { value: PlayStatus.REPLAY, label: 'Replay' },
];

export function GameFilters({
  filters,
  onFiltersChange,
  viewMode,
  onViewModeChange,
}: GameFiltersProps) {
  const { data: platforms } = useAllPlatforms();

  const hasActiveFilters = filters.search || filters.status || filters.platformId;

  const clearFilters = () => {
    onFiltersChange({ search: '' });
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

      {/* Platform filter */}
      <Select
        value={filters.platformId ?? 'all'}
        onValueChange={(value) =>
          onFiltersChange({
            ...filters,
            platformId: value === 'all' ? undefined : value,
          })
        }
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Platform" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Platforms</SelectItem>
          {platforms?.map((platform) => (
            <SelectItem key={platform.id} value={platform.id}>
              {platform.display_name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

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
