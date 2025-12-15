import { config } from '$lib/env';
import { auth } from './auth.svelte';
import { ui } from './ui.svelte';
import { loggers } from '$lib/services/logger';
import type { GameId } from '$lib/types/game';

const log = loggers.darkadia;
import type {
  DarkadiaGameResponse,
  DarkadiaGamesListResponse,
  DarkadiaUploadResponse,
  DarkadiaLibraryPreview,
  DarkadiaUploadState,
  DarkadiaStats,
  DarkadiaFilterState,
  DarkadiaBatchSession,
  DarkadiaConfigResponse,
  DarkadiaGameMatchResponse,
  DarkadiaGameSyncResponse,
  DarkadiaGameIgnoreResponse,
  DarkadiaGamesBulkUnignoreResponse,
  DarkadiaGamesBulkUnmatchResponse,
  DarkadiaResolutionSummaryResponse,
  DarkadiaUpdateMappingsRequest,
  DarkadiaUpdateMappingsResponse,
  DarkadiaImportJob
} from '$lib/types/darkadia';

export type DarkadiaGameStatusFilter = 'unmatched' | 'matched' | 'ignored' | 'synced';

// Batch processing interfaces (matching Steam pattern)
export interface DarkadiaBatchSessionStartResponse {
  session_id: string;
  total_items: number;
  operation_type: string;
  status: string;
  message: string;
}

export interface DarkadiaBatchNextResponse {
  session_id: string;
  batch_processed: number;
  batch_successful: number;
  batch_failed: number;
  batch_errors: string[];
  current_batch_items: DarkadiaGameResponse[];
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

export interface DarkadiaBatchCancelResponse {
  session_id: string;
  status: string;
  processed_items: number;
  successful_items: number;
  failed_items: number;
  message: string;
}

export interface DarkadiaState {
  // Games data
  games: DarkadiaGameResponse[];
  total: number;
  
  // Loading states
  isLoading: boolean;
  isUploading: boolean;
  isImporting: boolean;
  isAutoMatching: boolean;
  isSyncing: boolean;
  isUnignoringAll: boolean;
  isUnmatchingAll: boolean;
  
  // Upload state
  uploadState: DarkadiaUploadState;
  
  // Configuration
  config: DarkadiaConfigResponse | null;
  
  // Preview data
  previewData: DarkadiaLibraryPreview | null;
  
  // Filters
  filters: DarkadiaFilterState;
  
  // Stats
  stats: DarkadiaStats;
  
  // Batch processing
  activeBatchSession: DarkadiaBatchSession | null;
  isBatchProcessing: boolean;
  
  // Import job tracking
  currentImportJob: DarkadiaImportJob | null;
  
