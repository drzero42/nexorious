import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { JobCard } from './job-card';
import type { Job } from '@/types';
import { JobType, JobSource, JobStatus, JobPriority } from '@/types';


const mockJob: Job = {
  id: 'job-1',
  userId: 'user-1',
  jobType: JobType.SYNC,
  source: JobSource.STEAM,
  status: JobStatus.PROCESSING,
  priority: JobPriority.HIGH,
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

    it('shows View Details button', () => {
      render(<JobCard job={mockJob} />);

      expect(screen.getByText('View Details')).toBeInTheDocument();
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

    it('shows Delete button for non-terminal jobs', () => {
      const onDelete = vi.fn();
      render(<JobCard job={mockJob} onDelete={onDelete} />);

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
    it('displays Import job type correctly', () => {
      const importJob = { ...mockJob, jobType: JobType.IMPORT };
      render(<JobCard job={importJob} />);

      expect(screen.getByText('Import - Steam')).toBeInTheDocument();
    });

    it('displays Export job type correctly', () => {
      const exportJob = { ...mockJob, jobType: JobType.EXPORT };
      render(<JobCard job={exportJob} />);

      expect(screen.getByText('Export - Steam')).toBeInTheDocument();
    });

    it('displays Epic source correctly', () => {
      const epicJob = { ...mockJob, source: JobSource.EPIC };
      render(<JobCard job={epicJob} />);

      expect(screen.getByText('Sync - Epic Games')).toBeInTheDocument();
    });

    it('displays GOG source correctly', () => {
      const gogJob = { ...mockJob, source: JobSource.GOG };
      render(<JobCard job={gogJob} />);

      expect(screen.getByText('Sync - GOG')).toBeInTheDocument();
    });

    it('displays Darkadia source correctly', () => {
      const darkadiaJob = { ...mockJob, source: JobSource.DARKADIA };
      render(<JobCard job={darkadiaJob} />);

      expect(screen.getByText('Sync - Darkadia')).toBeInTheDocument();
    });
  });

  describe('job statuses', () => {
    it('displays Pending status correctly', () => {
      const pendingJob = { ...mockJob, status: JobStatus.PENDING };
      render(<JobCard job={pendingJob} />);

      expect(screen.getByText('Pending')).toBeInTheDocument();
    });

    it('displays Failed status correctly', () => {
      render(<JobCard job={mockFailedJob} />);

      expect(screen.getByText('Failed')).toBeInTheDocument();
    });

    it('displays Cancelled status correctly', () => {
      const cancelledJob = { ...mockJob, status: JobStatus.CANCELLED, isTerminal: true };
      render(<JobCard job={cancelledJob} />);

      expect(screen.getByText('Cancelled')).toBeInTheDocument();
    });
  });
});
