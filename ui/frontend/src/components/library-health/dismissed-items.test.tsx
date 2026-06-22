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
});
