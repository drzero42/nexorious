import { useState, useEffect, useMemo } from 'react';
import { createFileRoute, useNavigate, useParams } from '@tanstack/react-router';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
  pointerWithin,
  closestCorners,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Filter, XCircle } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import * as poolsApi from '@/api/pools';
import {
  usePool,
  usePoolSuggestions,
  useSetQueue,
  useAddPoolGame,
  useRemovePoolGame,
  poolKeys,
} from '@/hooks';
import { UpNextQueue } from '@/components/pools/up-next-queue';
import { CandidatesGrid } from '@/components/pools/candidates-grid';
import { SuggestionsGrid } from '@/components/pools/suggestions-grid';
import { PoolSortControl } from '@/components/pools/pool-sort-control';
import { PoolFilterEditor } from '@/components/pools/pool-filter-editor';
import { promoteToQueue, reorderQueue } from '@/lib/pool-queue';
import { resolveZone, planTransition, type PoolZone } from '@/lib/pool-dnd';
import {
  applyQueueOrder,
  removeMember,
  addCandidate,
  addToQueue,
  removeSuggestion,
} from '@/lib/pool-cache';
import type { SortField, SortOrder } from '@/lib/sort-options';
import type { PoolDetail } from '@/types';
import type { UserGamesListResponse } from '@/api/games';

export const Route = createFileRoute('/_authenticated/pools/$id')({
  component: PoolDetailPage,
});

const SUGGESTIONS_PER_PAGE = 24;

// Pointer needs a small move to start a drag (so clicks still open the game);
// touch needs a brief press-and-hold (so a quick swipe still scrolls the page).
const useDragSensors = () =>
  useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 5 } }),
  );

// Prefer the droppable directly under the pointer; fall back to nearest corners
// when the pointer is outside every zone (e.g. mid-flight between zones).
const collisionDetection: CollisionDetection = (args) => {
  const pointer = pointerWithin(args);
  return pointer.length > 0 ? pointer : closestCorners(args);
};

