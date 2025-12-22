import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { BulkActions, type BulkActionsProps } from './bulk-actions';
import { PlayStatus } from '@/types';

// Mock the hooks module
vi.mock('@/hooks', () => ({
  useBulkUpdateUserGames: vi.fn(),
  useBulkDeleteUserGames: vi.fn(),
}));


// Mock lucide-react icons
vi.mock('lucide-react', () => ({
  Trash2: ({ className }: { className?: string }) => (
    <div data-testid="trash2-icon" className={className}>
      Trash2
    </div>
  ),
  X: ({ className }: { className?: string }) => (
    <div data-testid="x-icon" className={className}>
      X
    </div>
  ),
  ChevronDown: ({ className }: { className?: string }) => (
    <div data-testid="chevron-down-icon" className={className}>
      ▼
    </div>
  ),
  ChevronUp: ({ className }: { className?: string }) => (
    <div data-testid="chevron-up-icon" className={className}>
      ▲
    </div>
  ),
  Check: ({ className }: { className?: string }) => (
    <div data-testid="check-icon" className={className}>
      ✓
    </div>
  ),
}));

const createDefaultProps = (overrides: Partial<BulkActionsProps> = {}): BulkActionsProps => ({
  selectedIds: new Set(['game-1', 'game-2']),
  onClearSelection: vi.fn(),
  onSuccess: vi.fn(),
  selectionMode: 'manual',
  visibleCount: 50,
  totalCount: 100,
  onSelectAllClick: vi.fn(),
  ...overrides,
});

interface MockMutation {
  mutateAsync: ReturnType<typeof vi.fn>;
  isPending: boolean;
  isError: boolean;
  isSuccess: boolean;
  error: Error | null;
}

const createMockMutation = (overrides: Partial<MockMutation> = {}): MockMutation => ({
  mutateAsync: vi.fn().mockResolvedValue({}),
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null,
  ...overrides,
});

