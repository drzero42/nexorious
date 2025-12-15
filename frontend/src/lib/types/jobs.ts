/**
 * TypeScript types for Jobs, Review, and Sync APIs.
 *
 * These types mirror the backend Pydantic schemas.
 */

// ============================================================================
// Job Types
// ============================================================================

export enum JobType {
  SYNC = 'sync',
  IMPORT = 'import',
  EXPORT = 'export'
}

export enum JobSource {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog',
  DARKADIA = 'darkadia',
  NEXORIOUS = 'nexorious',
  SYSTEM = 'system'
}

export enum JobStatus {
  PENDING = 'pending',
  PROCESSING = 'processing',
  AWAITING_REVIEW = 'awaiting_review',
  READY = 'ready',
  FINALIZING = 'finalizing',
  COMPLETED = 'completed',
  FAILED = 'failed',
  CANCELLED = 'cancelled'
}

export enum JobPriority {
  HIGH = 'high',
  LOW = 'low'
}

export interface Job {
  id: string;
  user_id: string;
  job_type: JobType;
  source: JobSource;
  status: JobStatus;
  priority: JobPriority;
  progress_current: number;
  progress_total: number;
  progress_percent: number;
  result_summary: Record<string, unknown>;
  error_message: string | null;
  file_path: string | null;
  taskiq_task_id: string | null;
  created_at: string;
  started_at: string | null;
  completed_at: string | null;
  is_terminal: boolean;
  duration_seconds: number | null;
  review_item_count: number | null;
  pending_review_count: number | null;
}

export interface JobListResponse {
  jobs: Job[];
  total: number;
  page: number;
  per_page: number;
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
  deleted_job_id: string;
}

export interface JobConfirmResponse {
  success: boolean;
  message: string;
  job: Job | null;
  games_added: number;
  games_skipped: number;
  games_removed: number;
}

export interface JobFilters {
  job_type?: JobType;
  source?: JobSource;
  status?: JobStatus;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
}

// ============================================================================
// Review Types
// ============================================================================

export enum ReviewItemStatus {
  PENDING = 'pending',
  MATCHED = 'matched',
  SKIPPED = 'skipped',
  REMOVAL = 'removal'
}

export interface IGDBCandidate {
  igdb_id: number;
  name: string;
  first_release_date: number | null;
  cover_url: string | null;
  summary: string | null;
  platforms: string[] | null;
  similarity_score: number | null;
}

export interface ReviewItem {
  id: string;
  job_id: string;
  user_id: string;
  status: ReviewItemStatus;
  source_title: string;
  source_metadata: Record<string, unknown>;
  igdb_candidates: IGDBCandidate[] | Record<string, unknown>[];
  resolved_igdb_id: number | null;
  created_at: string;
  resolved_at: string | null;
  job_type: string | null;
  job_source: string | null;
}

export interface ReviewItemDetail extends Omit<ReviewItem, 'igdb_candidates'> {
  igdb_candidates: IGDBCandidate[];
}

export interface ReviewListResponse {
  items: ReviewItem[];
  total: number;
  page: number;
  per_page: number;
  pages: number;
}

export interface ReviewSummary {
  total_pending: number;
  total_matched: number;
  total_skipped: number;
  total_removal: number;
  jobs_with_pending: number;
}

export interface MatchRequest {
  igdb_id: number;
}

export interface MatchResponse {
  success: boolean;
  message: string;
  item: ReviewItem | null;
}

export enum ReviewSource {
  IMPORT = 'import',
  SYNC = 'sync'
}

export interface ReviewFilters {
  status?: ReviewItemStatus;
  job_id?: string;
  source?: ReviewSource;
}

/**
 * Pending review counts grouped by job type (import vs sync).
 * Used by navigation badges to show how many items need review.
 */
export interface ReviewCountsByType {
  import_pending: number;
  sync_pending: number;
}

// ============================================================================
// Sync Types
// ============================================================================

export enum SyncFrequency {
  MANUAL = 'manual',
  HOURLY = 'hourly',
  DAILY = 'daily',
  WEEKLY = 'weekly'
}

export enum SyncPlatform {
  STEAM = 'steam',
  EPIC = 'epic',
  GOG = 'gog'
}

export interface SyncConfig {
  id: string;
  user_id: string;
  platform: string;
  frequency: SyncFrequency;
  auto_add: boolean;
  enabled: boolean;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SyncConfigListResponse {
  configs: SyncConfig[];
  total: number;
}

export interface SyncConfigUpdateRequest {
  frequency?: SyncFrequency;
  auto_add?: boolean;
  enabled?: boolean;
}

export interface ManualSyncTriggerResponse {
  message: string;
  job_id: string;
  platform: string;
  status: string;
}

export interface SyncStatusResponse {
  platform: string;
  is_syncing: boolean;
  last_synced_at: string | null;
  active_job_id: string | null;
}

// ============================================================================
// Helper functions
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
    [JobType.EXPORT]: 'Export'
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
    [JobSource.SYSTEM]: 'System'
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
    [JobStatus.CANCELLED]: 'Cancelled'
  };
  return labels[status] || status;
}

/**
 * Get a CSS class for a job status badge.
 */
export function getJobStatusColor(status: JobStatus): string {
  const colors: Record<JobStatus, string> = {
    [JobStatus.PENDING]: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300',
    [JobStatus.PROCESSING]: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300',
    [JobStatus.AWAITING_REVIEW]: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-300',
    [JobStatus.READY]: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300',
    [JobStatus.FINALIZING]: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300',
    [JobStatus.COMPLETED]: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-300',
    [JobStatus.FAILED]: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300',
    [JobStatus.CANCELLED]: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300'
  };
  return colors[status] || 'bg-gray-100 text-gray-800';
}

/**
 * Get a human-readable label for a sync frequency.
 */
export function getSyncFrequencyLabel(frequency: SyncFrequency): string {
  const labels: Record<SyncFrequency, string> = {
    [SyncFrequency.MANUAL]: 'Manual',
    [SyncFrequency.HOURLY]: 'Hourly',
    [SyncFrequency.DAILY]: 'Daily',
    [SyncFrequency.WEEKLY]: 'Weekly'
  };
  return labels[frequency] || frequency;
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
