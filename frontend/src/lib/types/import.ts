/**
 * Generic types and interfaces for import functionality.
 * 
 * These interfaces provide a foundation for creating reusable import
 * components that can work with different game import sources.
 */

export interface ImportGame {
  id: string;
  name: string;
  external_id: string;
  igdb_id: string | null;
  igdb_title: string | null;
  game_id: string | null;
  user_game_id: string | null;
  ignored: boolean;
  created_at: string;
  updated_at: string;
}

export interface ImportGameAction {
  type: 'match' | 'auto-match' | 'sync' | 'ignore' | 'unignore' | 'unmatch' | 'unsync';
  label: string;
  icon: string;
  enabled: (game: ImportGame) => boolean;
  handler: (game: ImportGame) => Promise<void>;
  buttonClass?: string;
  title?: string;
  requiresConfirmation?: boolean;
  confirmationMessage?: (game: ImportGame) => string;
}

export interface ImportGameStatus {
  label: string;
  color: string;
  icon: string;
}

export interface ImportStats {
  total: number;
  unmatched: number;
  matched: number;
  ignored: number;
  synced: number;
}

export interface ImportStatCard {
  label: string;
  value: number;
  icon: string;
  color?: string;
}

export interface ImportSource {
  id: string;
  name: string;
  icon: string;
  color: string;
}

export interface ImportConfiguration {
  hasApiKey: boolean;
  isVerified: boolean;
  isConfigured: boolean;
  maskedApiKey?: string;
  steamId?: string;
  configuredAt?: Date;
}

export interface ImportSearchResult {
  id: string;
  name: string;
  igdb_id: string;
  cover_url?: string;
  release_date?: string;
  platforms?: string[];
  genres?: string[];
}

export interface BulkOperationResponse {
  message: string;
  total_processed: number;
  successful_operations: number;
  failed_operations: number;
  skipped_items: number;
  errors: string[];
}

export interface BatchSession {
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
  isProcessing: boolean;
  errors: string[];
}

/**
 * Generic service interface that import sources must implement
 */
export interface ImportService {
  // Game management
  listGames(offset: number, limit: number, status?: string, search?: string): Promise<{ games: ImportGame[]; total: number }>;
  
  // Individual game actions
  matchGameToIGDB(gameId: string, igdbId: string | null): Promise<void>;
  autoMatchSingleGame(gameId: string): Promise<void>;
  syncGameToCollection(gameId: string): Promise<void>;
  toggleGameIgnored(gameId: string): Promise<void>;
  unsyncGameFromCollection(gameId: string): Promise<void>;
  
  // Bulk operations
  unmatchAllGames(): Promise<BulkOperationResponse>;
  unsyncAllGames(): Promise<BulkOperationResponse>;
  unignoreAllGames(): Promise<BulkOperationResponse>;
  
  // Batch processing
  startBatchAutoMatch(): Promise<{ session_id: string; total_items: number; operation_type: string; status: string; message: string }>;
  startBatchSync(): Promise<{ session_id: string; total_items: number; operation_type: string; status: string; message: string }>;
  processBatchAutoMatch(sessionId: string): Promise<void>;
  processBatchSync(sessionId: string): Promise<void>;
  cancelBatchOperation(sessionId: string): Promise<void>;
  clearBatchSession(): void;
  
  // Library import
  importLibrary(): Promise<void>;
  
  // State
  readonly isImporting: boolean;
  readonly isBatchProcessing: boolean;
  readonly isUnmatchingAll: boolean;
  readonly isUnsyncingAll: boolean;
  readonly isUnignoringAll: boolean;
  readonly activeBatchSession: BatchSession | null;
}