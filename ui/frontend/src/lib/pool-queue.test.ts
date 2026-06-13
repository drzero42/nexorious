import { describe, it, expect } from 'vitest';
import { reorderQueue, promoteToQueue, demoteFromQueue, setOnDeck } from './pool-queue';

describe('pool-queue mapping', () => {
  it('reorderQueue moves an id to a new index', () => {
    expect(reorderQueue(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a']);
    expect(reorderQueue(['a', 'b', 'c'], 2, 0)).toEqual(['c', 'a', 'b']);
  });

  it('reorderQueue is a no-op when from === to', () => {
    expect(reorderQueue(['a', 'b', 'c'], 1, 1)).toEqual(['a', 'b', 'c']);
  });

  it('promoteToQueue appends a candidate id to the end of the queue', () => {
    expect(promoteToQueue(['a', 'b'], 'c')).toEqual(['a', 'b', 'c']);
  });

  it('promoteToQueue is idempotent if the id is already queued', () => {
    expect(promoteToQueue(['a', 'b'], 'b')).toEqual(['a', 'b']);
  });

  it('demoteFromQueue drops an id from the queue list', () => {
    expect(demoteFromQueue(['a', 'b', 'c'], 'b')).toEqual(['a', 'c']);
  });

  it('setOnDeck moves an id to the front', () => {
    expect(setOnDeck(['a', 'b', 'c'], 'c')).toEqual(['c', 'a', 'b']);
    expect(setOnDeck(['a', 'b', 'c'], 'a')).toEqual(['a', 'b', 'c']);
  });
});
