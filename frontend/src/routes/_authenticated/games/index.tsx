import { createFileRoute, Link, useNavigate, useSearch } from '@tanstack/react-router';
import { Suspense, useMemo, useCallback, useState, useEffect } from 'react';
import { Plus } from 'lucide-react';
import { useUserGames, useUserGameIds } from '@/hooks';
import {
  GameFilters,
  GameGrid,
  GameList,
  BulkActions,
  GamesPagination,
} from '@/components/games';
import { Button } from '@/components/ui/button';
import { toast } from 'sonner';
import type { PlayStatus, OwnershipStatus, UserGame, SelectionMode } from '@/types';

export const Route = createFileRoute('/_authenticated/games/')({
  component: GamesPage,
});

type SortField = 'title' | 'created_at' | 'howlongtobeat_main' | 'personal_rating' | 'release_date' | 'hours_played' | 'rating_average';
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
  { value: 'hours_played', label: 'Hours Played', defaultOrder: 'desc' },
  { value: 'rating_average', label: 'IGDB Rating', defaultOrder: 'desc' },
];

const VALID_PER_PAGE = [25, 50, 100, 500] as const;
type PerPage = typeof VALID_PER_PAGE[number];

function parsePerPage(raw: string): PerPage {
  const n = parseInt(raw, 10);
  return (VALID_PER_PAGE as readonly number[]).includes(n) ? (n as PerPage) : 50;
}

