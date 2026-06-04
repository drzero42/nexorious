import { describe, it, expect, vi, beforeEach } from 'vitest';
import { configurePSN, getPSNStatus, disconnectPSN } from './sync';
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

  describe('configurePSN', () => {
    it('should configure PSN with valid NPSSO token', async () => {
      const mockResponse = {
        success: true,
        online_id: 'TestPSNUser',
        account_id: 'psn-account-123',
        message: 'PSN configured successfully',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

      const result = await configurePSN('valid-npsso-token');

      expect(api.put).toHaveBeenCalledWith('/sync/psn/connection', {
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
        message: 'Success',
      };

      vi.mocked(api.put).mockResolvedValueOnce(mockResponse);

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
        credentials_error: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      expect(api.get).toHaveBeenCalledWith('/sync/psn/connection');
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

      const result = await getPSNStatus();

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

      const result = await getPSNStatus();

      expect(result.credentialsError).toBe(true);
      expect(result.configured).toBe(true);
    });

    it('should transform snake_case to camelCase correctly', async () => {
      const mockResponse = {
        is_configured: true,
        online_id: 'TestUser',
        credentials_error: false,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPSNStatus();

      // Verify transformed keys
      expect(result).toHaveProperty('configured');
      expect(result).toHaveProperty('onlineId');
      expect(result).toHaveProperty('credentialsError');
      expect(result).not.toHaveProperty('is_configured');
      expect(result).not.toHaveProperty('online_id');
      expect(result).not.toHaveProperty('credentials_error');
    });
  });

  describe('disconnectPSN', () => {
    it('should disconnect PSN', async () => {
      vi.mocked(api.delete).mockResolvedValueOnce(undefined);

      await disconnectPSN();

      expect(api.delete).toHaveBeenCalledWith('/sync/psn/connection');
    });
  });
});
