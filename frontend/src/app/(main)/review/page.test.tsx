import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import ReviewPage from './page';

// Mock next/link
vi.mock('next/link', () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// Mock next/navigation
const mockSearchParams = new URLSearchParams();
const mockReplace = vi.fn();
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    replace: mockReplace,
  }),
  useSearchParams: () => mockSearchParams,
}));

// Mock the hooks
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual('@/hooks');
  return {
    ...actual,
    useReviewItems: vi.fn(),
    useReviewSummary: vi.fn(),
    useMatchReviewItem: vi.fn(() => ({ mutateAsync: vi.fn() })),
    useSkipReviewItem: vi.fn(() => ({ mutateAsync: vi.fn() })),
    useKeepReviewItem: vi.fn(() => ({ mutateAsync: vi.fn() })),
    useRemoveReviewItem: vi.fn(() => ({ mutateAsync: vi.fn() })),
    useSearchIGDB: vi.fn(() => ({ data: undefined, isLoading: false, error: null })),
  };
});

import { useReviewItems, useReviewSummary, useSearchIGDB, useMatchReviewItem } from '@/hooks';

const mockedUseReviewItems = vi.mocked(useReviewItems);
const mockedUseReviewSummary = vi.mocked(useReviewSummary);
const mockedUseSearchIGDB = vi.mocked(useSearchIGDB);
const mockedUseMatchReviewItem = vi.mocked(useMatchReviewItem);

const mockSummaryWithPending = {
  totalPending: 10,
  totalMatched: 5,
  totalSkipped: 2,
  totalRemoval: 1,
  jobsWithPending: 3,
};

const mockSummaryNoPending = {
  totalPending: 0,
  totalMatched: 15,
  totalSkipped: 2,
  totalRemoval: 1,
  jobsWithPending: 0,
};

const mockEmptyItems = {
  items: [],
  total: 0,
  page: 1,
  perPage: 20,
  pages: 0,
};

