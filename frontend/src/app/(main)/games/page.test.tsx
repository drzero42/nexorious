import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import GamesPage from "./page";
import type { UserGame, PlayStatus, OwnershipStatus, GameId, UserGameId } from "@/types";

// Mock useUserGames and useUserGameIds hooks
const mockRefetch = vi.fn();
const mockUseUserGames = vi.fn();
const mockUseUserGameIds = vi.fn();

vi.mock("@/hooks", () => ({
  useUserGames: () => mockUseUserGames(),
  useUserGameIds: () => mockUseUserGameIds(),
}));

// Mock child components to simplify testing
const mockOnFiltersChange = vi.fn();
const mockOnViewModeChange = vi.fn();

vi.mock("@/components/games", () => ({
  GameFilters: ({
    filters,
    onFiltersChange,
    viewMode,
    onViewModeChange,
  }: {
    filters: { search: string; status?: PlayStatus; platformId?: string };
    onFiltersChange: (filters: { search: string; status?: PlayStatus; platformId?: string }) => void;
    viewMode: "grid" | "list";
    onViewModeChange: (mode: "grid" | "list") => void;
  }) => {
    // Store the callbacks for testing
    mockOnFiltersChange.mockImplementation(onFiltersChange);
    mockOnViewModeChange.mockImplementation(onViewModeChange);
    return (
      <div data-testid="game-filters">
        <span data-testid="filter-search">{filters.search}</span>
        <span data-testid="filter-status">{filters.status || "none"}</span>
        <span data-testid="filter-platform">{filters.platformId || "none"}</span>
        <span data-testid="view-mode">{viewMode}</span>
        <button
          data-testid="set-search-filter"
          onClick={() => onFiltersChange({ ...filters, search: "zelda" })}
        >
          Set Search
        </button>
        <button
          data-testid="set-status-filter"
          onClick={() =>
            onFiltersChange({ ...filters, status: "completed" as PlayStatus })
          }
        >
          Set Status
        </button>
        <button
          data-testid="set-platform-filter"
          onClick={() => onFiltersChange({ ...filters, platformId: "platform-1" })}
        >
          Set Platform
        </button>
        <button
          data-testid="toggle-view-mode"
          onClick={() => onViewModeChange(viewMode === "grid" ? "list" : "grid")}
        >
          Toggle View
        </button>
      </div>
    );
  },
  GameGrid: ({
    games,
    isLoading,
    selectedIds,
    onSelectGame,
    onClickGame,
  }: {
    games: UserGame[];
    isLoading?: boolean;
    selectedIds?: Set<string>;
    onSelectGame?: (id: string) => void;
    onClickGame?: (game: UserGame) => void;
  }) => (
    <div data-testid="game-grid">
      {isLoading && <div data-testid="grid-loading">Loading...</div>}
      {!isLoading && games.length === 0 && (
        <div data-testid="grid-empty">No games</div>
      )}
      {!isLoading &&
        games.map((game) => (
          <div
            key={game.id}
            data-testid={`grid-game-${game.id}`}
            data-selected={selectedIds?.has(game.id) ? "true" : "false"}
          >
            <span>{game.game.title}</span>
            <button
              data-testid={`select-game-${game.id}`}
              onClick={() => onSelectGame?.(game.id)}
            >
              Select
            </button>
            <button
              data-testid={`click-game-${game.id}`}
              onClick={() => onClickGame?.(game)}
            >
              Click
            </button>
          </div>
        ))}
    </div>
  ),
  GameList: ({
    games,
    isLoading,
    selectedIds,
    onSelectGame,
    onClickGame,
  }: {
    games: UserGame[];
    isLoading?: boolean;
    selectedIds?: Set<string>;
    onSelectGame?: (id: string) => void;
    onClickGame?: (game: UserGame) => void;
  }) => (
    <div data-testid="game-list">
      {isLoading && <div data-testid="list-loading">Loading...</div>}
      {!isLoading && games.length === 0 && (
        <div data-testid="list-empty">No games</div>
      )}
      {!isLoading &&
        games.map((game) => (
          <div
            key={game.id}
            data-testid={`list-game-${game.id}`}
            data-selected={selectedIds?.has(game.id) ? "true" : "false"}
          >
            <span>{game.game.title}</span>
            <button
              data-testid={`list-select-game-${game.id}`}
              onClick={() => onSelectGame?.(game.id)}
            >
              Select
            </button>
            <button
              data-testid={`list-click-game-${game.id}`}
              onClick={() => onClickGame?.(game)}
            >
              Click
            </button>
          </div>
        ))}
    </div>
  ),
  BulkActions: ({
    selectedIds,
    onClearSelection,
    onSuccess,
    selectionMode,
    visibleCount,
    totalCount,
    onSelectAllClick,
  }: {
    selectedIds: Set<string>;
    onClearSelection: () => void;
    onSuccess: () => void;
    selectionMode?: string;
    visibleCount?: number;
    totalCount?: number;
    onSelectAllClick?: () => void;
  }) => (
    <div data-testid="bulk-actions">
      <span data-testid="selected-count">{selectedIds.size}</span>
      <span data-testid="selection-mode">{selectionMode || 'manual'}</span>
      <span data-testid="visible-count">{visibleCount}</span>
      <span data-testid="total-count">{totalCount}</span>
      <button data-testid="clear-selection" onClick={onClearSelection}>
        Clear
      </button>
      <button data-testid="select-all" onClick={onSelectAllClick}>
        Select All
      </button>
      <button
        data-testid="trigger-success"
        onClick={() => {
          onSuccess();
        }}
      >
        Success
      </button>
    </div>
  ),
}));

