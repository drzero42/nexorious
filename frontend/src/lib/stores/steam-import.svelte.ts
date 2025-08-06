import { config } from '$lib/env';
import { auth } from './auth.svelte';
import { SteamImportWebSocketService, type WebSocketConnectionStatus } from '$lib/services/SteamImportWebSocketService';

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
  
  // WebSocket connection state
  webSocketService: SteamImportWebSocketService | null;
  connectionStatus: WebSocketConnectionStatus;
  isConnected: boolean;
  reconnectAttempts: number;
  maxReconnectAttempts: number;
  lastActivity: Date | null;
  
  // UI state
  isLoading: boolean;
  error: string | null;
}

const initialState: SteamImportState = {
  currentJob: null,
  userDecisions: {},
  webSocketService: null,
  connectionStatus: 'disconnected',
  isConnected: false,
  reconnectAttempts: 0,
  maxReconnectAttempts: 10,
  lastActivity: null,
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
      // Clean up WebSocket connection
      if (state.webSocketService) {
        state.webSocketService.destroy();
      }
      state = { ...initialState };
    },

    // WebSocket connection management
    async connectToJob(jobId: string): Promise<void> {
      state = { ...state, isLoading: true, error: null };

      try {
        // First, fetch the current job status
        await this.fetchJobStatus(jobId);

        // Clean up existing WebSocket connection
        if (state.webSocketService) {
          state.webSocketService.destroy();
        }

        // Create new WebSocket service
        const wsService = new SteamImportWebSocketService(jobId, {
          onOpen: () => {
            state = {
              ...state,
              connectionStatus: 'connected',
              isConnected: true,
              reconnectAttempts: 0,
              lastActivity: new Date()
            };
          },

          onClose: () => {
            state = {
              ...state,
              connectionStatus: 'disconnected',
              isConnected: false
            };
          },

          onError: () => {
            state = {
              ...state,
              connectionStatus: 'error',
              isConnected: false
            };
          },

          onStatusChange: (status: string, data: any) => {
            if (state.currentJob) {
              state = {
                ...state,
                currentJob: { ...state.currentJob, status: status as any, ...data }
              };
            }
          },

          onProgress: (data: any) => {
            if (state.currentJob) {
              state = {
                ...state,
                currentJob: { ...state.currentJob, ...data },
                lastActivity: new Date()
              };
            }
          },

          onGameMatched: (data: any) => {
            // Update game status in current job
            this.updateGameStatus(data.steam_appid, 'matched', data);
          },

          onGameNeedsReview: (data: any) => {
            // Update game status and increment awaiting review count
            this.updateGameStatus(data.steam_appid, 'awaiting_user', data);
          },

          onGameImported: (data: any) => {
            // Update game status and increment imported count
            this.updateGameStatus(data.steam_appid, 'imported', data);
          },

          onPlatformAdded: (data: any) => {
            // Update game status and increment platform added count
            this.updateGameStatus(data.steam_appid, 'platform_added', data);
          },

          onGameSkipped: (data: any) => {
            // Update game status and increment skipped count
            this.updateGameStatus(data.steam_appid, 'skipped', data);
          },

          onImportComplete: (data: any) => {
            if (state.currentJob) {
              state = {
                ...state,
                currentJob: { 
                  ...state.currentJob, 
                  status: 'completed',
                  completed_at: new Date().toISOString(),
                  ...data 
                }
              };
            }
          },

          onImportError: (error: string) => {
            state = { ...state, error };
            if (state.currentJob) {
              state = {
                ...state,
                currentJob: { 
                  ...state.currentJob, 
                  status: 'failed',
                  error_message: error 
                }
              };
            }
          }
        });

        state = { ...state, webSocketService: wsService };

        // Connect to WebSocket
        await wsService.connect();
        
        // Update connection status based on service
        const reconnectionInfo = wsService.getReconnectionInfo();
        state = {
          ...state,
          connectionStatus: wsService.getConnectionStatus(),
          isConnected: wsService.getConnectionStatus() === 'connected',
          reconnectAttempts: reconnectionInfo.attempts,
          lastActivity: reconnectionInfo.lastActivity
        };

      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to connect to job';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
      }
    },

    disconnect(): void {
      if (state.webSocketService) {
        state.webSocketService.destroy();
        state = {
          ...state,
          webSocketService: null,
          connectionStatus: 'disconnected',
          isConnected: false,
          reconnectAttempts: 0,
          lastActivity: null
        };
      }
    },

    reconnect(): void {
      if (state.webSocketService) {
        state.webSocketService.reconnect();
      }
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
      console.log('🔧 DEBUG: submitUserDecisions called');
      console.log('🔧 DEBUG: jobId:', jobId);
      console.log('🔧 DEBUG: decisions:', decisions);
      console.log('🔧 DEBUG: decisions object keys:', Object.keys(decisions));
      console.log('🔧 DEBUG: decisions object length:', Object.keys(decisions).length);
      
      state = { ...state, isLoading: true, error: null };

      try {
        const payload = { decisions };
        const url = `${config.apiUrl}/steam/import/${jobId}/decision`;
        
        console.log('🔧 DEBUG: API URL:', url);
        console.log('🔧 DEBUG: Payload:', JSON.stringify(payload, null, 2));
        console.log('🔧 DEBUG: Auth token present:', !!auth.value.accessToken);
        
        const response = await fetch(url, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify(payload)
        });

        console.log('🔧 DEBUG: Response status:', response.status);
        console.log('🔧 DEBUG: Response status text:', response.statusText);
        console.log('🔧 DEBUG: Response ok:', response.ok);

        if (!response.ok) {
          console.log('🔧 DEBUG: Response not ok, handling error');
          
          if (response.status === 401) {
            console.log('🔧 DEBUG: 401 response, refreshing auth');
            await auth.refreshAuth();
            return this.submitUserDecisions(jobId, decisions);
          }
          
          let errorData;
          let rawResponseText = '';
          try {
            rawResponseText = await response.text();
            console.log('🔧 DEBUG: Raw error response text:', rawResponseText);
            errorData = JSON.parse(rawResponseText);
            console.log('🔧 DEBUG: Error response data:', errorData);
          } catch (jsonError) {
            console.log('🔧 DEBUG: Failed to parse error response as JSON:', jsonError);
            console.log('🔧 DEBUG: Raw response was:', rawResponseText);
            errorData = {};
          }
          
          const errorMessage = errorData.detail || `Failed to submit decisions (${response.status}: ${response.statusText})`;
          console.error('🔧 DEBUG: Throwing error:', errorMessage);
          throw new Error(errorMessage);
        }

        console.log('🔧 DEBUG: Submission successful, clearing user decisions');
        // Clear user decisions after successful submission
        state = { ...state, userDecisions: {} };

      } catch (error) {
        console.error('🔧 DEBUG: submitUserDecisions caught error:', error);
        const errorMessage = error instanceof Error ? error.message : 'Failed to submit decisions';
        state = { ...state, error: errorMessage };
        throw error;
      } finally {
        state = { ...state, isLoading: false };
        console.log('🔧 DEBUG: submitUserDecisions finally block, isLoading set to false');
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
    updateGameStatus(steamAppId: number, status: SteamImportGameResponse['status'], data: any): void {
      if (!state.currentJob?.games) return;

      const gameIndex = state.currentJob.games.findIndex(g => g.steam_appid === steamAppId);
      if (gameIndex !== -1) {
        const updatedGames = [...state.currentJob.games];
        updatedGames[gameIndex] = { ...updatedGames[gameIndex], status, ...data };
        
        state = {
          ...state,
          currentJob: { ...state.currentJob, games: updatedGames },
          lastActivity: new Date()
        };
      }
    },

    clearError(): void {
      state = { ...state, error: null };
    }
  };

  return steamImportStore;
}

export const steamImport = createSteamImportStore();