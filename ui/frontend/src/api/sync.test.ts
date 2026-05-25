import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as syncApi from './sync';
import {
  connectEpic,
  getEpicConnection,
  disconnectEpic,
} from './sync';
import { api } from './client';
import { SyncStorefront, SyncFrequency } from '@/types';

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
            storefront: 'steam',
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
      expect(result.configs[0].storefront).toBe(SyncStorefront.STEAM);
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
        storefront: 'steam',
        frequency: 'daily',
        auto_add: true,
        last_synced_at: '2025-01-01T00:00:00Z',
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        is_configured: true,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncConfig(SyncStorefront.STEAM);

      expect(api.get).toHaveBeenCalledWith('/sync/config/steam');
      expect(result.storefront).toBe(SyncStorefront.STEAM);
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
        storefront: 'steam',
        frequency: 'weekly',
        auto_add: false,
        last_synced_at: null,
        created_at: '2025-01-01T00:00:00Z',
        updated_at: '2025-01-01T00:00:00Z',
        is_configured: true,
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.updateSyncConfig(SyncStorefront.STEAM, {
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
        storefront: 'steam',
        status: 'queued',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.triggerSync(SyncStorefront.STEAM);

      expect(api.post).toHaveBeenCalledWith('/sync/steam');
      expect(result.jobId).toBe('job-123');
      expect(result.storefront).toBe('steam');
    });
  });

  describe('getSyncStatus', () => {
    it('should fetch and transform sync status', async () => {
      const mockResponse = {
        storefront: 'steam',
        is_syncing: true,
        last_synced_at: '2025-01-01T00:00:00Z',
        active_job_id: 'job-123',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await syncApi.getSyncStatus(SyncStorefront.STEAM);

      expect(api.get).toHaveBeenCalledWith('/sync/steam/status');
      expect(result.isSyncing).toBe(true);
      expect(result.activeJobId).toBe('job-123');
    });
  });

  describe('Epic Auth API', () => {
    it('should connect Epic with auth code and return account info', async () => {
      const mockResponse = {
        display_name: 'EpicUser123',
        account_id: 'acct-abc',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await connectEpic('TESTCODE123');

      expect(api.post).toHaveBeenCalledWith('/sync/epic/connect', {
        auth_code: 'TESTCODE123',
      });
      expect(result).toEqual({
        displayName: 'EpicUser123',
        accountId: 'acct-abc',
      });
    });

    it('should get Epic connection status when connected', async () => {
      const mockResponse = {
        connected: true,
        disabled: false,
        display_name: 'EpicUser123',
        account_id: 'acct-abc',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getEpicConnection();

      expect(api.get).toHaveBeenCalledWith('/sync/epic/connection');
      expect(result).toEqual({
        connected: true,
        disabled: false,
        credentialsError: false,
        displayName: 'EpicUser123',
        accountId: 'acct-abc',
        reason: undefined,
      });
    });

    it('should get Epic connection status when disabled', async () => {
      const mockResponse = {
        connected: false,
        disabled: true,
        reason: 'legendary_not_configured',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getEpicConnection();

      expect(result).toEqual({
        connected: false,
        disabled: true,
        credentialsError: false,
        displayName: undefined,
        accountId: undefined,
        reason: 'legendary_not_configured',
      });
    });

    it('should disconnect Epic via DELETE /epic/connection', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await disconnectEpic();

      expect(api.delete).toHaveBeenCalledWith('/sync/epic/connection');
    });
  });
});
