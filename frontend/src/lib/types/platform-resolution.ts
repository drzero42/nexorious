/**
 * Platform Resolution Types
 * 
 * TypeScript interfaces for platform resolution functionality, matching backend schemas
 * from backend/app/api/schemas/platform.py
 */

export interface PlatformSuggestion {
  platform_id: string;
  platform_name: string;
  platform_display_name: string;
  confidence: number;
  match_type: 'exact' | 'fuzzy' | 'partial';
  reason: string;
}

export interface StorefrontSuggestion {
  storefront_id: string;
  storefront_name: string;
  storefront_display_name: string;
  confidence: number;
  match_type: 'exact' | 'fuzzy' | 'partial';
  reason: string;
}

export interface PlatformResolutionData {
  status: 'pending' | 'suggested' | 'resolved' | 'failed';
  original_name: string;
  suggestions: PlatformSuggestion[];
  storefront_suggestions: StorefrontSuggestion[];
  resolved_platform_id?: string;
  resolved_storefront_id?: string;
  resolution_timestamp?: string;
  resolution_method?: 'auto' | 'manual' | 'admin_created';
  user_notes?: string;
}

export interface PendingPlatformResolution {
  import_id: string;
  user_id: string;
  original_platform_name: string;
  original_storefront_name?: string;
  affected_games_count: number;
  affected_games: string[];
  resolution_data: PlatformResolutionData;
  created_at: string;
}

// Request/Response types for API calls

export interface PlatformSuggestionsRequest {
  unknown_platform_name: string;
  unknown_storefront_name?: string;
  min_confidence?: number;
  max_suggestions?: number;
}

export interface PlatformSuggestionsResponse {
  unknown_platform_name: string;
  unknown_storefront_name?: string;
  platform_suggestions: PlatformSuggestion[];
  storefront_suggestions: StorefrontSuggestion[];
  total_platform_suggestions: number;
  total_storefront_suggestions: number;
}

export interface PlatformResolutionRequest {
  import_id: string;
  resolved_platform_id?: string;
  resolved_storefront_id?: string;
  user_notes?: string;
}

export interface BulkPlatformResolutionRequest {
  resolutions: PlatformResolutionRequest[];
}

export interface PlatformResolutionResult {
  import_id: string;
  success: boolean;
  resolved_platform?: {
    id: string;
    name: string;
    display_name: string;
    icon_url?: string;
  };
  resolved_storefront?: {
    id: string;
    name: string;
    display_name: string;
    icon_url?: string;
  };
  error_message?: string;
}

export interface BulkPlatformResolutionResponse {
  total_processed: number;
  successful_resolutions: number;
  failed_resolutions: number;
  results: PlatformResolutionResult[];
  errors: string[];
}

// Storefront Resolution Types (Task 15)

export interface PendingStorefrontResolution {
  import_id: string;
  user_id: string;
  original_platform_name?: string;
  original_storefront_name: string;
  platform_context?: string; // e.g., "PC-Windows"
  platform_id?: string; // If platform is already resolved
  affected_games_count: number;
  affected_games: string[];
  resolution_data: StorefrontResolutionData;
  created_at: string;
}

export interface StorefrontResolutionData {
  status: 'pending' | 'suggested' | 'resolved' | 'failed';
  original_name: string;
  suggestions: StorefrontSuggestion[];
  resolved_storefront_id?: string;
  resolution_timestamp?: string;
  resolution_method?: 'auto' | 'manual' | 'admin_created';
  user_notes?: string;
  platform_context?: string;
}

export interface StorefrontSuggestionsRequest {
  unknown_storefront_name: string;
  platform_id?: string | undefined;
  platform_name?: string | undefined;
  min_confidence?: number;
  max_suggestions?: number;
}

export interface StorefrontSuggestionsResponse {
  unknown_storefront_name: string;
  platform_context?: string;
  storefront_suggestions: StorefrontSuggestion[];
  total_suggestions: number;
}

export interface StorefrontResolutionRequest {
  import_id: string;
  resolved_storefront_id?: string;
  user_notes?: string | undefined;
}

export interface StorefrontResolutionResult {
  import_id: string;
  success: boolean;
  resolved_storefront?: {
    id: string;
    name: string;
    display_name: string;
    icon_url?: string;
  };
  error_message?: string;
}

export interface BulkStorefrontResolutionRequest {
  resolutions: StorefrontResolutionRequest[];
}

export interface BulkStorefrontResolutionResponse {
  total_processed: number;
  successful_resolutions: number;
  failed_resolutions: number;
  results: StorefrontResolutionResult[];
  errors: string[];
}

export interface PendingStorefrontsListResponse {
  pending_resolutions: PendingStorefrontResolution[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface PendingResolutionsListResponse {
  pending_resolutions: PendingPlatformResolution[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

// Frontend-specific types for UI state management

export interface PlatformResolutionUIState {
  isOpen: boolean;
  isLoading: boolean;
  pendingResolutions: PendingPlatformResolution[];
  selectedResolutions: Set<string>; // Set of import_ids
  bulkOperationInProgress: boolean;
  error?: string;
  successMessage?: string;
}

export interface PlatformMappingRowState {
  isLoadingSuggestions: boolean;
  isResolving: boolean;
  showCreateForm: boolean;
  expanded: boolean;
  selectedSuggestion?: PlatformSuggestion;
}

export interface StorefrontResolutionUIState {
  isOpen: boolean;
  isLoading: boolean;
  pendingResolutions: PendingStorefrontResolution[];
  selectedResolutions: Set<string>; // Set of import_ids
  bulkOperationInProgress: boolean;
  error?: string;
  successMessage?: string;
}

export interface StorefrontMappingRowState {
  isLoadingSuggestions: boolean;
  isResolving: boolean;
  showCreateForm: boolean;
  expanded: boolean;
  selectedSuggestion?: StorefrontSuggestion;
}

export interface PlatformCreationFormData {
  name: string;
  display_name: string;
  icon_url?: string;
}

export interface ResolutionAction {
  type: 'resolve' | 'create' | 'skip';
  import_id: string;
  platform_id?: string;
  storefront_id?: string;
  platform_data?: PlatformCreationFormData;
  storefront_data?: StorefrontCreationFormData;
  user_notes?: string;
}

export interface StorefrontCreationFormData {
  name: string;
  display_name: string;
  icon_url?: string;
  base_url?: string;
}

// Utility types for confidence visualization

export type ConfidenceLevel = 'high' | 'medium' | 'low';

export interface ConfidenceThresholds {
  high: number; // >= 0.8
  medium: number; // >= 0.6
  low: number; // < 0.6
}

// Error types specific to platform resolution

export interface PlatformResolutionError {
  code: string;
  message: string;
  import_id?: string;
  platform_name?: string;
  details?: Record<string, any>;
}