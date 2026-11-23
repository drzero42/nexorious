import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { JobProgressCard } from './job-progress-card';
import type { Job } from '@/types';
import { JobType, JobSource, JobStatus } from '@/types';

vi.mock('@/hooks', () => ({
  useJobSourceLabel: () => (source: string) => source,
}));

const processingJob: Job = {
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
    failed: 5,
    done: 52,
    total: 100,
    percent: 52,
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

describe('JobProgressCard', () => {
  it('shows the server count and the review backlog', () => {
    render(<JobProgressCard job={processingJob} onCancel={vi.fn()} />);

    expect(screen.getByText('52 / 100 (52%)')).toBeInTheDocument();
    expect(screen.getByText('3 waiting for your review')).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Progress' })).toHaveAttribute(
      'aria-valuenow',
      '52',
    );
  });
});
