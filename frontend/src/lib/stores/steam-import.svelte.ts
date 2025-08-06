import { config } from '$lib/env';
import { auth } from './auth.svelte';

// Types based on backend schemas
export interface SteamImportJobResponse {
  id: string;
  status: 'pending' | 'processing' | 'awaiting_review' | 'finalizing' | 'completed' | 'failed';
  total_games: number;
  processed_games: number;
  matched_games: number;
  awaiting_review_games: number;
  skipped_games: number;
  imported_games: number;
  platform_added_games: number;
  error_message?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  games?: SteamImportGameResponse[];
}

export interface SteamImportGameResponse {
  id: string;
  steam_appid: number;
  steam_name: string;
  status: 'matched' | 'awaiting_user' | 'skipped' | 'imported' | 'platform_added' | 'already_owned' | 'import_failed';
  matched_game_id?: string;
  user_decision?: Record<string, any>;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface UserDecision {
  action: 'import' | 'skip';
  igdb_id?: string;
  game_name?: string;
  notes?: string;
}

export interface SteamImportState {
  currentJob: SteamImportJobResponse | null;
  userDecisions: Record<string, UserDecision>; // steam_appid -> decision
  
  // Polling state
  isPolling: boolean;
  pollingInterval: NodeJS.Timeout | null;
  lastUpdated: Date | null;
  
  // UI state
  isLoading: boolean;
  error: string | null;
}

const initialState: SteamImportState = {
  currentJob: null,
  userDecisions: {},
  isPolling: false,
  pollingInterval: null,
  lastUpdated: null,
  isLoading: false,
  error: null
};

function createSteamImportStore() {
  let state = $state<SteamImportState>(initialState);

  const steamImportStore = {
    get value() {
      return state;
    },

    reset() {
      // Clean up polling
      this.stopPolling();
      state = { ...initialState };
    },

    // Polling management
    async startPolling(jobId: string): Promise<void> {
      state = { ...state, isLoading: true, error: null };

      try {
        // First, fetch the current job status
        await this.fetchJobStatus(jobId);

        // Clean up existing polling
        this.stopPolling();

        // Start polling every 3 seconds
        const pollingInterval = setInterval(async () => {
          try {
            await this.fetchJobStatus(jobId);
            state = { ...state, lastUpdated: new Date() };
          } catch (error) {
            console.error('Polling error:', error);
            // Don't stop polling on individual errors, just log them
          }
        }, 3000);

        state = {
          ...state,
          isPolling: true,
          pollingInterval,
          lastUpdated: new Date()
        };

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start polling';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    stopPolling(): void {
      if (state.pollingInterval) {
        clearInterval(state.pollingInterval);
        state = {
          ...state,
          pollingInterval: null,
          isPolling: false
        };
      }
    },

    // Legacy method for compatibility - now just restarts polling
    reconnect(jobId?: string): void {
      if (state.currentJob) {
        this.startPolling(state.currentJob.id);
      } else if (jobId) {
        this.startPolling(jobId);
      }
    },

    // Legacy method for backward compatibility
    async connectToJob(jobId: string): Promise<void> {
      return this.startPolling(jobId);
    },

    // Legacy method for backward compatibility
    disconnect(): void {
      this.stopPolling();
    },

    // Job management methods
    async createImportJob(): Promise<SteamImportJobResponse> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/import`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({})
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.createImportJob();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to create import job');
        }

        const job = await response.json();
        state = { ...state, currentJob: job };
        return job;

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create import job';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    async fetchJobStatus(jobId: string): Promise<SteamImportJobResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/steam/import/${jobId}`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.fetchJobStatus(jobId);
          }
          throw new Error('Failed to fetch job status');
        }

        const job = await response.json();
        state = { ...state, currentJob: job };
        return job;

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch job status';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    async cancelJob(jobId: string): Promise<void> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/import/${jobId}`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.cancelJob(jobId);
          }
          throw new Error('Failed to cancel job');
        }

        // Update job status
        if (state.currentJob?.id === jobId) {
          state = {
            ...state,
            currentJob: { ...state.currentJob, status: 'failed', error_message: 'Cancelled by user' }
          };
        }

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to cancel job';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    // User decision management
    setUserDecision(steamAppId: string, decision: UserDecision): void {
      state = {
        ...state,
        userDecisions: { ...state.userDecisions, [steamAppId]: decision }
      };
    },

    clearUserDecision(steamAppId: string): void {
      const { [steamAppId]: removed, ...remainingDecisions } = state.userDecisions;
      state = { ...state, userDecisions: remainingDecisions };
    },

    async submitUserDecisions(jobId: string, decisions: Record<string, UserDecision>): Promise<void> {
      state = { ...state, isLoading: true, error: null };

      try {
        const payload = { decisions };
        const url = `${config.apiUrl}/steam/import/${jobId}/decision`;
        
        const response = await fetch(url, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify(payload)
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.submitUserDecisions(jobId, decisions);
          }
          
          let errorData;
          try {
            errorData = await response.json();
          } catch (jsonError) {
            errorData = {};
          }
          
          const errorMessage = errorData.detail || `Failed to submit decisions (${response.status}: ${response.statusText})`;
          throw new Error(errorMessage);
        }

        // Clear user decisions after successful submission
        state = { ...state, userDecisions: {} };

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to submit decisions';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    async confirmFinalImport(jobId: string): Promise<void> {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam/import/${jobId}/confirm`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({})
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.confirmFinalImport(jobId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to confirm import');
        }

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to confirm import';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    // Helper methods

    clearError(): void {
      state = { ...state, error: null };
    }
  };

  return steamImportStore;
}

export const steamImport = createSteamImportStore();