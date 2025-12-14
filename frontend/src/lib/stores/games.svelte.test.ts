import { describe, it, expect, beforeEach, vi } from 'vitest';
import { toGameId } from '$lib/types/game';
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

// Mock the auth module - use vi.hoisted to ensure mock is available before import
const mockAuth = vi.hoisted(() => ({
  auth: {
    value: {
      accessToken: 'test-token' as string | null,
      user: { id: '1', username: 'testuser' } as { id: string; username: string } | null
    },
    refreshAuth: vi.fn(() => Promise.resolve(true))
  }
}));

vi.mock('./auth.svelte', () => mockAuth);

// Import store after mocks are set up
import { games } from './games.svelte';

describe('Games Store API Integration', () => {
  let mockFetch: ReturnType<typeof setupFetchMock>;

  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    mockFetch = setupFetchMock();
    games.reset();
    // Reset auth mock to default authenticated state
    mockAuth.auth.value.accessToken = 'test-token';
    mockAuth.auth.value.user = { id: '1', username: 'testuser' };
  });

  describe('API URL Configuration', () => {
    it('should use config.apiUrl for games list endpoint', async () => {
      await games.loadGames();

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toContain(`${mockConfig.apiUrl}/games`);
    });

    it('should use config.apiUrl for single game fetch', async () => {
      await games.getGame(toGameId(123));

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/123`);
    });

    it('should use config.apiUrl for IGDB search endpoint', async () => {
      await games.searchIGDB('test game');

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/search/igdb`);
    });

    it('should use config.apiUrl for IGDB import endpoint', async () => {
      await games.createFromIGDB(toGameId(123));

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/igdb-import?download_cover_art=true`);
    });

    it('should use config.apiUrl for game deletion', async () => {
      await games.deleteGame(toGameId(123));

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/123`);
    });

    it('should use config.apiUrl for metadata refresh', async () => {
      await games.refreshMetadata(toGameId(123));

      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);

      const callUrl = mockFetch.mock.calls[0]?.[0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/123/metadata/refresh`);
    });
  });

  describe('IGDB Integration', () => {
    it('should send query parameter (not title) in IGDB search request', async () => {
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

      const result = await games.searchIGDB('test game');

      expect(result).toBeDefined();
      expect(result.games).toEqual(mockIGDBCandidates);
      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
    });

    it('should update store state with IGDB candidates from games property', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));

      await games.searchIGDB('test game');

      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
      expect(games.value.isLoading).toBe(false);
      expect(games.value.error).toBe(null);
    });

    it('should handle IGDB import with correct parameters', async () => {
      await games.createFromIGDB(123 as any, { title: 'Test Game' });

      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/igdb-import?download_cover_art=true`,
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Content-Type': 'application/json'
          }),
          body: JSON.stringify({
            igdb_id: 123,
            custom_overrides: { title: 'Test Game' }
          })
        })
      );
    });
  });

  describe('Game CRUD Operations', () => {
    it('should fetch games list with pagination parameters', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockGamesListEndpoint(mockGameListResponse));

      await games.loadGames({}, 2, 10);

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

      await games.loadGames();

      expect(games.value.games).toEqual(mockGames);
      expect(games.value.isLoading).toBe(false);
      expect(games.value.error).toBe(null);
    });

    it('should fetch single game by ID', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockGameEndpoint(123, mockGame));

      const result = await games.getGame(toGameId(123));

      expect(result).toEqual(mockGame);
      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/123`,
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token'
          })
        })
      );
    });

    it('should delete game by ID', async () => {
      await games.deleteGame(toGameId(123));

      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/123`,
        expect.objectContaining({
          method: 'DELETE'
        })
      );
    });

    it('should refresh game metadata with options', async () => {
      await games.refreshMetadata(toGameId(123), ['title', 'description'], true);

      expect(mockFetch).toHaveBeenCalledWith(
        `${mockConfig.apiUrl}/games/123/metadata/refresh`,
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
      // Test error response - apiCall throws "HTTP {status}: {statusText}"
      mockFetch.mockImplementation(APIResponseMock.mockErrorResponse(404, 'Game not found'));

      await expect(games.getGame(toGameId(99999))).rejects.toThrow('HTTP 404: Error');

      expect(games.value.error).toBe('HTTP 404: Error');
      expect(games.value.isLoading).toBe(false);
    });

    it('should handle network errors gracefully', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockNetworkError());

      await expect(games.loadGames()).rejects.toThrow('Network error');

      expect(games.value.error).toBe('Network error');
      expect(games.value.isLoading).toBe(false);
    });

    it('should clear error state when clearError is called', async () => {
      // Create an error state by causing a failed API call
      mockFetch.mockImplementation(APIResponseMock.mockErrorResponse(500, 'Server error'));
      await expect(games.getGame(toGameId(99999))).rejects.toThrow('HTTP 500: Error');

      // Verify error state was set
      expect(games.value.error).toBe('HTTP 500: Error');

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
      // Set auth mock to unauthenticated state
      mockAuth.auth.value.accessToken = null;
      mockAuth.auth.value.user = null;

      await expect(games.loadGames()).rejects.toThrow('Not authenticated');
    });
  });

  describe('State Management', () => {
    it('should initialize with correct default state', () => {
      // reset() was called in beforeEach, so state should be initial
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