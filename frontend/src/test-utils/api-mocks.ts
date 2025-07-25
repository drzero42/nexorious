import { vi } from 'vitest';
import type { Game, GameListResponse, IGDBSearchResponse, IGDBGameCandidate } from '$lib/stores/games.svelte';

// Mock game data
export const mockGame: Game = {
  id: 'game-1',
  title: 'Test Game',
  description: 'A test game description',
  genre: 'Action',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2024-01-01',
  cover_art_url: 'https://example.com/cover.jpg',
  rating_average: 4.5,
  rating_count: 100,
  game_metadata: '{}',
  estimated_playtime_hours: 25,
  howlongtobeat_main: 18,  // Realistic main story completion time
  howlongtobeat_extra: 28,  // Realistic main + extras completion time
  howlongtobeat_completionist: 45,  // Realistic completionist time
  igdb_id: 'igdb-123',
  is_verified: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z'
};

export const mockGames: Game[] = [
  mockGame,
  {
    ...mockGame,
    id: 'game-2',
    title: 'Another Game',
    igdb_id: 'igdb-456'
  },
  {
    ...mockGame,
    id: 'game-3',
    title: 'Third Game',
    igdb_id: 'igdb-789'
  }
];

// Mock IGDB game candidates
export const mockIGDBCandidate: IGDBGameCandidate = {
  igdb_id: 'igdb-123',
  title: 'Test IGDB Game',
  release_date: '2024-01-01',
  cover_art_url: 'https://example.com/igdb-cover.jpg',
  description: 'A test game from IGDB',
  platforms: ['PC', 'PlayStation 5'],
  howlongtobeat_main: 12,  // Main story completion time
  howlongtobeat_extra: 20,  // Main + extras completion time
  howlongtobeat_completionist: 35  // Completionist time
};

export const mockIGDBCandidates: IGDBGameCandidate[] = [
  mockIGDBCandidate,
  {
    ...mockIGDBCandidate,
    igdb_id: 'igdb-888',
    title: 'Another IGDB Game',
    platforms: ['PC', 'Xbox Series X']
  }
];

// Mock API responses
export const mockGameListResponse: GameListResponse = {
  games: mockGames,
  total: mockGames.length,
  page: 1,
  per_page: 20,
  pages: 1
};

export const mockIGDBSearchResponse: IGDBSearchResponse = {
  games: mockIGDBCandidates,
  total: mockIGDBCandidates.length
};

// Mock error responses
export const mockErrorResponse = {
  detail: 'Test error message',
  status: 400
};

export const mockNetworkError = new Error('Network error');

// API Mock utilities
export class APIResponseMock {
  private static baseUrl = 'http://localhost:8000/api';
  
  // Set the base API URL for testing
  static setBaseUrl(url: string) {
    this.baseUrl = url;
  }

  static getBaseUrl() {
    return this.baseUrl;
  }

