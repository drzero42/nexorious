import { api } from './client';
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobsSummary,
  JobType,
  JobSource,
  JobStatus,
  JobPriority,
  JobProgress,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface JobProgressApiResponse {
  pending: number;
  processing: number;
  completed: number;
  pending_review: number;
  skipped: number;
  failed: number;
  total: number;
  percent: number;
}

interface JobApiResponse {
  id: string;
  user_id: string;
  job_type: string;
  source: string;
  status: string;
  priority: string;
  progress: JobProgressApiResponse;
  total_items: number;
  error_message: string | null;
  file_path: string | null;
  created_at: string;
  started_at: string | null;
  completed_at: string | null;
  is_terminal: boolean;
  duration_seconds: number | null;
}

interface JobListApiResponse {
  jobs: JobApiResponse[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

interface JobCancelApiResponse {
  success: boolean;
  message: string;
  job: JobApiResponse | null;
}

interface JobDeleteApiResponse {
  success: boolean;
  message: string;
  deleted_job_id: string;
}

interface JobsSummaryApiResponse {
  running_count: number;
  failed_count: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformProgress(apiProgress: JobProgressApiResponse): JobProgress {
  return {
    pending: apiProgress.pending,
    processing: apiProgress.processing,
    completed: apiProgress.completed,
    pendingReview: apiProgress.pending_review,
    skipped: apiProgress.skipped,
    failed: apiProgress.failed,
    total: apiProgress.total,
    percent: apiProgress.percent,
  };
}

function transformJob(apiJob: JobApiResponse): Job {
  return {
    id: apiJob.id,
    userId: apiJob.user_id,
    jobType: apiJob.job_type as JobType,
    source: apiJob.source as JobSource,
    status: apiJob.status as JobStatus,
    priority: apiJob.priority as JobPriority,
    progress: transformProgress(apiJob.progress),
    totalItems: apiJob.total_items,
    errorMessage: apiJob.error_message,
    filePath: apiJob.file_path,
    createdAt: apiJob.created_at,
    startedAt: apiJob.started_at,
    completedAt: apiJob.completed_at,
    isTerminal: apiJob.is_terminal,
    durationSeconds: apiJob.duration_seconds,
  };
}

// ============================================================================
// API Functions
// ============================================================================

/**
 * Get paginated list of jobs with optional filters.
 */
export async function getJobs(
  filters?: JobFilters,
  page: number = 1,
  perPage: number = 20
): Promise<JobListResponse> {
  const params: Record<string, string | number> = {
    page,
    per_page: perPage,
  };

  if (filters?.jobType) params.job_type = filters.jobType;
  if (filters?.source) params.source = filters.source;
  if (filters?.status) params.status = filters.status;
  if (filters?.sortBy) params.sort_by = filters.sortBy;
  if (filters?.sortOrder) params.sort_order = filters.sortOrder;

  const response = await api.get<JobListApiResponse>('/jobs/', { params });

  return {
    jobs: response.jobs.map(transformJob),
    total: response.total,
    page: response.page,
    perPage: response.per_page,
    pages: response.pages,
  };
}

/**
 * Get a specific job by ID.
 */
export async function getJob(jobId: string): Promise<Job> {
  const response = await api.get<JobApiResponse>(`/jobs/${jobId}`);
  return transformJob(response);
}

/**
 * Cancel a job that is not in a terminal state.
 */
export async function cancelJob(jobId: string): Promise<JobCancelResponse> {
  const response = await api.post<JobCancelApiResponse>(`/jobs/${jobId}/cancel`);
  return {
    success: response.success,
    message: response.message,
    job: response.job ? transformJob(response.job) : null,
  };
}

/**
 * Delete a job that is in a terminal state.
 */
export async function deleteJob(jobId: string): Promise<JobDeleteResponse> {
  const response = await api.delete<JobDeleteApiResponse>(`/jobs/${jobId}`);
  return {
    success: response.success,
    message: response.message,
    deletedJobId: response.deleted_job_id,
  };
}

/**
 * Get summary counts for jobs (running and failed).
 * This is a lightweight endpoint for sidebar badge display.
 */
export async function getJobsSummary(): Promise<JobsSummary> {
  const response = await api.get<JobsSummaryApiResponse>('/jobs/summary');
  return {
    runningCount: response.running_count,
    failedCount: response.failed_count,
  };
}
