import { useMutation, useQueryClient } from '@tanstack/react-query';
import * as importExportApi from '@/api/import-export';
import { JobType } from '@/types';
import type {
  ImportJobCreatedResponse,
  ExportJobCreatedResponse,
  ExportFormat,
  JobTypeStatus,
} from '@/types';
import { jobsKeys } from './use-jobs';

// Query Keys

export const importExportKeys = {
  all: ['import-export'] as const,
  jobs: () => [...importExportKeys.all, 'jobs'] as const,
};

// Optimistically mark a job type active so the progress card appears
// immediately, without waiting for the next status poll. Mirrors useTriggerSync.
function markJobTypeActive(
  queryClient: ReturnType<typeof useQueryClient>,
  jobType: JobType,
  jobId: string,
) {
  queryClient.setQueryData<JobTypeStatus>(jobsKeys.typeStatus(jobType), (old) => ({
    isActive: true,
    activeJobId: jobId,
    lastCompletedJobId: old?.lastCompletedJobId ?? null,
    lastCompletedAt: old?.lastCompletedAt ?? null,
  }));
  queryClient.invalidateQueries({ queryKey: jobsKeys.typeStatus(jobType) });
}

// Import Mutation Hooks

/**
 * Non-interactive import that trusts IGDB IDs.
 */
export function useImportNexorious() {
  const queryClient = useQueryClient();
  return useMutation<ImportJobCreatedResponse, Error, File>({
    mutationFn: (file) => importExportApi.importNexoriousJson(file),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}

/**
 * One-off Darkadia CSV migration. Matches each game to IGDB; ambiguous matches
 * land in the pending-review flow.
 */
export function useImportDarkadia() {
  const queryClient = useQueryClient();
  return useMutation<ImportJobCreatedResponse, Error, File>({
    mutationFn: (file) => importExportApi.importDarkadiaCsv(file),
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.IMPORT, result.job_id);
    },
  });
}

// Export Mutation Hooks

/**
 * Returns the job ID for tracking progress.
 */
export function useExportCollection() {
  const queryClient = useQueryClient();
  return useMutation<ExportJobCreatedResponse, Error, ExportFormat>({
    mutationFn: (format) => {
      if (format === 'json') {
        return importExportApi.exportCollectionJson();
      }
      return importExportApi.exportCollectionCsv();
    },
    onSuccess: (result) => {
      markJobTypeActive(queryClient, JobType.EXPORT, result.job_id);
    },
  });
}

export function useDownloadExport() {
  return useMutation<{ blob: Blob; filename: string }, Error, string>({
    mutationFn: (jobId) => importExportApi.downloadExport(jobId),
    onSuccess: ({ blob, filename }) => {
      importExportApi.triggerBlobDownload(blob, filename);
    },
  });
}
