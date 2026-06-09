import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
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
  name: 'epic-games-store',
  display_name: 'Epic Games Store',
  icon_url: 'https://example.com/epic.png',
  base_url: 'https://store.epicgames.com',
  is_active: true,
  source: 'official',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
};

describe('use-platforms hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('usePlatforms', () => {
    it('fetches paginated platforms list', async () => {
      server.use(
        http.get(`${API_URL}/platforms`, () => {
          return HttpResponse.json({
            platforms: [mockPlatformApi, mockPlatform2Api],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        }),
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
        http.get(`${API_URL}/platforms`, ({ request }) => {
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
        }),
      );

      const { result } = renderHook(() => usePlatforms({ activeOnly: false, source: 'custom' }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/platforms`, () => {
          return HttpResponse.json({ detail: 'Server error' }, { status: 500 });
        }),
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
        http.get(`${API_URL}/platforms`, () => {
          return HttpResponse.json({
            platforms: [mockPlatformApi, mockPlatform2Api],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        }),
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
        http.get(`${API_URL}/platforms`, ({ request }) => {
          const url = new URL(request.url);
          expect(url.searchParams.get('active_only')).toBe('false');

          return HttpResponse.json({
            platforms: [],
            total: 0,
            page: 1,
            per_page: 100,
            pages: 0,
          });
        }),
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
        }),
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
        }),
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
  });

  describe('usePlatformNames', () => {
    it('fetches platform names list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/simple-list`, () => {
          return HttpResponse.json(['PC', 'PlayStation 5', 'Xbox Series X']);
        }),
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
        http.get(`${API_URL}/platforms/storefronts`, () => {
          return HttpResponse.json({
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        }),
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
        http.get(`${API_URL}/platforms/storefronts`, ({ request }) => {
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
        }),
      );

      const { result } = renderHook(() => useStorefronts({ activeOnly: false, source: 'custom' }), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });
  });

  describe('useAllStorefronts', () => {
    it('fetches all storefronts as array', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts`, () => {
          return HttpResponse.json({
            storefronts: [mockPlatformApi.storefronts[0], mockStorefrontApi],
            total: 2,
            page: 1,
            per_page: 100,
            pages: 1,
          });
        }),
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
        }),
      );

      const { result } = renderHook(() => useStorefront('storefront-2'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.name).toBe('epic-games-store');
      expect(result.current.data?.name).toBe('epic-games-store');
    });
  });

  describe('disabled query guard for undefined ID', () => {
    // Each of these hooks passes an `enabled: !!id` guard to useQuery, so an
    // undefined id must leave the query disabled (pending + idle) and never fire
    // the request — verified via fetchStatus, no real-time sleep required.
    const cases: Array<[string, () => { isPending: boolean; fetchStatus: string }, string]> = [
      ['usePlatform', () => usePlatform(undefined), `${API_URL}/platforms/*`],
      [
        'usePlatformStorefronts',
        () => usePlatformStorefronts(undefined),
        `${API_URL}/platforms/*/storefronts`,
      ],
      ['useStorefront', () => useStorefront(undefined), `${API_URL}/platforms/storefronts/*`],
    ];
    it.each(cases)('%s does not fetch when the ID is undefined', (_name, hook, endpoint) => {
      const fetchSpy = vi.fn();
      server.use(
        http.get(endpoint, () => {
          fetchSpy();
          return HttpResponse.json(mockStorefrontApi);
        }),
      );

      const { result } = renderHook(hook, { wrapper: QueryWrapper });

      expect(result.current.isPending).toBe(true);
      expect(result.current.fetchStatus).toBe('idle');
      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe('useStorefrontNames', () => {
    it('fetches storefront names list', async () => {
      server.use(
        http.get(`${API_URL}/platforms/storefronts/simple-list`, () => {
          return HttpResponse.json(['Steam', 'Epic Games Store', 'GOG']);
        }),
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
});
