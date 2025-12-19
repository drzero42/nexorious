'use client';

import { GameCard } from './game-card';
import { Skeleton } from '@/components/ui/skeleton';
import type { UserGame } from '@/types';

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
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
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
        <p className="text-sm">
          Try adjusting your filters or add some games to your library.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
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
