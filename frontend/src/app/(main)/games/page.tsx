'use client';

import { Suspense, useMemo, useCallback } from 'react';
import { useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { Plus } from 'lucide-react';
import { useUserGames, useUserGameIds } from '@/hooks';
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
} from '@/components/games';
import { Button } from '@/components/ui/button';
import { toast } from 'sonner';
import type { PlayStatus, UserGame, SelectionMode } from '@/types';

type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date';
type SortOrder = 'asc' | 'desc';

interface SortOption {
  value: SortField;
  label: string;
  defaultOrder: SortOrder;
}

const SORT_OPTIONS: SortOption[] = [
  { value: 'title', label: 'Title', defaultOrder: 'asc' },
  { value: 'created_at', label: 'Date Added', defaultOrder: 'desc' },
  { value: 'howlongtobeat_main', label: 'Time to Beat', defaultOrder: 'asc' },
  { value: 'personal_rating', label: 'My Rating', defaultOrder: 'desc' },
  { value: 'release_date', label: 'Release Date', defaultOrder: 'desc' },
];

function GamesPageContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Transient UI state (not persisted in URL)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectionMode, setSelectionMode] = useState<SelectionMode>('manual');

  // Read filters from URL params
  const filters = useMemo(() => {
    const statusParam = searchParams.get('status');
    // Handle "null" string or empty string as undefined
    const status = statusParam && statusParam !== 'null' ? statusParam as PlayStatus : undefined;
    return {
      search: searchParams.get('q') ?? '',
      status,
      platforms: searchParams.getAll('platform'),
      storefronts: searchParams.getAll('storefront'),
      genres: searchParams.getAll('genre'),
      tags: searchParams.getAll('tag'),
    };
  }, [searchParams]);

  const sortBy = (searchParams.get('sort') as SortField) ?? 'title';
  const sortOrder = (searchParams.get('order') as SortOrder) ?? 'asc';
  const viewMode = (searchParams.get('view') as 'grid' | 'list') ?? 'grid';

  // Helper to update URL params
  const updateParams = useCallback((updates: Record<string, string | string[] | undefined>) => {
    const params = new URLSearchParams(searchParams.toString());

    Object.entries(updates).forEach(([key, value]) => {
      params.delete(key);  // Remove existing values for this key

      if (value === undefined || value === '' || (Array.isArray(value) && value.length === 0)) {
        return;  // Don't add empty values
      }

      if (Array.isArray(value)) {
        value.forEach((v) => params.append(key, v));
      } else {
        params.set(key, value);
      }
    });

    const queryString = params.toString();
    router.replace(queryString ? `/games?${queryString}` : '/games', { scroll: false });
  }, [router, searchParams]);

  // Build query params for API
  const queryParams = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      platform: filters.platforms.length > 0 ? filters.platforms : undefined,
      storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
      genre: filters.genres.length > 0 ? filters.genres : undefined,
      tags: filters.tags.length > 0 ? filters.tags : undefined,
      perPage: 50,
      sortBy,
      sortOrder,
    }),
    [filters, sortBy, sortOrder]
  );

  const { data, isLoading, refetch } = useUserGames(queryParams);
  const games = useMemo(() => data?.items ?? [], [data?.items]);
  const totalCount = data?.total ?? 0;
  const visibleCount = games.length;

  // Hook for fetching all IDs (disabled by default)
  const { refetch: fetchAllIds } = useUserGameIds(queryParams, { enabled: false });

  // Wrap filter changes to also clear selection and update URL
  const handleFiltersChange = useCallback((newFilters: {
    search: string;
    status?: PlayStatus;
    platforms?: string[];
    storefronts?: string[];
    genres?: string[];
    tags?: string[];
  }) => {
    updateParams({
      q: newFilters.search || undefined,
      status: newFilters.status,
      platform: newFilters.platforms,
      storefront: newFilters.storefronts,
      genre: newFilters.genres,
      tag: newFilters.tags,
    });
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [updateParams]);

  const handleSortByChange = useCallback((newSortBy: SortField) => {
    const option = SORT_OPTIONS.find((o) => o.value === newSortBy);
    const newOrder = option?.defaultOrder ?? 'asc';
    updateParams({ sort: newSortBy, order: newOrder });
  }, [updateParams]);

  const handleSortOrderToggle = useCallback(() => {
    const newOrder = sortOrder === 'asc' ? 'desc' : 'asc';
    updateParams({ order: newOrder });
  }, [sortOrder, updateParams]);

  const handleViewModeChange = useCallback((mode: 'grid' | 'list') => {
    updateParams({ view: mode === 'grid' ? undefined : mode }); // Don't store default 'grid' in URL
  }, [updateParams]);

  const handleSelectGame = useCallback((id: string) => {
    setSelectionMode('manual');
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, []);

  const handleSelectAllClick = useCallback(async () => {
    // Check if visible = total (collapsed state - only two states needed)
    const isCollapsedState = visibleCount === totalCount;

    if (selectionMode === 'manual' || (selectionMode === 'all-visible' && isCollapsedState)) {
      // Not all selected OR in collapsed state with all selected -> cycle appropriately
      if (selectedIds.size === 0 || selectionMode === 'manual') {
        // Select all visible
        const visibleIds = new Set(games.map((g) => g.id));
        setSelectedIds(visibleIds);
        setSelectionMode(isCollapsedState ? 'all-collection' : 'all-visible');
      } else if (isCollapsedState) {
        // In collapsed state, second click clears
        clearSelection();
      }
    } else if (selectionMode === 'all-visible') {
      // All visible selected -> select all in collection
      try {
        const result = await fetchAllIds();
        if (result.data) {
          setSelectedIds(new Set(result.data));
          setSelectionMode('all-collection');
        }
      } catch {
        toast.error('Failed to select all games');
      }
    } else if (selectionMode === 'all-collection') {
      // All in collection selected -> clear
      clearSelection();
    }
  }, [selectionMode, selectedIds.size, games, visibleCount, totalCount, fetchAllIds, clearSelection]);

  const handleClickGame = (game: UserGame) => {
    router.push(`/games/${game.id}`);
  };

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">Game Library</h1>
          {data && (
            <span className="text-muted-foreground">
              {data.total} game{data.total !== 1 ? 's' : ''}
            </span>
          )}
        </div>
        <Button asChild>
          <Link href="/games/add">
            <Plus className="h-4 w-4 mr-2" />
            Add Game
          </Link>
        </Button>
      </div>

      {/* Filters */}
      <GameFilters
        filters={filters}
        onFiltersChange={handleFiltersChange}
        viewMode={viewMode}
        onViewModeChange={handleViewModeChange}
        sortBy={sortBy}
        sortOrder={sortOrder}
        onSortByChange={handleSortByChange}
        onSortOrderToggle={handleSortOrderToggle}
      />

      {/* Bulk actions */}
      <BulkActions
        selectedIds={selectedIds}
        onClearSelection={clearSelection}
        onSuccess={() => refetch()}
        selectionMode={selectionMode}
        visibleCount={visibleCount}
        totalCount={totalCount}
        onSelectAllClick={handleSelectAllClick}
      />

      {/* Games display */}
      {viewMode === 'grid' ? (
        <GameGrid
          games={games}
          isLoading={isLoading}
          selectedIds={selectedIds}
          onSelectGame={handleSelectGame}
          onClickGame={handleClickGame}
        />
      ) : (
        <GameList
          games={games}
          isLoading={isLoading}
          selectedIds={selectedIds}
          onSelectGame={handleSelectGame}
          onClickGame={handleClickGame}
        />
      )}
    </div>
  );
}

// Loading fallback for Suspense
function GamesPageLoading() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold">Game Library</h1>
        </div>
        <Button asChild>
          <Link href="/games/add">
            <Plus className="h-4 w-4 mr-2" />
            Add Game
          </Link>
        </Button>
      </div>
      <div className="text-muted-foreground">Loading...</div>
    </div>
  );
}

export default function GamesPage() {
  return (
    <Suspense fallback={<GamesPageLoading />}>
      <GamesPageContent />
    </Suspense>
  );
}
