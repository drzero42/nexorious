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
    onView: vi.fn(),
    onEdit: vi.fn(),
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

  it('renders an Edit button (not Apply/Fix) for manual checks and fires onEdit', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: false });
    expect(screen.queryByRole('button', { name: /^apply$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^fix$/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^edit$/i }));
    expect(props.onEdit).toHaveBeenCalledWith('ug-1');
  });

  it('the game title opens the details view (onView), not edit', async () => {
    const user = userEvent.setup();
    const props = renderTable({ autoFixable: false });
    await user.click(screen.getByRole('button', { name: 'Celeste' }));
    expect(props.onView).toHaveBeenCalledWith('ug-1');
    expect(props.onEdit).not.toHaveBeenCalled();
  });

  it('fires onIgnore with the id', async () => {
    const user = userEvent.setup();
    const props = renderTable();
    await user.click(screen.getByRole('button', { name: /ignore/i }));
    expect(props.onIgnore).toHaveBeenCalledWith('ug-1');
  });

  it('shows the detail text when present', () => {
    renderTable({ items: [{ ...item, detail: 'acquired 2031-04-01 (future)' }] });
    expect(screen.getByText('acquired 2031-04-01 (future)')).toBeInTheDocument();
  });
});
