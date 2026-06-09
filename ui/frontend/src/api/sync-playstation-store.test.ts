import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  configurePlaystationStore,
  getPlaystationStoreStatus,
  disconnectPlaystationStore,
} from './sync';
import { api } from './client';

vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    put: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('PSN API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('configurePlaystationStore', () => {
    it('should configure PSN with valid NPSSO token', async () => {
      const mockResponse = {
        success: true,
        online_id: 'TestPSNUser',
        account_id: 'psn-account-123',
        message: 'PSN configured successfully',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await configurePlaystationStore('valid-npsso-token');

      expect(api.put).toHaveBeenCalledWith('/sync/playstation-store/connection', {
        npsso_token: 'valid-npsso-token',
      });
      expect(result).toEqual({
        valid: true,
        accountId: 'psn-account-123',
        onlineId: 'TestPSNUser',
        error: null,
      });
    });

    it('should handle invalid NPSSO token', async () => {
      const mockResponse = {
        success: false,
        online_id: null,
        account_id: null,
        message: 'Invalid NPSSO token',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await configurePlaystationStore('invalid-token');

      expect(result).toEqual({
        valid: false,
        accountId: null,
        onlineId: null,
        error: 'Invalid NPSSO token',
      });
    });
  });

  describe('getPlaystationStoreStatus', () => {
    it('should fetch PSN connection status when configured', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestPSNUser',
        credentials_error: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPlaystationStoreStatus();

      expect(api.get).toHaveBeenCalledWith('/sync/playstation-store/connection');
      expect(result).toEqual({
        configured: true,
        onlineId: 'TestPSNUser',
        credentialsError: false,
      });
    });

    it('should fetch PSN status when not configured', async () => {
      const mockResponse = {
        is_configured: false,
        online_id: null,
        credentials_error: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPlaystationStoreStatus();

      expect(result).toEqual({
        configured: false,
        onlineId: null,
        credentialsError: false,
      });
    });

    it('should detect credentials error', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestPSNUser',
        credentials_error: true,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPlaystationStoreStatus();

      expect(result.credentialsError).toBe(true);
      expect(result.configured).toBe(true);
    });
  });

  describe('disconnectPlaystationStore', () => {
    it('should disconnect PSN', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await disconnectPlaystationStore();

      expect(api.delete).toHaveBeenCalledWith('/sync/playstation-store/connection');
    });
  });
});