describe('BulkActions', () => {
  beforeEach(async () => {
    vi.clearAllMocks();

    // Default mock implementations
    const { useBulkUpdateUserGames, useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
    useBulkUpdateUserGames.mockReturnValue(createMockMutation() as unknown as ReturnType<typeof useBulkUpdateUserGames>);
    useBulkDeleteUserGames.mockReturnValue(createMockMutation() as unknown as ReturnType<typeof useBulkDeleteUserGames>);
  });

  describe('rendering', () => {
    it('renders select all checkbox when no games are selected but games exist', () => {
      const props = createDefaultProps({ selectedIds: new Set() });
      render(<BulkActions {...props} />);

      // Should render the select all checkbox
      expect(screen.getByRole('checkbox')).toBeInTheDocument();
    });

    it('renders selected count with singular form for one game', () => {
      const props = createDefaultProps({ selectedIds: new Set(['game-1']) });
      render(<BulkActions {...props} />);

      expect(screen.getByText('1 game selected')).toBeInTheDocument();
    });

    it('renders selected count with plural form for multiple games', () => {
      const props = createDefaultProps({ selectedIds: new Set(['game-1', 'game-2']) });
      render(<BulkActions {...props} />);

      expect(screen.getByText('2 games selected')).toBeInTheDocument();
    });

    it('renders selected count with plural form for three games', () => {
      const props = createDefaultProps({ selectedIds: new Set(['game-1', 'game-2', 'game-3']) });
      render(<BulkActions {...props} />);

      expect(screen.getByText('3 games selected')).toBeInTheDocument();
    });

    it('renders status change select', () => {
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('combobox')).toBeInTheDocument();
    });

    it('renders delete button', () => {
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('button', { name: /delete/i })).toBeInTheDocument();
    });

    it('renders clear selection button', () => {
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });
  });

  describe('status change', () => {
    it('displays all PlayStatus options in dropdown', async () => {
      const user = userEvent.setup();
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      // Open the select dropdown
      await user.click(screen.getByRole('combobox'));

      // Check all status options are present
      await waitFor(() => {
        expect(screen.getByRole('option', { name: 'Not Started' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'In Progress' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Completed' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Mastered' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Dominated' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Shelved' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Dropped' })).toBeInTheDocument();
        expect(screen.getByRole('option', { name: 'Replay' })).toBeInTheDocument();
      });
    });

    it('calls bulkUpdate mutation with correct arguments when status is changed', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      // Open the select dropdown and choose a status
      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Completed' }));

      expect(mockMutateAsync).toHaveBeenCalledWith({
        ids: ['game-1', 'game-2'],
        updates: { playStatus: PlayStatus.COMPLETED },
      });
    });

    it('calls onClearSelection after successful status update', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const onClearSelection = vi.fn();
      const props = createDefaultProps({ onClearSelection });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Completed' }));

      await waitFor(() => {
        expect(onClearSelection).toHaveBeenCalled();
      });
    });

    it('calls onSuccess after successful status update', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const onSuccess = vi.fn();
      const props = createDefaultProps({ onSuccess });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Shelved' }));

      await waitFor(() => {
        expect(onSuccess).toHaveBeenCalled();
      });
    });

    it('does not call onSuccess when it is not provided', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const props = createDefaultProps({ onSuccess: undefined });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Completed' }));

      // Should not throw when onSuccess is undefined
      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalled();
      });
    });

    it('handles status update error gracefully', async () => {
      const user = userEvent.setup();
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const mockMutateAsync = vi.fn().mockRejectedValue(new Error('Update failed'));
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Completed' }));

      await waitFor(() => {
        expect(consoleErrorSpy).toHaveBeenCalledWith(
          'Failed to update games:',
          expect.any(Error)
        );
      });

      consoleErrorSpy.mockRestore();
    });
  });

  describe('delete functionality', () => {
    it('opens AlertDialog when delete button is clicked', async () => {
      const user = userEvent.setup();
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));

      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
        expect(screen.getByText('Delete Games')).toBeInTheDocument();
      });
    });

    it('shows correct confirmation message with singular form', async () => {
      const user = userEvent.setup();
      const props = createDefaultProps({ selectedIds: new Set(['game-1']) });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));

      await waitFor(() => {
        expect(
          screen.getByText(/Are you sure you want to delete 1 game\?/)
        ).toBeInTheDocument();
      });
    });

    it('shows correct confirmation message with plural form', async () => {
      const user = userEvent.setup();
      const props = createDefaultProps({ selectedIds: new Set(['game-1', 'game-2']) });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));

      await waitFor(() => {
        expect(
          screen.getByText(/Are you sure you want to delete 2 games\?/)
        ).toBeInTheDocument();
      });
    });

    it('calls bulkDelete mutation when delete is confirmed', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps({ selectedIds: new Set(['game-1', 'game-2']) });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      expect(mockMutateAsync).toHaveBeenCalledWith(['game-1', 'game-2']);
    });

    it('closes dialog after successful delete', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
      });
    });

    it('calls onClearSelection after successful delete', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const onClearSelection = vi.fn();
      const props = createDefaultProps({ onClearSelection });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      await waitFor(() => {
        expect(onClearSelection).toHaveBeenCalled();
      });
    });

    it('calls onSuccess after successful delete', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const onSuccess = vi.fn();
      const props = createDefaultProps({ onSuccess });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      await waitFor(() => {
        expect(onSuccess).toHaveBeenCalled();
      });
    });

    it('does not call onSuccess when it is not provided after delete', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps({ onSuccess: undefined });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      // Should not throw when onSuccess is undefined
      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalled();
      });
    });

    it('closes dialog when cancel is clicked', async () => {
      const user = userEvent.setup();
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Cancel' }));

      await waitFor(() => {
        expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
      });
    });

    it('does not call bulkDelete when cancel is clicked', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      await user.click(screen.getByRole('button', { name: 'Cancel' }));

      expect(mockMutateAsync).not.toHaveBeenCalled();
    });

    it('handles delete error gracefully', async () => {
      const user = userEvent.setup();
      const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
      const mockMutateAsync = vi.fn().mockRejectedValue(new Error('Delete failed'));
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      await waitFor(() => {
        expect(consoleErrorSpy).toHaveBeenCalledWith('Failed to delete games:', expect.any(Error));
      });

      consoleErrorSpy.mockRestore();
    });
  });

  describe('clear selection', () => {
    it('calls onClearSelection when clear button is clicked', async () => {
      const user = userEvent.setup();
      const onClearSelection = vi.fn();
      const props = createDefaultProps({ onClearSelection });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /clear/i }));

      expect(onClearSelection).toHaveBeenCalled();
    });
  });

  describe('loading states', () => {
    it('disables buttons when bulk update is pending', async () => {
      const { useBulkUpdateUserGames, useBulkDeleteUserGames } = vi.mocked(
        await import('@/hooks')
      );
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ isPending: true }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );
      useBulkDeleteUserGames.mockReturnValue(createMockMutation() as unknown as ReturnType<typeof useBulkDeleteUserGames>);

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('combobox')).toBeDisabled();
      expect(screen.getByRole('button', { name: /delete/i })).toBeDisabled();
      expect(screen.getByRole('button', { name: /clear/i })).toBeDisabled();
    });

    it('disables buttons when bulk delete is pending', async () => {
      const { useBulkUpdateUserGames, useBulkDeleteUserGames } = vi.mocked(
        await import('@/hooks')
      );
      useBulkUpdateUserGames.mockReturnValue(createMockMutation() as unknown as ReturnType<typeof useBulkUpdateUserGames>);
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ isPending: true }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('combobox')).toBeDisabled();
      expect(screen.getByRole('button', { name: /delete/i })).toBeDisabled();
      expect(screen.getByRole('button', { name: /clear/i })).toBeDisabled();
    });

    it('delete button is disabled when bulk delete is pending', async () => {
      const { useBulkUpdateUserGames, useBulkDeleteUserGames } = vi.mocked(
        await import('@/hooks')
      );
      useBulkUpdateUserGames.mockReturnValue(createMockMutation() as unknown as ReturnType<typeof useBulkUpdateUserGames>);
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ isPending: true }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      // Delete button should be disabled when bulk delete is pending
      expect(screen.getByRole('button', { name: /delete/i })).toBeDisabled();
    });

    it('enables buttons when not loading', () => {
      const props = createDefaultProps();
      render(<BulkActions {...props} />);

      expect(screen.getByRole('combobox')).not.toBeDisabled();
      expect(screen.getByRole('button', { name: /delete/i })).not.toBeDisabled();
      expect(screen.getByRole('button', { name: /clear/i })).not.toBeDisabled();
    });
  });

  describe('edge cases', () => {
    it('renders when totalCount is 0', () => {
      const props = createDefaultProps({ selectedIds: new Set(), totalCount: 0 });
      const { container } = render(<BulkActions {...props} />);

      expect(container.firstChild).toBeNull();
    });

    it('handles single game with correct array conversion', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkUpdateUserGames } = vi.mocked(await import('@/hooks'));
      useBulkUpdateUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkUpdateUserGames>
      );

      const props = createDefaultProps({ selectedIds: new Set(['single-game']) });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('combobox'));
      await user.click(screen.getByRole('option', { name: 'Mastered' }));

      expect(mockMutateAsync).toHaveBeenCalledWith({
        ids: ['single-game'],
        updates: { playStatus: PlayStatus.MASTERED },
      });
    });

    it('converts Set to Array correctly for delete', async () => {
      const user = userEvent.setup();
      const mockMutateAsync = vi.fn().mockResolvedValue({});
      const { useBulkDeleteUserGames } = vi.mocked(await import('@/hooks'));
      useBulkDeleteUserGames.mockReturnValue(
        createMockMutation({ mutateAsync: mockMutateAsync }) as unknown as ReturnType<typeof useBulkDeleteUserGames>
      );

      const props = createDefaultProps({
        selectedIds: new Set(['game-a', 'game-b', 'game-c']),
      });
      render(<BulkActions {...props} />);

      await user.click(screen.getByRole('button', { name: /delete/i }));
      await waitFor(() => {
        expect(screen.getByRole('alertdialog')).toBeInTheDocument();
      });

      const confirmButton = screen.getByRole('button', { name: 'Delete' });
      await user.click(confirmButton);

      expect(mockMutateAsync).toHaveBeenCalledWith(['game-a', 'game-b', 'game-c']);
    });
  });
});
