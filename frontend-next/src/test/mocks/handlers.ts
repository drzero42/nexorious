import { http, HttpResponse } from "msw";
import type { User, LoginResponse, SetupStatusResponse } from "@/types";

// Match the API URL format from env.ts (development mode)
const API_URL = "http://localhost:8000/api";

// Mock data
export const mockUser: User = {
  id: "test-user-id",
  username: "testuser",
  isAdmin: false,
};

export const mockAdminUser: User = {
  id: "admin-user-id",
  username: "admin",
  isAdmin: true,
};

export const mockTokens = {
  access_token: "mock-access-token",
  refresh_token: "mock-refresh-token",
  token_type: "bearer",
  expires_in: 3600,
};

export const mockPlatforms = [
  {
    id: 'platform-1',
    name: 'pc',
    display_name: 'PC',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront_id: 'storefront-1',
    storefronts: [
      {
        id: 'storefront-1',
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
    id: 'platform-2',
    name: 'playstation-5',
    display_name: 'PlayStation 5',
    icon_url: null,
    is_active: true,
    source: 'system',
    default_storefront_id: null,
    storefronts: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

export const mockTags = [
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
    const username = body.get("username");
    const password = body.get("password");

    if (username === "testuser" && password === "password123") {
      return HttpResponse.json<LoginResponse>(mockTokens);
    }

    if (username === "admin" && password === "admin123") {
      return HttpResponse.json<LoginResponse>(mockTokens);
    }

    return HttpResponse.json(
      { detail: "Invalid credentials" },
      { status: 401 }
    );
  }),

  http.get(`${API_URL}/auth/me`, ({ request }) => {
    const authHeader = request.headers.get("Authorization");

    if (!authHeader || !authHeader.startsWith("Bearer ")) {
      return HttpResponse.json({ detail: "Not authenticated" }, { status: 401 });
    }

    // Return different users based on token for testing
    const token = authHeader.replace("Bearer ", "");
    if (token === "admin-token") {
      return HttpResponse.json(mockAdminUser);
    }

    return HttpResponse.json(mockUser);
  }),

  http.post(`${API_URL}/auth/refresh`, async ({ request }) => {
    const body = (await request.json()) as { refresh_token: string };

    if (body.refresh_token === "mock-refresh-token") {
      return HttpResponse.json<LoginResponse>({
        ...mockTokens,
        access_token: "new-mock-access-token",
      });
    }

    return HttpResponse.json({ detail: "Invalid refresh token" }, { status: 401 });
  }),

  http.get(`${API_URL}/auth/setup/status`, () => {
    return HttpResponse.json<SetupStatusResponse>({ needs_setup: false });
  }),

  http.post(`${API_URL}/auth/setup/admin`, async ({ request }) => {
    const body = (await request.json()) as { username: string; password: string };

    if (body.username && body.password) {
      return HttpResponse.json(mockAdminUser);
    }

    return HttpResponse.json(
      { detail: "Invalid setup data" },
      { status: 400 }
    );
  }),

  // Platform endpoints
  http.get(`${API_URL}/platforms/`, () => {
    return HttpResponse.json({
      platforms: mockPlatforms,
      total: mockPlatforms.length,
      page: 1,
      per_page: 100,
      pages: 1,
    });
  }),

  // Add platform to user game
  http.post(`${API_URL}/user-games/:userGameId/platforms`, async ({ params, request }) => {
    const body = (await request.json()) as {
      platform_id: string;
      storefront_id?: string;
    };

    const platform = mockPlatforms.find((p) => p.id === body.platform_id);

    return HttpResponse.json(
      {
        id: `ugp-${Date.now()}`,
        platform_id: body.platform_id,
        storefront_id: body.storefront_id ?? null,
        platform: platform ?? null,
        storefront: null,
        store_game_id: null,
        store_url: null,
        is_available: true,
        original_platform_name: null,
        created_at: new Date().toISOString(),
      },
      { status: 201 }
    );
  }),

  // Update platform association
  http.put(
    `${API_URL}/user-games/:userGameId/platforms/:associationId`,
    async ({ params, request }) => {
      const body = (await request.json()) as {
        platform_id: string;
        storefront_id?: string;
      };

      const platform = mockPlatforms.find((p) => p.id === body.platform_id);

      return HttpResponse.json({
        id: params.associationId,
        platform_id: body.platform_id,
        storefront_id: body.storefront_id ?? null,
        platform: platform ?? null,
        storefront: null,
        store_game_id: null,
        store_url: null,
        is_available: true,
        original_platform_name: null,
        created_at: new Date().toISOString(),
      });
    }
  ),

  // Remove platform from user game
  http.delete(`${API_URL}/user-games/:userGameId/platforms/:associationId`, () => {
    return HttpResponse.json({ message: 'Platform removed successfully' });
  }),

  // Game endpoints
  http.get(`${API_URL}/user-games/`, () => {
    return HttpResponse.json([]);
  }),

  http.get(`${API_URL}/user-games/:id`, ({ params }) => {
    const { id } = params;
    return HttpResponse.json({
      id,
      title: "Test Game",
      igdb_id: 12345,
      status: "backlog",
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

  // Assign tags to game
  http.post(`${API_URL}/tags/assign/:userGameId`, async ({ request }) => {
    const body = (await request.json()) as { tag_ids: string[] };
    return HttpResponse.json({
      message: 'Tags assigned successfully',
      new_associations: body.tag_ids.length,
      total_requested: body.tag_ids.length,
    });
  }),

  // Remove tags from game
  http.delete(`${API_URL}/tags/remove/:userGameId`, async ({ request }) => {
    const body = (await request.json()) as { tag_ids: string[] };
    return HttpResponse.json({
      message: 'Tags removed successfully',
      removed_associations: body.tag_ids.length,
      total_requested: body.tag_ids.length,
    });
  }),

  // Create or get tag
  http.post(`${API_URL}/tags/create-or-get`, ({ request }) => {
    const url = new URL(request.url);
    const name = url.searchParams.get('name') ?? 'New Tag';
    const color = url.searchParams.get('color') ?? '#808080';

    return HttpResponse.json({
      tag: {
        id: `tag-${Date.now()}`,
        user_id: 'test-user-id',
        name,
        color,
        description: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        game_count: 0,
      },
      created: true,
    });
  }),

  // IGDB endpoints
  http.post(`${API_URL}/games/search/igdb`, async ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get("query");

    if (!query) {
      return HttpResponse.json({ detail: "Query required" }, { status: 400 });
    }

    return HttpResponse.json([
      {
        id: 1,
        name: `${query} Game`,
        cover_url: "https://example.com/cover.jpg",
        release_date: "2024-01-01",
        platforms: ["PC", "PS5"],
      },
    ]);
  }),
];
