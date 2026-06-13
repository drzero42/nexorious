import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, useSortable, horizontalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { GripVertical, ArrowUpToLine, ArrowDownFromLine, X } from 'lucide-react';
import { GameCard } from '@/components/games/game-card';
import { reorderQueue, demoteFromQueue, setOnDeck } from '@/lib/pool-queue';
import type { UserGame, UserGameId } from '@/types';

interface UpNextQueueProps {
  queue: UserGame[];
  /** Declarative: the new ordered queue ids after any reorder/demote/on-deck op. */
  onSetQueue: (ids: string[]) => void;
  /** Remove from the pool entirely (calls removePoolGame, not setQueue). */
  onRemove: (userGameId: string) => void;
}

function QueueItem({
  game,
  onDeck,
  onSetOnDeck,
  onDemote,
  onRemove,
}: {
  game: UserGame;
  onDeck: boolean;
  onSetOnDeck: () => void;
  onDemote: () => void;
  onRemove: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: game.id,
  });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`w-40 shrink-0 ${isDragging ? 'opacity-50' : ''}`}
    >
      {onDeck && (
        <Badge className="mb-1 w-full justify-center" variant="default">
          Play Next
        </Badge>
      )}
      <GameCard
        game={game}
        topRightSlot={
          <button
            type="button"
            className="cursor-grab touch-none rounded bg-background/80 p-1"
            aria-label="Drag to reorder"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="h-4 w-4" />
          </button>
        }
        actionsSlot={
          <div className="flex items-center justify-between">
            {!onDeck && (
              <Button variant="ghost" size="sm" onClick={onSetOnDeck} aria-label="Set on deck">
                <ArrowUpToLine className="h-4 w-4" />
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={onDemote} aria-label="Demote to candidate">
              <ArrowDownFromLine className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="sm" onClick={onRemove} aria-label="Remove from pool">
              <X className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        }
      />
    </div>
  );
}

export function UpNextQueue({ queue, onSetQueue, onRemove }: UpNextQueueProps) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));
  const ids = queue.map((g) => g.id);

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const from = ids.indexOf(active.id as UserGameId);
    const to = ids.indexOf(over.id as UserGameId);
    if (from < 0 || to < 0) return;
    onSetQueue(reorderQueue(ids, from, to));
  };

  if (queue.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        Nothing queued yet — promote a candidate or a suggestion.
      </p>
    );
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={ids} strategy={horizontalListSortingStrategy}>
        <div className="flex gap-3 overflow-x-auto pb-2">
          {queue.map((game, i) => (
            <QueueItem
              key={game.id}
              game={game}
              onDeck={i === 0}
              onSetOnDeck={() => onSetQueue(setOnDeck(ids, game.id))}
              onDemote={() => onSetQueue(demoteFromQueue(ids, game.id))}
              onRemove={() => onRemove(game.id)}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}