  // Error handling
  error: string | null;
  lastRefresh: Date | null;
}

const initialUploadState: DarkadiaUploadState = {
  isDragging: false,
  isUploading: false,
  isImporting: false,
  uploadProgress: 0,
  importProgress: 0,
  uploadedFile: null,
  uploadResult: null,
  error: null
};

const initialFilters: DarkadiaFilterState = {
  searchQuery: '',
  statusFilter: null
};

const initialStats: DarkadiaStats = {
  totalCount: 0,
  unmatchedCount: 0,
  matchedCount: 0,
  ignoredCount: 0,
  syncedCount: 0
};

const initialState: DarkadiaState = {
  games: [],
  total: 0,
  isLoading: false,
  isUploading: false,
  isImporting: false,
  isAutoMatching: false,
  isSyncing: false,
  isUnignoringAll: false,
  isUnmatchingAll: false,
  uploadState: initialUploadState,
  config: null,
  previewData: null,
  filters: initialFilters,
  stats: initialStats,
  activeBatchSession: null,
  isBatchProcessing: false,
  currentImportJob: null,
  error: null,
  lastRefresh: null
};

function createDarkadiaStore() {
  let state = $state<DarkadiaState>(initialState);

  const darkadiaStore = {
    get value() {
      return state;
    },

    reset() {
      state = { ...initialState };
    },

    // Configuration methods
    async getConfig(): Promise<DarkadiaConfigResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/config`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.getConfig();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get Darkadia configuration');
        }

        const configData = await response.json() as DarkadiaConfigResponse;
        
        // Convert string dates to Date objects
        if (configData.configured_at) {
          configData.configured_at = new Date(configData.configured_at);
        }
        
        state = {
          ...state,
          config: configData,
          error: null
        };

        return configData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get configuration';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    async deleteConfig(): Promise<void> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/config`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.deleteConfig();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to delete Darkadia configuration');
        }

        state = {
          ...state,
          config: null,
          uploadState: initialUploadState,
          previewData: null,
          games: [],
          total: 0,
          stats: initialStats,
          error: null
        };

        ui.showSuccess('Darkadia configuration deleted successfully');
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete configuration';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Upload methods
    async uploadCSV(file: File): Promise<DarkadiaUploadResponse> {
      state = {
        ...state,
        uploadState: {
          ...state.uploadState,
          isUploading: true,
          uploadProgress: 0,
          uploadedFile: file,
          error: null
        }
      };

      try {
        const formData = new FormData();
        formData.append('file', file);

        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/upload`, {
          method: 'POST',
          body: formData,
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.uploadCSV(file);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to upload CSV file');
        }

        const uploadResult = await response.json() as DarkadiaUploadResponse;
        
        state = {
          ...state,
          uploadState: {
            ...state.uploadState,
            isUploading: false,
            uploadProgress: 100,
            uploadResult,
            error: null
          }
        };

        ui.showSuccess('CSV file uploaded successfully');
        
        // Auto-trigger import
        await this.triggerImport();
        
        return uploadResult;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to upload CSV file';
        
        state = {
          ...state,
          uploadState: {
            ...state.uploadState,
            isUploading: false,
            error: errorMessage
          }
        };
        
        ui.showError(errorMessage);
        throw error;
      }
    },

    async getLibraryPreview(): Promise<DarkadiaLibraryPreview> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/preview`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.getLibraryPreview();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get library preview');
        }

        const previewData = await response.json() as DarkadiaLibraryPreview;
        
        state = {
          ...state,
          previewData,
          error: null
        };

        return previewData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get library preview';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    // Import methods
    async triggerImport(): Promise<void> {
      state = {
        ...state,
        uploadState: {
          ...state.uploadState,
          isImporting: true,
          importProgress: 0
        }
      };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/import`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.triggerImport();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to trigger import');
        }

        const result = await response.json();
        
        ui.showSuccess('CSV import started successfully');
        
        // Initialize currentImportJob with pending state
        if (result.job_id) {
          state = {
            ...state,
            currentImportJob: {
              id: result.job_id,
              status: 'pending',
              progress: 0,
              total_items: 0,
              processed_items: 0,
              successful_items: 0,
              failed_items: 0,
              error_message: undefined,
              started_at: new Date(),
              completed_at: undefined
            }
          };
          
          this.pollImportJob(result.job_id);
        }
        
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to trigger import';
        
        state = {
          ...state,
          uploadState: {
            ...state.uploadState,
            isImporting: false,
            error: errorMessage
          }
        };
        
        ui.showError(errorMessage);
        throw error;
      }
    },

    async pollImportJob(jobId: string): Promise<void> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/jobs/${jobId}`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (response.ok) {
          const job = await response.json();
          
          // Update current import job state
          state = {
            ...state,
            currentImportJob: {
              id: job.id,
              status: job.status,
              progress: job.progress || 0,
              total_items: job.total_items || 0,
              processed_items: job.processed_items || 0,
              successful_items: job.successful_items || 0,
              failed_items: job.failed_items || 0,
              error_message: job.error_message || undefined,
              started_at: job.started_at ? new Date(job.started_at) : undefined,
              completed_at: job.completed_at ? new Date(job.completed_at) : undefined
            },
            uploadState: {
              ...state.uploadState,
              importProgress: job.progress || 0
            }
          };

          if (job.status === 'completed') {
            state = {
              ...state,
              uploadState: {
                ...state.uploadState,
                isImporting: false,
                importProgress: 100
              }
            };
            
            ui.showSuccess('CSV import completed successfully');
            
            // Refresh games list
            await this.listDarkadiaGames();
          } else if (job.status === 'failed') {
            state = {
              ...state,
              uploadState: {
                ...state.uploadState,
                isImporting: false,
                error: job.error_message || 'Import failed'
              }
            };
            
            ui.showError(job.error_message || 'Import failed');
          } else if (job.status === 'processing' || job.status === 'pending') {
            // Continue polling
            setTimeout(() => this.pollImportJob(jobId), 2000);
          } else if (job.status === 'cancelled') {
            state = {
              ...state,
              uploadState: {
                ...state.uploadState,
                isImporting: false,
                error: null
              }
            };
            
            ui.showInfo('CSV import was cancelled');
          }
        }
      } catch (error) {
        log.error('Failed to poll import job status', error);
        // Continue polling despite errors
        setTimeout(() => this.pollImportJob(jobId), 5000);
      }
    },

    async cancelImportJob(jobId: string): Promise<void> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/jobs/${jobId}/cancel`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.cancelImportJob(jobId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to cancel import job');
        }

        ui.showInfo('Import job cancellation requested');
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to cancel import job';
        ui.showError(errorMessage);
        throw error;
      }
    },

    clearImportJob(): void {
      state = {
        ...state,
        currentImportJob: null
      };
    },

    // Games listing and filtering
    async listDarkadiaGames(
      offset: number = 0,
      limit: number = 100,
      statusFilter?: DarkadiaGameStatusFilter,
      search?: string
    ): Promise<DarkadiaGamesListResponse> {
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

        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games?${params}`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.listDarkadiaGames(offset, limit, statusFilter, search);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to fetch Darkadia games');
        }

