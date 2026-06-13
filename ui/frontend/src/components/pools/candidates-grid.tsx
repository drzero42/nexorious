import { useMemo } from 'react';
import { SortableContext, rectSortingStrategy } from '@dnd-kit/sortable';
import { Button } from '@/components/ui/button';
import { ArrowUpToLine, X } from 'lucide-react';
import { SortablePoolCard, ZoneDroppable } from './pool-dnd';
import { ZONE_DROPPABLE_ID } from '@/lib/pool-dnd';
import type { UserGame } from '@/types';
import type { SortField, SortOrder } from '@/lib/sort-options';

interface CandidatesGridProps {
  candidates: UserGame[];
  sortBy: SortField;
  sortOrder: SortOrder;
  onPromote: (userGameId: string) => void;
  onRemove: (userGameId: string) => void;
  onOpen: (userGameId: string) => void;
}

/** Mirror the library's field semantics for the client-side sort. */
function compare(a: UserGame, b: UserGame, field: SortField): number {
  switch (field) {
    case 'title':
      return (a.game?.title ?? '').localeCompare(b.game?.title ?? '');
    case 'created_at':
      return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
    case 'howlongtobeat_main':
      return (a.game?.howlongtobeat_main ?? 0) - (b.game?.howlongtobeat_main ?? 0);
    case 'personal_rating':
      return (a.personal_rating ?? 0) - (b.personal_rating ?? 0);
    case 'release_date':
      return (
        new Date(a.game?.release_date ?? 0).getTime() -
        new Date(b.game?.release_date ?? 0).getTime()
      );
    case 'hours_played':
      return a.hours_played - b.hours_played;
    case 'rating_average':
      return (a.game?.rating_average ?? 0) - (b.game?.rating_average ?? 0);
  }
}

export function CandidatesGrid({
  candidates,
  sortBy,
  sortOrder,
  onPromote,
  onRemove,
  onOpen,
}: CandidatesGridProps) {
  const sorted = useMemo(() => {
    const arr = [...candidates];
    arr.sort((a, b) => {
      const c = compare(a, b, sortBy);
      return sortOrder === 'asc' ? c : -c;
    });
    return arr;
  }, [candidates, sortBy, sortOrder]);

  return (
    <ZoneDroppable id={ZONE_DROPPABLE_ID.candidates} className="min-h-24 p-1">
      {candidates.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          No candidates yet — drag games here from Suggestions or the library.
        </p>
      ) : (
        <SortableContext items={sorted.map((g) => g.id)} strategy={rectSortingStrategy}>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {sorted.map((game) => (
              <SortablePoolCard
                key={game.id}
                game={game}
                onOpen={onOpen}
                actions={
                  <div className="flex items-center justify-between">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onPromote(game.id)}
                      aria-label="Promote to queue"
                    >
                      <ArrowUpToLine className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onRemove(game.id)}
                      aria-label="Remove from pool"
                    >
                      <X className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                }
              />
            ))}
          </div>
        </SortableContext>
      )}
    </ZoneDroppable>
  );
}
