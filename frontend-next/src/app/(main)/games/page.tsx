'use client';

import { useState, useMemo } from 'react';
import { useUserGames } from '@/hooks';
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
} from '@/components/games';
import type { PlayStatus, UserGame } from '@/types';

export default function GamesPage() {
  const [filters, setFilters] = useState<{
    search: string;
    status?: PlayStatus;
    platformId?: string;
  }>({ search: '' });
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

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
  const games = data?.items ?? [];

  const handleSelectGame = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const clearSelection = () => setSelectedIds(new Set());

  const handleClickGame = (game: UserGame) => {
    // For now, just log - future: navigate to game details
    console.log('Clicked game:', game.id);
  };

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Game Library</h1>
        {data && (
          <span className="text-muted-foreground">
            {data.total} game{data.total !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {/* Filters */}
      <GameFilters
        filters={filters}
        onFiltersChange={setFilters}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />

      {/* Bulk actions */}
      <BulkActions
        selectedIds={selectedIds}
        onClearSelection={clearSelection}
        onSuccess={() => refetch()}
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
