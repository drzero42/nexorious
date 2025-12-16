/**
 * Sync store for managing platform sync configurations.
 *
 * Provides state management and API methods for viewing and updating
 * sync settings, and triggering manual syncs.
 */

import { config } from '$lib/env';
import { api } from '$lib/services/api';
import type {
  SyncConfig,
  SyncConfigListResponse,
  SyncConfigUpdateRequest,
  ManualSyncTriggerResponse,
  SyncStatusResponse,
  IgnoredGame,
  IgnoredGameListResponse
} from '$lib/types/jobs';
import { SyncFrequency, SyncPlatform } from '$lib/types/jobs';

// Re-export types and enums for convenience
export type { SyncConfig, SyncConfigUpdateRequest, SyncStatusResponse, IgnoredGame };
export { SyncFrequency, SyncPlatform };

export interface SyncState {
  configs: SyncConfig[];
  syncStatuses: Map<string, SyncStatusResponse>;
  ignoredGames: IgnoredGame[];
  ignoredGamesTotal: number;
  isLoading: boolean;
  isLoadingIgnored: boolean;
  isSyncing: Map<string, boolean>;
  error: string | null;
}

function createSyncStore() {
  let state = $state<SyncState>({
    configs: [],
    syncStatuses: new Map(),
    ignoredGames: [],
    ignoredGamesTotal: 0,
    isLoading: false,
    isLoadingIgnored: false,
    isSyncing: new Map(),
    error: null
  });

  const store = {
    get value() {
      return state;
    },

    /**
     * Check if any platform is currently syncing.
     */
    get isAnySyncing() {
      for (const syncing of state.isSyncing.values()) {
        if (syncing) return true;
      }
      return false;
    },

    /**
     * Load all sync configurations for the current user.
     */
    loadConfigs: async () => {
      state.isLoading = true;
      state.error = null;

      try {
        const response = await api.get(`${config.apiUrl}/sync/config`);
        const data: SyncConfigListResponse = await response.json();

        state.configs = data.configs;
        state.isLoading = false;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load sync configs';
        state.isLoading = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get sync configuration for a specific platform.
     */
    getConfig: async (platform: SyncPlatform) => {
      try {
        const response = await api.get(`${config.apiUrl}/sync/config/${platform}`);
        const syncConfig: SyncConfig = await response.json();

        // Update in the list
        const index = state.configs.findIndex((c) => c.platform === platform);
        if (index !== -1) {
          state.configs[index] = syncConfig;
        } else {
          state.configs.push(syncConfig);
        }

        return syncConfig;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load sync config';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Update sync configuration for a specific platform.
     */
    updateConfig: async (
      platform: SyncPlatform,
      updates: SyncConfigUpdateRequest
    ): Promise<SyncConfig> => {
      state.error = null;

      try {
        const response = await api.put(`${config.apiUrl}/sync/config/${platform}`, updates);
        const syncConfig: SyncConfig = await response.json();

        // Update in the list
        const index = state.configs.findIndex((c) => c.platform === platform);
        if (index !== -1) {
          state.configs[index] = syncConfig;
        } else {
          state.configs.push(syncConfig);
        }

        return syncConfig;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to update sync config';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Trigger a manual sync for a specific platform.
     */
    triggerSync: async (platform: SyncPlatform): Promise<ManualSyncTriggerResponse> => {
      state.error = null;
      state.isSyncing.set(platform, true);

      try {
        const response = await api.post(`${config.apiUrl}/sync/${platform}`);
        const result: ManualSyncTriggerResponse = await response.json();

        // Update sync status
        state.syncStatuses.set(platform, {
          platform: platform,
          is_syncing: true,
          last_synced_at: null,
          active_job_id: result.job_id
        });

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to trigger sync';
        state.error = errorMessage;
        state.isSyncing.set(platform, false);
        throw error;
      }
    },

    /**
     * Get the current sync status for a platform.
     */
    getSyncStatus: async (platform: SyncPlatform): Promise<SyncStatusResponse> => {
      try {
        const response = await api.get(`${config.apiUrl}/sync/${platform}/status`);
        const status: SyncStatusResponse = await response.json();

        state.syncStatuses.set(platform, status);
        state.isSyncing.set(platform, status.is_syncing);

        return status;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to get sync status';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Refresh sync statuses for all configured platforms.
     */
    refreshAllStatuses: async () => {
      const platforms = Object.values(SyncPlatform);
      await Promise.all(platforms.map((p) => store.getSyncStatus(p).catch(() => null)));
    },

    /**
     * Get config by platform from current state.
     */
    getConfigByPlatform: (platform: string): SyncConfig | undefined => {
      return state.configs.find((c) => c.platform === platform);
    },

    /**
     * Check if a specific platform is syncing.
     */
    isPlatformSyncing: (platform: string): boolean => {
      return state.isSyncing.get(platform) ?? false;
    },

    /**
     * Load ignored games list with optional source filter.
     */
    loadIgnoredGames: async (source?: SyncPlatform, skip: number = 0, limit: number = 100) => {
      state.isLoadingIgnored = true;
      state.error = null;

      try {
        const params = new URLSearchParams();
        if (source) params.append('source', source);
        params.append('skip', skip.toString());
        params.append('limit', limit.toString());

        const response = await api.get(`${config.apiUrl}/sync/ignored?${params}`);
        const data: IgnoredGameListResponse = await response.json();

        state.ignoredGames = data.items;
        state.ignoredGamesTotal = data.total;
        state.isLoadingIgnored = false;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to load ignored games';
        state.isLoadingIgnored = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Remove a game from the ignored list (it will appear in the next sync).
     */
    unignoreGame: async (id: string) => {
      state.error = null;

      try {
        await api.delete(`${config.apiUrl}/sync/ignored/${id}`);

        // Remove from local state
        state.ignoredGames = state.ignoredGames.filter((g) => g.id !== id);
        state.ignoredGamesTotal = Math.max(0, state.ignoredGamesTotal - 1);
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to unignore game';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get the count of ignored games.
     */
    get ignoredCount() {
      return state.ignoredGamesTotal;
    },

    /**
     * Clear error state.
     */
    clearError: () => {
      state.error = null;
    },

    /**
     * Reset store to initial state.
     */
    reset: () => {
      state = {
        configs: [],
        syncStatuses: new Map(),
        ignoredGames: [],
        ignoredGamesTotal: 0,
        isLoading: false,
        isLoadingIgnored: false,
        isSyncing: new Map(),
        error: null
      };
    }
  };

  return store;
}

export const sync = createSyncStore();
