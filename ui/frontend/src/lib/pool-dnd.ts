/**
 * Pure helpers for cross-zone drag-and-drop on the pool page. The three zones —
 * Up Next (queue), Candidates, and Suggestions — are the three membership
 * states, so a drag between them is a membership transition. This module maps a
 * drag's source/target zones onto the transition it means; the page component
 * executes it with the pool mutations.
 */

export type PoolZone = 'queue' | 'candidates' | 'suggestions';

/** Stable droppable container id per zone (distinct from card ids). */
export const ZONE_DROPPABLE_ID: Record<PoolZone, string> = {
  queue: 'zone:queue',
  candidates: 'zone:candidates',
  suggestions: 'zone:suggestions',
};

const ZONE_BY_DROPPABLE_ID: Record<string, PoolZone> = {
  'zone:queue': 'queue',
  'zone:candidates': 'candidates',
  'zone:suggestions': 'suggestions',
};

/**
 * Resolve a drag's `over` id to a zone. The target can be a zone's droppable
 * container id (dropped on empty space) or a card id (dropped on another card,
 * resolved via the card→zone map). Returns null if it resolves to neither.
 */
export function resolveZone(overId: string, cardZone: Record<string, PoolZone>): PoolZone | null {
  return ZONE_BY_DROPPABLE_ID[overId] ?? cardZone[overId] ?? null;
}

export type TransitionKind =
  | 'reorder' // within the queue
  | 'promote' // candidate → queue
  | 'add-candidate' // suggestion → candidates
  | 'add-and-queue' // suggestion → queue
  | 'demote' // queue → candidates
  | 'remove' // queue/candidate → suggestions (leaves the pool)
  | 'noop';

/** Map a drag from one zone to another onto its membership transition. */
export function planTransition(source: PoolZone, target: PoolZone): TransitionKind {
  if (source === target) return source === 'queue' ? 'reorder' : 'noop';
  switch (`${source}->${target}`) {
    case 'candidates->queue':
      return 'promote';
    case 'suggestions->candidates':
      return 'add-candidate';
    case 'suggestions->queue':
      return 'add-and-queue';
    case 'queue->candidates':
      return 'demote';
    case 'queue->suggestions':
    case 'candidates->suggestions':
      return 'remove';
    default:
      return 'noop';
  }
}
