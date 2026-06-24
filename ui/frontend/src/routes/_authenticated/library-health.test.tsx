import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { SmellSummaryItem } from '@/api/library-health';
import { sessionStorageMock } from '@/test/setup';
import { LibraryHealthPage } from './library-health';

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => () => ({}),
  useNavigate: () => vi.fn(),
}));

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

// Two flagged checks in different tiers — exercises the cross-tier merge in the
// page's expanded-set handling (check IDs are globally unique).
const checks: SmellSummaryItem[] = [
  {
    id: 'wishlisted-yet-owned',
    title: 'Wishlisted yet owned',
    description: 'desc-a',
    tier: 'inconsistency',
    auto_fixable: true,
    count: 1,
  },
  {
    id: 'played-but-not-started',
    title: 'Played but not started',
    description: 'desc-b',
    tier: 'nudge',
    auto_fixable: true,
    count: 1,
  },
];

vi.mock('@/hooks', () => ({
  smellKeys: { all: ['smells'] },
  useSmellSummary: () => ({
    data: checks,
    isLoading: false,
    isFetching: false,
    isError: false,
    error: null,
  }),
  // Children (CheckSection) hooks — items keyed off whichever check is expanded.
  useSmellItems: (checkID: string, _page: number, enabled: boolean) => ({
    data: enabled
      ? {
          items: [{ user_game_id: `ug-${checkID}`, game_id: 1, title: `Game ${checkID}` }],
          total: 1,
          page: 1,
          per_page: 200,
          pages: 1,
        }
      : undefined,
    isLoading: false,
    isFetching: false,
  }),
  useApplySmell: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useApplyAllSmell: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useIgnoreSmell: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useIgnoredItems: () => ({
    data: { items: [], total: 0, page: 1, per_page: 200, pages: 0 },
    isLoading: false,
  }),
  useRestoreSmell: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

describe('LibraryHealthPage expansion persistence', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('seeds the expanded checks from sessionStorage and shows their items without a click', async () => {
    sessionStorageMock.getItem.mockReturnValue(JSON.stringify(['wishlisted-yet-owned']));
    render(<LibraryHealthPage />);
    // The seeded check's flagged item is fetched and rendered immediately.
    expect(await screen.findByText('Game wishlisted-yet-owned')).toBeInTheDocument();
    // The non-seeded check's item is not.
    expect(screen.queryByText('Game played-but-not-started')).not.toBeInTheDocument();
  });

  it('persists a newly expanded check to sessionStorage', async () => {
    sessionStorageMock.getItem.mockReturnValue(null);
    const user = userEvent.setup();
    render(<LibraryHealthPage />);
    await user.click(screen.getByRole('button', { name: /played but not started/i }));
    await waitFor(() =>
      expect(sessionStorageMock.setItem).toHaveBeenCalledWith(
        'nexorious:library-health-expanded:v1',
        JSON.stringify(['played-but-not-started']),
      ),
    );
  });
});
