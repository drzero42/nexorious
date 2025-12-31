// frontend-next/src/api/sync.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as syncApi from './sync';
import {
  getSyncConfigs,
  getSyncConfig,
  updateSyncConfig,
  triggerSync,
  getSyncStatus,
  getIgnoredGames,
  unignoreGame,
  verifySteamCredentials,
  disconnectSteam,
  startEpicAuth,
  completeEpicAuth,
  checkEpicAuth,
  disconnectEpic,
} from './sync';
import { api } from './client';
import { SyncPlatform, SyncFrequency } from '@/types';

vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('syncApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getSyncConfigs', () => {
    it('should fetch and transform sync configs', async () => {
      const mockResponse = {
        configs: [
          {
            id: '1',
            user_id: 'user-1',
            platform: 'steam',
            frequency: 'daily',
            auto_add: true,
            last_synced_at: '2025-01-01T00:00:00Z',
            created_at: '2025-01-01T00:00:00Z',
            updated_at: '2025-01-01T00:00:00Z',
            is_configured: true,
          },
        ],
        total: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncConfigs();

      expect(api.get).toHaveBeenCalledWith('/sync/config');
      expect(result.configs[0].platform).toBe(SyncPlatform.STEAM);
      expect(result.configs[0].frequency).toBe(SyncFrequency.DAILY);
      expect(result.configs[0].autoAdd).toBe(true);
      expect(result.configs[0].userId).toBe('user-1');
    });
  });

  describe('getSyncConfig', () => {
    it('should fetch and transform single sync config', async () => {
      const mockResponse = {
        id: '1',
        user_id: 'user-1',
        platform: 'steam',
        frequency: 'daily',
        auto_add: true,
        last_synced_at: '2025-01-01T00:00:00Z',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        is_configured: true,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncConfig(SyncPlatform.STEAM);

      expect(api.get).toHaveBeenCalledWith('/sync/config/steam');
      expect(result.platform).toBe(SyncPlatform.STEAM);
      expect(result.frequency).toBe(SyncFrequency.DAILY);
      expect(result.autoAdd).toBe(true);
      expect(result.userId).toBe('user-1');
    });
  });

  describe('updateSyncConfig', () => {
    it('should update sync config with correct snake_case params', async () => {
      const mockResponse = {
        id: '1',
        user_id: 'user-1',
        platform: 'steam',
        frequency: 'weekly',
        auto_add: false,
        last_synced_at: null,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        is_configured: true,
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.updateSyncConfig(SyncPlatform.STEAM, {
        frequency: SyncFrequency.WEEKLY,
        autoAdd: false,
      });

      expect(api.put).toHaveBeenCalledWith('/sync/config/steam', {
        frequency: 'weekly',
        auto_add: false,
      });
      expect(result.frequency).toBe(SyncFrequency.WEEKLY);
      expect(result.autoAdd).toBe(false);
    });
  });

  describe('triggerSync', () => {
    it('should trigger sync and return job info', async () => {
      const mockResponse = {
        message: 'Sync started',
        job_id: 'job-123',
        platform: 'steam',
        status: 'queued',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.triggerSync(SyncPlatform.STEAM);

      expect(api.post).toHaveBeenCalledWith('/sync/steam');
      expect(result.jobId).toBe('job-123');
      expect(result.platform).toBe('steam');
    });
  });

  describe('getSyncStatus', () => {
    it('should fetch and transform sync status', async () => {
      const mockResponse = {
        platform: 'steam',
        is_syncing: true,
        last_synced_at: '2025-01-01T00:00:00Z',
        active_job_id: 'job-123',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncStatus(SyncPlatform.STEAM);

      expect(api.get).toHaveBeenCalledWith('/sync/steam/status');
      expect(result.isSyncing).toBe(true);
      expect(result.activeJobId).toBe('job-123');
    });
  });

  describe('getIgnoredGames', () => {
    it('should fetch ignored games with filters', async () => {
      const mockResponse = {
        items: [
          {
            id: 'ignored-1',
            source: 'STEAM',
            external_id: '12345',
            title: 'Some Game',
            created_at: '2025-01-01T00:00:00Z',
          },
        ],
        total: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getIgnoredGames({ source: 'STEAM', limit: 10 });

      expect(api.get).toHaveBeenCalledWith('/sync/ignored', {
        params: { source: 'STEAM', limit: 10 },
      });
      expect(result.items[0].externalId).toBe('12345');
      expect(result.total).toBe(1);
    });
  });

  describe('unignoreGame', () => {
    it('should delete ignored game', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await syncApi.unignoreGame('ignored-1');

      expect(api.delete).toHaveBeenCalledWith('/sync/ignored/ignored-1');
    });
  });

  describe('Epic Auth API', () => {
    it('should start Epic auth and return URL', async () => {
      const mockResponse = {
        auth_url: 'https://www.epicgames.com/id/api/redirect',
        instructions: 'Please visit the URL and login',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await startEpicAuth();

      expect(api.post).toHaveBeenCalledWith('/sync/epic/auth/start');
      expect(result).toEqual({
        authUrl: 'https://www.epicgames.com/id/api/redirect',
        instructions: 'Please visit the URL and login',
      });
    });

    it('should complete Epic auth with valid code', async () => {
      const mockResponse = {
        valid: true,
        display_name: 'EpicUser123',
        error: null,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await completeEpicAuth('TESTCODE123');

      expect(api.post).toHaveBeenCalledWith('/sync/epic/auth/complete', {
        code: 'TESTCODE123',
      });
      expect(result).toEqual({
        valid: true,
        displayName: 'EpicUser123',
        error: null,
      });
    });

    it('should handle invalid auth code', async () => {
      const mockResponse = {
        valid: false,
        display_name: null,
        error: 'invalid_code',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await completeEpicAuth('BADCODE');

      expect(result).toEqual({
        valid: false,
        displayName: null,
        error: 'invalid_code',
      });
    });

    it('should check Epic auth status', async () => {
      const mockResponse = {
        is_authenticated: true,
        display_name: 'EpicUser123',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await checkEpicAuth();

      expect(api.get).toHaveBeenCalledWith('/sync/epic/auth/check');
      expect(result).toEqual({
        isAuthenticated: true,
        displayName: 'EpicUser123',
      });
    });

    it('should disconnect Epic', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await disconnectEpic();

      expect(api.delete).toHaveBeenCalledWith('/sync/epic/connection');
    });

    it('should transform snake_case to camelCase correctly', async () => {
      const mockResponse = {
        auth_url: 'https://example.com',
        instructions: 'test',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await startEpicAuth();

      // Verify transformed keys
      expect(result).toHaveProperty('authUrl');
      expect(result).toHaveProperty('instructions');
      expect(result).not.toHaveProperty('auth_url');
    });
  });
});
