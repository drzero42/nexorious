import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameCard } from './game-card';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, GameId, UserGameId } from '@/types';

// Mock next/image
vi.mock('next/image', () => ({
  default: ({ src, alt, ...props }: { src: string; alt: string; [key: string]: unknown }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={src} alt={alt} {...props} />
  ),
}));

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
  ownership_status: OwnershipStatus.OWNED,
  personal_rating: 4,
  is_loved: false,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  personal_notes: '<p>Some notes</p>',
  acquired_date: '2024-01-15',
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
      hours_played: 0,
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  tags: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('GameCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('rendering', () => {
    it('renders game title', () => {
      const game = createMockGame();
      render(<GameCard game={game} />);

      expect(screen.getByText('Test Game')).toBeInTheDocument();
    });

    it('renders "Unknown Game" when game title is not available', () => {
      const game = createMockGame({
        game: undefined as unknown as UserGame['game'],
      });
      render(<GameCard game={game} />);

      expect(screen.getByText('Unknown Game')).toBeInTheDocument();
    });

    it('renders cover image when cover_art_url is provided', () => {
      const game = createMockGame();
      render(<GameCard game={game} />);

      const img = screen.getByRole('img', { name: 'Test Game' });
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute('src', 'http://localhost:8000/covers/test.jpg');
    });

    it('renders absolute cover URL without prepending staticUrl', () => {
      const game = createMockGame({
        game: {
          ...createMockGame().game,
          cover_art_url: 'https://example.com/cover.jpg',
        },
      });
      render(<GameCard game={game} />);

      const img = screen.getByRole('img', { name: 'Test Game' });
      expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg');
    });

    it('renders "No Cover" placeholder when cover_art_url is not provided', () => {
      const game = createMockGame({
        game: {
          ...createMockGame().game,
          cover_art_url: undefined,
        },
      });
      render(<GameCard game={game} />);

      expect(screen.getByText('No Cover')).toBeInTheDocument();
    });

    it('renders platform names', () => {
      const game = createMockGame();
      render(<GameCard game={game} />);

      expect(screen.getByText('PC')).toBeInTheDocument();
    });

    it('renders multiple platform names separated by comma', () => {
      const game = createMockGame({
        platforms: [
          {
            id: 'ugp-1',
            platform: 'pc',
            platform_details: {
              name: 'pc',
              display_name: 'PC',
              is_active: true,
              source: 'system',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            hours_played: 0,
            created_at: '2024-01-01T00:00:00Z',
          },
          {
            id: 'ugp-2',
            platform: 'ps5',
            platform_details: {
              name: 'ps5',
              display_name: 'PlayStation 5',
              is_active: true,
              source: 'system',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            hours_played: 0,
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
      });
      render(<GameCard game={game} />);

      expect(screen.getByText('PC, PlayStation 5')).toBeInTheDocument();
    });

    it('does not render platform section when no platforms', () => {
      const game = createMockGame({ platforms: [] });
      render(<GameCard game={game} />);

      // Should not find any platform text
      expect(screen.queryByText('PC')).not.toBeInTheDocument();
    });

    it('renders hours played', () => {
      const game = createMockGame({ hours_played: 25 });
      render(<GameCard game={game} />);

      expect(screen.getByText('25h')).toBeInTheDocument();
    });

    it('renders 0h when hours_played is undefined', () => {
      const game = createMockGame({ hours_played: undefined as unknown as number });
      render(<GameCard game={game} />);

      expect(screen.getByText('0h')).toBeInTheDocument();
    });
  });

  describe('personal rating', () => {
    it('renders personal rating when provided', () => {
      const game = createMockGame({ personal_rating: 4 });
      render(<GameCard game={game} />);

      expect(screen.getByText('4')).toBeInTheDocument();
    });

    it('renders "Not rated" when personal_rating is null', () => {
      const game = createMockGame({ personal_rating: null });
      render(<GameCard game={game} />);

      expect(screen.getByText('Not rated')).toBeInTheDocument();
    });

    it('renders "Not rated" when personal_rating is undefined', () => {
      const game = createMockGame({ personal_rating: undefined });
      render(<GameCard game={game} />);

      expect(screen.getByText('Not rated')).toBeInTheDocument();
    });
  });

  describe('play status badge', () => {
    const statusTestCases: Array<{ status: PlayStatus; label: string }> = [
      { status: PlayStatus.NOT_STARTED, label: 'Not Started' },
      { status: PlayStatus.IN_PROGRESS, label: 'In Progress' },
      { status: PlayStatus.COMPLETED, label: 'Completed' },
      { status: PlayStatus.MASTERED, label: 'Mastered' },
      { status: PlayStatus.DOMINATED, label: 'Dominated' },
      { status: PlayStatus.SHELVED, label: 'Shelved' },
      { status: PlayStatus.DROPPED, label: 'Dropped' },
      { status: PlayStatus.REPLAY, label: 'Replay' },
    ];

    it.each(statusTestCases)(
      'renders "$label" badge for $status status',
      ({ status, label }) => {
        const game = createMockGame({ play_status: status });
        render(<GameCard game={game} />);

        expect(screen.getByText(label)).toBeInTheDocument();
      }
    );
  });

  describe('loved indicator', () => {
    it('renders loved indicator when is_loved is true', () => {
      const game = createMockGame({ is_loved: true });
      render(<GameCard game={game} />);

      // Heart symbol
      expect(screen.getByText('♥')).toBeInTheDocument();
    });

    it('does not render loved indicator when is_loved is false', () => {
      const game = createMockGame({ is_loved: false });
      render(<GameCard game={game} />);

      expect(screen.queryByText('♥')).not.toBeInTheDocument();
    });
  });

  describe('selection', () => {
    it('renders checkbox when onSelect is provided', () => {
      const game = createMockGame();
      const onSelect = vi.fn();
      render(<GameCard game={game} onSelect={onSelect} />);

      expect(screen.getByRole('checkbox')).toBeInTheDocument();
    });

    it('does not render checkbox when onSelect is not provided', () => {
      const game = createMockGame();
      render(<GameCard game={game} />);

      expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    });

    it('checkbox reflects selected state', () => {
      const game = createMockGame();
      const onSelect = vi.fn();
      render(<GameCard game={game} selected={true} onSelect={onSelect} />);

      expect(screen.getByRole('checkbox')).toBeChecked();
    });

    it('checkbox reflects unselected state', () => {
      const game = createMockGame();
      const onSelect = vi.fn();
      render(<GameCard game={game} selected={false} onSelect={onSelect} />);

      expect(screen.getByRole('checkbox')).not.toBeChecked();
    });

    it('calls onSelect with game id when checkbox is clicked', async () => {
      const user = userEvent.setup();
      const game = createMockGame();
      const onSelect = vi.fn();
      render(<GameCard game={game} onSelect={onSelect} />);

      await user.click(screen.getByRole('checkbox'));

      expect(onSelect).toHaveBeenCalledWith('f47ac10b-58cc-4372-a567-0e02b2c3d479');
    });

    it('checkbox click does not trigger onClick', async () => {
      const user = userEvent.setup();
      const game = createMockGame();
      const onSelect = vi.fn();
      const onClick = vi.fn();
      render(<GameCard game={game} onSelect={onSelect} onClick={onClick} />);

      await user.click(screen.getByRole('checkbox'));

      expect(onSelect).toHaveBeenCalled();
      expect(onClick).not.toHaveBeenCalled();
    });

    it('applies ring style when selected', () => {
      const game = createMockGame();
      const onSelect = vi.fn();
      const { container } = render(<GameCard game={game} selected={true} onSelect={onSelect} />);

      const card = container.querySelector('.ring-2');
      expect(card).toBeInTheDocument();
    });
  });

  describe('click handling', () => {
    it('calls onClick when card is clicked', async () => {
      const user = userEvent.setup();
      const game = createMockGame();
      const onClick = vi.fn();
      render(<GameCard game={game} onClick={onClick} />);

      await user.click(screen.getByText('Test Game'));

      expect(onClick).toHaveBeenCalled();
    });

    it('does not throw when clicked without onClick handler', async () => {
      const user = userEvent.setup();
      const game = createMockGame();
      render(<GameCard game={game} />);

      // Should not throw
      await expect(user.click(screen.getByText('Test Game'))).resolves.not.toThrow();
    });
  });

  describe('edge cases', () => {
    it('handles game with null game object gracefully', () => {
      const game = createMockGame({
        game: null as unknown as UserGame['game'],
      });

      // Should not throw
      expect(() => render(<GameCard game={game} />)).not.toThrow();
    });

    it('uses platform slug when display_name is not available', () => {
      const game = createMockGame({
        platforms: [
          {
            id: 'ugp-1',
            platform: 'xbox',
            platform_details: {
              name: 'xbox',
              display_name: undefined as unknown as string,
              is_active: true,
              source: 'system',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            hours_played: 0,
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
      });
      render(<GameCard game={game} />);

      expect(screen.getByText('xbox')).toBeInTheDocument();
    });

    it('filters out platforms with no platform_details', () => {
      const game = createMockGame({
        platforms: [
          {
            id: 'ugp-1',
            platform: 'pc',
            platform_details: {
              name: 'pc',
              display_name: 'PC',
              is_active: true,
              source: 'system',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            hours_played: 0,
            created_at: '2024-01-01T00:00:00Z',
          },
          {
            id: 'ugp-2',
            platform: undefined,
            platform_details: undefined,
            is_available: true,
            hours_played: 0,
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
      });
      render(<GameCard game={game} />);

      // Should only show PC, not crash or show empty string
      expect(screen.getByText('PC')).toBeInTheDocument();
    });
  });
});