describe('ReviewPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchParams.delete('status');
    mockSearchParams.delete('source');
    mockSearchParams.delete('job_id');
  });

  it('renders page header', () => {
    mockedUseReviewItems.mockReturnValue({
      data: mockEmptyItems,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
      isFetching: false,
    } as unknown as ReturnType<typeof useReviewItems>);

    mockedUseReviewSummary.mockReturnValue({
      data: mockSummaryWithPending,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useReviewSummary>);

    render(<ReviewPage />);

    expect(screen.getByRole('heading', { name: 'Review Queue' })).toBeInTheDocument();
  });

  it('displays loading skeleton when loading', () => {
    mockedUseReviewItems.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
      isFetching: false,
    } as unknown as ReturnType<typeof useReviewItems>);

    mockedUseReviewSummary.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as unknown as ReturnType<typeof useReviewSummary>);

    render(<ReviewPage />);

    const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  describe('Smart Default Filter', () => {
    it('defaults to pending status filter when there are pending items', async () => {
      mockedUseReviewItems.mockReturnValue({
        data: mockEmptyItems,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryWithPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      render(<ReviewPage />);

      // Wait for the effect to run and check that the status filter shows "Pending"
      await waitFor(() => {
        const statusSelect = screen.getByRole('combobox', { name: /status/i });
        expect(statusSelect).toHaveTextContent('Pending');
      });
    });

    it('shows all statuses when there are no pending items', async () => {
      mockedUseReviewItems.mockReturnValue({
        data: mockEmptyItems,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryNoPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      render(<ReviewPage />);

      await waitFor(() => {
        const statusSelect = screen.getByRole('combobox', { name: /status/i });
        expect(statusSelect).toHaveTextContent('All Statuses');
      });
    });

    it('respects explicit status URL parameter over smart default', async () => {
      mockSearchParams.set('status', 'matched');

      mockedUseReviewItems.mockReturnValue({
        data: mockEmptyItems,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryWithPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      render(<ReviewPage />);

      await waitFor(() => {
        const statusSelect = screen.getByRole('combobox', { name: /status/i });
        // Should NOT be "Pending" because URL explicitly set "matched"
        expect(statusSelect).not.toHaveTextContent('Pending');
      });
    });
  });

  describe('IGDB Search in Modal', () => {
    const mockReviewItem = {
      id: 'item-1',
      sourceTitle: 'Test Game',
      status: 'pending',
      igdbCandidates: [],
      resolvedIgdbId: null,
      matchConfidence: null,
      jobId: 'job-1',
      source: 'import',
      sourceMetadata: null,
      createdAt: '2024-01-01',
      updatedAt: '2024-01-01',
    };

    const mockItemsWithReviewItem = {
      items: [mockReviewItem],
      total: 1,
      page: 1,
      perPage: 20,
      pages: 1,
    };

    it('shows search input in modal when viewing candidates', async () => {
      const user = userEvent.setup();

      mockedUseReviewItems.mockReturnValue({
        data: mockItemsWithReviewItem,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryWithPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      render(<ReviewPage />);

      // Click view button to open modal
      const viewButton = screen.getByRole('button', { name: /search igdb/i });
      await user.click(viewButton);

      // Search input should be visible in modal
      expect(screen.getByPlaceholderText(/search igdb/i)).toBeInTheDocument();
    });

    it('displays search results when typing 3+ characters', async () => {
      const user = userEvent.setup();

      const mockSearchResults = [
        {
          igdb_id: 123,
          title: 'Search Result Game',
          release_date: '2023-01-15',
          cover_art_url: 'https://example.com/cover.jpg',
          platforms: ['PC', 'PlayStation 5'],
          description: 'A great game',
        },
      ];

      mockedUseReviewItems.mockReturnValue({
        data: mockItemsWithReviewItem,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryWithPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      mockedUseSearchIGDB.mockReturnValue({
        data: mockSearchResults,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useSearchIGDB>);

      render(<ReviewPage />);

      // Open modal
      const viewButton = screen.getByRole('button', { name: /search igdb/i });
      await user.click(viewButton);

      // Type search query
      const searchInput = screen.getByPlaceholderText(/search igdb/i);
      await user.type(searchInput, 'Search Result');

      // Results should appear
      expect(screen.getByText('Search Result Game')).toBeInTheDocument();
      expect(screen.getByText('(2023)')).toBeInTheDocument();
    });

    it('matches review item when clicking search result', async () => {
      const user = userEvent.setup();
      const mockMatchMutate = vi.fn().mockResolvedValue({});

      const mockSearchResults = [
        {
          igdb_id: 456,
          title: 'Clicked Game',
          release_date: '2022-06-15',
          cover_art_url: null,
          platforms: ['PC'],
          description: 'Another game',
        },
      ];

      // Re-mock with custom mutateAsync
      mockedUseMatchReviewItem.mockReturnValue({
        mutateAsync: mockMatchMutate,
      } as unknown as ReturnType<typeof useMatchReviewItem>);

      mockedUseReviewItems.mockReturnValue({
        data: mockItemsWithReviewItem,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
        isFetching: false,
      } as unknown as ReturnType<typeof useReviewItems>);

      mockedUseReviewSummary.mockReturnValue({
        data: mockSummaryWithPending,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useReviewSummary>);

      mockedUseSearchIGDB.mockReturnValue({
        data: mockSearchResults,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useSearchIGDB>);

      render(<ReviewPage />);

      // Open modal
      const viewButton = screen.getByRole('button', { name: /search igdb/i });
      await user.click(viewButton);

      // Type and click result
      const searchInput = screen.getByPlaceholderText(/search igdb/i);
      await user.type(searchInput, 'Clicked');

      const resultButton = screen.getByRole('button', { name: /clicked game/i });
      await user.click(resultButton);

      // Verify match was called with correct params
      expect(mockMatchMutate).toHaveBeenCalledWith({
        itemId: 'item-1',
        igdbId: 456,
      });
    });
  });
});
