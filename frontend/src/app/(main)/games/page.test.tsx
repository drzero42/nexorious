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
    filters: { search: string; status?: PlayStatus; platforms?: string[]; storefronts?: string[]; genres?: string[]; tags?: string[] };
    onFiltersChange: (filters: { search: string; status?: PlayStatus; platforms?: string[]; storefronts?: string[]; genres?: string[]; tags?: string[] }) => void;
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
        <span data-testid="filter-platforms">{(filters.platforms ?? []).join(",") || "none"}</span>
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
          onClick={() => onFiltersChange({ ...filters, platforms: ["windows"] })}
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
const mockReplace = vi.fn();
let mockSearchParams = new URLSearchParams();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
    prefetch: vi.fn(),
  }),
  useSearchParams: () => mockSearchParams,
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
    mockSearchParams = new URLSearchParams(); // Reset URL params
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

    it("passes isLoading to GameList when loading in list view", () => {
      // Set URL to list view
      mockSearchParams = new URLSearchParams("view=list");
      mockUseUserGames.mockReturnValue({
        data: undefined,
        isLoading: true,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

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

    it("shows empty state in GameList when no games in list view", () => {
      // Set URL to list view
      mockSearchParams = new URLSearchParams("view=list");
      mockUseUserGames.mockReturnValue({
        data: { items: [], total: 0, page: 1, per_page: 50, pages: 0 },
        isLoading: false,
        refetch: mockRefetch,
      });

      render(<GamesPage />);

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

    it("shows list view when URL has view=list", () => {
      mockSearchParams = new URLSearchParams("view=list");
      render(<GamesPage />);

      expect(screen.getByTestId("view-mode")).toHaveTextContent("list");
      expect(screen.getByTestId("game-list")).toBeInTheDocument();
      expect(screen.queryByTestId("game-grid")).not.toBeInTheDocument();
    });

    it("updates URL to list view when toggle is clicked", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));

      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("view=list"),
        { scroll: false }
      );
    });

    it("updates URL to remove view param when toggling back to grid", async () => {
      // Start with list view
      mockSearchParams = new URLSearchParams("view=list");
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("toggle-view-mode"));

      // Grid is default, so view param should be removed (URL should be /games or empty)
      expect(mockReplace).toHaveBeenCalledWith("/games", { scroll: false });
    });

    it("passes games to GameList when URL has view=list", () => {
      mockSearchParams = new URLSearchParams("view=list");
      render(<GamesPage />);

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

      expect(screen.getByTestId("filter-platforms")).toHaveTextContent("none");
    });

    it("updates search filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-search-filter"));

      // Search filter now updates URL, verify replace was called with the search param
      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("q=zelda"),
        { scroll: false }
      );
    });

    it("updates status filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-status-filter"));

      // Status filter now updates URL, verify replace was called with the status param
      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("status=completed"),
        { scroll: false }
      );
    });

    it("updates platform filter when changed", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-platform-filter"));

      // Platform filter now updates URL, verify replace was called with the platform param
      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("platform=windows"),
        { scroll: false }
      );
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
      // Set URL to list view
      mockSearchParams = new URLSearchParams("view=list");
      const user = userEvent.setup();
      render(<GamesPage />);

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
      // Set URL to list view
      mockSearchParams = new URLSearchParams("view=list");
      const user = userEvent.setup();
      render(<GamesPage />);

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
    it("passes empty search when no URL params are set", () => {
      render(<GamesPage />);

      // useUserGames is called with the initial params
      expect(mockUseUserGames).toHaveBeenCalled();
    });

    it("reads filters from URL params", () => {
      // Set URL params before rendering
      mockSearchParams = new URLSearchParams("q=zelda&status=completed&platform=windows");
      render(<GamesPage />);

      // Verify filters are read from URL
      expect(screen.getByTestId("filter-search")).toHaveTextContent("zelda");
      expect(screen.getByTestId("filter-status")).toHaveTextContent("completed");
      expect(screen.getByTestId("filter-platforms")).toHaveTextContent("windows");
    });

    it("reads view mode from URL params", () => {
      // Set URL params for list view
      mockSearchParams = new URLSearchParams("view=list");
      render(<GamesPage />);

      // Verify list view is displayed
      expect(screen.getByTestId("view-mode")).toHaveTextContent("list");
      expect(screen.getByTestId("game-list")).toBeInTheDocument();
    });

    it("reads sort settings from URL params", () => {
      // Set URL params for sort
      mockSearchParams = new URLSearchParams("sort=created_at&order=desc");
      render(<GamesPage />);

      // Verify useUserGames is called (sort params are passed through)
      expect(mockUseUserGames).toHaveBeenCalled();
    });

    it("updates URL when filters change", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      await user.click(screen.getByTestId("set-search-filter"));

      // Verify router.replace was called with the new search param
      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("q=zelda"),
        { scroll: false }
      );
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
    it("selection state is transient (not in URL)", async () => {
      // Selection is stored in useState, not URL, so it's not persisted across URL changes
      const user = userEvent.setup();
      render(<GamesPage />);

      // Select a game in grid view
      await user.click(screen.getByTestId("select-game-game-1-uuid-1234-5678-abcd-123456789012"));
      expect(screen.getByTestId("selected-count")).toHaveTextContent("1");

      // Verify the selection is maintained within the same render
      expect(screen.getByTestId("grid-game-game-1-uuid-1234-5678-abcd-123456789012")).toHaveAttribute("data-selected", "true");
    });

    it("updates URL when view mode changes", async () => {
      const user = userEvent.setup();
      render(<GamesPage />);

      // Switch view mode to list
      await user.click(screen.getByTestId("toggle-view-mode"));

      // View mode updates URL (list mode is stored, grid is default and not stored)
      expect(mockReplace).toHaveBeenCalledWith(
        expect.stringContaining("view=list"),
        { scroll: false }
      );
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
