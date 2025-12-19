import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getReviewItems,
  getReviewItem,
  getReviewSummary,
  getReviewCountsByType,
  matchReviewItem,
  skipReviewItem,
  keepReviewItem,
  removeReviewItem,
  getPlatformSummary,
  finalizeImport,
} from '@/api';
import type { ReviewFilters, MatchResponse, FinalizeImportResponse } from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const reviewKeys = {
  all: ['review'] as const,
  items: (filters?: ReviewFilters, page?: number) =>
    [...reviewKeys.all, 'items', { filters, page }] as const,
  item: (id: string) => [...reviewKeys.all, 'item', id] as const,
  summary: () => [...reviewKeys.all, 'summary'] as const,
  countsByType: () => [...reviewKeys.all, 'countsByType'] as const,
  platformSummary: (jobId: string) => [...reviewKeys.all, 'platformSummary', jobId] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch paginated review items with filters.
 */
export function useReviewItems(filters?: ReviewFilters, page: number = 1, perPage: number = 20) {
  return useQuery({
    queryKey: reviewKeys.items(filters, page),
    queryFn: () => getReviewItems(filters, page, perPage),
  });
}

/**
 * Hook to fetch a single review item by ID.
 */
export function useReviewItem(itemId: string) {
  return useQuery({
    queryKey: reviewKeys.item(itemId),
    queryFn: () => getReviewItem(itemId),
    enabled: !!itemId,
  });
}

/**
 * Hook to fetch review summary statistics.
 */
export function useReviewSummary() {
  return useQuery({
    queryKey: reviewKeys.summary(),
    queryFn: getReviewSummary,
  });
}

/**
 * Hook to fetch review counts by type (import/sync).
 */
export function useReviewCountsByType() {
  return useQuery({
    queryKey: reviewKeys.countsByType(),
    queryFn: getReviewCountsByType,
  });
}

/**
 * Hook to fetch platform summary for a job.
 */
export function usePlatformSummary(jobId: string | null) {
  return useQuery({
    queryKey: reviewKeys.platformSummary(jobId || ''),
    queryFn: () => getPlatformSummary(jobId!),
    enabled: !!jobId,
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to match a review item to an IGDB ID.
 */
export function useMatchReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ itemId, igdbId }: { itemId: string; igdbId: number }): Promise<MatchResponse> =>
      matchReviewItem(itemId, igdbId),
    onSuccess: () => {
      // Invalidate all review queries to refresh data
      queryClient.invalidateQueries({ queryKey: reviewKeys.all });
    },
  });
}

/**
 * Hook to skip a review item.
 */
export function useSkipReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemId: string): Promise<MatchResponse> => skipReviewItem(itemId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: reviewKeys.all });
    },
  });
}

/**
 * Hook to keep a review item (for removal items).
 */
export function useKeepReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemId: string): Promise<MatchResponse> => keepReviewItem(itemId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: reviewKeys.all });
    },
  });
}

/**
 * Hook to remove a review item (for removal items).
 */
export function useRemoveReviewItem() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (itemId: string): Promise<MatchResponse> => removeReviewItem(itemId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: reviewKeys.all });
    },
  });
}

/**
 * Hook to finalize an import job.
 */
export function useFinalizeImport() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      jobId,
      platformMappings,
      storefrontMappings,
    }: {
      jobId: string;
      platformMappings: Record<string, string>;
      storefrontMappings: Record<string, string>;
    }): Promise<FinalizeImportResponse> => finalizeImport(jobId, platformMappings, storefrontMappings),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: reviewKeys.all });
      // Also invalidate jobs queries since the job status changes
      queryClient.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
}
