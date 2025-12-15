import { browser } from '$app/environment';
import { config } from '$lib/env';
import { loggers } from '$lib/services/logger';

const log = loggers.api;

export interface AppStatusState {
  igdbConfigured: boolean;
  isLoading: boolean;
  error: string | null;
  hasFetched: boolean;
}

const initialState: AppStatusState = {
  igdbConfigured: true, // Assume configured until we know otherwise
  isLoading: false,
  error: null,
  hasFetched: false
};

function createAppStatusStore() {
  let state = $state<AppStatusState>(initialState);

  const appStatusStore = {
    get value() {
      return state;
    },

    /**
     * Fetch the application status from the backend.
     * This is called once on app initialization.
     */
    fetchStatus: async () => {
      // Only fetch once and only in browser
      if (state.hasFetched || !browser) {
        return;
      }

      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/status`);

        if (!response.ok) {
          throw new Error(`Failed to fetch status: ${response.status}`);
        }

        const data = await response.json();
        state = {
          igdbConfigured: data.igdb_configured,
          isLoading: false,
          error: null,
          hasFetched: true
        };
      } catch (error) {
        log.error('Failed to fetch app status', error);
        state = {
          ...state,
          isLoading: false,
          error: error instanceof Error ? error.message : 'Failed to fetch status',
          hasFetched: true
        };
      }
    },

    /**
     * Reset the store to its initial state.
     * Useful for testing or manual refresh.
     */
    reset: () => {
      state = initialState;
    }
  };

  return appStatusStore;
}

export const appStatus = createAppStatusStore();
