import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { QueryWrapper } from '@/test/test-utils';
import { setAuthHandlers } from '@/api/client';
import {
  useJobs,
  useJob,
  useCancelJob,
  useDeleteJob,
  useRetryFailedItems,
  useRetryJobItem,
  jobsKeys,
} from './use-jobs';
import { JobType, JobSource, JobStatus, JobItemStatus } from '@/types';

const API_URL = '/api';

// Mock job data (API format - snake_case)
const mockJobApi = {
  id: 'job-1',
  user_id: 'user-1',
  job_type: 'sync',
  source: 'steam',
  status: 'processing',
  priority: 'high',
  progress: {
    pending: 20,
    processing: 5,
    completed: 50,
    pending_review: 3,
    skipped: 2,
    failed: 0,
    total: 100,
    percent: 50,
  },
  total_items: 100,
  error_message: null,
  file_path: null,
  created_at: '2025-01-01T00:00:00Z',
  started_at: '2025-01-01T00:01:00Z',
  completed_at: null,
  is_terminal: false,
  duration_seconds: 60,
};

const mockCompletedJobApi = {
  ...mockJobApi,
  id: 'job-2',
  status: 'completed',
  is_terminal: true,
  completed_at: '2025-01-01T00:02:00Z',
  progress: {
    ...mockJobApi.progress,
    pending: 0,
    processing: 0,
    completed: 100,
    percent: 100,
  },
};

