import { SortableContext, horizontalListSortingStrategy } from '@dnd-kit/sortable';
import { Button } from '@/components/ui/button';
import { ArrowUpToLine, ArrowDownFromLine, X } from 'lucide-react';
import { SortablePoolCard, ZoneDroppable } from './pool-dnd';
import { ZONE_DROPPABLE_ID } from '@/lib/pool-dnd';
import { demoteFromQueue, setOnDeck } from '@/lib/pool-queue';
import type { UserGame } from '@/types';

interface UpNextQueueProps {
  queue: UserGame[];
  /** Declarative: the new ordered queue ids after any reorder/demote/on-deck op. */
  onSetQueue: (ids: string[]) => void;
  /** Remove from the pool entirely (calls removePoolGame, not setQueue). */
  onRemove: (userGameId: string) => void;
  /** Open the game's detail page. */
  onOpen: (userGameId: string) => void;
}

export function UpNextQueue({ queue, onSetQueue, onRemove, onOpen }: UpNextQueueProps) {
  const ids = queue.map((g) => g.id);

  return (
    <ZoneDroppable id={ZONE_DROPPABLE_ID.queue} className="min-h-24 p-1">
      {queue.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          Nothing queued yet — drag a candidate or suggestion here, or use the promote button.
        </p>
      ) : (
        <SortableContext items={ids} strategy={horizontalListSortingStrategy}>
          <div className="flex gap-3 overflow-x-auto pb-2">
            {queue.map((game, i) => (
              <SortablePoolCard
                key={game.id}
                game={game}
                onOpen={onOpen}
                playNext={i === 0}
                className="w-40 shrink-0"
                actions={
                  <div className="flex items-center justify-between">
                    {i !== 0 && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => onSetQueue(setOnDeck(ids, game.id))}
                        aria-label="Set play next"
                      >
                        <ArrowUpToLine className="h-4 w-4" />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onSetQueue(demoteFromQueue(ids, game.id))}
                      aria-label="Demote to candidate"
                    >
                      <ArrowDownFromLine className="h-4 w-4" />
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