        const data = await response.json() as DarkadiaGamesListResponse;
        
        // Convert string dates to Date objects
        data.games = data.games.map(game => ({
          ...game,
          created_at: new Date(game.created_at),
          updated_at: new Date(game.updated_at)
        }));
        
        // Update stats
        const stats = this.calculateStats(data.games);
        
        state = {
          ...state,
          games: data.games,
          total: data.total,
          stats,
          isLoading: false,
          error: null,
          lastRefresh: new Date()
        };

        return data;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to fetch Darkadia games';
        state = { ...state, isLoading: false, error: errorMessage };
        throw error;
      }
    },

    calculateStats(games: DarkadiaGameResponse[]): DarkadiaStats {
      const stats: DarkadiaStats = {
        totalCount: games.length,
        unmatchedCount: 0,
        matchedCount: 0,
        ignoredCount: 0,
        syncedCount: 0
      };

      games.forEach(game => {
        if (game.ignored) {
          stats.ignoredCount++;
        } else if (game.game_id) {
          stats.syncedCount++;
        } else if (game.igdb_id) {
          stats.matchedCount++;
        } else {
          stats.unmatchedCount++;
        }
      });

      return stats;
    },

    // Game operations (matching Steam pattern)
    async matchGame(userId: string, gameId: string, igdbId: GameId | null): Promise<DarkadiaGameResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${gameId}/match`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({ igdb_id: igdbId ? Number(igdbId) : null })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.matchGame(userId, gameId, igdbId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to match game');
        }

        const result = await response.json() as DarkadiaGameMatchResponse;
        
        // Update the game in local state
        state = {
          ...state,
          games: state.games.map(game => 
            game.id === gameId ? result.game : game
          )
        };

        ui.showSuccess(result.message);
        return result.game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to match game';
        ui.showError(errorMessage);
        throw error;
      }
    },

    async syncGame(userId: string, gameId: string): Promise<DarkadiaGameSyncResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${gameId}/sync`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.syncGame(userId, gameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to sync game');
        }

        const result = await response.json() as DarkadiaGameSyncResponse;
        
        // Update the game in local state
        state = {
          ...state,
          games: state.games.map(game => 
            game.id === gameId ? result.game : game
          )
        };

        ui.showSuccess(result.message);
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to sync game';
        ui.showError(errorMessage);
        throw error;
      }
    },

    async ignoreGame(userId: string, gameId: string): Promise<DarkadiaGameResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${gameId}/ignore`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.ignoreGame(userId, gameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to ignore game');
        }

        const result = await response.json() as DarkadiaGameIgnoreResponse;
        
        // Update the game in local state
        state = {
          ...state,
          games: state.games.map(game => 
            game.id === gameId ? result.game : game
          )
        };

        ui.showSuccess(result.message);
        return result.game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to ignore game';
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Batch operations (matching Steam implementation pattern)
    async startBatchAutoMatch(): Promise<DarkadiaBatchSessionStartResponse> {
      state = { ...state, isAutoMatching: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/batch/auto-match/start`, {
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
          throw new Error(errorData.detail || 'Failed to start batch auto-match');
        }

        const sessionData = await response.json() as DarkadiaBatchSessionStartResponse;

        state = {
          ...state,
          activeBatchSession: {
            sessionId: sessionData.session_id,
            operationType: 'auto_match',
            isActive: true,
            isComplete: false,
            status: sessionData.status,
            totalItems: sessionData.total_items,
            processedItems: 0,
            successfulItems: 0,
            failedItems: 0,
            remainingItems: sessionData.total_items,
            progressPercentage: 0,
            isProcessing: true,
            errors: []
          },
          isBatchProcessing: true
        };

        ui.showInfo(`Started auto-matching ${sessionData.total_items} games`);
        return sessionData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start batch auto-match';
        state = { ...state, isAutoMatching: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    async startBatchSync(): Promise<DarkadiaBatchSessionStartResponse> {
      state = { ...state, isSyncing: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/batch/sync/start`, {
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
          throw new Error(errorData.detail || 'Failed to start batch sync');
        }

        const sessionData = await response.json() as DarkadiaBatchSessionStartResponse;

        state = {
          ...state,
          activeBatchSession: {
            sessionId: sessionData.session_id,
            operationType: 'sync',
            isActive: true,
            isComplete: false,
            status: sessionData.status,
            totalItems: sessionData.total_items,
            processedItems: 0,
            successfulItems: 0,
            failedItems: 0,
            remainingItems: sessionData.total_items,
            progressPercentage: 0,
            isProcessing: true,
            errors: []
          },
          isBatchProcessing: true
        };

        ui.showInfo(`Started syncing ${sessionData.total_items} games`);
        return sessionData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to start batch sync';
        state = { ...state, isSyncing: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    async processBatchNext(sessionId: string): Promise<DarkadiaBatchNextResponse> {
      try {
        // Map operation type to correct URL path
        const operationType = state.activeBatchSession?.operationType;
        const urlPath = operationType === 'auto_match' ? 'auto-match' : 'sync';
        
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/batch/${urlPath}/${sessionId}/next`, {
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
            return this.processBatchNext(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to process next batch');
        }

        const batchData = await response.json() as DarkadiaBatchNextResponse;

        // Update batch session state
        if (state.activeBatchSession) {
          state = {
            ...state,
            activeBatchSession: {
              ...state.activeBatchSession,
              processedItems: batchData.processed_items,
              successfulItems: batchData.successful_items,
              failedItems: batchData.failed_items,
              remainingItems: state.activeBatchSession.totalItems - batchData.processed_items,
              progressPercentage: batchData.progress_percentage,
              isProcessing: !batchData.is_complete,
              errors: [...state.activeBatchSession.errors, ...batchData.batch_errors],
              isComplete: batchData.is_complete,
              status: batchData.status
            }
          };

          if (batchData.is_complete) {
            state = {
              ...state,
              isBatchProcessing: false,
              isAutoMatching: false,
              isSyncing: false
            };
          }
        }

        return batchData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to process next batch';
        ui.showError(errorMessage);
        throw error;
      }
    },

    async cancelBatchSession(sessionId: string): Promise<DarkadiaBatchCancelResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/batch/${sessionId}`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.cancelBatchSession(sessionId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to cancel batch session');
        }

        const cancelData = await response.json() as DarkadiaBatchCancelResponse;
        
        this.clearBatchSession();
        ui.showInfo('Batch session cancelled');
        
        return cancelData;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to cancel batch session';
        ui.showError(errorMessage);
        throw error;
      }
    },

    clearBatchSession() {
      state = {
        ...state,
        activeBatchSession: null,
        isBatchProcessing: false,
        isAutoMatching: false,
        isSyncing: false
      };
    },

    // Bulk operations without batch processing
    async unignoreAllGames(): Promise<DarkadiaGamesBulkUnignoreResponse> {
      state = { ...state, isUnignoringAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/unignore-all`, {
          method: 'POST',
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
          throw new Error(errorData.detail || 'Failed to unignore all games');
        }

        const result = await response.json() as DarkadiaGamesBulkUnignoreResponse;
        
        state = { ...state, isUnignoringAll: false };
        ui.showSuccess(result.message);
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unignore all games';
        state = { ...state, isUnignoringAll: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    async resetImport(): Promise<void> {
      const wasLoading = state.isLoading;
      state = { 
        ...state, 
        isLoading: true, 
        error: null 
      };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/reset`, {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.resetImport();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to reset Darkadia import');
        }

        const result = await response.json();
        
        // Reset all local state to initial values
        state = {
          games: [],
          total: 0,
          isLoading: false,
          isUploading: false,
          isImporting: false,
          isAutoMatching: false,
          isSyncing: false,
          isUnignoringAll: false,
          isUnmatchingAll: false,
          uploadState: initialUploadState,
          config: null,
          previewData: null,
          filters: initialFilters,
          stats: initialStats,
          activeBatchSession: null,
          isBatchProcessing: false,
          currentImportJob: null,
          error: null,
          lastRefresh: new Date()
        };
        
        // Show success message with details
        ui.showSuccess(
          `Reset completed: ${result.unsynced_games} games removed from collection, ` +
          `${result.deleted_games} staging games deleted, ` +
          `${result.deleted_imports} import records cleared`
        );
        
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to reset Darkadia import';
        state = { 
          ...state, 
          isLoading: wasLoading,
          error: errorMessage 
        };
        ui.showError(errorMessage);
        throw error;
      }
    },

    async unmatchAllGames(): Promise<DarkadiaGamesBulkUnmatchResponse> {
      state = { ...state, isUnmatchingAll: true, error: null };

      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/unmatch-all`, {
          method: 'POST',
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
          throw new Error(errorData.detail || 'Failed to unmatch all games');
        }

        const result = await response.json() as DarkadiaGamesBulkUnmatchResponse;
        
        state = { ...state, isUnmatchingAll: false };
        ui.showSuccess(result.message);
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to unmatch all games';
        state = { ...state, isUnmatchingAll: false, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Platform/Storefront Resolution Methods
    async getResolutionSummary(): Promise<DarkadiaResolutionSummaryResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/resolution-summary`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.getResolutionSummary();
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get resolution summary');
        }

        const result = await response.json() as DarkadiaResolutionSummaryResponse;
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get resolution summary';
        state = { ...state, error: errorMessage };
        throw error;
      }
    },

    async updateMappings(request: DarkadiaUpdateMappingsRequest): Promise<DarkadiaUpdateMappingsResponse> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/update-mappings`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify(request)
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.updateMappings(request);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to update mappings');
        }

        const result = await response.json() as DarkadiaUpdateMappingsResponse;
        ui.showSuccess(result.message);
        
        // Refresh games list to reflect changes
        await this.listDarkadiaGames();
        
        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update mappings';
        state = { ...state, error: errorMessage };
        ui.showError(errorMessage);
        throw error;
      }
    },

    // Manual matching methods
    async manualMatchGame(userId: string, gameId: string, igdbGame?: any, platformChanges?: any[]): Promise<DarkadiaGameResponse> {
      try {
        // Step 1: Update IGDB match if provided
        if (igdbGame) {
          await this.matchGame(userId, gameId, igdbGame.igdb_id);
        }

        // Step 2: Update platform configurations if provided
        if (platformChanges && platformChanges.length > 0) {
          for (const change of platformChanges) {
            await this.updateGamePlatform(gameId, change.copy_identifier, change.platform_id, change.storefront_id);
          }
        }

        // Refresh the games list to get updated data
        await this.listDarkadiaGames();
        
        // Find and return the updated game
        const game = state.games.find(g => g.id === gameId);
        if (!game) {
          throw new Error('Game not found after manual match');
        }

        ui.showSuccess('Manual match completed successfully');
        return game;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to complete manual match';
        ui.showError(errorMessage);
        throw error;
      }
    },

    async updateGamePlatform(gameId: string, copyIdentifier: string, platformId?: string, storefrontId?: string): Promise<void> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${gameId}/platforms`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${auth.value.accessToken}`
          },
          body: JSON.stringify({
            copy_identifier: copyIdentifier,
            platform_id: platformId || null,
            storefront_id: storefrontId || null
          })
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.updateGamePlatform(gameId, copyIdentifier, platformId, storefrontId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to update game platform');
        }

        const result = await response.json();
        log.debug('Platform update result', result);
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to update game platform';
        log.error('Error updating game platform', error);
        throw new Error(errorMessage);
      }
    },

    async getGamePlatformOptions(gameId: string): Promise<any> {
      try {
        const response = await fetch(`${config.apiUrl}/import/sources/darkadia/games/${gameId}/platforms`, {
          headers: {
            'Authorization': `Bearer ${auth.value.accessToken}`
          }
        });

        if (!response.ok) {
          if (response.status === 401) {
            await auth.refreshAuth();
            return this.getGamePlatformOptions(gameId);
          }
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.detail || 'Failed to get platform options');
        }

        return await response.json();
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to get platform options';
        log.error('Error getting platform options', error);
        throw new Error(errorMessage);
      }
    }
  };

  return darkadiaStore;
}

export const darkadia = createDarkadiaStore();