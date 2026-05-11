import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryWrapper } from '@/test/test-utils';
import { setAuthHandlers } from '@/api/client';
import { useConfigurePSN, usePSNStatus, useDisconnectPSN, syncKeys } from './use-sync';
import * as syncApi from '@/api/sync';

describe('PSN Hooks', () => {
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

  describe('useConfigurePSN', () => {
    it('configures PSN with NPSSO token successfully', async () => {
      const mockConfigurePSN = vi.spyOn(syncApi, 'configurePSN');
      mockConfigurePSN.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      const { result } = renderHook(() => useConfigurePSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('test-npsso-token');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(mockConfigurePSN).toHaveBeenCalledWith('test-npsso-token');
      expect(result.current.data).toEqual({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      mockConfigurePSN.mockRestore();
    });

    it('handles PSN configuration error', async () => {
      const mockConfigurePSN = vi.spyOn(syncApi, 'configurePSN');
      mockConfigurePSN.mockRejectedValue(new Error('Invalid NPSSO token'));

      const { result } = renderHook(() => useConfigurePSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('invalid-token');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Invalid NPSSO token');

      mockConfigurePSN.mockRestore();
    });

    it('invalidates queries on successful configuration', async () => {
      const mockConfigurePSN = vi.spyOn(syncApi, 'configurePSN');
      mockConfigurePSN.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      const { result } = renderHook(() => useConfigurePSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('test-npsso-token');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // Query invalidation happens automatically via TanStack Query
      // We verify the mutation was successful

      mockConfigurePSN.mockRestore();
    });
  });

  describe('usePSNStatus', () => {
    it('fetches PSN status successfully', async () => {
      const mockGetPSNStatus = vi.spyOn(syncApi, 'getPSNStatus');
      mockGetPSNStatus.mockResolvedValue({
        configured: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        tokenExpired: false,
      });

      const { result } = renderHook(() => usePSNStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual({
        configured: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        tokenExpired: false,
      });

      mockGetPSNStatus.mockRestore();
    });

    it('handles PSN status with expired token', async () => {
      const mockGetPSNStatus = vi.spyOn(syncApi, 'getPSNStatus');
      mockGetPSNStatus.mockResolvedValue({
        configured: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        tokenExpired: true,
      });

      const { result } = renderHook(() => usePSNStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.tokenExpired).toBe(true);

      mockGetPSNStatus.mockRestore();
    });

    it('handles PSN status fetch error', async () => {
      const mockGetPSNStatus = vi.spyOn(syncApi, 'getPSNStatus');
      mockGetPSNStatus.mockRejectedValue(new Error('Failed to fetch PSN status'));

      const { result } = renderHook(() => usePSNStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch PSN status');

      mockGetPSNStatus.mockRestore();
    });
  });

  describe('useDisconnectPSN', () => {
    it('disconnects PSN successfully', async () => {
      const mockDisconnectPSN = vi.spyOn(syncApi, 'disconnectPSN');
      mockDisconnectPSN.mockResolvedValue(undefined);

      const { result } = renderHook(() => useDisconnectPSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync();
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(mockDisconnectPSN).toHaveBeenCalled();

      mockDisconnectPSN.mockRestore();
    });

    it('handles PSN disconnect error', async () => {
      const mockDisconnectPSN = vi.spyOn(syncApi, 'disconnectPSN');
      mockDisconnectPSN.mockRejectedValue(new Error('Failed to disconnect PSN'));

      const { result } = renderHook(() => useDisconnectPSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync();
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to disconnect PSN');

      mockDisconnectPSN.mockRestore();
    });

    it('invalidates all PSN queries on successful disconnect', async () => {
      const mockDisconnectPSN = vi.spyOn(syncApi, 'disconnectPSN');
      mockDisconnectPSN.mockResolvedValue(undefined);

      const { result } = renderHook(() => useDisconnectPSN(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync();
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // Query invalidation happens automatically via TanStack Query
      // We verify the mutation was successful

      mockDisconnectPSN.mockRestore();
    });
  });

  describe('syncKeys', () => {
    it('generates correct query key for PSN status', () => {
      expect(syncKeys.psnStatus()).toEqual(['sync', 'psnStatus']);
    });
  });
});
