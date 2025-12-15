import { config } from '$lib/env';
import { auth } from './auth.svelte';
import { ui } from './ui.svelte';
import { loggers } from '$lib/services/logger';
import type { GameId, UserGameId } from '$lib/types/game';

const log = loggers.steamGames;

// Steam Games API interfaces based on backend schemas
export interface SteamGameResponse {
  id: string; // Import record ID (remains as string UUID)
  external_id: string;
  name: string;
  igdb_id: GameId | null; // IGDB ID as integer
  igdb_title: string | null;
  game_id: GameId | null; // Game ID (same as igdb_id when matched)
  user_game_id: UserGameId | null; // User's game collection ID (UUID)
  ignored: boolean;
  created_at: string;
  updated_at: string;
}

export interface SteamGamesListResponse {
  total: number;
  games: SteamGameResponse[];
}

export interface SteamGamesImportStartedResponse {
  message: string;
  started: boolean;
}

export interface SteamGameMatchRequest {
  igdb_id: GameId | null;
}

export interface SteamGameMatchResponse {
  message: string;
  game: SteamGameResponse;
}

export interface SteamGameSyncResponse {
  message: string;
  game: SteamGameResponse;
  user_game_id: UserGameId;
  action: string;
}

export interface SteamGameIgnoreResponse {
  message: string;
  game: SteamGameResponse;
  ignored: boolean;
}

// Unified bulk operation response interface matching backend schema
export interface BulkOperationResponse {
  message: string;
  total_processed: number;
  successful_operations: number;
  failed_operations: number;
  skipped_items: number;
  errors: string[];
}

// Legacy interfaces for backward compatibility (will be replaced)
export interface SteamGamesBulkSyncResponse {
  message: string;
  total_processed: number;
  successful_syncs: number;
  failed_syncs: number;
  skipped_games: number;
  errors: string[];
}

export interface SteamGamesBulkUnignoreResponse {
  message: string;
  total_processed: number;
  successful_unignores: number;
  failed_unignores: number;
  errors: string[];
}

export interface SteamGamesBulkUnmatchResponse {
  message: string;
  total_processed: number;
  successful_unmatches: number;
  failed_unmatches: number;
  unsynced_games: number;
  errors: string[];
}

export interface SteamGamesBulkUnsyncResponse {
  message: string;
  total_processed: number;
  successful_unsyncs: number;
  failed_unsyncs: number;
  errors: string[];
}

export interface SteamGameUnsyncResponse {
  message: string;
  game: SteamGameResponse;
}

export interface SteamGamesAutoMatchResponse {
  message: string;
  total_processed: number;
  successful_matches: number;
  failed_matches: number;
  skipped_games: number;
  errors: string[];
}

export interface SteamGameAutoMatchSingleResponse {
  message: string;
  steam_game: SteamGameResponse;
  matched: boolean;
  confidence: number | null;
}

// Batch processing interfaces
export interface BatchSessionStartResponse {
  session_id: string;
  total_items: number;
  operation_type: string;
  status: string;
  message: string;
}

export interface BatchNextResponse {
  session_id: string;
  batch_processed: number;
  batch_successful: number;
  batch_failed: number;
  batch_errors: string[];
  current_batch_items: SteamGameResponse[];
  total_items: number;
  processed_items: number;
  successful_items: number;
  failed_items: number;
  remaining_items: number;
  progress_percentage: number;
  status: string;
  is_complete: boolean;
  message: string;
}

export interface BatchStatusResponse {
  session_id: string;
  operation_type: string;
  total_items: number;
  processed_items: number;
  successful_items: number;
  failed_items: number;
  remaining_items: number;
  progress_percentage: number;
  status: string;
  is_complete: boolean;
  created_at: string;
  updated_at: string;
  errors: string[];
}

export interface BatchCancelResponse {
  session_id: string;
  status: string;
  processed_items: number;
  successful_items: number;
  failed_items: number;
  message: string;
}

export interface BatchSessionState {
  sessionId: string;
  operationType: 'auto_match' | 'sync';
  totalItems: number;
  processedItems: number;
  successfulItems: number;
  failedItems: number;
  remainingItems: number;
  progressPercentage: number;
  status: string;
  isComplete: boolean;
  errors: string[];
  isProcessing: boolean;
}

export type SteamGameStatusFilter = 'unmatched' | 'matched' | 'ignored' | 'synced';

