import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as jobsApi from '@/api/jobs';
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobItemStatus,
  JobType,
  JobItemDetail,
  RetryFailedResponse,
} from '@/types';
import { isJobInProgress } from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const jobsKeys = {
  all: ['jobs'] as const,
  lists: () => [...jobsKeys.all, 'list'] as const,
  list: (filters?: JobFilters, page?: number, perPage?: number) =>
    [...jobsKeys.lists(), { filters, page, perPage }] as const,
  details: () => [...jobsKeys.all, 'detail'] as const,
  detail: (id: string) => [...jobsKeys.details(), id] as const,
  items: (jobId: string, status?: JobItemStatus, page?: number) =>
    [...jobsKeys.detail(jobId), 'items', { status, page }] as const,
  active: (jobType: JobType) => [...jobsKeys.all, 'active', jobType] as const,
  recent: (source: string, limit?: number) => [...jobsKeys.all, 'recent', source, limit] as const,
};

// ============================================================================
// Query Hooks
// ============================================================================

/**
 * Hook to fetch paginated list of jobs with optional filters.
 * Automatically polls when jobs are in progress.
 */
export function useJobs(
  filters?: JobFilters,
  page: number = 1,
  perPage: number = 20,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: jobsKeys.list(filters, page, perPage),
    queryFn: () => jobsApi.getJobs(filters, page, perPage),
    enabled: options?.enabled,
    refetchInterval: (query) => {
      // Poll every 5 seconds if any jobs are in progress
      const data = query.state.data as JobListResponse | undefined;
      const hasJobsInProgress = data?.jobs.some(isJobInProgress);
      return hasJobsInProgress ? 5000 : false;
    },
  });
}

/**
 * Hook to fetch a specific job by ID.
 * Automatically polls when job is in progress.
 */
export function useJob(jobId: string | undefined, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.detail(jobId || ''),
    queryFn: () => jobsApi.getJob(jobId!),
    enabled: !!jobId && (options?.enabled ?? true),
    refetchInterval: (query) => {
      // Poll every 3 seconds if job is in progress
      const data = query.state.data as Job | undefined;
      if (data && isJobInProgress(data)) {
        return 3000;
      }
      return false;
    },
  });
}

/**
 * Hook to fetch job summary counts for sidebar badge.
 * Returns counts of running and failed jobs.
 */
export function useJobsSummary() {
  return useQuery({
    queryKey: [...jobsKeys.all, 'summary'] as const,
    queryFn: () => jobsApi.getJobsSummary(),
    refetchInterval: 10000, // Poll every 10 seconds for badge updates
  });
}

/**
 * Hook to fetch paginated list of items for a specific job.
 * Useful for viewing details of what a job processed.
 * Supports polling via refetchInterval option.
 */
export function useJobItems(
  jobId: string,
  status?: JobItemStatus,
  page: number = 1,
  pageSize: number = 50,
  options?: { enabled?: boolean; refetchInterval?: number | false }
) {
  return useQuery({
    queryKey: jobsKeys.items(jobId, status, page),
    queryFn: () => jobsApi.getJobItems(jobId, status, page, pageSize),
    enabled: options?.enabled !== false && !!jobId,
    refetchInterval: options?.refetchInterval,
  });
}

/**
 * Hook to check for an active job of a specific type.
 * Polls every 3 seconds when there's an active job.
 * Returns null if no active job exists.
 */
export function useActiveJob(jobType: JobType, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.active(jobType),
    queryFn: () => jobsApi.getActiveJob(jobType),
    enabled: options?.enabled !== false,
    refetchInterval: (query) => {
      // Poll every 3 seconds if there's an active job
      const job = query.state.data as Job | null;
      return job && !job.isTerminal ? 3000 : false;
    },
  });
}

/**
 * Hook to fetch total count of items needing review.
 * Polls every 30 seconds for badge updates.
 */
export function usePendingReviewCount() {
  return useQuery({
    queryKey: [...jobsKeys.all, 'pendingReviewCount'] as const,
    queryFn: () => jobsApi.getPendingReviewCount(),
    refetchInterval: 30000,
  });
}

/**
 * Hook to fetch recent completed jobs for a source with item details.
 */
export function useRecentJobs(source: string, limit: number = 5, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: jobsKeys.recent(source, limit),
    queryFn: () => jobsApi.getRecentJobs(source, limit),
    enabled: options?.enabled !== false,
  });
}

// ============================================================================
// Mutation Hooks
// ============================================================================

/**
 * Hook to cancel a job.
 */
export function useCancelJob() {
  const queryClient = useQueryClient();

  return useMutation<JobCancelResponse, Error, string>({
    mutationFn: (jobId) => jobsApi.cancelJob(jobId),
    onSuccess: (result, jobId) => {
      if (result.success && result.job) {
        // Update the specific job in cache
        queryClient.setQueryData(jobsKeys.detail(jobId), result.job);
      }
      // Invalidate job lists to refetch
      queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
    },
  });
}

/**
 * Hook to delete a job.
 */
export function useDeleteJob() {
  const queryClient = useQueryClient();

  return useMutation<JobDeleteResponse, Error, string>({
    mutationFn: (jobId) => jobsApi.deleteJob(jobId),
    onSuccess: (result, jobId) => {
      if (result.success) {
        // Remove from cache
        queryClient.removeQueries({ queryKey: jobsKeys.detail(jobId) });
        // Invalidate job lists to refetch
        queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
      }
    },
  });
}

/**
 * Hook to resolve a job item to an IGDB ID.
 */
export function useResolveJobItem() {
  const queryClient = useQueryClient();

  return useMutation<JobItemDetail, Error, { itemId: string; igdbId: number }>({
    mutationFn: ({ itemId, igdbId }) => jobsApi.resolveJobItem(itemId, igdbId),
    onSuccess: () => {
      // Invalidate job queries to refresh progress counts
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}

/**
 * Hook to skip a job item.
 */
export function useSkipJobItem() {
  const queryClient = useQueryClient();

  return useMutation<JobItemDetail, Error, { itemId: string; reason?: string }>({
    mutationFn: ({ itemId, reason }) => jobsApi.skipJobItem(itemId, reason),
    onSuccess: () => {
      // Invalidate job queries to refresh progress counts
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}

/**
 * Hook to retry all failed items in a job.
 */
export function useRetryFailedItems() {
  const queryClient = useQueryClient();

  return useMutation<RetryFailedResponse, Error, string>({
    mutationFn: (jobId) => jobsApi.retryFailedItems(jobId),
    onSuccess: (result, jobId) => {
      if (result.success) {
        // Invalidate job detail to refresh progress
        queryClient.invalidateQueries({ queryKey: jobsKeys.detail(jobId) });
        // Invalidate job lists
        queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
      }
    },
  });
}

/**
 * Hook to retry a single failed job item.
 */
export function useRetryJobItem() {
  const queryClient = useQueryClient();

  return useMutation<JobItemDetail, Error, string>({
    mutationFn: (itemId) => jobsApi.retryJobItem(itemId),
    onSuccess: () => {
      // Invalidate all job queries to refresh progress
      queryClient.invalidateQueries({ queryKey: jobsKeys.all });
    },
  });
}
