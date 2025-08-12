import { auth } from './auth.svelte';
import type { Game } from './games.svelte';
import type { Platform, Storefront } from './platforms.svelte';
import { config } from '$lib/env';

// Enhanced TypeScript patterns for fine-grained reactivity
// type DeepPartial<T> = {
//   [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
// };

// Event system for cross-view synchronization
class GameCollectionEventBus {
  private listeners = new Map<string, Set<Function>>();

  emit(event: string, data: any) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      callbacks.forEach(callback => callback(data));
    }
  }

  on(event: string, callback: Function) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(callback);
  }

  off(event: string, callback: Function) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      callbacks.delete(callback);
    }
  }
}

export const gameEventBus = new GameCollectionEventBus();

// Optimistic update management
interface OptimisticUpdate<T> {
  id: string;
  optimistic: T;
  original: T;
  timestamp: number;
}

interface OptimisticState<T extends { id: string }> {
  pending: Map<string, OptimisticUpdate<T>>;
  rollbacks: Map<string, () => void>;
}

class OptimisticUpdates<T extends { id: string }> {
  private state = $state<OptimisticState<T>>({
    pending: new Map(),
    rollbacks: new Map()
  });

  apply(
    id: string, 
    optimisticData: Partial<T>,
    original: T,
    rollbackFn: () => void
  ): T {
    const update: OptimisticUpdate<T> = {
      id,
      optimistic: { ...original, ...optimisticData } as T,
      original,
      timestamp: Date.now()
    };

    this.state.pending.set(id, update);
    this.state.rollbacks.set(id, rollbackFn);
    
    return update.optimistic;
  }

  commit(id: string): void {
    this.state.pending.delete(id);
    this.state.rollbacks.delete(id);
  }

  rollback(id: string): void {
    const rollbackFn = this.state.rollbacks.get(id);
    
    if (rollbackFn) {
      rollbackFn();
      this.state.pending.delete(id);
      this.state.rollbacks.delete(id);
    }
  }

  rollbackAll(): void {
    for (const [id] of this.state.pending) {
      this.rollback(id);
    }
  }

  get isPending() {
    return this.state.pending.size > 0;
  }

  isPendingFor(id: string): boolean {
    return this.state.pending.has(id);
  }
}

export enum OwnershipStatus {
  OWNED = 'owned',
  BORROWED = 'borrowed',
  RENTED = 'rented',
  SUBSCRIPTION = 'subscription',
  NO_LONGER_OWNED = 'no_longer_owned'
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
}

export interface UserGamePlatformCreateRequest {
  platform_id: string;
  storefront_id?: string;
  store_game_id?: string;
  store_url?: string;
}

export interface UserGameFilters {
  q?: string; // Search query
  play_status?: PlayStatus;
  ownership_status?: OwnershipStatus;
  is_loved?: boolean;
  platform_id?: string;
  storefront_id?: string;
  rating_min?: number;
  rating_max?: number;
  has_notes?: boolean;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
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
  ownership_status?: OwnershipStatus;
}

export interface BulkDeleteRequest {
  user_game_ids: string[];
}

export interface BulkAddPlatformRequest {
  user_game_ids: string[];
  platform_associations: UserGamePlatformCreateRequest[];
}

export interface BulkRemovePlatformRequest {
  user_game_ids: string[];
  platform_association_ids: string[];
}

export interface SuccessResponse {
  success: boolean;
  message: string;
  updated_count?: number;
  deleted_count?: number;
  failed_count?: number;
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

// Enhanced state with entity-based storage
export interface UserGamesEntityState {
  entities: Map<string, UserGame>;
  ids: string[];
  currentUserGameId: string | null;
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
  // Optimistic update tracking
  optimisticUpdates: OptimisticUpdates<UserGame>;
  // Bulk operation status
  bulkOperations: {
    isProcessing: boolean;
    processingIds: Set<string>;
    lastOperation?: string;
  };
}

// Legacy interface for backward compatibility
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

const initialEntityState: UserGamesEntityState = {
  entities: new Map(),
  ids: [],
  currentUserGameId: null,
  stats: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    per_page: 20,
    total: 0,
    pages: 0
  },
  optimisticUpdates: new OptimisticUpdates<UserGame>(),
  bulkOperations: {
    isProcessing: false,
    processingIds: new Set(),
  }
};

