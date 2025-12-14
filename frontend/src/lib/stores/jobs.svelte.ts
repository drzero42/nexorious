/**
 * Jobs store for managing background job tracking.
 *
 * Provides state management and API methods for listing, viewing,
 * cancelling, and deleting background jobs (sync, import, export).
 */

import { auth } from './auth.svelte';
import { config } from '$lib/env';
import type {
  Job,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobConfirmResponse,
  JobFilters
} from '$lib/types/jobs';
import { JobType, JobSource, JobStatus } from '$lib/types/jobs';

// Re-export types and enums for convenience
export type { Job, JobFilters };
export { JobType, JobSource, JobStatus };

export interface JobsState {
  jobs: Job[];
  currentJob: Job | null;
  isLoading: boolean;
  error: string | null;
  filters: JobFilters;
  pagination: {
    page: number;
    per_page: number;
    total: number;
    pages: number;
  };
}

const initialState: JobsState = {
  jobs: [],
  currentJob: null,
  isLoading: false,
  error: null,
  filters: {},
  pagination: {
    page: 1,
    per_page: 20,
    total: 0,
    pages: 0
  }
};

function createJobsStore() {
  let state = $state<JobsState>({ ...initialState });

  const apiCall = async (url: string, options: RequestInit = {}) => {
    const authState = auth.value;
    if (!authState.accessToken) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${authState.accessToken}`,
        ...options.headers
      }
    });

    if (!response.ok) {
      if (response.status === 401) {
        const refreshed = await auth.refreshAuth();
        if (refreshed) {
          return fetch(url, {
            ...options,
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${auth.value.accessToken}`,
              ...options.headers
            }
          });
        }
      }

      let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      try {
        const errorBody = await response.json();
        if (errorBody.detail) {
          errorMessage = errorBody.detail;
        }
      } catch {
        // Use default message if we can't parse the error body
      }

      throw new Error(errorMessage);
    }

    return response;
  };

  const store = {
    get value() {
      return state;
    },

    /**
     * Load jobs with optional filters and pagination.
     */
    loadJobs: async (
      filters: JobFilters = {},
      page: number = 1,
      per_page: number = 20
    ) => {
      state.isLoading = true;
      state.error = null;

      try {
        const params = new URLSearchParams();

        if (filters.job_type) params.append('job_type', filters.job_type);
        if (filters.source) params.append('source', filters.source);
        if (filters.status) params.append('status', filters.status);
        if (filters.sort_by) params.append('sort_by', filters.sort_by);
        if (filters.sort_order) params.append('sort_order', filters.sort_order);

        params.append('page', page.toString());
        params.append('per_page', per_page.toString());

        const response = await apiCall(`${config.apiUrl}/jobs/?${params}`);
        const data: JobListResponse = await response.json();

        state.jobs = data.jobs;
        state.filters = filters;
        state.pagination = {
          page: data.page,
          per_page: data.per_page,
          total: data.total,
          pages: data.pages
        };
        state.isLoading = false;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load jobs';
        state.isLoading = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Get a specific job by ID.
     */
    getJob: async (jobId: string) => {
      // Check if we already have it
      const existing = state.jobs.find((j) => j.id === jobId);
      if (existing) {
        state.currentJob = existing;
      }

      state.isLoading = true;
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/jobs/${jobId}`);
        const job: Job = await response.json();

        state.currentJob = job;

        // Update in the list if present
        const index = state.jobs.findIndex((j) => j.id === jobId);
        if (index !== -1) {
          state.jobs[index] = job;
        }

        state.isLoading = false;
        return job;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to load job';
        state.isLoading = false;
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Cancel a job that is not in a terminal state.
     */
    cancelJob: async (jobId: string): Promise<JobCancelResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/jobs/${jobId}/cancel`, {
          method: 'POST'
        });
        const result: JobCancelResponse = await response.json();

        if (result.success && result.job) {
          // Update in the list
          const index = state.jobs.findIndex((j) => j.id === jobId);
          if (index !== -1) {
            state.jobs[index] = result.job;
          }

          // Update current job if it's the same
          if (state.currentJob?.id === jobId) {
            state.currentJob = result.job;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to cancel job';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Delete a job that is in a terminal state.
     */
    deleteJob: async (jobId: string): Promise<JobDeleteResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/jobs/${jobId}`, {
          method: 'DELETE'
        });
        const result: JobDeleteResponse = await response.json();

        if (result.success) {
          // Remove from the list
          state.jobs = state.jobs.filter((j) => j.id !== jobId);

          // Clear current job if it's the same
          if (state.currentJob?.id === jobId) {
            state.currentJob = null;
          }

          // Update pagination
          state.pagination.total = Math.max(0, state.pagination.total - 1);
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to delete job';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Confirm an import job after all review items are resolved.
     */
    confirmJob: async (jobId: string): Promise<JobConfirmResponse> => {
      state.error = null;

      try {
        const response = await apiCall(`${config.apiUrl}/jobs/${jobId}/confirm`, {
          method: 'POST'
        });
        const result: JobConfirmResponse = await response.json();

        if (result.success && result.job) {
          // Update in the list
          const index = state.jobs.findIndex((j) => j.id === jobId);
          if (index !== -1) {
            state.jobs[index] = result.job;
          }

          // Update current job if it's the same
          if (state.currentJob?.id === jobId) {
            state.currentJob = result.job;
          }
        }

        return result;
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Failed to confirm job';
        state.error = errorMessage;
        throw error;
      }
    },

    /**
     * Refresh the current job's data from the server.
     */
    refreshCurrentJob: async () => {
      if (!state.currentJob) return null;
      return store.getJob(state.currentJob.id);
    },

    /**
     * Refresh the jobs list with current filters.
     */
    refresh: async () => {
      return store.loadJobs(state.filters, state.pagination.page, state.pagination.per_page);
    },

    /**
     * Clear the current job selection.
     */
    clearCurrentJob: () => {
      state.currentJob = null;
    },

    /**
     * Clear filters.
     */
    clearFilters: () => {
      state.filters = {};
    },

    /**
     * Clear error state.
     */
    clearError: () => {
      state.error = null;
    },

    /**
     * Reset store to initial state.
     */
    reset: () => {
      state = { ...initialState };
    }
  };

  return store;
}

export const jobs = createJobsStore();
