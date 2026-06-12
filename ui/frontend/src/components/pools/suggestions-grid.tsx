import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Plus } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import { usePoolSuggestions } from '@/hooks';
import type { SortField, SortOrder } from '@/lib/sort-options';

interface SuggestionsGridProps {
  poolId: string;
  hasFilter: boolean;
  sortBy: SortField;
  sortOrder: SortOrder;
  page: number;
  onPageChange: (page: number) => void;
  onAdd: (userGameId: string) => void;
  onOpen: (userGameId: string) => void;
}

const PER_PAGE = 24;

export function SuggestionsGrid({
  poolId,
  hasFilter,
  sortBy,
  sortOrder,
  page,
  onPageChange,
  onAdd,
  onOpen,
}: SuggestionsGridProps) {
  const { data, isLoading } = usePoolSuggestions({
    poolId,
    sortBy,
    sortOrder,
    page,
    perPage: PER_PAGE,
  });

  if (!hasFilter) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Add a filter to get suggestions.
      </p>
    );
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="aspect-[3/4]" />
        ))}
      </div>
    );
  }

  // Suggestions = filter matches NOT already in the pool.
  const items = (data?.items ?? []).filter((g) => g.pool_membership == null);

  if (items.length === 0) {
    return <p className="py-8 text-center text-sm text-muted-foreground">No matches right now.</p>;
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {items.map((game) => (
          <GameCard
            key={game.id}
            game={game}
            onClick={() => onOpen(game.id)}
            actionsSlot={
              <Button variant="outline" size="sm" className="w-full" onClick={() => onAdd(game.id)}>
                <Plus className="mr-1 h-4 w-4" /> Add
              </Button>
            }
          />
        ))}
      </div>
      {data && data.pages > 1 && (
        <div className="flex items-center justify-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => onPageChange(page - 1)}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {data.page} of {data.pages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= data.pages}
            onClick={() => onPageChange(page + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
