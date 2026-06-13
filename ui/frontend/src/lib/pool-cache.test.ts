import { describe, it, expect } from 'vitest';
import {
  applyQueueOrder,
  removeMember,
  addCandidate,
  addToQueue,
  removeSuggestion,
} from './pool-cache';
import type { PoolDetail, UserGame } from '@/types';
import type { UserGamesListResponse } from '@/api/games';

const g = (id: string): UserGame => ({ id }) as unknown as UserGame;

function detail(queue: string[], candidates: string[]): PoolDetail {
  return {
    id: 'p1',
    queue: queue.map(g),
    candidates: candidates.map(g),
  } as unknown as PoolDetail;
}

const ids = (list: UserGame[]) => list.map((x) => x.id);

describe('applyQueueOrder', () => {
  it('reorders within the queue', () => {
    const d = applyQueueOrder(detail(['a', 'b', 'c'], []), ['c', 'a', 'b']);
    expect(ids(d.queue)).toEqual(['c', 'a', 'b']);
    expect(ids(d.candidates)).toEqual([]);
  });

  it('promotes a candidate into the queue', () => {
    const d = applyQueueOrder(detail(['a'], ['b', 'c']), ['a', 'b']);
    expect(ids(d.queue)).toEqual(['a', 'b']);
    expect(ids(d.candidates)).toEqual(['c']);
  });

  it('demotes a queued game to candidates when dropped from the id list', () => {
    const d = applyQueueOrder(detail(['a', 'b'], ['c']), ['a']);
    expect(ids(d.queue)).toEqual(['a']);
    expect(ids(d.candidates)).toContain('b');
    expect(ids(d.candidates)).toContain('c');
  });
});

describe('removeMember', () => {
  it('removes from the queue', () => {
    const d = removeMember(detail(['a', 'b'], ['c']), 'a');
    expect(ids(d.queue)).toEqual(['b']);
    expect(ids(d.candidates)).toEqual(['c']);
  });
  it('removes from candidates', () => {
    const d = removeMember(detail(['a'], ['b', 'c']), 'c');
    expect(ids(d.candidates)).toEqual(['b']);
  });
});

describe('addCandidate', () => {
  it('prepends a new candidate', () => {
    const d = addCandidate(detail(['a'], ['b']), g('c'));
    expect(ids(d.candidates)).toEqual(['c', 'b']);
  });
  it('is a no-op when the game is already a member', () => {
    const d = addCandidate(detail(['a'], ['b']), g('a'));
    expect(ids(d.candidates)).toEqual(['b']);
  });
});

describe('addToQueue', () => {
  it('appends to the queue and removes it from candidates', () => {
    const d = addToQueue(detail(['a'], ['b']), g('b'));
    expect(ids(d.queue)).toEqual(['a', 'b']);
    expect(ids(d.candidates)).toEqual([]);
  });
  it('is a no-op when already queued', () => {
    const d = addToQueue(detail(['a', 'b'], []), g('a'));
    expect(ids(d.queue)).toEqual(['a', 'b']);
  });
});

describe('removeSuggestion', () => {
  it('drops the game from the suggestions page', () => {
    const list = {
      items: [g('a'), g('b')],
      total: 2,
      page: 1,
      perPage: 24,
      pages: 1,
    } as UserGamesListResponse;
    expect(removeSuggestion(list, 'a').items.map((x) => x.id)).toEqual(['b']);
  });
});
