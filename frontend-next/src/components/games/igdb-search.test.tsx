import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { IGDBSearch } from './igdb-search';
import type { IGDBGameCandidate, GameId } from '@/types';

// Mock lucide-react icons
vi.mock('lucide-react', () => ({
  Search: ({ className }: { className?: string }) => <div data-testid="search-icon" className={className} />,
  Loader2: ({ className }: { className?: string }) => <div data-testid="loader-icon" className={className} />,
  Gamepad2: ({ className }: { className?: string }) => <div data-testid="gamepad-icon" className={className} />,
  Calendar: ({ className }: { className?: string }) => <div data-testid="calendar-icon" className={className} />,
  Monitor: ({ className }: { className?: string }) => <div data-testid="monitor-icon" className={className} />,
}));

// Mock useSearchIGDB hook
const mockUseSearchIGDB = vi.fn();
vi.mock('@/hooks/use-games', () => ({
  useSearchIGDB: (query: string) => mockUseSearchIGDB(query),
}));

// Mock game data
const createMockGame = (overrides: Partial<IGDBGameCandidate> = {}): IGDBGameCandidate => ({
  igdb_id: 12345 as GameId,
  title: 'Test Game',
  release_date: '2024-01-15',
  cover_art_url: 'https://example.com/cover.jpg',
  description: 'A test game description',
  platforms: ['PC', 'PlayStation 5'],
  howlongtobeat_main: 20,
  howlongtobeat_extra: 35,
  howlongtobeat_completionist: 50,
  ...overrides,
});

