import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { CurrentlyPlayingSection } from './CurrentlyPlayingSection';
import * as useGamesHook from '@/hooks/use-games';
import { PlayStatus, OwnershipStatus } from '@/types';
import type { UserGame, UserGameId, GameId } from '@/types';

// Mock the useActiveGames hook
vi.mock('@/hooks/use-games', async () => {
  const actual = await vi.importActual('@/hooks/use-games');
  return {
    ...actual,
    useActiveGames: vi.fn(),
  };
});

// Mock Next.js Image component - passes through the src prop as-is
vi.mock('next/image', () => ({
  default: ({ src, alt }: any) => {
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={src} alt={alt} />;
  },
}));

// Mock Next.js Link component
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: any) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));

const createMockGame = (overrides: Partial<UserGame> = {}): UserGame => ({
  id: 'game-1' as UserGameId,
  game: {
    id: 1 as GameId,
    title: 'Test Game',
    description: 'Test description',
    cover_art_url: '/storage/covers/test-game.jpg',
    rating_average: 8.5,
    rating_count: 100,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  ownership_status: OwnershipStatus.OWNED,
  play_status: PlayStatus.IN_PROGRESS,
  hours_played: 10,
  is_loved: false,
  platforms: [
    {
      id: 'ugp-1',
      platform: 'pc',
      platform_details: {
        name: 'pc',
        display_name: 'PC',
        icon_url: '',
        is_active: true,
        source: 'official',
        default_storefront: 'steam',
        storefronts: [],
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
      is_available: true,
      created_at: '2024-01-01T00:00:00Z',
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides,
});

describe('CurrentlyPlayingSection', () => {
  const mockUseActiveGames = useGamesHook.useActiveGames as ReturnType<
    typeof vi.fn
  >;

  describe('Rendering conditions', () => {
    it('does not render when no active games exist', () => {
      mockUseActiveGames.mockReturnValue({
        data: { items: [], total: 0, page: 1, perPage: 50, pages: 0 },
        isLoading: false,
        isError: false,
        error: null,
      });

      const { container } = render(<CurrentlyPlayingSection />);
      expect(container.firstChild).toBeNull();
    });

    it('does not render while loading', () => {
      mockUseActiveGames.mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
      });

      const { container } = render(<CurrentlyPlayingSection />);
      expect(container.firstChild).toBeNull();
    });

    it('renders section header when active games exist', () => {
      const mockGame = createMockGame();
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      expect(screen.getByText('Currently Playing')).toBeInTheDocument();
    });
  });

  describe('Game cards', () => {
    it('renders game cards with correct titles', () => {
      const game1 = createMockGame({
        id: 'game-1' as UserGameId,
        game: { ...createMockGame().game, id: 1 as GameId, title: 'Game One' },
      });
      const game2 = createMockGame({
        id: 'game-2' as UserGameId,
        game: { ...createMockGame().game, id: 2 as GameId, title: 'Game Two' },
      });

      mockUseActiveGames.mockReturnValue({
        data: { items: [game1, game2], total: 2, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      expect(screen.getByText('Game One')).toBeInTheDocument();
      expect(screen.getByText('Game Two')).toBeInTheDocument();
    });

    it('displays platform badge for each game', () => {
      const mockGame = createMockGame();
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      expect(screen.getByText('PC')).toBeInTheDocument();
    });

    it('shows "Unknown Platform" when game has no platforms', () => {
      const mockGame = createMockGame({ platforms: [] });
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      expect(screen.getByText('Unknown Platform')).toBeInTheDocument();
    });

    it('shows "+X more" when game has multiple platforms', () => {
      const mockGame = createMockGame({
        platforms: [
          {
            id: 'ugp-1',
            platform: 'pc',
            platform_details: {
              name: 'pc',
              display_name: 'PC',
              icon_url: '',
              is_active: true,
              source: 'official',
              default_storefront: 'steam',
              storefronts: [],
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            created_at: '2024-01-01T00:00:00Z',
          },
          {
            id: 'ugp-2',
            platform: 'ps5',
            platform_details: {
              name: 'ps5',
              display_name: 'PlayStation 5',
              icon_url: '',
              is_active: true,
              source: 'official',
              default_storefront: 'playstation',
              storefronts: [],
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            created_at: '2024-01-01T00:00:00Z',
          },
          {
            id: 'ugp-3',
            platform: 'switch',
            platform_details: {
              name: 'switch',
              display_name: 'Nintendo Switch',
              icon_url: '',
              is_active: true,
              source: 'official',
              default_storefront: 'nintendo',
              storefronts: [],
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z',
            },
            is_available: true,
            created_at: '2024-01-01T00:00:00Z',
          },
        ],
      });

      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('+2 more')).toBeInTheDocument();
    });

    it('renders links to game detail pages', () => {
      const mockGame = createMockGame({ id: 'test-game-id-123' as UserGameId });
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      const link = screen.getByRole('link');
      expect(link).toHaveAttribute('href', '/games/test-game-id-123');
    });

    it('renders cover art images with correct URLs', () => {
      const mockGame = createMockGame({
        game: {
          ...createMockGame().game,
          cover_art_url: '/storage/covers/my-game.jpg',
        },
      });
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      const image = screen.getByAltText('Test Game');
      // The component correctly constructs the full URL, but the mock Image component
      // just passes through the src. In production, Next.js Image handles the URL properly.
      expect(image).toHaveAttribute('src');
      expect(image.getAttribute('src')).toContain('/storage/covers/my-game.jpg');
    });

    it('handles missing cover art gracefully', () => {
      const mockGame = createMockGame({
        game: { ...createMockGame().game, cover_art_url: undefined },
      });
      mockUseActiveGames.mockReturnValue({
        data: { items: [mockGame], total: 1, page: 1, perPage: 50, pages: 1 },
        isLoading: false,
        isError: false,
        error: null,
      });

      render(<CurrentlyPlayingSection />);
      // Check for the fallback SVG's "No Cover" text
      expect(screen.getByText('No Cover')).toBeInTheDocument();
    });
  });
});
