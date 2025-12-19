import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  getReviewItems,
  getReviewItem,
  getReviewSummary,
  getReviewCountsByType,
  matchReviewItem,
  skipReviewItem,
  keepReviewItem,
  removeReviewItem,
  getPlatformSummary,
  finalizeImport,
} from './review';
import { api } from './client';

// Mock the api client
vi.mock('./client', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

describe('Review API', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  describe('getReviewItems', () => {
    it('should fetch review items with default pagination', async () => {
      const mockResponse = {
        items: [
          {
            id: 'item-1',
            job_id: 'job-1',
            user_id: 'user-1',
            status: 'pending',
            source_title: 'Test Game',
            source_metadata: {},
            igdb_candidates: [],
            resolved_igdb_id: null,
            match_confidence: null,
            created_at: '2024-01-01T00:00:00Z',
            resolved_at: null,
            job_type: 'import',
            job_source: 'darkadia',
          },
        ],
        total: 1,
        page: 1,
        per_page: 20,
        pages: 1,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getReviewItems();

      expect(api.get).toHaveBeenCalledWith('/review/', {
        params: { page: 1, per_page: 20 },
      });
      expect(result.items).toHaveLength(1);
      expect(result.items[0].id).toBe('item-1');
      expect(result.items[0].sourceTitle).toBe('Test Game');
    });

    it('should apply filters to the request', async () => {
      const mockResponse = {
        items: [],
        total: 0,
        page: 1,
        per_page: 20,
        pages: 0,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      await getReviewItems({ status: 'pending' as never, jobId: 'job-1', source: 'import' as never }, 2, 10);

      expect(api.get).toHaveBeenCalledWith('/review/', {
        params: { page: 2, per_page: 10, status: 'pending', job_id: 'job-1', source: 'import' },
      });
    });
  });

  describe('getReviewItem', () => {
    it('should fetch a single review item by ID', async () => {
      const mockResponse = {
        id: 'item-1',
        job_id: 'job-1',
        user_id: 'user-1',
        status: 'pending',
        source_title: 'Test Game',
        source_metadata: {},
        igdb_candidates: [
          {
            igdb_id: 12345,
            name: 'Test Game Match',
            first_release_date: 1609459200,
            cover_url: 'https://example.com/cover.jpg',
            summary: 'A test game',
            platforms: ['PC', 'PlayStation 5'],
            similarity_score: 0.95,
          },
        ],
        resolved_igdb_id: null,
        match_confidence: null,
        created_at: '2024-01-01T00:00:00Z',
        resolved_at: null,
        job_type: 'import',
        job_source: 'darkadia',
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getReviewItem('item-1');

      expect(api.get).toHaveBeenCalledWith('/review/item-1');
      expect(result.id).toBe('item-1');
      expect(result.igdbCandidates).toHaveLength(1);
      expect(result.igdbCandidates[0].igdbId).toBe(12345);
    });
  });

  describe('getReviewSummary', () => {
    it('should fetch review summary statistics', async () => {
      const mockResponse = {
        total_pending: 10,
        total_matched: 5,
        total_skipped: 2,
        total_removal: 1,
        jobs_with_pending: 3,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getReviewSummary();

      expect(api.get).toHaveBeenCalledWith('/review/summary');
      expect(result.totalPending).toBe(10);
      expect(result.totalMatched).toBe(5);
      expect(result.jobsWithPending).toBe(3);
    });
  });

  describe('getReviewCountsByType', () => {
    it('should fetch review counts by type', async () => {
      const mockResponse = {
        import_pending: 5,
        sync_pending: 3,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getReviewCountsByType();

      expect(api.get).toHaveBeenCalledWith('/review/counts');
      expect(result.importPending).toBe(5);
      expect(result.syncPending).toBe(3);
    });
  });

  describe('matchReviewItem', () => {
    it('should match a review item to an IGDB ID', async () => {
      const mockResponse = {
        success: true,
        message: 'Item matched successfully',
        item: {
          id: 'item-1',
          job_id: 'job-1',
          user_id: 'user-1',
          status: 'matched',
          source_title: 'Test Game',
          source_metadata: {},
          igdb_candidates: [],
          resolved_igdb_id: 12345,
          match_confidence: 0.95,
          created_at: '2024-01-01T00:00:00Z',
          resolved_at: '2024-01-02T00:00:00Z',
          job_type: 'import',
          job_source: 'darkadia',
        },
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await matchReviewItem('item-1', 12345);

      expect(api.post).toHaveBeenCalledWith('/review/item-1/match', { igdb_id: 12345 });
      expect(result.success).toBe(true);
      expect(result.item?.resolvedIgdbId).toBe(12345);
    });
  });

  describe('skipReviewItem', () => {
    it('should skip a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Item skipped',
        item: {
          id: 'item-1',
          job_id: 'job-1',
          user_id: 'user-1',
          status: 'skipped',
          source_title: 'Test Game',
          source_metadata: {},
          igdb_candidates: [],
          resolved_igdb_id: null,
          match_confidence: null,
          created_at: '2024-01-01T00:00:00Z',
          resolved_at: '2024-01-02T00:00:00Z',
          job_type: 'import',
          job_source: 'darkadia',
        },
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await skipReviewItem('item-1');

      expect(api.post).toHaveBeenCalledWith('/review/item-1/skip');
      expect(result.success).toBe(true);
      expect(result.item?.status).toBe('skipped');
    });
  });

  describe('keepReviewItem', () => {
    it('should keep a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Game kept in collection',
        item: {
          id: 'item-1',
          job_id: 'job-1',
          user_id: 'user-1',
          status: 'matched',
          source_title: 'Test Game',
          source_metadata: { removal_detected: true },
          igdb_candidates: [],
          resolved_igdb_id: null,
          match_confidence: null,
          created_at: '2024-01-01T00:00:00Z',
          resolved_at: '2024-01-02T00:00:00Z',
          job_type: 'sync',
          job_source: 'steam',
        },
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await keepReviewItem('item-1');

      expect(api.post).toHaveBeenCalledWith('/review/item-1/keep');
      expect(result.success).toBe(true);
    });
  });

  describe('removeReviewItem', () => {
    it('should remove a review item', async () => {
      const mockResponse = {
        success: true,
        message: 'Game marked for removal',
        item: {
          id: 'item-1',
          job_id: 'job-1',
          user_id: 'user-1',
          status: 'removal',
          source_title: 'Test Game',
          source_metadata: { removal_detected: true },
          igdb_candidates: [],
          resolved_igdb_id: null,
          match_confidence: null,
          created_at: '2024-01-01T00:00:00Z',
          resolved_at: '2024-01-02T00:00:00Z',
          job_type: 'sync',
          job_source: 'steam',
        },
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await removeReviewItem('item-1');

      expect(api.post).toHaveBeenCalledWith('/review/item-1/remove');
      expect(result.success).toBe(true);
      expect(result.item?.status).toBe('removal');
    });
  });

  describe('getPlatformSummary', () => {
    it('should fetch platform summary for a job', async () => {
      const mockResponse = {
        platforms: [
          {
            original: 'PC',
            count: 10,
            suggested_id: 'pc-windows',
            suggested_name: 'PC (Windows)',
          },
        ],
        storefronts: [
          {
            original: 'Steam',
            count: 8,
            suggested_id: 'steam',
            suggested_name: 'Steam',
          },
        ],
        all_resolved: true,
      };

      vi.mocked(api.get).mockResolvedValueOnce(mockResponse);

      const result = await getPlatformSummary('job-1');

      expect(api.get).toHaveBeenCalledWith('/review/platform-summary', {
        params: { job_id: 'job-1' },
      });
      expect(result.platforms).toHaveLength(1);
      expect(result.platforms[0].suggestedId).toBe('pc-windows');
      expect(result.storefronts).toHaveLength(1);
      expect(result.allResolved).toBe(true);
    });
  });

  describe('finalizeImport', () => {
    it('should finalize an import with mappings', async () => {
      const mockResponse = {
        success: true,
        message: 'Import finalized successfully',
        games_created: 5,
        games_skipped: 2,
        games_failed: 0,
        errors: [],
      };

      vi.mocked(api.post).mockResolvedValueOnce(mockResponse);

      const result = await finalizeImport(
        'job-1',
        { PC: 'pc-windows' },
        { Steam: 'steam' }
      );

      expect(api.post).toHaveBeenCalledWith('/review/finalize', {
        job_id: 'job-1',
        platform_mappings: [{ original: 'PC', resolved_id: 'pc-windows' }],
        storefront_mappings: [{ original: 'Steam', resolved_id: 'steam' }],
      });
      expect(result.success).toBe(true);
      expect(result.gamesCreated).toBe(5);
      expect(result.gamesSkipped).toBe(2);
    });
  });
});
