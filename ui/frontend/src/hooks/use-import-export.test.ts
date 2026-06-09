import { createElement, type ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { useImportNexorious, useExportCollection, useDownloadExport } from './use-import-export';
import { jobsKeys } from './use-jobs';
import { ExportFormat, JobType } from '@/types';
import type { JobTypeStatus } from '@/types';

const API_URL = '/api';

// Build a QueryWrapper bound to a caller-owned QueryClient so the test can read
// the cache the hook writes to. The shared QueryWrapper creates its own internal
// client, which is invisible from the test. This file is .ts (no JSX), so the
// provider element is created via createElement.
function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

// A QueryClient that retains cache entries without an active observer. The hook
// only mounts a mutation observer, so the optimistic typeStatus write (followed
// by an invalidateQueries call) would be garbage-collected under gcTime: 0
// before the test can read it. In production useJobTypeStatus keeps the entry
// alive; here a non-zero gcTime stands in for that observer.
function createCacheReadableQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

describe('use-import-export hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('useImportNexorious', () => {
    it('uploads file and returns job info', async () => {
      server.use(
        http.post(`${API_URL}/import/nexorious`, () => {
          return HttpResponse.json({
            job_id: 'job-123',
            source: 'nexorious',
            status: 'pending',
            message: 'Import job created. Processing 5 games.',
            total_items: 5,
          });
        }),
      );

      const { result } = renderHook(() => useImportNexorious(), {
        wrapper: QueryWrapper,
      });

      const mockFile = new File(['{"games": []}'], 'backup.json', { type: 'application/json' });

      await act(async () => {
        await result.current.mutateAsync(mockFile);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.job_id).toBe('job-123');
      expect(result.current.data?.source).toBe('nexorious');
      expect(result.current.data?.total_items).toBe(5);
    });

    it('handles import error', async () => {
      server.use(
        http.post(`${API_URL}/import/nexorious`, () => {
          return HttpResponse.json({ detail: 'Invalid JSON file' }, { status: 400 });
        }),
      );

      const { result } = renderHook(() => useImportNexorious(), {
        wrapper: QueryWrapper,
      });

      const mockFile = new File(['invalid'], 'bad.json', { type: 'application/json' });

      await act(async () => {
        try {
          await result.current.mutateAsync(mockFile);
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Invalid JSON file');
    });

    it('handles conflict when import already in progress', async () => {
      server.use(
        http.post(`${API_URL}/import/nexorious`, () => {
          return HttpResponse.json(
            { detail: 'Import already in progress. Job ID: job-existing' },
            { status: 409 },
          );
        }),
      );

      const { result } = renderHook(() => useImportNexorious(), {
        wrapper: QueryWrapper,
      });

      const mockFile = new File(['{"games": []}'], 'backup.json', { type: 'application/json' });

      await act(async () => {
        try {
          await result.current.mutateAsync(mockFile);
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toContain('Import already in progress');
    });

    it('optimistically marks the import job type active on success, preserving prior last-completed info', async () => {
      server.use(
        http.post(`${API_URL}/import/nexorious`, () => {
          return HttpResponse.json({
            job_id: 'job-new',
            source: 'nexorious',
            status: 'pending',
            message: 'Import job created.',
            total_items: 3,
          });
        }),
      );

      const queryClient = createCacheReadableQueryClient();

      // Seed a prior status with non-null last-completed info.
      const prior: JobTypeStatus = {
        isActive: false,
        activeJobId: null,
        lastCompletedJobId: 'job-prev',
        lastCompletedAt: '2026-01-01T00:00:00Z',
      };
      queryClient.setQueryData(jobsKeys.typeStatus(JobType.IMPORT), prior);

      const { result } = renderHook(() => useImportNexorious(), {
        wrapper: wrapperFor(queryClient),
      });

      const mockFile = new File(['{"games": []}'], 'backup.json', { type: 'application/json' });

      await act(async () => {
        await result.current.mutateAsync(mockFile);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(queryClient.getQueryData(jobsKeys.typeStatus(JobType.IMPORT))).toEqual({
        isActive: true,
        activeJobId: 'job-new',
        lastCompletedJobId: 'job-prev',
        lastCompletedAt: '2026-01-01T00:00:00Z',
      });
    });
  });

  describe('useExportCollection', () => {
    it.each([
      [ExportFormat.JSON, 'json', 'export-123', 50],
      [ExportFormat.CSV, 'csv', 'export-456', 100],
    ])('starts %s export and returns job info', async (format, path, jobId, estimatedItems) => {
      server.use(
        http.post(`${API_URL}/export/${path}`, () => {
          return HttpResponse.json({
            job_id: jobId,
            status: 'pending',
            message: 'Export job created.',
            estimated_items: estimatedItems,
          });
        }),
      );

      const { result } = renderHook(() => useExportCollection(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(format);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.job_id).toBe(jobId);
      expect(result.current.data?.estimated_items).toBe(estimatedItems);
    });

    it('handles empty collection error', async () => {
      server.use(
        http.post(`${API_URL}/export/json`, () => {
          return HttpResponse.json(
            { detail: 'No games in collection to export.' },
            { status: 400 },
          );
        }),
      );

      const { result } = renderHook(() => useExportCollection(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync(ExportFormat.JSON);
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('No games in collection to export.');
    });

    it('optimistically marks the export job type active on success, preserving prior last-completed info', async () => {
      server.use(
        http.post(`${API_URL}/export/json`, () => {
          return HttpResponse.json({
            job_id: 'export-new',
            status: 'pending',
            message: 'Export job created.',
            estimated_items: 42,
          });
        }),
      );

      const queryClient = createCacheReadableQueryClient();

      // Seed a prior status with non-null last-completed info.
      const prior: JobTypeStatus = {
        isActive: false,
        activeJobId: null,
        lastCompletedJobId: 'export-prev',
        lastCompletedAt: '2026-02-02T00:00:00Z',
      };
      queryClient.setQueryData(jobsKeys.typeStatus(JobType.EXPORT), prior);

      const { result } = renderHook(() => useExportCollection(), {
        wrapper: wrapperFor(queryClient),
      });

      await act(async () => {
        await result.current.mutateAsync(ExportFormat.JSON);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(queryClient.getQueryData(jobsKeys.typeStatus(JobType.EXPORT))).toEqual({
        isActive: true,
        activeJobId: 'export-new',
        lastCompletedJobId: 'export-prev',
        lastCompletedAt: '2026-02-02T00:00:00Z',
      });
    });
  });

  describe('useDownloadExport', () => {
    it('downloads export file successfully', async () => {
      const fileContent = JSON.stringify({ games: [] });

      server.use(
        http.get(`${API_URL}/export/export-123/download`, () => {
          return new HttpResponse(fileContent, {
            headers: {
              'Content-Type': 'application/json',
              'Content-Disposition': 'attachment; filename="nexorious_collection_20250101.json"',
            },
          });
        }),
      );

      const { result } = renderHook(() => useDownloadExport(), {
        wrapper: QueryWrapper,
      });

      // Mock URL.createObjectURL/revokeObjectURL and document methods for download trigger
      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/mock-url');
      const mockRevokeObjectURL = vi.fn();

      global.URL.createObjectURL = mockCreateObjectURL;
      global.URL.revokeObjectURL = mockRevokeObjectURL;

      // Create a real anchor element to avoid Node type issues
      const realAnchor = document.createElement('a');
      const mockClick = vi.fn();
      realAnchor.click = mockClick;

      const originalCreateElement = document.createElement.bind(document);
      vi.spyOn(document, 'createElement').mockImplementation((tagName: string) => {
        if (tagName === 'a') {
          return realAnchor;
        }
        return originalCreateElement(tagName);
      });

      await act(async () => {
        await result.current.mutateAsync('export-123');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      // The onSuccess callback should have triggered the download
      expect(mockClick).toHaveBeenCalled();
      expect(mockCreateObjectURL).toHaveBeenCalled();
      expect(mockRevokeObjectURL).toHaveBeenCalled();
      expect(realAnchor.download).toBe('nexorious_collection_20250101.json');
    });

    it('handles expired export file', async () => {
      server.use(
        http.get(`${API_URL}/export/export-old/download`, () => {
          return HttpResponse.json({ detail: 'Export file has expired.' }, { status: 410 });
        }),
      );

      const { result } = renderHook(() => useDownloadExport(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('export-old');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Export file has expired.');
    });

    it('handles export not ready', async () => {
      server.use(
        http.get(`${API_URL}/export/export-pending/download`, () => {
          return HttpResponse.json(
            { detail: 'Export not ready. Current status: processing' },
            { status: 400 },
          );
        }),
      );

      const { result } = renderHook(() => useDownloadExport(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('export-pending');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toContain('Export not ready');
    });
  });
});
