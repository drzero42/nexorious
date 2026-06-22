import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { FlaggedItemsTable } from './flagged-items-table';
import type { FlaggedItem } from '@/api/library-health';

const item: FlaggedItem = { user_game_id: 'ug-1', game_id: 10, title: 'Celeste' };

function renderTable(over: Partial<React.ComponentProps<typeof FlaggedItemsTable>> = {}) {
  const props = {
    items: [item],
    autoFixable: true,
    onApply: vi.fn(),
    onIgnore: vi.fn(),
    onOpenGame: vi.fn(),
    ...over,
  };
  render(<FlaggedItemsTable {...props} />);
  return props;
}

describe('FlaggedItemsTable', () => {
  it('renders an Apply button for auto-fixable checks and fires onApply with the id', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: true });
    await user.click(screen.getByRole('button', { name: /apply/i }));
    expect(props.onApply).toHaveBeenCalledWith('ug-1');
  });

  it('renders a Fix button (not Apply) for manual checks and fires onOpenGame', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: false });
    expect(screen.queryByRole('button', { name: /^apply$/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /fix/i }));
    expect(props.onOpenGame).toHaveBeenCalledWith('ug-1');
  });

  it('fires onIgnore with the id', async () => {
    const user = userEvent.setup();
    const props = renderTable();
    await user.click(screen.getByRole('button', { name: /ignore/i }));
    expect(props.onIgnore).toHaveBeenCalledWith('ug-1');
  });

  it('shows the suggested storefront when present', () => {
    renderTable({
      autoFixable: false,
      items: [{ ...item, suggested_storefront: 'Steam' }],
    });
    expect(screen.getByText(/suggested:\s*steam/i)).toBeInTheDocument();
  });

  it('shows the detail text when present', () => {
    renderTable({ items: [{ ...item, detail: 'acquired 2031-04-01 (future)' }] });
    expect(screen.getByText('acquired 2031-04-01 (future)')).toBeInTheDocument();
  });
});
