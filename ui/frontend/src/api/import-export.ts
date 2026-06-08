import { api, apiUploadFile, apiDownloadFile } from './client';
import type { ImportJobCreatedResponse, ExportJobCreatedResponse } from '@/types';

// ============================================================================
// Import API Functions
// ============================================================================

/**
 * Import games from a Nexorious JSON export file.
 * This is a non-interactive import that trusts the IGDB IDs in the export.
 */
export async function importNexoriousJson(file: File): Promise<ImportJobCreatedResponse> {
  const response = await apiUploadFile<ImportJobCreatedResponse>('/import/nexorious', file);
  return response;
}

/**
 * Import a Darkadia CSV export. Parsing/consolidation happens server-side; the
 * returned job drives IGDB matching and the interactive pending-review flow.
 */
export async function importDarkadiaCsv(file: File): Promise<ImportJobCreatedResponse> {
  const response = await apiUploadFile<ImportJobCreatedResponse>('/import/darkadia', file);
  return response;
}

// ============================================================================
// Export API Functions
// ============================================================================

/**
 * Start a JSON export of all user games.
 */
export async function exportCollectionJson(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobCreatedResponse>('/export/json');
  return response;
}

/**
 * Start a CSV export of all user games.
 */
export async function exportCollectionCsv(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobCreatedResponse>('/export/csv');
  return response;
}

/**
 * Download a completed export file.
 * Returns the file blob for client-side download.
 */
export async function downloadExport(jobId: string): Promise<{ blob: Blob; filename: string }> {
  return apiDownloadFile(`/export/${jobId}/download`);
}

/**
 * Helper function to trigger browser download of a blob.
 */
export function triggerBlobDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}