function GamesPageContent() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false });

  // Transient UI state (not persisted in URL)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [selectionMode, setSelectionMode] = useState<SelectionMode>('manual');

  // Read filters from URL params
  const filters = useMemo(() => {
    const statusParam = (search as Record<string, string>)['status'];
    const ownershipParam = (search as Record<string, string>)['ownership'];
    // Handle "null" string or empty string as undefined
    const status = statusParam && statusParam !== 'null' ? statusParam as PlayStatus : undefined;
    const ownershipStatus = ownershipParam && ownershipParam !== 'null' ? ownershipParam as OwnershipStatus : undefined;
    const s = search as Record<string, string | string[]>;
    const getAll = (key: string): string[] => {
      const val = s[key];
      if (!val) return [];
      return Array.isArray(val) ? val : [val];
    };
    return {
      search: (s['q'] as string) ?? '',
      status,
      ownershipStatus,
      platforms: getAll('platform'),
      storefronts: getAll('storefront'),
      genres: getAll('genre'),
      gameModes: getAll('gameMode'),
      themes: getAll('theme'),
      playerPerspectives: getAll('playerPerspective'),
      tags: getAll('tag'),
    };
  }, [search]);

  const s = search as Record<string, string>;
  const sortBy = (s['sort'] as SortField) ?? 'title';
  const sortOrder = (s['order'] as SortOrder) ?? 'asc';
  const viewMode = (s['view'] as 'grid' | 'list') ?? 'grid';
  const rawPage = parseInt(s['page'] ?? '1', 10);
  const currentPage = isNaN(rawPage) || rawPage < 1 ? 1 : rawPage;
  const currentPerPage = parsePerPage(s['perPage'] ?? '50');

  // Helper to update URL params
  const updateParams = useCallback((updates: Record<string, string | string[] | undefined>) => {
    const currentSearch = search as Record<string, string | string[]>;
    const params: Record<string, string | string[]> = { ...currentSearch };

    Object.entries(updates).forEach(([key, value]) => {
      if (value === undefined || value === '' || (Array.isArray(value) && value.length === 0)) {
        delete params[key];
      } else {
        params[key] = value;
      }
    });

    navigate({ to: '/games', search: params as Record<string, string>, replace: true });
  }, [navigate, search]);

  // Shared filter fields — no pagination params
  const filterFields = useMemo(
    () => ({
      search: filters.search || undefined,
      status: filters.status,
      ownershipStatus: filters.ownershipStatus,
      platform: filters.platforms.length > 0 ? filters.platforms : undefined,
      storefront: filters.storefronts.length > 0 ? filters.storefronts : undefined,
      genre: filters.genres.length > 0 ? filters.genres : undefined,
      gameMode: filters.gameModes.length > 0 ? filters.gameModes : undefined,
      theme: filters.themes.length > 0 ? filters.themes : undefined,
      playerPerspective: filters.playerPerspectives.length > 0 ? filters.playerPerspectives : undefined,
      tags: filters.tags.length > 0 ? filters.tags : undefined,
    }),
    [filters]
  );

  // Passed to useUserGames — includes page + perPage
  const listQueryParams = useMemo(
    () => ({
      ...filterFields,
      page: currentPage,
      perPage: currentPerPage,
      sortBy,
      sortOrder,
    }),
    [filterFields, currentPage, currentPerPage, sortBy, sortOrder]
  );

  // Passed to useUserGameIds — no page/perPage so "select all" spans all pages
  const idsQueryParams = useMemo(
    () => ({
      ...filterFields,
      sortBy,
      sortOrder,
    }),
    [filterFields, sortBy, sortOrder]
  );

  const { data, isLoading, refetch } = useUserGames(listQueryParams);
  const games = useMemo(() => data?.items ?? [], [data?.items]);
  const totalCount = data?.total ?? 0;
  const visibleCount = games.length;

  // Reset to page 1 if the URL page exceeds available pages after data loads
  useEffect(() => {
    if (data && data.pages > 0 && currentPage > data.pages) {
      updateParams({ page: undefined });
    }
  }, [data, currentPage, updateParams]);

  // Hook for fetching all IDs (disabled by default)
  const { refetch: fetchAllIds } = useUserGameIds(idsQueryParams, { enabled: false });

  // Wrap filter changes to also clear selection and update URL
  const handleFiltersChange = useCallback((newFilters: {
    search: string;
    status?: PlayStatus;
    ownershipStatus?: OwnershipStatus;
    platforms?: string[];
    storefronts?: string[];
    genres?: string[];
    gameModes?: string[];
    themes?: string[];
    playerPerspectives?: string[];
    tags?: string[];
  }) => {
    updateParams({
      q: newFilters.search || undefined,
      status: newFilters.status,
      ownership: newFilters.ownershipStatus,
      platform: newFilters.platforms,
      storefront: newFilters.storefronts,
      genre: newFilters.genres,
      gameMode: newFilters.gameModes,
      theme: newFilters.themes,
      playerPerspective: newFilters.playerPerspectives,
      tag: newFilters.tags,
      page: undefined,
    });
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [updateParams]);

  const handleSortByChange = useCallback((newSortBy: SortField) => {
    const option = SORT_OPTIONS.find((o) => o.value === newSortBy);
    const newOrder = option?.defaultOrder ?? 'asc';
    updateParams({ sort: newSortBy, order: newOrder, page: undefined });
  }, [updateParams]);

  const handleSortOrderToggle = useCallback(() => {
    const newOrder = sortOrder === 'asc' ? 'desc' : 'asc';
    updateParams({ order: newOrder, page: undefined });
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

  const handlePageChange = useCallback((page: number) => {
    updateParams({ page: page === 1 ? undefined : String(page) });
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [updateParams]);

  const handlePerPageChange = useCallback((perPage: number) => {
    updateParams({
      perPage: perPage === 50 ? undefined : String(perPage),
      page: undefined,
    });
    setSelectedIds(new Set());
    setSelectionMode('manual');
  }, [updateParams]);

  const handleClickGame = (game: UserGame) => {
    sessionStorage.setItem('games_list_return_url', JSON.stringify(search));
    navigate({ to: '/games/$id', params: { id: game.id } });
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
          <Link to="/games/add">
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

      {/* Top pagination bar — includes per-page selector */}
      {data && (
        <GamesPagination
          page={currentPage}
          perPage={currentPerPage}
          totalPages={data.pages}
          totalCount={data.total}
          onPageChange={handlePageChange}
          onPerPageChange={handlePerPageChange}
          showPerPageSelector={true}
        />
      )}

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

      {/* Bottom pagination bar — page navigation only */}
      {data && (
        <GamesPagination
          page={currentPage}
          perPage={currentPerPage}
          totalPages={data.pages}
          totalCount={data.total}
          onPageChange={handlePageChange}
          onPerPageChange={handlePerPageChange}
          showPerPageSelector={false}
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
          <Link to="/games/add">
            <Plus className="h-4 w-4 mr-2" />
            Add Game
          </Link>
        </Button>
      </div>
      <div className="text-muted-foreground">Loading...</div>
    </div>
  );
}

function GamesPage() {
  return (
    <Suspense fallback={<GamesPageLoading />}>
      <GamesPageContent />
    </Suspense>
  );
}
