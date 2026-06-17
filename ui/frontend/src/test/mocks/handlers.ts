import { http, HttpResponse } from 'msw';
import type { User } from '@/types';

// Match the API URL format from env.ts
// In test mode, NODE_ENV is 'test' so apiUrl is '/api'
// MSW in Node interprets relative URLs against http://localhost
const API_URL = '/api';

// Mock data
const mockUser: User = {
  id: 'test-user-id',
  username: 'testuser',
  isAdmin: false,
};

const mockAdminUser: User = {
  id: 'admin-user-id',
  username: 'admin',
  isAdmin: true,
};

const mockPlatforms = [
  {
    name: 'pc',
    display_name: 'PC',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront: 'steam',
    storefronts: [
      {
        name: 'steam',
        display_name: 'Steam',
        icon_url: null,
        base_url: 'https://store.steampowered.com',
        is_active: true,
        source: 'system',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      },
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'playstation-5',
    display_name: 'PlayStation 5',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront: null,
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

const mockTags = [
  {
    id: 'tag-1',
    user_id: 'test-user-id',
    name: 'RPG',
    color: '#FF5733',
    description: 'Role-playing games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 5,
  },
  {
    id: 'tag-2',
    user_id: 'test-user-id',
    name: 'Action',
    color: '#33FF57',
    description: 'Action games',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    game_count: 3,
  },
];

// Default handlers
export const handlers = [
  // Auth endpoints
  http.post(`${API_URL}/auth/login`, async ({ request }) => {
    const body = (await request.formData()) as FormData;
    const username = body.get('username');
    const password = body.get('password');

    if (username === 'testuser' && password === 'password123') {
      return HttpResponse.json<User>(mockUser);
    }

    if (username === 'admin' && password === 'admin123') {
      return HttpResponse.json<User>(mockAdminUser);
    }

    return HttpResponse.json({ detail: 'Invalid credentials' }, { status: 401 });
  }),

  http.get(`${API_URL}/auth/me`, ({ request }) => {
    const authHeader = request.headers.get('Authorization');

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
    }

    // Return different users based on token for testing
    const token = authHeader.replace('Bearer ', '');
    if (token === 'admin-token') {
      return HttpResponse.json(mockAdminUser);
    }

    return HttpResponse.json(mockUser);
  }),

  http.post(`${API_URL}/auth/logout`, () => {
    return HttpResponse.json({ message: 'Logged out successfully' });
  }),

  // Platform endpoints
  http.get(`${API_URL}/platforms`, () => {
    return HttpResponse.json({
      platforms: mockPlatforms,
      total: mockPlatforms.length,
      page: 1,
      per_page: 100,
      pages: 1,
    });
  }),

  // Add platform to user game
  http.post(`${API_URL}/user-games/:userGameId/platforms`, async ({ request }) => {
    const body = (await request.json()) as {
      platform: string;
      storefront?: string;
    };

    const platform = mockPlatforms.find((p) => p.name === body.platform);

    return HttpResponse.json(
      {
        id: `ugp-${Date.now()}`,
        platform: body.platform,
        storefront: body.storefront ?? null,
        platform_details: platform ?? null,
        storefront_details: null,
        is_available: true,
        created_at: new Date().toISOString(),
      },
      { status: 201 },
    );
  }),

  // Update platform association
  http.put(
    `${API_URL}/user-games/:userGameId/platforms/:associationId`,
    async ({ params, request }) => {
      const body = (await request.json()) as {
        platform: string;
        storefront?: string;
      };

      const platform = mockPlatforms.find((p) => p.name === body.platform);

      return HttpResponse.json({
        id: params.associationId,
        platform: body.platform,
        storefront: body.storefront ?? null,
        platform_details: platform ?? null,
        storefront_details: null,
        is_available: true,
        created_at: new Date().toISOString(),
      });
    },
  ),

  // Remove platform from user game
  http.delete(`${API_URL}/user-games/:userGameId/platforms/:associationId`, () => {
    return HttpResponse.json({ message: 'Platform removed successfully' });
  }),

  // Game endpoints
  http.get(`${API_URL}/user-games/genres`, () => {
    return HttpResponse.json({ genres: ['Action', 'Adventure', 'RPG'] });
  }),

  http.get(`${API_URL}/user-games/stats`, () => {
    return HttpResponse.json({
      total_games: 0,
      completion_stats: {},
      ownership_stats: {},
      platform_stats: {},
      genre_stats: {},
      pile_of_shame: 0,
      completion_rate: 0,
      average_rating: null,
      total_hours_played: 0,
    });
  }),

  http.get(`${API_URL}/user-games`, () => {
    return HttpResponse.json([]);
  }),

  http.get(`${API_URL}/user-games/:id`, ({ params }) => {
    const { id } = params;
    return HttpResponse.json({
      id,
      title: 'Test Game',
      igdb_id: 12345,
      status: 'backlog',
    });
  }),

  // Tags endpoints
  http.get(`${API_URL}/tags/`, () => {
    return HttpResponse.json({
      tags: mockTags,
      total: mockTags.length,
      page: 1,
      per_page: 100,
      total_pages: 1,
    });
  }),

  // Jobs endpoints
  http.get(`${API_URL}/jobs/status/:job_type`, () => {
    return HttpResponse.json({
      is_active: false,
      active_job_id: null,
      last_completed_job_id: null,
      last_completed_at: null,
    });
  }),

  // IGDB endpoints
  http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get('query');

    if (!query) {
      return HttpResponse.json({ detail: 'Query required' }, { status: 400 });
    }

    return HttpResponse.json([
      {
        id: 1,
        name: `${query} Game`,
        cover_url: 'https://example.com/cover.jpg',
        release_date: '2024-01-01',
        platforms: ['PC', 'PS5'],
      },
    ]);
  }),
];
