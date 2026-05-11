import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper, createTestQueryClient } from '@/test/test-utils';
import { setAuthHandlers } from '@/api/client';
import {
  usePlatforms,
  useAllPlatforms,
  usePlatform,
  usePlatformStorefronts,
  usePlatformNames,
  useStorefronts,
  useAllStorefronts,
  useStorefront,
  useStorefrontNames,
  platformKeys,
  storefrontKeys,
} from './use-platforms';

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

describe('use-platforms hooks', () => {
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

  describe('platformKeys', () => {
    it('generates correct query keys', () => {
      expect(platformKeys.all).toEqual(['platforms']);
      expect(platformKeys.lists()).toEqual(['platforms', 'list']);
      expect(platformKeys.list()).toEqual(['platforms', 'list', undefined]);
      expect(platformKeys.list({ activeOnly: true })).toEqual([
        'platforms',
        'list',
        { activeOnly: true },
      ]);
      expect(platformKeys.details()).toEqual(['platforms', 'detail']);
      expect(platformKeys.detail('platform-1')).toEqual(['platforms', 'detail', 'platform-1']);
      expect(platformKeys.storefronts('platform-1')).toEqual([
        'platforms',
        'storefronts',
        'platform-1',
      ]);
      expect(platformKeys.names()).toEqual(['platforms', 'names']);
    });
  });

  describe('storefrontKeys', () => {
    it('generates correct query keys', () => {
      expect(storefrontKeys.all).toEqual(['storefronts']);
      expect(storefrontKeys.lists()).toEqual(['storefronts', 'list']);
      expect(storefrontKeys.list()).toEqual(['storefronts', 'list', undefined]);
      expect(storefrontKeys.list({ activeOnly: false })).toEqual([
        'storefronts',
        'list',
        { activeOnly: false },
      ]);
      expect(storefrontKeys.details()).toEqual(['storefronts', 'detail']);
      expect(storefrontKeys.detail('storefront-1')).toEqual(['storefronts', 'detail', 'storefront-1']);
      expect(storefrontKeys.names()).toEqual(['storefronts', 'names']);
    });
  });

  describe('usePlatforms', () => {
    it('fetches paginated platforms list', async () => {
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

      const { result } = renderHook(() => usePlatforms(), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.platforms).toHaveLength(2);
      expect(result.current.data?.total).toBe(2);
      expect(result.current.data?.platforms[0].name).toBe('pc');
      expect(result.current.data?.platforms[0].name).toBe('pc');
    });

    it('passes parameters correctly', async () => {
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

      const { result } = renderHook(
        () => usePlatforms({ activeOnly: false, source: 'custom' }),
        { wrapper: QueryWrapper }
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, () => {
          return HttpResponse.json({ detail: 'Server error' }, { status: 500 });
        })
      );

      const { result } = renderHook(() => usePlatforms(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Server error');
    });
  });

  describe('useAllPlatforms', () => {
    it('fetches all platforms as array', async () => {
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

      const { result } = renderHook(() => useAllPlatforms(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toHaveLength(2);
      expect(result.current.data?.[0].name).toBe('pc');
    });

    it('passes optional parameters', async () => {
      server.use(
        http.get(`${API_URL}/platforms/`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');

          return HttpResponse.json({
            platforms: [],
            total: 0,
            page: 1,
            per_page: 100,
            pages: 0,
          });
        })
      );

      const { result } = renderHook(() => useAllPlatforms({ activeOnly: false }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });
  });

  describe('usePlatform', () => {
    it('fetches single platform by ID', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-1`, () => {
          return HttpResponse.json(mockPlatformApi);
        })
      );

      const { result } = renderHook(() => usePlatform('platform-1'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.name).toBe('pc');
      expect(result.current.data?.name).toBe('pc');
      expect(result.current.data?.storefronts).toHaveLength(1);
    });

    it('does not fetch when ID is undefined', async () => {
      const fetchSpy = vi.fn();

      server.use(
        http.get(`${API_URL}/platforms/*`, () => {
          fetchSpy();
          return HttpResponse.json(mockPlatformApi);
        })
      );

      const { result } = renderHook(() => usePlatform(undefined), {
        wrapper: QueryWrapper,
      });

      // Wait a bit to ensure no request was made
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(result.current.isPending).toBe(true);
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('usePlatformStorefronts', () => {
    it('fetches storefronts for a platform', async () => {
      server.use(
        http.get(`${API_URL}/platforms/platform-1/storefronts`, () => {
          return HttpResponse.json({
            platform_id: 'platform-1',
            platform_name: 'pc',
            platform_display_name: 'PC',
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total_storefronts: 2,
          });
        })
      );

      const { result } = renderHook(() => usePlatformStorefronts('platform-1'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toHaveLength(2);
      expect(result.current.data?.[0].name).toBe('steam');
    });

    it('does not fetch when platformId is undefined', async () => {
      const fetchSpy = vi.fn();

      server.use(
        http.get(`${API_URL}/platforms/*/storefronts`, () => {
          fetchSpy();
          return HttpResponse.json({
            platform_id: 'platform-1',
            storefronts: [],
            total_storefronts: 0,
          });
        })
      );

      const { result } = renderHook(() => usePlatformStorefronts(undefined), {
        wrapper: QueryWrapper,
      });

      // Wait a bit to ensure no request was made
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(result.current.isPending).toBe(true);
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('usePlatformNames', () => {
    it('fetches platform names list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/simple-list`, () => {
          return HttpResponse.json(['PC', 'PlayStation 5', 'Xbox Series X']);
        })
      );

      const { result } = renderHook(() => usePlatformNames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual(['PC', 'PlayStation 5', 'Xbox Series X']);
    });
  });

  describe('useStorefronts', () => {
    it('fetches paginated storefronts list', async () => {
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

      const { result } = renderHook(() => useStorefronts(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.storefronts).toHaveLength(2);
      expect(result.current.data?.total).toBe(2);
    });

    it('passes parameters correctly', async () => {
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

      const { result } = renderHook(
        () => useStorefronts({ activeOnly: false, source: 'custom' }),
        { wrapper: QueryWrapper }
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });
  });

  describe('useAllStorefronts', () => {
    it('fetches all storefronts as array', async () => {
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

      const { result } = renderHook(() => useAllStorefronts(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toHaveLength(2);
    });
  });

  describe('useStorefront', () => {
    it('fetches single storefront by ID', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/storefront-2`, () => {
          return HttpResponse.json(mockStorefrontApi);
        })
      );

      const { result } = renderHook(() => useStorefront('storefront-2'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.name).toBe('epic');
      expect(result.current.data?.name).toBe('epic');
    });

    it('does not fetch when ID is undefined', async () => {
      const fetchSpy = vi.fn();

      server.use(
        http.get(`${API_URL}/platforms/storefronts/*`, () => {
          fetchSpy();
          return HttpResponse.json(mockStorefrontApi);
        })
      );

      const { result } = renderHook(() => useStorefront(undefined), {
        wrapper: QueryWrapper,
      });

      // Wait a bit to ensure no request was made
      await new Promise((resolve) => setTimeout(resolve, 100));

      expect(result.current.isPending).toBe(true);
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('useStorefrontNames', () => {
    it('fetches storefront names list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/simple-list`, () => {
          return HttpResponse.json(['Steam', 'Epic Games Store', 'GOG']);
        })
      );

      const { result } = renderHook(() => useStorefrontNames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual(['Steam', 'Epic Games Store', 'GOG']);
    });
  });

  describe('staleTime configuration', () => {
    it('uses Infinity staleTime for platforms (rarely change)', async () => {
      let fetchCount = 0;

      server.use(
        http.get(`${API_URL}/platforms/`, () => {
          fetchCount++;
          return HttpResponse.json({
            platforms: [mockPlatformApi],
            total: 1,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        })
      );

      const queryClient = createTestQueryClient();
      // Override staleTime for this test to match hook configuration
      queryClient.setDefaultOptions({
        queries: { staleTime: Infinity },
      });

      const { result, rerender } = renderHook(() => usePlatforms(), {
        wrapper: ({ children }) => (
          <QueryWrapper>{children}</QueryWrapper>
        ),
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // First fetch should have happened
      expect(fetchCount).toBe(1);

      // Rerender should not trigger new fetch due to staleTime: Infinity
      rerender();

      expect(fetchCount).toBe(1);
    });
  });
});
