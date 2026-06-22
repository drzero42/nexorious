import { describe, it, expect, vi, beforeEach } from 'vitest';
import { api } from './client';
import {
  applySmell,
  fetchAllFlaggedIds,
  applyAllSmell,
  getSmellSummary,
  getSmellItems,
} from './library-health';
import type { FlaggedListResponse } from './library-health';

vi.mock('./client', () => ({
  api: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
}));

const mockApi = vi.mocked(api);

function page(ids: string[], pageNo: number, pages: number): FlaggedListResponse {
  return {
    items: ids.map((id) => ({ user_game_id: id, game_id: 1, title: id })),
    total: pages * 200,
    page: pageNo,
    per_page: 200,
    pages,
  };
}

describe('request paths', () => {
  beforeEach(() => vi.clearAllMocks());

  // Regression: the shared `api` client prepends config.apiUrl ('/api'), so a
  // leading '/api' in BASE doubled to '/api/api/library/smells' (a 404).
  it('targets /library/smells with no doubled /api prefix', async () => {
    mockApi.get.mockResolvedValue([]);
    await getSmellSummary();
    expect(mockApi.get).toHaveBeenCalledWith('/library/smells');

    mockApi.get.mockResolvedValue(page([], 1, 0));
    await getSmellItems('orphan-game');
    expect(mockApi.get).toHaveBeenCalledWith('/library/smells/orphan-game', {
      params: { page: 1, per_page: 200 },
    });
  });
});

describe('applySmell', () => {
  beforeEach(() => vi.clearAllMocks());

  it('sends a single request when ids fit under the cap and sums the result', async () => {
    mockApi.post.mockResolvedValue({ applied: 3, skipped: 1 });
    const res = await applySmell('wishlisted-yet-owned', ['a', 'b', 'c', 'd']);
    expect(mockApi.post).toHaveBeenCalledTimes(1);
    expect(mockApi.post).toHaveBeenCalledWith('/library/smells/wishlisted-yet-owned/apply', {
      user_game_ids: ['a', 'b', 'c', 'd'],
    });
    expect(res).toEqual({ applied: 3, skipped: 1 });
  });

  it('chunks ids into groups of 200 and aggregates applied/skipped', async () => {
    const ids = Array.from({ length: 450 }, (_, i) => `g${i}`);
    mockApi.post
      .mockResolvedValueOnce({ applied: 200, skipped: 0 })
      .mockResolvedValueOnce({ applied: 200, skipped: 0 })
      .mockResolvedValueOnce({ applied: 40, skipped: 10 });
    const res = await applySmell('beat-but-not-marked', ids);
    expect(mockApi.post).toHaveBeenCalledTimes(3);
    expect(
      (mockApi.post.mock.calls[0][1] as { user_game_ids: string[] }).user_game_ids,
    ).toHaveLength(200);
    expect(
      (mockApi.post.mock.calls[2][1] as { user_game_ids: string[] }).user_game_ids,
    ).toHaveLength(50);
    expect(res).toEqual({ applied: 440, skipped: 10 });
  });
});

describe('fetchAllFlaggedIds', () => {
  beforeEach(() => vi.clearAllMocks());

  it('walks every page and returns all ids', async () => {
    mockApi.get
      .mockResolvedValueOnce(page(['a', 'b'], 1, 2))
      .mockResolvedValueOnce(page(['c', 'd'], 2, 2));
    const ids = await fetchAllFlaggedIds('orphan-game');
    expect(ids).toEqual(['a', 'b', 'c', 'd']);
    expect(mockApi.get).toHaveBeenCalledTimes(2);
  });

  it('stops after a single page when pages=1', async () => {
    mockApi.get.mockResolvedValueOnce(page(['a'], 1, 1));
    const ids = await fetchAllFlaggedIds('orphan-game');
    expect(ids).toEqual(['a']);
    expect(mockApi.get).toHaveBeenCalledTimes(1);
  });
});

describe('applyAllSmell', () => {
  beforeEach(() => vi.clearAllMocks());

  it('returns zero without calling apply when there are no flagged ids', async () => {
    mockApi.get.mockResolvedValueOnce(page([], 1, 0));
    const res = await applyAllSmell('wishlisted-yet-owned');
    expect(res).toEqual({ applied: 0, skipped: 0 });
    expect(mockApi.post).not.toHaveBeenCalled();
  });
});
