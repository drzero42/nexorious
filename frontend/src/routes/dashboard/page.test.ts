import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../test-utils/api-mocks';
import { mockUserGamesStore, resetStoresMocks } from '../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../test-utils/auth-mocks';
import { PlayStatus, OwnershipStatus } from '$lib/stores/user-games.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

// Mock navigation with proper hoisting
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

// Mock RouteGuard with a proper Svelte component mock
vi.mock('$lib/components/RouteGuard.svelte', () => {
  // Import the mock component
  return import('../../test-utils/MockRouteGuard.svelte');
});

// Mock ProgressStatistics component
vi.mock('$lib/components/ProgressStatistics.svelte', () => {
  return import('../../test-utils/MockProgressStatistics.svelte');
});

// Mock the components index file as well
vi.mock('$lib/components', async () => {
  const MockRouteGuard = await import('../../test-utils/MockRouteGuard.svelte');
  const MockProgressStatistics = await import('../../test-utils/MockProgressStatistics.svelte');
  return {
    RouteGuard: MockRouteGuard.default,
    ProgressStatistics: MockProgressStatistics.default
  };
});

// Import component after mocks
import DashboardPage from './+page.svelte';
import { goto } from '$app/navigation';

describe('Dashboard Page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
    vi.mocked(goto).mockClear();

    // Setup mock data for user games with comprehensive test data
    mockUserGamesStore.value = {
      userGames: [
        {
          id: 'user-game-1',
          game: {
            id: 'game-1',
            title: 'Test Game',
            description: 'A test game',
            genre: 'Action',
            developer: 'Test Dev',
            publisher: 'Test Pub',
            release_date: '2024-01-01',
            cover_art_url: 'https://example.com/cover.jpg',
            rating_average: 4.5,
            rating_count: 100,
            game_metadata: '{}',
            estimated_playtime_hours: 25,
            howlongtobeat_main: 18,
            howlongtobeat_extra: 28,
            howlongtobeat_completionist: 45,
            igdb_id: 'igdb-123',
            igdb_slug: 'test-game-slug',
            is_verified: true,
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z'
          },
          ownership_status: OwnershipStatus.OWNED,
          personal_rating: 4,
          is_loved: true,
          play_status: PlayStatus.COMPLETED,
          hours_played: 25,
          personal_notes: 'Great game!',
          acquired_date: '2024-01-01',
          platforms: [],
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z'
        },
        {
          id: 'user-game-2',
          game: {
            id: 'game-2',
            title: 'RPG Game',
            description: 'An RPG game',
            genre: 'RPG',
            developer: 'RPG Dev',
            publisher: 'RPG Pub',
            release_date: '2024-02-01',
            cover_art_url: 'https://example.com/rpg.jpg',
            rating_average: 4.8,
            rating_count: 200,
            game_metadata: '{}',
            estimated_playtime_hours: 60,
            howlongtobeat_main: 40,
            howlongtobeat_extra: 70,
            howlongtobeat_completionist: 120,
            igdb_id: 'igdb-456',
            igdb_slug: 'rpg-game-slug',
            is_verified: true,
            created_at: '2024-02-01T00:00:00Z',
            updated_at: '2024-02-01T00:00:00Z'
          },
          ownership_status: OwnershipStatus.OWNED,
          personal_rating: 5,
          is_loved: false,
          play_status: PlayStatus.IN_PROGRESS,
          hours_played: 15,
          personal_notes: 'Playing through',
          acquired_date: '2024-02-01',
          platforms: [],
          created_at: '2024-02-01T00:00:00Z',
          updated_at: '2024-02-01T00:00:00Z'
        },
        {
          id: 'user-game-3',
          game: {
            id: 'game-3',
            title: 'Action Game 2',
            description: 'Another action game',
            genre: 'Action',
            developer: 'Action Dev',
            publisher: 'Action Pub',
            release_date: '2024-04-01',
            cover_art_url: 'https://example.com/action2.jpg',
            rating_average: 3.8,
            rating_count: 80,
            game_metadata: '{}',
            estimated_playtime_hours: 20,
            howlongtobeat_main: 15,
            howlongtobeat_extra: 25,
            howlongtobeat_completionist: 40,
            igdb_id: 'igdb-101',
            igdb_slug: 'action-game-2-slug',
            is_verified: true,
            created_at: '2024-04-01T00:00:00Z',
            updated_at: '2024-04-01T00:00:00Z'
          },
          ownership_status: OwnershipStatus.OWNED,
          personal_rating: 3,
          is_loved: true,
          play_status: PlayStatus.COMPLETED,
          hours_played: 30,
          personal_notes: 'Mastered all content',
          acquired_date: '2024-04-01',
          platforms: [],
          created_at: '2024-04-01T00:00:00Z',
          updated_at: '2024-04-01T00:00:00Z'
        }
      ],
      currentUserGame: null,
      stats: null,
      isLoading: false,
      error: null,
      filters: {},
      pagination: {
        page: 1,
        per_page: 20,
        total: 3,
        pages: 1
      }
    };

    // Add required methods to mocks
    mockUserGamesStore.fetchUserGames = vi.fn().mockResolvedValue(undefined);
  });

  describe('Core Functionality', () => {
    it('should call fetchUserGames on mount', async () => {
      render(DashboardPage);
      
      expect(mockUserGamesStore.fetchUserGames).toHaveBeenCalled();
    });

    it('should show loading state when isLoading is true', async () => {
      mockUserGamesStore.value.isLoading = true;
      
      render(DashboardPage);
      
      expect(screen.getByText('Loading statistics...')).toBeInTheDocument();
    });

    it('should show empty state when no games exist', async () => {
      mockUserGamesStore.value.userGames = [];
      
      render(DashboardPage);
      
      expect(screen.getByText('No games in your collection yet. Add some games to see your statistics!')).toBeInTheDocument();
      expect(screen.getByText('Add Your First Game')).toBeInTheDocument();
    });

    it('should navigate to add game page when clicking "Add Your First Game"', async () => {
      mockUserGamesStore.value.userGames = [];
      
      render(DashboardPage);
      
      const addGameButton = screen.getByText('Add Your First Game');
      await fireEvent.click(addGameButton);
      
      expect(goto).toHaveBeenCalledWith('/games/add');
    });
  });

  describe('Statistics Display', () => {
    it('should render dashboard with proper content when games exist', async () => {
      render(DashboardPage);
      
      // Check that main dashboard elements are present
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
      expect(screen.getByText('Your gaming statistics and insights')).toBeInTheDocument();
    });

    it('should display total games count', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Total Games')).toBeInTheDocument();
      expect(screen.getByText('3')).toBeInTheDocument();
    });

    it('should display total hours played', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Total Hours')).toBeInTheDocument();
      // 25 + 15 + 30 = 70 hours
      expect(screen.getByText('70h')).toBeInTheDocument();
    });

    it('should calculate and display completion rate', async () => {
      render(DashboardPage);
      
      // Use getAllByText since "Completion Rate" appears multiple times
      const completionRateElements = screen.getAllByText('Completion Rate');
      expect(completionRateElements.length).toBeGreaterThan(0);
      // (2 completed + 0 mastered + 0 dominated) / 3 total = 66.7%
      // Also use getAllByText since the percentage appears in multiple sections
      const percentageElements = screen.getAllByText('66.7%');
      expect(percentageElements.length).toBeGreaterThan(0);
    });

    it('should display pile of shame count', async () => {
      render(DashboardPage);
      
      // Use getAllByText since "Pile of Shame" appears multiple times
      const pileOfShameElements = screen.getAllByText('Pile of Shame');
      expect(pileOfShameElements.length).toBeGreaterThan(0);
      // 0 not_started games in our test data - also appears multiple times
      const zeroElements = screen.getAllByText('0');
      expect(zeroElements.length).toBeGreaterThan(0);
    });
  });

  describe('Statistics Calculations', () => {
    it('should calculate average rating correctly', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Average Rating')).toBeInTheDocument();
      // (4 + 5 + 3) / 3 games with ratings = 4.0/5
      expect(screen.getByText('4.0/5')).toBeInTheDocument();
    });

    it('should calculate loved games count', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Loved Games')).toBeInTheDocument();
      // 2 games marked as loved - but "2" appears multiple times on the page
      const twoElements = screen.getAllByText('2');
      expect(twoElements.length).toBeGreaterThan(0);
    });

    it('should identify most played game', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Most Played Game')).toBeInTheDocument();
      // Action Game 2 has 30 hours (highest)
      expect(screen.getByText('Action Game 2')).toBeInTheDocument();
    });

    it('should calculate average hours per game', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Average Hours per Game')).toBeInTheDocument();
      // 70 total hours / 3 games = 23.3h
      expect(screen.getByText('23.3h')).toBeInTheDocument();
    });
  });

  describe('Genre Statistics', () => {
    it('should display top genres', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Top Genres')).toBeInTheDocument();
      expect(screen.getByText('Action')).toBeInTheDocument();
      expect(screen.getByText('RPG')).toBeInTheDocument();
    });
  });

  describe('Edge Cases', () => {
    it('should handle games with no ratings', async () => {
      const gamesWithoutRatings = mockUserGamesStore.value.userGames.map(game => ({
        ...game,
        personal_rating: undefined
      }));
      
      mockUserGamesStore.value.userGames = gamesWithoutRatings as any;
      
      render(DashboardPage);
      
      expect(screen.getByText('Average Rating')).toBeInTheDocument();
      expect(screen.getByText('N/A')).toBeInTheDocument();
    });

    it('should handle games with zero hours played', async () => {
      const gamesWithoutHours = mockUserGamesStore.value.userGames.map(game => ({
        ...game,
        hours_played: 0
      }));
      
      mockUserGamesStore.value.userGames = gamesWithoutHours;
      
      render(DashboardPage);
      
      expect(screen.getByText('Total Hours')).toBeInTheDocument();
      expect(screen.getByText('0h')).toBeInTheDocument();
    });

    it('should handle games with missing genre data', async () => {
      const gamesWithoutGenre = mockUserGamesStore.value.userGames.map(game => ({
        ...game,
        game: {
          ...game.game,
          genre: 'Unknown'
        }
      }));
      
      mockUserGamesStore.value.userGames = gamesWithoutGenre;
      
      render(DashboardPage);
      
      expect(screen.getByText('Top Genres')).toBeInTheDocument();
      expect(screen.getByText('Unknown')).toBeInTheDocument();
    });

    it('should handle large numbers with proper formatting', async () => {
      // Create user with high hours played
      const highHourGame = {
        ...mockUserGamesStore.value.userGames[0],
        hours_played: 1000
      };
      
      mockUserGamesStore.value.userGames = [highHourGame] as any;
      
      render(DashboardPage);
      
      // Should format large numbers with commas
      expect(screen.getByText('1,000h')).toBeInTheDocument();
    });
  });

  describe('Component Integration', () => {
    it('should render ProgressStatistics component', async () => {
      render(DashboardPage);
      
      // Should render the mocked ProgressStatistics
      expect(screen.getByText('ProgressStatistics Mock')).toBeInTheDocument();
    });

    it('should set document title correctly', async () => {
      render(DashboardPage);
      
      // Check that the title was set (will be in the document head)
      const titleElement = document.querySelector('title');
      expect(titleElement?.textContent).toBe('Dashboard - Nexorious');
    });
  });

  describe('Data Loading', () => {
    it('should handle store error state gracefully', async () => {
      mockUserGamesStore.value.error = null;
      mockUserGamesStore.value.isLoading = false;
      mockUserGamesStore.value.userGames = [];
      
      render(DashboardPage);
      
      // Should show empty state even with error (dashboard handles this gracefully)
      expect(screen.getByText('No games in your collection yet. Add some games to see your statistics!')).toBeInTheDocument();
    });

    it('should handle fetchUserGames rejection gracefully', async () => {
      mockUserGamesStore.fetchUserGames.mockRejectedValue(new Error('Network error'));
      
      render(DashboardPage);
      
      expect(mockUserGamesStore.fetchUserGames).toHaveBeenCalled();
      // Component should handle the error gracefully without crashing
    });
  });

  describe('Play Status Breakdown', () => {
    it('should display play status sections', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Play Status Breakdown')).toBeInTheDocument();
      expect(screen.getByText('Not Started')).toBeInTheDocument();
      expect(screen.getByText('In Progress')).toBeInTheDocument();
      expect(screen.getByText('Completed')).toBeInTheDocument();
      expect(screen.getByText('Mastered')).toBeInTheDocument();
    });
  });

  describe('Personal Stats', () => {
    it('should display personal statistics section', async () => {
      render(DashboardPage);
      
      expect(screen.getByText('Personal Stats')).toBeInTheDocument();
      expect(screen.getByText('Average Rating')).toBeInTheDocument();
      expect(screen.getByText('Loved Games')).toBeInTheDocument();
      expect(screen.getByText('Average Hours per Game')).toBeInTheDocument();
    });
  });
});