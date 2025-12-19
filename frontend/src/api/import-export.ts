import { api, apiUploadFile, apiDownloadFile } from './client';
import type {
  ImportJobCreatedResponse,
  ExportJobCreatedResponse,
} from '@/types';

// ============================================================================
// API Response Types (snake_case from backend)
// ============================================================================

interface ImportJobApiResponse {
  job_id: string;
  source: string;
  status: string;
  message: string;
  total_items: number | null;
}

interface ExportJobApiResponse {
  job_id: string;
  status: string;
  message: string;
  estimated_items: number;
}

// ============================================================================
// Transformation Functions
// ============================================================================

function transformImportJobResponse(apiResponse: ImportJobApiResponse): ImportJobCreatedResponse {
  return {
    job_id: apiResponse.job_id,
    source: apiResponse.source,
    status: apiResponse.status,
    message: apiResponse.message,
    total_items: apiResponse.total_items,
  };
}

function transformExportJobResponse(apiResponse: ExportJobApiResponse): ExportJobCreatedResponse {
  return {
    job_id: apiResponse.job_id,
    status: apiResponse.status,
    message: apiResponse.message,
    estimated_items: apiResponse.estimated_items,
  };
}

// ============================================================================
// Import API Functions
// ============================================================================

/**
 * Import games from a Nexorious JSON export file.
 * This is a non-interactive import that trusts the IGDB IDs in the export.
 */
export async function importNexoriousJson(file: File): Promise<ImportJobCreatedResponse> {
  const response = await apiUploadFile<ImportJobApiResponse>('/import/nexorious', file);
  return transformImportJobResponse(response);
}

/**
 * Import games from a Darkadia CSV export file.
 * This is an interactive import that requires title-based matching and review.
 */
export async function importDarkadiaCsv(file: File): Promise<ImportJobCreatedResponse> {
  const response = await apiUploadFile<ImportJobApiResponse>('/import/darkadia', file);
  return transformImportJobResponse(response);
}

// ============================================================================
// Export API Functions
// ============================================================================

/**
 * Start a JSON export of the user's game collection.
 */
export async function exportCollectionJson(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobApiResponse>('/export/collection/json');
  return transformExportJobResponse(response);
}

/**
 * Start a CSV export of the user's game collection.
 */
export async function exportCollectionCsv(): Promise<ExportJobCreatedResponse> {
  const response = await api.post<ExportJobApiResponse>('/export/collection/csv');
  return transformExportJobResponse(response);
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
