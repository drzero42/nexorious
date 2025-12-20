import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import * as jobsApi from '@/api/jobs';
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobConfirmResponse,
  JobsSummary,
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
 * Hook to confirm an import job.
 */
export function useConfirmJob() {
  const queryClient = useQueryClient();

  return useMutation<JobConfirmResponse, Error, string>({
    mutationFn: (jobId) => jobsApi.confirmJob(jobId),
    onSuccess: (result, jobId) => {
      if (result.success && result.job) {
        // Update the specific job in cache
        queryClient.setQueryData(jobsKeys.detail(jobId), result.job);
      }
      // Invalidate job lists and game collection
      queryClient.invalidateQueries({ queryKey: jobsKeys.lists() });
      queryClient.invalidateQueries({ queryKey: ['games'] });
    },
  });
}
