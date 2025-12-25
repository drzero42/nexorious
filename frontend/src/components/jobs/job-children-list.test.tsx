import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { JobChildrenList } from './job-children-list';
import * as useJobsHooks from '@/hooks/use-jobs';
import { JobStatus, JobType, JobSource, JobPriority } from '@/types';
import type { Job } from '@/types';

vi.mock('@/hooks/use-jobs');

const mockUseJobChildren = vi.spyOn(useJobsHooks, 'useJobChildren');

const createMockJob = (overrides: Partial<Job>): Job => ({
  id: '1',
  userId: 'user-1',
  jobType: JobType.IMPORT,
  source: JobSource.STEAM,
  status: JobStatus.COMPLETED,
  priority: JobPriority.HIGH,
  progressCurrent: 100,
  progressTotal: 100,
  progressPercent: 100,
  resultSummary: {},
  errorMessage: null,
  filePath: null,
  taskiqTaskId: null,
  createdAt: '2025-01-01T00:00:00Z',
  startedAt: '2025-01-01T00:01:00Z',
  completedAt: '2025-01-01T00:02:00Z',
  isTerminal: true,
  durationSeconds: 60,
  reviewItemCount: null,
  pendingReviewCount: null,
  ...overrides,
});

describe('JobChildrenList', () => {
  it('renders children with status indicators', () => {
    mockUseJobChildren.mockReturnValue({
      data: [
        createMockJob({ id: '1', status: JobStatus.COMPLETED, resultSummary: { title: 'Game 1' }, errorMessage: null, isTerminal: true }),
        createMockJob({ id: '2', status: JobStatus.FAILED, resultSummary: { title: 'Game 2' }, errorMessage: 'Some error', isTerminal: true }),
        createMockJob({ id: '3', status: JobStatus.PROCESSING, resultSummary: { title: 'Game 3' }, errorMessage: null, isTerminal: false }),
      ],
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useJobsHooks.useJobChildren>);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText('Game 1')).toBeInTheDocument();
    expect(screen.getByText('Game 2')).toBeInTheDocument();
    expect(screen.getByText('Game 3')).toBeInTheDocument();
  });

  it('shows loading state', () => {
    mockUseJobChildren.mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
    } as unknown as ReturnType<typeof useJobsHooks.useJobChildren>);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('shows error state', () => {
    mockUseJobChildren.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
    } as unknown as ReturnType<typeof useJobsHooks.useJobChildren>);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText(/failed to load/i)).toBeInTheDocument();
  });

  it('shows empty state when no children', () => {
    mockUseJobChildren.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useJobsHooks.useJobChildren>);

    render(<JobChildrenList jobId="parent-id" />);

    expect(screen.getByText(/no child jobs found/i)).toBeInTheDocument();
  });
});
