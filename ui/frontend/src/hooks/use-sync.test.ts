import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { createElement, type ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { server } from '@/test/mocks/server';
import { QueryWrapper, createTestQueryClient } from '@/test/test-utils';
import {
  useSyncConfigs,
  useSyncConfig,
  useSyncStatus,
  useUpdateSyncConfig,
  useTriggerSync,
  useConnectEpicGamesStore,
  useSyncStatusWithConnections,
  useGOGConnection,
  useRematchExternalGame,
  syncKeys,
} from './use-sync';
import { usePendingReviewCount } from './use-jobs';
import { SyncStorefront, SyncFrequency } from '@/types';
import * as syncApi from '@/api/sync';

const API_URL = '/api';

// Mock sync config data (API format - snake_case)
const mockSyncConfigApi = {
  id: '1',
  user_id: 'user-1',
  storefront: 'steam',
  frequency: 'daily',
  last_synced_at: null,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  is_configured: true,
};

const mockSyncStatusApi = {
  storefront: 'steam',
  is_syncing: false,
  last_synced_at: '2025-01-01T12:00:00Z',
  active_job_id: null,
};

describe('useSyncStatusWithConnections', () => {
  // The stores this hook refreshes today, named one by one so a store added
  // later is added here, with its connection key in the hook, on purpose.
  const connectionKeys: Partial<Record<SyncStorefront, readonly unknown[]>> = {
    [SyncStorefront.STEAM]: syncKeys.steamConnection(),
    [SyncStorefront.EPIC_GAMES_STORE]: syncKeys.epicGamesStoreConnection(),
    [SyncStorefront.GOG]: syncKeys.gogConnection(),
    [SyncStorefront.PLAYSTATION_STORE]: syncKeys.playstationStoreStatus(),
    [SyncStorefront.HUMBLE]: syncKeys.humbleStatus(),
    [SyncStorefront.AMAZON_GAMES]: syncKeys.amazonGamesConnection(),
  };
  const idle = { ...mockSyncStatusApi, external_game_count: 10 };
  const running = { ...idle, is_syncing: true, active_job_id: 'job-1' };

  // Serves whatever status is current, renders the hook, and counts the
  // connection-status invalidations per store.
  async function setup(platform: SyncStorefront, first: object) {
    let current = { ...first, storefront: platform };
    let requests = 0;
    server.use(
      http.get(`${API_URL}/sync/${platform}/status`, () => {
        requests += 1;
        return HttpResponse.json(current);
      }),
    );
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const spy = vi.spyOn(queryClient, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(() => useSyncStatusWithConnections(platform), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const invalidated = () => {
      const counts: Partial<Record<SyncStorefront, number>> = {};
      for (const [store, key] of Object.entries(connectionKeys)) {
        const n = spy.mock.calls.filter(
          ([filters]) => JSON.stringify(filters?.queryKey) === JSON.stringify(key),
        ).length;
        if (n > 0) counts[store as SyncStorefront] = n;
      }
      return counts;
    };
    // One poll: the next status arrives and the render it causes has run its effects.
    const poll = async (next: object) => {
      current = { ...next, storefront: platform };
      const served = requests;
      await act(async () => {
        await result.current.refetch();
      });
      expect(requests).toBe(served + 1);
      await waitFor(() => expect(result.current.isFetching).toBe(false));
      await act(async () => {});
    };
    return { invalidated, poll };
  }

  it('refetches no connection status on the first poll or on steady polls', async () => {
    const { invalidated, poll } = await setup(SyncStorefront.STEAM, idle);
    await poll(idle);
    await poll({ ...idle, external_game_count: 11 });
    expect(invalidated()).toEqual({});
  });

  it.each(Object.keys(connectionKeys) as SyncStorefront[])(
    'refetches only %s connection status once when its sync ends',
    async (platform) => {
      const { invalidated, poll } = await setup(platform, running);
      await poll(running);
      expect(invalidated()).toEqual({});
      await poll(idle);
      expect(invalidated()).toEqual({ [platform]: 1 });
      await poll(idle);
      expect(invalidated()).toEqual({ [platform]: 1 });
    },
  );

  it.each([
    ['a new last sync time', idle, { ...idle, last_synced_at: '2025-01-02T12:00:00Z' }],
    ['a job that started', idle, running],
    ['one job replaced by another', running, { ...running, active_job_id: 'job-2' }],
  ])('refetches once on %s', async (_name, before, after) => {
    const { invalidated, poll } = await setup(SyncStorefront.GOG, before);
    await poll(after);
    expect(invalidated()).toEqual({ [SyncStorefront.GOG]: 1 });
  });

  it('compares a store only with itself when the page changes store', async () => {
    server.use(
      http.get(`${API_URL}/sync/steam/status`, () =>
        HttpResponse.json({ ...running, storefront: 'steam' }),
      ),
      http.get(`${API_URL}/sync/gog/status`, () =>
        HttpResponse.json({ ...idle, storefront: 'gog' }),
      ),
    );
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const spy = vi.spyOn(queryClient, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result, rerender } = renderHook(
      ({ platform }: { platform: SyncStorefront }) => useSyncStatusWithConnections(platform),
      { wrapper, initialProps: { platform: SyncStorefront.STEAM } },
    );
    await waitFor(() => expect(result.current.data?.isSyncing).toBe(true));
    rerender({ platform: SyncStorefront.GOG });
    await waitFor(() => expect(result.current.data?.storefront).toBe(SyncStorefront.GOG));
    await waitFor(() => expect(result.current.data?.isSyncing).toBe(false));
    expect(spy).not.toHaveBeenCalled();
  });

  // End to end with the real connection query mounted, as the store page does:
  // a sync that flags the sign-in shows on the page, with one extra request.
  it('refetches the mounted connection once, picking up a flagged sign-in', async () => {
    let status: Record<string, unknown> = { ...running, storefront: 'gog' };
    let credentialsError = false;
    let connectionRequests = 0;
    server.use(
      http.get(`${API_URL}/sync/gog/status`, () => HttpResponse.json(status)),
      http.get(`${API_URL}/sync/gog/connection`, () => {
        connectionRequests += 1;
        return HttpResponse.json({
          connected: true,
          credentials_error: credentialsError,
          username: 'u',
        });
      }),
    );
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { result } = renderHook(
      () => ({
        status: useSyncStatusWithConnections(SyncStorefront.GOG),
        connection: useGOGConnection(),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.status.isSuccess).toBe(true));
    await waitFor(() => expect(result.current.connection.data?.credentialsError).toBe(false));
    expect(connectionRequests).toBe(1);

    const poll = async () => {
      await act(async () => {
        await result.current.status.refetch();
      });
      await waitFor(() => expect(result.current.status.isFetching).toBe(false));
      await act(async () => {});
    };
    await poll(); // steady
    expect(connectionRequests).toBe(1);

    credentialsError = true;
    status = { ...idle, storefront: 'gog' };
    await poll(); // the sync ended and flagged the sign-in
    await waitFor(() => expect(result.current.connection.data?.credentialsError).toBe(true));
    expect(connectionRequests).toBe(2);
  });

  // The documented limit: a sync that starts and fails between two polls
  // changes no field the status endpoint reports, so nothing is refetched.
  it('cannot see a sync that started and failed between two polls', async () => {
    const { invalidated, poll } = await setup(SyncStorefront.STEAM, idle);
    await poll({ ...idle });
    expect(invalidated()).toEqual({});
  });
});

describe('use-sync hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('useSyncConfigs', () => {
    it('fetches sync configs successfully', async () => {
      server.use(
        http.get(`${API_URL}/sync/config`, () => {
          return HttpResponse.json({
            configs: [mockSyncConfigApi],
            total: 1,
          });
        }),
      );

      const { result } = renderHook(() => useSyncConfigs(), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.configs).toHaveLength(1);
      expect(result.current.data?.configs[0].storefront).toBe(SyncStorefront.STEAM);
      expect(result.current.data?.configs[0].frequency).toBe(SyncFrequency.DAILY);
      expect(result.current.data?.total).toBe(1);
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/sync/config`, () => {
          return HttpResponse.json({ detail: 'Failed to fetch sync configs' }, { status: 500 });
        }),
      );

      const { result } = renderHook(() => useSyncConfigs(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch sync configs');
    });
  });

  describe('useSyncConfig', () => {
    it('fetches sync config for specific platform', async () => {
      server.use(
        http.get(`${API_URL}/sync/config/steam`, () => {
          return HttpResponse.json(mockSyncConfigApi);
        }),
      );

      const { result } = renderHook(() => useSyncConfig(SyncStorefront.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.storefront).toBe(SyncStorefront.STEAM);
      expect(result.current.data?.frequency).toBe(SyncFrequency.DAILY);
      expect(result.current.data?.isConfigured).toBe(true);
    });

    it('handles 404 error', async () => {
      server.use(
        http.get(`${API_URL}/sync/config/epic-games-store`, () => {
          return HttpResponse.json({ detail: 'Config not found' }, { status: 404 });
        }),
      );

      const { result } = renderHook(() => useSyncConfig(SyncStorefront.EPIC_GAMES_STORE), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Config not found');
    });
  });

  describe('useSyncStatus', () => {
    it('fetches sync status for specific platform', async () => {
      server.use(
        http.get(`${API_URL}/sync/steam/status`, () => {
          return HttpResponse.json(mockSyncStatusApi);
        }),
      );

      const { result } = renderHook(() => useSyncStatus(SyncStorefront.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.storefront).toBe(SyncStorefront.STEAM);
      expect(result.current.data?.isSyncing).toBe(false);
      expect(result.current.data?.lastSyncedAt).toBe('2025-01-01T12:00:00Z');
      expect(result.current.data?.activeJobId).toBeNull();
    });

    it('handles syncing state', async () => {
      server.use(
        http.get(`${API_URL}/sync/steam/status`, () => {
          return HttpResponse.json({
            ...mockSyncStatusApi,
            is_syncing: true,
            active_job_id: 'job-123',
          });
        }),
      );

      const { result } = renderHook(() => useSyncStatus(SyncStorefront.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.isSyncing).toBe(true);
      expect(result.current.data?.activeJobId).toBe('job-123');
    });
  });

  describe('useUpdateSyncConfig', () => {
    it('updates sync config successfully', async () => {
      const updatedConfig = {
        ...mockSyncConfigApi,
        frequency: 'weekly',
      };

      server.use(
        http.put(`${API_URL}/sync/config/steam`, async ({ request }) => {
          const body = (await request.json()) as {
            frequency?: string;
          };
          expect(body.frequency).toBe('weekly');
          return HttpResponse.json(updatedConfig);
        }),
      );

      const { result } = renderHook(() => useUpdateSyncConfig(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          storefront: SyncStorefront.STEAM,
          data: {
            frequency: SyncFrequency.WEEKLY,
          },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.frequency).toBe(SyncFrequency.WEEKLY);
    });

    it('handles update error', async () => {
      server.use(
        http.put(`${API_URL}/sync/config/steam`, () => {
          return HttpResponse.json({ detail: 'Update failed' }, { status: 400 });
        }),
      );

      const { result } = renderHook(() => useUpdateSyncConfig(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync({
            storefront: SyncStorefront.STEAM,
            data: { frequency: SyncFrequency.WEEKLY },
          });
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Update failed');
    });
  });

  describe('useTriggerSync', () => {
    it('triggers sync successfully', async () => {
      server.use(
        http.post(`${API_URL}/sync/steam`, () => {
          return HttpResponse.json({
            message: 'Sync started',
            job_id: 'job-123',
            storefront: 'steam',
            status: 'queued',
          });
        }),
      );

      const { result } = renderHook(() => useTriggerSync(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(SyncStorefront.STEAM);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.jobId).toBe('job-123');
      expect(result.current.data?.storefront).toBe('steam');
      expect(result.current.data?.status).toBe('queued');
      expect(result.current.data?.message).toBe('Sync started');
    });

    it('handles sync already in progress error', async () => {
      server.use(
        http.post(`${API_URL}/sync/steam`, () => {
          return HttpResponse.json({ detail: 'Sync already in progress' }, { status: 409 });
        }),
      );

      const { result } = renderHook(() => useTriggerSync(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync(SyncStorefront.STEAM);
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Sync already in progress');
    });
  });

  describe('Epic Auth Hooks', () => {
    it('should call connectEpicGamesStore with the auth code', async () => {
      const mockConnectEpicGamesStore = vi.spyOn(syncApi, 'connectEpicGamesStore');
      mockConnectEpicGamesStore.mockResolvedValue({
        displayName: 'EpicUser',
        accountId: 'acct-abc',
      });

      const { result } = renderHook(() => useConnectEpicGamesStore(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('TESTCODE');
      });

      expect(mockConnectEpicGamesStore).toHaveBeenCalledWith('TESTCODE');
      mockConnectEpicGamesStore.mockRestore();
    });
  });

  describe('useRematchExternalGame', () => {
    it('refreshes the review badge after a rematch', async () => {
      let count = 1;
      server.use(
        http.get(`${API_URL}/jobs/pending-review-count`, () =>
          HttpResponse.json({ pending_review_count: count, counts_by_source: { steam: count } }),
        ),
        http.post(`${API_URL}/sync/external-games/:id/rematch`, () => {
          count = 0;
          return new HttpResponse(null, { status: 204 });
        }),
      );
      const queryClient = createTestQueryClient();
      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children);

      const { result } = renderHook(
        () => ({ badge: usePendingReviewCount(), rematch: useRematchExternalGame() }),
        { wrapper },
      );
      await waitFor(() => expect(result.current.badge.data?.pendingReviewCount).toBe(1));

      await act(async () => {
        await result.current.rematch.mutateAsync({ id: 'eg-1', igdbId: 7 });
      });

      await waitFor(() => expect(result.current.badge.data?.pendingReviewCount).toBe(0));
    });
  });
});
