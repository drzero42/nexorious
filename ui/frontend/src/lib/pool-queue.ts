/**
 * Pure helpers that map Up Next queue operations to the ordered `ids` list that
 * PUT /api/pools/:id/queue expects. The backend is declarative: the returned
 * list IS the new queue (position = index); any member not listed demotes to
 * Candidate. "Remove from pool" is NOT here — it calls removePoolGame, not
 * setQueue.
 */

/** Move the item at `from` to `to`, returning a new array. */
export function reorderQueue(ids: string[], from: number, to: number): string[] {
  if (from === to) return ids;
  const next = [...ids];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

/** Append a candidate to the end of the queue (idempotent). */
export function promoteToQueue(queueIds: string[], userGameId: string): string[] {
  if (queueIds.includes(userGameId)) return queueIds;
  return [...queueIds, userGameId];
}

/** Drop an id from the queue (it becomes a Candidate on the next setQueue). */
export function demoteFromQueue(queueIds: string[], userGameId: string): string[] {
  return queueIds.filter((id) => id !== userGameId);
}

/** Move an id to the front (on deck). */
export function setOnDeck(queueIds: string[], userGameId: string): string[] {
  if (queueIds[0] === userGameId) return queueIds;
  return [userGameId, ...queueIds.filter((id) => id !== userGameId)];
}
