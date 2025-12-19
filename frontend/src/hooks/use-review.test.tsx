import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ReactNode } from 'react';
import {
  useReviewItems,
  useReviewItem,
  useReviewSummary,
  useReviewCountsByType,
  usePlatformSummary,
  useMatchReviewItem,
  useSkipReviewItem,
  useKeepReviewItem,
  useRemoveReviewItem,
  useFinalizeImport,
  reviewKeys,
} from './use-review';
import * as reviewApi from '@/api/review';
import { ReviewItemStatus } from '@/types';

// Mock the API
vi.mock('@/api/review', () => ({
  getReviewItems: vi.fn(),
  getReviewItem: vi.fn(),
  getReviewSummary: vi.fn(),
  getReviewCountsByType: vi.fn(),
  getPlatformSummary: vi.fn(),
  matchReviewItem: vi.fn(),
  skipReviewItem: vi.fn(),
  keepReviewItem: vi.fn(),
  removeReviewItem: vi.fn(),
  finalizeImport: vi.fn(),
}));

describe('Review Hooks', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    vi.clearAllMocks();
  });

  afterEach(() => {
    queryClient.clear();
    vi.resetAllMocks();
  });

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  describe('reviewKeys', () => {
    it('should generate correct query keys', () => {
      expect(reviewKeys.all).toEqual(['review']);
      expect(reviewKeys.items({ status: 'pending' as never }, 1)).toEqual([
        'review',
        'items',
        { filters: { status: 'pending' }, page: 1 },
      ]);
      expect(reviewKeys.item('item-1')).toEqual(['review', 'item', 'item-1']);
      expect(reviewKeys.summary()).toEqual(['review', 'summary']);
      expect(reviewKeys.countsByType()).toEqual(['review', 'countsByType']);
      expect(reviewKeys.platformSummary('job-1')).toEqual(['review', 'platformSummary', 'job-1']);
    });
  });

  describe('useReviewItems', () => {
    it('should fetch review items', async () => {
      const mockData = {
        items: [
          {
            id: 'item-1',
            jobId: 'job-1',
            userId: 'user-1',
            status: ReviewItemStatus.PENDING,
            sourceTitle: 'Test Game',
            sourceMetadata: {},
            igdbCandidates: [],
            resolvedIgdbId: null,
            matchConfidence: null,
            createdAt: '2024-01-01T00:00:00Z',
            resolvedAt: null,
            jobType: 'import',
            jobSource: 'darkadia',
          },
        ],
        total: 1,
        page: 1,
        perPage: 20,
        pages: 1,
      };

      vi.mocked(reviewApi.getReviewItems).mockResolvedValueOnce(mockData);

      const { result } = renderHook(() => useReviewItems(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockData);
      expect(reviewApi.getReviewItems).toHaveBeenCalledWith(undefined, 1, 20);
    });

    it('should apply filters and pagination', async () => {
      const mockData = {
        items: [],
        total: 0,
        page: 2,
        perPage: 10,
        pages: 0,
      };

      vi.mocked(reviewApi.getReviewItems).mockResolvedValueOnce(mockData);

      const { result } = renderHook(
        () => useReviewItems({ status: 'pending' as never }, 2, 10),
        { wrapper }
      );

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(reviewApi.getReviewItems).toHaveBeenCalledWith({ status: 'pending' }, 2, 10);
    });
  });

  describe('useReviewItem', () => {
    it('should fetch a single review item', async () => {
      const mockItem = {
        id: 'item-1',
        jobId: 'job-1',
        userId: 'user-1',
        status: ReviewItemStatus.PENDING,
        sourceTitle: 'Test Game',
        sourceMetadata: {},
        igdbCandidates: [],
        resolvedIgdbId: null,
        matchConfidence: null,
        createdAt: '2024-01-01T00:00:00Z',
        resolvedAt: null,
        jobType: 'import',
        jobSource: 'darkadia',
      };

      vi.mocked(reviewApi.getReviewItem).mockResolvedValueOnce(mockItem);

      const { result } = renderHook(() => useReviewItem('item-1'), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockItem);
      expect(reviewApi.getReviewItem).toHaveBeenCalledWith('item-1');
    });

    it('should not fetch when itemId is empty', async () => {
      const { result } = renderHook(() => useReviewItem(''), { wrapper });

      expect(result.current.isLoading).toBe(false);
      expect(result.current.fetchStatus).toBe('idle');
      expect(reviewApi.getReviewItem).not.toHaveBeenCalled();
    });
  });

  describe('useReviewSummary', () => {
    it('should fetch review summary', async () => {
      const mockSummary = {
        totalPending: 10,
        totalMatched: 5,
        totalSkipped: 2,
        totalRemoval: 1,
        jobsWithPending: 3,
      };

      vi.mocked(reviewApi.getReviewSummary).mockResolvedValueOnce(mockSummary);

      const { result } = renderHook(() => useReviewSummary(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockSummary);
    });
  });

  describe('useReviewCountsByType', () => {
    it('should fetch review counts by type', async () => {
      const mockCounts = {
        importPending: 5,
        syncPending: 3,
      };

      vi.mocked(reviewApi.getReviewCountsByType).mockResolvedValueOnce(mockCounts);

      const { result } = renderHook(() => useReviewCountsByType(), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockCounts);
    });
  });

  describe('usePlatformSummary', () => {
    it('should fetch platform summary', async () => {
      const mockSummary = {
        platforms: [{ original: 'PC', count: 10, suggestedId: 'pc-windows', suggestedName: 'PC (Windows)' }],
        storefronts: [],
        allResolved: true,
      };

      vi.mocked(reviewApi.getPlatformSummary).mockResolvedValueOnce(mockSummary);

      const { result } = renderHook(() => usePlatformSummary('job-1'), { wrapper });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockSummary);
      expect(reviewApi.getPlatformSummary).toHaveBeenCalledWith('job-1');
    });

    it('should not fetch when jobId is null', async () => {
      const { result } = renderHook(() => usePlatformSummary(null), { wrapper });

      expect(result.current.isLoading).toBe(false);
      expect(result.current.fetchStatus).toBe('idle');
      expect(reviewApi.getPlatformSummary).not.toHaveBeenCalled();
    });
  });

  describe('useMatchReviewItem', () => {
    it('should match a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Item matched',
        item: { id: 'item-1', status: 'matched' },
      };

      vi.mocked(reviewApi.matchReviewItem).mockResolvedValueOnce(mockResponse as never);

      const { result } = renderHook(() => useMatchReviewItem(), { wrapper });

      await result.current.mutateAsync({ itemId: 'item-1', igdbId: 12345 });

      expect(reviewApi.matchReviewItem).toHaveBeenCalledWith('item-1', 12345);
    });
  });

  describe('useSkipReviewItem', () => {
    it('should skip a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Item skipped',
        item: { id: 'item-1', status: 'skipped' },
      };

      vi.mocked(reviewApi.skipReviewItem).mockResolvedValueOnce(mockResponse as never);

      const { result } = renderHook(() => useSkipReviewItem(), { wrapper });

      await result.current.mutateAsync('item-1');

      expect(reviewApi.skipReviewItem).toHaveBeenCalledWith('item-1');
    });
  });

  describe('useKeepReviewItem', () => {
    it('should keep a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Item kept',
        item: { id: 'item-1', status: 'matched' },
      };

      vi.mocked(reviewApi.keepReviewItem).mockResolvedValueOnce(mockResponse as never);

      const { result } = renderHook(() => useKeepReviewItem(), { wrapper });

      await result.current.mutateAsync('item-1');

      expect(reviewApi.keepReviewItem).toHaveBeenCalledWith('item-1');
    });
  });

  describe('useRemoveReviewItem', () => {
    it('should remove a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Item removed',
        item: { id: 'item-1', status: 'removal' },
      };

      vi.mocked(reviewApi.removeReviewItem).mockResolvedValueOnce(mockResponse as never);

      const { result } = renderHook(() => useRemoveReviewItem(), { wrapper });

      await result.current.mutateAsync('item-1');

      expect(reviewApi.removeReviewItem).toHaveBeenCalledWith('item-1');
    });
  });

  describe('useFinalizeImport', () => {
    it('should finalize an import', async () => {
      const mockResponse = {
        success: true,
        message: 'Import finalized',
        gamesCreated: 5,
        gamesSkipped: 2,
        gamesFailed: 0,
        errors: [],
      };

      vi.mocked(reviewApi.finalizeImport).mockResolvedValueOnce(mockResponse);

      const { result } = renderHook(() => useFinalizeImport(), { wrapper });

      await result.current.mutateAsync({
        jobId: 'job-1',
        platformMappings: { PC: 'pc-windows' },
        storefrontMappings: { Steam: 'steam' },
      });

      expect(reviewApi.finalizeImport).toHaveBeenCalledWith(
        'job-1',
        { PC: 'pc-windows' },
        { Steam: 'steam' }
      );
    });
  });
});
