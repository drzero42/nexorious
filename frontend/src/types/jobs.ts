/**
 * Types for background job management.
 * Jobs include sync, import, and export operations.
 */

// ============================================================================
// Enums
// ============================================================================

export enum JobType {
  SYNC = 'sync',
  IMPORT = 'import',
  EXPORT = 'export',
}

export enum JobSource {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
  DARKADIA = 'darkadia',
  NEXORIOUS = 'nexorious',
  SYSTEM = 'system',
}

export enum JobStatus {
  PENDING = 'pending',
  PROCESSING = 'processing',
  COMPLETED = 'completed',
  FAILED = 'failed',
  CANCELLED = 'cancelled',
}

export enum JobItemStatus {
  PENDING = 'pending',
  PROCESSING = 'processing',
  COMPLETED = 'completed',
  PENDING_REVIEW = 'pending_review',
  SKIPPED = 'skipped',
  FAILED = 'failed',
}

export enum JobPriority {
  HIGH = 'high',
  LOW = 'low',
}

// ============================================================================
// Interfaces
// ============================================================================

/**
 * Progress counts by JobItem status.
 */
export interface JobProgress {
  pending: number;
  processing: number;
  completed: number;
  pendingReview: number;
  skipped: number;
  failed: number;
  total: number;
  percent: number;
}

export interface Job {
  id: string;
  userId: string;
  jobType: JobType;
  source: JobSource;
  status: JobStatus;
  priority: JobPriority;
  progress: JobProgress;
  totalItems: number;
  errorMessage: string | null;
  filePath: string | null;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
  isTerminal: boolean;
  durationSeconds: number | null;
}

export interface JobFilters {
  jobType?: JobType;
  source?: JobSource;
  status?: JobStatus;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
}

// Note: JobChildrenFilters removed - child jobs no longer supported

export interface JobListResponse {
  jobs: Job[];
  total: number;
  page: number;
  perPage: number;
  pages: number;
}

export interface JobItem {
  id: string;
  jobId: string;
  itemKey: string;
  sourceTitle: string;
  status: JobItemStatus;
  errorMessage: string | null;
  resultGameTitle: string | null;
  resultIgdbId: number | null;
  createdAt: string;
  processedAt: string | null;
}

export interface JobItemListResponse {
  items: JobItem[];
  total: number;
  page: number;
  pageSize: number;
  pages: number;
}

export interface JobCancelResponse {
  success: boolean;
  message: string;
  job: Job | null;
}

export interface JobDeleteResponse {
  success: boolean;
  message: string;
  deletedJobId: string;
}

export interface JobsSummary {
  runningCount: number;
  failedCount: number;
}

export interface PendingReviewCountResponse {
  pendingReviewCount: number;
}

export interface JobItemDetail extends JobItem {
  sourceMetadataJson: string;
  resultJson: string;
  igdbCandidatesJson: string;
  resolvedIgdbId: number | null;
  resolvedAt: string | null;
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Check if a job status is terminal (completed, failed, or cancelled).
 */
export function isTerminalStatus(status: JobStatus): boolean {
  return [JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED].includes(status);
}

/**
 * Get a human-readable label for a job type.
 */
export function getJobTypeLabel(type: JobType): string {
  const labels: Record<JobType, string> = {
    [JobType.SYNC]: 'Sync',
    [JobType.IMPORT]: 'Import',
    [JobType.EXPORT]: 'Export',
  };
  return labels[type] || type;
}

/**
 * Get a human-readable label for a job source.
 */
export function getJobSourceLabel(source: JobSource): string {
  const labels: Record<JobSource, string> = {
    [JobSource.STEAM]: 'Steam',
    [JobSource.EPIC]: 'Epic Games',
    [JobSource.GOG]: 'GOG',
    [JobSource.DARKADIA]: 'Darkadia',
    [JobSource.NEXORIOUS]: 'Nexorious',
    [JobSource.SYSTEM]: 'System',
  };
  return labels[source] || source;
}

/**
 * Get a human-readable label for a job status.
 */
export function getJobStatusLabel(status: JobStatus): string {
  const labels: Record<JobStatus, string> = {
    [JobStatus.PENDING]: 'Pending',
    [JobStatus.PROCESSING]: 'Processing',
    [JobStatus.COMPLETED]: 'Completed',
    [JobStatus.FAILED]: 'Failed',
    [JobStatus.CANCELLED]: 'Cancelled',
  };
  return labels[status] || status;
}

/**
 * Get CSS classes for a job status badge.
 */
export function getJobStatusVariant(
  status: JobStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case JobStatus.COMPLETED:
      return 'default';
    case JobStatus.PROCESSING:
      return 'secondary';
    case JobStatus.FAILED:
      return 'destructive';
    case JobStatus.PENDING:
    case JobStatus.CANCELLED:
    default:
      return 'outline';
  }
}

/**
 * Format duration in seconds to human-readable string.
 */
export function formatDuration(seconds: number | null): string {
  if (seconds === null || seconds === undefined) return '-';

  if (seconds < 60) {
    return `${Math.round(seconds)}s`;
  } else if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    const secs = Math.round(seconds % 60);
    return secs > 0 ? `${minutes}m ${secs}s` : `${minutes}m`;
  } else {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.round((seconds % 3600) / 60);
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }
}

/**
 * Format a date string to a relative time (e.g., "5m ago").
 */
export function formatRelativeTime(dateStr: string | null): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffMins < 1) return 'Just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

/**
 * Check if a job is currently in progress (not terminal).
 */
export function isJobInProgress(job: Job): boolean {
  return !job.isTerminal && job.status !== JobStatus.PENDING;
}

/**
 * Check if a job can be cancelled.
 * Jobs can be cancelled at any point before they reach a terminal state.
 */
export function canCancelJob(job: Job): boolean {
  return !job.isTerminal;
}

/**
 * Check if a job can be deleted.
 * Jobs can be deleted at any point (terminal or not).
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function canDeleteJob(_job: Job): boolean {
  return true;
}

/**
 * Check if an import job has items pending review.
 */
export function hasPendingReview(job: Job): boolean {
  return job.jobType === JobType.IMPORT && job.progress.pendingReview > 0;
}

/**
 * Get a human-readable label for a job item status.
 */
export function getJobItemStatusLabel(status: JobItemStatus): string {
  const labels: Record<JobItemStatus, string> = {
    [JobItemStatus.PENDING]: 'Pending',
    [JobItemStatus.PROCESSING]: 'Processing',
    [JobItemStatus.COMPLETED]: 'Completed',
    [JobItemStatus.PENDING_REVIEW]: 'Needs Review',
    [JobItemStatus.SKIPPED]: 'Skipped',
    [JobItemStatus.FAILED]: 'Failed',
  };
  return labels[status];
}

/**
 * Get CSS classes for a job item status badge.
 */
export function getJobItemStatusVariant(
  status: JobItemStatus
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case JobItemStatus.COMPLETED:
      return 'default';
    case JobItemStatus.FAILED:
      return 'destructive';
    case JobItemStatus.PENDING_REVIEW:
      return 'secondary';
    default:
      return 'outline';
  }
}
