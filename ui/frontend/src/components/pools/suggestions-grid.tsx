import { SortableContext, rectSortingStrategy } from '@dnd-kit/sortable';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Plus } from 'lucide-react';
import { SortablePoolCard, ZoneDroppable } from './pool-dnd';
import { ZONE_DROPPABLE_ID } from '@/lib/pool-dnd';
import type { UserGame } from '@/types';

interface SuggestionsGridProps {
  /** Filter-matching games not already in the pool (parent pre-filters). */
  items: UserGame[];
  isLoading: boolean;
  hasFilter: boolean;
  page: number;
  pages: number;
  onPageChange: (page: number) => void;
  onAdd: (userGameId: string) => void;
  onOpen: (userGameId: string) => void;
}

export function SuggestionsGrid({
  items,
  isLoading,
  hasFilter,
  page,
  pages,
  onPageChange,
  onAdd,
  onOpen,
}: SuggestionsGridProps) {
  // The zone is always a drop target (so a candidate/queued card can be dragged
  // here to leave the pool), even when there is nothing to suggest.
  let body: React.ReactNode;
  if (!hasFilter) {
    body = (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Add a filter to get suggestions.
      </p>
    );
  } else if (isLoading) {
    body = (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="aspect-[3/4]" />
        ))}
      </div>
    );
  } else if (items.length === 0) {
    body = <p className="py-8 text-center text-sm text-muted-foreground">No matches right now.</p>;
  } else {
    body = (
      <SortableContext items={items.map((g) => g.id)} strategy={rectSortingStrategy}>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {items.map((game) => (
            <SortablePoolCard
              key={game.id}
              game={game}
              onOpen={onOpen}
              actions={
                <Button
                  variant="outline"
                  size="sm"
                  className="w-full"
                  onClick={() => onAdd(game.id)}
                >
                  <Plus className="mr-1 h-4 w-4" /> Add
                </Button>
              }
            />
          ))}
        </div>
      </SortableContext>
    );
  }

  return (
    <div className="space-y-4">
      <ZoneDroppable id={ZONE_DROPPABLE_ID.suggestions} className="min-h-24 p-1">
        {body}
      </ZoneDroppable>
      {hasFilter && pages > 1 && (
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
            Page {page} of {pages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= pages}
            onClick={() => onPageChange(page + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
