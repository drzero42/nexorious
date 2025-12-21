import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { setAuthHandlers } from '@/api/client';
import {
  useImportNexorious,
  useExportCollection,
  useDownloadExport,
  importExportKeys,
} from './use-import-export';
import { ExportFormat } from '@/types';

const API_URL = '/api';

describe('use-import-export hooks', () => {
  let mockGetAccessToken: Mock<() => string | null>;
  let mockRefreshTokens: Mock<() => Promise<boolean>>;
  let mockLogout: Mock<() => void>;

  beforeEach(() => {
    vi.clearAllMocks();

    mockGetAccessToken = vi.fn<() => string | null>().mockReturnValue('test-access-token');
    mockRefreshTokens = vi.fn<() => Promise<boolean>>().mockResolvedValue(false);
    mockLogout = vi.fn<() => void>();

    setAuthHandlers(mockGetAccessToken, mockRefreshTokens, mockLogout);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('importExportKeys', () => {
    it('generates correct query keys for all', () => {
      expect(importExportKeys.all).toEqual(['import-export']);
    });

    it('generates correct query keys for jobs', () => {
      expect(importExportKeys.jobs()).toEqual(['import-export', 'jobs']);
    });
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
        })
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
          return HttpResponse.json(
            { detail: 'Invalid JSON file' },
            { status: 400 }
          );
        })
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
            { status: 409 }
          );
        })
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
  });

  describe('useExportCollection', () => {
    it('starts JSON export and returns job info', async () => {
      server.use(
        http.post(`${API_URL}/export/collection/json`, () => {
          return HttpResponse.json({
            job_id: 'export-123',
            status: 'pending',
            message: 'Export job created. Check job status for progress.',
            estimated_items: 50,
          });
        })
      );

      const { result } = renderHook(() => useExportCollection(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(ExportFormat.JSON);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.job_id).toBe('export-123');
      expect(result.current.data?.estimated_items).toBe(50);
    });

    it('starts CSV export and returns job info', async () => {
      server.use(
        http.post(`${API_URL}/export/collection/csv`, () => {
          return HttpResponse.json({
            job_id: 'export-456',
            status: 'pending',
            message: 'Export job created.',
            estimated_items: 100,
          });
        })
      );

      const { result } = renderHook(() => useExportCollection(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync(ExportFormat.CSV);
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.job_id).toBe('export-456');
      expect(result.current.data?.estimated_items).toBe(100);
    });

    it('handles empty collection error', async () => {
      server.use(
        http.post(`${API_URL}/export/collection/json`, () => {
          return HttpResponse.json(
            { detail: 'No games in collection to export.' },
            { status: 400 }
          );
        })
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
        })
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
          return HttpResponse.json(
            { detail: 'Export file has expired.' },
            { status: 410 }
          );
        })
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
            { status: 400 }
          );
        })
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
