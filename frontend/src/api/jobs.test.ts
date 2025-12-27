import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as jobsApi from './jobs';
import { api } from './client';
import { JobType, JobSource, JobStatus, JobPriority } from '@/types';

vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
  },
}));

// Mock API response data (snake_case from backend)
const mockJobApiResponse = {
  id: 'job-1',
  user_id: 'user-1',
  job_type: 'sync',
  source: 'steam',
  status: 'processing',
  priority: 'high',
  progress: {
    pending: 10,
    processing: 5,
    completed: 30,
    pending_review: 3,
    skipped: 2,
    failed: 0,
    total: 50,
    percent: 70,
  },
  total_items: 50,
  error_message: null,
  file_path: null,
  created_at: '2025-01-01T00:00:00Z',
  started_at: '2025-01-01T00:01:00Z',
  completed_at: null,
  is_terminal: false,
  duration_seconds: 60,
};

describe('jobsApi', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('getJobs', () => {
    it('should fetch and transform jobs list', async () => {
      const mockResponse = {
        jobs: [mockJobApiResponse],
        total: 1,
        page: 1,
        per_page: 20,
        pages: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.getJobs();

      expect(api.get).toHaveBeenCalledWith('/jobs/', {
        params: { page: 1, per_page: 20 },
      });
      expect(result.jobs).toHaveLength(1);
      expect(result.jobs[0].id).toBe('job-1');
      expect(result.jobs[0].userId).toBe('user-1');
      expect(result.jobs[0].jobType).toBe(JobType.SYNC);
      expect(result.jobs[0].source).toBe(JobSource.STEAM);
      expect(result.jobs[0].status).toBe(JobStatus.PROCESSING);
      expect(result.jobs[0].priority).toBe(JobPriority.HIGH);
      expect(result.jobs[0].progress.pending).toBe(10);
      expect(result.jobs[0].progress.completed).toBe(30);
      expect(result.jobs[0].progress.pendingReview).toBe(3);
      expect(result.jobs[0].progress.total).toBe(50);
      expect(result.jobs[0].progress.percent).toBe(70);
      expect(result.jobs[0].totalItems).toBe(50);
      expect(result.jobs[0].isTerminal).toBe(false);
      expect(result.jobs[0].durationSeconds).toBe(60);
      expect(result.total).toBe(1);
      expect(result.page).toBe(1);
      expect(result.perPage).toBe(20);
      expect(result.pages).toBe(1);
    });

    it('should pass filters correctly', async () => {
      const mockResponse = {
        jobs: [],
        total: 0,
        page: 1,
        per_page: 10,
        pages: 0,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await jobsApi.getJobs(
        {
          jobType: JobType.IMPORT,
          source: JobSource.DARKADIA,
          status: JobStatus.COMPLETED,
          sortBy: 'created_at',
          sortOrder: 'desc',
        },
        2,
        10
      );

      expect(api.get).toHaveBeenCalledWith('/jobs/', {
        params: {
          page: 2,
          per_page: 10,
          job_type: 'import',
          source: 'darkadia',
          status: 'completed',
          sort_by: 'created_at',
          sort_order: 'desc',
        },
      });
    });

    it('should omit undefined filters', async () => {
      const mockResponse = {
        jobs: [],
        total: 0,
        page: 1,
        per_page: 20,
        pages: 0,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await jobsApi.getJobs({ jobType: JobType.SYNC });

      expect(api.get).toHaveBeenCalledWith('/jobs/', {
        params: {
          page: 1,
          per_page: 20,
          job_type: 'sync',
        },
      });
    });
  });

  describe('getJob', () => {
    it('should fetch and transform single job', async () => {
      vi.mocked(api.get).mockResolvedValueOnce(mockJobApiResponse);

      const result = await jobsApi.getJob('job-1');

      expect(api.get).toHaveBeenCalledWith('/jobs/job-1');
      expect(result.id).toBe('job-1');
      expect(result.userId).toBe('user-1');
      expect(result.jobType).toBe(JobType.SYNC);
      expect(result.source).toBe(JobSource.STEAM);
      expect(result.status).toBe(JobStatus.PROCESSING);
      expect(result.totalItems).toBe(50);
      expect(result.progress.percent).toBe(70);
    });
  });

  describe('cancelJob', () => {
    it('should cancel job and return transformed response', async () => {
      const cancelledJob = {
        ...mockJobApiResponse,
        status: 'cancelled',
        is_terminal: true,
        completed_at: '2025-01-01T00:02:00Z',
      };

      const mockResponse = {
        success: true,
        message: 'Job cancelled successfully',
        job: cancelledJob,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.cancelJob('job-1');

      expect(api.post).toHaveBeenCalledWith('/jobs/job-1/cancel');
      expect(result.success).toBe(true);
      expect(result.message).toBe('Job cancelled successfully');
      expect(result.job?.status).toBe(JobStatus.CANCELLED);
      expect(result.job?.isTerminal).toBe(true);
    });

    it('should handle cancel failure with null job', async () => {
      const mockResponse = {
        success: false,
        message: 'Cannot cancel completed job',
        job: null,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.cancelJob('job-1');

      expect(result.success).toBe(false);
      expect(result.message).toBe('Cannot cancel completed job');
      expect(result.job).toBeNull();
    });
  });

  describe('deleteJob', () => {
    it('should delete job and return response', async () => {
      const mockResponse = {
        success: true,
        message: 'Job deleted successfully',
        deleted_job_id: 'job-1',
      };

      vi.mocked(api.delete).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.deleteJob('job-1');

      expect(api.delete).toHaveBeenCalledWith('/jobs/job-1');
      expect(result.success).toBe(true);
      expect(result.message).toBe('Job deleted successfully');
      expect(result.deletedJobId).toBe('job-1');
    });
  });

  describe('getJobsSummary', () => {
    it('should fetch and transform jobs summary', async () => {
      const mockResponse = {
        running_count: 3,
        failed_count: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.getJobsSummary();

      expect(api.get).toHaveBeenCalledWith('/jobs/summary');
      expect(result.runningCount).toBe(3);
      expect(result.failedCount).toBe(1);
    });
  });

  describe('retryFailedItems', () => {
    it('should call POST /jobs/{jobId}/retry-failed', async () => {
      const mockResponse = {
        success: true,
        message: 'Retrying 3 failed items',
        retried_count: 3,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.retryFailedItems('job-123');

      expect(api.post).toHaveBeenCalledWith('/jobs/job-123/retry-failed');
      expect(result).toEqual({
        success: true,
        message: 'Retrying 3 failed items',
        retriedCount: 3,
      });
    });
  });

  describe('retryJobItem', () => {
    it('should call POST /job-items/{itemId}/retry', async () => {
      const mockResponse = {
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
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.retryJobItem('item-123');

      expect(api.post).toHaveBeenCalledWith('/job-items/item-123/retry');
      expect(result.status).toBe('pending');
    });
  });
});
