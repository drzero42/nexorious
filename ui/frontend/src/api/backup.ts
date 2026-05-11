/**
 * Backup/Restore API client functions.
 */

import { api, apiDownloadFile, apiUploadFile } from './client';
import type {
  BackupConfig,
  BackupConfigBackend,
  BackupConfigUpdateRequest,
  BackupInfo,
  BackupInfoBackend,
  BackupListResponse,
  BackupCreateResponse,
  BackupDeleteResponse,
  RestoreResponse,
} from '@/types';

function mapBackupConfigToFrontend(backend: BackupConfigBackend): BackupConfig {
  return {
    schedule: backend.schedule,
    scheduleTime: backend.schedule_time,
    scheduleDay: backend.schedule_day,
    retentionMode: backend.retention_mode,
    retentionValue: backend.retention_value,
    updatedAt: backend.updated_at,
  };
}

function mapBackupInfoToFrontend(backend: BackupInfoBackend): BackupInfo {
  return {
    id: backend.id,
    createdAt: backend.created_at,
    backupType: backend.backup_type,
    sizeBytes: backend.size_bytes,
    stats: backend.stats,
  };
}

/**
 * Get backup configuration
 */
export async function getBackupConfig(): Promise<BackupConfig> {
  const response = await api.get<BackupConfigBackend>('/admin/backups/config');
  return mapBackupConfigToFrontend(response);
}

/**
 * Update backup configuration
 */
export async function updateBackupConfig(
  config: BackupConfigUpdateRequest
): Promise<BackupConfig> {
  const response = await api.put<BackupConfigBackend>('/admin/backups/config', config);
  return mapBackupConfigToFrontend(response);
}

/**
 * List all backups
 */
export async function listBackups(): Promise<BackupInfo[]> {
  const response = await api.get<BackupListResponse>('/admin/backups');
  return response.backups.map(mapBackupInfoToFrontend);
}

/**
 * Create a new backup
 */
export async function createBackup(): Promise<BackupCreateResponse> {
  return api.post<BackupCreateResponse>('/admin/backups', {});
}

/**
 * Delete a backup
 */
export async function deleteBackup(backupId: string): Promise<BackupDeleteResponse> {
  return api.delete<BackupDeleteResponse>(`/admin/backups/${backupId}`);
}

/**
 * Download a backup file
 */
export async function downloadBackup(backupId: string): Promise<{ blob: Blob; filename: string }> {
  return apiDownloadFile(`/admin/backups/${backupId}/download`);
}

/**
 * Restore from a server backup
 */
export async function restoreBackup(backupId: string): Promise<RestoreResponse> {
  return api.post<RestoreResponse>(`/admin/backups/${backupId}/restore`, { confirm: true });
}

/**
 * Upload and restore from a backup file
 */
export async function uploadAndRestoreBackup(file: File): Promise<RestoreResponse> {
  return apiUploadFile<RestoreResponse>('/admin/backups/restore/upload', file, 'file');
}
