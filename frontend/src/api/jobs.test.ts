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
  progress_current: 50,
  progress_total: 100,
  progress_percent: 50,
  result_summary: { games_found: 10 },
  error_message: null,
  file_path: null,
  taskiq_task_id: 'task-123',
  created_at: '2025-01-01T00:00:00Z',
  started_at: '2025-01-01T00:01:00Z',
  completed_at: null,
  is_terminal: false,
  duration_seconds: 60,
  review_item_count: 5,
  pending_review_count: 3,
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
      expect(result.jobs[0].progressCurrent).toBe(50);
      expect(result.jobs[0].progressTotal).toBe(100);
      expect(result.jobs[0].progressPercent).toBe(50);
      expect(result.jobs[0].isTerminal).toBe(false);
      expect(result.jobs[0].durationSeconds).toBe(60);
      expect(result.jobs[0].reviewItemCount).toBe(5);
      expect(result.jobs[0].pendingReviewCount).toBe(3);
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
      expect(result.taskiqTaskId).toBe('task-123');
      expect(result.resultSummary).toEqual({ games_found: 10 });
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

  describe('confirmJob', () => {
    it('should confirm import job and return transformed response', async () => {
      const completedJob = {
        ...mockJobApiResponse,
        job_type: 'import',
        status: 'completed',
        is_terminal: true,
        completed_at: '2025-01-01T00:02:00Z',
      };

      const mockResponse = {
        success: true,
        message: 'Import confirmed successfully',
        job: completedJob,
        games_added: 8,
        games_skipped: 2,
        games_removed: 0,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.confirmJob('job-1');

      expect(api.post).toHaveBeenCalledWith('/jobs/job-1/confirm');
      expect(result.success).toBe(true);
      expect(result.message).toBe('Import confirmed successfully');
      expect(result.job?.status).toBe(JobStatus.COMPLETED);
      expect(result.gamesAdded).toBe(8);
      expect(result.gamesSkipped).toBe(2);
      expect(result.gamesRemoved).toBe(0);
    });

    it('should handle confirm failure with null job', async () => {
      const mockResponse = {
        success: false,
        message: 'Cannot confirm job with pending review items',
        job: null,
        games_added: 0,
        games_skipped: 0,
        games_removed: 0,
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.confirmJob('job-1');

      expect(result.success).toBe(false);
      expect(result.message).toBe('Cannot confirm job with pending review items');
      expect(result.job).toBeNull();
      expect(result.gamesAdded).toBe(0);
    });
  });

  describe('getJobChildren', () => {
    it('should fetch children for a parent job', async () => {
      const childJob1 = {
        ...mockJobApiResponse,
        id: 'child-1',
        status: 'completed',
      };
      const childJob2 = {
        ...mockJobApiResponse,
        id: 'child-2',
        status: 'processing',
      };
      const mockResponse = [childJob1, childJob2];

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.getJobChildren('parent-job-1');

      expect(api.get).toHaveBeenCalledWith('/jobs/parent-job-1/children', {
        params: {},
      });
      expect(result).toHaveLength(2);
      expect(result[0].id).toBe('child-1');
      expect(result[0].status).toBe(JobStatus.COMPLETED);
      expect(result[1].id).toBe('child-2');
      expect(result[1].status).toBe(JobStatus.PROCESSING);
    });

    it('should pass status filter when provided', async () => {
      const mockResponse = [mockJobApiResponse];

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await jobsApi.getJobChildren('parent-job-1', {
        status: JobStatus.COMPLETED,
      });

      expect(api.get).toHaveBeenCalledWith('/jobs/parent-job-1/children', {
        params: { status: 'completed' },
      });
    });

    it('should pass limit and offset filters when provided', async () => {
      const mockResponse = [mockJobApiResponse];

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await jobsApi.getJobChildren('parent-job-1', {
        limit: 10,
        offset: 5,
      });

      expect(api.get).toHaveBeenCalledWith('/jobs/parent-job-1/children', {
        params: { limit: 10, offset: 5 },
      });
    });

    it('should pass all filters when provided', async () => {
      const mockResponse = [mockJobApiResponse];

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await jobsApi.getJobChildren('parent-job-1', {
        status: JobStatus.FAILED,
        limit: 20,
        offset: 10,
      });

      expect(api.get).toHaveBeenCalledWith('/jobs/parent-job-1/children', {
        params: { status: 'failed', limit: 20, offset: 10 },
      });
    });

    it('should return empty array when no children exist', async () => {
      const mockResponse: typeof mockJobApiResponse[] = [];

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await jobsApi.getJobChildren('parent-job-1');

      expect(result).toHaveLength(0);
    });
  });
});
