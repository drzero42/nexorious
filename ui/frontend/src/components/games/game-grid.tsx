import { GameCard } from './game-card';
import { Skeleton } from '@/components/ui/skeleton';
import type { UserGame } from '@/types';

const gridClasses = 'grid grid-cols-[repeat(auto-fill,minmax(min(180px,45%),1fr))] gap-4';

export interface GameGridProps {
  games: UserGame[];
  isLoading?: boolean;
  selectedIds?: Set<string>;
  onSelectGame?: (id: string) => void;
  onClickGame?: (game: UserGame) => void;
}

function GameCardSkeleton() {
  return (
    <div className="space-y-3">
      <Skeleton className="aspect-[3/4] w-full" />
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-3 w-1/2" />
    </div>
  );
}

export function GameGrid({
  games,
  isLoading,
  selectedIds,
  onSelectGame,
  onClickGame,
}: GameGridProps) {
  if (isLoading) {
    return (
      <div className={gridClasses}>
        {Array.from({ length: 12 }).map((_, i) => (
          <GameCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  if (games.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>No games found</p>
        <p className="text-sm">Try adjusting your filters or add some games to your library.</p>
      </div>
    );
  }

  return (
    <div className={gridClasses}>
      {games.map((game) => (
        <GameCard
          key={game.id}
          game={game}
          selected={selectedIds?.has(game.id)}
          onSelect={onSelectGame}
          onClick={() => onClickGame?.(game)}
        />
      ))}
    </div>
  );
}
