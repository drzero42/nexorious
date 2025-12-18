import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { setAuthHandlers } from './client';
import {
  getPlatforms,
  getAllPlatforms,
  getPlatform,
  getPlatformStorefronts,
  getStorefronts,
  getAllStorefronts,
  getStorefront,
  getPlatformNames,
  getStorefrontNames,
} from './platforms';

const API_URL = '/api';

// Mock platform data
const mockPlatformApi = {
  id: 'platform-1',
  name: 'pc',
  display_name: 'PC',
  icon_url: 'https://example.com/pc.png',
  is_active: true,
  source: 'official',
  default_storefront_id: 'storefront-1',
  storefronts: [
    {
      id: 'storefront-1',
      name: 'steam',
      display_name: 'Steam',
      icon_url: 'https://example.com/steam.png',
      base_url: 'https://store.steampowered.com',
      is_active: true,
      source: 'official',
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
    },
  ],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockPlatform2Api = {
  id: 'platform-2',
  name: 'playstation-5',
  display_name: 'PlayStation 5',
  icon_url: null,
  is_active: true,
  source: 'official',
  default_storefront_id: null,
  storefronts: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

const mockStorefrontApi = {
  id: 'storefront-2',
  name: 'epic',
  display_name: 'Epic Games Store',
  icon_url: 'https://example.com/epic.png',
  base_url: 'https://store.epicgames.com',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

describe('platforms.ts', () => {
  let mockGetAccessToken: Mock<() => string | null>;
  let mockRefreshTokens: Mock<() => Promise<boolean>>;
  let mockLogout: Mock<() => void>;

  beforeEach(() => {
    vi.clearAllMocks();

    mockGetAccessToken = vi.fn<() => string | null>().mockReturnValue('test-access-token');
    mockRefreshTokens = vi.fn<() => Promise<boolean>>().mockResolvedValue(false);
    mockLogout = vi.fn<() => void>();

    setAuthHandlers(mockGetAccessToken, mockRefreshTokens, mockLogout);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('getPlatforms', () => {
    it('returns paginated platforms list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('true');
          expect(url.searchParams.get('page')).toBe('1');
          expect(url.searchParams.get('per_page')).toBe('100');

          return HttpResponse.json({
            platforms: [mockPlatformApi, mockPlatform2Api],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        })
      );

      const result = await getPlatforms();

      expect(result.platforms).toHaveLength(2);
      expect(result.total).toBe(2);
      expect(result.page).toBe(1);
      expect(result.perPage).toBe(100);
      expect(result.pages).toBe(1);

      // Verify platform transformation
      expect(result.platforms[0]).toEqual({
        id: 'platform-1',
        name: 'pc',
        display_name: 'PC',
        icon_url: 'https://example.com/pc.png',
        is_active: true,
        source: 'official',
        default_storefront_id: 'storefront-1',
        storefronts: [
          {
            id: 'storefront-1',
            name: 'steam',
            display_name: 'Steam',
            icon_url: 'https://example.com/steam.png',
            base_url: 'https://store.steampowered.com',
            is_active: true,
            source: 'official',
            created_at: '2024-01-01T00:00:00Z',
            updated_at: '2024-01-01T00:00:00Z',
          },
        ],
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z',
      });
    });

    it('passes custom parameters', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');
          expect(url.searchParams.get('source')).toBe('custom');
          expect(url.searchParams.get('page')).toBe('2');
          expect(url.searchParams.get('per_page')).toBe('50');

          return HttpResponse.json({
            platforms: [],
            total: 0,
            page: 2,
            per_page: 50,
            pages: 0,
          });
        })
      );

      const result = await getPlatforms({
        activeOnly: false,
        source: 'custom',
        page: 2,
        perPage: 50,
      });

      expect(result.platforms).toHaveLength(0);
      expect(result.page).toBe(2);
    });

    it('requires authentication', async () => {
      mockGetAccessToken.mockReturnValue(null);

      await expect(getPlatforms()).rejects.toMatchObject({
        message: 'Not authenticated',
        status: 401,
      });
    });
  });

  describe('getAllPlatforms', () => {
    it('returns all platforms array', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, () => {
          return HttpResponse.json({
            platforms: [mockPlatformApi, mockPlatform2Api],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        })
      );

      const result = await getAllPlatforms();

      expect(Array.isArray(result)).toBe(true);
      expect(result).toHaveLength(2);
      expect(result[0].id).toBe('platform-1');
      expect(result[1].id).toBe('platform-2');
    });

    it('passes optional parameters', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');
          expect(url.searchParams.get('source')).toBe('custom');

          return HttpResponse.json({
            platforms: [],
            total: 0,
            page: 1,
            per_page: 100,
            pages: 0,
          });
        })
      );

      await getAllPlatforms({ activeOnly: false, source: 'custom' });
    });
  });

  describe('getPlatform', () => {
    it('returns single platform by ID', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-1`, () => {
          return HttpResponse.json(mockPlatformApi);
        })
      );

      const result = await getPlatform('platform-1');

      expect(result.id).toBe('platform-1');
      expect(result.name).toBe('pc');
      expect(result.display_name).toBe('PC');
      expect(result.storefronts).toHaveLength(1);
    });

    it('throws error for non-existent platform', async () => {
      server.use(
        http.get(`${API_URL}/platforms/non-existent`, () => {
          return HttpResponse.json({ detail: 'Platform not found' }, { status: 404 });
        })
      );

      await expect(getPlatform('non-existent')).rejects.toMatchObject({
        message: 'Platform not found',
        status: 404,
      });
    });
  });

  describe('getPlatformStorefronts', () => {
    it('returns storefronts for a platform', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-1/storefronts`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('true');

          return HttpResponse.json({
            platform_id: 'platform-1',
            platform_name: 'pc',
            platform_display_name: 'PC',
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total_storefronts: 2,
          });
        })
      );

      const result = await getPlatformStorefronts('platform-1');

      expect(result).toHaveLength(2);
      expect(result[0].id).toBe('storefront-1');
      expect(result[1].id).toBe('storefront-2');
    });

    it('passes activeOnly parameter', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-1/storefronts`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');

          return HttpResponse.json({
            platform_id: 'platform-1',
            platform_name: 'pc',
            platform_display_name: 'PC',
            storefronts: [],
            total_storefronts: 0,
          });
        })
      );

      await getPlatformStorefronts('platform-1', false);
    });

    it('returns empty array for platform without storefronts', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-2/storefronts`, () => {
          return HttpResponse.json({
            platform_id: 'platform-2',
            platform_name: 'playstation-5',
            platform_display_name: 'PlayStation 5',
            storefronts: [],
            total_storefronts: 0,
          });
        })
      );

      const result = await getPlatformStorefronts('platform-2');

      expect(result).toHaveLength(0);
    });
  });

  describe('getStorefronts', () => {
    it('returns paginated storefronts list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('true');
          expect(url.searchParams.get('page')).toBe('1');
          expect(url.searchParams.get('per_page')).toBe('100');

          return HttpResponse.json({
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        })
      );

      const result = await getStorefronts();

      expect(result.storefronts).toHaveLength(2);
      expect(result.total).toBe(2);
      expect(result.perPage).toBe(100);
    });

    it('passes custom parameters', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');
          expect(url.searchParams.get('source')).toBe('custom');
          expect(url.searchParams.get('page')).toBe('3');
          expect(url.searchParams.get('per_page')).toBe('25');

          return HttpResponse.json({
            storefronts: [],
            total: 0,
            page: 3,
            per_page: 25,
            pages: 0,
          });
        })
      );

      await getStorefronts({
        activeOnly: false,
        source: 'custom',
        page: 3,
        perPage: 25,
      });
    });
  });

  describe('getAllStorefronts', () => {
    it('returns all storefronts array', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/`, () => {
          return HttpResponse.json({
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        })
      );

      const result = await getAllStorefronts();

      expect(Array.isArray(result)).toBe(true);
      expect(result).toHaveLength(2);
    });

    it('passes optional parameters', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');
          expect(url.searchParams.get('source')).toBe('custom');

          return HttpResponse.json({
            storefronts: [],
            total: 0,
            page: 1,
            per_page: 100,
            pages: 0,
          });
        })
      );

      await getAllStorefronts({ activeOnly: false, source: 'custom' });
    });
  });

  describe('getStorefront', () => {
    it('returns single storefront by ID', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/storefront-2`, () => {
          return HttpResponse.json(mockStorefrontApi);
        })
      );

      const result = await getStorefront('storefront-2');

      expect(result.id).toBe('storefront-2');
      expect(result.name).toBe('epic');
      expect(result.display_name).toBe('Epic Games Store');
      expect(result.base_url).toBe('https://store.epicgames.com');
    });

    it('throws error for non-existent storefront', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/non-existent`, () => {
          return HttpResponse.json({ detail: 'Storefront not found' }, { status: 404 });
        })
      );

      await expect(getStorefront('non-existent')).rejects.toMatchObject({
        message: 'Storefront not found',
        status: 404,
      });
    });
  });

  describe('getPlatformNames', () => {
    it('returns simple list of platform names', async () => {
      server.use(
        http.get(`${API_URL}/platforms/simple-list`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('true');

          return HttpResponse.json(['PC', 'PlayStation 5', 'Xbox Series X']);
        })
      );

      const result = await getPlatformNames();

      expect(result).toEqual(['PC', 'PlayStation 5', 'Xbox Series X']);
    });

    it('passes activeOnly parameter', async () => {
      server.use(
        http.get(`${API_URL}/platforms/simple-list`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');

          return HttpResponse.json(['PC', 'PlayStation 5', 'Inactive Platform']);
        })
      );

      const result = await getPlatformNames(false);

      expect(result).toHaveLength(3);
    });
  });

  describe('getStorefrontNames', () => {
    it('returns simple list of storefront names', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/simple-list`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('true');

          return HttpResponse.json(['Steam', 'Epic Games Store', 'GOG']);
        })
      );

      const result = await getStorefrontNames();

      expect(result).toEqual(['Steam', 'Epic Games Store', 'GOG']);
    });

    it('passes activeOnly parameter', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/simple-list`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');

          return HttpResponse.json(['Steam', 'Inactive Store']);
        })
      );

      const result = await getStorefrontNames(false);

      expect(result).toHaveLength(2);
    });
  });
});
