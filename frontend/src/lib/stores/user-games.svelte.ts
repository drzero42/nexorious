import { auth } from './auth.svelte';
import type { Game } from './games.svelte';
import type { Platform, Storefront } from './platforms.svelte';
import { config } from '$lib/env';

export enum OwnershipStatus {
  OWNED = 'owned',
  BORROWED = 'borrowed',
  RENTED = 'rented',
  SUBSCRIPTION = 'subscription'
}

export enum PlayStatus {
  NOT_STARTED = 'not_started',
  IN_PROGRESS = 'in_progress',
  COMPLETED = 'completed',
  MASTERED = 'mastered',
  DOMINATED = 'dominated',
  SHELVED = 'shelved',
  DROPPED = 'dropped',
  REPLAY = 'replay'
}

export interface UserGamePlatform {
  id: string;
  platform: Platform;
  storefront?: Storefront;
  store_game_id?: string;
  store_url?: string;
  is_available: boolean;
  created_at: string;
}

export interface UserGame {
  id: string;
  game: Game;
  ownership_status: OwnershipStatus;
  is_physical: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved: boolean;
  play_status: PlayStatus;
  hours_played: number;
  personal_notes?: string;
  acquired_date?: string;
  last_played?: string;
  platforms: UserGamePlatform[];
  created_at: string;
  updated_at: string;
}

export interface UserGameCreateRequest {
  game_id: string;
  ownership_status?: OwnershipStatus;
  is_physical?: boolean;
  physical_location?: string;
  acquired_date?: string;
  platforms?: string[];
}

export interface UserGameUpdateRequest {
  ownership_status?: OwnershipStatus;
  is_physical?: boolean;
  physical_location?: string;
  personal_rating?: number | null;
  is_loved?: boolean;
  acquired_date?: string;
}

export interface ProgressUpdateRequest {
  play_status: PlayStatus;
  hours_played?: number;
  personal_notes?: string;
  last_played?: string;
}

export interface UserGamePlatformCreateRequest {
  platform_id: string;
  storefront_id?: string;
  store_game_id?: string;
  store_url?: string;
}

export interface UserGameFilters {
  play_status?: PlayStatus;
  ownership_status?: OwnershipStatus;
  is_loved?: boolean;
  platform_id?: string;
  storefront_id?: string;
  rating_min?: number;
  rating_max?: number;
  has_notes?: boolean;
}

export interface UserGameListResponse {
  user_games: UserGame[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface BulkStatusUpdateRequest {
  user_game_ids: string[];
  play_status?: PlayStatus;
  personal_rating?: number | null;
  is_loved?: boolean;
}

export interface CollectionStats {
  total_games: number;
  by_status: Record<PlayStatus, number>;
  by_platform: Record<string, number>;
  by_rating: Record<string, number>;
  pile_of_shame: number;
  completion_rate: number;
  average_rating?: number;
  total_hours_played: number;
}

export interface UserGamesState {
  userGames: UserGame[];
  currentUserGame: UserGame | null;
  stats: CollectionStats | null;
  isLoading: boolean;
  error: string | null;
  filters: UserGameFilters;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    pages: number;
  };
}

const initialState: UserGamesState = {
  userGames: [],
  currentUserGame: null,
  stats: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    per_page: 20,
    total: 0,
    pages: 0
  }
};

