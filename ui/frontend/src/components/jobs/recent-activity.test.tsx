import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@/test/test-utils';
import { RecentActivity } from './recent-activity';
import type { RecentJobDetail } from '@/types';
import { JobType } from '@/types';

// Mock the data hook so the component renders synchronously.
const downloadExport = vi.fn();
vi.mock('@/hooks', () => ({
  useRecentJobs: vi.fn(),
  useDownloadExport: () => ({ mutate: downloadExport, isPending: false }),
  useJobSourceLabel: () => (source: string) => source,
}));
// JobItemsDetails fetches on mount; stub it to a marker for the fallback case.
vi.mock('./job-items-details', () => ({
  JobItemsDetails: () => <div data-testid="job-items-details" />,
}));

import { useRecentJobs } from '@/hooks';

const baseJob = (over: Partial<RecentJobDetail>): RecentJobDetail => ({
  id: 'j1',
  jobType: JobType.SYNC,
  source: 'steam' as RecentJobDetail['source'],
  status: 'completed',
  createdAt: '2026-06-01T00:00:00Z',
  completedAt: '2026-06-01T00:01:00Z',
  errorMessage: null,
  totalItems: 1,
  completedCount: 1,
  skippedCount: 0,
  failedCount: 0,
  progress: {
    pending: 0,
    processing: 0,
    completed: 1,
    pendingReview: 0,
    skipped: 0,
    failed: 0,
    total: 1,
    percent: 100,
  },
  addedItems: [],
  updatedItems: [],
  removedItems: [],
  statusChangedItems: [],
  skippedItems: [],
  alreadyInLibraryItems: [],
  ...over,
});

describe('RecentActivity', () => {
  it('renders the rich breakdown when change rows exist', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: {
        jobs: [baseJob({ addedItems: [{ title: 'Portal', oldStatus: null, newStatus: null }] })],
      },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity source="steam" />);

    // Expand the job row to reveal the breakdown content.
    const trigger = screen.getByRole('button', { name: /completed/i });
    fireEvent.click(trigger);

    expect(screen.getByText('Added to library')).toBeInTheDocument();
    expect(screen.queryByTestId('job-items-details')).not.toBeInTheDocument();
  });

  it('falls back to per-item details when there are no change rows', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: {
        jobs: [baseJob({ jobType: JobType.METADATA_REFRESH })],
      },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity jobTypes={[JobType.METADATA_REFRESH]} />);
    // The card itself renders (positive signal), but with no change rows the
    // breakdown labels must be absent.
    expect(screen.getByText('Recent Activity')).toBeInTheDocument();
    expect(screen.queryByText('Added to library')).not.toBeInTheDocument();

    // Expanding the row reveals the per-item details fallback (not the breakdown).
    fireEvent.click(screen.getByRole('button', { name: /completed/i }));
    expect(screen.getByTestId('job-items-details')).toBeInTheDocument();
  });

  it('shows a games count (not "completed") in the header for a completed export', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: {
        jobs: [
          baseJob({
            jobType: JobType.EXPORT,
            source: 'nexorious' as RecentJobDetail['source'],
            totalItems: 1731,
          }),
        ],
      },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity jobTypes={[JobType.EXPORT]} />);
    expect(screen.getByText('1731 games')).toBeInTheDocument();
    expect(screen.queryByText(/\bcompleted\b/)).not.toBeInTheDocument();
  });

  it('offers a download (not per-item details) when expanding a completed export', () => {
    vi.mocked(useRecentJobs).mockReturnValue({
      data: {
        jobs: [
          baseJob({ jobType: JobType.EXPORT, source: 'nexorious' as RecentJobDetail['source'] }),
        ],
      },
      isLoading: false,
    } as ReturnType<typeof useRecentJobs>);

    render(<RecentActivity jobTypes={[JobType.EXPORT]} />);
    // Status badge ("Completed") is the row trigger's accessible name.
    fireEvent.click(screen.getByRole('button', { name: /completed/i }));

    const downloadBtn = screen.getByRole('button', { name: /download export/i });
    expect(downloadBtn).toBeInTheDocument();
    expect(screen.queryByTestId('job-items-details')).not.toBeInTheDocument();

    fireEvent.click(downloadBtn);
    expect(downloadExport).toHaveBeenCalledWith('j1', expect.anything());
  });
});
