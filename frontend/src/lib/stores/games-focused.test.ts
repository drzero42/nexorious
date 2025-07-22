import { describe, it, expect, beforeEach, vi } from 'vitest';
import { 
  APIResponseMock, 
  setupFetchMock, 
  resetFetchMock, 
  verifyAPIUrlUsage,
  mockConfig,
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

describe('Games Store - PR Focused Tests', () => {
  let mockFetch: any;
  
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    mockFetch = setupFetchMock();
  });

  describe('API URL Configuration (PR Fix)', () => {
    it('should use config.apiUrl instead of hardcoded /api/ for games list', async () => {
      const { games } = await import('./games.svelte');
      
      await games.loadGames();
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toContain(`${mockConfig.apiUrl}/games`);
      expect(callUrl.startsWith(mockConfig.apiUrl)).toBe(true);
    });

    it('should use config.apiUrl instead of hardcoded /api/ for IGDB search', async () => {
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/search/igdb`);
      expect(callUrl.startsWith(mockConfig.apiUrl)).toBe(true);
    });

    it('should use config.apiUrl instead of hardcoded /api/ for IGDB import', async () => {
      const { games } = await import('./games.svelte');
      
      await games.createFromIGDB('igdb-123');
      
      expect(mockFetch).toHaveBeenCalled();
      verifyAPIUrlUsage(mockFetch, mockConfig.apiUrl);
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/igdb-import`);
      expect(callUrl.startsWith(mockConfig.apiUrl)).toBe(true);
    });

    it('should use config.apiUrl for other game operations', async () => {
      const { games } = await import('./games.svelte');
      
      await games.updateGame('game-123', { title: 'Updated' });
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe(`${mockConfig.apiUrl}/games/game-123`);
      expect(callUrl.startsWith(mockConfig.apiUrl)).toBe(true);
    });
  });

  describe('IGDB API Request Structure (PR Fix)', () => {
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
            query: 'test game title',  // Should be 'query' not 'title'
            limit: 10
          })
        })
      );
    });

    it('should not send title parameter in IGDB search request', async () => {
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game title', 10);
      
      const requestBody = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(requestBody).toHaveProperty('query');
      expect(requestBody).not.toHaveProperty('title');
    });
  });

  describe('IGDB Response Structure (PR Fix)', () => {
    it('should handle IGDB response with games property (not candidates)', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));
      
      const { games } = await import('./games.svelte');
      
      const result = await games.searchIGDB('test game');
      
      expect(result).toBeDefined();
      expect(result.games).toEqual(mockIGDBCandidates);
    });

    it('should update store state with IGDB candidates from games property', async () => {
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(mockIGDBSearchResponse));
      
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test game');
      
      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
      expect(games.value.isLoading).toBe(false);
      expect(games.value.error).toBe(null);
    });

    it('should not expect candidates property in IGDB response', async () => {
      // Mock response with games property only
      const responseWithGamesOnly = { games: mockIGDBCandidates, total: 1 };
      mockFetch.mockImplementation(APIResponseMock.mockIGDBSearchEndpoint(responseWithGamesOnly));
      
      const { games } = await import('./games.svelte');
      
      // This should work without errors
      await games.searchIGDB('test game');
      
      expect(games.value.igdbCandidates).toEqual(mockIGDBCandidates);
    });
  });

  describe('Config Integration', () => {
    it('should work with different API URL configurations', async () => {
      // Test with production-style URL
      const prodConfig = { ...mockConfig, apiUrl: 'https://production-api.com/api' };
      vi.doMock('$lib/env', () => ({ config: prodConfig }));
      
      vi.resetModules();
      const { games } = await import('./games.svelte');
      
      await games.loadGames();
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toContain('https://production-api.com/api/games');
    });

    it('should work with development-style URL', async () => {
      // Test with localhost URL
      const devConfig = { ...mockConfig, apiUrl: 'http://localhost:8000/api' };
      vi.doMock('$lib/env', () => ({ config: devConfig }));
      
      vi.resetModules();
      const { games } = await import('./games.svelte');
      
      await games.searchIGDB('test');
      
      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toBe('http://localhost:8000/api/games/search/igdb');
    });
  });
});