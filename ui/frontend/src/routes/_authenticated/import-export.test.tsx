import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { JobType, JobSource, JobStatus } from '@/types';
import { ImportExportPage } from './import-export';

// Mutable holder for the active job the mocked hooks return (vi.mock is hoisted).
const h = vi.hoisted(() => ({ job: undefined as unknown }));

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn(), setQueryData: vi.fn() }),
}));

// Stub the heavy job components; the review surface is a marker we assert on.
vi.mock('@/components/jobs', () => ({
  JobProgressCard: () => <div data-testid="job-progress-card" />,
  RecentActivity: () => <div data-testid="recent-activity" />,
  JobItemsDetails: () => <div data-testid="import-review" />,
}));

vi.mock('@/hooks', () => ({
  useImportNexorious: () => ({ mutateAsync: vi.fn() }),
  useImportDarkadia: () => ({ mutateAsync: vi.fn() }),
  useExportCollection: () => ({ mutateAsync: vi.fn() }),
  useJob: (id?: string) => ({ data: id === 'job1' ? h.job : undefined }),
  useJobTypeStatus: (type: JobType) => ({
    data:
      type === JobType.IMPORT
        ? { isActive: true, activeJobId: 'job1', lastCompletedJobId: null, lastCompletedAt: null }
        : undefined,
  }),
  useJobCompletionEffect: () => {},
  useCancelJob: () => ({ mutate: vi.fn(), isPending: false }),
  useDownloadExport: () => ({ mutate: vi.fn(), isPending: false }),
  useRetryFailedItems: () => ({ mutateAsync: vi.fn(), isPending: false }),
  jobsKeys: {
    lists: () => ['lists'],
    recents: () => ['recents'],
    typeStatus: (t: JobType) => ['typeStatus', t],
  },
}));

function makeJob(source: JobSource) {
  return {
    id: 'job1',
    userId: 'u',
    jobType: JobType.IMPORT,
    source,
    status: JobStatus.PROCESSING,
    priority: 'high',
    progress: { pending: 5, processing: 0, completed: 10, pendingReview: 3, skipped: 0, failed: 0 },
    totalItems: 18,
    errorMessage: null,
    filePath: null,
    createdAt: '2026-06-05T00:00:00Z',
    startedAt: '2026-06-05T00:00:00Z',
    completedAt: null,
    isTerminal: false,
    durationSeconds: null,
  };
}

describe('ImportExportPage review surface', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders the per-item review surface for an active Darkadia import', () => {
    h.job = makeJob(JobSource.DARKADIA);
    render(<ImportExportPage />);
    expect(screen.getByTestId('import-review')).toBeInTheDocument();
  });

  it('does not render the review surface for an active Nexorious import', () => {
    h.job = makeJob(JobSource.NEXORIOUS);
    render(<ImportExportPage />);
    expect(screen.queryByTestId('import-review')).not.toBeInTheDocument();
  });
});
