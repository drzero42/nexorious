import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
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
          }
        ]
      };
      (mockUserGamesStore.value as any).userGames = [gameWithPlatforms];
      
      render(GameDetailPage);
      
      // Wait for the component to finish loading
      await waitFor(() => {
        expect(screen.getByText('Available On')).toBeInTheDocument();
      });
      
      // Check for platform names - now displayed in grouped format
      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('Steam')).toBeInTheDocument();
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
          }
        ]
      };
      (mockUserGamesStore.value as any).userGames = [gameWithPlatforms];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Available On')).toBeInTheDocument();
      });
      
      const steamLink = screen.getByLabelText('View PC on Steam');
      expect(steamLink).toBeInTheDocument();
      expect(steamLink).toHaveAttribute('href', 'https://store.steampowered.com/app/12345/test-game/');
      expect(steamLink).toHaveAttribute('target', '_blank');
      expect(steamLink).toHaveAttribute('rel', 'noopener noreferrer');
    });
  });

  describe('IGDB Rating and Verification Display', () => {
    it('should display IGDB rating when available', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      expect(screen.getByText('4.5/10')).toBeInTheDocument();
      expect(screen.getByText('(100 reviews)')).toBeInTheDocument();
    });

    it('should display verification badge when game is verified', async () => {
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      const verifiedBadge = screen.getByText('Verified');
      expect(verifiedBadge).toBeInTheDocument();
      expect(verifiedBadge.closest('.bg-green-100')).toBeInTheDocument();
    });

    it('should not display rating section when no rating or verification', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithoutRating = {
        ...baseGame,
        game: {
          ...baseGame.game,
          rating_count: 0,
          rating_average: undefined,
          is_verified: false
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutRating];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
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
          howlongtobeat_main: undefined,
          howlongtobeat_extra: undefined,
          howlongtobeat_completionist: undefined
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutTimes];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
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
          igdb_slug: undefined // Remove slug but keep ID
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithoutSlug];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('IGDB ID')).toBeInTheDocument();
      const igdbText = screen.getByText('igdb-123');
      expect(igdbText).toBeInTheDocument();
      // Should NOT be a link when no slug
      expect(igdbText.closest('a')).toBeNull();
      expect(igdbText.tagName.toLowerCase()).toBe('span');
    });

    it('should handle missing optional fields gracefully', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithMissingFields = {
        ...baseGame,
        game: {
          ...baseGame.game,
          developer: undefined,
          estimated_playtime_hours: undefined,
          igdb_id: undefined
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithMissingFields];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.queryByText('Developer')).not.toBeInTheDocument();
      expect(screen.queryByText('Estimated Playtime')).not.toBeInTheDocument();
      expect(screen.queryByText('IGDB ID')).not.toBeInTheDocument();
      
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
      
      // Verify metadata sections are displayed (note: Available On won't be displayed due to empty platforms)
      expect(screen.queryByText('Available On')).not.toBeInTheDocument(); // platforms array is empty in mock
      expect(screen.getByText('Game Rating')).toBeInTheDocument();
      expect(screen.getByText('How Long to Beat')).toBeInTheDocument();
      expect(screen.getByText('Description')).toBeInTheDocument();
      
      // Verify specific content from mockGameMetadata
      expect(screen.getByText('A test game description')).toBeInTheDocument();
      expect(screen.getByText('4.5/10')).toBeInTheDocument();
      expect(screen.getByText('Verified')).toBeInTheDocument();
      
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
          description: undefined,
          genre: undefined,
          developer: undefined,
          publisher: undefined,
          rating_count: 0,
          rating_average: undefined,
          estimated_playtime_hours: undefined,
          howlongtobeat_main: undefined,
          howlongtobeat_extra: undefined,
          howlongtobeat_completionist: undefined,
          igdb_id: undefined,
          is_verified: false
        },
        platforms: []
      };
      (mockUserGamesStore.value as any).userGames = [minimalGame];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Minimal Game')).toBeInTheDocument();
      });
      
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
    it('should handle decimal rating values correctly', async () => {
      const baseGame = mockUserGamesStore.value.userGames[0];
      if (!baseGame) throw new Error('Base game not found in mock');
      
      const gameWithDecimalRating = {
        ...baseGame,
        game: {
          ...baseGame.game,
          rating_average: 7.75
        }
      };
      (mockUserGamesStore.value as any).userGames = [gameWithDecimalRating];
      
      render(GameDetailPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });
      
      expect(screen.getByText('7.8/10')).toBeInTheDocument(); // Should round to 1 decimal
    });

  });
});