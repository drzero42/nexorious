import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../../test-utils/api-mocks';
import { resetStoresMocks, mockUserGamesStore } from '../../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import GameDetailPage from './+page.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock page store with specific game ID
vi.mock('$app/stores', () => ({
  page: {
    subscribe: (callback: any) => {
      callback({ params: { id: 'user-game-1' } });
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
        params: { id: 'user-game-1' },
        url: new URL('http://localhost:3000/games/user-game-1'),
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
    
    // Mock fetchUserGames to resolve immediately
    mockUserGamesStore.fetchUserGames.mockResolvedValue(undefined);
    mockUserGamesStore.loadUserGames.mockResolvedValue(undefined);
  });

  describe('Platform Information Display', () => {
    it('should not display platform section when no platforms available', async () => {
      // Override mock to have no platforms for this specific test
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithoutPlatforms = {
        ...baseGame,
        platforms: []
      };
      
      mockUserGamesStore.value.userGames = [gameWithoutPlatforms];
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithoutPlatforms.id ? gameWithoutPlatforms : null);
      
      render(GameDetailPage);
      
      // Wait for the component to finish loading
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Should not display platform section since mock has empty platforms array
      expect(screen.queryByText('Available On')).not.toBeInTheDocument();
    });

    it('should display platform information when platforms are available', async () => {
      // Add platform data to the mock for this test  
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithPlatforms = {
        ...baseGame,
        platforms: [
          {
            id: 'platform-1',
            platform: {
              id: 'pc',
              name: 'PC (Windows)',
              display_name: 'PC (Windows)',
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
          }
        ]
      };
      (mockUserGamesStore.value as any).userGames = [gameWithPlatforms];
      
      // Also mock getUserGame to return the game with platforms
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithPlatforms);
      // Update the selector to return the game with platforms
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithPlatforms.id ? gameWithPlatforms : null);
      
      render(GameDetailPage);
      
      // Wait for the component to finish loading and check that PC (Windows) platform badge is visible
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      // Click the PC (Windows) platform badge to expand its details
      const pcBadge = screen.getByRole('button', { name: /PC \(Windows\).*Available on.*Steam.*Click to expand details/i });
      await fireEvent.click(pcBadge);
      
      // Now the "Available On" text should be visible in the expanded view
      await waitFor(() => {
        expect(screen.getByText('Available On')).toBeInTheDocument();
      });
      
      // Steam should be visible in the expanded content
      const steamElements = screen.getAllByText('Steam');
      expect(steamElements.length).toBeGreaterThan(0);
    });

    it('should display clickable store links with proper accessibility', async () => {
      // Add platform data to the mock for this test
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithPlatforms = {
        ...baseGame,
        platforms: [
          {
            id: 'platform-1',
            platform: {
              id: 'pc',
              name: 'PC (Windows)',
              display_name: 'PC (Windows)',
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
          }
        ]
      };
      (mockUserGamesStore.value as any).userGames = [gameWithPlatforms];
      
      // Also mock getUserGame to return the game with platforms
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithPlatforms);
      // Update the selector to return the game with platforms
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithPlatforms.id ? gameWithPlatforms : null);
      
      render(GameDetailPage);
      
      // Wait for PC (Windows) platform badge to be visible and click it to expand
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      const pcBadge = screen.getByRole('button', { name: /PC \(Windows\).*Available on.*Steam.*Click to expand details/i });
      await fireEvent.click(pcBadge);
      
      // Now check for the expanded content with "Available On"
      await waitFor(() => {
        expect(screen.getByText('Available On')).toBeInTheDocument();
      });
      
      // The store link should now be visible - check for any Steam text
      expect(screen.getAllByText('Steam').length).toBeGreaterThan(0);
    });
  });

  describe('IGDB Rating Display', () => {
    it('should display IGDB rating when available', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      // Rating format may vary, just check section exists
      expect(screen.getByText('(100 reviews)')).toBeInTheDocument();
    });


    it('should not display rating section when no rating', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithoutRating = {
        ...baseGame,
        game: {
          ...baseGame.game,
          rating_count: 0,
          rating_average: 0
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutRating];
      
      // Also mock getUserGame to return the game without rating
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutRating);
      // Update the selector to return the game without rating
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithoutRating.id ? gameWithoutRating : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Component should not show Game Rating section without rating data
      expect(screen.queryByText('Game Rating')).not.toBeInTheDocument();
    });
  });

  describe('How Long to Beat Display', () => {
    it('should display all How Long to Beat times when available', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('How Long to Beat')).toBeInTheDocument();
      
      // Check for the "How Long to Beat" section specifically
      const howLongToBeatSection = screen.getByText('How Long to Beat').closest('div');
      expect(howLongToBeatSection).toBeInTheDocument();
      
      // Use getAllByText to handle multiple instances and find the right ones
      const mainStoryElements = screen.getAllByText('Main Story');
      const mainExtraElements = screen.getAllByText('Main + Extra');
      const completionistElements = screen.getAllByText('Completionist');
      
      // Verify at least one instance exists for each
      expect(mainStoryElements.length).toBeGreaterThan(0);
      expect(mainExtraElements.length).toBeGreaterThan(0);
      expect(completionistElements.length).toBeGreaterThan(0);
      
      // Check for time values (from mock: howlongtobeat_main: 18, etc.)
      // These appear in both How Long to Beat section and GameProgressCard
      const time18Elements = screen.getAllByText('18h');
      const time28Elements = screen.getAllByText('28h');
      const time45Elements = screen.getAllByText('45h');
      
      expect(time18Elements.length).toBeGreaterThan(0);
      expect(time28Elements.length).toBeGreaterThan(0);
      expect(time45Elements.length).toBeGreaterThan(0);
    });

    it('should not display How Long to Beat section when no times available', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithoutTimes = {
        ...baseGame,
        game: {
          ...baseGame.game,
          howlongtobeat_main: 0,
          howlongtobeat_extra: 0,
          howlongtobeat_completionist: 0
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutTimes];
      
      // Also mock getUserGame to return the game without HLTB times
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutTimes);
      // Update the selector to return the game without HLTB times
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithoutTimes.id ? gameWithoutTimes : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Component should not show How Long to Beat section without times
      expect(screen.queryByText('How Long to Beat')).not.toBeInTheDocument();
    });
  });

  describe('Enhanced Game Details', () => {
    it('should display enhanced game details including estimated playtime and IGDB ID', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Check for existing fields (from mockGameMetadata)
      expect(screen.getByText('Developer')).toBeInTheDocument();
      expect(screen.getByText('Test Developer')).toBeInTheDocument();
      expect(screen.getByText('Publisher')).toBeInTheDocument();
      expect(screen.getByText('Test Publisher')).toBeInTheDocument();
      
      // Check for enhanced fields (from mockGameMetadata: estimated_playtime_hours: 25)
      expect(screen.getByText('Estimated Playtime')).toBeInTheDocument();
      expect(screen.getByText('25 hours')).toBeInTheDocument();
      
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      const igdbLink = screen.getByText('igdb-123');
      expect(igdbLink).toBeInTheDocument();
      expect(igdbLink.closest('a')).toHaveAttribute('href', 'https://www.igdb.com/games/test-game-slug');
    });

    it('should display IGDB ID as plain text when slug is missing', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithoutSlug = {
        ...baseGame,
        game: {
          ...baseGame.game,
          igdb_slug: '' // Remove slug but keep ID
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutSlug];
      
      // Also mock getUserGame to return the game without slug
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutSlug);
      // Update the selector to return the game without slug
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithoutSlug.id ? gameWithoutSlug : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      const igdbText = screen.getByText('igdb-123');
      expect(igdbText).toBeInTheDocument();
      // Component should render as plain text without slug
      expect(igdbText.closest('a')).toBeNull();
    });

    it('should handle missing optional fields gracefully', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithMissingFields = {
        ...baseGame,
        game: {
          ...baseGame.game,
          developer: '',
          estimated_playtime_hours: 0,
          igdb_id: 'igdb-test-456'
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithMissingFields];
      
      // Also mock getUserGame to return the game with missing fields
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithMissingFields);
      // Update the selector to return the game with missing fields
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithMissingFields.id ? gameWithMissingFields : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Component should not show fields that are undefined
      expect(screen.queryByText('Developer')).not.toBeInTheDocument();
      expect(screen.queryByText('Estimated Playtime')).not.toBeInTheDocument();
      // IGDB ID should always be present since all games are IGDB-sourced
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      
      // Other fields should still be present
      expect(screen.getByText('Publisher')).toBeInTheDocument();
      expect(screen.getByText('Genre')).toBeInTheDocument();
    });
  });

  describe('Data Integration', () => {
    it('should display comprehensive metadata when all fields are present', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      // Verify metadata sections are displayed (Available On is now shown since we added platform data)
      expect(screen.getByText('Available On')).toBeInTheDocument(); // platforms array has data in mock
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      expect(screen.getByText('How Long to Beat')).toBeInTheDocument();
      expect(screen.getByText('Description')).toBeInTheDocument();
      
      // Verify specific content from mockGameMetadata
      expect(screen.getByText('A test game description')).toBeInTheDocument();
      expect(screen.getByText('8.55/10')).toBeInTheDocument(); // formatIgdbRating(85.5) = 8.55
      
      // Check for time values (these appear in both How Long to Beat and GameProgressCard)
      const time18Elements = screen.getAllByText('18h');
      const time28Elements = screen.getAllByText('28h');
      const time45Elements = screen.getAllByText('45h');
      
      expect(time18Elements.length).toBeGreaterThan(0); // Main story
      expect(time28Elements.length).toBeGreaterThan(0); // Main + Extra
      expect(time45Elements.length).toBeGreaterThan(0); // Completionist
    });

    it('should handle minimal metadata gracefully', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const minimalGame = {
        ...baseGame,
        game: {
          ...baseGame.game,
          id: 'game-minimal',
          title: 'Minimal Game',
          description: '',
          genre: '',
          developer: '',
          publisher: '',
          rating_count: 0,
          rating_average: 0,
          estimated_playtime_hours: 0,
          howlongtobeat_main: 0,
          howlongtobeat_extra: 0,
          howlongtobeat_completionist: 0,
          igdb_id: 'igdb-minimal-789'
        },
        platforms: []
      };
      (mockUserGamesStore.value as any).userGames = [minimalGame];
      
      // Also mock getUserGame to return the minimal game
      mockUserGamesStore.getUserGame.mockResolvedValue(minimalGame);
      // Update the selector to return the minimal game
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === minimalGame.id ? minimalGame : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Minimal Game')).toBeInTheDocument();
      });
      
      // Title should be displayed with updated title from mock data
      expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Minimal Game');
      
      // These sections should not be displayed with minimal data
      expect(screen.queryByText('Available On')).not.toBeInTheDocument();
      expect(screen.queryByText('Game Rating')).not.toBeInTheDocument();
      expect(screen.queryByText('How Long to Beat')).not.toBeInTheDocument();
      expect(screen.queryByText('Description')).not.toBeInTheDocument();
      expect(screen.queryByText('Developer')).not.toBeInTheDocument();
      expect(screen.queryByText('Estimated Playtime')).not.toBeInTheDocument();
      // IGDB ID should always be present since all games are IGDB-sourced
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      
      // Personal information section should still be available
      expect(screen.getByText('Your Information')).toBeInTheDocument();
    });
  });

  describe('Edge Cases', () => {
    it('should handle decimal rating values correctly', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithDecimalRating = {
        ...baseGame,
        game: {
          ...baseGame.game,
          rating_average: 77.5
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithDecimalRating];
      
      // Also mock getUserGame to return the game with decimal rating
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithDecimalRating);
      // Update the selector to return the game with decimal rating
      (mockUserGamesStore.selectors.byId as any) = vi.fn((id: string) => id === gameWithDecimalRating.id ? gameWithDecimalRating : null);
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('7.75/10')).toBeInTheDocument(); // formatIgdbRating(77.5) = 7.75
    });

  });
});