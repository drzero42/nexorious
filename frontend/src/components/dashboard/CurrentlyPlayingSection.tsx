'use client';

import Link from 'next/link';
import Image from 'next/image';
import { Badge } from '@/components/ui/badge';
import { config } from '@/lib/env';
import { useActiveGames } from '@/hooks/use-games';
import type { UserGame } from '@/types';

function getCoverUrl(game: UserGame): string | null {
  if (game.game?.cover_art_url) {
    // If it's a relative path, prepend static URL
    if (game.game.cover_art_url.startsWith('/')) {
      return `${config.staticUrl}${game.game.cover_art_url}`;
    }
    return game.game.cover_art_url;
  }
  return null;
}

function getPlatformDisplay(game: UserGame): {
  firstPlatform: string;
  additionalCount: number;
} {
  if (!game.platforms || game.platforms.length === 0) {
    return { firstPlatform: 'Unknown Platform', additionalCount: 0 };
  }

  const firstPlatform =
    game.platforms[0].platform_details?.display_name ??
    game.platforms[0].platform ??
    'Unknown Platform';
  const additionalCount = game.platforms.length - 1;

  return { firstPlatform, additionalCount };
}

export function CurrentlyPlayingSection() {
  const { data, isLoading } = useActiveGames();

  // Hide section if loading or no games
  if (isLoading || !data || data.items.length === 0) {
    return null;
  }

  return (
    <section className="space-y-4">
      <h2 className="text-2xl font-semibold">Currently Playing</h2>

      {/* Horizontal scroll container */}
      <div className="flex gap-4 overflow-x-auto scroll-smooth pb-4 -mx-4 px-4 scrollbar-hide">
        {data.items.map((game) => {
          const coverUrl = getCoverUrl(game);
          const { firstPlatform, additionalCount } = getPlatformDisplay(game);

          return (
            <Link
              key={game.id}
              href={`/games/${game.id}`}
              className="flex-shrink-0 w-[140px] sm:w-40 group shadow-md hover:shadow-lg transition-shadow rounded-lg"
            >
              {/* Cover art with 3:4 aspect ratio */}
              <div className="aspect-[3/4] relative bg-muted rounded-lg overflow-hidden mb-2">
                {coverUrl ? (
                  <Image
                    src={coverUrl}
                    alt={game.game?.title ?? 'Game cover'}
                    fill
                    unoptimized
                    className="object-cover group-hover:scale-105 transition-transform duration-300"
                    sizes="160px"
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
              </div>

              {/* Game title */}
              <h3
                className="text-sm font-medium line-clamp-2 mb-1"
                title={game.game?.title ?? 'Unknown Game'}
              >
                {game.game?.title ?? 'Unknown Game'}
              </h3>

              {/* Platform badge */}
              <div className="flex items-center gap-1">
                <Badge variant="secondary" className="text-xs">
                  {firstPlatform}
                </Badge>
                {additionalCount > 0 && (
                  <span className="text-xs text-muted-foreground">
                    +{additionalCount} more
                  </span>
                )}
              </div>
            </Link>
          );
        })}
      </div>
    </section>
  );
}
