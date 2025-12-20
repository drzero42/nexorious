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
  AWAITING_REVIEW = 'awaiting_review',
  READY = 'ready',
  FINALIZING = 'finalizing',
  COMPLETED = 'completed',
  FAILED = 'failed',
  CANCELLED = 'cancelled',
}

export enum JobPriority {
  HIGH = 'high',
  LOW = 'low',
}

// ============================================================================
// Interfaces
// ============================================================================

export interface Job {
  id: string;
  userId: string;
  jobType: JobType;
  source: JobSource;
  status: JobStatus;
  priority: JobPriority;
  progressCurrent: number;
  progressTotal: number;
  progressPercent: number;
  resultSummary: Record<string, unknown>;
  errorMessage: string | null;
  filePath: string | null;
  taskiqTaskId: string | null;
  createdAt: string;
  startedAt: string | null;
  completedAt: string | null;
  isTerminal: boolean;
  durationSeconds: number | null;
  reviewItemCount: number | null;
  pendingReviewCount: number | null;
}

export interface JobFilters {
  jobType?: JobType;
  source?: JobSource;
  status?: JobStatus;
  sortBy?: string;
  sortOrder?: 'asc' | 'desc';
}

export interface JobListResponse {
  jobs: Job[];
  total: number;
  page: number;
  perPage: number;
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

export interface JobConfirmResponse {
  success: boolean;
  message: string;
  job: Job | null;
  gamesAdded: number;
  gamesSkipped: number;
  gamesRemoved: number;
}

export interface JobsSummary {
  runningCount: number;
  failedCount: number;
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
    [JobStatus.AWAITING_REVIEW]: 'Awaiting Review',
    [JobStatus.READY]: 'Ready',
    [JobStatus.FINALIZING]: 'Finalizing',
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
    case JobStatus.READY:
      return 'default';
    case JobStatus.PROCESSING:
    case JobStatus.FINALIZING:
      return 'secondary';
    case JobStatus.FAILED:
      return 'destructive';
    case JobStatus.PENDING:
    case JobStatus.AWAITING_REVIEW:
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
 * Check if an import job can be confirmed.
 */
export function canConfirmJob(job: Job): boolean {
  return (
    job.jobType === JobType.IMPORT &&
    (job.status === JobStatus.READY || job.status === JobStatus.AWAITING_REVIEW) &&
    job.pendingReviewCount === 0
  );
}
