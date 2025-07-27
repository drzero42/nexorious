import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../../test-utils/api-mocks';
import { resetStoresMocks } from '../../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import '../../../test-utils/navigation-mocks'; // Import to apply global mocks
import { mockGoto, mockPage } from '../../../test-utils/navigation-mocks';
import GameDetailPage from './+page.svelte';
import type { UserGame, PlayStatus, OwnershipStatus } from '$lib/stores/user-games.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));


// Create comprehensive UserGame mock with all metadata
const createMockUserGame = (overrides: Partial<UserGame> = {}): UserGame => ({
  id: 'game-1',
  game: {
    id: 'game-1',
    title: 'Test Game',
    description: 'A comprehensive test game with all metadata',
    genre: 'Action, RPG',
    developer: 'Test Developer Studio',
    publisher: 'Test Publisher Inc',
    release_date: '2024-01-01',
    cover_art_url: 'https://example.com/cover.jpg',
    rating_average: 8.5,
    rating_count: 2500,
    game_metadata: '{}',
    estimated_playtime_hours: 40,
    howlongtobeat_main: 25,
    howlongtobeat_extra: 35,
    howlongtobeat_completionist: 50,
    igdb_id: 'igdb-123',
    is_verified: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z'
  },
  ownership_status: 'owned' as OwnershipStatus,
  is_physical: false,
  personal_rating: 4,
  is_loved: true,
  play_status: 'completed' as PlayStatus,
  hours_played: 30,
  personal_notes: 'Amazing game with great story!',
  acquired_date: '2024-01-01',
  platforms: [
    {
      id: 'platform-1',
      platform: {
        id: 'pc',
        name: 'PC',
        display_name: 'PC',
        source: 'manual',
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'
      },
      storefront: {
        id: 'steam',
        name: 'Steam',
        display_name: 'Steam',
        icon_url: 'https://example.com/steam-icon.png',
        base_url: 'https://store.steampowered.com',
        source: 'manual',
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'
      },
      store_game_id: '12345',
      store_url: 'https://store.steampowered.com/app/12345/test-game/',
      is_available: true,
      created_at: '2024-01-01T00:00:00Z'
    },
    {
      id: 'platform-2',
      platform: {
        id: 'ps5',
        name: 'PlayStation 5',
        display_name: 'PlayStation 5',
        source: 'manual',
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'
      },
      storefront: {
        id: 'psn',
        name: 'PlayStation Store',
        display_name: 'PlayStation Store',
        icon_url: 'https://example.com/psn-icon.png',
        base_url: 'https://store.playstation.com',
        source: 'manual',
        is_active: true,
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'
      },
      store_url: 'https://store.playstation.com/product/test-game',
      is_available: true,
      created_at: '2024-01-01T00:00:00Z'
    }
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  ...overrides
});

// Mock user games store with comprehensive data
const mockUserGamesStore = {
  value: {
    userGames: [createMockUserGame()],
    isLoading: false,
    error: undefined
  },
  subscribe: vi.fn((callback) => {
    callback(mockUserGamesStore.value);
    return () => {};
  }),
  fetchUserGames: vi.fn(),
  updateUserGame: vi.fn(),
  updateProgress: vi.fn(),
  removeFromCollection: vi.fn()
};

// Mock the stores
vi.mock('$lib/stores/user-games.svelte', () => ({
  userGames: mockUserGamesStore
}));

// Also mock the main stores export
vi.mock('$lib/stores', () => ({
  userGames: mockUserGamesStore
}));

// Mock page store with specific game ID
vi.mock('$app/stores', () => ({
  page: {
    subscribe: (callback: any) => {
      callback({ params: { id: 'game-1' } });
      return () => {};
    }
  }
}));

// Mock navigation
vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  page: {
    subscribe: vi.fn((callback) => {
      callback({
        params: { id: 'game-1' },
        url: new URL('http://localhost:3000/games/game-1'),
        route: { id: '/games/[id]' },
        status: 200,
        error: null,
        data: {},
        form: null
      });
      return () => {};
    })
  }
}));

