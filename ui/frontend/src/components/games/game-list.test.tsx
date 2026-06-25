import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameList } from './game-list';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock the env config
vi.mock('@/lib/env', () => ({
  config: {
    staticUrl: 'http://localhost:8000',
  },
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
  is_wishlisted: false,
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

describe('GameList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('loading state', () => {
    it('renders 10 skeleton rows when isLoading is true', () => {
      render(<GameList games={[]} isLoading={true} />);

      // Check for skeleton elements - they have animate-pulse class
      const skeletons = screen
        .getAllByRole('generic')
        .filter((el) => el.className.includes('animate-pulse'));

      // There should be 9 skeleton cells per row × 10 rows = 90 skeleton divs
      expect(skeletons.length).toBeGreaterThanOrEqual(10);
    });

    it('does not render games when isLoading is true', () => {
      const games = [createMockGame()];
      render(<GameList games={games} isLoading={true} />);

      expect(screen.queryByText('Test Game')).not.toBeInTheDocument();
    });

    it('does not render empty state when isLoading is true', () => {
      render(<GameList games={[]} isLoading={true} />);

      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });
  });

  describe('empty state', () => {
    it('renders empty state message when games array is empty', () => {
      render(<GameList games={[]} />);

      expect(screen.getByText('No games found')).toBeInTheDocument();
      expect(
        screen.getByText(/Try adjusting your filters or add some games to your library/i),
      ).toBeInTheDocument();
    });

    it('does not render table when games array is empty', () => {
      render(<GameList games={[]} />);

      expect(screen.queryByText('Cover')).not.toBeInTheDocument();
      expect(screen.queryByText('Title')).not.toBeInTheDocument();
    });

    it('does not render empty state when isLoading is true', () => {
      render(<GameList games={[]} isLoading={true} />);

      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });
  });

  describe('table headers', () => {
    it('renders all column headers without selection column', () => {
      const games = [createMockGame()];
      render(<GameList games={games} />);

      expect(screen.getByText('Cover')).toBeInTheDocument();
      expect(screen.getByText('Title')).toBeInTheDocument();
      expect(screen.getByText('Status')).toBeInTheDocument();
      expect(screen.getByText('Platform(s)')).toBeInTheDocument();
      expect(screen.getByText('Hours')).toBeInTheDocument();
      expect(screen.getByText('Time to Beat')).toBeInTheDocument();
      expect(screen.getByText('Rating')).toBeInTheDocument();
      expect(screen.getByText('IGDB')).toBeInTheDocument();
    });

    it('renders selection column header when onSelectGame is provided', () => {
      const games = [createMockGame()];
      const onSelectGame = vi.fn();
      render(<GameList games={games} onSelectGame={onSelectGame} />);

      // Should have 9 headers: checkbox + Cover + Title + Status + Platform(s) + Hours + Time to Beat + Rating + IGDB
      const headers = screen.getAllByRole('columnheader');
      expect(headers.length).toBe(9);
    });

    it('does not render selection column header when onSelectGame is not provided', () => {
      const games = [createMockGame()];
      render(<GameList games={games} />);

      // Should have 8 headers: Cover + Title + Status + Platform(s) + Hours + Time to Beat + Rating + IGDB
      const headers = screen.getAllByRole('columnheader');
      expect(headers.length).toBe(8);
    });
  });

  describe('rendering games', () => {
    it('renders game rows when games are provided', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({
          id: 'game-2' as UserGameId,
          game: { ...createMockGame().game, title: 'Another Game' },
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('Test Game')).toBeInTheDocument();
      expect(screen.getByText('Another Game')).toBeInTheDocument();
    });

    it('renders correct number of game rows', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({ id: 'game-2' as UserGameId }),
        createMockGame({ id: 'game-3' as UserGameId }),
      ];
      render(<GameList games={games} />);

      // Should have 3 data rows (excluding header row)
      const rows = screen.getAllByRole('row');
      // 1 header row + 3 data rows = 4 total
      expect(rows.length).toBe(4);
    });

    it('renders "Unknown Game" when game title is not available', () => {
      const games = [
        createMockGame({
          game: undefined as unknown as UserGame['game'],
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('Unknown Game')).toBeInTheDocument();
    });
  });

  describe('cover image display', () => {
    it('renders cover image when cover_art_url starts with /', () => {
      const games = [createMockGame()];
      render(<GameList games={games} />);

      const img = screen.getByRole('img', { name: 'Test Game' });
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute('src', 'http://localhost:8000/covers/test.jpg');
    });

    it('renders absolute cover URL without prepending staticUrl', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            cover_art_url: 'https://example.com/cover.jpg',
          },
        }),
      ];
      render(<GameList games={games} />);

      const img = screen.getByRole('img', { name: 'Test Game' });
      expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg');
    });

    it('renders "N/A" placeholder when cover_art_url is not provided', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            cover_art_url: undefined,
          },
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('N/A')).toBeInTheDocument();
    });

    it('renders "N/A" placeholder when game object is missing', () => {
      const games = [
        createMockGame({
          game: undefined as unknown as UserGame['game'],
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('N/A')).toBeInTheDocument();
    });
  });

  describe('status badges', () => {
    const statusTestCases: Array<{
      status: PlayStatus;
      label: string;
      color: string;
    }> = [
      { status: PlayStatus.NOT_STARTED, label: 'Not Started', color: 'bg-gray-500' },
      { status: PlayStatus.IN_PROGRESS, label: 'In Progress', color: 'bg-blue-500' },
      { status: PlayStatus.COMPLETED, label: 'Completed', color: 'bg-green-500' },
      { status: PlayStatus.MASTERED, label: 'Mastered', color: 'bg-purple-500' },
      { status: PlayStatus.DOMINATED, label: 'Dominated', color: 'bg-yellow-500' },
      { status: PlayStatus.SHELVED, label: 'Shelved', color: 'bg-orange-500' },
      { status: PlayStatus.DROPPED, label: 'Dropped', color: 'bg-red-500' },
      { status: PlayStatus.REPLAY, label: 'Replay', color: 'bg-cyan-500' },
    ];

    it.each(statusTestCases)(
      'renders "$label" badge for $status status with $color color',
      ({ status, label, color }) => {
        const games = [createMockGame({ play_status: status })];
        const { container } = render(<GameList games={games} />);

        expect(screen.getByText(label)).toBeInTheDocument();

        // Check for the color class
        const badge = container.querySelector(`.${color}`);
        expect(badge).toBeInTheDocument();
      },
    );
  });

  describe('loved indicator', () => {
    it('renders loved indicator (heart) when is_loved is true', () => {
      const games = [createMockGame({ is_loved: true })];
      render(<GameList games={games} />);

      // Heart symbol &#9829; renders as ♥
      expect(screen.getByText('♥')).toBeInTheDocument();
    });

    it('does not render loved indicator when is_loved is false', () => {
      const games = [createMockGame({ is_loved: false })];
      render(<GameList games={games} />);

      expect(screen.queryByText('♥')).not.toBeInTheDocument();
    });

    it('renders multiple loved indicators for multiple loved games', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId, is_loved: true }),
        createMockGame({ id: 'game-2' as UserGameId, is_loved: true }),
      ];
      render(<GameList games={games} />);

      const hearts = screen.getAllByText('♥');
      expect(hearts.length).toBe(2);
    });
  });

  describe('platform display', () => {
    it('renders single platform with icon and label', () => {
      const games = [
        createMockGame({
          platforms: [
            {
              id: 'ugp-1',
              platform: 'pc',
              platform_details: {
                name: 'pc',
                display_name: 'PC',
                icon_url: '/logos/platforms/pc/pc-icon-light.svg',
                is_active: true,
                source: 'system',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
              is_available: true,
              hours_played: 10,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
          ],
        }),
      ];
      render(<GameList games={games} />);

      // List view shows the icon (decorative — alt="") with its text label.
      expect(
        document.querySelector(
          'img[src="http://localhost:8000/logos/platforms/pc/pc-icon-light.svg"]',
        ),
      ).toBeInTheDocument();
      expect(screen.getByText('PC')).toBeInTheDocument();
    });

    it('renders multiple platforms with icons and labels', () => {
      const games = [
        createMockGame({
          platforms: [
            {
              id: 'ugp-1',
              platform: 'pc',
              platform_details: {
                name: 'pc',
                display_name: 'PC',
                icon_url: '/logos/platforms/pc/pc-icon-light.svg',
                is_active: true,
                source: 'system',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
              is_available: true,
              hours_played: 5,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
            {
              id: 'ugp-2',
              platform: 'ps5',
              platform_details: {
                name: 'ps5',
                display_name: 'PlayStation 5',
                icon_url: '/logos/platforms/ps5/ps5-icon-light.svg',
                is_active: true,
                source: 'system',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
              is_available: true,
              hours_played: 5,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
          ],
        }),
      ];
      render(<GameList games={games} />);

      // List view shows the icons (decorative — alt="") with their text labels.
      expect(
        document.querySelector(
          'img[src="http://localhost:8000/logos/platforms/pc/pc-icon-light.svg"]',
        ),
      ).toBeInTheDocument();
      expect(
        document.querySelector(
          'img[src="http://localhost:8000/logos/platforms/ps5/ps5-icon-light.svg"]',
        ),
      ).toBeInTheDocument();
      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
    });

    it('renders fallback initial when icon_url is not available', () => {
      const games = [
        createMockGame({
          platforms: [
            {
              id: 'ugp-1',
              platform: 'xbox',
              platform_details: {
                name: 'xbox',
                display_name: 'Xbox',
                is_active: true,
                source: 'system',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
              is_available: true,
              hours_played: 10,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
          ],
        }),
      ];
      render(<GameList games={games} />);

      // Fallback shows first letter and display name
      expect(screen.getByText('X')).toBeInTheDocument();
      expect(screen.getByText('Xbox')).toBeInTheDocument();
    });

    it('renders "-" when no platforms', () => {
      const games = [createMockGame({ platforms: [] })];
      render(<GameList games={games} />);

      // Find the cell that should contain the dash
      const cells = screen.getAllByRole('cell');
      const platformCell = cells.find((cell) => cell.textContent === '-');
      expect(platformCell).toBeInTheDocument();
    });

    it('filters out platforms with no platform_details', () => {
      const games = [
        createMockGame({
          platforms: [
            {
              id: 'ugp-1',
              platform: 'pc',
              platform_details: {
                name: 'pc',
                display_name: 'PC',
                icon_url: '/logos/platforms/pc/pc-icon-light.svg',
                is_active: true,
                source: 'system',
                created_at: '2024-01-01T00:00:00Z',
                updated_at: '2024-01-01T00:00:00Z',
              },
              is_available: true,
              hours_played: 10,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
            {
              id: 'ugp-2',
              platform: undefined,
              platform_details: undefined,
              is_available: true,
              hours_played: 0,
              ownership_status: OwnershipStatus.OWNED,
              created_at: '2024-01-01T00:00:00Z',
            },
          ],
        }),
      ];
      render(<GameList games={games} />);

      // Should only show PC icon (decorative — alt="") and label, not crash or show empty string
      expect(
        document.querySelector(
          'img[src="http://localhost:8000/logos/platforms/pc/pc-icon-light.svg"]',
        ),
      ).toBeInTheDocument();
      expect(screen.getByText('PC')).toBeInTheDocument();
    });
  });

  describe('hours played display', () => {
    it('renders hours played', () => {
      const games = [createMockGame({ hours_played: 25 })];
      render(<GameList games={games} />);

      expect(screen.getByText('25h')).toBeInTheDocument();
    });

    it.each<{ name: string; value: number }>([
      { name: '0', value: 0 },
      { name: 'undefined', value: undefined as unknown as number },
      { name: 'null', value: null as unknown as number },
    ])('renders "0h" when hours_played is $name', ({ value }) => {
      const games = [createMockGame({ hours_played: value })];
      render(<GameList games={games} />);

      expect(screen.getByText('0h')).toBeInTheDocument();
    });
  });

  describe('time to beat display', () => {
    it('renders TTB values when all three are present', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            howlongtobeat_main: 10,
            howlongtobeat_extra: 20,
            howlongtobeat_completionist: 30,
          },
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('10h / 20h / 30h')).toBeInTheDocument();
    });

    it('renders em-dash for null individual TTB values', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            howlongtobeat_main: 15,
            howlongtobeat_extra: null as unknown as number,
            howlongtobeat_completionist: null as unknown as number,
          },
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('15h / — / —')).toBeInTheDocument();
    });

    it('renders em-dash cell when all TTB values are null', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            howlongtobeat_main: undefined,
            howlongtobeat_extra: undefined,
            howlongtobeat_completionist: undefined,
          },
        }),
      ];
      render(<GameList games={games} />);

      // Should show em-dash in the TTB cell
      const cells = screen.getAllByRole('cell');
      const ttbCell = cells.find((cell) => cell.textContent === '—');
      expect(ttbCell).toBeInTheDocument();
    });
  });

  describe('rating display', () => {
    it('renders personal rating with star when provided', () => {
      const games = [createMockGame({ personal_rating: 4 })];
      render(<GameList games={games} />);

      expect(screen.getByText('4')).toBeInTheDocument();
      // Star symbol &#9733; renders as ★
      expect(screen.getByText('★')).toBeInTheDocument();
    });

    it('renders "-" when personal_rating is null', () => {
      const games = [createMockGame({ personal_rating: null })];
      render(<GameList games={games} />);

      // Find the cell that contains the dash
      const cells = screen.getAllByRole('cell');
      const ratingCell = cells.find((cell) => cell.textContent === '-');
      expect(ratingCell).toBeInTheDocument();
    });

    it('renders "-" when personal_rating is undefined', () => {
      const games = [createMockGame({ personal_rating: undefined })];
      render(<GameList games={games} />);

      // Find the cell that contains the dash
      const cells = screen.getAllByRole('cell');
      const ratingCell = cells.find((cell) => cell.textContent === '-');
      expect(ratingCell).toBeInTheDocument();
    });

    it('renders different ratings for different games', () => {
      const games = [
        createMockGame({
          id: 'game-1' as UserGameId,
          game: { ...createMockGame().game, title: 'Game 1' },
          personal_rating: 3,
        }),
        createMockGame({
          id: 'game-2' as UserGameId,
          game: { ...createMockGame().game, title: 'Game 2' },
          personal_rating: 5,
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('3')).toBeInTheDocument();
      expect(screen.getByText('5')).toBeInTheDocument();
    });
  });

  describe('selection checkboxes', () => {
    it('renders checkbox for each game when onSelectGame is provided', () => {
      const games = [
        createMockGame({ id: 'game-1' as UserGameId }),
        createMockGame({ id: 'game-2' as UserGameId }),
      ];
      const onSelectGame = vi.fn();
      render(<GameList games={games} onSelectGame={onSelectGame} />);

      const checkboxes = screen.getAllByRole('checkbox');
      expect(checkboxes.length).toBe(2);
    });

    it('does not render checkboxes when onSelectGame is not provided', () => {
      const games = [createMockGame()];
      render(<GameList games={games} />);

      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    });

    it('checkbox reflects selected state', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const selectedIds = new Set(['game-1']);
      const onSelectGame = vi.fn();
      render(<GameList games={games} selectedIds={selectedIds} onSelectGame={onSelectGame} />);

      expect(screen.getByRole('checkbox')).toBeChecked();
    });

    it('checkbox reflects unselected state', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const selectedIds = new Set<string>();
      const onSelectGame = vi.fn();
      render(<GameList games={games} selectedIds={selectedIds} onSelectGame={onSelectGame} />);

      expect(screen.getByRole('checkbox')).not.toBeChecked();
    });

    it('calls onSelectGame with game id when checkbox is clicked', async () => {
      const user = userEvent.setup();
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const onSelectGame = vi.fn();
      render(<GameList games={games} onSelectGame={onSelectGame} />);

      await user.click(screen.getByRole('checkbox'));

      expect(onSelectGame).toHaveBeenCalledWith('game-1');
      expect(onSelectGame).toHaveBeenCalledTimes(1);
    });

    it('checkbox click does not trigger onClickGame', async () => {
      const user = userEvent.setup();
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const onSelectGame = vi.fn();
      const onClickGame = vi.fn();
      render(<GameList games={games} onSelectGame={onSelectGame} onClickGame={onClickGame} />);

      await user.click(screen.getByRole('checkbox'));

      expect(onSelectGame).toHaveBeenCalled();
      expect(onClickGame).not.toHaveBeenCalled();
    });

    it('applies bg-muted style to selected row', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const selectedIds = new Set(['game-1']);
      const onSelectGame = vi.fn();
      const { container } = render(
        <GameList games={games} selectedIds={selectedIds} onSelectGame={onSelectGame} />,
      );

      // Find the data row (not header row) with bg-muted
      const row = container.querySelector('tbody tr.bg-muted');
      expect(row).toBeInTheDocument();
    });
  });

  describe('row click handling', () => {
    it('calls onClickGame with game object when row is clicked', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });
      const onClickGame = vi.fn();
      render(<GameList games={[game]} onClickGame={onClickGame} />);

      await user.click(screen.getByText('Test Game'));

      expect(onClickGame).toHaveBeenCalledWith(game);
      expect(onClickGame).toHaveBeenCalledTimes(1);
    });

    it('calls onClickGame for correct game when multiple games are present', async () => {
      const user = userEvent.setup();
      const game1 = createMockGame({
        id: 'game-1' as UserGameId,
        game: { ...createMockGame().game, title: 'Game 1' },
      });
      const game2 = createMockGame({
        id: 'game-2' as UserGameId,
        game: { ...createMockGame().game, title: 'Game 2' },
      });
      const onClickGame = vi.fn();
      render(<GameList games={[game1, game2]} onClickGame={onClickGame} />);

      await user.click(screen.getByText('Game 2'));

      expect(onClickGame).toHaveBeenCalledWith(game2);
      expect(onClickGame).not.toHaveBeenCalledWith(game1);
    });

    it('row has cursor-pointer class', () => {
      const games = [createMockGame({ id: 'game-1' as UserGameId })];
      const { container } = render(<GameList games={games} />);

      const row = container.querySelector('tbody tr.cursor-pointer');
      expect(row).toBeInTheDocument();
    });
  });

  describe('edge cases', () => {
    it('handles games with null game object', () => {
      const game = createMockGame({
        id: 'game-1' as UserGameId,
        game: null as unknown as UserGame['game'],
      });

      expect(() => render(<GameList games={[game]} />)).not.toThrow();
      expect(screen.getByText('Unknown Game')).toBeInTheDocument();
    });

    it('handles undefined game title gracefully', () => {
      const game = createMockGame({
        game: {
          ...createMockGame().game,
          title: undefined as unknown as string,
        },
      });

      render(<GameList games={[game]} />);
      expect(screen.getByText('Unknown Game')).toBeInTheDocument();
    });
  });

  describe('IGDB rating display', () => {
    it('renders formatted IGDB rating when rating_average is provided', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            rating_average: 85.0,
          },
        }),
      ];
      render(<GameList games={games} />);

      expect(screen.getByText('8.5')).toBeInTheDocument();
    });

    it('renders em dash when rating_average is null', () => {
      const games = [
        createMockGame({
          game: {
            ...createMockGame().game,
            rating_average: null as unknown as number,
          },
        }),
      ];
      render(<GameList games={games} />);

      // Find a cell that contains the em dash (—)
      const cells = screen.getAllByRole('cell');
      const igdbCell = cells.find((cell) => cell.textContent === '—');
      expect(igdbCell).toBeInTheDocument();
    });
  });

  describe('integration scenarios', () => {
    it('handles combined selection and click callbacks', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });
      const onSelectGame = vi.fn();
      const onClickGame = vi.fn();

      render(<GameList games={[game]} onSelectGame={onSelectGame} onClickGame={onClickGame} />);

      // Click checkbox
      await user.click(screen.getByRole('checkbox'));
      expect(onSelectGame).toHaveBeenCalledWith('game-1');

      // Click game row
      await user.click(screen.getByText('Test Game'));
      expect(onClickGame).toHaveBeenCalledWith(game);
    });

    it('transitions from loading to showing games', () => {
      const games = [createMockGame()];
      const { rerender } = render(<GameList games={[]} isLoading={true} />);

      // Should show skeletons initially
      const skeletons = screen
        .getAllByRole('generic')
        .filter((el) => el.className.includes('animate-pulse'));
      expect(skeletons.length).toBeGreaterThanOrEqual(10);

      // Re-render with games loaded
      rerender(<GameList games={games} isLoading={false} />);

      expect(screen.getByText('Test Game')).toBeInTheDocument();
      expect(screen.queryByText('No games found')).not.toBeInTheDocument();
    });

    it('transitions from loading to empty state', () => {
      const { rerender } = render(<GameList games={[]} isLoading={true} />);

      // Should show skeletons initially
      expect(screen.queryByText('No games found')).not.toBeInTheDocument();

      // Re-render with loading complete but no games
      rerender(<GameList games={[]} isLoading={false} />);

      expect(screen.getByText('No games found')).toBeInTheDocument();
    });

    it('maintains selected state when games list updates', () => {
      const game1 = createMockGame({ id: 'game-1' as UserGameId });
      const game2 = createMockGame({
        id: 'game-2' as UserGameId,
        game: { ...createMockGame().game, title: 'Game 2' },
      });
      const selectedIds = new Set(['game-1']);
      const onSelectGame = vi.fn();

      const { rerender } = render(
        <GameList games={[game1]} selectedIds={selectedIds} onSelectGame={onSelectGame} />,
      );

      // Game 1 should be selected
      expect(screen.getByRole('checkbox')).toBeChecked();

      // Add second game, keep same selection
      rerender(
        <GameList games={[game1, game2]} selectedIds={selectedIds} onSelectGame={onSelectGame} />,
      );

      // Get both checkboxes
      const checkboxes = screen.getAllByRole('checkbox');
      expect(checkboxes.length).toBe(2);

      // Game 1 should still be selected
      expect(checkboxes[0]).toBeChecked();

      // Game 2 should not be selected
      expect(checkboxes[1]).not.toBeChecked();
    });
  });

  describe('mobile card layout', () => {
    const originalMatchMedia = window.matchMedia;

    beforeEach(() => {
      // Force the compact media query to match so the card layout renders.
      window.matchMedia = vi.fn().mockImplementation((query: string) => ({
        matches: true,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      }));
    });

    afterEach(() => {
      window.matchMedia = originalMatchMedia;
    });

    it('renders cards instead of a table on narrow viewports', () => {
      render(<GameList games={[createMockGame()]} />);

      expect(screen.queryByRole('table')).not.toBeInTheDocument();
      expect(screen.queryByRole('columnheader')).not.toBeInTheDocument();
      expect(screen.getByText('Test Game')).toBeInTheDocument();
      expect(screen.getByText('In Progress')).toBeInTheDocument();
    });

    it('renders a checkbox per game and toggles selection', async () => {
      const user = userEvent.setup();
      const onSelectGame = vi.fn();
      render(
        <GameList
          games={[createMockGame({ id: 'game-1' as UserGameId })]}
          onSelectGame={onSelectGame}
        />,
      );

      await user.click(screen.getByRole('checkbox'));
      expect(onSelectGame).toHaveBeenCalledWith('game-1');
    });

    it('calls onClickGame when a card is clicked', async () => {
      const user = userEvent.setup();
      const game = createMockGame({ id: 'game-1' as UserGameId });
      const onClickGame = vi.fn();
      render(<GameList games={[game]} onClickGame={onClickGame} />);

      await user.click(screen.getByText('Test Game'));
      expect(onClickGame).toHaveBeenCalledWith(game);
    });

    it('does not trigger onClickGame when the card checkbox is clicked', async () => {
      const user = userEvent.setup();
      const onSelectGame = vi.fn();
      const onClickGame = vi.fn();
      render(
        <GameList
          games={[createMockGame({ id: 'game-1' as UserGameId })]}
          onSelectGame={onSelectGame}
          onClickGame={onClickGame}
        />,
      );

      await user.click(screen.getByRole('checkbox'));
      expect(onSelectGame).toHaveBeenCalled();
      expect(onClickGame).not.toHaveBeenCalled();
    });

    it('shows the N/A placeholder when cover art is missing', () => {
      render(
        <GameList
          games={[
            createMockGame({
              game: { ...createMockGame().game, cover_art_url: undefined },
            }),
          ]}
        />,
      );

      expect(screen.getByText('N/A')).toBeInTheDocument();
    });

    it('renders card skeletons (not a table) while loading', () => {
      render(<GameList games={[]} isLoading={true} />);

      expect(screen.queryByRole('table')).not.toBeInTheDocument();
      const skeletons = screen
        .getAllByRole('generic')
        .filter((el) => el.className.includes('animate-pulse'));
      expect(skeletons.length).toBeGreaterThanOrEqual(10);
    });
  });
});
