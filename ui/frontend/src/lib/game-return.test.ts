import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setGameReturn, getGameReturn, navigateToGameReturn } from './game-return';

describe('game-return referrer helper', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('round-trips a full referrer (to, params, search, label)', () => {
    setGameReturn({
      to: '/pools/$id',
      params: { id: 'pool-1' },
      search: { q: 'rpg' },
      label: 'Pool',
    });

    expect(getGameReturn()).toEqual({
      to: '/pools/$id',
      params: { id: 'pool-1' },
      search: { q: 'rpg' },
      label: 'Pool',
    });
  });

  it('defaults to { to: /games, label: Games } when nothing is stored', () => {
    expect(getGameReturn()).toEqual({ to: '/games', label: 'Games' });
  });

  it('falls back to the default when the stored value is unparseable', () => {
    sessionStorage.setItem('game_return', 'not json{');
    expect(getGameReturn()).toEqual({ to: '/games', label: 'Games' });
  });

  it('falls back to the default when the stored value lacks required fields', () => {
    sessionStorage.setItem('game_return', JSON.stringify({ search: { q: 'x' } }));
    expect(getGameReturn()).toEqual({ to: '/games', label: 'Games' });
  });

  it('navigateToGameReturn replays only the present keys', () => {
    const navigate = vi.fn();
    setGameReturn({ to: '/library-health', label: 'Library Health' });

    navigateToGameReturn(navigate);

    expect(navigate).toHaveBeenCalledWith({ to: '/library-health' });
  });

  it('navigateToGameReturn includes params and search when stored', () => {
    const navigate = vi.fn();
    setGameReturn({ to: '/pools/$id', params: { id: 'p1' }, search: { q: 'z' }, label: 'Pool' });

    navigateToGameReturn(navigate);

    expect(navigate).toHaveBeenCalledWith({
      to: '/pools/$id',
      params: { id: 'p1' },
      search: { q: 'z' },
    });
  });

  it('navigateToGameReturn falls back to bare /games when nothing is stored', () => {
    const navigate = vi.fn();
    navigateToGameReturn(navigate);
    expect(navigate).toHaveBeenCalledWith({ to: '/games' });
  });
});
