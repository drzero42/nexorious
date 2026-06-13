import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { PlatformIconList } from '@/components/ui/platform-icon';
import type { UserGame } from '@/types';
import type { ReactNode } from 'react';
import { Timer, Gamepad2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { formatTtb, formatIgdbRating, formatHoursPlayed, getCoverUrl } from '@/lib/game-utils';
import { statusColors, statusLabels } from '@/lib/play-status';
import { isBuyFirst } from '@/lib/game-flags';

export interface GameCardProps {
  game: UserGame;
  selected?: boolean;
  onSelect?: (id: string) => void;
  onClick?: () => void;
  /** Optional overlay rendered top-right (e.g. a drag handle or per-card menu). */
  topRightSlot?: ReactNode;
  /** Optional action row rendered below the card body (e.g. "+ add" / "promote"). */
  actionsSlot?: ReactNode;
}

export function GameCard({
  game,
  selected,
  onSelect,
  onClick,
  topRightSlot,
  actionsSlot,
}: GameCardProps) {
  const coverUrl = getCoverUrl(game);
  const buyFirst = isBuyFirst(game);

  return (
    <Card
      className={cn(
        'overflow-hidden cursor-pointer transition-all hover:shadow-lg group',
        selected && 'ring-2 ring-primary',
      )}
      onClick={onClick}
    >
      {/* Cover image */}
      <div className="aspect-[3/4] relative bg-muted">
        {coverUrl ? (
          <img
            src={coverUrl}
            alt={game.game?.title ?? 'Game cover'}
            style={{ width: '100%', height: '100%', objectFit: 'cover' }}
            className="object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-muted-foreground">
            <div className="text-center">
              <svg
                className="mx-auto h-12 w-12 text-muted-foreground/50"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
                />
              </svg>
              <p className="mt-2 text-sm">No Cover</p>
            </div>
          </div>
        )}

        {/* Selection checkbox */}
        {onSelect && (
          <div className="absolute top-2 left-2 z-10" onClick={(e) => e.stopPropagation()}>
            <Checkbox
              checked={selected}
              onCheckedChange={() => onSelect(game.id)}
              className="bg-background/80 backdrop-blur-sm"
            />
          </div>
        )}

        {/* Bottom-left badge: play status, or "Buy first" for wishlisted-unowned
            entries (which carry the badge instead of a play affordance). */}
        {buyFirst ? (
          <div className="absolute bottom-2 left-2">
            <Badge variant="secondary" className="text-xs">
              Buy first
            </Badge>
          </div>
        ) : (
          !game.is_wishlisted && (
            <div className="absolute bottom-2 left-2">
              <Badge className={cn('text-white border-0', statusColors[game.play_status])}>
                {statusLabels[game.play_status]}
              </Badge>
            </div>
          )
        )}

        {/* Top-right row: loved heart and an optional context slot (e.g. drag
            handle) sit side by side so they never overlap. */}
        {(game.is_loved || topRightSlot) && (
          <div className="absolute top-2 right-2 z-10 flex items-center gap-1">
            {game.is_loved && (
              <span className="inline-flex items-center justify-center w-6 h-6 rounded-full bg-red-100 text-red-600 text-sm">
                &#9829;
              </span>
            )}
            {topRightSlot && <div onClick={(e) => e.stopPropagation()}>{topRightSlot}</div>}
          </div>
        )}
      </div>

      <CardContent className="p-3">
        <h3 className="font-medium truncate" title={game.game?.title ?? 'Unknown Game'}>
          {game.game?.title ?? 'Unknown Game'}
        </h3>
        {game.platforms && game.platforms.length > 0 && (
          <div className="mt-1">
            <PlatformIconList platforms={game.platforms} size="sm" showTooltips />
          </div>
        )}
        <div className="flex items-center justify-between mt-2">
          {game.personal_rating ? (
            <div className="flex items-center space-x-1">
              <span className="text-yellow-400">&#9733;</span>
              <span className="text-sm font-medium">{game.personal_rating}</span>
            </div>
          ) : (
            <div className="flex items-center space-x-1 text-muted-foreground">
              <span>&#9734;</span>
            </div>
          )}
          {game.game?.rating_average != null && (
            <div className="flex items-center gap-1 text-muted-foreground">
              <Gamepad2 className="h-3 w-3" />
              <span className="text-sm font-medium text-foreground">
                {formatIgdbRating(game.game.rating_average)}
              </span>
            </div>
          )}
          <span className="text-sm text-muted-foreground">
            {formatHoursPlayed(game.hours_played)}
          </span>
        </div>
        {(game.game?.howlongtobeat_main != null ||
          game.game?.howlongtobeat_extra != null ||
          game.game?.howlongtobeat_completionist != null) && (
          <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
            <Timer className="h-3 w-3" />
            <span>
              {formatTtb(game.game?.howlongtobeat_main)} /{' '}
              {formatTtb(game.game?.howlongtobeat_extra)} /{' '}
              {formatTtb(game.game?.howlongtobeat_completionist)}
            </span>
          </div>
        )}

        {/* Actions slot (e.g. promote / remove buttons) */}
        {actionsSlot && (
          <div className="mt-2" onClick={(e) => e.stopPropagation()}>
            {actionsSlot}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
