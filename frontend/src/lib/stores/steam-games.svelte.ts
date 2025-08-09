import { config } from '$lib/env';
import { auth } from './auth.svelte';
import { ui } from './ui.svelte';

// Steam Games API interfaces based on backend schemas
export interface SteamGameResponse {
  id: string;
  steam_appid: number;
  game_name: string;
  igdb_id: string | null;
  game_id: string | null;
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
  igdb_id: string | null;
}

export interface SteamGameMatchResponse {
  message: string;
  steam_game: SteamGameResponse;
}

export interface SteamGameSyncResponse {
  message: string;
  steam_game: SteamGameResponse;
  user_game_id: string;
  action: string;
}

export interface SteamGameIgnoreResponse {
  message: string;
  steam_game: SteamGameResponse;
  ignored: boolean;
}

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

export type SteamGameStatusFilter = 'unmatched' | 'matched' | 'ignored' | 'synced';

export interface SteamGamesState {
  games: SteamGameResponse[];
  total: number;
  isLoading: boolean;
  isImporting: boolean;
  isSyncing: boolean;
  isAutoMatching: boolean;
  isUnignoringAll: boolean;
  error: string | null;
  lastRefresh: Date | null;
}

const initialState: SteamGamesState = {
  games: [],
  total: 0,
  isLoading: false,
  isImporting: false,
  isSyncing: false,
  isAutoMatching: false,
  isUnignoringAll: false,
  error: null,
  lastRefresh: null
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

        const response = await fetch(`${config.apiUrl}/steam-games?${params}`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.listSteamGames(offset, limit, statusFilter, search);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to fetch Steam games');
        }

        const data = await response.json() as SteamGamesListResponse;
        
        state = {
          ...state,
          games: data.games,
          total: data.total,
          isLoading: false,
          error: null,
          lastRefresh: new Date()
        };

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
        const response = await fetch(`${config.apiUrl}/steam-games/import`, {
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
    async matchSteamGameToIGDB(steamGameId: string, igdbId: string | null): Promise<SteamGameMatchResponse> {
      state = { ...state, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam-games/${steamGameId}/match`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({ igdb_id: igdbId })
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
          updatedGames[gameIndex] = data.steam_game;
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
      console.log('🎮 [Steam Sync] Starting sync for Steam game:', steamGameId);
      state = { ...state, error: null };

      try {
        const url = `${config.apiUrl}/steam-games/${steamGameId}/sync`;
        const requestBody = {};
        console.log('🎮 [Steam Sync] Request URL:', url);
        console.log('🎮 [Steam Sync] Request body:', requestBody);
        console.log('🎮 [Steam Sync] Auth token available:', !!auth.value.accessToken);
        console.log('🎮 [Steam Sync] Auth token prefix:', auth.value.accessToken?.substring(0, 20) + '...');

        const response = await fetch(url, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify(requestBody)
        });

        console.log('🎮 [Steam Sync] Response status:', response.status, response.statusText);
        console.log('🎮 [Steam Sync] Response headers:', Object.fromEntries(response.headers.entries()));

        if (!response.ok) {
          const responseText = await response.text();
          console.log('🎮 [Steam Sync] Error response body:', responseText);
          
          if (response.status === 401) {
            console.log('🎮 [Steam Sync] Auth token expired, refreshing...');
            await auth.refreshAuth();
            return this.syncSteamGameToCollection(steamGameId);
          }
          
          let errorData;
          try {
            errorData = JSON.parse(responseText);
          } catch (e) {
            errorData = { detail: responseText || 'Failed to sync Steam game to collection' };
          }
          
          console.log('🎮 [Steam Sync] Parsed error data:', errorData);
          throw new Error(errorData.detail || 'Failed to sync Steam game to collection');
        }

        const data = await response.json() as SteamGameSyncResponse;
        console.log('🎮 [Steam Sync] Success response data:', data);
        
        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.steam_game;
          state = { ...state, games: updatedGames };
          console.log('🎮 [Steam Sync] Updated local state for game at index:', gameIndex);
        } else {
          console.log('🎮 [Steam Sync] Game not found in local state:', steamGameId);
        }

        ui.showSuccess(data.message);
        console.log('🎮 [Steam Sync] Sync completed successfully');
        return data;
      } catch (error) {
        console.log('🎮 [Steam Sync] Error caught:', error);
        const errorMessage = error instanceof Error ? error.message : 'Failed to sync Steam game';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Sync all matched Steam games
    async syncAllMatchedGames(): Promise<SteamGamesBulkSyncResponse> {
      state = { ...state, isSyncing: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam-games/sync`, {
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

        const data = await response.json() as SteamGamesBulkSyncResponse;
        
        state = {
          ...state,
          isSyncing: false,
          error: null
        };

        if (data.successful_syncs > 0) {
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
    async retryAutoMatching(): Promise<SteamGamesAutoMatchResponse> {
      state = { ...state, isAutoMatching: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam-games/auto-match`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.retryAutoMatching();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to retry auto-matching');
        }

        const data = await response.json() as SteamGamesAutoMatchResponse;
        
        state = {
          ...state,
          isAutoMatching: false,
          error: null
        };

        if (data.successful_matches > 0) {
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
      state = { ...state, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam-games/${steamGameId}/auto-match`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.autoMatchSingleGame(steamGameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to auto-match Steam game');
        }

        const data = await response.json() as SteamGameAutoMatchSingleResponse;
        
        // Update the game in our local state
        const gameIndex = state.games.findIndex(g => g.id === steamGameId);
        if (gameIndex !== -1) {
          const updatedGames = [...state.games];
          updatedGames[gameIndex] = data.steam_game;
          state = { ...state, games: updatedGames };
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
        const response = await fetch(`${config.apiUrl}/steam-games/${steamGameId}/ignore`, {
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
          updatedGames[gameIndex] = data.steam_game;
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
    async unignoreAllGames(): Promise<SteamGamesBulkUnignoreResponse> {
      state = { ...state, isUnignoringAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/steam-games/unignore-all`, {
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

        const data = await response.json() as SteamGamesBulkUnignoreResponse;
        
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