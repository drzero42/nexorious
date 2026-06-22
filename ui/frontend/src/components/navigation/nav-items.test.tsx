import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useNavItems } from './nav-items';

const mockReview = vi.fn();
const mockImportSources = vi.fn();
const mockSmellSummary = vi.fn();
vi.mock('@/hooks/use-jobs', () => ({
  usePendingReviewCount: () => mockReview(),
}));
vi.mock('@/hooks', () => ({
  useImportSources: () => mockImportSources(),
  useSmellSummary: () => mockSmellSummary(),
}));

describe('useNavItems review badges', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSmellSummary.mockReturnValue({ data: [] });
  });

  it('Sync badge excludes import-source reviews; Import badge shows the import count', () => {
    mockReview.mockReturnValue({
      data: { pendingReviewCount: 30, countsBySource: { steam: 4, psn: 1, vglist: 25 } },
    });
    mockImportSources.mockReturnValue({
      data: [
        {
          slug: 'vglist',
          display_name: 'vglist',
          description: '',
          features: [],
          accept: ['.json'],
        },
      ],
    });
    const { result } = renderHook(() => useNavItems());
    const sync = result.current.mainItems.find((i) => i.href === '/sync');
    const imp = result.current.mainItems.find((i) => i.href === '/import-export');
    expect(sync?.badge).toBe(5); // 30 total - 25 vglist
    expect(imp?.badge).toBe(25);
  });

  it('no Import badge when there are no import-source reviews; Sync unaffected', () => {
    mockReview.mockReturnValue({ data: { pendingReviewCount: 3, countsBySource: { steam: 3 } } });
    mockImportSources.mockReturnValue({ data: [] });
    const { result } = renderHook(() => useNavItems());
    const sync = result.current.mainItems.find((i) => i.href === '/sync');
    const imp = result.current.mainItems.find((i) => i.href === '/import-export');
    expect(sync?.badge).toBe(3);
    expect(imp?.badge).toBe(0);
  });

  it('Library Health badge sums only inconsistency-tier counts', () => {
    mockReview.mockReturnValue({ data: { pendingReviewCount: 0, countsBySource: {} } });
    mockImportSources.mockReturnValue({ data: [] });
    mockSmellSummary.mockReturnValue({
      data: [
        {
          id: 'orphan-game',
          title: 'x',
          description: '',
          tier: 'inconsistency',
          auto_fixable: false,
          count: 2,
        },
        {
          id: 'missing-ownership-status',
          title: 'x',
          description: '',
          tier: 'inconsistency',
          auto_fixable: false,
          count: 3,
        },
        {
          id: 'unrated-after-finishing',
          title: 'x',
          description: '',
          tier: 'nudge',
          auto_fixable: false,
          count: 5,
        },
      ],
    });
    const { result } = renderHook(() => useNavItems());
    const health = result.current.mainItems.find((i) => i.href === '/library-health');
    expect(health?.badge).toBe(5); // 2 + 3 inconsistency only; the nudge (5) is excluded
  });
});
