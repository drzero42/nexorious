import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RecentActivity } from './recent-activity';
import type { RecentJobDetail, SyncChangeItem } from '@/types';

// Suppress TanStack Query console errors in tests.
vi.mock('@/hooks', () => ({
  useRecentJobs: vi.fn(),
}));
import { useRecentJobs } from '@/hooks';

const makeItem = (title: string): SyncChangeItem => ({ title });

const baseJob: RecentJobDetail = {
  id: 'j1',
  status: 'completed',
  createdAt: '2026-01-01T00:00:00Z',
  completedAt: '2026-01-01T01:00:00Z',
  totalItems: 10,
  completedCount: 8,
  skippedCount: 2,
  failedCount: 0,
  addedItems: [makeItem('New Game A'), makeItem('New Game B')],
  removedItems: [],
  statusChangedItems: [],
  skippedItems: [],
  alreadyInLibraryItems: [],
};

describe('RecentActivity component', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders skippedItems section when items are present', async () => {
    const job: RecentJobDetail = {
      ...baseJob,
      skippedItems: [makeItem('Skipped Game 1'), makeItem('Skipped Game 2')],
    };
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [job] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    // Expand the job card first.
    const trigger = screen.getByRole('button', { name: /completed/i });
    await userEvent.click(trigger);

    expect(screen.getByText('Skipped')).toBeInTheDocument();
  });

  it('renders alreadyInLibraryItems section when items are present', async () => {
    const job: RecentJobDetail = {
      ...baseJob,
      alreadyInLibraryItems: [makeItem('Old Game A'), makeItem('Old Game B'), makeItem('Old Game C')],
    };
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [job] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    const trigger = screen.getByRole('button', { name: /completed/i });
    await userEvent.click(trigger);

    expect(screen.getByText('Already in library')).toBeInTheDocument();
  });

  it('does not render skippedItems section when array is empty', () => {
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [baseJob] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    expect(screen.queryByText('Skipped')).not.toBeInTheDocument();
  });

  it('does not render alreadyInLibraryItems section when array is empty', () => {
    (useRecentJobs as ReturnType<typeof vi.fn>).mockReturnValue({
      data: { jobs: [baseJob] },
      isLoading: false,
      error: null,
    });

    render(<RecentActivity platform="steam" />);

    expect(screen.queryByText('Already in library')).not.toBeInTheDocument();
  });
});
