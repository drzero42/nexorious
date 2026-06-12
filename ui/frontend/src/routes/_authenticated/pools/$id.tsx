import { useState } from 'react';
import { createFileRoute, useNavigate, useParams } from '@tanstack/react-router';
import { toast } from 'sonner';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { ArrowLeft, Filter, XCircle } from 'lucide-react';
import { usePool, useSetQueue, useAddPoolGame, useRemovePoolGame } from '@/hooks';
import { UpNextQueue } from '@/components/pools/up-next-queue';
import { CandidatesGrid } from '@/components/pools/candidates-grid';
import { SuggestionsGrid } from '@/components/pools/suggestions-grid';
import { PoolSortControl } from '@/components/pools/pool-sort-control';
import { PoolFilterEditor } from '@/components/pools/pool-filter-editor';
import { promoteToQueue } from '@/lib/pool-queue';
import type { SortField, SortOrder } from '@/lib/sort-options';

export const Route = createFileRoute('/_authenticated/pools/$id')({
  component: PoolDetailPage,
});

function PoolDetailPage() {
  const { id } = useParams({ from: '/_authenticated/pools/$id' });
  const navigate = useNavigate();
  const { data: pool, isLoading, error } = usePool(id);
  const setQueue = useSetQueue();
  const addGame = useAddPoolGame();
  const removeGame = useRemovePoolGame();

  const [candSort, setCandSort] = useState<SortField>('title');
  const [candOrder, setCandOrder] = useState<SortOrder>('asc');
  const [sugSort, setSugSort] = useState<SortField>('title');
  const [sugOrder, setSugOrder] = useState<SortOrder>('asc');
  const [sugPage, setSugPage] = useState(1);
  const [showFilter, setShowFilter] = useState(false);

  const openGame = (userGameId: string) =>
    navigate({ to: '/games/$id', params: { id: userGameId } });

  const handleSetQueue = (ids: string[]) => {
    if (!pool) return;
    setQueue.mutate(
      { poolId: pool.id, ids },
      { onError: () => toast.error('Failed to update queue') },
    );
  };

  const handlePromote = async (userGameId: string) => {
    if (!pool) return;
    const ids = promoteToQueue(
      pool.queue.map((g) => g.id),
      userGameId,
    );
    handleSetQueue(ids);
  };

  const handleAdd = (userGameId: string) => {
    if (!pool) return;
    addGame.mutate(
      { poolId: pool.id, userGameId },
      {
        onSuccess: () => toast.success('Added to pool'),
        onError: () => toast.error('Failed to add to pool'),
      },
    );
  };

  const handleRemove = (userGameId: string) => {
    if (!pool) return;
    removeGame.mutate(
      { poolId: pool.id, userGameId },
      { onError: () => toast.error('Failed to remove from pool') },
    );
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
    // A 404 (deleted in another tab) sends the user back to the index.
    toast.error('Pool not found');
    navigate({ to: '/pools' });
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <XCircle className="h-12 w-12 text-destructive" />
        <h2 className="mt-4 text-lg font-semibold">Pool not found</h2>
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

      <Card>
        <CardHeader>
          <CardTitle>Up Next</CardTitle>
        </CardHeader>
        <CardContent>
          <UpNextQueue queue={pool.queue} onSetQueue={handleSetQueue} onRemove={handleRemove} />
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
            poolId={pool.id}
            hasFilter={pool.has_filter}
            sortBy={sugSort}
            sortOrder={sugOrder}
            page={sugPage}
            onPageChange={setSugPage}
            onAdd={handleAdd}
            onOpen={openGame}
          />
        </CardContent>
      </Card>

      <PoolFilterEditor
        poolId={pool.id}
        open={showFilter}
        onOpenChange={setShowFilter}
        initialFilter={pool.filter}
      />
    </div>
  );
}
