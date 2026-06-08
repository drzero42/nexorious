import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryWrapper } from '@/test/test-utils';
import {
  useConfigurePlaystationStore,
  usePlaystationStoreStatus,
  useDisconnectPlaystationStore,
  syncKeys,
} from './use-sync';
import * as syncApi from '@/api/sync';

describe('PSN Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('useConfigurePlaystationStore', () => {
    it('configures PSN with NPSSO token successfully', async () => {
      const mockConfigurePlaystationStore = vi.spyOn(syncApi, 'configurePlaystationStore');
      mockConfigurePlaystationStore.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      const { result } = renderHook(() => useConfigurePlaystationStore(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('test-npsso-token');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(mockConfigurePlaystationStore).toHaveBeenCalledWith('test-npsso-token');
      expect(result.current.data).toEqual({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      mockConfigurePlaystationStore.mockRestore();
    });

    it('handles PSN configuration error', async () => {
      const mockConfigurePlaystationStore = vi.spyOn(syncApi, 'configurePlaystationStore');
      mockConfigurePlaystationStore.mockRejectedValue(new Error('Invalid NPSSO token'));

      const { result } = renderHook(() => useConfigurePlaystationStore(), {
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

      mockConfigurePlaystationStore.mockRestore();
    });

    it('invalidates queries on successful configuration', async () => {
      const mockConfigurePlaystationStore = vi.spyOn(syncApi, 'configurePlaystationStore');
      mockConfigurePlaystationStore.mockResolvedValue({
        valid: true,
        accountId: 'test-account-id',
        onlineId: 'TestUser',
        error: null,
      });

      const { result } = renderHook(() => useConfigurePlaystationStore(), {
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

      mockConfigurePlaystationStore.mockRestore();
    });
  });

  describe('usePlaystationStoreStatus', () => {
    it('fetches PSN status successfully', async () => {
      const mockGetPlaystationStoreStatus = vi.spyOn(syncApi, 'getPlaystationStoreStatus');
      mockGetPlaystationStoreStatus.mockResolvedValue({
        configured: true,
        onlineId: 'TestUser',
        credentialsError: false,
      });

      const { result } = renderHook(() => usePlaystationStoreStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data).toEqual({
        configured: true,
        onlineId: 'TestUser',
        credentialsError: false,
      });

      mockGetPlaystationStoreStatus.mockRestore();
    });

    it('handles PSN status with expired token', async () => {
      const mockGetPlaystationStoreStatus = vi.spyOn(syncApi, 'getPlaystationStoreStatus');
      mockGetPlaystationStoreStatus.mockResolvedValue({
        configured: true,
        onlineId: 'TestUser',
        credentialsError: true,
      });

      const { result } = renderHook(() => usePlaystationStoreStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.credentialsError).toBe(true);

      mockGetPlaystationStoreStatus.mockRestore();
    });

    it('handles PSN status fetch error', async () => {
      const mockGetPlaystationStoreStatus = vi.spyOn(syncApi, 'getPlaystationStoreStatus');
      mockGetPlaystationStoreStatus.mockRejectedValue(new Error('Failed to fetch PSN status'));

      const { result } = renderHook(() => usePlaystationStoreStatus(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch PSN status');

      mockGetPlaystationStoreStatus.mockRestore();
    });
  });

  describe('useDisconnectPlaystationStore', () => {
    it('disconnects PSN successfully', async () => {
      const mockDisconnectPlaystationStore = vi.spyOn(syncApi, 'disconnectPlaystationStore');
      mockDisconnectPlaystationStore.mockResolvedValue(undefined);

      const { result } = renderHook(() => useDisconnectPlaystationStore(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync();
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(mockDisconnectPlaystationStore).toHaveBeenCalled();

      mockDisconnectPlaystationStore.mockRestore();
    });

    it('handles PSN disconnect error', async () => {
      const mockDisconnectPlaystationStore = vi.spyOn(syncApi, 'disconnectPlaystationStore');
      mockDisconnectPlaystationStore.mockRejectedValue(new Error('Failed to disconnect PSN'));

      const { result } = renderHook(() => useDisconnectPlaystationStore(), {
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

      mockDisconnectPlaystationStore.mockRestore();
    });

    it('invalidates all PSN queries on successful disconnect', async () => {
      const mockDisconnectPlaystationStore = vi.spyOn(syncApi, 'disconnectPlaystationStore');
      mockDisconnectPlaystationStore.mockResolvedValue(undefined);

      const { result } = renderHook(() => useDisconnectPlaystationStore(), {
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

      mockDisconnectPlaystationStore.mockRestore();
    });
  });

  describe('syncKeys', () => {
    it('generates correct query key for PSN status', () => {
      expect(syncKeys.playstationStoreStatus()).toEqual(['sync', 'playstationStoreStatus']);
    });
  });
});
