import { describe, it, expect, vi, beforeEach } from 'vitest';
import { configurePSN, getPSNStatus, disconnectPSN } from './sync';
import { api } from './client';

vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

describe('PSN API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('configurePSN', () => {
    it('should configure PSN with valid NPSSO token', async () => {
      const mockResponse = {
        success: true,
        online_id: 'TestPSNUser',
        account_id: 'psn-account-123',
        region: 'US',
        message: 'PSN configured successfully',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await configurePSN('valid-npsso-token');

      expect(api.post).toHaveBeenCalledWith('/sync/psn/configure', {
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
        region: null,
        message: 'Invalid NPSSO token',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await configurePSN('invalid-token');

      expect(result).toEqual({
        valid: false,
        accountId: null,
        onlineId: null,
        error: 'Invalid NPSSO token',
      });
    });

    it('should transform snake_case to camelCase correctly', async () => {
      const mockResponse = {
        success: true,
        online_id: 'TestUser',
        account_id: '12345',
        region: 'EU',
        message: 'Success',
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await configurePSN('test-token');

      // Verify transformed keys
      expect(result).toHaveProperty('valid');
      expect(result).toHaveProperty('onlineId');
      expect(result).toHaveProperty('accountId');
      expect(result).not.toHaveProperty('online_id');
      expect(result).not.toHaveProperty('account_id');
      expect(result).not.toHaveProperty('success');
    });
  });

  describe('getPSNStatus', () => {
    it('should fetch PSN connection status when configured', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestPSNUser',
        account_id: 'psn-account-123',
        region: 'US',
        token_expired: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      expect(api.get).toHaveBeenCalledWith('/sync/psn/status');
      expect(result).toEqual({
        configured: true,
        accountId: 'psn-account-123',
        onlineId: 'TestPSNUser',
        tokenExpired: false,
      });
    });

    it('should fetch PSN status when not configured', async () => {
      const mockResponse = {
        is_configured: false,
        online_id: null,
        account_id: null,
        region: null,
        token_expired: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      expect(result).toEqual({
        configured: false,
        accountId: null,
        onlineId: null,
        tokenExpired: false,
      });
    });

    it('should detect expired token', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestPSNUser',
        account_id: 'psn-account-123',
        region: 'US',
        token_expired: true,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      expect(result.tokenExpired).toBe(true);
      expect(result.configured).toBe(true);
    });

    it('should transform snake_case to camelCase correctly', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestUser',
        account_id: '12345',
        region: 'EU',
        token_expired: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      // Verify transformed keys
      expect(result).toHaveProperty('configured');
      expect(result).toHaveProperty('onlineId');
      expect(result).toHaveProperty('accountId');
      expect(result).toHaveProperty('tokenExpired');
      expect(result).not.toHaveProperty('is_configured');
      expect(result).not.toHaveProperty('online_id');
      expect(result).not.toHaveProperty('account_id');
      expect(result).not.toHaveProperty('token_expired');
    });
  });

  describe('disconnectPSN', () => {
    it('should disconnect PSN', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await disconnectPSN();

      expect(api.delete).toHaveBeenCalledWith('/sync/psn/disconnect');
    });
  });
});
