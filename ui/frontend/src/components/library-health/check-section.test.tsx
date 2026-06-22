import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { Accordion } from '@/components/ui/accordion';
import { CheckSection } from './check-section';
import type { SmellSummaryItem } from '@/api/library-health';

vi.mock('@/hooks', () => ({
  useSmellItems: vi.fn(),
  useApplySmell: vi.fn(),
  useApplyAllSmell: vi.fn(),
  useIgnoreSmell: vi.fn(),
  useIgnoredItems: vi.fn(() => ({
    data: { items: [], total: 0, page: 1, per_page: 200, pages: 0 },
    isLoading: false,
  })),
  useRestoreSmell: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
}));

const autoCheck: SmellSummaryItem = {
  id: 'wishlisted-yet-owned',
  title: 'Wishlisted yet owned',
  description: 'Still on your wishlist even though it is already in your library.',
  tier: 'inconsistency',
  auto_fixable: true,
  count: 2,
};

const cleanCheck: SmellSummaryItem = {
  ...autoCheck,
  id: 'orphan-game',
  title: 'Orphan game',
  auto_fixable: false,
  count: 0,
};

const mkMutation = (over = {}) => ({
  mutateAsync: vi.fn().mockResolvedValue({ applied: 2, skipped: 0 }),
  isPending: false,
  ...over,
});

function renderInAccordion(check: SmellSummaryItem) {
  return render(
    <Accordion type="multiple">
      <CheckSection check={check} onOpenGame={vi.fn()} />
    </Accordion>,
  );
}

describe('CheckSection', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useSmellItems.mockReturnValue({
      data: {
        items: [
          { user_game_id: 'ug-1', game_id: 1, title: 'A' },
          { user_game_id: 'ug-2', game_id: 2, title: 'B' },
        ],
        total: 2,
        page: 1,
        per_page: 200,
        pages: 1,
      },
      isLoading: false,
    } as unknown as ReturnType<typeof hooks.useSmellItems>);
    hooks.useApplySmell.mockReturnValue(
      mkMutation() as unknown as ReturnType<typeof hooks.useApplySmell>,
    );
    hooks.useApplyAllSmell.mockReturnValue(
      mkMutation() as unknown as ReturnType<typeof hooks.useApplyAllSmell>,
    );
    hooks.useIgnoreSmell.mockReturnValue(
      mkMutation({
        mutateAsync: vi.fn().mockResolvedValue({ ignored: 1 }),
      }) as unknown as ReturnType<typeof hooks.useIgnoreSmell>,
    );
  });

  it('renders a zero-count check as a non-expandable "All clear" row', () => {
    renderInAccordion(cleanCheck);
    expect(screen.getByText('Orphan game')).toBeInTheDocument();
    expect(screen.getByText(/all clear/i)).toBeInTheDocument();
    // No expand trigger for a clean check.
    expect(screen.queryByRole('button', { name: /orphan game/i })).not.toBeInTheDocument();
  });

  it('shows title, count and an Auto-fix badge for an auto-fixable check', () => {
    renderInAccordion(autoCheck);
    expect(screen.getByText('Wishlisted yet owned')).toBeInTheDocument();
    expect(screen.getByText(/auto-fix/i)).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
  });

  it('opens a confirm dialog for "Apply to all" and fires applyAll on confirm', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn().mockResolvedValue({ applied: 2, skipped: 0 });
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useApplyAllSmell.mockReturnValue(
      mkMutation({ mutateAsync }) as unknown as ReturnType<typeof hooks.useApplyAllSmell>,
    );

    renderInAccordion(autoCheck);
    await user.click(screen.getByRole('button', { name: /wishlisted yet owned/i })); // expand
    await user.click(await screen.findByRole('button', { name: /apply to all/i }));
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^apply$/i }));
    await waitFor(() =>
      expect(mutateAsync).toHaveBeenCalledWith({ checkID: 'wishlisted-yet-owned' }),
    );
  });

  it('does not fire applyAll when the confirm dialog is cancelled', async () => {
    const user = userEvent.setup();
    const mutateAsync = vi.fn();
    const hooks = vi.mocked(await import('@/hooks'));
    hooks.useApplyAllSmell.mockReturnValue(
      mkMutation({ mutateAsync }) as unknown as ReturnType<typeof hooks.useApplyAllSmell>,
    );

    renderInAccordion(autoCheck);
    await user.click(screen.getByRole('button', { name: /wishlisted yet owned/i }));
    await user.click(await screen.findByRole('button', { name: /apply to all/i }));
    await user.click(await screen.findByRole('button', { name: /cancel/i }));
    expect(mutateAsync).not.toHaveBeenCalled();
  });
});
