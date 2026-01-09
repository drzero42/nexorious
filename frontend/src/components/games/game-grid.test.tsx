import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameGrid } from './game-grid';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock the GameCard component to simplify testing
vi.mock('./game-card', () => ({
  GameCard: ({
    game,
    selected,
    onSelect,
    onClick,
  }: {
    game: UserGame;
    selected?: boolean;
    onSelect?: (id: string) => void;
    onClick?: () => void;
  }) => (
    <div data-testid={`game-card-${game.id}`}>
      <span>{game.game?.title || 'Unknown Game'}</span>
      {onSelect && (
        <input
          type="checkbox"
          checked={selected}
          onChange={() => onSelect(game.id)}
          data-testid={`checkbox-${game.id}`}
        />
      )}
      <button onClick={onClick} data-testid={`click-${game.id}`}>
        Click Game
      </button>
    </div>
  ),
}));

const createMockGame = (overrides: Partial<UserGame> = {}): UserGame => ({
  id: 'f47ac10b-58cc-4372-a567-0e02b2c3d479' as UserGameId,
  game: {
    id: 123 as GameId,
    title: 'Test Game',
    description: 'A test game description',
    genre: 'RPG',
    developer: 'Test Developer',
    publisher: 'Test Publisher',
    release_date: '2024-01-01',
    cover_art_url: '/covers/test.jpg',
    rating_average: 4.5,
    rating_count: 100,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  personal_rating: 4,
  is_loved: false,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  personal_notes: '<p>Some notes</p>',
  platforms: [
    {
      id: 'ugp-1',
      platform: 'pc',
      storefront: 'steam',
      platform_details: {
        name: 'pc',
        display_name: 'PC',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      storefront_details: {
        name: 'steam',
        display_name: 'Steam',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      is_available: true,
      hours_played: 10,
      ownership_status: OwnershipStatus.OWNED,
      acquired_date: '2024-01-15',
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  tags: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('GameGrid', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('renders 12 skeleton placeholders when isLoading is true', () => {
      render(<GameGrid games={[]} isLoading={true} />);

      // Check for skeleton elements - they have animate-pulse class
      const skeletons = screen.getAllByRole('generic').filter((el) =>
        el.className.includes('animate-pulse')
      );

      // There should be 36 skeleton divs (12 skeletons × 3 skeleton parts each)
      // But we specifically check for the main skeleton wrappers
      expect(skeletons.length).toBeGreaterThanOrEqual(12);
    });

    it('does not render games when isLoading is true', () => {
      const games = [createMockGame()];
      render(<GameGrid games={games} isLoading={true} />);

      expect(screen.queryByText('Test Game')).not.toBeInTheDocument();
    });

    it('does not render empty state when isLoading is true', () => {
      render(<GameGrid games={[]} isLoading={true} />);

      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });
  });

  describe('empty state', () => {
    it('renders empty state message when games array is empty', () => {
      render(<GameGrid games={[]} />);

      expect(screen.getByText('No games found')).toBeInTheDocument();
      expect(
        screen.getByText(/Try adjusting your filters or add some games to your library/i)
      ).toBeInTheDocument();
    });

    it('does not render game cards when games array is empty', () => {
      render(<GameGrid games={[]} />);

      expect(screen.queryByTestId(/^game-card-/)).not.toBeInTheDocument();
    });

    it('does not render empty state when isLoading is true', () => {
      render(<GameGrid games={[]} isLoading={true} />);

      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });
  });

  describe('rendering games', () => {
    it('renders game cards when games are provided', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({
          id: 'game-2' as UserGameId,
          game: { ...createMockGame().game, title: 'Another Game' },
        }),
      ];
      render(<GameGrid games={games} />);

      expect(screen.getByText('Test Game')).toBeInTheDocument();
      expect(screen.getByText('Another Game')).toBeInTheDocument();
    });

    it('renders correct number of game cards', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({ id: 'game-2' as UserGameId }),
        createMockGame({ id: 'game-3' as UserGameId }),
      ];
      render(<GameGrid games={games} />);

      expect(screen.getByTestId('game-card-game-1')).toBeInTheDocument();
      expect(screen.getByTestId('game-card-game-2')).toBeInTheDocument();
      expect(screen.getByTestId('game-card-game-3')).toBeInTheDocument();
    });

    it('does not render skeletons when games are provided', () => {
      const games = [createMockGame()];
      render(<GameGrid games={games} />);

      const skeletons = screen.queryAllByRole('generic').filter((el) =>
        el.className.includes('animate-pulse')
      );

      expect(skeletons.length).toBe(0);
    });

    it('does not render empty state when games are provided', () => {
      const games = [createMockGame()];
      render(<GameGrid games={games} />);

      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });
  });

  describe('selection state', () => {
    it('passes selected state to GameCard', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({ id: 'game-2' as UserGameId }),
      ];
      const selectedIds = new Set(['game-1']);
      const onSelectGame = vi.fn();

      render(
        <GameGrid
          games={games}
          selectedIds={selectedIds}
          onSelectGame={onSelectGame}
        />
      );

      // Game 1 should have a checked checkbox
      const checkbox1 = screen.getByTestId('checkbox-game-1') as HTMLInputElement;
      expect(checkbox1.checked).toBe(true);

      // Game 2 should have an unchecked checkbox
      const checkbox2 = screen.getByTestId('checkbox-game-2') as HTMLInputElement;
      expect(checkbox2.checked).toBe(false);
    });

    it('passes onSelectGame callback to GameCard', async () => {
      const user = userEvent.setup();
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const onSelectGame = vi.fn();

      render(<GameGrid games={games} onSelectGame={onSelectGame} />);

      await user.click(screen.getByTestId('checkbox-game-1'));

      expect(onSelectGame).toHaveBeenCalledWith('game-1');
      expect(onSelectGame).toHaveBeenCalledTimes(1);
    });

    it('does not render checkboxes when onSelectGame is not provided', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];

      render(<GameGrid games={games} />);

      expect(screen.queryByTestId('checkbox-game-1')).not.toBeInTheDocument();
    });

    it('updates selection state for multiple games independently', async () => {
      const user = userEvent.setup();
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({ id: 'game-2' as UserGameId }),
      ];
      const onSelectGame = vi.fn();

      render(<GameGrid games={games} onSelectGame={onSelectGame} />);

      await user.click(screen.getByTestId('checkbox-game-1'));
      await user.click(screen.getByTestId('checkbox-game-2'));

      expect(onSelectGame).toHaveBeenCalledWith('game-1');
      expect(onSelectGame).toHaveBeenCalledWith('game-2');
      expect(onSelectGame).toHaveBeenCalledTimes(2);
    });
  });

  describe('click handling', () => {
    it('calls onClickGame with game object when card is clicked', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });
      const onClickGame = vi.fn();

      render(<GameGrid games={[game]} onClickGame={onClickGame} />);

      await user.click(screen.getByTestId('click-game-1'));

      expect(onClickGame).toHaveBeenCalledWith(game);
      expect(onClickGame).toHaveBeenCalledTimes(1);
    });

    it('calls onClickGame for correct game when multiple games are present', async () => {
      const user = userEvent.setup();
      const game1 = createMockGame({ id: 'game-1' as UserGameId });
      const game2 = createMockGame({ id: 'game-2' as UserGameId });
      const onClickGame = vi.fn();

      render(<GameGrid games={[game1, game2]} onClickGame={onClickGame} />);

      await user.click(screen.getByTestId('click-game-2'));

      expect(onClickGame).toHaveBeenCalledWith(game2);
      expect(onClickGame).not.toHaveBeenCalledWith(game1);
    });

    it('does not throw when clicked without onClickGame handler', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });

      render(<GameGrid games={[game]} />);

      // Should not throw
      await expect(
        user.click(screen.getByTestId('click-game-1'))
      ).resolves.not.toThrow();
    });
  });

  describe('edge cases', () => {
    it('handles empty games array with undefined selectedIds', () => {
      expect(() => render(<GameGrid games={[]} />)).not.toThrow();
    });

    it('handles games with selectedIds but no onSelectGame', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const selectedIds = new Set(['game-1']);

      // Should not crash, but checkboxes won't render without onSelectGame
      expect(() =>
        render(<GameGrid games={games} selectedIds={selectedIds} />)
      ).not.toThrow();
    });

    it('renders large number of games without issues', () => {
      const games = Array.from({ length: 100 }, (_, i) =>
        createMockGame({
          id: `game-${i}` as UserGameId,
          game: { ...createMockGame().game, title: `Game ${i}` },
        })
      );

      render(<GameGrid games={games} />);

      expect(screen.getByText('Game 0')).toBeInTheDocument();
      expect(screen.getByText('Game 99')).toBeInTheDocument();
    });

    it('handles games with null or undefined game object', () => {
      const game = createMockGame({
        id: 'game-1' as UserGameId,
        game: null as unknown as UserGame['game'],
      });

      expect(() => render(<GameGrid games={[game]} />)).not.toThrow();
      expect(screen.getByText('Unknown Game')).toBeInTheDocument();
    });
  });

  describe('integration scenarios', () => {
    it('handles combined selection and click callbacks', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });
      const onSelectGame = vi.fn();
      const onClickGame = vi.fn();

      render(
        <GameGrid
          games={[game]}
          onSelectGame={onSelectGame}
          onClickGame={onClickGame}
        />
      );

      // Click checkbox
      await user.click(screen.getByTestId('checkbox-game-1'));
      expect(onSelectGame).toHaveBeenCalledWith('game-1');

      // Click game
      await user.click(screen.getByTestId('click-game-1'));
      expect(onClickGame).toHaveBeenCalledWith(game);
    });

    it('transitions from loading to showing games', () => {
      const games = [createMockGame()];
      const { rerender } = render(<GameGrid games={[]} isLoading={true} />);

      // Should show skeletons initially
      const skeletons = screen.getAllByRole('generic').filter((el) =>
        el.className.includes('animate-pulse')
      );
      expect(skeletons.length).toBeGreaterThanOrEqual(12);

      // Re-render with games loaded
      rerender(<GameGrid games={games} isLoading={false} />);

      expect(screen.getByText('Test Game')).toBeInTheDocument();
      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });

    it('transitions from loading to empty state', () => {
      const { rerender } = render(<GameGrid games={[]} isLoading={true} />);

      // Should show skeletons initially
      expect(screen.queryByText('No games found')).not.toBeInTheDocument();

      // Re-render with loading complete but no games
      rerender(<GameGrid games={[]} isLoading={false} />);

      expect(screen.getByText('No games found')).toBeInTheDocument();
    });

    it('maintains selected state when games list updates', () => {
      const game1 = createMockGame({ id: 'game-1' as UserGameId });
      const game2 = createMockGame({ id: 'game-2' as UserGameId });
      const selectedIds = new Set(['game-1']);
      const onSelectGame = vi.fn();

      const { rerender } = render(
        <GameGrid
          games={[game1]}
          selectedIds={selectedIds}
          onSelectGame={onSelectGame}
        />
      );

      // Game 1 should be selected
      const checkbox1 = screen.getByTestId('checkbox-game-1') as HTMLInputElement;
      expect(checkbox1.checked).toBe(true);

      // Add second game, keep same selection
      rerender(
        <GameGrid
          games={[game1, game2]}
          selectedIds={selectedIds}
          onSelectGame={onSelectGame}
        />
      );

      // Game 1 should still be selected
      const checkbox1After = screen.getByTestId(
        'checkbox-game-1'
      ) as HTMLInputElement;
      expect(checkbox1After.checked).toBe(true);

      // Game 2 should not be selected
      const checkbox2 = screen.getByTestId('checkbox-game-2') as HTMLInputElement;
      expect(checkbox2.checked).toBe(false);
    });
  });
});
