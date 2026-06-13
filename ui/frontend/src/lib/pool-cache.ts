/**
 * Pure transforms for optimistic React Query cache updates on the pool page.
 * Each mirrors a backend membership transition so a drag/button feels instant;
 * the mutation's invalidation later reconciles against the server.
 */
import type { PoolDetail, UserGame } from '@/types';
import type { UserGamesListResponse } from '@/api/games';

/**
 * Recompute queue/candidates so the queue is exactly `ids` (in order) and every
 * other member becomes a candidate — mirrors the declarative PUT …/queue.
 * Covers reorder, promote (id added to ids) and demote (id removed from ids).
 */
export function applyQueueOrder(detail: PoolDetail, ids: string[]): PoolDetail {
  const members = [...detail.queue, ...detail.candidates];
  const byId = new Map<string, UserGame>(members.map((g) => [g.id, g]));
  const queue = ids.map((id) => byId.get(id)).filter((g): g is UserGame => g != null);
  const queued = new Set(ids);
  const candidates = members.filter((g) => !queued.has(g.id));
  return { ...detail, queue, candidates };
}

/** Remove a member from the pool entirely (queue or candidates). */
export function removeMember(detail: PoolDetail, userGameId: string): PoolDetail {
  return {
    ...detail,
    queue: detail.queue.filter((g) => g.id !== userGameId),
    candidates: detail.candidates.filter((g) => g.id !== userGameId),
  };
}

/** Add a game as a candidate (no-op if it is already a member). */
export function addCandidate(detail: PoolDetail, game: UserGame): PoolDetail {
  if ([...detail.queue, ...detail.candidates].some((g) => g.id === game.id)) return detail;
  return { ...detail, candidates: [game, ...detail.candidates] };
}

/** Add a game to the end of the queue (and drop it from candidates if present). */
export function addToQueue(detail: PoolDetail, game: UserGame): PoolDetail {
  if (detail.queue.some((g) => g.id === game.id)) return detail;
  return {
    ...detail,
    queue: [...detail.queue, game],
    candidates: detail.candidates.filter((g) => g.id !== game.id),
  };
}

/** Drop a game from a suggestions page (it became a member / left the pool). */
export function removeSuggestion(
  list: UserGamesListResponse,
  userGameId: string,
): UserGamesListResponse {
  return { ...list, items: list.items.filter((g) => g.id !== userGameId) };
}
