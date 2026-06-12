import type { UserGame } from '@/types';

/**
 * A wishlisted, not-yet-owned game shows a "buy first" badge in pool zones
 * instead of a play affordance. Acquiring it (a platform appears) flips this to
 * false in place — consistent with ClearWishlistOnAcquire keeping the queue slot.
 */
export function isBuyFirst(game: UserGame): boolean {
  return game.is_wishlisted && (game.platforms?.length ?? 0) === 0;
}
