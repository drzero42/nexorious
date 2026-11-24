import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { DismissedItems } from './dismissed-items';

vi.mock('@/hooks', () => ({
  useIgnoredItems: vi.fn(),
  useRestoreSmell: vi.fn(),
}));

const mkMutation = (over = {}) => ({
  mutateAsync: vi.fn().mockResolvedValue({}),
  isPending: false,
  ...over,
});

describe('DismissedItems', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const { useIgnoredItems, useRestoreSmell } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockReturnValue({
      data: {
        items: [{ user_game_id: 'ug-1', title: 'Hades', created_at: '2026-06-01' }],
        total: 1,
        page: 1,
        per_page: 200,
        pages: 1,
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useIgnoredItems>);
    useRestoreSmell.mockReturnValue(mkMutation() as unknown as ReturnType<typeof useRestoreSmell>);
  });

  it('lists dismissed items', () => {
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.getByText('Hades')).toBeInTheDocument();
  });

  it('calls restore with the check id and game id', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn().mockResolvedValue({ restored: 1 });
    const { useRestoreSmell } = vi.mocked(await import('@/hooks'));
    useRestoreSmell.mockReturnValue(
      mkMutation({ mutateAsync }) as unknown as ReturnType<typeof useRestoreSmell>,
    );
    render(<DismissedItems checkID="orphan-game" />);
    await user.click(screen.getByRole('button', { name: /restore/i }));
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({ checkID: 'orphan-game', userGameIds: ['ug-1'] }),
    );
  });

  it('shows an empty message when there are no dismissed items', async () => {
    const { useIgnoredItems } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockReturnValue({
      data: { items: [], total: 0, page: 1, per_page: 200, pages: 0 },
      isLoading: false,
    } as unknown as ReturnType<typeof useIgnoredItems>);
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.getByText(/no dismissed items/i)).toBeInTheDocument();
  });

  const mkPage = (
    page: number,
    pages: number,
    items = [{ user_game_id: `ug-p${page}`, title: `Game ${page}`, created_at: '2026-06-01' }],
  ) => ({
    data: { items, total: pages * 25, page, per_page: 25, pages },
    isLoading: false,
    isFetching: false,
  });

  it('hides pagination controls for a single page', () => {
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.queryByRole('button', { name: /next/i })).not.toBeInTheDocument();
  });

  it('pages through dismissed items beyond the first page', async () => {
    const user = userEvent.setup();
    const { useIgnoredItems } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockImplementation(
      (_checkID: string, page: number) =>
        mkPage(page, 3) as unknown as ReturnType<typeof useIgnoredItems>,
    );
    render(<DismissedItems checkID="orphan-game" />);
    expect(screen.getByText('Page 1 of 3')).toBeInTheDocument();
    expect(screen.getByText('75 dismissed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /previous/i })).toBeDisabled();

    await user.click(screen.getByRole('button', { name: /next/i }));
    expect(useIgnoredItems).toHaveBeenLastCalledWith('orphan-game', 2, true);
    expect(screen.getByText('Page 2 of 3')).toBeInTheDocument();
    expect(screen.getByText('Game 2')).toBeInTheDocument();
  });

  it('steps back a page after restoring the last item on a later page', async () => {
    const user = userEvent.setup();
    const { useIgnoredItems } = vi.mocked(await import('@/hooks'));
    useIgnoredItems.mockImplementation(
      (_checkID: string, page: number) =>
        mkPage(page, 2) as unknown as ReturnType<typeof useIgnoredItems>,
    );
    render(<DismissedItems checkID="orphan-game" />);
    await user.click(screen.getByRole('button', { name: /next/i }));
    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /restore/i }));
    await waitFor(() => expect(screen.getByText('Page 1 of 2')).toBeInTheDocument());
    expect(useIgnoredItems).toHaveBeenLastCalledWith('orphan-game', 1, true);
  });
});
