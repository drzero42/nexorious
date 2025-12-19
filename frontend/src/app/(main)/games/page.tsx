'use client';

import { useState, useMemo, useCallback } from 'react';
import { useRouter } from 'next/navigation';
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

export default function GamesPage() {
  const router = useRouter();
  const [filters, setFilters] = useState<{
    search: string;
    status?: PlayStatus;
    platformId?: string;
  }>({ search: '' });
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectionMode, setSelectionMode] = useState<SelectionMode>('manual');

  // Build query params
  const queryParams = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      platformId: filters.platformId,
      perPage: 50,
    }),
    [filters]
  );

  const { data, isLoading, refetch } = useUserGames(queryParams);
  const games = useMemo(() => data?.items ?? [], [data?.items]);
  const totalCount = data?.total ?? 0;
  const visibleCount = games.length;

  // Hook for fetching all IDs (disabled by default)
  const { refetch: fetchAllIds } = useUserGameIds(queryParams, { enabled: false });

  // Wrap setFilters to also clear selection
  const handleFiltersChange = useCallback((newFilters: typeof filters) => {
    setFilters(newFilters);
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, []);

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
        onViewModeChange={setViewMode}
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
