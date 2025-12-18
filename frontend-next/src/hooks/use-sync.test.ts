import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { setAuthHandlers } from '@/api/client';
import {
  useSyncConfigs,
  useSyncConfig,
  useSyncStatus,
  useIgnoredGames,
  useUpdateSyncConfig,
  useTriggerSync,
  useUnignoreGame,
  syncKeys,
} from './use-sync';
import { SyncPlatform, SyncFrequency } from '@/types';

const API_URL = '/api';

// Mock sync config data (API format - snake_case)
const mockSyncConfigApi = {
  id: '1',
  user_id: 'user-1',
  platform: 'steam',
  frequency: 'daily',
  auto_add: true,
  enabled: true,
  last_synced_at: null,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
};

const mockSyncStatusApi = {
  platform: 'steam',
  is_syncing: false,
  last_synced_at: '2025-01-01T12:00:00Z',
  active_job_id: null,
};

const mockIgnoredGameApi = {
  id: 'ignored-1',
  source: 'steam',
  external_id: '12345',
  title: 'Ignored Game',
  created_at: '2025-01-01T00:00:00Z',
};

describe('use-sync hooks', () => {
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

  describe('syncKeys', () => {
    it('generates correct query keys for all', () => {
      expect(syncKeys.all).toEqual(['sync']);
    });

    it('generates correct query keys for configs', () => {
      expect(syncKeys.configs()).toEqual(['sync', 'configs']);
    });

    it('generates correct query keys for config with platform', () => {
      expect(syncKeys.config(SyncPlatform.STEAM)).toEqual(['sync', 'configs', 'steam']);
      expect(syncKeys.config(SyncPlatform.GOG)).toEqual(['sync', 'configs', 'gog']);
    });

    it('generates correct query keys for statuses', () => {
      expect(syncKeys.statuses()).toEqual(['sync', 'statuses']);
    });

    it('generates correct query keys for status with platform', () => {
      expect(syncKeys.status(SyncPlatform.STEAM)).toEqual(['sync', 'statuses', 'steam']);
      expect(syncKeys.status(SyncPlatform.GOG)).toEqual(['sync', 'statuses', 'gog']);
    });

    it('generates correct query keys for ignoredGames', () => {
      expect(syncKeys.ignoredGames()).toEqual(['sync', 'ignored', undefined]);
      expect(syncKeys.ignoredGames({ source: 'steam' })).toEqual([
        'sync',
        'ignored',
        { source: 'steam' },
      ]);
      expect(syncKeys.ignoredGames({ limit: 10, offset: 0 })).toEqual([
        'sync',
        'ignored',
        { limit: 10, offset: 0 },
      ]);
    });
  });

  describe('useSyncConfigs', () => {
    it('fetches sync configs successfully', async () => {
      server.use(
        http.get(`${API_URL}/sync/config`, () => {
          return HttpResponse.json({
            configs: [mockSyncConfigApi],
            total: 1,
          });
        })
      );

      const { result } = renderHook(() => useSyncConfigs(), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.configs).toHaveLength(1);
      expect(result.current.data?.configs[0].platform).toBe(SyncPlatform.STEAM);
      expect(result.current.data?.configs[0].frequency).toBe(SyncFrequency.DAILY);
      expect(result.current.data?.configs[0].autoAdd).toBe(true);
      expect(result.current.data?.total).toBe(1);
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/sync/config`, () => {
          return HttpResponse.json(
            { detail: 'Failed to fetch sync configs' },
            { status: 500 }
          );
        })
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
        })
      );

      const { result } = renderHook(() => useSyncConfig(SyncPlatform.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.platform).toBe(SyncPlatform.STEAM);
      expect(result.current.data?.frequency).toBe(SyncFrequency.DAILY);
      expect(result.current.data?.enabled).toBe(true);
    });

    it('handles 404 error', async () => {
      server.use(
        http.get(`${API_URL}/sync/config/epic`, () => {
          return HttpResponse.json({ detail: 'Config not found' }, { status: 404 });
        })
      );

      const { result } = renderHook(() => useSyncConfig(SyncPlatform.EPIC), {
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
        })
      );

      const { result } = renderHook(() => useSyncStatus(SyncPlatform.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.platform).toBe(SyncPlatform.STEAM);
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
        })
      );

      const { result } = renderHook(() => useSyncStatus(SyncPlatform.STEAM), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.isSyncing).toBe(true);
      expect(result.current.data?.activeJobId).toBe('job-123');
    });
  });

  describe('useIgnoredGames', () => {
    it('fetches ignored games list successfully', async () => {
      server.use(
        http.get(`${API_URL}/sync/ignored`, () => {
          return HttpResponse.json({
            items: [mockIgnoredGameApi],
            total: 1,
          });
        })
      );

      const { result } = renderHook(() => useIgnoredGames(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.items).toHaveLength(1);
      expect(result.current.data?.items[0].title).toBe('Ignored Game');
      expect(result.current.data?.items[0].source).toBe('steam');
      expect(result.current.data?.total).toBe(1);
    });

    it('passes filter parameters correctly', async () => {
      let capturedParams: URLSearchParams | null = null;

      server.use(
        http.get(`${API_URL}/sync/ignored`, ({ request }) => {
          const url = new URL(request.url);
          capturedParams = url.searchParams;

          return HttpResponse.json({
            items: [],
            total: 0,
          });
        })
      );

      const { result } = renderHook(
        () =>
          useIgnoredGames({
            source: 'steam',
            limit: 10,
            offset: 20,
          }),
        { wrapper: QueryWrapper }
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(capturedParams).not.toBeNull();
      expect(capturedParams!.get('source')).toBe('steam');
      expect(capturedParams!.get('limit')).toBe('10');
      expect(capturedParams!.get('offset')).toBe('20');
    });
  });

  describe('useUpdateSyncConfig', () => {
    it('updates sync config successfully', async () => {
      const updatedConfig = {
        ...mockSyncConfigApi,
        frequency: 'weekly',
        auto_add: false,
      };

      server.use(
        http.put(`${API_URL}/sync/config/steam`, async ({ request }) => {
          const body = (await request.json()) as {
            frequency?: string;
            auto_add?: boolean;
            enabled?: boolean;
          };
          expect(body.frequency).toBe('weekly');
          expect(body.auto_add).toBe(false);
          return HttpResponse.json(updatedConfig);
        })
      );

      const { result } = renderHook(() => useUpdateSyncConfig(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync({
          platform: SyncPlatform.STEAM,
          data: {
            frequency: SyncFrequency.WEEKLY,
            autoAdd: false,
          },
        });
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.frequency).toBe(SyncFrequency.WEEKLY);
      expect(result.current.data?.autoAdd).toBe(false);
    });

    it('handles update error', async () => {
      server.use(
        http.put(`${API_URL}/sync/config/steam`, () => {
          return HttpResponse.json({ detail: 'Update failed' }, { status: 400 });
        })
      );

      const { result } = renderHook(() => useUpdateSyncConfig(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync({
            platform: SyncPlatform.STEAM,
            data: { enabled: false },
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
            platform: 'steam',
            status: 'queued',
          });
        })
      );

      const { result } = renderHook(() => useTriggerSync(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(SyncPlatform.STEAM);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.jobId).toBe('job-123');
      expect(result.current.data?.platform).toBe('steam');
      expect(result.current.data?.status).toBe('queued');
      expect(result.current.data?.message).toBe('Sync started');
    });

    it('handles sync already in progress error', async () => {
      server.use(
        http.post(`${API_URL}/sync/steam`, () => {
          return HttpResponse.json(
            { detail: 'Sync already in progress' },
            { status: 409 }
          );
        })
      );

      const { result } = renderHook(() => useTriggerSync(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync(SyncPlatform.STEAM);
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

  describe('useUnignoreGame', () => {
    it('removes game from ignored list successfully', async () => {
      server.use(
        http.delete(`${API_URL}/sync/ignored/ignored-1`, () => {
          return new HttpResponse(null, { status: 204 });
        })
      );

      const { result } = renderHook(() => useUnignoreGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('ignored-1');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });
    });

    it('handles unignore error', async () => {
      server.use(
        http.delete(`${API_URL}/sync/ignored/non-existent`, () => {
          return HttpResponse.json({ detail: 'Ignored game not found' }, { status: 404 });
        })
      );

      const { result } = renderHook(() => useUnignoreGame(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('non-existent');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Ignored game not found');
    });
  });
});
