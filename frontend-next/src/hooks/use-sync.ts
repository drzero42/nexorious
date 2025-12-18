import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as syncApi from '@/api/sync';
import { SyncPlatform } from '@/types';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
} from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncPlatform) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncPlatform) => [...syncKeys.statuses(), platform] as const,
  ignoredGames: (params?: { source?: string; limit?: number; offset?: number }) =>
    [...syncKeys.all, 'ignored', params] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch all sync configurations for the current user.
 */
export function useSyncConfigs() {
  return useQuery({
    queryKey: syncKeys.configs(),
    queryFn: () => syncApi.getSyncConfigs(),
  });
}

/**
 * Hook to fetch sync configuration for a specific platform.
 */
export function useSyncConfig(platform: SyncPlatform) {
  return useQuery({
    queryKey: syncKeys.config(platform),
    queryFn: () => syncApi.getSyncConfig(platform),
  });
}

/**
 * Hook to fetch sync status for a specific platform.
 */
export function useSyncStatus(platform: SyncPlatform) {
  return useQuery({
    queryKey: syncKeys.status(platform),
    queryFn: () => syncApi.getSyncStatus(platform),
    refetchInterval: (query) => {
      // Poll every 5 seconds if syncing is in progress
      const data = query.state.data as SyncStatus | undefined;
      return data?.isSyncing ? 5000 : false;
    },
  });
}

/**
 * Hook to fetch all sync statuses for supported platforms.
 * Returns a map of platform -> status for easy lookup.
 */
export function useSyncStatuses() {
  return {
    steam: useQuery({
      queryKey: syncKeys.status(SyncPlatform.STEAM),
      queryFn: () => syncApi.getSyncStatus(SyncPlatform.STEAM),
      refetchInterval: 10000,
    }),
  };
}

/**
 * Hook to fetch ignored games list.
 */
export function useIgnoredGames(params?: { source?: string; limit?: number; offset?: number }) {
  return useQuery({
    queryKey: syncKeys.ignoredGames(params),
    queryFn: () => syncApi.getIgnoredGames(params),
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to update sync configuration for a platform.
 */
export function useUpdateSyncConfig() {
  const queryClient = useQueryClient();

  return useMutation<
    SyncConfig,
    Error,
    { platform: SyncPlatform; data: SyncConfigUpdateData }
  >({
    mutationFn: ({ platform, data }) => syncApi.updateSyncConfig(platform, data),
    onSuccess: (updatedConfig, { platform }) => {
      // Update the specific config in cache
      queryClient.setQueryData(syncKeys.config(platform), updatedConfig);
      // Invalidate the configs list to refetch
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
    },
  });
}

/**
 * Hook to trigger a manual sync for a platform.
 */
export function useTriggerSync() {
  const queryClient = useQueryClient();

  return useMutation<ManualSyncResponse, Error, SyncPlatform>({
    mutationFn: (platform) => syncApi.triggerSync(platform),
    onSuccess: (_result, platform) => {
      // Invalidate status to show syncing state
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}

/**
 * Hook to remove a game from the ignored list.
 */
export function useUnignoreGame() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: (id) => syncApi.unignoreGame(id),
    onSuccess: () => {
      // Invalidate ignored games list
      queryClient.invalidateQueries({ queryKey: syncKeys.ignoredGames() });
    },
  });
}