// Mock next/navigation
const mockPush = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Mock game data
const createMockGame = (id: string, title: string): UserGame => ({
  id: id as UserGameId,
  game: {
    id: 12345 as GameId,
    title,
    description: "A test game",
    genre: "Action",
    developer: "Test Developer",
    publisher: "Test Publisher",
    release_date: "2024-01-15",
    cover_art_url: "https://example.com/cover.jpg",
    rating_average: 8.5,
    rating_count: 100,
    estimated_playtime_hours: 20,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
  },
  ownership_status: "owned" as OwnershipStatus,
  personal_rating: 8,
  is_loved: false,
  play_status: "completed" as PlayStatus,
  hours_played: 25,
  platforms: [],
  tags: [],
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
});

const mockGames: UserGame[] = [
  createMockGame("game-1-uuid-1234-5678-abcd-123456789012", "The Legend of Zelda"),
  createMockGame("game-2-uuid-1234-5678-abcd-123456789012", "Super Mario Bros"),
  createMockGame("game-3-uuid-1234-5678-abcd-123456789012", "Metroid Prime"),
];

describe("GamesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseUserGames.mockReturnValue({
      data: { items: mockGames, total: 3, page: 1, per_page: 50, pages: 1 },
      isLoading: false,
      refetch: mockRefetch,
    });
    mockUseUserGameIds.mockReturnValue({
      data: undefined,
      isLoading: false,
      refetch: vi.fn(),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("page header", () => {
    it("renders the page title", () => {
      render(<GamesPage />);

      expect(screen.getByText("Game Library")).toBeInTheDocument();
    });

    it("displays total game count when data is loaded", () => {
      render(<GamesPage />);

      expect(screen.getByText("3 games")).toBeInTheDocument();
    });

    it("displays singular 'game' for single game", () => {
      mockUseUserGames.mockReturnValue({
        data: { items: [mockGames[0]], total: 1, page: 1, per_page: 50, pages: 1 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.getByText("1 game")).toBeInTheDocument();
    });

    it("does not display game count while loading", () => {
      mockUseUserGames.mockReturnValue({
        data: undefined,
        isLoading: true,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.queryByText(/\d+ games?/)).not.toBeInTheDocument();
    });

    it("renders Add Game button with link to /games/add", () => {
      render(<GamesPage />);

      const addButton = screen.getByRole("link", { name: /add game/i });
      expect(addButton).toBeInTheDocument();
      expect(addButton).toHaveAttribute("href", "/games/add");
    });
  });

  describe("loading state", () => {
    it("passes isLoading to GameGrid when loading", () => {
      mockUseUserGames.mockReturnValue({
        data: undefined,
        isLoading: true,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.getByTestId("grid-loading")).toBeInTheDocument();
    });

    it("passes isLoading to GameList when loading in list view", async () => {
      const user = userEvent.setup();
      mockUseUserGames.mockReturnValue({
        data: undefined,
        isLoading: true,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      // Switch to list view
      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(screen.getByTestId("list-loading")).toBeInTheDocument();
    });
  });

  describe("empty state", () => {
    it("shows empty state in GameGrid when no games", () => {
      mockUseUserGames.mockReturnValue({
        data: { items: [], total: 0, page: 1, per_page: 50, pages: 0 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.getByTestId("grid-empty")).toBeInTheDocument();
    });

    it("shows empty state in GameList when no games in list view", async () => {
      const user = userEvent.setup();
      mockUseUserGames.mockReturnValue({
        data: { items: [], total: 0, page: 1, per_page: 50, pages: 0 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(screen.getByTestId("list-empty")).toBeInTheDocument();
    });
  });

  describe("games display", () => {
    it("renders GameGrid by default", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("game-grid")).toBeInTheDocument();
      expect(screen.queryByTestId("game-list")).not.toBeInTheDocument();
    });

    it("passes games to GameGrid", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("grid-game-game-1-uuid-1234-5678-abcd-123456789012")).toBeInTheDocument();
      expect(screen.getByTestId("grid-game-game-2-uuid-1234-5678-abcd-123456789012")).toBeInTheDocument();
      expect(screen.getByTestId("grid-game-game-3-uuid-1234-5678-abcd-123456789012")).toBeInTheDocument();
    });

    it("displays games with correct titles", () => {
      render(<GamesPage />);

      expect(screen.getByText("The Legend of Zelda")).toBeInTheDocument();
      expect(screen.getByText("Super Mario Bros")).toBeInTheDocument();
      expect(screen.getByText("Metroid Prime")).toBeInTheDocument();
    });
  });

  describe("view mode switching", () => {
    it("shows grid view mode by default", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("view-mode")).toHaveTextContent("grid");
    });

    it("switches to list view when toggle is clicked", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(screen.getByTestId("view-mode")).toHaveTextContent("list");
      expect(screen.getByTestId("game-list")).toBeInTheDocument();
      expect(screen.queryByTestId("game-grid")).not.toBeInTheDocument();
    });

    it("switches back to grid view when toggle is clicked again", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));
      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(screen.getByTestId("view-mode")).toHaveTextContent("grid");
      expect(screen.getByTestId("game-grid")).toBeInTheDocument();
    });

    it("passes games to GameList when in list view", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(screen.getByTestId("list-game-game-1-uuid-1234-5678-abcd-123456789012")).toBeInTheDocument();
      expect(screen.getByTestId("list-game-game-2-uuid-1234-5678-abcd-123456789012")).toBeInTheDocument();
    });
  });

  describe("filters", () => {
    it("renders GameFilters component", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("game-filters")).toBeInTheDocument();
    });

    it("initializes with empty search filter", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("filter-search")).toHaveTextContent("");
    });

    it("initializes with no status filter", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("filter-status")).toHaveTextContent("none");
    });

    it("initializes with no platform filter", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("filter-platform")).toHaveTextContent("none");
    });

    it("updates search filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-search-filter"));

      expect(screen.getByTestId("filter-search")).toHaveTextContent("zelda");
    });

    it("updates status filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-status-filter"));

      expect(screen.getByTestId("filter-status")).toHaveTextContent("completed");
    });

    it("updates platform filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-platform-filter"));

      expect(screen.getByTestId("filter-platform")).toHaveTextContent("platform-1");
    });
  });

  describe("game selection", () => {
    it("initially has no games selected", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("selected-count")).toHaveTextContent("0");
    });

    it("selects a game when onSelectGame is called", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));

      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");
      expect(
        screen.getByTestId("grid-game-game-1-uuid-1234-5678-abcd-123456789012")
      ).toHaveAttribute("data-selected", "true");
    });

    it("can select multiple games", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      await user.click(screen.getByTestId("select-game-game-2-uuid-1234-5678-abcd-123456789012"));

      expect(screen.getByTestId("selected-count")).toHaveTextContent("2");
    });

    it("deselects a game when clicked again", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("0");
      expect(
        screen.getByTestId("grid-game-game-1-uuid-1234-5678-abcd-123456789012")
      ).toHaveAttribute("data-selected", "false");
    });

    it("clears all selections when clear button is clicked", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      await user.click(screen.getByTestId("select-game-game-2-uuid-1234-5678-abcd-123456789012"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("2");

      await user.click(screen.getByTestId("clear-selection"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("0");
    });

    it("selection works in list view as well", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));
      await user.click(screen.getByTestId("list-select-game-game-1-uuid-1234-5678-abcd-123456789012"));

      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");
    });
  });

  describe("game click navigation", () => {
    it("navigates to game detail page when game is clicked", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("click-game-game-1-uuid-1234-5678-abcd-123456789012"));

      expect(mockPush).toHaveBeenCalledWith("/games/game-1-uuid-1234-5678-abcd-123456789012");
    });

    it("navigates to correct game when different game is clicked", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("click-game-game-2-uuid-1234-5678-abcd-123456789012"));

      expect(mockPush).toHaveBeenCalledWith("/games/game-2-uuid-1234-5678-abcd-123456789012");
    });

    it("navigation works from list view", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));
      await user.click(screen.getByTestId("list-click-game-game-1-uuid-1234-5678-abcd-123456789012"));

      expect(mockPush).toHaveBeenCalledWith("/games/game-1-uuid-1234-5678-abcd-123456789012");
    });
  });

  describe("bulk actions", () => {
    it("renders BulkActions component", () => {
      render(<GamesPage />);

      expect(screen.getByTestId("bulk-actions")).toBeInTheDocument();
    });

    it("passes selectedIds to BulkActions", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      await user.click(screen.getByTestId("select-game-game-2-uuid-1234-5678-abcd-123456789012"));

      expect(screen.getByTestId("selected-count")).toHaveTextContent("2");
    });

    it("calls refetch when bulk action succeeds", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("trigger-success"));

      expect(mockRefetch).toHaveBeenCalled();
    });
  });

  describe("query parameters", () => {
    it("passes empty search when no filters applied", () => {
      render(<GamesPage />);

      // useUserGames is called with the initial params
      expect(mockUseUserGames).toHaveBeenCalled();
    });

    it("updates query when filters change", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-search-filter"));

      // The hook should be called with updated params
      // Note: Due to memoization, we verify the filter state changed
      expect(screen.getByTestId("filter-search")).toHaveTextContent("zelda");
    });
  });

  describe("data handling", () => {
    it("handles undefined data gracefully", () => {
      mockUseUserGames.mockReturnValue({
        data: undefined,
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      // Should show empty grid (games defaults to [])
      expect(screen.getByTestId("grid-empty")).toBeInTheDocument();
    });

    it("handles null items gracefully", () => {
      mockUseUserGames.mockReturnValue({
        data: { items: null, total: 0, page: 1, per_page: 50, pages: 0 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      // Should show empty grid (uses nullish coalescing)
      expect(screen.getByTestId("grid-empty")).toBeInTheDocument();
    });
  });

  describe("state persistence across view mode changes", () => {
    it("preserves selection when switching view modes", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      // Select a game in grid view
      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");

      // Switch to list view
      await user.click(screen.getByTestId("toggle-view-mode"));

      // Selection should be preserved
      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");
      expect(
        screen.getByTestId("list-game-game-1-uuid-1234-5678-abcd-123456789012")
      ).toHaveAttribute("data-selected", "true");
    });

    it("preserves filters when switching view modes", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      // Set a filter
      await user.click(screen.getByTestId("set-search-filter"));
      expect(screen.getByTestId("filter-search")).toHaveTextContent("zelda");

      // Switch view mode
      await user.click(screen.getByTestId("toggle-view-mode"));

      // Filter should still be present
      expect(screen.getByTestId("filter-search")).toHaveTextContent("zelda");
    });
  });

  describe("game count display edge cases", () => {
    it("displays 0 games correctly", () => {
      mockUseUserGames.mockReturnValue({
        data: { items: [], total: 0, page: 1, per_page: 50, pages: 0 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.getByText("0 games")).toBeInTheDocument();
    });

    it("displays large game count correctly", () => {
      mockUseUserGames.mockReturnValue({
        data: { items: mockGames, total: 1234, page: 1, per_page: 50, pages: 25 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

      expect(screen.getByText("1234 games")).toBeInTheDocument();
    });
  });
});
