import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { JobStatus, JobType } from '@/types';
import { MaintenancePage } from './maintenance';

const h = vi.hoisted(() => ({ status: 'completed' as string }));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  useNavigate: () => vi.fn(),
}));
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn(), clear: vi.fn() }),
}));
vi.mock('@/providers', () => ({ useAuth: () => ({ user: { isAdmin: true } }) }));
vi.mock('@/hooks/use-health-status', () => ({
  useHealthStatus: () => ({ data: { igdb_status: 'ok' } }),
}));
vi.mock('@/api/admin', () => ({}));
vi.mock('@/hooks', () => ({
  useJobTypeStatus: (type: JobType) => ({
    data:
      type === JobType.METADATA_REFRESH
        ? { isActive: false, activeJobId: null, lastCompletedJobId: 'job1', lastCompletedAt: null }
        : undefined,
  }),
  useJob: () => ({
    data: {
      id: 'job1',
      jobType: JobType.METADATA_REFRESH,
      status: h.status,
      isTerminal: true,
      progress: {
        pending: 0,
        completed: 1,
        pendingReview: 0,
        skipped: 0,
        failed: 1,
        cancelled: 0,
      },
    },
  }),
  useJobCompletionEffect: () => {},
  useCancelJob: () => ({ mutate: vi.fn(), isPending: false }),
  jobsKeys: { lists: () => [], recents: () => [], typeStatus: () => [] },
}));
vi.mock('@/components/jobs', () => ({
  JobProgressCard: () => <div data-testid="job-progress-card" />,
  RecentActivity: () => null,
  JobItemsDetails: ({ jobStatus }: { jobStatus: string }) => (
    <div data-testid="job-items-details" data-job-status={jobStatus} />
  ),
}));

// The page hands the refresh Job's status to the item details, which offer a
// retry only for a completed Job: a failed or cancelled refresh is never
// reopened.
describe('MaintenancePage item details', () => {
  it.each([JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED])(
    'passes a %s job status through',
    (status) => {
      h.status = status;
      render(<MaintenancePage />);
      expect(screen.getByTestId('job-items-details')).toHaveAttribute('data-job-status', status);
    },
  );
});
