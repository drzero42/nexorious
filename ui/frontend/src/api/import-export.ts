import { api, apiUploadFile, apiDownloadFile } from './client';
import type {
  ImportJobCreatedResponse,
  ExportJobCreatedResponse,
  ImportSourceInfo,
  CsvInspectResponse,
  CsvMapping,
} from '@/types';

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

/** List the registered mapper-based import sources (drives the picker). */
export async function fetchImportSources(): Promise<ImportSourceInfo[]> {
  return api.get<ImportSourceInfo[]>('/import/sources');
}

/** Upload a file to a registered import source by slug. */
export async function importFromSource(
  slug: string,
  file: File,
): Promise<ImportJobCreatedResponse> {
  return apiUploadFile<ImportJobCreatedResponse>(`/import/${slug}`, file);
}

/** Inspect a CSV file: headers, row count, and per-column distinct values. */
export async function inspectCsv(file: File): Promise<CsvInspectResponse> {
  return apiUploadFile<CsvInspectResponse>('/import/csv/inspect', file);
}

/** Import a CSV with a user-built column/status mapping. */
export async function importCsv(
  file: File,
  mapping: CsvMapping,
): Promise<ImportJobCreatedResponse> {
  return apiUploadFile<ImportJobCreatedResponse>('/import/csv', file, 'file', {
    mapping: JSON.stringify(mapping),
  });
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
