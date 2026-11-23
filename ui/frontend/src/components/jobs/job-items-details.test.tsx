import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import { JobItemsDetails } from './job-items-details';
import { JobStatus } from '@/types';

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

const { mutation } = vi.hoisted(() => ({
  mutation: () => ({ mutateAsync: () => Promise.resolve(), isPending: false }),
}));
vi.mock('@/hooks', () => ({
  useJobItems: () => ({
    data: {
      items: [{ id: 'i1', sourceTitle: 'Broken Game', errorMessage: 'boom' }],
      total: 1,
      pages: 1,
    },
    isLoading: false,
    refetch: vi.fn(),
  }),
  useRetryFailedItems: mutation,
  useRetryJobItem: mutation,
  useResolveJobItem: mutation,
  useSkipJobItem: mutation,
}));

vi.mock('@/components/sync/igdb-match-dialog', () => ({ IGDBMatchDialog: () => null }));

const progress = {
  pending: 0,
  processing: 0,
  completed: 3,
  pendingReview: 0,
  skipped: 0,
  failed: 1,
  cancelled: 2,
};

describe('JobItemsDetails retry', () => {
  it('offers retry for the failed items of a completed job', () => {
    render(<JobItemsDetails jobId="j1" progress={progress} jobStatus={JobStatus.COMPLETED} />);
    expect(screen.getByText('Retry All')).toBeInTheDocument();
    expect(screen.getByTitle('Retry')).toBeInTheDocument();
  });

  // A failed or cancelled Job is never reopened (the server answers 409), and
  // an active one is still being worked.
  it.each([JobStatus.FAILED, JobStatus.CANCELLED, JobStatus.PROCESSING])(
    'offers no retry for a %s job',
    (status) => {
      render(<JobItemsDetails jobId="j1" progress={progress} jobStatus={status} />);
      expect(screen.queryByText('Retry All')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Retry')).not.toBeInTheDocument();
    },
  );

  it('lists cancelled items in a section of their own', () => {
    render(<JobItemsDetails jobId="j1" progress={progress} jobStatus={JobStatus.CANCELLED} />);
    expect(screen.getByText('Cancelled')).toBeInTheDocument();
  });
});
