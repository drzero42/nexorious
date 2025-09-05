/**
 * Type definitions for Darkadia CSV import functionality
 * Matches backend API schemas in backend/app/api/schemas/darkadia.py
 */

// Upload and Configuration Types

export interface DarkadiaConfigRequest {
  csv_file_path: string;
}

export interface DarkadiaConfigResponse {
  has_csv_file: boolean;
  csv_file_path?: string;
  file_exists: boolean;
  file_hash?: string;
  configured_at?: Date;
}

export interface DarkadiaVerificationRequest {
  csv_file_path: string;
}

export interface DarkadiaUploadResponse {
  message: string;
  file_id: string;
  total_games: number;
  file_path: string;
  file_size: number;
  preview_games: Array<Record<string, any>>;
}

// Game Types

export interface DarkadiaGamePreview {
  name: string;
  platforms: string;
  rating: string;
  played: boolean;
  finished: boolean;
}

export interface DarkadiaLibraryPreview {
  total_games_estimate: number;
  preview_games: DarkadiaGamePreview[];
  file_info: Record<string, any>;
  platform_analysis: Record<string, any>;
}

export interface DarkadiaPlatformInfo {
  original_platform_name?: string;
  original_storefront_name?: string;
  resolved_platform_name?: string;
  resolved_storefront_name?: string;
  platform_resolution_status: 'resolved' | 'pending' | 'mapped' | 'ignored' | 'conflict';
  storefront_resolution_status?: 'resolved' | 'pending' | 'mapped' | 'ignored' | 'conflict';
  copy_identifier?: string;
}

export interface DarkadiaGameResponse {
  id: string;
  external_id: string;
  name: string;
  igdb_id?: string;
  igdb_title?: string;
  game_id?: string;
  user_game_id?: string;
  ignored: boolean;
  created_at: Date;
  updated_at: Date;
  
  // Multi-platform support
  platforms: DarkadiaPlatformInfo[];
  
  // Legacy single platform fields (for backward compatibility)
  platform_resolved?: boolean;
  original_platform_name?: string;
  platform_resolution_status?: 'resolved' | 'pending' | 'mapped' | 'ignored' | 'conflict';
  platform_name?: string;
  original_storefront_name?: string;
  storefront_resolution_status?: 'resolved' | 'pending' | 'mapped' | 'ignored' | 'conflict';
  storefront_name?: string;
}

export interface DarkadiaGamesListResponse {
  total: number;
  games: DarkadiaGameResponse[];
}

// Import Types

export interface DarkadiaImportStartResponse {
  message: string;
  imported_count: number;
  skipped_count: number;
  auto_matched_count: number;
  total_games: number;
  errors: string[];
}

// Game Operation Types

export interface DarkadiaGameMatchRequest {
  igdb_id?: string;
}

export interface DarkadiaGameMatchResponse {
  message: string;
  game: DarkadiaGameResponse;
}

export interface DarkadiaGameSyncResponse {
  message: string;
  game: DarkadiaGameResponse;
  user_game_id: string;
  action: string;
}

export interface DarkadiaGameIgnoreResponse {
  message: string;
  game: DarkadiaGameResponse;
  ignored: boolean;
}

// Bulk Operations Types

export interface DarkadiaGamesBulkSyncResponse {
  message: string;
  total_processed: number;
  successful_syncs: number;
  failed_syncs: number;
  errors: string[];
}

export interface DarkadiaGamesBulkUnignoreResponse {
  message: string;
  total_processed: number;
  successful_unignores: number;
  failed_unignores: number;
}

export interface DarkadiaGamesBulkUnmatchResponse {
  message: string;
  total_processed: number;
  successful_unmatches: number;
  failed_unmatches: number;
  errors: string[];
}

export interface DarkadiaGamesAutoMatchResponse {
  message: string;
  total_processed: number;
  successful_matches: number;
  failed_matches: number;
  errors: string[];
}

export interface DarkadiaGameAutoMatchSingleResponse {
  message: string;
  game: DarkadiaGameResponse;
  matched: boolean;
  confidence?: number;
}

// Platform Resolution Types

export interface DarkadiaPlatformStatus {
  name: string;
  games_count: number;
  is_known: boolean;
  mapped_name?: string;
  suggested_mapping?: string;
  resolution_status: string;
  suggestions: Array<Record<string, any>>;
}

export interface DarkadiaPlatformAnalysis {
  platform_stats: DarkadiaPlatformStatus[];
  unknown_platforms: string[];
  unknown_storefronts: string[];
  platform_suggestions: Record<string, any>;
  total_platforms: number;
  unknown_platform_count: number;
  known_platform_count: number;
}

export interface DarkadiaImportWithPlatformStatus {
  message: string;
  imported_count: number;
  skipped_count: number;
  auto_matched_count: number;
  total_games: number;
  errors: string[];
  platform_analysis: DarkadiaPlatformAnalysis;
  pending_resolutions: number;
  auto_resolved_platforms: number;
}

export interface DarkadiaResolutionSummary {
  total_pending_resolutions: number;
  total_affected_games: number;
  most_common_unresolved: Array<Record<string, any>>;
  suggested_resolutions_available: number;
  recent_resolutions: Array<Record<string, any>>;
}

// Platform/Storefront Resolution Summary Types

export interface DarkadiaResolutionMappingInfo {
  original: string;
  mapped: string;
  game_count: number;
}

export interface DarkadiaResolutionSummaryResponse {
  platforms: DarkadiaResolutionMappingInfo[];
  storefronts: DarkadiaResolutionMappingInfo[];
}

export interface DarkadiaUpdateMappingRequest {
  original_name: string;
  new_mapped_name: string;
  mapping_type: 'platform' | 'storefront';
}

export interface DarkadiaUpdateMappingsRequest {
  mappings: DarkadiaUpdateMappingRequest[];
}

export interface DarkadiaUpdateMappingsResponse {
  message: string;
  updated_mappings: number;
  affected_games: number;
  errors: string[];
}

// Frontend-specific types

export interface DarkadiaUploadState {
  isDragging: boolean;
  isUploading: boolean;
  isImporting: boolean;
  uploadProgress: number;
  importProgress: number;
  uploadedFile: File | null;
  uploadResult: DarkadiaUploadResponse | null;
  error: string | null;
}

export interface DarkadiaFilterState {
  searchQuery: string;
  statusFilter: 'all' | 'unmatched' | 'matched' | 'ignored' | 'synced' | null;
}

export interface DarkadiaBatchSession {
  sessionId: string;
  operationType: 'auto_match' | 'sync';
  isActive: boolean;
  isComplete: boolean;
  status: string;
  totalItems: number;
  processedItems: number;
  successfulItems: number;
  failedItems: number;
  remainingItems: number;
  progressPercentage: number;
  isProcessing: boolean;
  errors: string[];
}

export interface DarkadiaStats {
  totalCount: number;
  unmatchedCount: number;
  matchedCount: number;
  ignoredCount: number;
  syncedCount: number;
}

export interface DarkadiaImportJob {
  id: string;
  status: 'pending' | 'processing' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  total_items: number;
  processed_items: number;
  successful_items: number;
  failed_items: number;
  error_message?: string | undefined;
  started_at?: Date | undefined;
  completed_at?: Date | undefined;
}