  // Create mock fetch responses
  static createResponse(data: any, status = 200) {
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: status === 200 ? 'OK' : 'Error',
      json: vi.fn().mockResolvedValue(data),
      text: vi.fn().mockResolvedValue(JSON.stringify(data)),
      headers: new Headers({
        'Content-Type': 'application/json'
      })
    } as unknown as Response;
  }

  // Games list endpoint mock
  static mockGamesListEndpoint(response = mockGameListResponse, status = 200) {
    return vi.fn().mockImplementation((url: string) => {
      if (url.includes('/games?') || url.endsWith('/games')) {
        return Promise.resolve(this.createResponse(response, status));
      }
      throw new Error(`Unexpected URL: ${url}`);
    });
  }

  // Single game endpoint mock
  static mockGameEndpoint(gameId: string, response = mockGame, status = 200) {
    return vi.fn().mockImplementation((url: string) => {
      if (url.includes(`/games/${gameId}`)) {
        return Promise.resolve(this.createResponse(response, status));
      }
      throw new Error(`Unexpected URL: ${url}`);
    });
  }

  // IGDB search endpoint mock
  static mockIGDBSearchEndpoint(response = mockIGDBSearchResponse, status = 200) {
    return vi.fn().mockImplementation((url: string, options?: RequestInit) => {
      if (url.includes('/games/search/igdb')) {
        // Verify the request body contains 'query' parameter
        if (options?.body) {
          const body = JSON.parse(options.body as string);
          if (!body.query) {
            throw new Error('Expected query parameter in IGDB search request');
          }
        }
        return Promise.resolve(this.createResponse(response, status));
      }
      throw new Error(`Unexpected URL: ${url}`);
    });
  }

  // IGDB import endpoint mock
  static mockIGDBImportEndpoint(response = mockGame, status = 201) {
    return vi.fn().mockImplementation((url: string) => {
      if (url.includes('/games/igdb-import')) {
        return Promise.resolve(this.createResponse(response, status));
      }
      throw new Error(`Unexpected URL: ${url}`);
    });
  }

  // Generic game CRUD operations mock
  static mockGameCRUDEndpoints() {
    return vi.fn().mockImplementation((url: string, options?: RequestInit) => {
      const method = options?.method || 'GET';
      
      // Games list
      if ((url.includes('/games?') || url.endsWith('/games')) && method === 'GET') {
        return Promise.resolve(this.createResponse(mockGameListResponse));
      }
      
      // Single game fetch
      if (/\/games\/[^\/]+$/.test(url) && method === 'GET') {
        return Promise.resolve(this.createResponse(mockGame));
      }
      
      // Create game
      if (url.endsWith('/games') && method === 'POST') {
        return Promise.resolve(this.createResponse({...mockGame, id: 'new-game-id'}, 201));
      }
      
      // Update game
      if (/\/games\/[^\/]+$/.test(url) && method === 'PUT') {
        return Promise.resolve(this.createResponse(mockGame));
      }
      
      // Delete game
      if (/\/games\/[^\/]+$/.test(url) && method === 'DELETE') {
        return Promise.resolve(this.createResponse(null, 204));
      }
      
      // IGDB search
      if (url.includes('/games/search/igdb') && method === 'POST') {
        return Promise.resolve(this.createResponse(mockIGDBSearchResponse));
      }
      
      // IGDB import
      if (url.includes('/games/igdb-import') && method === 'POST') {
        return Promise.resolve(this.createResponse(mockGame, 201));
      }
      
      // Metadata refresh
      if (url.includes('/metadata/refresh') && method === 'POST') {
        return Promise.resolve(this.createResponse(mockGame));
      }
      
      // User games endpoints
      if (url.includes('/user-games') && method === 'POST') {
        return Promise.resolve(this.createResponse({
          ...mockGame,
          id: 'user-game-1',
          game_id: 'game-1'
        }, 201));
      }
      
      if (url.includes('/user-games') && method === 'GET') {
        return Promise.resolve(this.createResponse({ games: [mockGame] }));
      }
      
      if (/\/user-games\/[^\/]+$/.test(url) && method === 'PUT') {
        return Promise.resolve(this.createResponse(mockGame));
      }
      
      if (/\/user-games\/[^\/]+$/.test(url) && method === 'DELETE') {
        return Promise.resolve(this.createResponse(null, 204));
      }
      
      throw new Error(`Unexpected API call: ${method} ${url}`);
    });
  }

  // Error response mocks
  static mockErrorResponse(status = 400, message = 'Test error') {
    return vi.fn().mockRejectedValue({
      status,
      message,
      json: vi.fn().mockResolvedValue({ detail: message })
    });
  }

  static mockNetworkError() {
    return vi.fn().mockRejectedValue(mockNetworkError);
  }
}

// Global fetch mock setup
export function setupFetchMock() {
  const mockFetch = APIResponseMock.mockGameCRUDEndpoints();
  global.fetch = mockFetch;
  return mockFetch;
}

// Reset fetch mock
export function resetFetchMock() {
  if (global.fetch && 'mockClear' in global.fetch) {
    (global.fetch as any).mockClear();
  }
}

// Verify API URL usage in fetch calls
export function verifyAPIUrlUsage(fetchMock: any, expectedBaseUrl: string) {
  const calls = fetchMock.mock.calls;
  calls.forEach((call: any) => {
    const url = call[0] as string;
    if (!url.startsWith(expectedBaseUrl)) {
      throw new Error(`Expected API call to start with ${expectedBaseUrl}, but got: ${url}`);
    }
  });
}

// Mock config for testing
export const mockConfig = {
  apiUrl: 'http://test-api:8000/api',
  appName: 'Test Nexorious',
  appVersion: '1.0.0-test',
  environment: 'test' as const,
  isDevelopment: true,
  isProduction: false
};