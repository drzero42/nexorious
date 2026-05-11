import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { PlatformIconList } from '@/components/ui/platform-icon';
import { config } from '@/lib/env';
import type { UserGame, PlayStatus } from '@/types';
import { cn } from '@/lib/utils';
import { Timer } from 'lucide-react';
import { formatTtb, formatIgdbRating } from '@/lib/game-utils';

export interface GameListProps {
  games: UserGame[];
  isLoading?: boolean;
  selectedIds?: Set<string>;
  onSelectGame?: (id: string) => void;
  onClickGame?: (game: UserGame) => void;
}

const statusColors: Record<PlayStatus, string> = {
  not_started: 'bg-gray-500',
  in_progress: 'bg-blue-500',
  completed: 'bg-green-500',
  mastered: 'bg-purple-500',
  dominated: 'bg-yellow-500',
  shelved: 'bg-orange-500',
  dropped: 'bg-red-500',
  replay: 'bg-cyan-500',
};

const statusLabels: Record<PlayStatus, string> = {
  not_started: 'Not Started',
  in_progress: 'In Progress',
  completed: 'Completed',
  mastered: 'Mastered',
  dominated: 'Dominated',
  shelved: 'Shelved',
  dropped: 'Dropped',
  replay: 'Replay',
};

function getCoverUrl(game: UserGame): string | null {
  if (game.game?.cover_art_url) {
    if (game.game.cover_art_url.startsWith('/')) {
      return `${config.staticUrl}${game.game.cover_art_url}`;
    }
    return game.game.cover_art_url;
  }
  return null;
}

function GameListSkeleton() {
  return (
    <>
      {Array.from({ length: 10 }).map((_, i) => (
        <TableRow key={i}>
          <TableCell>
            <Skeleton className="h-4 w-4" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-12 w-9" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-48" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-5 w-20" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-24" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-12" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-20" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-8" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-12" />
          </TableCell>
        </TableRow>
      ))}
    </>
  );
}

export function GameList({
  games,
  isLoading,
  selectedIds,
  onSelectGame,
  onClickGame,
}: GameListProps) {
  if (!isLoading && games.length === 0) {
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
    <Table>
      <TableHeader>
        <TableRow>
          {onSelectGame && <TableHead className="w-12" />}
          <TableHead className="w-16">Cover</TableHead>
          <TableHead>Title</TableHead>
          <TableHead className="w-28">Status</TableHead>
          <TableHead className="w-36">Platform(s)</TableHead>
          <TableHead className="w-20">Hours</TableHead>
          <TableHead className="w-32">Time to Beat</TableHead>
          <TableHead className="w-20">Rating</TableHead>
          <TableHead className="w-20">IGDB</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {isLoading ? (
          <GameListSkeleton />
        ) : (
          games.map((game) => {
            const coverUrl = getCoverUrl(game);
            const isSelected = selectedIds?.has(game.id);

            return (
              <TableRow
                key={game.id}
                className={cn(
                  'cursor-pointer',
                  isSelected && 'bg-muted'
                )}
                onClick={() => onClickGame?.(game)}
              >
                {onSelectGame && (
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={() => onSelectGame(game.id)}
                    />
                  </TableCell>
                )}
                <TableCell>
                  <div className="relative h-12 w-9 bg-muted rounded overflow-hidden">
                    {coverUrl ? (
                      <img
                        src={coverUrl}
                        alt={game.game?.title ?? 'Game cover'}
                        style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                        loading="lazy"
                      />
                    ) : (
                      <div className="w-full h-full flex items-center justify-center text-muted-foreground text-xs">
                        N/A
                      </div>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate max-w-xs">
                      {game.game?.title ?? 'Unknown Game'}
                    </span>
                    {game.is_loved && (
                      <span className="text-red-500 text-sm">&#9829;</span>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge
                    className={cn(
                      'text-white border-0',
                      statusColors[game.play_status]
                    )}
                  >
                    {statusLabels[game.play_status]}
                  </Badge>
                </TableCell>
                <TableCell>
                  <PlatformIconList
                    platforms={game.platforms ?? []}
                    size="sm"
                    showLabels
                  />
                </TableCell>
                <TableCell>
                  <span className="text-sm">{game.hours_played || 0}h</span>
                </TableCell>
                <TableCell>
                  {game.game?.howlongtobeat_main != null ||
                  game.game?.howlongtobeat_extra != null ||
                  game.game?.howlongtobeat_completionist != null ? (
                    <div className="flex items-center gap-1 text-xs text-muted-foreground">
                      <Timer className="h-3 w-3" />
                      <span>
                        {formatTtb(game.game?.howlongtobeat_main)} /{' '}
                        {formatTtb(game.game?.howlongtobeat_extra)} /{' '}
                        {formatTtb(game.game?.howlongtobeat_completionist)}
                      </span>
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell>
                  {game.personal_rating ? (
                    <div className="flex items-center gap-1">
                      <span className="text-yellow-400">&#9733;</span>
                      <span className="text-sm font-medium">
                        {game.personal_rating}
                      </span>
                    </div>
                  ) : (
                    <span className="text-sm text-muted-foreground">-</span>
                  )}
                </TableCell>
                <TableCell>
                  <span className="text-sm">{formatIgdbRating(game.game?.rating_average)}</span>
                </TableCell>
              </TableRow>
            );
          })
        )}
      </TableBody>
    </Table>
  );
}
