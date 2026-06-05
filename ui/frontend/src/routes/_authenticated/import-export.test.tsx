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

function makeTerminalJob(opts: { jobType?: JobType; status: JobStatus; failed?: number }) {
  return {
    id: 'job1',
    userId: 'u',
    jobType: opts.jobType ?? JobType.IMPORT,
    source: JobSource.NEXORIOUS,
    status: opts.status,
    priority: 'high',
    progress: {
      pending: 0,
      processing: 0,
      completed: 10,
      pendingReview: 0,
      skipped: 0,
      failed: opts.failed ?? 0,
    },
    totalItems: 10,
    errorMessage: null,
    filePath: null,
    createdAt: '2026-06-05T00:00:00Z',
    startedAt: '2026-06-05T00:00:00Z',
    completedAt: '2026-06-05T00:01:00Z',
    isTerminal: true,
    durationSeconds: 60,
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

describe('ImportExportPage auto-dismiss on clean completion', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('hides the progress box when an import completes cleanly', () => {
    h.job = makeTerminalJob({ status: JobStatus.COMPLETED, failed: 0 });
    render(<ImportExportPage />);
    expect(screen.queryByTestId('job-progress-card')).not.toBeInTheDocument();
  });

  it('hides the progress box (and its Download button) when an export completes cleanly', () => {
    h.job = makeTerminalJob({ jobType: JobType.EXPORT, status: JobStatus.COMPLETED, failed: 0 });
    render(<ImportExportPage />);
    expect(screen.queryByTestId('job-progress-card')).not.toBeInTheDocument();
    expect(screen.queryByText('Download Export')).not.toBeInTheDocument();
  });

  it('keeps the progress box and Retry Failed when a completed job has failed items', () => {
    h.job = makeTerminalJob({ status: JobStatus.COMPLETED, failed: 2 });
    render(<ImportExportPage />);
    expect(screen.getByTestId('job-progress-card')).toBeInTheDocument();
    expect(screen.getByText('Retry Failed')).toBeInTheDocument();
  });

  it('keeps the progress box when a job is cancelled', () => {
    h.job = makeTerminalJob({ status: JobStatus.CANCELLED, failed: 0 });
    render(<ImportExportPage />);
    expect(screen.getByTestId('job-progress-card')).toBeInTheDocument();
    expect(screen.getByText('Start New')).toBeInTheDocument();
  });

  it('keeps the progress box for a terminal failed job', () => {
    h.job = makeTerminalJob({ status: JobStatus.FAILED, failed: 0 });
    render(<ImportExportPage />);
    expect(screen.getByTestId('job-progress-card')).toBeInTheDocument();
  });
});
