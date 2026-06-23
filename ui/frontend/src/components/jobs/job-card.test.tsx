import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { JobCard } from './job-card';
import type { Job } from '@/types';
import { JobType, JobSource, JobStatus } from '@/types';

// JobCard derives storefront labels from the catalog via useJobSourceLabel.
// Mock it to a deterministic labeller so these render tests stay provider-free.
vi.mock('@/hooks', () => ({
  useJobSourceLabel:
    () =>
    (source: string): string =>
      (
        ({
          steam: 'Steam',
          'epic-games-store': 'Epic Games Store',
          gog: 'GOG',
        }) as Record<string, string>
      )[source] ?? source,
  useDateFormat: () => ({
    formatDate: (v: string | null | undefined) => (v ? new Date(v).toLocaleDateString() : '-'),
    formatDateTime: (v: string | null | undefined) => (v ? new Date(v).toLocaleString() : '-'),
    formatRelativeTime: (v: string | null | undefined, fallback = '-') =>
      v ? new Date(v).toLocaleString() : fallback,
  }),
}));

const mockJob: Job = {
  id: 'job-1',
  userId: 'user-1',
  jobType: JobType.SYNC,
  source: JobSource.STEAM,
  status: JobStatus.PROCESSING,
  priority: 'high',
  progress: {
    pending: 20,
    processing: 5,
    completed: 50,
    pendingReview: 3,
    skipped: 2,
    failed: 0,
    total: 100,
    percent: 50,
  },
  totalItems: 100,
  errorMessage: null,
  filePath: null,
  createdAt: '2025-01-01T00:00:00Z',
  startedAt: '2025-01-01T00:01:00Z',
  completedAt: null,
  isTerminal: false,
  durationSeconds: 60,
};

const mockCompletedJob: Job = {
  ...mockJob,
  id: 'job-2',
  status: JobStatus.COMPLETED,
  isTerminal: true,
  completedAt: '2025-01-01T00:02:00Z',
  progress: {
    ...mockJob.progress,
    pending: 0,
    processing: 0,
    completed: 100,
    percent: 100,
  },
};

const mockFailedJob: Job = {
  ...mockJob,
  id: 'job-3',
  status: JobStatus.FAILED,
  isTerminal: true,
  errorMessage: 'Connection timed out',
  completedAt: '2025-01-01T00:02:00Z',
};

describe('JobCard', () => {
  describe('compact mode', () => {
    it('renders compact card with basic info', () => {
      render(<JobCard job={mockJob} compact />);

      expect(screen.getByText('Sync - Steam')).toBeInTheDocument();
      expect(screen.getByText('Processing')).toBeInTheDocument();
    });

    it('shows progress bar when job is processing', () => {
      render(<JobCard job={mockJob} compact />);

      // Progress bar should be visible
      const progressBar = document.querySelector('[role="progressbar"]');
      expect(progressBar).toBeInTheDocument();
    });

    it('does not show progress bar for completed jobs', () => {
      render(<JobCard job={mockCompletedJob} compact />);

      const progressBar = document.querySelector('[role="progressbar"]');
      expect(progressBar).not.toBeInTheDocument();
    });

    it('links to job detail page', () => {
      render(<JobCard job={mockJob} compact />);

      const link = screen.getByRole('link');
      expect(link).toHaveAttribute('href', '/jobs/job-1');
    });
  });

  describe('full card mode', () => {
    it('renders full card with all info', () => {
      render(<JobCard job={mockJob} />);

      expect(screen.getByText('Sync - Steam')).toBeInTheDocument();
      expect(screen.getByText('Processing')).toBeInTheDocument();
      expect(screen.getByText('Duration:')).toBeInTheDocument();
      expect(screen.getByText('1m')).toBeInTheDocument();
      expect(screen.getByText('Pending Review:')).toBeInTheDocument();
      expect(screen.getByText('3')).toBeInTheDocument();
    });

    it('shows progress section for processing jobs', () => {
      render(<JobCard job={mockJob} />);

      expect(screen.getByText('Progress')).toBeInTheDocument();
      // 50 completed + 3 pending review + 2 skipped + 0 failed = 55 done / 100 total
      expect(screen.getByText('55 / 100 (50%)')).toBeInTheDocument();
    });

    it('does not show progress section for completed jobs', () => {
      render(<JobCard job={mockCompletedJob} />);

      expect(screen.queryByText('Progress')).not.toBeInTheDocument();
    });

    it('shows error message for failed jobs', () => {
      render(<JobCard job={mockFailedJob} />);

      expect(screen.getByText('Connection timed out')).toBeInTheDocument();
    });

    it('shows Cancel button for cancellable jobs', () => {
      const onCancel = vi.fn();
      render(<JobCard job={mockJob} onCancel={onCancel} />);

      expect(screen.getByText('Cancel')).toBeInTheDocument();
    });

    it('does not show Cancel button for terminal jobs', () => {
      const onCancel = vi.fn();
      render(<JobCard job={mockCompletedJob} onCancel={onCancel} />);

      expect(screen.queryByText('Cancel')).not.toBeInTheDocument();
    });

    it('shows Delete button for terminal jobs', () => {
      const onDelete = vi.fn();
      render(<JobCard job={mockCompletedJob} onDelete={onDelete} />);

      expect(screen.getByText('Delete')).toBeInTheDocument();
    });

    it('calls onCancel when Cancel button is clicked', async () => {
      const user = userEvent.setup();
      const onCancel = vi.fn();
      render(<JobCard job={mockJob} onCancel={onCancel} />);

      await user.click(screen.getByText('Cancel'));

      expect(onCancel).toHaveBeenCalledWith(mockJob);
    });

    it('calls onDelete when Delete button is clicked', async () => {
      const user = userEvent.setup();
      const onDelete = vi.fn();
      render(<JobCard job={mockCompletedJob} onDelete={onDelete} />);

      await user.click(screen.getByText('Delete'));

      expect(onDelete).toHaveBeenCalledWith(mockCompletedJob);
    });

    it('shows loading state for Cancel button', () => {
      render(<JobCard job={mockJob} onCancel={vi.fn()} isCancelling />);

      const cancelButton = screen.getByText('Cancel').closest('button');
      expect(cancelButton).toBeDisabled();
    });

    it('shows loading state for Delete button', () => {
      render(<JobCard job={mockJob} onDelete={vi.fn()} isDeleting />);

      const deleteButton = screen.getByText('Delete').closest('button');
      expect(deleteButton).toBeDisabled();
    });
  });

  describe('job types and sources', () => {
    it.each([
      [{ jobType: JobType.IMPORT }, 'Import - Steam'],
      [{ jobType: JobType.EXPORT }, 'Export - Steam'],
      [{ source: JobSource.EPIC_GAMES_STORE }, 'Sync - Epic Games Store'],
      [{ source: JobSource.GOG }, 'Sync - GOG'],
    ] as const)('renders %o as "%s"', (overrides, expected) => {
      render(<JobCard job={{ ...mockJob, ...overrides }} />);

      expect(screen.getByText(expected)).toBeInTheDocument();
    });
  });

  describe('job statuses', () => {
    it.each([
      [{ status: JobStatus.PENDING }, 'Pending'],
      [{ status: JobStatus.FAILED, isTerminal: true }, 'Failed'],
      [{ status: JobStatus.CANCELLED, isTerminal: true }, 'Cancelled'],
    ] as const)('renders status %o as "%s"', (overrides, expected) => {
      render(<JobCard job={{ ...mockJob, ...overrides }} />);

      expect(screen.getByText(expected)).toBeInTheDocument();
    });
  });
});
