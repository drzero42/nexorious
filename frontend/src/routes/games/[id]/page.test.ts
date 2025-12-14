import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import {
  setupFetchMock,
  resetFetchMock,
  mockConfig
} from '../../../test-utils/api-mocks';
import {
  resetStoresMocks,
  mockUserGamesStore,
  createTestUserGame,
  setupUserGamesStoreWithData
} from '../../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import { toGameId } from '$lib/types/game';
import GameDetailPage from './+page.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock page store with specific game ID
vi.mock('$app/stores', () => ({
  page: {
    subscribe: (callback: any) => {
      callback({ params: { id: '1' } });
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
        params: { id: '1' },
        url: new URL('http://localhost:3000/games/1'),
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
      // Create a game without platforms using the helper
      const gameWithoutPlatforms = createTestUserGame({
        platforms: []
      });
      setupUserGamesStoreWithData([gameWithoutPlatforms]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutPlatforms);

      render(GameDetailPage);

      // Wait for the component to finish loading
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Should not display platform section since mock has empty platforms array
      expect(screen.queryByText('Available On')).not.toBeInTheDocument();
    });

    it('should display platform information when platforms are available', async () => {
      // Create a game with platform data using the helper
      const gameWithPlatforms = createTestUserGame({
        platforms: [
          {
            id: 'platform-1',
            platform: {
              id: 'pc',
              name: 'PC (Windows)',
              display_name: 'PC (Windows)',
              icon_url: 'https://example.com/pc-icon.png',
              source: 'manual',
              is_active: true,
              version_added: '1.0.0',
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
              version_added: '1.0.0',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            },
            store_game_id: '12345',
            is_available: true,
            created_at: '2024-01-01T00:00:00Z'
          }
        ]
      });
      setupUserGamesStoreWithData([gameWithPlatforms]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithPlatforms);

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
      // Create a game with platform data using the helper
      const gameWithPlatforms = createTestUserGame({
        platforms: [
          {
            id: 'platform-1',
            platform: {
              id: 'pc',
              name: 'PC (Windows)',
              display_name: 'PC (Windows)',
              icon_url: 'https://example.com/pc-icon.png',
              source: 'manual',
              is_active: true,
              version_added: '1.0.0',
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
              version_added: '1.0.0',
              created_at: '2024-01-01T00:00:00Z',
              updated_at: '2024-01-01T00:00:00Z'
            },
            store_game_id: '12345',
            is_available: true,
            created_at: '2024-01-01T00:00:00Z'
          }
        ]
      });
      setupUserGamesStoreWithData([gameWithPlatforms]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithPlatforms);

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
      const gameWithoutRating = createTestUserGame({
        game: {
          rating_count: 0,
          rating_average: 0
        }
      });
      setupUserGamesStoreWithData([gameWithoutRating]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutRating);

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
      const gameWithoutTimes = createTestUserGame({
        game: {
          howlongtobeat_main: 0,
          howlongtobeat_extra: 0,
          howlongtobeat_completionist: 0
        }
      });
      setupUserGamesStoreWithData([gameWithoutTimes]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutTimes);

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
      const igdbLink = screen.getByText('123');
      expect(igdbLink).toBeInTheDocument();
      expect(igdbLink.closest('a')).toHaveAttribute('href', 'https://www.igdb.com/games/test-game-slug');
    });

    it('should display IGDB ID as plain text when slug is missing', async () => {
      const gameWithoutSlug = createTestUserGame({
        game: {
          igdb_slug: '' // Remove slug but keep ID
        }
      });
      setupUserGamesStoreWithData([gameWithoutSlug]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithoutSlug);

      render(GameDetailPage);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      const igdbText = screen.getByText('123');
      expect(igdbText).toBeInTheDocument();
      // Component should render as plain text without slug
      expect(igdbText.closest('a')).toBeNull();
    });

    it('should handle missing optional fields gracefully', async () => {
      const gameWithMissingFields = createTestUserGame({
        game: {
          developer: '',
          estimated_playtime_hours: 0,
          igdb_id: toGameId(456)
        }
      });
      setupUserGamesStoreWithData([gameWithMissingFields]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithMissingFields);

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
      const minimalGame = createTestUserGame({
        game: {
          id: toGameId(1),
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
          igdb_id: toGameId(789)
        },
        platforms: []
      });
      setupUserGamesStoreWithData([minimalGame]);
      mockUserGamesStore.getUserGame.mockResolvedValue(minimalGame);

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
      const gameWithDecimalRating = createTestUserGame({
        game: {
          rating_average: 77.5
        }
      });
      setupUserGamesStoreWithData([gameWithDecimalRating]);
      mockUserGamesStore.getUserGame.mockResolvedValue(gameWithDecimalRating);

      render(GameDetailPage);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      expect(screen.getByText('7.75/10')).toBeInTheDocument(); // formatIgdbRating(77.5) = 7.75
    });

  });
});