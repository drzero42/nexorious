import { useMutation } from '@tanstack/react-query';
import * as importExportApi from '@/api/import-export';
import type {
  ImportJobCreatedResponse,
  ExportJobCreatedResponse,
  ExportFormat,
} from '@/types';

// ============================================================================
// Query Keys
// ============================================================================

export const importExportKeys = {
  all: ['import-export'] as const,
  jobs: () => [...importExportKeys.all, 'jobs'] as const,
};

// ============================================================================
// Import Mutation Hooks
// ============================================================================

/**
 * Hook to import games from a Nexorious JSON export file.
 * Non-interactive import that trusts IGDB IDs.
 */
export function useImportNexorious() {
  return useMutation<ImportJobCreatedResponse, Error, File>({
    mutationFn: (file) => importExportApi.importNexoriousJson(file),
  });
}

/**
 * Hook to import games from a Darkadia CSV export file.
 * Interactive import that requires review for unmatched titles.
 */
export function useImportDarkadia() {
  return useMutation<ImportJobCreatedResponse, Error, File>({
    mutationFn: (file) => importExportApi.importDarkadiaCsv(file),
  });
}

// ============================================================================
// Export Mutation Hooks
// ============================================================================

/**
 * Hook to start an export of the user's game collection.
 * Returns the job ID for tracking progress.
 */
export function useExportCollection() {
  return useMutation<ExportJobCreatedResponse, Error, ExportFormat>({
    mutationFn: (format) => {
      if (format === 'json') {
        return importExportApi.exportCollectionJson();
      }
      return importExportApi.exportCollectionCsv();
    },
  });
}

/**
 * Hook to start an export of the user's wishlist.
 * Returns the job ID for tracking progress.
 */
export function useExportWishlist() {
  return useMutation<ExportJobCreatedResponse, Error, ExportFormat>({
    mutationFn: (format) => {
      if (format === 'json') {
        return importExportApi.exportWishlistJson();
      }
      return importExportApi.exportWishlistCsv();
    },
  });
}

/**
 * Hook to download a completed export file.
 */
export function useDownloadExport() {
  return useMutation<{ blob: Blob; filename: string }, Error, string>({
    mutationFn: (jobId) => importExportApi.downloadExport(jobId),
    onSuccess: ({ blob, filename }) => {
      importExportApi.triggerBlobDownload(blob, filename);
    },
  });
}