function createUserGamesStore() {
  let state = $state<UserGamesState>(initialState);

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authState.accessToken}`,
        ...options.headers,
      },
    });

    if (!response.ok) {
      if (response.status === 401) {
        // Try to refresh token
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          // Retry the request with new token
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              'Authorization': `Bearer ${auth.value.accessToken}`,
              ...options.headers,
            },
          });
        }
      }
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    return response;
  };

  const store = {
    get value() {
      return state;
    },

    // Load user's game collection
    loadUserGames: async (filters: UserGameFilters = {}, page: number = 1, per_page: number = 20) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const params = new URLSearchParams();
        
        // Add filters
        Object.entries(filters).forEach(([key, value]) => {
          if (value !== undefined && value !== null && value !== '') {
            params.append(key, value.toString());
          }
        });
        
        params.append('page', page.toString());
        params.append('per_page', per_page.toString());

        const response = await apiCall(`${config.apiUrl}/user-games?${params}`);
        const data: UserGameListResponse = await response.json();

        state = {
          ...state,
          userGames: data.user_games,
          filters,
          pagination: {
            page: data.page,
            per_page: data.per_page,
            total: data.total,
            pages: data.pages
          },
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load user games';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get a specific user game by ID
    getUserGame: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}`);
        const userGame: UserGame = await response.json();

        state = {
          ...state,
          currentUserGame: userGame,
          isLoading: false
        };

        return userGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load user game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Add a game to user's collection
    addGameToCollection: async (gameData: UserGameCreateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games`, {
          method: 'POST',
          body: JSON.stringify(gameData),
        });
        
        const userGame: UserGame = await response.json();

        state = {
          ...state,
          userGames: [userGame, ...state.userGames],
          currentUserGame: userGame,
          isLoading: false
        };

        return userGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to add game to collection';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update user game details
    updateUserGame: async (id: string, gameData: UserGameUpdateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}`, {
          method: 'PUT',
          body: JSON.stringify(gameData),
        });
        
        const updatedUserGame: UserGame = await response.json();

        state = {
          ...state,
          userGames: state.userGames.map(userGame => 
            userGame.id === id ? updatedUserGame : userGame
          ),
          currentUserGame: state.currentUserGame?.id === id ? updatedUserGame : state.currentUserGame,
          isLoading: false
        };

        return updatedUserGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update user game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update game progress
    updateProgress: async (id: string, progressData: ProgressUpdateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}/progress`, {
          method: 'PUT',
          body: JSON.stringify(progressData),
        });
        
        const updatedUserGame: UserGame = await response.json();

        state = {
          ...state,
          userGames: state.userGames.map(userGame => 
            userGame.id === id ? updatedUserGame : userGame
          ),
          currentUserGame: state.currentUserGame?.id === id ? updatedUserGame : state.currentUserGame,
          isLoading: false
        };

        return updatedUserGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update progress';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Remove game from collection
    removeFromCollection: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`${config.apiUrl}/user-games/${id}`, {
          method: 'DELETE',
        });

        state = {
          ...state,
          userGames: state.userGames.filter(userGame => userGame.id !== id),
          currentUserGame: state.currentUserGame?.id === id ? null : state.currentUserGame,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove game from collection';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Add platform to user game
    addPlatformToUserGame: async (userGameId: string, platformData: UserGamePlatformCreateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${userGameId}/platforms`, {
          method: 'POST',
          body: JSON.stringify(platformData),
        });
        
        const updatedUserGame: UserGame = await response.json();

        state = {
          ...state,
          userGames: state.userGames.map(userGame => 
            userGame.id === userGameId ? updatedUserGame : userGame
          ),
          currentUserGame: state.currentUserGame?.id === userGameId ? updatedUserGame : state.currentUserGame,
          isLoading: false
        };

        return updatedUserGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to add platform to user game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Remove platform from user game
    removePlatformFromUserGame: async (userGameId: string, platformId: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`${config.apiUrl}/user-games/${userGameId}/platforms/${platformId}`, {
          method: 'DELETE',
        });

        // Update the user game in the state
        state = {
          ...state,
          userGames: state.userGames.map(userGame => 
            userGame.id === userGameId 
              ? { ...userGame, platforms: userGame.platforms.filter(p => p.id !== platformId) }
              : userGame
          ),
          currentUserGame: state.currentUserGame?.id === userGameId 
            ? { ...state.currentUserGame, platforms: state.currentUserGame.platforms.filter(p => p.id !== platformId) }
            : state.currentUserGame,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove platform from user game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Bulk update status
    bulkUpdateStatus: async (data: BulkStatusUpdateRequest) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/bulk-update`, {
          method: 'POST',
          body: JSON.stringify(data),
        });
        
        const updatedUserGames: UserGame[] = await response.json();

        // Update the affected user games in the state
        state = {
          ...state,
          userGames: state.userGames.map(userGame => {
            const updated = updatedUserGames.find(updated => updated.id === userGame.id);
            return updated || userGame;
          }),
          isLoading: false
        };

        return updatedUserGames;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk update status';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get collection statistics
    getCollectionStats: async () => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/stats`);
        const stats: CollectionStats = await response.json();

        state = {
          ...state,
          stats,
          isLoading: false
        };

        return stats;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get collection stats';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get games by play status
    getGamesByStatus: (status: PlayStatus) => {
      return state.userGames.filter(userGame => userGame.play_status === status);
    },

    // Get loved games
    getLovedGames: () => {
      return state.userGames.filter(userGame => userGame.is_loved);
    },

    // Get games by rating
    getGamesByRating: (rating: number) => {
      return state.userGames.filter(userGame => userGame.personal_rating === rating);
    },

    // Get pile of shame (not started games)
    getPileOfShame: () => {
      return state.userGames.filter(userGame => userGame.play_status === PlayStatus.NOT_STARTED);
    },

    // Clear current user game
    clearCurrentUserGame: () => {
      state = { ...state, currentUserGame: null };
    },

    // Clear filters
    clearFilters: () => {
      state = { ...state, filters: {} };
    },

    // Clear error
    clearError: () => {
      state = { ...state, error: null };
    },

    // Alias for loadUserGames for backward compatibility
    fetchUserGames: async (filters: UserGameFilters = {}, page: number = 1, per_page: number = 20) => {
      return await store.loadUserGames(filters, page, per_page);
    }
  };

  return store;
}

export const userGames = createUserGamesStore();