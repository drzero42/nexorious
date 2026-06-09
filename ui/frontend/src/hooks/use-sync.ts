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
  EpicGamesStoreConnectResponse,
  EpicGamesStoreConnectionResponse,
  GOGConnectResponse,
  GOGConnectionResponse,
  PlaystationStoreConfigureResponse,
  PlaystationStoreStatusResponse,
  HumbleConnectResponse,
  HumbleStatusResponse,
} from '@/types';

// Query Keys

export const syncKeys = {
  all: ['sync'] as const,
  configs: () => [...syncKeys.all, 'configs'] as const,
  config: (platform: SyncStorefront) => [...syncKeys.configs(), platform] as const,
  statuses: () => [...syncKeys.all, 'statuses'] as const,
  status: (platform: SyncStorefront) => [...syncKeys.statuses(), platform] as const,
  steamConnection: () => [...syncKeys.all, 'steamConnection'] as const,
  epicGamesStoreConnection: () => [...syncKeys.all, 'epicGamesStoreConnection'] as const,
  gogConnection: () => [...syncKeys.all, 'gogConnection'] as const,
  playstationStoreStatus: () => [...syncKeys.all, 'playstationStoreStatus'] as const,
  humbleStatus: () => [...syncKeys.all, 'humbleStatus'] as const,
  externalGames: (platform: SyncStorefront) =>
    [...syncKeys.all, 'external-games', platform] as const,
};

// Query Hooks

export function useSyncConfigs() {
  return useQuery({
    queryKey: syncKeys.configs(),
    queryFn: () => syncApi.getSyncConfigs(),
  });
}

export function useSyncConfig(platform: SyncStorefront) {
  return useQuery({
    queryKey: syncKeys.config(platform),
    queryFn: () => syncApi.getSyncConfig(platform),
  });
}

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

// Mutation Hooks

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

export function useVerifySteamCredentials() {
  return useMutation<SteamVerifyResponse, Error, { steamId: string; webApiKey: string }>({
    mutationFn: ({ steamId, webApiKey }) => syncApi.verifySteamCredentials(steamId, webApiKey),
  });
}

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
 * Returns connected state, credentialsError flag, and username.
 */
export function useSteamConnection(options?: { enabled?: boolean }) {
  return useQuery<SteamConnectionData, Error>({
    queryKey: syncKeys.steamConnection(),
    queryFn: syncApi.getSteamConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
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

// Epic Auth Hooks

/**
 * Tells the UI whether Epic sync is disabled server-side, connected, or
 * simply not configured.
 */
export function useEpicGamesStoreConnection(options?: { enabled?: boolean }) {
  return useQuery<EpicGamesStoreConnectionResponse, Error>({
    queryKey: syncKeys.epicGamesStoreConnection(),
    queryFn: syncApi.getEpicGamesStoreConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
  });
}

/**
 * On success, refreshes connection status and the user's sync configs.
 */
export function useConnectEpicGamesStore() {
  const queryClient = useQueryClient();

  return useMutation<EpicGamesStoreConnectResponse, Error, string>({
    mutationFn: (authCode: string) => syncApi.connectEpicGamesStore(authCode),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.EPIC_GAMES_STORE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicGamesStoreConnection() });
    },
  });
}

/**
 * Invalidates all Epic-related queries on success.
 */
export function useDisconnectEpicGamesStore() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectEpicGamesStore,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.EPIC_GAMES_STORE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.epicGamesStoreConnection() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Epic:', error);
    },
  });
}

// GOG Auth Hooks

export function useGOGConnection(options?: { enabled?: boolean }) {
  return useQuery<GOGConnectionResponse, Error>({
    queryKey: syncKeys.gogConnection(),
    queryFn: syncApi.getGOGConnection,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
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

// PSN Auth Hooks

/**
 * Invalidates sync configs and PSN status on success.
 */
export function useConfigurePlaystationStore() {
  const queryClient = useQueryClient();

  return useMutation<PlaystationStoreConfigureResponse, Error, string>({
    mutationFn: (npssoToken: string) => syncApi.configurePlaystationStore(npssoToken),
    onSuccess: () => {
      // Invalidate sync configs to refresh connection status
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({
        queryKey: syncKeys.config(SyncStorefront.PLAYSTATION_STORE),
      });
      queryClient.invalidateQueries({ queryKey: syncKeys.playstationStoreStatus() });
    },
    onError: (error) => {
      console.error('Failed to configure PSN:', error);
    },
  });
}

/**
 * Cached for 5 minutes.
 */
export function usePlaystationStoreStatus(options?: { enabled?: boolean }) {
  return useQuery<PlaystationStoreStatusResponse, Error>({
    queryKey: syncKeys.playstationStoreStatus(),
    queryFn: syncApi.getPlaystationStoreStatus,
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
  });
}

/**
 * Invalidates all PSN-related queries on success.
 */
export function useDisconnectPlaystationStore() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectPlaystationStore,
    onSuccess: () => {
      // Invalidate all PSN-related queries
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({
        queryKey: syncKeys.config(SyncStorefront.PLAYSTATION_STORE),
      });
      queryClient.invalidateQueries({ queryKey: syncKeys.playstationStoreStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect PSN:', error);
    },
  });
}

// Humble Bundle Auth Hooks

/**
 * Connect Humble Bundle. Invalidates sync configs and Humble status on success.
 */
export function useConnectHumble() {
  const queryClient = useQueryClient();

  return useMutation<HumbleConnectResponse, Error, string>({
    mutationFn: (sessionCookie: string) => syncApi.connectHumble(sessionCookie),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.HUMBLE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
    },
    onError: (error) => {
      console.error('Failed to connect Humble Bundle:', error);
    },
  });
}

/**
 * Humble Bundle connection status. Cached for 5 minutes.
 */
export function useHumbleStatus(options?: { enabled?: boolean }) {
  return useQuery<HumbleStatusResponse, Error>({
    queryKey: syncKeys.humbleStatus(),
    queryFn: syncApi.getHumbleStatus,
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: true,
    enabled: options?.enabled,
  });
}

/**
 * Disconnect Humble Bundle. Invalidates all Humble-related queries on success.
 */
export function useDisconnectHumble() {
  const queryClient = useQueryClient();

  return useMutation<void, Error>({
    mutationFn: syncApi.disconnectHumble,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: syncKeys.configs() });
      queryClient.invalidateQueries({ queryKey: syncKeys.config(SyncStorefront.HUMBLE) });
      queryClient.invalidateQueries({ queryKey: syncKeys.humbleStatus() });
    },
    onError: (error) => {
      console.error('Failed to disconnect Humble Bundle:', error);
    },
  });
}

// External Games Hooks

export function useExternalGames(platform: SyncStorefront, options?: { refetchInterval?: number }) {
  return useQuery({
    queryKey: syncKeys.externalGames(platform),
    queryFn: () => syncApi.getExternalGames(platform),
    refetchInterval: options?.refetchInterval,
  });
}

/**
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
