import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useNavItems } from './nav-items';

const mockReview = vi.fn();
vi.mock('@/hooks/use-jobs', () => ({
  usePendingReviewCount: () => mockReview(),
}));

describe('useNavItems review badges', () => {
  beforeEach(() => vi.clearAllMocks());

  it('Sync badge excludes Darkadia import; Import badge shows the Darkadia count', () => {
    mockReview.mockReturnValue({
      data: { pendingReviewCount: 30, countsBySource: { steam: 4, psn: 1, darkadia: 25 } },
    });
    const { result } = renderHook(() => useNavItems());
    const sync = result.current.mainItems.find((i) => i.href === '/sync');
    const imp = result.current.mainItems.find((i) => i.href === '/import-export');
    expect(sync?.badge).toBe(5); // 30 total - 25 darkadia
    expect(imp?.badge).toBe(25);
  });

  it('no Import badge when there are no Darkadia reviews; Sync unaffected', () => {
    mockReview.mockReturnValue({ data: { pendingReviewCount: 3, countsBySource: { steam: 3 } } });
    const { result } = renderHook(() => useNavItems());
    const sync = result.current.mainItems.find((i) => i.href === '/sync');
    const imp = result.current.mainItems.find((i) => i.href === '/import-export');
    expect(sync?.badge).toBe(3);
    expect(imp?.badge).toBe(0);
  });
});