export interface SteamGamesState {
  games: SteamGameResponse[];
  total: number;
  isLoading: boolean;
  isImporting: boolean;
  isSyncing: boolean;
  isAutoMatching: boolean;
  isUnignoringAll: boolean;
  isUnmatchingAll: boolean;
  isUnsyncingAll: boolean;
  error: string | null;
  lastRefresh: Date | null;
  // Batch processing state
  activeBatchSession: BatchSessionState | null;
  isBatchProcessing: boolean;
}

const initialState: SteamGamesState = {
  games: [],
  total: 0,
  isLoading: false,
  isImporting: false,
  isSyncing: false,
  isAutoMatching: false,
  isUnignoringAll: false,
  isUnmatchingAll: false,
  isUnsyncingAll: false,
  error: null,
  lastRefresh: null,
  activeBatchSession: null,
  isBatchProcessing: false
};

function createSteamGamesStore() {
  let state = $state<SteamGamesState>(initialState);

  const steamGamesStore = {
    get value() {
      return state;
    },

    reset() {
      state = { ...initialState };
    },

    // List Steam games with filtering and pagination
    async listSteamGames(
      offset: number = 0,
      limit: number = 100,
      statusFilter?: SteamGameStatusFilter,
      search?: string
    ): Promise<SteamGamesListResponse> {
      log.debug('Starting listSteamGames', { offset, limit, statusFilter, search });
      
      state = { ...state, isLoading: true, error: null };

      try {
        const params = new URLSearchParams({
          offset: offset.toString(),
          limit: limit.toString()
        });

        if (statusFilter) {
          params.append('status_filter', statusFilter);
        }

        if (search && search.trim()) {
          params.append('search', search.trim());
        }

        const url = `${config.apiUrl}/import/sources/steam/games?${params}`;
        log.debug('Making API call', { url });

        const response = await fetch(url, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        log.debug('List API response', { status: response.status, ok: response.ok });

        if (!response.ok) {
          if (response.status === 401) {
            log.debug('Token expired, refreshing auth');
            await auth.refreshAuth();
            return this.listSteamGames(offset, limit, statusFilter, search);
          }
          const errorData = await response.json().catch(() => ({}));
          log.error('List API error', errorData);
          throw new Error(errorData.detail || 'Failed to fetch Steam games');
        }

        const data = await response.json() as SteamGamesListResponse;
        log.debug('List response received', { total: data.total, gamesCount: data.games.length });
        
        state = {
          ...state,
          games: data.games,
          total: data.total,
          isLoading: false,
          error: null,
          lastRefresh: new Date()
        };

        log.debug('Store state updated', { gamesCount: state.games.length, total: state.total });
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch Steam games';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Start Steam library import
    async importSteamLibrary(): Promise<SteamGamesImportStartedResponse> {
      state = { ...state, isImporting: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/import`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.importSteamLibrary();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to start Steam library import');
        }

        const data = await response.json() as SteamGamesImportStartedResponse;
        
        state = {
          ...state,
          isImporting: false,
          error: null
        };

        ui.showSuccess(data.message);
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start Steam library import';
        state = { ...state, isImporting: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Match Steam game to IGDB game
    async matchSteamGameToIGDB(steamGameId: string, igdbId: GameId | null): Promise<SteamGameMatchResponse> {
      state = { ...state, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/${steamGameId}/match`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({ igdb_id: igdbId ? Number(igdbId) : null })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.matchSteamGameToIGDB(steamGameId, igdbId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to match Steam game to IGDB');
        }

        const data = await response.json() as SteamGameMatchResponse;
        
        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.game;
          state = { ...state, games: updatedGames };
        }

        ui.showSuccess(data.message);
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to match Steam game';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Sync individual Steam game to collection
    async syncSteamGameToCollection(steamGameId: string): Promise<SteamGameSyncResponse> {
      log.debug('Starting sync for Steam game', { steamGameId });
      state = { ...state, error: null };

      try {
        const url = `${config.apiUrl}/import/sources/steam/games/${steamGameId}/sync`;
        const requestBody = {};

        const response = await fetch(url, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify(requestBody)
        });

        log.debug('Sync response', { status: response.status, statusText: response.statusText });

        if (!response.ok) {
          const responseText = await response.text();

          if (response.status === 401) {
            log.debug('Auth token expired, refreshing');
            await auth.refreshAuth();
            return this.syncSteamGameToCollection(steamGameId);
          }

          let errorData;
          try {
            errorData = JSON.parse(responseText);
          } catch (e) {
            errorData = { detail: responseText || 'Failed to sync Steam game to collection' };
          }

          log.error('Sync error', errorData);
          throw new Error(errorData.detail || 'Failed to sync Steam game to collection');
        }

        const data = await response.json() as SteamGameSyncResponse;
        log.debug('Sync success', { steamGameId, action: data.action });

        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.game;
          state = { ...state, games: updatedGames };
        }

        ui.showSuccess(data.message);
        return data;
      } catch (error) {
        log.error('Sync failed', error);
        const errorMessage = error instanceof Error ? error.message : 'Failed to sync Steam game';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Sync all matched Steam games
    async syncAllMatchedGames(): Promise<BulkOperationResponse> {
      state = { ...state, isSyncing: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/sync`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.syncAllMatchedGames();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to sync Steam games');
        }

        const data = await response.json() as BulkOperationResponse;
        
        state = {
          ...state,
          isSyncing: false,
          error: null
        };

        if (data.successful_operations > 0) {
          ui.showSuccess(data.message);
        } else if (data.total_processed === 0) {
          ui.showInfo(data.message);
        } else {
          ui.showWarning(data.message);
        }

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to sync Steam games';
        state = { ...state, isSyncing: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Manually retry auto-matching for all unmatched Steam games
    async retryAutoMatching(): Promise<BulkOperationResponse> {
      log.debug('Starting retryAutoMatching');
      state = { ...state, isAutoMatching: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/auto-match`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        log.debug('Auto-match API response', { status: response.status, ok: response.ok });

        if (!response.ok) {
          if (response.status === 401) {
            log.debug('Token expired, refreshing auth');
            await auth.refreshAuth();
            return this.retryAutoMatching();
          }
          const errorData = await response.json().catch(() => ({}));
          log.error('Auto-match API error', errorData);
          throw new Error(errorData.detail || 'Failed to retry auto-matching');
        }

        const data = await response.json() as BulkOperationResponse;
        log.debug('Auto-match response', { successful: data.successful_operations, total: data.total_processed });

        state = {
          ...state,
          isAutoMatching: false,
          error: null
        };

        if (data.successful_operations > 0) {
          ui.showSuccess(data.message);
        } else if (data.total_processed === 0) {
          ui.showInfo(data.message);
        } else {
          ui.showWarning(data.message);
        }

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to retry auto-matching';
        state = { ...state, isAutoMatching: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Auto-match a single Steam game to IGDB
    async autoMatchSingleGame(steamGameId: string): Promise<SteamGameAutoMatchSingleResponse> {
      log.debug('Starting autoMatchSingleGame', { steamGameId });
      state = { ...state, error: null };

      try {
        const url = `${config.apiUrl}/import/sources/steam/games/${steamGameId}/auto-match`;

        const response = await fetch(url, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        log.debug('Single auto-match API response', { status: response.status, ok: response.ok });

        if (!response.ok) {
          if (response.status === 401) {
            log.debug('Token expired, refreshing auth');
            await auth.refreshAuth();
            return this.autoMatchSingleGame(steamGameId);
          }
          const errorData = await response.json().catch(() => ({}));
          log.error('Single auto-match API error', errorData);
          throw new Error(errorData.detail || 'Failed to auto-match Steam game');
        }

        const data = await response.json() as SteamGameAutoMatchSingleResponse;
        log.debug('Single auto-match response', { matched: data.matched, confidence: data.confidence });

        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);

        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.steam_game;
          state = { ...state, games: updatedGames };
        } else {
          log.warn('Game not found in state for update', { steamGameId });
        }

        // Show appropriate success/info message
        if (data.matched) {
          ui.showSuccess(data.message);
        } else {
          ui.showInfo(data.message);
        }

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to auto-match Steam game';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Toggle Steam game ignored status
    async toggleSteamGameIgnored(steamGameId: string): Promise<SteamGameIgnoreResponse> {
      state = { ...state, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/${steamGameId}/ignore`, {
          method: 'PUT',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.toggleSteamGameIgnored(steamGameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to toggle Steam game ignored status');
        }

        const data = await response.json() as SteamGameIgnoreResponse;
        
        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.game;
          state = { ...state, games: updatedGames };
        }

        ui.showSuccess(data.message);
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to toggle ignored status';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Unignore all Steam games
    async unignoreAllGames(): Promise<BulkOperationResponse> {
      state = { ...state, isUnignoringAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/unignore-all`, {
          method: 'PUT',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.unignoreAllGames();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to unignore all Steam games');
        }

        const data = await response.json() as BulkOperationResponse;
        
        state = { ...state, isUnignoringAll: false, error: null };
        
        ui.showSuccess(data.message || 'All games have been unignored successfully');
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unignore all games';
        state = { ...state, error: errorMessage, isUnignoringAll: false };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Unmatch all Steam games
    async unmatchAllGames(): Promise<BulkOperationResponse> {
      state = { ...state, isUnmatchingAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/unmatch-all`, {
          method: 'PUT',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.unmatchAllGames();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to unmatch all Steam games');
        }

        const data = await response.json() as BulkOperationResponse;
        
        state = { ...state, isUnmatchingAll: false, error: null };
        
        ui.showSuccess(data.message || 'All matched games have been unmatched successfully');
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unmatch all games';
        state = { ...state, error: errorMessage, isUnmatchingAll: false };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Unsync individual Steam game from collection
    async unsyncSteamGameFromCollection(steamGameId: string): Promise<SteamGameUnsyncResponse> {
      state = { ...state, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/${steamGameId}/unsync`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.unsyncSteamGameFromCollection(steamGameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to unsync Steam game from collection');
        }

        const data = await response.json() as SteamGameUnsyncResponse;
        
        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.game;
          state = { ...state, games: updatedGames };
        }

        ui.showSuccess(data.message);
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unsync Steam game';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Unsync all Steam games from collection
    async unsyncAllGames(): Promise<BulkOperationResponse> {
      state = { ...state, isUnsyncingAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/games/unsync`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.unsyncAllGames();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to unsync all Steam games');
        }

        const data = await response.json() as BulkOperationResponse;
        
        state = { ...state, isUnsyncingAll: false, error: null };
        
        ui.showSuccess(data.message || 'All synced games have been unsynced successfully');
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unsync all games';
        state = { ...state, error: errorMessage, isUnsyncingAll: false };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // ===== BATCH PROCESSING METHODS =====

    // Start batch auto-matching session
    async startBatchAutoMatch(): Promise<BatchSessionStartResponse> {
      log.debug('Starting batch auto-match session');
      state = { ...state, error: null, isBatchProcessing: true };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/batch/auto-match/start`, {
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
            return this.startBatchAutoMatch();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to start batch auto-match session');
        }

        const data = await response.json() as BatchSessionStartResponse;
        
        // Initialize batch session state
        if (data.session_id) {
          state = {
            ...state,
            activeBatchSession: {
              sessionId: data.session_id,
              operationType: 'auto_match',
              totalItems: data.total_items,
              processedItems: 0,
              successfulItems: 0,
              failedItems: 0,
              remainingItems: data.total_items,
              progressPercentage: 0,
              status: data.status,
              isComplete: false,
              errors: [],
              isProcessing: false
            }
          };
        }

        ui.showSuccess(data.message);
        log.debug('Batch auto-match session started', { sessionId: data.session_id, totalItems: data.total_items });
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start batch auto-match session';
        state = { ...state, error: errorMessage, isBatchProcessing: false };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Process next batch in auto-matching session
    async processBatchAutoMatch(sessionId: string): Promise<BatchNextResponse> {
      log.debug('Processing next auto-match batch', { sessionId });
      
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/batch/auto-match/${sessionId}/next`, {
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
            return this.processBatchAutoMatch(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to process batch auto-match');
        }

        const data = await response.json() as BatchNextResponse;
        
        // Update batch session state
        if (state.activeBatchSession) {
          state = {
            ...state,
            activeBatchSession: {
              ...state.activeBatchSession,
              processedItems: data.processed_items,
              successfulItems: data.successful_items,
              failedItems: data.failed_items,
              remainingItems: data.remaining_items,
              progressPercentage: data.progress_percentage,
              status: data.status,
              isComplete: data.is_complete,
              errors: data.current_batch_items ? [...state.activeBatchSession.errors] : [...state.activeBatchSession.errors, ...data.batch_errors],
              isProcessing: !data.is_complete
            }
          };
        }

        log.debug('Batch auto-match processed', { successful: data.batch_successful, failed: data.batch_failed });
        return data;
      } catch (error) {
        log.error('Error processing batch auto-match', error);
        const errorMessage = error instanceof Error ? error.message : 'Failed to process batch auto-match';
        
        // Update error state
        state = { 
          ...state, 
          error: errorMessage,
          isBatchProcessing: false,
          activeBatchSession: state.activeBatchSession ? {
            ...state.activeBatchSession,
            isProcessing: false,
            status: 'failed',
            isComplete: true,
            errors: [...state.activeBatchSession.errors, errorMessage]
          } : null
        };
        
        throw error;
      }
    },

    // Start batch sync session
    async startBatchSync(): Promise<BatchSessionStartResponse> {
      log.debug('Starting batch sync session');
      state = { ...state, error: null, isBatchProcessing: true };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/batch/sync/start`, {
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
            return this.startBatchSync();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to start batch sync session');
        }

        const data = await response.json() as BatchSessionStartResponse;
        
        // Initialize batch session state
        if (data.session_id) {
          state = {
            ...state,
            activeBatchSession: {
              sessionId: data.session_id,
              operationType: 'sync',
              totalItems: data.total_items,
              processedItems: 0,
              successfulItems: 0,
              failedItems: 0,
              remainingItems: data.total_items,
              progressPercentage: 0,
              status: data.status,
              isComplete: false,
              errors: [],
              isProcessing: false
            }
          };
        }

        ui.showSuccess(data.message);
        log.debug('Batch sync session started', { sessionId: data.session_id, totalItems: data.total_items });
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start batch sync session';
        state = { ...state, error: errorMessage, isBatchProcessing: false };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Process next batch in sync session
    async processBatchSync(sessionId: string): Promise<BatchNextResponse> {
      log.debug('Processing next sync batch', { sessionId });
      
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/batch/sync/${sessionId}/next`, {
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
            return this.processBatchSync(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to process batch sync');
        }

        const data = await response.json() as BatchNextResponse;
        
        // Update batch session state
        if (state.activeBatchSession) {
          state = {
            ...state,
            activeBatchSession: {
              ...state.activeBatchSession,
              processedItems: data.processed_items,
              successfulItems: data.successful_items,
              failedItems: data.failed_items,
              remainingItems: data.remaining_items,
              progressPercentage: data.progress_percentage,
              status: data.status,
              isComplete: data.is_complete,
              errors: data.current_batch_items ? [...state.activeBatchSession.errors] : [...state.activeBatchSession.errors, ...data.batch_errors],
              isProcessing: !data.is_complete
            }
          };
        }

        log.debug('Batch sync processed', { successful: data.batch_successful, failed: data.batch_failed });
        return data;
      } catch (error) {
        log.error('Error processing batch sync', error);
        const errorMessage = error instanceof Error ? error.message : 'Failed to process batch sync';
        
        // Update error state
        state = { 
          ...state, 
          error: errorMessage,
          isBatchProcessing: false,
          activeBatchSession: state.activeBatchSession ? {
            ...state.activeBatchSession,
            isProcessing: false,
            status: 'failed',
            isComplete: true,
            errors: [...state.activeBatchSession.errors, errorMessage]
          } : null
        };
        
        throw error;
      }
    },

    // Cancel batch operation
    async cancelBatchOperation(sessionId: string): Promise<BatchCancelResponse> {
      log.debug('Cancelling batch session', { sessionId });

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/steam/batch/${sessionId}`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.cancelBatchOperation(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to cancel batch operation');
        }

        const data = await response.json() as BatchCancelResponse;
        
        // Clear batch session state
        state = { 
          ...state, 
          activeBatchSession: null,
          isBatchProcessing: false
        };

        ui.showSuccess(data.message);
        log.debug('Batch session cancelled', { sessionId });
        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to cancel batch operation';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Get batch status
    async getBatchStatus(sessionId: string): Promise<BatchStatusResponse> {
      try {
        const operationType = state.activeBatchSession?.operationType || 'auto_match';
        const endpoint = operationType === 'auto_match'
          ? `${config.apiUrl}/import/jobs?source=steam&type=auto_match`
          : `${config.apiUrl}/import/jobs?source=steam&type=bulk_sync`;

        const response = await fetch(endpoint, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.getBatchStatus(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get batch status');
        }

        const data = await response.json() as BatchStatusResponse;
        
        // Update batch session state if it matches current session
        if (state.activeBatchSession && state.activeBatchSession.sessionId === sessionId) {
          state.activeBatchSession = {
            ...state.activeBatchSession,
            processedItems: data.processed_items,
            successfulItems: data.successful_items,
            failedItems: data.failed_items,
            remainingItems: data.remaining_items,
            progressPercentage: data.progress_percentage,
            status: data.status,
            isComplete: data.is_complete,
            errors: data.errors
          };
          state = { ...state, activeBatchSession: { ...state.activeBatchSession } };
        }

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get batch status';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Clear batch session
    clearBatchSession() {
      state = { 
        ...state, 
        activeBatchSession: null,
        isBatchProcessing: false
      };
    },

    // Clear error state
    clearError() {
      state = { ...state, error: null };
    },

    // Refresh data manually
    async refresh(statusFilter?: SteamGameStatusFilter, search?: string) {
      await this.listSteamGames(0, 100, statusFilter, search);
    }
  };

  return steamGamesStore;
}

export const steamGames = createSteamGamesStore();