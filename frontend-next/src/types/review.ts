/**
 * Types for review queue management.
 * Review items are games from imports/syncs that need user matching decisions.
 */

// ============================================================================
// Enums
// ============================================================================

export enum ReviewItemStatus {
  PENDING = 'pending',
  MATCHED = 'matched',
  SKIPPED = 'skipped',
  REMOVAL = 'removal',
}

export enum ReviewSource {
  IMPORT = 'import',
  SYNC = 'sync',
}

// ============================================================================
// Interfaces
// ============================================================================

export interface IGDBCandidate {
  igdbId: number;
  name: string;
  firstReleaseDate: number | null;
  coverUrl: string | null;
  summary: string | null;
  platforms: string[] | null;
  similarityScore: number | null;
}

export interface ReviewItem {
  id: string;
  jobId: string;
  userId: string;
  status: ReviewItemStatus;
  sourceTitle: string;
  sourceMetadata: Record<string, unknown>;
  igdbCandidates: IGDBCandidate[];
  resolvedIgdbId: number | null;
  matchConfidence: number | null;
  createdAt: string;
  resolvedAt: string | null;
  jobType: string | null;
  jobSource: string | null;
}

export interface ReviewFilters {
  status?: ReviewItemStatus;
  jobId?: string;
  source?: ReviewSource;
}

export interface ReviewListResponse {
  items: ReviewItem[];
  total: number;
  page: number;
  perPage: number;
  pages: number;
}

export interface ReviewSummary {
  totalPending: number;
  totalMatched: number;
  totalSkipped: number;
  totalRemoval: number;
  jobsWithPending: number;
}

export interface ReviewCountsByType {
  importPending: number;
  syncPending: number;
}

export interface MatchResponse {
  success: boolean;
  message: string;
  item: ReviewItem | null;
}

export interface PlatformMappingSuggestion {
  original: string;
  count: number;
  suggestedId: string | null;
  suggestedName: string | null;
}

export interface PlatformSummaryResponse {
  platforms: PlatformMappingSuggestion[];
  storefronts: PlatformMappingSuggestion[];
  allResolved: boolean;
}

export interface PlatformMapping {
  original: string;
  resolvedId: string;
}

export interface FinalizeImportRequest {
  jobId: string;
  platformMappings: PlatformMapping[];
  storefrontMappings: PlatformMapping[];
}

export interface FinalizeImportResponse {
  success: boolean;
  message: string;
  gamesCreated: number;
  gamesSkipped: number;
  gamesFailed: number;
  errors: string[];
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Get a human-readable label for a review item status.
 */
export function getReviewStatusLabel(status: ReviewItemStatus): string {
  const labels: Record<ReviewItemStatus, string> = {
    [ReviewItemStatus.PENDING]: 'Pending',
    [ReviewItemStatus.MATCHED]: 'Matched',
    [ReviewItemStatus.SKIPPED]: 'Skipped',
    [ReviewItemStatus.REMOVAL]: 'Removed',
  };
  return labels[status] || status;
}

/**
 * Get badge variant for a review item status.
 */
export function getReviewStatusVariant(
  status: ReviewItemStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case ReviewItemStatus.MATCHED:
      return 'default';
    case ReviewItemStatus.PENDING:
      return 'secondary';
    case ReviewItemStatus.REMOVAL:
      return 'destructive';
    case ReviewItemStatus.SKIPPED:
    default:
      return 'outline';
  }
}

/**
 * Check if a review item is a removal detection.
 */
export function isRemovalItem(item: ReviewItem): boolean {
  return item.sourceMetadata?.removal_detected === true;
}

/**
 * Format a Unix timestamp to a year string.
 */
export function formatReleaseYear(timestamp: number | null): string {
  if (!timestamp) return '';
  const date = new Date(timestamp * 1000);
  return `(${date.getFullYear()})`;
}
