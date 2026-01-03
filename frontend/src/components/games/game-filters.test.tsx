import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { GameFilters, type GameFiltersProps } from './game-filters';
import { PlayStatus } from '@/types';
import type { Platform, Storefront } from '@/types/platform';
import type { Tag } from '@/types';

// Mock the hooks module
vi.mock('@/hooks', () => ({
  useAllPlatforms: vi.fn(),
  useAllStorefronts: vi.fn(),
  useUserGameGenres: vi.fn(),
  useAllTags: vi.fn(),
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
  ArrowDownAZ: ({ className }: { className?: string }) => (
    <div data-testid="arrow-down-az-icon" className={className}>
      AZ↓
    </div>
  ),
  ArrowUpAZ: ({ className }: { className?: string }) => (
    <div data-testid="arrow-up-az-icon" className={className}>
      AZ↑
    </div>
  ),
  ArrowDown: ({ className }: { className?: string }) => (
    <div data-testid="arrow-down-icon" className={className}>
      ↓
    </div>
  ),
  ArrowUp: ({ className }: { className?: string }) => (
    <div data-testid="arrow-up-icon" className={className}>
      ↑
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

const mockStorefronts: Storefront[] = [
  {
    name: 'steam',
    display_name: 'Steam',
    is_active: true,
    source: 'system',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'epic',
    display_name: 'Epic Games Store',
    is_active: true,
    source: 'system',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
];

const mockGenres: string[] = ['Action', 'RPG', 'Adventure'];

const mockTags: Tag[] = [
  {
    id: 'tag-1',
    name: 'Favorite',
    color: '#ff0000',
    user_id: 'user-1',
    game_count: 5,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 'tag-2',
    name: 'Backlog',
    color: '#00ff00',
    user_id: 'user-1',
    game_count: 10,
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
  sortBy: 'title',
  sortOrder: 'asc',
  onSortByChange: vi.fn(),
  onSortOrderToggle: vi.fn(),
};

describe('GameFilters', () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // Default mock implementation
    const { useAllPlatforms, useAllStorefronts, useUserGameGenres, useAllTags } = vi.mocked(await import('@/hooks'));
    useAllPlatforms.mockReturnValue({
      data: mockPlatforms,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useAllPlatforms>);
    useAllStorefronts.mockReturnValue({
      data: mockStorefronts,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useAllStorefronts>);
    useUserGameGenres.mockReturnValue({
      data: mockGenres,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useUserGameGenres>);
    useAllTags.mockReturnValue({
      data: mockTags,
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useAllTags>);
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

  describe('platform filter (multi-select)', () => {
    it('renders platforms multi-select button', () => {
      render(<GameFilters {...defaultProps} />);

      // MultiSelectFilter button contains the label text "Platforms"
      // The button has role="combobox" so find by text
      expect(screen.getByText('Platforms')).toBeInTheDocument();
    });

    it('displays platforms from useAllPlatforms hook', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);

      // MultiSelectFilter shows checkboxes for each option
      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('PlayStation 5')).toBeInTheDocument();
      expect(screen.getByText('Xbox Series X')).toBeInTheDocument();
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

      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);

      // Should show "No options available" message
      expect(screen.getByText('No options available')).toBeInTheDocument();
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

      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);

      // Should show "No options available" message
      expect(screen.getByText('No options available')).toBeInTheDocument();
    });

    it('calls onFiltersChange when platform is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);

      // Click on PlayStation 5 checkbox
      const ps5Label = screen.getByText('PlayStation 5');
      await user.click(ps5Label);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        platforms: ['ps5'],
      });
    });

    it('calls onFiltersChange when platform is deselected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            platforms: ['ps5'],
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const platformsButton = screen.getByText('Platforms (1)').closest('button')!;
      await user.click(platformsButton);

      // Click on PlayStation 5 checkbox to deselect
      const ps5Label = screen.getByText('PlayStation 5');
      await user.click(ps5Label);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        platforms: [],
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

      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);

      const pcLabel = screen.getByText('PC');
      await user.click(pcLabel);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'Test',
        status: PlayStatus.IN_PROGRESS,
        platforms: ['pc'],
      });
    });

    it('shows selected count in button label', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            platforms: ['pc', 'ps5'],
          }}
        />
      );

      expect(screen.getByText('Platforms (2)')).toBeInTheDocument();
    });
  });

  describe('storefront filter (multi-select)', () => {
    it('renders storefronts multi-select button', () => {
      render(<GameFilters {...defaultProps} />);

      expect(screen.getByText('Storefronts')).toBeInTheDocument();
    });

    it('displays storefronts from useAllStorefronts hook', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const storefrontsButton = screen.getByText('Storefronts').closest('button')!;
      await user.click(storefrontsButton);

      expect(screen.getByText('Steam')).toBeInTheDocument();
      expect(screen.getByText('Epic Games Store')).toBeInTheDocument();
    });

    it('calls onFiltersChange when storefront is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const storefrontsButton = screen.getByText('Storefronts').closest('button')!;
      await user.click(storefrontsButton);

      const steamLabel = screen.getByText('Steam');
      await user.click(steamLabel);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        storefronts: ['steam'],
      });
    });
  });

  describe('genre filter (multi-select)', () => {
    it('renders genres multi-select button', () => {
      render(<GameFilters {...defaultProps} />);

      expect(screen.getByText('Genres')).toBeInTheDocument();
    });

    it('displays genres from useUserGameGenres hook', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const genresButton = screen.getByText('Genres').closest('button')!;
      await user.click(genresButton);

      expect(screen.getByText('Action')).toBeInTheDocument();
      expect(screen.getByText('RPG')).toBeInTheDocument();
      expect(screen.getByText('Adventure')).toBeInTheDocument();
    });

    it('calls onFiltersChange when genre is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const genresButton = screen.getByText('Genres').closest('button')!;
      await user.click(genresButton);

      const rpgLabel = screen.getByText('RPG');
      await user.click(rpgLabel);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        genres: ['RPG'],
      });
    });
  });

  describe('tags filter (multi-select)', () => {
    it('renders tags multi-select button', () => {
      render(<GameFilters {...defaultProps} />);

      expect(screen.getByText('Tags')).toBeInTheDocument();
    });

    it('displays tags from useAllTags hook', async () => {
      const user = userEvent.setup();
      render(<GameFilters {...defaultProps} />);

      const tagsButton = screen.getByText('Tags').closest('button')!;
      await user.click(tagsButton);

      expect(screen.getByText('Favorite')).toBeInTheDocument();
      expect(screen.getByText('Backlog')).toBeInTheDocument();
    });

    it('calls onFiltersChange when tag is selected', async () => {
      const user = userEvent.setup();
      const onFiltersChange = vi.fn();
      render(
        <GameFilters
          {...defaultProps}
          onFiltersChange={onFiltersChange}
        />
      );

      const tagsButton = screen.getByText('Tags').closest('button')!;
      await user.click(tagsButton);

      const favoriteLabel = screen.getByText('Favorite');
      await user.click(favoriteLabel);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        tags: ['Favorite'],
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

    it('shows clear button when platform filter is active (legacy platformId)', () => {
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

    it('shows clear button when platforms multi-select filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            platforms: ['pc'],
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when storefronts filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            storefronts: ['steam'],
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when genres filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            genres: ['RPG'],
          }}
        />
      );

      expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument();
    });

    it('shows clear button when tags filter is active', () => {
      render(
        <GameFilters
          {...defaultProps}
          filters={{
            search: '',
            tags: ['Favorite'],
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
            platforms: ['pc'],
            storefronts: ['steam'],
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
            platforms: ['pc'],
            storefronts: ['steam'],
            genres: ['RPG'],
            tags: ['Favorite'],
          }}
          onFiltersChange={onFiltersChange}
        />
      );

      const clearButton = screen.getByRole('button', { name: /clear/i });
      await user.click(clearButton);

      expect(onFiltersChange).toHaveBeenCalledWith({
        search: '',
        status: undefined,
        platformId: undefined,
        platforms: [],
        storefronts: [],
        genres: [],
        tags: [],
      });
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

      // Change status - click on the button showing "All Statuses"
      vi.clearAllMocks();
      const statusSelect = screen.getByText('All Statuses').closest('button')!;
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

      // Change platform (now multi-select)
      vi.clearAllMocks();
      const platformsButton = screen.getByText('Platforms').closest('button')!;
      await user.click(platformsButton);
      const pcLabel = screen.getByText('PC');
      await user.click(pcLabel);
      expect(onFiltersChange).toHaveBeenCalledWith({
        search: 'T',
        status: PlayStatus.COMPLETED,
        platforms: ['pc'],
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

  describe('sort controls', () => {
    it('renders sort dropdown', () => {
      render(<GameFilters {...defaultProps} />);

      // Find the sort select - it shows "Title" by default
      expect(screen.getByText('Title')).toBeInTheDocument();
    });

    it('renders sort direction toggle button', () => {
      render(<GameFilters {...defaultProps} />);

      expect(screen.getByTitle('Ascending')).toBeInTheDocument();
    });

    it('shows descending title when sortOrder is desc', () => {
      render(<GameFilters {...defaultProps} sortOrder="desc" />);

      expect(screen.getByTitle('Descending')).toBeInTheDocument();
    });

    it('calls onSortByChange when sort option is selected', async () => {
      const user = userEvent.setup();
      const onSortByChange = vi.fn();
      render(
        <GameFilters {...defaultProps} onSortByChange={onSortByChange} />
      );

      // Find the sort select by its current value "Title"
      const sortSelect = screen.getByText('Title').closest('button')!;
      await user.click(sortSelect);

      const dateAddedOption = screen.getByRole('option', { name: 'Date Added' });
      await user.click(dateAddedOption);

      expect(onSortByChange).toHaveBeenCalledWith('created_at');
    });

    it('calls onSortOrderToggle when direction button is clicked', async () => {
      const user = userEvent.setup();
      const onSortOrderToggle = vi.fn();
      render(
        <GameFilters {...defaultProps} onSortOrderToggle={onSortOrderToggle} />
      );

      await user.click(screen.getByTitle('Ascending'));
      expect(onSortOrderToggle).toHaveBeenCalled();
    });

    it('shows alphabetical icon for title sort', () => {
      render(<GameFilters {...defaultProps} sortBy="title" sortOrder="asc" />);

      expect(screen.getByTestId('arrow-down-az-icon')).toBeInTheDocument();
    });

    it('shows arrow icon for non-title sort', () => {
      render(<GameFilters {...defaultProps} sortBy="created_at" sortOrder="asc" />);

      expect(screen.getByTestId('arrow-up-icon')).toBeInTheDocument();
    });
  });
});
