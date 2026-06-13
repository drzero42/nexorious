import { describe, it, expect } from 'vitest';
import { resolveZone, planTransition, ZONE_DROPPABLE_ID, type PoolZone } from './pool-dnd';

describe('resolveZone', () => {
  const cardZone: Record<string, PoolZone> = {
    'ug-q': 'queue',
    'ug-c': 'candidates',
    'ug-s': 'suggestions',
  };

  it('resolves a zone droppable id to its zone', () => {
    expect(resolveZone(ZONE_DROPPABLE_ID.queue, cardZone)).toBe('queue');
    expect(resolveZone(ZONE_DROPPABLE_ID.candidates, cardZone)).toBe('candidates');
    expect(resolveZone(ZONE_DROPPABLE_ID.suggestions, cardZone)).toBe('suggestions');
  });

  it('resolves a card id to the zone that card belongs to', () => {
    expect(resolveZone('ug-q', cardZone)).toBe('queue');
    expect(resolveZone('ug-s', cardZone)).toBe('suggestions');
  });

  it('returns null for an unknown id', () => {
    expect(resolveZone('nope', cardZone)).toBeNull();
  });
});

describe('planTransition', () => {
  it('maps every cross-zone drag to the right membership transition', () => {
    expect(planTransition('candidates', 'queue')).toBe('promote');
    expect(planTransition('suggestions', 'candidates')).toBe('add-candidate');
    expect(planTransition('suggestions', 'queue')).toBe('add-and-queue');
    expect(planTransition('queue', 'candidates')).toBe('demote');
    expect(planTransition('queue', 'suggestions')).toBe('remove');
    expect(planTransition('candidates', 'suggestions')).toBe('remove');
  });

  it('reorders within the queue and no-ops within the other zones', () => {
    expect(planTransition('queue', 'queue')).toBe('reorder');
    expect(planTransition('candidates', 'candidates')).toBe('noop');
    expect(planTransition('suggestions', 'suggestions')).toBe('noop');
  });
});
