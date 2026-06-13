import type { ReactNode } from 'react';
import { useSortable } from '@dnd-kit/sortable';
import { useDroppable } from '@dnd-kit/core';
import { CSS } from '@dnd-kit/utilities';
import { Badge } from '@/components/ui/badge';
import { GameCard } from '@/components/games/game-card';
import { cn } from '@/lib/utils';
import type { UserGame } from '@/types';

interface SortablePoolCardProps {
  game: UserGame;
  onOpen: (userGameId: string) => void;
  /** Per-card action row (promote / demote / remove / add). */
  actions?: ReactNode;
  /** Marks the on-deck card in the queue with a "Play Next" badge. */
  playNext?: boolean;
  /** Extra classes for the draggable wrapper (e.g. a fixed width in the queue). */
  className?: string;
}

/**
 * A draggable + clickable game card. The whole cover is the drag handle (the
 * grab cursor signals it); a plain click still opens the game because the
 * DndContext's sensors only start a drag after a small move / press-and-hold.
 */
export function SortablePoolCard({
  game,
  onOpen,
  actions,
  playNext,
  className,
}: SortablePoolCardProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: game.id,
  });
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={cn(isDragging && 'opacity-50', className)}
      {...attributes}
      {...listeners}
    >
      {playNext && (
        <Badge className="mb-1 w-full justify-center" variant="default">
          Play Next
        </Badge>
      )}
      <GameCard
        game={game}
        onClick={() => onOpen(game.id)}
        actionsSlot={actions}
        className="cursor-grab active:cursor-grabbing"
      />
    </div>
  );
}

interface ZoneDroppableProps {
  id: string;
  className?: string;
  children: ReactNode;
}

/** Drop target wrapping a zone's content; highlights while a drag hovers it. */
export function ZoneDroppable({ id, className, children }: ZoneDroppableProps) {
  const { setNodeRef, isOver } = useDroppable({ id });
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'rounded-md transition-colors',
        isOver && 'bg-accent/40 ring-2 ring-primary/40',
        className,
      )}
    >
      {children}
    </div>
  );
}