function createUserGamesStore() {
  let entityState = $state<UserGamesEntityState>(initialEntityState);

  // Computed selectors for backward compatibility and performance
  const selectors = {
    get userGames() {
      return entityState.ids.map(id => entityState.entities.get(id)!).filter(Boolean);
    },
    
    get currentUserGame() {
      return entityState.currentUserGameId ? entityState.entities.get(entityState.currentUserGameId) || null : null;
    },
    
    get isLoading() {
      return entityState.isLoading || entityState.optimisticUpdates.isPending;
    },
    
    get hasOptimisticUpdates() {
      return entityState.optimisticUpdates.isPending;
    },
    
    byId: (id: string) => entityState.entities.get(id),
    
    byStatus: (status: PlayStatus) => 
      selectors.userGames.filter(game => game.play_status === status),
      
    byPlatform: (platformId: string) => 
      selectors.userGames.filter(game => 
        game.platforms.some(p => p.platform.id === platformId)
      ),
      
    byRating: (rating: number) => 
      selectors.userGames.filter(game => game.personal_rating === rating),
      
    get lovedGames() {
      return selectors.userGames.filter(game => game.is_loved);
    },
    
    get pileOfShame() {
      return selectors.userGames.filter(game => game.play_status === PlayStatus.NOT_STARTED);
    }
  };
  
  // Entity management utilities
  const entityOperations = {
    addOne: (entity: UserGame) => {
      entityState.entities.set(entity.id, entity);
      if (!entityState.ids.includes(entity.id)) {
        entityState.ids.unshift(entity.id); // Add to beginning like original
      }
    },
    
    addMany: (entities: UserGame[]) => {
      entities.forEach(entity => {
        entityState.entities.set(entity.id, entity);
        if (!entityState.ids.includes(entity.id)) {
          entityState.ids.push(entity.id);
        }
      });
    },
    
    updateOne: (id: string, changes: Partial<UserGame>) => {
      const existing = entityState.entities.get(id);
      if (existing) {
        const updated = { ...existing, ...changes, updated_at: new Date().toISOString() };
        entityState.entities.set(id, updated);
        
        // Update current user game if it's the same
        if (entityState.currentUserGameId === id) {
          // No need to set currentUserGameId again, selector will pick up the change
        }
        
        // Emit update event for cross-view synchronization
        gameEventBus.emit('user-game-updated', { id, game: updated, changes });
        
        return updated;
      }
      return null;
    },
    
    updateMany: (updates: Array<{ id: string; changes: Partial<UserGame> }>) => {
      const updatedGames: UserGame[] = [];
      updates.forEach(({ id, changes }) => {
        const updated = entityOperations.updateOne(id, changes);
        if (updated) updatedGames.push(updated);
      });
      return updatedGames;
    },
    
    removeOne: (id: string) => {
      entityState.entities.delete(id);
      entityState.ids = entityState.ids.filter(existingId => existingId !== id);
      if (entityState.currentUserGameId === id) {
        entityState.currentUserGameId = null;
      }
    },
    
    removeMany: (ids: string[]) => {
      ids.forEach(id => {
        entityState.entities.delete(id);
        if (entityState.currentUserGameId === id) {
          entityState.currentUserGameId = null;
        }
      });
      entityState.ids = entityState.ids.filter(id => !ids.includes(id));
    },
    
    clear: () => {
      entityState.entities.clear();
      entityState.ids = [];
      entityState.currentUserGameId = null;
    },
    
    replaceAll: (entities: UserGame[]) => {
      entityOperations.clear();
      entityOperations.addMany(entities);
    }
  };

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
      // Try to get more detailed error info from response body
      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      try {
        const errorBody = await response.json();
        if (errorBody.detail) {
          errorMessage = errorBody.detail;
        }
      } catch (e) {
        // If we can't parse the error body, use the default message
      }
      
      console.error(`API call failed: ${options.method || 'GET'} ${url}`, {
        status: response.status,
        statusText: response.statusText,
        errorMessage
      });
      
      throw new Error(errorMessage);
    }

    return response;
  };

  const store = {
    get value() {
      // Return backward-compatible state structure
      const legacyState: UserGamesState = {
        userGames: selectors.userGames,
        currentUserGame: selectors.currentUserGame,
        stats: entityState.stats,
        isLoading: selectors.isLoading,
        error: entityState.error,
        filters: entityState.filters,
        pagination: entityState.pagination
      };
      return legacyState;
    },
    
    get entityState() {
      return entityState;
    },
    
    get selectors() {
      return selectors;
    },

    // Load user's game collection with enhanced entity storage
    loadUserGames: async (filters: UserGameFilters = {}, page: number = 1, per_page: number = 20) => {
      entityState.isLoading = true;
      entityState.error = null;

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

        const response = await apiCall(`${config.apiUrl}/user-games/?${params}`);
        const data: UserGameListResponse = await response.json();

        // Replace all entities with new data (for pagination/filtering scenarios)
        entityOperations.replaceAll(data.user_games);
        
        entityState.filters = filters;
        entityState.pagination = {
          page: data.page,
          per_page: data.per_page,
          total: data.total,
          pages: data.pages
        };
        entityState.isLoading = false;
        
        // Emit collection updated event
        gameEventBus.emit('collection-loaded', {
          games: data.user_games,
          pagination: entityState.pagination,
          filters
        });
        
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load user games';
        entityState.isLoading = false;
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Get a specific user game by ID with enhanced caching
    getUserGame: async (id: string) => {
      // Check if we already have this game in our entities
      const existingGame = entityState.entities.get(id);
      if (existingGame && !entityState.optimisticUpdates.isPendingFor(id)) {
        entityState.currentUserGameId = id;
        return existingGame;
      }

      entityState.isLoading = true;
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}`);
        const userGame: UserGame = await response.json();

        // Add/update the entity
        entityOperations.addOne(userGame);
        entityState.currentUserGameId = id;
        entityState.isLoading = false;

        return userGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load user game';
        entityState.isLoading = false;
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Add a game to user's collection
    addGameToCollection: async (gameData: UserGameCreateRequest) => {
      entityState.isLoading = true;
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/`, {
          method: 'POST',
          body: JSON.stringify(gameData),
        });
        
        const userGame: UserGame = await response.json();

        // Add to entities
        entityOperations.addOne(userGame);
        entityState.currentUserGameId = userGame.id;
        entityState.isLoading = false;
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('game-added', { game: userGame });

        return userGame;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to add game to collection';
        entityState.isLoading = false;
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Update user game details with optimistic updates
    updateUserGame: async (id: string, gameData: UserGameUpdateRequest) => {
      const existingGame = entityState.entities.get(id);
      if (!existingGame) {
        throw new Error(`Game with id ${id} not found in collection`);
      }

      // Apply optimistic update immediately
      const rollback = () => {
        entityOperations.updateOne(id, existingGame);
      };
      
      // Apply optimistic update
      entityState.optimisticUpdates.apply(
        id, 
        gameData, 
        existingGame, 
        rollback
      );
      
      // Update entity with optimistic data
      entityOperations.updateOne(id, gameData);
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}`, {
          method: 'PUT',
          body: JSON.stringify(gameData),
        });
        
        const updatedUserGame: UserGame = await response.json();
        
        // Commit optimistic update and replace with server response
        entityState.optimisticUpdates.commit(id);
        entityOperations.updateOne(id, updatedUserGame);

        return updatedUserGame;
      } catch (error) {
        // Rollback optimistic update on failure
        entityState.optimisticUpdates.rollback(id);
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to update user game';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Update game progress with optimistic updates
    updateProgress: async (id: string, progressData: ProgressUpdateRequest) => {
      const existingGame = entityState.entities.get(id);
      if (!existingGame) {
        throw new Error(`Game with id ${id} not found in collection`);
      }

      // Apply optimistic update immediately
      const rollback = () => {
        entityOperations.updateOne(id, existingGame);
      };
      
      // Apply optimistic update
      entityState.optimisticUpdates.apply(
        id, 
        progressData, 
        existingGame, 
        rollback
      );
      
      // Update entity with optimistic data
      entityOperations.updateOne(id, progressData);
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${id}/progress`, {
          method: 'PUT',
          body: JSON.stringify(progressData),
        });
        
        const updatedUserGame: UserGame = await response.json();
        
        // Commit optimistic update and replace with server response
        entityState.optimisticUpdates.commit(id);
        entityOperations.updateOne(id, updatedUserGame);

        return updatedUserGame;
      } catch (error) {
        // Rollback optimistic update on failure
        entityState.optimisticUpdates.rollback(id);
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to update progress';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Remove game from collection with optimistic updates
    removeFromCollection: async (id: string) => {
      const existingGame = entityState.entities.get(id);
      if (!existingGame) {
        throw new Error(`Game with id ${id} not found in collection`);
      }

      // Apply optimistic removal immediately
      const rollback = () => {
        entityOperations.addOne(existingGame);
        if (entityState.currentUserGameId === id) {
          entityState.currentUserGameId = id;
        }
      };
      
      entityState.optimisticUpdates.apply(
        id, 
        {}, // No changes, just tracking
        existingGame, 
        rollback
      );
      
      // Remove from entities optimistically
      entityOperations.removeOne(id);
      entityState.error = null;

      try {
        await apiCall(`${config.apiUrl}/user-games/${id}`, {
          method: 'DELETE',
        });
        
        // Commit optimistic update
        entityState.optimisticUpdates.commit(id);
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('game-removed', { gameId: id, game: existingGame });
        
      } catch (error) {
        // Rollback optimistic update on failure
        entityState.optimisticUpdates.rollback(id);
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove game from collection';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Add platform to user game with optimistic updates
    addPlatformToUserGame: async (userGameId: string, platformData: UserGamePlatformCreateRequest) => {
      const existingGame = entityState.entities.get(userGameId);
      if (!existingGame) {
        throw new Error(`Game with id ${userGameId} not found in collection`);
      }

      // Create optimistic platform entry (we won't have the real ID yet)
      const optimisticPlatform: UserGamePlatform = {
        id: `temp-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
        platform: { id: platformData.platform_id } as Platform, // Will be populated by server
        storefront: platformData.storefront_id ? { id: platformData.storefront_id } as Storefront : undefined,
        store_game_id: platformData.store_game_id,
        store_url: platformData.store_url,
        is_available: true,
        created_at: new Date().toISOString()
      } as UserGamePlatform;

      const optimisticGameUpdate = {
        platforms: [...existingGame.platforms, optimisticPlatform]
      };

      // Apply optimistic update immediately
      const rollback = () => {
        entityOperations.updateOne(userGameId, { platforms: existingGame.platforms });
      };
      
      entityState.optimisticUpdates.apply(
        userGameId, 
        optimisticGameUpdate, 
        existingGame, 
        rollback
      );
      
      // Update entity with optimistic data
      entityOperations.updateOne(userGameId, optimisticGameUpdate);
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/${userGameId}/platforms`, {
          method: 'POST',
          body: JSON.stringify(platformData),
        });
        
        const updatedUserGame: UserGame = await response.json();
        
        // Commit optimistic update and replace with server response
        entityState.optimisticUpdates.commit(userGameId);
        entityOperations.updateOne(userGameId, updatedUserGame);
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('platform-added', { gameId: userGameId, game: updatedUserGame, platformData });

        return updatedUserGame;
      } catch (error) {
        // Rollback optimistic update on failure
        entityState.optimisticUpdates.rollback(userGameId);
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to add platform to user game';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Remove platform from user game with optimistic updates
    removePlatformFromUserGame: async (userGameId: string, platformId: string) => {
      const existingGame = entityState.entities.get(userGameId);
      if (!existingGame) {
        throw new Error(`Game with id ${userGameId} not found in collection`);
      }

      const platformToRemove = existingGame.platforms.find(p => p.id === platformId);
      if (!platformToRemove) {
        throw new Error(`Platform with id ${platformId} not found in game`);
      }

      const optimisticGameUpdate = {
        platforms: existingGame.platforms.filter(p => p.id !== platformId)
      };

      // Apply optimistic update immediately
      const rollback = () => {
        entityOperations.updateOne(userGameId, { platforms: existingGame.platforms });
      };
      
      entityState.optimisticUpdates.apply(
        userGameId, 
        optimisticGameUpdate, 
        existingGame, 
        rollback
      );
      
      // Update entity with optimistic data
      entityOperations.updateOne(userGameId, optimisticGameUpdate);
      entityState.error = null;

      try {
        await apiCall(`${config.apiUrl}/user-games/${userGameId}/platforms/${platformId}`, {
          method: 'DELETE',
        });
        
        // Commit optimistic update (no server response for DELETE)
        entityState.optimisticUpdates.commit(userGameId);
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('platform-removed', { 
          gameId: userGameId, 
          platformId, 
          platform: platformToRemove, 
          updatedGame: entityState.entities.get(userGameId)
        });
        
      } catch (error) {
        // Rollback optimistic update on failure
        entityState.optimisticUpdates.rollback(userGameId);
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to remove platform from user game';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Bulk update status with optimistic updates
    bulkUpdateStatus: async (data: BulkStatusUpdateRequest) => {
      entityState.bulkOperations.isProcessing = true;
      entityState.bulkOperations.processingIds = new Set(data.user_game_ids);
      entityState.bulkOperations.lastOperation = 'bulk-update';
      entityState.error = null;

      // Store original games for rollback
      const originalGames = new Map<string, UserGame>();
      const updates: Array<{ id: string; changes: Partial<UserGame> }> = [];
      
      data.user_game_ids.forEach(id => {
        const existingGame = entityState.entities.get(id);
        if (existingGame) {
          originalGames.set(id, existingGame);
          
          const changes: Partial<UserGame> = {};
          if (data.play_status !== undefined) changes.play_status = data.play_status;
          if (data.personal_rating !== undefined) changes.personal_rating = data.personal_rating;
          if (data.is_loved !== undefined) changes.is_loved = data.is_loved;
          if (data.ownership_status !== undefined) changes.ownership_status = data.ownership_status;
          
          updates.push({ id, changes });
          
          // Apply optimistic update
          const rollback = () => {
            entityOperations.updateOne(id, originalGames.get(id)!);
          };
          
          entityState.optimisticUpdates.apply(id, changes, existingGame, rollback);
        }
      });
      
      // Apply all updates optimistically
      entityOperations.updateMany(updates);

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/bulk-update`, {
          method: 'PUT',
          body: JSON.stringify(data),
        });
        
        const result: SuccessResponse = await response.json();
        
        // Commit all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.commit(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('bulk-updated', {
          gameIds: data.user_game_ids,
          changes: data,
          result
        });

        return result;
      } catch (error) {
        // Rollback all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.rollback(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk update status';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Bulk delete games with optimistic updates
    bulkDelete: async (data: BulkDeleteRequest) => {
      entityState.bulkOperations.isProcessing = true;
      entityState.bulkOperations.processingIds = new Set(data.user_game_ids);
      entityState.bulkOperations.lastOperation = 'bulk-delete';
      entityState.error = null;

      // Store original games for rollback
      const originalGames = new Map<string, UserGame>();
      data.user_game_ids.forEach(id => {
        const existingGame = entityState.entities.get(id);
        if (existingGame) {
          originalGames.set(id, existingGame);
          
          // Apply optimistic removal
          const rollback = () => {
            entityOperations.addOne(originalGames.get(id)!);
          };
          
          entityState.optimisticUpdates.apply(id, {}, existingGame, rollback);
        }
      });
      
      // Remove games optimistically
      entityOperations.removeMany(data.user_game_ids);
      
      // Update pagination
      entityState.pagination.total = Math.max(0, entityState.pagination.total - data.user_game_ids.length);

      try {
        await apiCall(`${config.apiUrl}/user-games/bulk-delete`, {
          method: 'DELETE',
          body: JSON.stringify(data),
        });
        
        // Commit all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.commit(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('bulk-deleted', {
          gameIds: data.user_game_ids,
          games: Array.from(originalGames.values())
        });
        
      } catch (error) {
        // Rollback all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.rollback(id);
        });
        
        // Restore pagination
        entityState.pagination.total = entityState.pagination.total + data.user_game_ids.length;
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk delete games';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Bulk add platforms to games with optimistic updates
    bulkAddPlatforms: async (data: BulkAddPlatformRequest) => {
      entityState.bulkOperations.isProcessing = true;
      entityState.bulkOperations.processingIds = new Set(data.user_game_ids);
      entityState.bulkOperations.lastOperation = 'bulk-add-platforms';
      entityState.error = null;

      // Store original games for rollback
      const originalGames = new Map<string, UserGame>();
      const updates: Array<{ id: string; changes: Partial<UserGame> }> = [];
      
      data.user_game_ids.forEach(gameId => {
        const existingGame = entityState.entities.get(gameId);
        if (existingGame) {
          originalGames.set(gameId, existingGame);
          
          // Create optimistic platform entries
          const optimisticPlatforms = data.platform_associations.map(platformData => ({
            id: `temp-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
            platform: { id: platformData.platform_id } as Platform,
            storefront: platformData.storefront_id ? { id: platformData.storefront_id } as Storefront : undefined,
            store_game_id: platformData.store_game_id,
            store_url: platformData.store_url,
            is_available: true,
            created_at: new Date().toISOString()
          } as UserGamePlatform));
          
          const changes: Partial<UserGame> = {
            platforms: [...existingGame.platforms, ...optimisticPlatforms]
          };
          
          updates.push({ id: gameId, changes });
          
          // Apply optimistic update
          const rollback = () => {
            entityOperations.updateOne(gameId, originalGames.get(gameId)!);
          };
          
          entityState.optimisticUpdates.apply(gameId, changes, existingGame, rollback);
        }
      });
      
      // Apply all updates optimistically
      entityOperations.updateMany(updates);

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/bulk-add-platforms`, {
          method: 'POST',
          body: JSON.stringify(data),
        });
        
        const result: SuccessResponse = await response.json();
        
        // For platform operations, we need to reload affected games to get proper server IDs
        // But we'll do it more efficiently by only reloading affected games
        const updatedGamesPromises = data.user_game_ids.map(id => 
          apiCall(`${config.apiUrl}/user-games/${id}`).then(res => res.json())
        );
        
        const updatedGames: UserGame[] = await Promise.all(updatedGamesPromises);
        
        // Update entities with server response and commit optimistic updates
        updatedGames.forEach(game => {
          entityState.optimisticUpdates.commit(game.id);
          entityOperations.updateOne(game.id, game);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('bulk-platforms-added', {
          gameIds: data.user_game_ids,
          platformData: data.platform_associations,
          result
        });

        return result;
      } catch (error) {
        // Rollback all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.rollback(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk add platforms';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Bulk remove platforms from games with optimistic updates
    bulkRemovePlatforms: async (data: BulkRemovePlatformRequest) => {
      entityState.bulkOperations.isProcessing = true;
      entityState.bulkOperations.processingIds = new Set(data.user_game_ids);
      entityState.bulkOperations.lastOperation = 'bulk-remove-platforms';
      entityState.error = null;

      // Store original games for rollback
      const originalGames = new Map<string, UserGame>();
      const updates: Array<{ id: string; changes: Partial<UserGame> }> = [];
      
      data.user_game_ids.forEach(gameId => {
        const existingGame = entityState.entities.get(gameId);
        if (existingGame) {
          originalGames.set(gameId, existingGame);
          
          // Remove platforms optimistically
          const updatedPlatforms = existingGame.platforms.filter(
            platform => !data.platform_association_ids.includes(platform.id)
          );
          
          const changes: Partial<UserGame> = {
            platforms: updatedPlatforms
          };
          
          updates.push({ id: gameId, changes });
          
          // Apply optimistic update
          const rollback = () => {
            entityOperations.updateOne(gameId, originalGames.get(gameId)!);
          };
          
          entityState.optimisticUpdates.apply(gameId, changes, existingGame, rollback);
        }
      });
      
      // Apply all updates optimistically
      entityOperations.updateMany(updates);

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/bulk-remove-platforms`, {
          method: 'DELETE',
          body: JSON.stringify(data),
        });
        
        const result: SuccessResponse = await response.json();
        
        // Commit all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.commit(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        // Emit event for cross-view synchronization
        gameEventBus.emit('bulk-platforms-removed', {
          gameIds: data.user_game_ids,
          platformIds: data.platform_association_ids,
          result
        });

        return result;
      } catch (error) {
        // Rollback all optimistic updates
        data.user_game_ids.forEach(id => {
          entityState.optimisticUpdates.rollback(id);
        });
        
        entityState.bulkOperations.isProcessing = false;
        entityState.bulkOperations.processingIds.clear();
        
        const errorMessage = error instanceof Error ? error.message : 'Failed to bulk remove platforms';
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Get collection statistics
    getCollectionStats: async () => {
      entityState.isLoading = true;
      entityState.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/user-games/stats`);
        const stats: CollectionStats = await response.json();

        entityState.stats = stats;
        entityState.isLoading = false;

        return stats;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get collection stats';
        entityState.isLoading = false;
        entityState.error = errorMessage;
        throw error;
      }
    },

    // Enhanced getter methods using entity-based selectors
    getGamesByStatus: (status: PlayStatus) => {
      return selectors.byStatus(status);
    },

    getLovedGames: () => {
      return selectors.lovedGames;
    },

    getGamesByRating: (rating: number) => {
      return selectors.byRating(rating);
    },

    getPileOfShame: () => {
      return selectors.pileOfShame;
    },
    
    getGamesByPlatform: (platformId: string) => {
      return selectors.byPlatform(platformId);
    },
    
    getGameById: (id: string) => {
      return selectors.byId(id);
    },

    // Enhanced state management
    clearCurrentUserGame: () => {
      entityState.currentUserGameId = null;
    },

    clearFilters: () => {
      entityState.filters = {};
    },

    clearError: () => {
      entityState.error = null;
    },
    
    clearOptimisticUpdates: () => {
      entityState.optimisticUpdates.rollbackAll();
    },

    // Get available platform associations for bulk removal
    getAvailablePlatformAssociationsForGames: (gameIds: string[]) => {
      const platformAssociations = new Map<string, {
        platformId: string;
        platformName: string;
        storefrontId?: string;
        storefrontName?: string;
        associationIds: string[];
        platformIconUrl?: string;
      }>();

      gameIds.forEach(gameId => {
        const userGame = entityState.entities.get(gameId);
        if (userGame) {
          userGame.platforms.forEach(platform => {
            const key = `${platform.platform.id}-${platform.storefront?.id || 'none'}`;
            
            if (!platformAssociations.has(key)) {
              const association: {
                platformId: string;
                platformName: string;
                storefrontId?: string;
                storefrontName?: string;
                associationIds: string[];
                platformIconUrl?: string;
              } = {
                platformId: platform.platform.id,
                platformName: platform.platform.display_name,
                associationIds: []
              };
              
              if (platform.storefront?.id) {
                association.storefrontId = platform.storefront.id;
              }
              
              if (platform.storefront?.display_name) {
                association.storefrontName = platform.storefront.display_name;
              }
              
              if (platform.platform.icon_url) {
                association.platformIconUrl = platform.platform.icon_url;
              }
              
              platformAssociations.set(key, association);
            }
            
            const association = platformAssociations.get(key)!;
            association.associationIds.push(platform.id);
          });
        }
      });

      return Array.from(platformAssociations.values());
    },
    
    // Context preservation helpers
    getNavigationContext: () => {
      return {
        filters: entityState.filters,
        pagination: entityState.pagination,
        currentUserGameId: entityState.currentUserGameId,
        hasOptimisticUpdates: entityState.optimisticUpdates.isPending
      };
    },
    
    restoreNavigationContext: (context: any) => {
      if (context.filters) entityState.filters = context.filters;
      if (context.pagination) entityState.pagination = context.pagination;
      if (context.currentUserGameId) entityState.currentUserGameId = context.currentUserGameId;
    },
    
    // Enhanced debugging and monitoring
    getEntityStats: () => {
      return {
        totalEntities: entityState.entities.size,
        idsLength: entityState.ids.length,
        pendingOptimisticUpdates: entityState.optimisticUpdates.isPending,
        bulkOperationInProgress: entityState.bulkOperations.isProcessing
      };
    },

    // Backward compatibility aliases
    fetchUserGames: async (filters: UserGameFilters = {}, page: number = 1, per_page: number = 20) => {
      return await store.loadUserGames(filters, page, per_page);
    },
    
    // Event system integration
    on: gameEventBus.on.bind(gameEventBus),
    off: gameEventBus.off.bind(gameEventBus),
    emit: gameEventBus.emit.bind(gameEventBus),
    
    // Testing utility - only for test environments
    __testSetData: (games: UserGame[]) => {
      entityOperations.replaceAll(games);
    }
  };

  return store;
}

export const userGames = createUserGamesStore();