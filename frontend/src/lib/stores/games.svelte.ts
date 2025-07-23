import { auth } from './auth.svelte';
import { config } from '$lib/env';

export interface Game {
  id: string;
  title: string;
  description?: string;
  genre?: string;
  developer?: string;
  publisher?: string;
  release_date?: string;
  cover_art_url?: string;
  rating_average?: number;
  rating_count: number;
  game_metadata: string;
  estimated_playtime_hours?: number;
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
  igdb_id?: string;
  is_verified: boolean;
  created_at: string;
  updated_at: string;
}

export interface GameSearchFilters {
  q?: string;
  genre?: string;
  developer?: string;
  publisher?: string;
  release_year?: number;
  is_verified?: boolean;
}

export interface GameListResponse {
  games: Game[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface IGDBGameCandidate {
  igdb_id: string;
  title: string;
  release_date?: string;
  cover_art_url?: string;
  description?: string;
  platforms: string[];
  howlongtobeat_main?: number;
  howlongtobeat_extra?: number;
  howlongtobeat_completionist?: number;
}

export interface IGDBSearchResponse {
  games: IGDBGameCandidate[];
  total: number;
}

export interface GamesState {
  games: Game[];
  currentGame: Game | null;
  searchResults: Game[];
  igdbCandidates: IGDBGameCandidate[];
  isLoading: boolean;
  error: string | null;
  filters: GameSearchFilters;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    pages: number;
  };
}

const initialState: GamesState = {
  games: [],
  currentGame: null,
  searchResults: [],
  igdbCandidates: [],
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

function createGamesStore() {
  let state = $state<GamesState>(initialState);

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

  return {
    get value() {
      return state;
    },

    // Load games with search and pagination
    loadGames: async (filters: GameSearchFilters = {}, page: number = 1, per_page: number = 20) => {
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

        const response = await apiCall(`${config.apiUrl}/games?${params}`);
        const data: GameListResponse = await response.json();

        state = {
          ...state,
          games: data.games,
          searchResults: data.games,
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
        const errorMessage = error instanceof Error ? error.message : 'Failed to load games';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Get a specific game by ID
    getGame: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games/${id}`);
        const game: Game = await response.json();

        state = {
          ...state,
          currentGame: game,
          isLoading: false
        };

        return game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Search IGDB for game candidates
    searchIGDB: async (title: string, limit: number = 10) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games/search/igdb`, {
          method: 'POST',
          body: JSON.stringify({ query: title, limit }),
        });
        
        const data: IGDBSearchResponse = await response.json();

        state = {
          ...state,
          igdbCandidates: data.games,
          isLoading: false
        };

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to search IGDB';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new game from IGDB metadata
    createFromIGDB: async (igdb_id: string, custom_overrides: Record<string, any> = {}) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games/igdb-import`, {
          method: 'POST',
          body: JSON.stringify({
            igdb_id,
            custom_overrides
          }),
        });
        
        const game: Game = await response.json();

        // Add the new game to the current games list
        state = {
          ...state,
          games: [game, ...state.games],
          currentGame: game,
          isLoading: false
        };

        return game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create game from IGDB';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Create a new game manually
    createGame: async (gameData: Omit<Game, 'id' | 'created_at' | 'updated_at' | 'rating_count' | 'is_verified'>) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games`, {
          method: 'POST',
          body: JSON.stringify(gameData),
        });
        
        const game: Game = await response.json();

        state = {
          ...state,
          games: [game, ...state.games],
          currentGame: game,
          isLoading: false
        };

        return game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to create game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Update an existing game
    updateGame: async (id: string, gameData: Partial<Game>) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games/${id}`, {
          method: 'PUT',
          body: JSON.stringify(gameData),
        });
        
        const updatedGame: Game = await response.json();

        // Update the game in the current games list
        state = {
          ...state,
          games: state.games.map(game => 
            game.id === id ? updatedGame : game
          ),
          currentGame: state.currentGame?.id === id ? updatedGame : state.currentGame,
          isLoading: false
        };

        return updatedGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Delete a game
    deleteGame: async (id: string) => {
      state = { ...state, isLoading: true, error: null };

      try {
        await apiCall(`${config.apiUrl}/games/${id}`, {
          method: 'DELETE',
        });

        // Remove the game from the current games list
        state = {
          ...state,
          games: state.games.filter(game => game.id !== id),
          currentGame: state.currentGame?.id === id ? null : state.currentGame,
          isLoading: false
        };
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete game';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Refresh metadata for a game
    refreshMetadata: async (id: string, fields?: string[], force: boolean = false) => {
      state = { ...state, isLoading: true, error: null };

      try {
        const response = await apiCall(`${config.apiUrl}/games/${id}/metadata/refresh`, {
          method: 'POST',
          body: JSON.stringify({ fields, force }),
        });
        
        const result = await response.json();

        // Update the game in the current games list
        state = {
          ...state,
          games: state.games.map(game => 
            game.id === id ? result.game : game
          ),
          currentGame: state.currentGame?.id === id ? result.game : state.currentGame,
          isLoading: false
        };

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to refresh metadata';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    // Clear search results
    clearSearch: () => {
      state = {
        ...state,
        searchResults: [],
        igdbCandidates: [],
        filters: {}
      };
    },

    // Clear current game
    clearCurrentGame: () => {
      state = { ...state, currentGame: null };
    },

    // Clear error
    clearError: () => {
      state = { ...state, error: null };
    }
  };
}

export const games = createGamesStore();