/**
 * Sync store for managing platform sync configurations.
 *
 * Provides state management and API methods for viewing and updating
 * sync settings, and triggering manual syncs.
 */

import { auth } from './auth.svelte';
import { config } from '$lib/env';
import type {
  SyncConfig,
  SyncConfigListResponse,
  SyncConfigUpdateRequest,
  ManualSyncTriggerResponse,
  SyncStatusResponse
} from '$lib/types/jobs';
import { SyncFrequency, SyncPlatform } from '$lib/types/jobs';

// Re-export types and enums for convenience
export type { SyncConfig, SyncConfigUpdateRequest, SyncStatusResponse };
export { SyncFrequency, SyncPlatform };

export interface SyncState {
  configs: SyncConfig[];
  syncStatuses: Map<string, SyncStatusResponse>;
  isLoading: boolean;
  isSyncing: Map<string, boolean>;
  error: string | null;
}

function createSyncStore() {
  let state = $state<SyncState>({
    configs: [],
    syncStatuses: new Map(),
    isLoading: false,
    isSyncing: new Map(),
    error: null
  });

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authState.accessToken}`,
        ...options.headers
      }
    });

    if (!response.ok) {
      if (response.status === 401) {
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${auth.value.accessToken}`,
              ...options.headers
            }
          });
        }
      }

      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      try {
        const errorBody = await response.json();
        if (errorBody.detail) {
          errorMessage = errorBody.detail;
        }
      } catch {
        // Use default message if we can't parse the error body
      }

      throw new Error(errorMessage);
    }

    return response;
  };

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
        const response = await apiCall(`${config.apiUrl}/sync/config`);
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
        const response = await apiCall(`${config.apiUrl}/sync/config/${platform}`);
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
        const response = await apiCall(`${config.apiUrl}/sync/config/${platform}`, {
          method: 'PUT',
          body: JSON.stringify(updates)
        });
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
        const response = await apiCall(`${config.apiUrl}/sync/${platform}`, {
          method: 'POST'
        });
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
        const response = await apiCall(`${config.apiUrl}/sync/${platform}/status`);
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
        isLoading: false,
        isSyncing: new Map(),
        error: null
      };
    }
  };

  return store;
}

export const sync = createSyncStore();
