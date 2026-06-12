import { describe, it, expect } from 'vitest';
import { cardHasFacets, sanitizeFilter, isValidFilter } from './pool-filter';
import type { FilterCard, PoolFilter } from '@/types';

describe('pool-filter', () => {
  it('cardHasFacets is false for an empty card', () => {
    expect(cardHasFacets({})).toBe(false);
  });

  it('cardHasFacets is false for a card with only empty arrays / blank q', () => {
    expect(cardHasFacets({ genre: [], q: '' })).toBe(false);
  });

  it('cardHasFacets is true when any facet is set', () => {
    expect(cardHasFacets({ genre: ['RPG'] })).toBe(true);
    expect(cardHasFacets({ play_status: 'backlog' })).toBe(true);
    expect(cardHasFacets({ is_loved: true })).toBe(true);
    expect(cardHasFacets({ rating_min: 7 })).toBe(true);
    expect(cardHasFacets({ q: 'witcher' })).toBe(true);
  });

  it('sanitizeFilter strips empty arrays and blank scalars, dropping empty cards', () => {
    const dirty: PoolFilter = {
      filters: [{ genre: ['RPG'], theme: [], q: '' }, {}, { platform: ['windows'] }],
    };
    expect(sanitizeFilter(dirty)).toEqual({
      filters: [{ genre: ['RPG'] }, { platform: ['windows'] }],
    });
  });

  it('isValidFilter is false when no card has facets', () => {
    expect(isValidFilter({ filters: [] })).toBe(false);
    expect(isValidFilter({ filters: [{}, { genre: [] }] })).toBe(false);
  });

  it('isValidFilter is true when at least one card has facets', () => {
    const f: PoolFilter = { filters: [{ genre: ['RPG'] }] };
    expect(isValidFilter(f)).toBe(true);
  });

  it('a card round-trips through sanitize unchanged when already clean', () => {
    const clean: FilterCard = { genre: ['RPG'], platform: ['windows'], is_loved: true };
    expect(sanitizeFilter({ filters: [clean] })).toEqual({ filters: [clean] });
  });
});