function PoolDetailPage() {
  const { id } = useParams({ from: '/_authenticated/pools/$id' });
  const navigate = useNavigate();
  const { data: pool, isLoading, error } = usePool(id);
  const setQueue = useSetQueue();
  const addGame = useAddPoolGame();
  const removeGame = useRemovePoolGame();
  const sensors = useDragSensors();
  const qc = useQueryClient();

  const [candSort, setCandSort] = useState<SortField>('title');
  const [candOrder, setCandOrder] = useState<SortOrder>('asc');
  const [sugSort, setSugSort] = useState<SortField>('title');
  const [sugOrder, setSugOrder] = useState<SortOrder>('asc');
  const [sugPage, setSugPage] = useState(1);
  const [showFilter, setShowFilter] = useState(false);
  const [activeId, setActiveId] = useState<string | null>(null);

  const { data: sugData, isLoading: sugLoading } = usePoolSuggestions({
    poolId: id,
    sortBy: sugSort,
    sortOrder: sugOrder,
    page: sugPage,
    perPage: SUGGESTIONS_PER_PAGE,
  });
  // Suggestions = filter matches NOT already in the pool.
  const suggestionItems = useMemo(
    () => (sugData?.items ?? []).filter((g) => g.pool_membership == null),
    [sugData],
  );

  // Card id → zone, for resolving drag targets. The three zones are disjoint
  // (queued/candidate are members; suggestions are non-members).
  const cardZone = useMemo(() => {
    const map: Record<string, PoolZone> = {};
    pool?.queue.forEach((g) => (map[g.id] = 'queue'));
    pool?.candidates.forEach((g) => (map[g.id] = 'candidates'));
    suggestionItems.forEach((g) => (map[g.id] = 'suggestions'));
    return map;
  }, [pool, suggestionItems]);

  const activeGame = useMemo(() => {
    if (!activeId || !pool) return null;
    return (
      [...pool.queue, ...pool.candidates, ...suggestionItems].find((g) => g.id === activeId) ?? null
    );
  }, [activeId, pool, suggestionItems]);

  useEffect(() => {
    if (!isLoading && (error || !pool)) {
      toast.error('Pool not found');
      void navigate({ to: '/pools' });
    }
  }, [isLoading, error, pool, navigate]);

  const openGame = (userGameId: string) =>
    navigate({ to: '/games/$id', params: { id: userGameId } });

  // --- Optimistic cache helpers -------------------------------------------
  // Each handler edits the React Query cache immediately so a drag/button feels
  // instant, then rolls back on error. The mutation's own onSuccess invalidation
  // reconciles against the server. Suggestions live across paginated query keys,
  // so we snapshot/patch all of them under the pool's suggestions prefix.

  const detailKey = (poolId: string) => poolKeys.detail(poolId);
  const suggestionsKey = (poolId: string) => poolKeys.suggestions(poolId);

  /** Find a game in any cached suggestions page (needed to build optimistic moves). */
  const findSuggestion = (poolId: string, userGameId: string) => {
    for (const [, data] of qc.getQueriesData<UserGamesListResponse>({
      queryKey: suggestionsKey(poolId),
    })) {
      const found = data?.items.find((g) => g.id === userGameId);
      if (found) return found;
    }
    return undefined;
  };

  const patchDetail = (poolId: string, fn: (d: PoolDetail) => PoolDetail) => {
    const key = detailKey(poolId);
    const prev = qc.getQueryData<PoolDetail>(key);
    if (prev) qc.setQueryData(key, fn(prev));
    return prev;
  };

  const handleSetQueue = (ids: string[]) => {
    if (!pool) return;
    const prev = patchDetail(pool.id, (d) => applyQueueOrder(d, ids));
    setQueue.mutate(
      { poolId: pool.id, ids },
      {
        onError: () => {
          if (prev) qc.setQueryData(detailKey(pool.id), prev);
          toast.error('Failed to update queue');
        },
      },
    );
  };

  const handlePromote = (userGameId: string) => {
    if (!pool) return;
    handleSetQueue(
      promoteToQueue(
        pool.queue.map((g) => g.id),
        userGameId,
      ),
    );
  };

  const handleRemove = (userGameId: string) => {
    if (!pool) return;
    const prev = patchDetail(pool.id, (d) => removeMember(d, userGameId));
    removeGame.mutate(
      { poolId: pool.id, userGameId },
      {
        onError: () => {
          if (prev) qc.setQueryData(detailKey(pool.id), prev);
          toast.error('Failed to remove from pool');
        },
      },
    );
  };

  const handleAdd = (userGameId: string) => {
    if (!pool) return;
    const game = findSuggestion(pool.id, userGameId);
    const prevDetail = game ? patchDetail(pool.id, (d) => addCandidate(d, game)) : undefined;
    const prevSug = game
      ? qc.getQueriesData<UserGamesListResponse>({ queryKey: suggestionsKey(pool.id) })
      : [];
    if (game) {
      qc.setQueriesData<UserGamesListResponse>({ queryKey: suggestionsKey(pool.id) }, (old) =>
        old ? removeSuggestion(old, userGameId) : old,
      );
    }
    addGame.mutate(
      { poolId: pool.id, userGameId },
      {
        onSuccess: () => toast.success('Added to pool'),
        onError: () => {
          if (prevDetail) qc.setQueryData(detailKey(pool.id), prevDetail);
          prevSug.forEach(([k, d]) => qc.setQueryData(k, d));
          toast.error('Failed to add to pool');
        },
      },
    );
  };

  // Add a suggestion as a member, then promote it into the queue (suggestion → up-next).
  const addThenQueue = async (userGameId: string, queueIds: string[]) => {
    if (!pool) return;
    const game = findSuggestion(pool.id, userGameId);
    const prevDetail = game ? patchDetail(pool.id, (d) => addToQueue(d, game)) : undefined;
    const prevSug = game
      ? qc.getQueriesData<UserGamesListResponse>({ queryKey: suggestionsKey(pool.id) })
      : [];
    if (game) {
      qc.setQueriesData<UserGamesListResponse>({ queryKey: suggestionsKey(pool.id) }, (old) =>
        old ? removeSuggestion(old, userGameId) : old,
      );
    }
    // Call the raw API (not the add/setQueue hooks) so neither leg fires its own
    // onSuccess invalidation. The add hook's invalidation would refetch the pool
    // detail between the two calls, briefly returning the game as a candidate (the
    // queue PUT hasn't landed yet) and flickering the card through Candidates. We
    // invalidate once, after both calls settle, so the only reconcile sees the
    // final queued state.
    try {
      await poolsApi.addPoolGame(pool.id, userGameId);
    } catch {
      if (prevDetail) qc.setQueryData(detailKey(pool.id), prevDetail);
      prevSug.forEach(([k, d]) => qc.setQueryData(k, d));
      toast.error('Failed to add to pool');
      return;
    }
    try {
      // Add landed; queue PUT failure just falls through to reconcile (the game
      // stays a candidate server-side, which the final invalidation reflects).
      await poolsApi.setQueue(pool.id, [...queueIds, userGameId]);
    } catch {
      // ignore — reconciled by the invalidation below
    } finally {
      qc.invalidateQueries({ queryKey: detailKey(pool.id) });
      qc.invalidateQueries({ queryKey: suggestionsKey(pool.id) });
      qc.invalidateQueries({ queryKey: poolKeys.memberships(userGameId) });
      qc.invalidateQueries({ queryKey: poolKeys.lists() });
    }
  };

  const handleDragEnd = (event: DragEndEvent) => {
    setActiveId(null);
    const { active, over } = event;
    if (!pool || !over) return;
    const draggedId = String(active.id);
    const source = cardZone[draggedId];
    const target = resolveZone(String(over.id), cardZone);
    if (!source || !target) return;

    const queueIds: string[] = pool.queue.map((g) => g.id);
    switch (planTransition(source, target)) {
      case 'reorder': {
        const from = queueIds.indexOf(draggedId);
        // Dropped on a queue card → take its slot; dropped on the zone → end.
        let to = queueIds.indexOf(String(over.id));
        if (to < 0) to = queueIds.length - 1;
        if (from < 0 || from === to) return;
        handleSetQueue(reorderQueue(queueIds, from, to));
        break;
      }
      case 'promote':
        handleSetQueue([...queueIds, draggedId]);
        break;
      case 'add-candidate':
        handleAdd(draggedId);
        break;
      case 'add-and-queue':
        void addThenQueue(draggedId, queueIds);
        break;
      case 'demote':
        handleSetQueue(queueIds.filter((qid) => qid !== draggedId));
        break;
      case 'remove':
        handleRemove(draggedId);
        break;
      case 'noop':
        break;
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-48" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (error || !pool) {
    return (
      <div className="text-center py-12">
        <div className="mx-auto max-w-md">
          <XCircle className="mx-auto h-12 w-12 text-destructive" />
          <h3 className="mt-4 text-lg font-medium">Pool not found</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            The requested pool could not be found.
          </p>
          <div className="mt-6">
            <Button onClick={() => navigate({ to: '/pools' })}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Pools
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" onClick={() => navigate({ to: '/pools' })}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <h1 className="flex items-center gap-2 text-2xl font-bold">
            {pool.color && (
              <span
                className="h-4 w-4 rounded-full border"
                style={{ backgroundColor: pool.color }}
              />
            )}
            {pool.name}
          </h1>
        </div>
        <Button variant="outline" onClick={() => setShowFilter(true)}>
          <Filter className="mr-2 h-4 w-4" />
          {pool.has_filter ? 'Edit filter' : 'Add filter'}
        </Button>
      </div>

      <DndContext
        sensors={sensors}
        collisionDetection={collisionDetection}
        onDragStart={(e: DragStartEvent) => setActiveId(String(e.active.id))}
        onDragCancel={() => setActiveId(null)}
        onDragEnd={handleDragEnd}
      >
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Up Next</CardTitle>
            </CardHeader>
            <CardContent>
              <UpNextQueue
                queue={pool.queue}
                onSetQueue={handleSetQueue}
                onRemove={handleRemove}
                onOpen={openGame}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>Candidates ({pool.candidates.length})</CardTitle>
              <PoolSortControl
                sortBy={candSort}
                sortOrder={candOrder}
                onSortByChange={setCandSort}
                onSortOrderChange={setCandOrder}
              />
            </CardHeader>
            <CardContent>
              <CandidatesGrid
                candidates={pool.candidates}
                sortBy={candSort}
                sortOrder={candOrder}
                onPromote={handlePromote}
                onRemove={handleRemove}
                onOpen={openGame}
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>Suggestions</CardTitle>
              <PoolSortControl
                sortBy={sugSort}
                sortOrder={sugOrder}
                onSortByChange={(f) => {
                  setSugSort(f);
                  setSugPage(1);
                }}
                onSortOrderChange={(o) => {
                  setSugOrder(o);
                  setSugPage(1);
                }}
              />
            </CardHeader>
            <CardContent>
              <SuggestionsGrid
                items={suggestionItems}
                isLoading={sugLoading}
                hasFilter={pool.has_filter}
                page={sugPage}
                pages={sugData?.pages ?? 1}
                onPageChange={setSugPage}
                onAdd={handleAdd}
                onOpen={openGame}
              />
            </CardContent>
          </Card>
        </div>

        <DragOverlay dropAnimation={null}>
          {activeGame ? (
            <div className="w-40">
              <GameCard game={activeGame} />
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>

      <PoolFilterEditor
        poolId={pool.id}
        open={showFilter}
        onOpenChange={setShowFilter}
        initialFilter={pool.filter}
      />
    </div>
  );
}
