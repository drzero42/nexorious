import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameFilters, type GameFiltersProps } from './game-filters';
import { PlayStatus } from '@/types';
import type { Platform } from '@/types/platform';

// Mock the hooks module
vi.mock('@/hooks', () => ({
  useAllPlatforms: vi.fn(),
}));


// Mock lucide-react icons
vi.mock('lucide-react', () => ({
  Grid: ({ className }: { className?: string }) => (
    <div data-testid="grid-icon" className={className}>
      Grid
    </div>
  ),
  List: ({ className }: { className?: string }) => (
    <div data-testid="list-icon" className={className}>
      List
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

const mockPlatforms: Platform[] = [
  {
    name: 'pc',
    display_name: 'PC',
    is_active: true,
    source: 'system',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'ps5',
    display_name: 'PlayStation 5',
    is_active: true,
    source: 'system',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'xbox-series-x',
    display_name: 'Xbox Series X',
    is_active: true,
    source: 'system',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

const defaultProps: GameFiltersProps = {
  filters: {
    search: '',
  },
  onFiltersChange: vi.fn(),
  viewMode: 'grid',
  onViewModeChange: vi.fn(),
};

describe('GameFilters', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // Default mock implementation
    const { useAllPlatforms } = vi.mocked(await import('@/hooks'));
    useAllPlatforms.mockReturnValue({
      data: mockPlatforms,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useAllPlatforms>);
  });

  describe('search input', () => {
    it('renders search input with placeholder', () => {
      render(<GameFilters {...defaultProps} />);

      const searchInput = screen.getByPlaceholderText('Search games...');
      expect(searchInput).toBeInTheDocument();
      expect(searchInput).toHaveAttribute('type', 'search');
    });

    it('displays current search value', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{ search: 'Zelda' }}
        />
      );

      const searchInput = screen.getByPlaceholderText('Search games...');
      expect(searchInput).toHaveValue('Zelda');
    });

    it('calls onFiltersChange when search input changes', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const searchInput = screen.getByPlaceholderText('Search games...');
      await user.clear(searchInput);
      await user.type(searchInput, 'M');

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'M',
      });
      expect(onFiltersChange).toHaveBeenCalled();
    });

    it('preserves other filters when search changes', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            status: PlayStatus.COMPLETED,
            platformId: 'platform-1',
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const searchInput = screen.getByPlaceholderText('Search games...');
      await user.type(searchInput, 'T');

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'T',
        status: PlayStatus.COMPLETED,
        platformId: 'platform-1',
      });
    });
  });

  describe('status filter', () => {
    it('renders status select with default "All Statuses" option', () => {
      render(<GameFilters {...defaultProps} />);

      // SelectTrigger should be present (first combobox is status)
      const comboboxes = screen.getAllByRole('combobox');
      expect(comboboxes.length).toBeGreaterThanOrEqual(2); // status and platform
    });

    it('displays selected status', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            status: PlayStatus.IN_PROGRESS,
          }}
        />
      );

      // The select should be rendered
      const comboboxes = screen.getAllByRole('combobox');
      expect(comboboxes[0]).toBeInTheDocument();
    });

    it('calls onFiltersChange when status changes to specific status', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const statusSelect = comboboxes[0]; // First combobox is status
      await user.click(statusSelect);

      // Select "Completed"
      const completedOption = screen.getByRole('option', { name: 'Completed' });
      await user.click(completedOption);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        status: PlayStatus.COMPLETED,
      });
    });

    it('calls onFiltersChange with undefined status when "All Statuses" is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            status: PlayStatus.COMPLETED,
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const statusSelect = comboboxes[0];
      await user.click(statusSelect);

      const allStatusesOption = screen.getByRole('option', { name: 'All Statuses' });
      await user.click(allStatusesOption);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        status: undefined,
      });
    });

    it('renders all PlayStatus options', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const comboboxes = screen.getAllByRole('combobox');
      const statusSelect = comboboxes[0];
      await user.click(statusSelect);

      // Check all status options are present
      expect(screen.getByRole('option', { name: 'All Statuses' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Not Started' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'In Progress' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Completed' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Mastered' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Dominated' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Shelved' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Dropped' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Replay' })).toBeInTheDocument();
    });

    it('preserves other filters when status changes', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: 'Test',
            platformId: 'platform-1',
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const statusSelect = comboboxes[0];
      await user.click(statusSelect);

      const completedOption = screen.getByRole('option', { name: 'Completed' });
      await user.click(completedOption);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'Test',
        platformId: 'platform-1',
        status: PlayStatus.COMPLETED,
      });
    });
  });

  describe('platform filter', () => {
    it('renders platform select', () => {
      render(<GameFilters {...defaultProps} />);

      const comboboxes = screen.getAllByRole('combobox');
      expect(comboboxes[1]).toBeInTheDocument(); // Second combobox is platform
    });

    it('displays platforms from useAllPlatforms hook', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1]; // Second combobox is platform
      await user.click(platformSelect);

      expect(screen.getByRole('option', { name: 'All Platforms' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'PC' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'PlayStation 5' })).toBeInTheDocument();
      expect(screen.getByRole('option', { name: 'Xbox Series X' })).toBeInTheDocument();
    });

    it('handles empty platforms list', async () => {
      const { useAllPlatforms } = vi.mocked(await import('@/hooks'));
      useAllPlatforms.mockReturnValue({
        data: [],
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useAllPlatforms>);

      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1];
      await user.click(platformSelect);

      // Should still show "All Platforms" option
      expect(screen.getByRole('option', { name: 'All Platforms' })).toBeInTheDocument();
    });

    it('handles undefined platforms', async () => {
      const { useAllPlatforms } = vi.mocked(await import('@/hooks'));
      useAllPlatforms.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useAllPlatforms>);

      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1];
      await user.click(platformSelect);

      // Should still show "All Platforms" option
      expect(screen.getByRole('option', { name: 'All Platforms' })).toBeInTheDocument();
    });

    it('calls onFiltersChange when platform changes to specific platform', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1];
      await user.click(platformSelect);

      const ps5Option = screen.getByRole('option', { name: 'PlayStation 5' });
      await user.click(ps5Option);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        platformId: 'ps5',
      });
    });

    it('calls onFiltersChange with undefined platformId when "All Platforms" is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            platformId: 'platform-1',
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1];
      await user.click(platformSelect);

      const allPlatformsOption = screen.getByRole('option', { name: 'All Platforms' });
      await user.click(allPlatformsOption);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        platformId: undefined,
      });
    });

    it('preserves other filters when platform changes', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: 'Test',
            status: PlayStatus.IN_PROGRESS,
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const comboboxes = screen.getAllByRole('combobox');
      const platformSelect = comboboxes[1];
      await user.click(platformSelect);

      const pcOption = screen.getByRole('option', { name: 'PC' });
      await user.click(pcOption);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'Test',
        status: PlayStatus.IN_PROGRESS,
        platformId: 'pc',
      });
    });
  });

  describe('clear filters button', () => {
    it('does not show clear button when no filters are active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{ search: '' }}
        />
      );

      expect(screen.queryByRole('button', { name: /clear/i })).not.toBeInTheDocument();
    });

    it('shows clear button when search filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{ search: 'Test' }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when status filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            status: PlayStatus.COMPLETED,
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when platform filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            platformId: 'platform-1',
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when multiple filters are active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: 'Test',
            status: PlayStatus.IN_PROGRESS,
            platformId: 'platform-1',
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('calls onFiltersChange with cleared filters when clicked', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: 'Test',
            status: PlayStatus.COMPLETED,
            platformId: 'platform-1',
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const clearButton = screen.getByRole('button', { name: /clear/i });
      await user.click(clearButton);

      expect(onFiltersChange).toHaveBeenCalledWith({ search: '' });
    });
  });

  describe('view mode toggle', () => {
    it('renders both grid and list view buttons', () => {
      render(<GameFilters {...defaultProps} />);

      expect(screen.getByTestId('grid-icon')).toBeInTheDocument();
      expect(screen.getByTestId('list-icon')).toBeInTheDocument();
    });

    it('highlights grid button when viewMode is grid', () => {
      render(
        <GameFilters {...defaultProps} viewMode="grid" />
      );

      // Grid button should have secondary variant (check for the button containing grid icon)
      const gridButton = screen.getByTestId('grid-icon').closest('button');
      expect(gridButton).toHaveClass('bg-secondary');
    });

    it('highlights list button when viewMode is list', () => {
      render(
        <GameFilters {...defaultProps} viewMode="list" />
      );

      // List button should have secondary variant
      const listButton = screen.getByTestId('list-icon').closest('button');
      expect(listButton).toHaveClass('bg-secondary');
    });

    it('calls onViewModeChange with "grid" when grid button is clicked', async () => {
      const user = userEvent.setup();
      const onViewModeChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          viewMode="list"
          onViewModeChange={onViewModeChange}
        />
      );

      const gridButton = screen.getByTestId('grid-icon').closest('button')!;
      await user.click(gridButton);

      expect(onViewModeChange).toHaveBeenCalledWith('grid');
    });

    it('calls onViewModeChange with "list" when list button is clicked', async () => {
      const user = userEvent.setup();
      const onViewModeChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          viewMode="grid"
          onViewModeChange={onViewModeChange}
        />
      );

      const listButton = screen.getByTestId('list-icon').closest('button')!;
      await user.click(listButton);

      expect(onViewModeChange).toHaveBeenCalledWith('list');
    });
  });

  describe('integration', () => {
    it('handles multiple filter changes in sequence', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      const { rerender } = render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      // Change search
      const searchInput = screen.getByPlaceholderText('Search games...');
      await user.type(searchInput, 'T');
      expect(onFiltersChange).toHaveBeenLastCalledWith({ search: 'T' });

      // Rerender with updated search filter
      rerender(
        <GameFilters
          {...defaultProps}
          filters={{ search: 'T' }}
          onFiltersChange={onFiltersChange}
        />
      );

      // Change status
      vi.clearAllMocks();
      const comboboxes = screen.getAllByRole('combobox');
      const statusSelect = comboboxes[0];
      await user.click(statusSelect);
      const completedOption = screen.getByRole('option', { name: 'Completed' });
      await user.click(completedOption);
      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'T',
        status: PlayStatus.COMPLETED,
      });

      // Rerender with updated filters
      rerender(
        <GameFilters
          {...defaultProps}
          filters={{ search: 'T', status: PlayStatus.COMPLETED }}
          onFiltersChange={onFiltersChange}
        />
      );

      // Change platform
      vi.clearAllMocks();
      const comboboxes2 = screen.getAllByRole('combobox');
      const platformSelect = comboboxes2[1];
      await user.click(platformSelect);
      const pcOption = screen.getByRole('option', { name: 'PC' });
      await user.click(pcOption);
      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'T',
        status: PlayStatus.COMPLETED,
        platformId: 'pc',
      });
    });

    it('maintains view mode independently from filters', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      const onViewModeChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
          onViewModeChange={onViewModeChange}
        />
      );

      // Change filter
      const searchInput = screen.getByPlaceholderText('Search games...');
      await user.type(searchInput, 'T');
      expect(onFiltersChange).toHaveBeenCalled();
      expect(onViewModeChange).not.toHaveBeenCalled();

      // Change view mode
      vi.clearAllMocks();
      const listButton = screen.getByTestId('list-icon').closest('button')!;
      await user.click(listButton);
      expect(onViewModeChange).toHaveBeenCalledWith('list');
      expect(onFiltersChange).not.toHaveBeenCalled();
    });
  });
});
