/**
 * Backup/Restore types for admin backup functionality.
 */

// Enums matching backend
export type BackupSchedule = 'manual' | 'daily' | 'weekly';
export type RetentionMode = 'days' | 'count';
export type BackupType = 'scheduled' | 'manual' | 'pre_restore';

// Configuration
export interface BackupConfig {
  schedule: BackupSchedule;
  scheduleTime: string;
  scheduleDay: number | null;
  retentionMode: RetentionMode;
  retentionValue: number;
  updatedAt: string;
}

export interface BackupConfigBackend {
  schedule: BackupSchedule;
  schedule_time: string;
  schedule_day: number | null;
  retention_mode: RetentionMode;
  retention_value: number;
  updated_at: string;
}

export interface BackupConfigUpdateRequest {
  schedule?: BackupSchedule;
  schedule_time?: string;
  schedule_day?: number | null;
  retention_mode?: RetentionMode;
  retention_value?: number;
}

// Backup info
export interface BackupStats {
  users: number;
  games: number;
  tags: number;
}

export interface BackupInfo {
  id: string;
  createdAt: string;
  backupType: BackupType;
  sizeBytes: number;
  stats: BackupStats;
}

export interface BackupInfoBackend {
  id: string;
  created_at: string;
  backup_type: BackupType;
  size_bytes: number;
  stats: BackupStats;
}

export interface BackupListResponse {
  backups: BackupInfoBackend[];
  total: number;
}

// Operations
export interface BackupCreateResponse {
  job_id: string;
  message: string;
}

export interface BackupDeleteResponse {
  success: boolean;
  message: string;
}

export interface RestoreResponse {
  success: boolean;
  message: string;
  session_invalidated: boolean;
}
