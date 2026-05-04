import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as syncApi from '@/api/sync';
import { SyncPlatform } from '@/types';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SteamVerifyResponse,
  EpicAuthStartResponse,
  EpicAuthCompleteResponse,
  EpicAuthCheckResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
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
  epicAuth: () => [...syncKeys.all, 'epicAuth'] as const,
  psnStatus: () => [...syncKeys.all, 'psnStatus'] as const,
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
      // Poll every 5 seconds if syncing is in progress, otherwise every 30 seconds
      // The baseline 30s polling catches automatic syncs that start in the background
      const data = query.state.data as SyncStatus | undefined;
      return data?.isSyncing ? 5000 : 30000;
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
    onSuccess: (result, platform) => {
      // Optimistically set isSyncing to true and include the jobId from the response
      // This ensures the job progress card can immediately fetch job details
      // without waiting for the next status poll
      queryClient.setQueryData(
        syncKeys.status(platform),
        (old: SyncStatus | undefined) => ({
          platform: old?.platform ?? platform,
          isSyncing: true,
          lastSyncedAt: old?.lastSyncedAt ?? null,
          activeJobId: result.jobId,
        })
      );
      // Also invalidate to get fresh data from server
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}

/**
 * Hook to verify Steam credentials before saving.
 */
export function useVerifySteamCredentials() {
  return useMutation<
    SteamVerifyResponse,
    Error,
    { steamId: string; webApiKey: string }
  >({
    mutationFn: ({ steamId, webApiKey }) =>
      syncApi.verifySteamCredentials(steamId, webApiKey),
  });
}

/**
 * Hook to disconnect Steam integration.
 */
export function useDisconnectSteam() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, void>({
    mutationFn: () => syncApi.disconnectSteam(),
    onSuccess: () => {
      // Invalidate all sync-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
  });
}

// ============================================================================
// Epic Auth Hooks
// ============================================================================

/**
 * Hook to start Epic authentication flow.
 * Returns auth URL for user to visit.
 */
export function useStartEpicAuth() {
  return useMutation<EpicAuthStartResponse, Error>({
    mutationFn: syncApi.startEpicAuth,
    onError: (error) => {
      console.error('Failed to start Epic auth:', error);
    },
  });
}

/**
 * Hook to complete Epic authentication with code.
 * Invalidates sync configs on success.
 */
export function useCompleteEpicAuth() {
  const queryClient = useQueryClient();

  return useMutation<EpicAuthCompleteResponse, Error, string>({
    mutationFn: (code: string) => syncApi.completeEpicAuth(code),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.EPIC) });
    },
  });
}

/**
 * Hook to check current Epic authentication status.
 * Cached for 5 minutes.
 */
export function useCheckEpicAuth() {
  return useQuery<EpicAuthCheckResponse, Error>({
    queryKey: syncKeys.epicAuth(),
    queryFn: syncApi.checkEpicAuth,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
  });
}

/**
 * Hook to disconnect Epic integration.
 * Invalidates all Epic-related queries on success.
 */
export function useDisconnectEpic() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectEpic,
    onSuccess: () => {
      // Invalidate all Epic-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.EPIC) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicAuth() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Epic:', error);
    },
  });
}

// ============================================================================
// PSN Auth Hooks
// ============================================================================

/**
 * Hook to configure PSN with NPSSO token.
 * Invalidates sync configs and PSN status on success.
 */
export function useConfigurePSN() {
  const queryClient = useQueryClient();

  return useMutation<PSNConfigureResponse, Error, string>({
    mutationFn: (npssoToken: string) => syncApi.configurePSN(npssoToken),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.PSN) });
      queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
    },
    onError: (error) => {
      console.error('Failed to configure PSN:', error);
    },
  });
}

/**
 * Hook to check current PSN connection status.
 * Cached for 5 minutes.
 */
export function usePSNStatus() {
  return useQuery<PSNStatusResponse, Error>({
    queryKey: syncKeys.psnStatus(),
    queryFn: syncApi.getPSNStatus,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
  });
}

/**
 * Hook to disconnect PSN integration.
 * Invalidates all PSN-related queries on success.
 */
export function useDisconnectPSN() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectPSN,
    onSuccess: () => {
      // Invalidate all PSN-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncPlatform.PSN) });
      queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect PSN:', error);
    },
  });
}
