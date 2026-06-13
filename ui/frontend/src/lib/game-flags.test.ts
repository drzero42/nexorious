import { describe, it, expect } from 'vitest';
import { isBuyFirst } from './game-flags';
import type { UserGame } from '@/types';

function ug(partial: Partial<UserGame>): UserGame {
  return {
    id: 'ug1' as UserGame['id'],
    game: { id: 1 as never, title: 'X', rating_count: 0, created_at: '', updated_at: '' } as never,
    is_loved: false,
    play_status: 'backlog' as never,
    is_wishlisted: false,
    hours_played: 0,
    platforms: [],
    created_at: '',
    updated_at: '',
    ...partial,
  };
}

describe('isBuyFirst', () => {
  it('is true for a wishlisted game with no platforms', () => {
    expect(isBuyFirst(ug({ is_wishlisted: true, platforms: [] }))).toBe(true);
  });

  it('is false when not wishlisted', () => {
    expect(isBuyFirst(ug({ is_wishlisted: false, platforms: [] }))).toBe(false);
  });

  it('is false when wishlisted but already has a platform (acquired)', () => {
    expect(isBuyFirst(ug({ is_wishlisted: true, platforms: [{} as never] }))).toBe(false);
  });
});
