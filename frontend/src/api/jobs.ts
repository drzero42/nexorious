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
  JobItem,
  JobItemStatus,
  JobItemListResponse,
  PendingReviewCountResponse,
  JobItemDetail,
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

interface JobItemApiResponse {
  id: string;
  job_id: string;
  item_key: string;
  source_title: string;
  status: string;
  error_message: string | null;
  result_game_title: string | null;
  result_igdb_id: number | null;
  created_at: string;
  processed_at: string | null;
}

interface JobItemListApiResponse {
  items: JobItemApiResponse[];
  total: number;
  page: number;
  page_size: number;
  pages: number;
}

interface PendingReviewCountApiResponse {
  pending_review_count: number;
}

interface JobItemDetailApiResponse extends JobItemApiResponse {
  source_metadata_json: string;
  result_json: string;
  igdb_candidates_json: string;
  resolved_igdb_id: number | null;
  resolved_at: string | null;
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

function transformJobItem(apiItem: JobItemApiResponse): JobItem {
  return {
    id: apiItem.id,
    jobId: apiItem.job_id,
    itemKey: apiItem.item_key,
    sourceTitle: apiItem.source_title,
    status: apiItem.status as JobItemStatus,
    errorMessage: apiItem.error_message,
    resultGameTitle: apiItem.result_game_title,
    resultIgdbId: apiItem.result_igdb_id,
    createdAt: apiItem.created_at,
    processedAt: apiItem.processed_at,
  };
}

function transformJobItemDetail(apiItem: JobItemDetailApiResponse): JobItemDetail {
  return {
    ...transformJobItem(apiItem),
    sourceMetadataJson: apiItem.source_metadata_json,
    resultJson: apiItem.result_json,
    igdbCandidatesJson: apiItem.igdb_candidates_json,
    resolvedIgdbId: apiItem.resolved_igdb_id,
    resolvedAt: apiItem.resolved_at,
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

/**
 * Get paginated list of items for a specific job.
 */
export async function getJobItems(
  jobId: string,
  status?: JobItemStatus,
  page: number = 1,
  pageSize: number = 50
): Promise<JobItemListResponse> {
  const params: Record<string, string | number> = { page, page_size: pageSize };
  if (status) params.status = status;

  const response = await api.get<JobItemListApiResponse>(
    `/jobs/${jobId}/items`,
    { params }
  );

  return {
    items: response.items.map(transformJobItem),
    total: response.total,
    page: response.page,
    pageSize: response.page_size,
    pages: response.pages,
  };
}

/**
 * Get the currently active job for a specific job type.
 * Returns null if no active job exists.
 */
export async function getActiveJob(jobType: JobType): Promise<Job | null> {
  const response = await api.get<JobApiResponse | null>(`/jobs/active/${jobType}`);
  return response ? transformJob(response) : null;
}

/**
 * Get total count of items needing review across all jobs.
 * Used for nav badge display.
 */
export async function getPendingReviewCount(): Promise<PendingReviewCountResponse> {
  const response = await api.get<PendingReviewCountApiResponse>('/jobs/pending-review-count');
  return { pendingReviewCount: response.pending_review_count };
}

/**
 * Resolve a job item to an IGDB game.
 */
export async function resolveJobItem(itemId: string, igdbId: number): Promise<JobItemDetail> {
  const response = await api.post<JobItemDetailApiResponse>(
    `/job-items/${itemId}/resolve`,
    { igdb_id: igdbId }
  );
  return transformJobItemDetail(response);
}

/**
 * Skip a job item without matching.
 */
export async function skipJobItem(itemId: string, reason?: string): Promise<JobItemDetail> {
  const response = await api.post<JobItemDetailApiResponse>(
    `/job-items/${itemId}/skip`,
    { reason }
  );
  return transformJobItemDetail(response);
}
