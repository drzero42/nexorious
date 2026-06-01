import { api } from './client';
import type {
  Job,
  JobFilters,
  JobListResponse,
  JobCancelResponse,
  JobDeleteResponse,
  JobsSummary,
  JobTypeStatus,
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
  RetryFailedResponse,
  SyncChangeItem,
  RecentJobDetail,
  RecentJobsResponse,
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

interface JobTypeStatusApiResponse {
  is_active: boolean;
  active_job_id: string | null;
  last_completed_job_id: string | null;
  last_completed_at: string | null;
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
  result_user_game_id: string | null;
  created_at: string;
  processed_at: string | null;
  igdb_candidates_json?: string; // Optional - present for PENDING_REVIEW items
  external_game_id: string | null;
}

interface JobItemListApiResponse {
  items: JobItemApiResponse[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

interface PendingReviewCountApiResponse {
  pending_review_count: number;
  counts_by_source: Record<string, number>;
}

interface JobItemDetailApiResponse extends JobItemApiResponse {
  source_metadata_json: string;
  result_json: string;
  igdb_candidates_json: string;
  resolved_igdb_id: number | null;
  resolved_at: string | null;
}

interface RetryFailedApiResponse {
  success: boolean;
  message: string;
  retried_count: number;
}

interface SyncChangeItemApiResponse {
  title: string;
  old_status?: string | null;
  new_status?: string | null;
}

interface RecentJobDetailApiResponse {
  id: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  total_items: number;
  progress: {
    completed: number;
    skipped: number;
    failed: number;
    pending: number;
    processing: number;
    pending_review: number;
    total: number;
    percent: number;
  };
  added_items: SyncChangeItemApiResponse[];
  removed_items: SyncChangeItemApiResponse[];
  status_changed_items: SyncChangeItemApiResponse[];
  skipped_items?: SyncChangeItemApiResponse[];
  already_in_library_items?: SyncChangeItemApiResponse[];
}

interface RecentJobsApiResponse {
  jobs: RecentJobDetailApiResponse[];
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

export function transformJobItem(apiItem: JobItemApiResponse): JobItem {
  return {
    id: apiItem.id,
    jobId: apiItem.job_id,
    itemKey: apiItem.item_key,
    sourceTitle: apiItem.source_title,
    status: apiItem.status as JobItemStatus,
    errorMessage: apiItem.error_message,
    resultGameTitle: apiItem.result_game_title,
    resultIgdbId: apiItem.result_igdb_id,
    resultUserGameId: apiItem.result_user_game_id,
    createdAt: apiItem.created_at,
    processedAt: apiItem.processed_at,
    igdbCandidatesJson: apiItem.igdb_candidates_json,
    externalGameId: apiItem.external_game_id,
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

function transformSyncChangeItem(sc: SyncChangeItemApiResponse): SyncChangeItem {
  return {
    title: sc.title,
    oldStatus: sc.old_status ?? null,
    newStatus: sc.new_status ?? null,
  };
}

function transformRecentJob(api: RecentJobDetailApiResponse): RecentJobDetail {
  const p = api.progress ?? {
    completed: 0,
    skipped: 0,
    failed: 0,
    pending: 0,
    processing: 0,
    pending_review: 0,
    total: 0,
    percent: 0,
  };
  return {
    id: api.id,
    status: api.status,
    createdAt: api.created_at,
    completedAt: api.completed_at,
    totalItems: api.total_items,
    completedCount: p.completed,
    skippedCount: p.skipped,
    failedCount: p.failed,
    addedItems: (api.added_items ?? []).map(transformSyncChangeItem),
    removedItems: (api.removed_items ?? []).map(transformSyncChangeItem),
    statusChangedItems: (api.status_changed_items ?? []).map(transformSyncChangeItem),
    skippedItems: (api.skipped_items ?? []).map(transformSyncChangeItem),
    alreadyInLibraryItems: (api.already_in_library_items ?? []).map(transformSyncChangeItem),
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
  perPage: number = 20,
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

  const response = await api.get<JobListApiResponse>('/jobs', { params });

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
 * Get lightweight status for a job type: the active job (if any) and the most
 * recent terminal job. Used for continuous polling + completion detection.
 */
export async function getJobTypeStatus(jobType: JobType): Promise<JobTypeStatus> {
  const response = await api.get<JobTypeStatusApiResponse>(`/jobs/status/${jobType}`);
  return {
    isActive: response.is_active,
    activeJobId: response.active_job_id,
    lastCompletedJobId: response.last_completed_job_id,
    lastCompletedAt: response.last_completed_at,
  };
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
  pageSize: number = 50,
): Promise<JobItemListResponse> {
  const params: Record<string, string | number> = { page, per_page: pageSize };
  if (status) params.status = status;

  const response = await api.get<JobItemListApiResponse>(`/jobs/${jobId}/items`, { params });

  return {
    items: response.items.map(transformJobItem),
    total: response.total,
    page: response.page,
    pageSize: response.per_page,
    pages: response.total_pages,
  };
}

/**
 * Get total count of items needing review across all jobs.
 * Used for nav badge display.
 */
export async function getPendingReviewCount(): Promise<PendingReviewCountResponse> {
  const response = await api.get<PendingReviewCountApiResponse>('/jobs/pending-review-count');
  return {
    pendingReviewCount: response.pending_review_count,
    countsBySource: response.counts_by_source,
  };
}

/**
 * Retry all failed items in a job.
 */
export async function retryFailedItems(jobId: string): Promise<RetryFailedResponse> {
  const response = await api.post<RetryFailedApiResponse>(`/jobs/${jobId}/retry-failed`);
  return {
    success: response.success,
    message: response.message,
    retriedCount: response.retried_count,
  };
}

/**
 * Retry a single failed job item.
 */
export async function retryJobItem(itemId: string): Promise<JobItemDetail> {
  const response = await api.post<JobItemDetailApiResponse>(`/job-items/${itemId}/retry`);
  return transformJobItemDetail(response);
}

/**
 * Get recent completed jobs for a specific source with item details.
 */
export async function getRecentJobs(
  source: string,
  limit: number = 5,
): Promise<RecentJobsResponse> {
  const response = await api.get<RecentJobsApiResponse>(`/jobs/recent/${source}`, {
    params: { limit },
  });
  return {
    jobs: response.jobs.map(transformRecentJob),
  };
}
