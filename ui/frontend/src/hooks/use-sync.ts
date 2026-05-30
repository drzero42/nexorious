import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import * as syncApi from '@/api/sync';
import { SyncStorefront } from '@/types';
import type {
  SyncConfig,
  SyncConfigUpdateData,
  SyncStatus,
  ManualSyncResponse,
  SteamVerifyResponse,
  SteamConnectionData,
  EpicConnectResponse,
  EpicConnectionResponse,
  GOGConnectResponse,
  GOGConnectionResponse,
  PSNConfigureResponse,
  PSNStatusResponse,
} from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncStorefront) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncStorefront) => [...syncKeys.statuses(), platform] as const,
  steamConnection: () => [...syncKeys.all, 'steamConnection'] as const,
  epicConnection: () => [...syncKeys.all, 'epicConnection'] as const,
  gogConnection: () => [...syncKeys.all, 'gogConnection'] as const,
  psnStatus: () => [...syncKeys.all, 'psnStatus'] as const,
  externalGames: (platform: SyncStorefront) =>
    [...syncKeys.all, 'external-games', platform] as const,
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
export function useSyncConfig(platform: SyncStorefront) {
  return useQuery({
    queryKey: syncKeys.config(platform),
    queryFn: () => syncApi.getSyncConfig(platform),
  });
}

/**
 * Hook to fetch sync status for a specific platform.
 */
export function useSyncStatus(platform: SyncStorefront) {
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
      queryKey: syncKeys.status(SyncStorefront.STEAM),
      queryFn: () => syncApi.getSyncStatus(SyncStorefront.STEAM),
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

  return useMutation<SyncConfig, Error, { storefront: SyncStorefront; data: SyncConfigUpdateData }>(
    {
      mutationFn: ({ storefront, data }) => syncApi.updateSyncConfig(storefront, data),
      onSuccess: (updatedConfig, { storefront }) => {
        // Update the specific config in cache
        queryClient.setQueryData(syncKeys.config(storefront), updatedConfig);
        // Invalidate the configs list to refetch
        queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      },
    },
  );
}

/**
 * Hook to trigger a manual sync for a platform.
 */
export function useTriggerSync() {
  const queryClient = useQueryClient();

  return useMutation<ManualSyncResponse, Error, SyncStorefront>({
    mutationFn: (platform) => syncApi.triggerSync(platform),
    onSuccess: (result, platform) => {
      // Optimistically set isSyncing to true and include the jobId from the response
      // This ensures the job progress card can immediately fetch job details
      // without waiting for the next status poll
      queryClient.setQueryData(syncKeys.status(platform), (old: SyncStatus | undefined) => ({
        storefront: old?.storefront ?? platform,
        isSyncing: true,
        lastSyncedAt: old?.lastSyncedAt ?? null,
        activeJobId: result.jobId,
        externalGameCount: old?.externalGameCount ?? 0,
      }));
      // Also invalidate to get fresh data from server
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}

/**
 * Hook to verify Steam credentials before saving.
 */
export function useVerifySteamCredentials() {
  return useMutation<SteamVerifyResponse, Error, { steamId: string; webApiKey: string }>({
    mutationFn: ({ steamId, webApiKey }) => syncApi.verifySteamCredentials(steamId, webApiKey),
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

/**
 * Hook to fetch Steam connection status.
 * Returns connected state, credentialsError flag, steamId, and username.
 */
export function useSteamConnection() {
  return useQuery<SteamConnectionData, Error>({
    queryKey: syncKeys.steamConnection(),
    queryFn: syncApi.getSteamConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
  });
}

export function useResetSyncData() {
  const queryClient = useQueryClient();

  return useMutation<void, Error, SyncStorefront>({
    mutationFn: (platform) => syncApi.resetSyncData(platform),
    onSuccess: (_, platform) => {
      queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(platform) });
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(platform) });
      queryClient.invalidateQueries({ queryKey: syncKeys.status(platform) });
    },
  });
}

// ============================================================================
// Epic Auth Hooks
// ============================================================================

/**
 * Hook to fetch the current Epic Games Store connection status.
 * Tells the UI whether Epic sync is disabled (LEGENDARY_WORK_DIR unset on
 * the backend), connected, or simply not configured.
 */
export function useEpicConnection() {
  return useQuery<EpicConnectionResponse, Error>({
    queryKey: syncKeys.epicConnection(),
    queryFn: syncApi.getEpicConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
  });
}

/**
 * Hook to connect Epic Games Store by exchanging the legendary auth code.
 * On success, refreshes connection status and the user's sync configs.
 */
export function useConnectEpic() {
  const queryClient = useQueryClient();

  return useMutation<EpicConnectResponse, Error, string>({
    mutationFn: (authCode: string) => syncApi.connectEpic(authCode),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.EPIC) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicConnection() });
    },
  });
}

/**
 * Hook to disconnect Epic Games Store.
 * Invalidates all Epic-related queries on success.
 */
export function useDisconnectEpic() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectEpic,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.EPIC) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicConnection() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Epic:', error);
    },
  });
}

// ============================================================================
// GOG Auth Hooks
// ============================================================================

export function useGOGConnection() {
  return useQuery<GOGConnectionResponse, Error>({
    queryKey: syncKeys.gogConnection(),
    queryFn: syncApi.getGOGConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
  });
}

export function useConnectGOG() {
  const queryClient = useQueryClient();

  return useMutation<GOGConnectResponse, Error, string>({
    mutationFn: (authCode: string) => syncApi.connectGOG(authCode),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.GOG) });
      queryClient.invalidateQueries({ queryKey: syncKeys.gogConnection() });
    },
  });
}

export function useDisconnectGOG() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectGOG,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.GOG) });
      queryClient.invalidateQueries({ queryKey: syncKeys.gogConnection() });
    },
    onError: (error) => {
      console.error('Failed to disconnect GOG:', error);
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
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.PSN) });
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
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.PSN) });
      queryClient.invalidateQueries({ queryKey: syncKeys.psnStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect PSN:', error);
    },
  });
}

// ============================================================================
// External Games Hooks
// ============================================================================

/**
 * Hook to fetch external games for a specific platform.
 */
export function useExternalGames(platform: SyncStorefront, options?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: syncKeys.externalGames(platform),
    queryFn: () => syncApi.getExternalGames(platform),
    refetchInterval: options?.refetchInterval,
  });
}

/**
 * Hook to skip an external game.
 * Invalidates all sync queries on success.
 */
export function useSkipExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => syncApi.skipExternalGame(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to skip game');
    },
  });
}

/**
 * Hook to unskip an external game.
 * Invalidates all sync queries on success.
 */
export function useUnskipExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => syncApi.unskipExternalGame(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to unskip game');
    },
  });
}

/**
 * Hook to retry all failed external games for a storefront.
 * Invalidates external games query on success.
 */
export function useRetryFailedExternalGames() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, SyncStorefront>({
    mutationFn: (storefront) => syncApi.retryFailedExternalGames(storefront),
    onSuccess: (_, storefront) => {
      queryClient.invalidateQueries({ queryKey: syncKeys.externalGames(storefront) });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to retry failed games');
    },
  });
}

/**
 * Hook to rematch an external game to a different IGDB entry.
 * Invalidates all sync queries on success.
 */
export function useRematchExternalGame() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      igdbId,
      orphanAction,
    }: {
      id: string;
      igdbId: number;
      orphanAction?: 'keep' | 'remove';
    }) => syncApi.rematchExternalGame(id, igdbId, orphanAction),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.all });
    },
    onError: (err: Error) => {
      toast.error(err.message ?? 'Failed to rematch game');
    },
  });
}
