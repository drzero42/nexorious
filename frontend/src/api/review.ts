import { api } from './client';
import type {
  ReviewItem,
  ReviewFilters,
  ReviewListResponse,
  ReviewSummary,
  ReviewCountsByType,
  MatchResponse,
  PlatformSummaryResponse,
  FinalizeImportResponse,
  IGDBCandidate,
  ReviewItemStatus,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface IGDBCandidateApiResponse {
  igdb_id: number;
  name: string;
  first_release_date: number | null;
  cover_url: string | null;
  summary: string | null;
  platforms: string[] | null;
  similarity_score: number | null;
}

interface ReviewItemApiResponse {
  id: string;
  job_id: string;
  user_id: string;
  status: string;
  source_title: string;
  source_metadata: Record<string, unknown>;
  igdb_candidates: IGDBCandidateApiResponse[];
  resolved_igdb_id: number | null;
  match_confidence: number | null;
  created_at: string;
  resolved_at: string | null;
  job_type: string | null;
  job_source: string | null;
}

interface ReviewListApiResponse {
  items: ReviewItemApiResponse[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

interface ReviewSummaryApiResponse {
  total_pending: number;
  total_matched: number;
  total_skipped: number;
  total_removal: number;
  jobs_with_pending: number;
}

interface ReviewCountsByTypeApiResponse {
  import_pending: number;
  sync_pending: number;
}

interface MatchResponseApiResponse {
  success: boolean;
  message: string;
  item: ReviewItemApiResponse | null;
}

interface PlatformMappingSuggestionApiResponse {
  original: string;
  count: number;
  suggested_id: string | null;
  suggested_name: string | null;
}

interface PlatformSummaryApiResponse {
  platforms: PlatformMappingSuggestionApiResponse[];
  storefronts: PlatformMappingSuggestionApiResponse[];
  all_resolved: boolean;
}

interface FinalizeImportApiResponse {
  success: boolean;
  message: string;
  games_created: number;
  games_skipped: number;
  games_failed: number;
  errors: string[];
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformIGDBCandidate(apiCandidate: IGDBCandidateApiResponse): IGDBCandidate {
  return {
    igdbId: apiCandidate.igdb_id,
    name: apiCandidate.name,
    firstReleaseDate: apiCandidate.first_release_date,
    coverUrl: apiCandidate.cover_url,
    summary: apiCandidate.summary,
    platforms: apiCandidate.platforms,
    similarityScore: apiCandidate.similarity_score,
  };
}

function transformReviewItem(apiItem: ReviewItemApiResponse): ReviewItem {
  return {
    id: apiItem.id,
    jobId: apiItem.job_id,
    userId: apiItem.user_id,
    status: apiItem.status as ReviewItemStatus,
    sourceTitle: apiItem.source_title,
    sourceMetadata: apiItem.source_metadata,
    igdbCandidates: (apiItem.igdb_candidates || []).map(transformIGDBCandidate),
    resolvedIgdbId: apiItem.resolved_igdb_id,
    matchConfidence: apiItem.match_confidence,
    createdAt: apiItem.created_at,
    resolvedAt: apiItem.resolved_at,
    jobType: apiItem.job_type,
    jobSource: apiItem.job_source,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get paginated list of review items with optional filters.
 */
export async function getReviewItems(
  filters?: ReviewFilters,
  page: number = 1,
  perPage: number = 20
): Promise<ReviewListResponse> {
  const params: Record<string, string | number> = {
    page,
    per_page: perPage,
  };

  if (filters?.status) params.status = filters.status;
  if (filters?.jobId) params.job_id = filters.jobId;
  if (filters?.source) params.source = filters.source;

  const response = await api.get<ReviewListApiResponse>('/review/', { params });

  return {
    items: response.items.map(transformReviewItem),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}

/**
 * Get a specific review item by ID.
 */
export async function getReviewItem(itemId: string): Promise<ReviewItem> {
  const response = await api.get<ReviewItemApiResponse>(`/review/${itemId}`);
  return transformReviewItem(response);
}

/**
 * Get review summary statistics.
 */
export async function getReviewSummary(): Promise<ReviewSummary> {
  const response = await api.get<ReviewSummaryApiResponse>('/review/summary');
  return {
    totalPending: response.total_pending,
    totalMatched: response.total_matched,
    totalSkipped: response.total_skipped,
    totalRemoval: response.total_removal,
    jobsWithPending: response.jobs_with_pending,
  };
}

/**
 * Get pending review counts grouped by job type.
 */
export async function getReviewCountsByType(): Promise<ReviewCountsByType> {
  const response = await api.get<ReviewCountsByTypeApiResponse>('/review/counts');
  return {
    importPending: response.import_pending,
    syncPending: response.sync_pending,
  };
}

/**
 * Match a review item to an IGDB ID.
 */
export async function matchReviewItem(itemId: string, igdbId: number): Promise<MatchResponse> {
  const response = await api.post<MatchResponseApiResponse>(`/review/${itemId}/match`, {
    igdb_id: igdbId,
  });
  return {
    success: response.success,
    message: response.message,
    item: response.item ? transformReviewItem(response.item) : null,
  };
}

/**
 * Skip a review item without matching.
 */
export async function skipReviewItem(itemId: string): Promise<MatchResponse> {
  const response = await api.post<MatchResponseApiResponse>(`/review/${itemId}/skip`);
  return {
    success: response.success,
    message: response.message,
    item: response.item ? transformReviewItem(response.item) : null,
  };
}

/**
 * Keep a game flagged for removal in the collection.
 */
export async function keepReviewItem(itemId: string): Promise<MatchResponse> {
  const response = await api.post<MatchResponseApiResponse>(`/review/${itemId}/keep`);
  return {
    success: response.success,
    message: response.message,
    item: response.item ? transformReviewItem(response.item) : null,
  };
}

/**
 * Remove a game flagged for removal from the collection.
 */
export async function removeReviewItem(itemId: string): Promise<MatchResponse> {
  const response = await api.post<MatchResponseApiResponse>(`/review/${itemId}/remove`);
  return {
    success: response.success,
    message: response.message,
    item: response.item ? transformReviewItem(response.item) : null,
  };
}

/**
 * Get platform summary for a job (for platform/storefront mapping).
 */
export async function getPlatformSummary(jobId: string): Promise<PlatformSummaryResponse> {
  const response = await api.get<PlatformSummaryApiResponse>('/review/platform-summary', {
    params: { job_id: jobId },
  });
  return {
    platforms: response.platforms.map((p) => ({
      original: p.original,
      count: p.count,
      suggestedId: p.suggested_id,
      suggestedName: p.suggested_name,
    })),
    storefronts: response.storefronts.map((s) => ({
      original: s.original,
      count: s.count,
      suggestedId: s.suggested_id,
      suggestedName: s.suggested_name,
    })),
    allResolved: response.all_resolved,
  };
}

/**
 * Finalize an import job with platform/storefront mappings.
 */
export async function finalizeImport(
  jobId: string,
  platformMappings: Record<string, string>,
  storefrontMappings: Record<string, string>
): Promise<FinalizeImportResponse> {
  const response = await api.post<FinalizeImportApiResponse>('/review/finalize', {
    job_id: jobId,
    platform_mappings: Object.entries(platformMappings).map(([original, resolvedId]) => ({
      original,
      resolved_id: resolvedId,
    })),
    storefront_mappings: Object.entries(storefrontMappings).map(([original, resolvedId]) => ({
      original,
      resolved_id: resolvedId,
    })),
  });
  return {
    success: response.success,
    message: response.message,
    gamesCreated: response.games_created,
    gamesSkipped: response.games_skipped,
    gamesFailed: response.games_failed,
    errors: response.errors,
  };
}
