import { describe, it, expect, beforeEach, vi } from 'vitest';
import { 
  APIResponseMock, 
  setupFetchMock, 
  resetFetchMock, 
  verifyAPIUrlUsage,
  mockConfig,
  mockGame,
  mockGames,
  mockGameListResponse,
  mockIGDBSearchResponse,
  mockIGDBCandidates 
} from '../../test-utils/api-mocks';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('./auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

describe('Games Store API Integration', () => {
  let mockFetch: any;
  
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    mockFetch = setupFetchMock();
  });

  describe('API URL Configuration', () => {
    it('should use config.apiUrl for games list endpoint', async () => {
      const { games } = await import('./games.svelte');
      
      await games.loadGames();
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toContain(`${mockConfig.apiUrl}/games`);
    });

    it('should use config.apiUrl for single game fetch', async () => {
      const { games } = await import('./games.svelte');
      
      await games.getGame('test-game-id');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/test-game-id`);
    });

    it('should use config.apiUrl for IGDB search endpoint', async () => {
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/search/igdb`);
    });

    it('should use config.apiUrl for IGDB import endpoint', async () => {
      const { games } = await import('./games.svelte');
      
      await games.createFromIGDB('igdb-123');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/igdb-import`);
    });

    it('should use config.apiUrl for game creation', async () => {
      const { games } = await import('./games.svelte');
      
      await games.createGame({ title: 'New Game' });
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games`);
    });

    it('should use config.apiUrl for game updates', async () => {
      const { games } = await import('./games.svelte');
      
      await games.updateGame('game-123', { title: 'Updated Game' });
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/game-123`);
    });

    it('should use config.apiUrl for game deletion', async () => {
      const { games } = await import('./games.svelte');
      
      await games.deleteGame('game-123');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/game-123`);
    });

    it('should use config.apiUrl for metadata refresh', async () => {
      const { games } = await import('./games.svelte');
      
      await games.refreshMetadata('game-123');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/game-123/metadata/refresh`);
    });
  });

  describe('IGDB Integration', () => {
    it('should send query parameter (not title) in IGDB search request', async () => {
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game title', 10);
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/search/igdb`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify({
            query: 'test game title',
            limit: 10
          })
        })
      );
    });

    it('should handle IGDB response with games property (not candidates)', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));
      
      const { games } = await import('./games.svelte');
      
      const result = await games.searchIGDB('test game');
      
      expect(result).toBeDefined();
      expect(result.games).toEqual(mockIGDBCandidates);
      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
    });

    it('should update store state with IGDB candidates from games property', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));
      
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game');
      
      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
      expect(games.value.isLoading).toBe(false);
      expect(games.value.error).toBe(null);
    });

    it('should handle IGDB import with correct parameters', async () => {
      const { games } = await import('./games.svelte');
      
      await games.createFromIGDB('igdb-123', { title: 'Test Game' });
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/igdb-import`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify({
            igdb_id: 'igdb-123',
            custom_overrides: { title: 'Test Game' }
          })
        })
      );
    });
  });

  describe('Game CRUD Operations', () => {
    it('should fetch games list with pagination parameters', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockGamesListEndpoint(mockGameListResponse));
      
      const { games } = await import('./games.svelte');
      
      await games.loadGames({ page: 2, per_page: 10 });
      
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining(`${mockConfig.apiUrl}/games?page=2&per_page=10`),
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token'
          })
        })
      );
    });

    it('should update store state after successful games fetch', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockGamesListEndpoint(mockGameListResponse));
      
      const { games } = await import('./games.svelte');
      
      await games.loadGames();
      
      expect(games.value.games).toEqual(mockGames);
      expect(games.value.isLoading).toBe(false);
      expect(games.value.error).toBe(null);
    });

    it('should fetch single game by ID', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockGameEndpoint('game-123', mockGame));
      
      const { games } = await import('./games.svelte');
      
      const result = await games.getGame('game-123');
      
      expect(result).toEqual(mockGame);
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/game-123`,
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token'
          })
        })
      );
    });

    it('should add new game with correct data', async () => {
      const { games } = await import('./games.svelte');
      const newGameData = { title: 'New Game', genre: 'Action', game_metadata: '{}' };
      
      await games.createGame(newGameData);
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify(newGameData)
        })
      );
    });

    it('should update existing game with correct data', async () => {
      const { games } = await import('./games.svelte');
      const updateData = { title: 'Updated Game' };
      
      await games.updateGame('game-123', updateData);
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/game-123`,
        expect.objectContaining({
          method: 'PUT',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify(updateData)
        })
      );
    });

    it('should delete game by ID', async () => {
      const { games } = await import('./games.svelte');
      
      await games.deleteGame('game-123');
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/game-123`,
        expect.objectContaining({
          method: 'DELETE'
        })
      );
    });

    it('should refresh game metadata with options', async () => {
      const { games } = await import('./games.svelte');
      
      await games.refreshMetadata('game-123', ['title', 'description'], true);
      
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/game-123/metadata/refresh`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify({
            fields: ['title', 'description'],
            force: true
          })
        })
      );
    });
  });

  describe('Error Handling', () => {
    it('should handle API errors and update error state', async () => {
      // Test error response
      mockFetch.mockImplementation(APIResponseMock.mockErrorResponse(404, 'Game not found'));
      
      const { games } = await import('./games.svelte');
      
      try {
        await games.getGame('nonexistent-game');
      } catch (error) {
        // Error is expected
      }
      
      expect(games.value.error).toBeTruthy();
      expect(games.value.isLoading).toBe(false);
    });

    it('should handle network errors gracefully', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockNetworkError());
      
      const { games } = await import('./games.svelte');
      
      try {
        await games.loadGames();
      } catch (error) {
        // Error is expected
      }
      
      expect(games.value.error).toBeTruthy();
      expect(games.value.isLoading).toBe(false);
    });

    it('should clear error state when clearError is called', async () => {
      const { games } = await import('./games.svelte');
      
      // Create an error state by causing a failed API call
      mockFetch.mockImplementation(APIResponseMock.mockErrorResponse(500, 'Server error'));
      try {
        await games.getGame('nonexistent-game');
      } catch (error) {
        // Expected error
      }
      
      // Verify error state was set
      expect(games.value.error).toBeTruthy();
      
      // Clear the error
      games.clearError();
      
      expect(games.value.error).toBe(null);
    });

    it('should set loading state during API calls', async () => {
      let resolvePromise: (value: any) => void;
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      
      mockFetch.mockReturnValue(pendingPromise);
      
      const { games } = await import('./games.svelte');
      
      // Start the async operation
      const fetchPromise = games.loadGames();
      
      // Check that loading state is set
      expect(games.value.isLoading).toBe(true);
      
      // Resolve the promise and wait for completion
      resolvePromise!(APIResponseMock.createResponse(mockGameListResponse));
      await fetchPromise;
      
      expect(games.value.isLoading).toBe(false);
    });
  });

  describe('Authentication Integration', () => {
    it('should include Authorization header in API requests', async () => {
      const { games } = await import('./games.svelte');
      
      await games.loadGames();
      
      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token'
          })
        })
      );
    });

    it('should handle requests without authentication token', async () => {
      // Mock auth store without token
      vi.doMock('./auth.svelte', () => ({
        auth: {
          value: {
            accessToken: null,
            user: null
          }
        }
      }));

      // Re-import to get updated mock
      vi.resetModules();
      const { games } = await import('./games.svelte');
      
      try {
        await games.loadGames();
      } catch (error) {
        // Expected to fail with "Not authenticated"
        expect((error as Error).message).toBe('Not authenticated');
      }
    });
  });

  describe('State Management', () => {
    it('should initialize with correct default state', async () => {
      // Reset modules to get a fresh store instance
      vi.resetModules();
      const { games } = await import('./games.svelte');
      
      expect(games.value).toEqual(
        expect.objectContaining({
          games: expect.any(Array),
          currentGame: null,
          searchResults: expect.any(Array),
          igdbCandidates: expect.any(Array),
          isLoading: false,
          error: null,
          filters: expect.any(Object),
          pagination: expect.any(Object)
        })
      );
    });

    it('should clear search results when requested', async () => {
      // Reset modules to get a fresh store after the auth test
      vi.resetModules();
      
      // Re-mock the auth store with authenticated state for this test
      vi.doMock('./auth.svelte', () => ({
        auth: {
          value: {
            accessToken: 'test-token',
            user: { id: '1', username: 'testuser' }
          }
        }
      }));
      
      const { games } = await import('./games.svelte');
      
      // Set some search results by performing a search first
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));
      await games.searchIGDB('test game');
      
      // Verify we have search results before clearing
      expect(games.value.igdbCandidates.length).toBeGreaterThan(0);
      
      // Clear the search results
      games.clearSearch();
      
      expect(games.value.igdbCandidates).toEqual([]);
    });
  });
});