describe('IGDBSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  describe('initial state', () => {
    it('renders search input with placeholder', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      expect(screen.getByPlaceholderText('Search for a game...')).toBeInTheDocument();
    });

    it('renders custom placeholder when provided', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} placeholder="Find a game..." />);

      expect(screen.getByPlaceholderText('Find a game...')).toBeInTheDocument();
    });

    it('renders initial state message when no query', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      expect(screen.getByText('Search IGDB')).toBeInTheDocument();
      expect(screen.getByText('Find games from the Internet Game Database')).toBeInTheDocument();
    });

    it('auto-focuses input when autoFocus is true', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} autoFocus={true} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      expect(input).toHaveFocus();
    });

    it('does not auto-focus input by default', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      expect(input).not.toHaveFocus();
    });
  });

  describe('disabled state', () => {
    it('disables input when disabled prop is true', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} disabled={true} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      expect(input).toBeDisabled();
    });

    it('enables input by default', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      expect(input).not.toBeDisabled();
    });
  });

  describe('minimum characters message', () => {
    it('shows min chars message when 1 character is typed', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'z');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      expect(screen.getByText('Type at least 3 characters to search')).toBeInTheDocument();
    });

    it('shows min chars message when 2 characters are typed', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'ze');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      expect(screen.getByText('Type at least 3 characters to search')).toBeInTheDocument();
    });

    it('does not show min chars message when 3 characters are typed', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: [],
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'zel');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      expect(screen.queryByText('Type at least 3 characters to search')).not.toBeInTheDocument();
    });
  });

  describe('debounce behavior', () => {
    it('debounces search query by 300ms', async () => {
      const user = userEvent.setup({ delay: null });
      let currentQuery = '';

      mockUseSearchIGDB.mockImplementation((query: string) => {
        currentQuery = query;
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
        };
      });

      const { rerender } = render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');

      // Before debounce timeout, query should still be empty
      expect(currentQuery).toBe('');

      // Advance timers by 299ms (before debounce completes)
      vi.advanceTimersByTime(299);
      rerender(<IGDBSearch onSelect={vi.fn()} />);
      expect(currentQuery).toBe('');

      // Advance by 1ms more to complete debounce (total 300ms)
      vi.advanceTimersByTime(1);
      rerender(<IGDBSearch onSelect={vi.fn()} />);

      // Now debounced query should be updated
      expect(currentQuery).toBe('test');
    });

    it('resets debounce timer on each keystroke', async () => {
      const user = userEvent.setup({ delay: null });
      let currentQuery = '';

      mockUseSearchIGDB.mockImplementation((query: string) => {
        currentQuery = query;
        return {
          data: undefined,
          isLoading: false,
          isFetching: false,
        };
      });

      const { rerender } = render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');

      // Type first character
      await user.type(input, 't');
      vi.advanceTimersByTime(200);

      // Type second character before debounce completes
      await user.type(input, 'e');
      vi.advanceTimersByTime(200);

      // Query should still be empty
      rerender(<IGDBSearch onSelect={vi.fn()} />);
      expect(currentQuery).toBe('');

      // Complete the debounce from the last keystroke
      vi.advanceTimersByTime(100);
      rerender(<IGDBSearch onSelect={vi.fn()} />);
      expect(currentQuery).toBe('te');
    });
  });

  describe('loading state', () => {
    it('shows loading state when isLoading is true', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      expect(screen.getByText('Searching IGDB...')).toBeInTheDocument();
      expect(screen.getByTestId('loader-icon')).toBeInTheDocument();
    });

    it('shows loading state when isFetching is true', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: true,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      expect(screen.getByText('Searching IGDB...')).toBeInTheDocument();
      expect(screen.getByTestId('loader-icon')).toBeInTheDocument();
    });

    it('shows loading state when both isLoading and isFetching are true', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: true,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      expect(screen.getByText('Searching IGDB...')).toBeInTheDocument();
    });
  });

  describe('empty results state', () => {
    it('shows empty state when no results are found', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: [],
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'nonexistentgame');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText(/No games found for/)).toBeInTheDocument();
        expect(screen.getByText('Try a different search term')).toBeInTheDocument();
      });
    });

    it('shows empty state with query in message', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: [],
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'testquery');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText(/No games found for "testquery"/)).toBeInTheDocument();
      });
    });

    it('does not show empty state when results array is undefined', async () => {
      const user = userEvent.setup({ delay: null });
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'testquery');

      // Advance timers for debounce
      vi.advanceTimersByTime(300);

      expect(screen.queryByText(/No games found/)).not.toBeInTheDocument();
    });
  });

  describe('results display', () => {
    it('renders game results with title', async () => {
      const game = createMockGame({ title: 'The Legend of Zelda' });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'zelda');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('The Legend of Zelda')).toBeInTheDocument();
      });
    });

    it('renders game with cover art', async () => {
      const game = createMockGame({
        title: 'Test Game',
        cover_art_url: 'https://example.com/cover.jpg',
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        const img = screen.getByAltText('Test Game cover');
        expect(img).toBeInTheDocument();
        expect(img).toHaveAttribute('src', 'https://example.com/cover.jpg');
      });
    });

    it('renders placeholder when cover art is missing', async () => {
      const game = createMockGame({
        title: 'Test Game',
        cover_art_url: undefined,
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByTestId('gamepad-icon')).toBeInTheDocument();
      });
    });

    it('renders release year when release_date is provided', async () => {
      const game = createMockGame({
        title: 'Test Game',
        release_date: '2024-06-15',
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('2024')).toBeInTheDocument();
        expect(screen.getByTestId('calendar-icon')).toBeInTheDocument();
      });
    });

    it('does not render release year when release_date is missing', async () => {
      const game = createMockGame({
        title: 'Test Game',
        release_date: undefined,
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Calendar icon should not be present
      const calendarIcons = screen.queryAllByTestId('calendar-icon');
      expect(calendarIcons).toHaveLength(0);
    });

    it('renders platforms list', async () => {
      const game = createMockGame({
        title: 'Test Game',
        platforms: ['PC', 'PlayStation 5', 'Xbox Series X'],
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('PC, PlayStation 5, Xbox Series X')).toBeInTheDocument();
        expect(screen.getByTestId('monitor-icon')).toBeInTheDocument();
      });
    });

    it('truncates platforms list when more than 3 platforms', async () => {
      const game = createMockGame({
        title: 'Test Game',
        platforms: ['PC', 'PlayStation 5', 'Xbox Series X', 'Nintendo Switch', 'Steam Deck'],
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('PC, PlayStation 5, Xbox Series X +2')).toBeInTheDocument();
      });
    });

    it('does not render platforms section when platforms array is empty', async () => {
      const game = createMockGame({
        title: 'Test Game',
        platforms: [],
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Monitor icon should not be present
      const monitorIcons = screen.queryAllByTestId('monitor-icon');
      expect(monitorIcons).toHaveLength(0);
    });

    it('renders description when provided', async () => {
      const game = createMockGame({
        title: 'Test Game',
        description: 'This is an amazing adventure game with stunning graphics.',
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('This is an amazing adventure game with stunning graphics.')).toBeInTheDocument();
      });
    });

    it('does not render description when missing', async () => {
      const game = createMockGame({
        title: 'Test Game',
        description: undefined,
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Description should not render
      const descriptions = screen.queryByText(/amazing adventure/);
      expect(descriptions).not.toBeInTheDocument();
    });

    it('renders multiple game results', async () => {
      const games = [
        createMockGame({ igdb_id: 1 as GameId, title: 'Game One' }),
        createMockGame({ igdb_id: 2 as GameId, title: 'Game Two' }),
        createMockGame({ igdb_id: 3 as GameId, title: 'Game Three' }),
      ];
      mockUseSearchIGDB.mockReturnValue({
        data: games,
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'game');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Game One')).toBeInTheDocument();
        expect(screen.getByText('Game Two')).toBeInTheDocument();
        expect(screen.getByText('Game Three')).toBeInTheDocument();
      });
    });

    it('displays correct result count in heading', async () => {
      const games = [
        createMockGame({ igdb_id: 1 as GameId, title: 'Game One' }),
        createMockGame({ igdb_id: 2 as GameId, title: 'Game Two' }),
      ];
      mockUseSearchIGDB.mockReturnValue({
        data: games,
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'game');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Found 2 games')).toBeInTheDocument();
      });
    });

    it('uses singular "game" when only one result', async () => {
      const game = createMockGame({ title: 'Single Game' });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'single');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Found 1 game')).toBeInTheDocument();
      });
    });
  });

  describe('game selection', () => {
    it('calls onSelect when a game is clicked', async () => {
      const game = createMockGame({ title: 'Clickable Game' });
      const onSelect = vi.fn();
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={onSelect} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'click');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Clickable Game')).toBeInTheDocument();
      });

      await user.click(screen.getByText('Clickable Game'));

      expect(onSelect).toHaveBeenCalledTimes(1);
      expect(onSelect).toHaveBeenCalledWith(game);
    });

    it('calls onSelect with correct game when multiple results', async () => {
      const game1 = createMockGame({ igdb_id: 1 as GameId, title: 'First Game' });
      const game2 = createMockGame({ igdb_id: 2 as GameId, title: 'Second Game' });
      const onSelect = vi.fn();
      mockUseSearchIGDB.mockReturnValue({
        data: [game1, game2],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={onSelect} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'game');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Second Game')).toBeInTheDocument();
      });

      await user.click(screen.getByText('Second Game'));

      expect(onSelect).toHaveBeenCalledTimes(1);
      expect(onSelect).toHaveBeenCalledWith(game2);
    });
  });

  describe('className prop', () => {
    it('applies custom className to wrapper', () => {
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      const { container } = render(
        <IGDBSearch onSelect={vi.fn()} className="custom-class" />
      );

      const wrapper = container.querySelector('.custom-class');
      expect(wrapper).toBeInTheDocument();
    });
  });

  describe('edge cases', () => {
    it('handles invalid release_date gracefully', async () => {
      const game = createMockGame({
        title: 'Test Game',
        release_date: 'invalid-date',
      });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...');
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Should not crash, calendar icon should not be present
      const calendarIcons = screen.queryAllByTestId('calendar-icon');
      expect(calendarIcons).toHaveLength(0);
    });

    it('handles clearing search query', async () => {
      const game = createMockGame({ title: 'Test Game' });
      mockUseSearchIGDB.mockReturnValue({
        data: [game],
        isLoading: false,
        isFetching: false,
      });

      const user = userEvent.setup({ delay: null });
      render(<IGDBSearch onSelect={vi.fn()} />);

      const input = screen.getByPlaceholderText('Search for a game...') as HTMLInputElement;

      // Type a query
      await user.type(input, 'test');
      vi.advanceTimersByTime(300);

      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
      });

      // Clear the query
      await user.clear(input);
      vi.advanceTimersByTime(300);

      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: false,
        isFetching: false,
      });

      // Should show initial state again
      await waitFor(() => {
        expect(screen.getByText('Search IGDB')).toBeInTheDocument();
      });
    });

    it('does not call onSelect when loading', async () => {
      const onSelect = vi.fn();
      mockUseSearchIGDB.mockReturnValue({
        data: undefined,
        isLoading: true,
        isFetching: false,
      });

      render(<IGDBSearch onSelect={onSelect} />);

      // Loading state should be present
      expect(screen.getByText('Searching IGDB...')).toBeInTheDocument();

      // onSelect should not be called
      expect(onSelect).not.toHaveBeenCalled();
    });
  });
});
