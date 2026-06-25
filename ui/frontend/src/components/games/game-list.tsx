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
import type { UserGame } from '@/types';
import { cn } from '@/lib/utils';
import { Timer } from 'lucide-react';
import { useMediaQuery } from '@/hooks';
import { formatTtb, formatIgdbRating, formatHoursPlayed, getCoverUrl } from '@/lib/game-utils';
import { statusColors, statusLabels } from '@/lib/play-status';

// Below this width the data-dense table overflows the viewport, so we render a
// stacked card layout instead. `lg` (not `md`) because the sidebar reappears at
// `md`, leaving too little room for the full table on tablets.
const COMPACT_QUERY = '(max-width: 1023px)';

export interface GameListProps {
  games: UserGame[];
  isLoading?: boolean;
  selectedIds?: Set<string>;
  onSelectGame?: (id: string) => void;
  onClickGame?: (game: UserGame) => void;
}

function GameCover({ game, className }: { game: UserGame; className?: string }) {
  const coverUrl = getCoverUrl(game);
  return (
    <div className={cn('relative bg-muted rounded overflow-hidden', className)}>
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
  );
}

function TableSkeleton({ withSelection }: { withSelection: boolean }) {
  return (
    <>
      {Array.from({ length: 10 }).map((_, i) => (
        <TableRow key={i}>
          {withSelection && (
            <TableCell>
              <Skeleton className="h-4 w-4" />
            </TableCell>
          )}
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

function CardSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 10 }).map((_, i) => (
        <div key={i} className="flex gap-3 rounded-lg border p-3">
          <Skeleton className="h-16 w-12 shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-4 w-32" />
          </div>
        </div>
      ))}
    </div>
  );
}

function GameStatRow({ game }: { game: UserGame }) {
  const hasTtb =
    game.game?.howlongtobeat_main != null ||
    game.game?.howlongtobeat_extra != null ||
    game.game?.howlongtobeat_completionist != null;

  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      <span className="flex items-center gap-1">
        <Timer className="h-3 w-3" />
        {formatHoursPlayed(game.hours_played)}
      </span>
      {game.personal_rating ? (
        <span className="flex items-center gap-1">
          <span className="text-yellow-400">&#9733;</span>
          <span className="font-medium text-foreground">{game.personal_rating}</span>
        </span>
      ) : null}
      <span>IGDB {formatIgdbRating(game.game?.rating_average)}</span>
      {hasTtb && (
        <span>
          TTB {formatTtb(game.game?.howlongtobeat_main)} /{' '}
          {formatTtb(game.game?.howlongtobeat_extra)} /{' '}
          {formatTtb(game.game?.howlongtobeat_completionist)}
        </span>
      )}
    </div>
  );
}

function GameCard({
  game,
  isSelected,
  onSelectGame,
  onClickGame,
}: {
  game: UserGame;
  isSelected?: boolean;
  onSelectGame?: (id: string) => void;
  onClickGame?: (game: UserGame) => void;
}) {
  return (
    <div
      className={cn(
        'flex gap-3 rounded-lg border p-3 cursor-pointer transition-colors hover:bg-muted/50',
        isSelected && 'bg-muted',
      )}
      onClick={() => onClickGame?.(game)}
    >
      {onSelectGame && (
        <div className="flex items-start pt-0.5" onClick={(e) => e.stopPropagation()}>
          <Checkbox checked={isSelected} onCheckedChange={() => onSelectGame(game.id)} />
        </div>
      )}
      <GameCover game={game} className="h-16 w-12 shrink-0" />
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex items-center gap-2">
          <span className="font-medium truncate">{game.game?.title ?? 'Unknown Game'}</span>
          {game.is_loved && <span className="text-red-500 text-sm">&#9829;</span>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge className={cn('text-white border-0', statusColors[game.play_status])}>
            {statusLabels[game.play_status]}
          </Badge>
          <PlatformIconList platforms={game.platforms ?? []} size="sm" showLabels />
        </div>
        <GameStatRow game={game} />
      </div>
    </div>
  );
}

function GameCardList({ games, selectedIds, onSelectGame, onClickGame }: GameListProps) {
  return (
    <div className="space-y-3">
      {games.map((game) => (
        <GameCard
          key={game.id}
          game={game}
          isSelected={selectedIds?.has(game.id)}
          onSelectGame={onSelectGame}
          onClickGame={onClickGame}
        />
      ))}
    </div>
  );
}

function GameTable({ games, isLoading, selectedIds, onSelectGame, onClickGame }: GameListProps) {
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
          <TableSkeleton withSelection={!!onSelectGame} />
        ) : (
          games.map((game) => {
            const isSelected = selectedIds?.has(game.id);

            return (
              <TableRow
                key={game.id}
                className={cn('cursor-pointer', isSelected && 'bg-muted')}
                onClick={() => onClickGame?.(game)}
              >
                {onSelectGame && (
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <Checkbox checked={isSelected} onCheckedChange={() => onSelectGame(game.id)} />
                  </TableCell>
                )}
                <TableCell>
                  <GameCover game={game} className="h-12 w-9" />
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate max-w-xs">
                      {game.game?.title ?? 'Unknown Game'}
                    </span>
                    {game.is_loved && <span className="text-red-500 text-sm">&#9829;</span>}
                  </div>
                </TableCell>
                <TableCell>
                  <Badge className={cn('text-white border-0', statusColors[game.play_status])}>
                    {statusLabels[game.play_status]}
                  </Badge>
                </TableCell>
                <TableCell>
                  <PlatformIconList platforms={game.platforms ?? []} size="sm" showLabels />
                </TableCell>
                <TableCell>
                  <span className="text-sm">{formatHoursPlayed(game.hours_played)}</span>
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
                      <span className="text-sm font-medium">{game.personal_rating}</span>
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

export function GameList({
  games,
  isLoading,
  selectedIds,
  onSelectGame,
  onClickGame,
}: GameListProps) {
  const isCompact = useMediaQuery(COMPACT_QUERY);

  if (!isLoading && games.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>No games found</p>
        <p className="text-sm">Try adjusting your filters or add some games to your library.</p>
      </div>
    );
  }

  if (isCompact) {
    if (isLoading) {
      return <CardSkeleton />;
    }
    return (
      <GameCardList
        games={games}
        selectedIds={selectedIds}
        onSelectGame={onSelectGame}
        onClickGame={onClickGame}
      />
    );
  }

  return (
    <GameTable
      games={games}
      isLoading={isLoading}
      selectedIds={selectedIds}
      onSelectGame={onSelectGame}
      onClickGame={onClickGame}
    />
  );
}