describe('Game Detail Page - Enhanced Metadata', () => {
  
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
    
    // Reset mock store
    mockUserGamesStore.value = {
      userGames: [createMockUserGame()],
      isLoading: false,
      error: undefined
    };
    
    // Mock fetchUserGames to resolve immediately
    mockUserGamesStore.fetchUserGames.mockResolvedValue();
  });

  describe('Platform Information Display', () => {
    it('should display platform information when platforms are available', async () => {
      render(GameDetailPage);
      
      // Wait for the component to finish loading
      await waitFor(() => {
        expect(screen.getByText('Available On')).toBeInTheDocument();
      });
      
      // Check for platform names
      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
      
      // Check for storefront information
      expect(screen.getByText('(Steam)')).toBeInTheDocument();
      expect(screen.getByText('(PlayStation Store)')).toBeInTheDocument();
    });

    it('should display clickable store links with proper accessibility', () => {
      render(GameDetailPage);
      
      const steamLink = screen.getByLabelText('View PC store page');
      expect(steamLink).toBeInTheDocument();
      expect(steamLink).toHaveAttribute('href', 'https://store.steampowered.com/app/12345/test-game/');
      expect(steamLink).toHaveAttribute('target', '_blank');
      expect(steamLink).toHaveAttribute('rel', 'noopener noreferrer');
      
      const psnLink = screen.getByLabelText('View PlayStation 5 store page');
      expect(psnLink).toBeInTheDocument();
      expect(psnLink).toHaveAttribute('href', 'https://store.playstation.com/product/test-game');
    });

    it('should not display platform section when no platforms available', () => {
      const gameWithoutPlatforms = createMockUserGame({ platforms: [] });
      mockUserGamesStore.value.userGames = [gameWithoutPlatforms];
      
      render(GameDetailPage);
      
      expect(screen.queryByText('Available On')).not.toBeInTheDocument();
    });
  });

  describe('IGDB Rating and Verification Display', () => {
    it('should display IGDB rating when available', () => {
      render(GameDetailPage);
      
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      expect(screen.getByText('8.5/10')).toBeInTheDocument();
      expect(screen.getByText('(2,500 reviews)')).toBeInTheDocument();
    });

    it('should display verification badge when game is verified', () => {
      render(GameDetailPage);
      
      const verifiedBadge = screen.getByText('Verified');
      expect(verifiedBadge).toBeInTheDocument();
      expect(verifiedBadge.closest('.bg-green-100')).toBeInTheDocument();
    });

    it('should not display rating section when no rating or verification', () => {
      const gameWithoutRating = createMockUserGame({
        game: {
          ...createMockUserGame().game,
          rating_count: 0,
          is_verified: false
        }
      });
      mockUserGamesStore.value.userGames = [gameWithoutRating];
      
      render(GameDetailPage);
      
      expect(screen.queryByText('Game Rating')).not.toBeInTheDocument();
    });
  });

  describe('How Long to Beat Display', () => {
    it('should display all How Long to Beat times when available', () => {
      render(GameDetailPage);
      
      expect(screen.getByText('How Long to Beat')).toBeInTheDocument();
      
      // Check for main story time
      expect(screen.getByText('Main Story')).toBeInTheDocument();
      expect(screen.getByText('25h')).toBeInTheDocument();
      
      // Check for main + extra time
      expect(screen.getByText('Main + Extra')).toBeInTheDocument();
      expect(screen.getByText('35h')).toBeInTheDocument();
      
      // Check for completionist time
      expect(screen.getByText('Completionist')).toBeInTheDocument();
      expect(screen.getByText('50h')).toBeInTheDocument();
    });

    it('should not display How Long to Beat section when no times available', () => {
      const gameWithoutTimes = createMockUserGame({
        game: {
          ...createMockUserGame().game
        }
      });
      mockUserGamesStore.value.userGames = [gameWithoutTimes];
      
      render(GameDetailPage);
      
      expect(screen.queryByText('How Long to Beat')).not.toBeInTheDocument();
    });
  });

  describe('Enhanced Game Details', () => {
    it('should display enhanced game details including estimated playtime and IGDB ID', () => {
      render(GameDetailPage);
      
      // Check for existing fields
      expect(screen.getByText('Developer')).toBeInTheDocument();
      expect(screen.getByText('Test Developer Studio')).toBeInTheDocument();
      expect(screen.getByText('Publisher')).toBeInTheDocument();
      expect(screen.getByText('Test Publisher Inc')).toBeInTheDocument();
      
      // Check for enhanced fields
      expect(screen.getByText('Estimated Playtime')).toBeInTheDocument();
      expect(screen.getByText('40 hours')).toBeInTheDocument();
      
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      const igdbLink = screen.getByText('igdb-123');
      expect(igdbLink).toBeInTheDocument();
      expect(igdbLink.closest('a')).toHaveAttribute('href', 'https://www.igdb.com/games/igdb-123');
    });

    it('should handle missing optional fields gracefully', () => {
      const gameWithMissingFields = createMockUserGame({
        game: {
          ...createMockUserGame().game
        }
      });
      mockUserGamesStore.value.userGames = [gameWithMissingFields];
      
      render(GameDetailPage);
      
      expect(screen.queryByText('Developer')).not.toBeInTheDocument();
      expect(screen.queryByText('Estimated Playtime')).not.toBeInTheDocument();
      expect(screen.queryByText('IGDB ID')).not.toBeInTheDocument();
      
      // Other fields should still be present
      expect(screen.getByText('Publisher')).toBeInTheDocument();
      expect(screen.getByText('Genre')).toBeInTheDocument();
    });
  });

  describe('Data Integration', () => {
    it('should display comprehensive metadata when all fields are present', () => {
      render(GameDetailPage);
      
      // Verify all metadata sections are displayed
      expect(screen.getByText('Available On')).toBeInTheDocument();
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      expect(screen.getByText('How Long to Beat')).toBeInTheDocument();
      expect(screen.getByText('Description')).toBeInTheDocument();
      
      // Verify specific content
      expect(screen.getByText('A comprehensive test game with all metadata')).toBeInTheDocument();
      expect(screen.getByText('8.5/10')).toBeInTheDocument();
      expect(screen.getByText('Verified')).toBeInTheDocument();
      expect(screen.getByText('25h')).toBeInTheDocument(); // Main story
      expect(screen.getByText('35h')).toBeInTheDocument(); // Main + Extra
      expect(screen.getByText('50h')).toBeInTheDocument(); // Completionist
    });

    it('should handle minimal metadata gracefully', () => {
      const minimalGame = createMockUserGame({
        game: {
          id: 'game-minimal',
          title: 'Minimal Game',
          rating_count: 0,
          game_metadata: '{}',
          is_verified: false,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z'
        },
        platforms: []
      });
      mockUserGamesStore.value.userGames = [minimalGame];
      
      render(GameDetailPage);
      
      // Title should still be displayed
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Minimal Game');
      
      // Optional sections should not be displayed
      expect(screen.queryByText('Available On')).not.toBeInTheDocument();
      expect(screen.queryByText('Game Rating')).not.toBeInTheDocument();
      expect(screen.queryByText('How Long to Beat')).not.toBeInTheDocument();
      expect(screen.queryByText('Description')).not.toBeInTheDocument();
      expect(screen.queryByText('Developer')).not.toBeInTheDocument();
      expect(screen.queryByText('Estimated Playtime')).not.toBeInTheDocument();
      expect(screen.queryByText('IGDB ID')).not.toBeInTheDocument();
      
      // Personal information section should still be available
      expect(screen.getByText('Your Information')).toBeInTheDocument();
    });
  });

  describe('Edge Cases', () => {
    it('should handle decimal rating values correctly', () => {
      const gameWithDecimalRating = createMockUserGame({
        game: {
          ...createMockUserGame().game,
          rating_average: 7.75
        }
      });
      mockUserGamesStore.value.userGames = [gameWithDecimalRating];
      
      render(GameDetailPage);
      
      expect(screen.getByText('7.8/10')).toBeInTheDocument(); // Should round to 1 decimal
    });

    it('should handle very large review counts with proper formatting', () => {
      const gameWithLargeReviewCount = createMockUserGame({
        game: {
          ...createMockUserGame().game,
          rating_count: 15432
        }
      });
      mockUserGamesStore.value.userGames = [gameWithLargeReviewCount];
      
      render(GameDetailPage);
      
      expect(screen.getByText('(15,432 reviews)')).toBeInTheDocument();
    });
  });
});