describe('use-jobs hooks', () => {
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

  describe('jobsKeys', () => {
    it('generates correct query keys for all', () => {
      expect(jobsKeys.all).toEqual(['jobs']);
    });

    it('generates correct query keys for lists', () => {
      expect(jobsKeys.lists()).toEqual(['jobs', 'list']);
    });

    it('generates correct query keys for list with filters', () => {
      expect(jobsKeys.list()).toEqual(['jobs', 'list', { filters: undefined, page: undefined, perPage: undefined }]);
      expect(jobsKeys.list({ jobType: JobType.SYNC }, 1, 20)).toEqual([
        'jobs',
        'list',
        { filters: { jobType: 'sync' }, page: 1, perPage: 20 },
      ]);
    });

    it('generates correct query keys for details', () => {
      expect(jobsKeys.details()).toEqual(['jobs', 'detail']);
    });

    it('generates correct query keys for detail with id', () => {
      expect(jobsKeys.detail('job-1')).toEqual(['jobs', 'detail', 'job-1']);
    });
  });

  describe('useJobs', () => {
    it('fetches jobs list successfully', async () => {
      server.use(
        http.get(`${API_URL}/jobs/`, () => {
          return HttpResponse.json({
            jobs: [mockJobApi, mockCompletedJobApi],
            total: 2,
            page: 1,
            per_page: 20,
            pages: 1,
          });
        })
      );

      const { result } = renderHook(() => useJobs(), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.jobs).toHaveLength(2);
      expect(result.current.data?.jobs[0].jobType).toBe(JobType.SYNC);
      expect(result.current.data?.jobs[0].source).toBe(JobSource.STEAM);
      expect(result.current.data?.jobs[0].status).toBe(JobStatus.PROCESSING);
      expect(result.current.data?.total).toBe(2);
    });

    it('passes filters to API', async () => {
      let capturedParams: URLSearchParams | null = null;

      server.use(
        http.get(`${API_URL}/jobs/`, ({ request }) => {
          const url = new URL(request.url);
          capturedParams = url.searchParams;

          return HttpResponse.json({
            jobs: [],
            total: 0,
            page: 2,
            per_page: 10,
            pages: 0,
          });
        })
      );

      const { result } = renderHook(
        () =>
          useJobs(
            { jobType: JobType.IMPORT, source: JobSource.DARKADIA, status: JobStatus.COMPLETED },
            2,
            10
          ),
        { wrapper: QueryWrapper }
      );

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(capturedParams).not.toBeNull();
      expect(capturedParams!.get('job_type')).toBe('import');
      expect(capturedParams!.get('source')).toBe('darkadia');
      expect(capturedParams!.get('status')).toBe('completed');
      expect(capturedParams!.get('page')).toBe('2');
      expect(capturedParams!.get('per_page')).toBe('10');
    });

    it('handles error state', async () => {
      server.use(
        http.get(`${API_URL}/jobs/`, () => {
          return HttpResponse.json({ detail: 'Failed to fetch jobs' }, { status: 500 });
        })
      );

      const { result } = renderHook(() => useJobs(), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Failed to fetch jobs');
    });
  });

  describe('useJob', () => {
    it('fetches single job successfully', async () => {
      server.use(
        http.get(`${API_URL}/jobs/job-1`, () => {
          return HttpResponse.json(mockJobApi);
        })
      );

      const { result } = renderHook(() => useJob('job-1'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.id).toBe('job-1');
      expect(result.current.data?.jobType).toBe(JobType.SYNC);
      expect(result.current.data?.progress.percent).toBe(50);
    });

    it('does not fetch when jobId is undefined', () => {
      const { result } = renderHook(() => useJob(undefined), {
        wrapper: QueryWrapper,
      });

      expect(result.current.isLoading).toBe(false);
      expect(result.current.isFetching).toBe(false);
    });

    it('handles 404 error', async () => {
      server.use(
        http.get(`${API_URL}/jobs/non-existent`, () => {
          return HttpResponse.json({ detail: 'Job not found' }, { status: 404 });
        })
      );

      const { result } = renderHook(() => useJob('non-existent'), {
        wrapper: QueryWrapper,
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Job not found');
    });
  });

  describe('useCancelJob', () => {
    it('cancels job successfully', async () => {
      const cancelledJob = {
        ...mockJobApi,
        status: 'cancelled',
        is_terminal: true,
      };

      server.use(
        http.post(`${API_URL}/jobs/job-1/cancel`, () => {
          return HttpResponse.json({
            success: true,
            message: 'Job cancelled successfully',
            job: cancelledJob,
          });
        })
      );

      const { result } = renderHook(() => useCancelJob(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('job-1');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.success).toBe(true);
      expect(result.current.data?.job?.status).toBe(JobStatus.CANCELLED);
    });

    it('handles cancel error', async () => {
      server.use(
        http.post(`${API_URL}/jobs/job-1/cancel`, () => {
          return HttpResponse.json({ detail: 'Cannot cancel completed job' }, { status: 400 });
        })
      );

      const { result } = renderHook(() => useCancelJob(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('job-1');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Cannot cancel completed job');
    });
  });

  describe('useDeleteJob', () => {
    it('deletes job successfully', async () => {
      server.use(
        http.delete(`${API_URL}/jobs/job-2`, () => {
          return HttpResponse.json({
            success: true,
            message: 'Job deleted successfully',
            deleted_job_id: 'job-2',
          });
        })
      );

      const { result } = renderHook(() => useDeleteJob(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('job-2');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.success).toBe(true);
      expect(result.current.data?.deletedJobId).toBe('job-2');
    });

    it('handles delete error for non-terminal job', async () => {
      server.use(
        http.delete(`${API_URL}/jobs/job-1`, () => {
          return HttpResponse.json(
            { detail: 'Cannot delete job that is not in terminal state' },
            { status: 400 }
          );
        })
      );

      const { result } = renderHook(() => useDeleteJob(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('job-1');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Cannot delete job that is not in terminal state');
    });
  });

  describe('useRetryFailedItems', () => {
    it('should invalidate queries on success', async () => {
      server.use(
        http.post(`${API_URL}/jobs/job-123/retry-failed`, () => {
          return HttpResponse.json({
            success: true,
            message: 'Retrying 3 items',
            retried_count: 3,
          });
        })
      );

      const { result } = renderHook(() => useRetryFailedItems(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('job-123');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.success).toBe(true);
      expect(result.current.data?.retriedCount).toBe(3);
    });

    it('handles retry error', async () => {
      server.use(
        http.post(`${API_URL}/jobs/job-123/retry-failed`, () => {
          return HttpResponse.json({ detail: 'No failed items to retry' }, { status: 400 });
        })
      );

      const { result } = renderHook(() => useRetryFailedItems(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('job-123');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('No failed items to retry');
    });
  });

  describe('useRetryJobItem', () => {
    it('should invalidate queries on success', async () => {
      server.use(
        http.post(`${API_URL}/job-items/item-123/retry`, () => {
          return HttpResponse.json({
            id: 'item-123',
            job_id: 'job-123',
            item_key: 'game_1',
            source_title: 'Test Game',
            status: 'pending',
            error_message: null,
            result_game_title: null,
            result_igdb_id: null,
            created_at: '2024-01-01T00:00:00Z',
            processed_at: null,
            source_metadata_json: '{}',
            result_json: '{}',
            igdb_candidates_json: '[]',
            resolved_igdb_id: null,
            resolved_at: null,
          });
        })
      );

      const { result } = renderHook(() => useRetryJobItem(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        await result.current.mutateAsync('item-123');
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.id).toBe('item-123');
      expect(result.current.data?.status).toBe(JobItemStatus.PENDING);
    });

    it('handles retry error for non-failed item', async () => {
      server.use(
        http.post(`${API_URL}/job-items/item-123/retry`, () => {
          return HttpResponse.json({ detail: 'Item is not in failed state' }, { status: 400 });
        })
      );

      const { result } = renderHook(() => useRetryJobItem(), {
        wrapper: QueryWrapper,
      });

      await act(async () => {
        try {
          await result.current.mutateAsync('item-123');
        } catch {
          // Expected error
        }
      });

      await waitFor(() => {
        expect(result.current.isError).toBe(true);
      });

      expect(result.current.error?.message).toBe('Item is not in failed state');
    });
  });
});
