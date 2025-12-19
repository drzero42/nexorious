import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import GameDetailPage from "./page";
import type {
  UserGame,
  PlayStatus,
  OwnershipStatus,
  GameId,
  UserGameId,
  UserGamePlatform,
} from "@/types";

// Mock useUserGame and useDeleteUserGame hooks
const mockMutateAsync = vi.fn();
const mockUseUserGame = vi.fn();
const mockUseDeleteUserGame = vi.fn();

vi.mock("@/hooks", () => ({
  useUserGame: (id: string) => mockUseUserGame(id),
  useUpdateUserGame: () => ({ mutateAsync: vi.fn() }),
  useDeleteUserGame: () => mockUseDeleteUserGame(),
}));

// Mock next/navigation
const mockPush = vi.fn();
const mockParams = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
  useParams: () => mockParams(),
}));

// Mock StarRating component
vi.mock("@/components/ui/star-rating", () => ({
  StarRating: ({
    value,
    readonly,
    size,
    showLabel,
  }: {
    value?: number | null;
    readonly?: boolean;
    size?: string;
    showLabel?: boolean;
  }) => (
    <div
      data-testid="star-rating"
      data-value={value ?? "null"}
      data-readonly={readonly}
      data-size={size}
      data-show-label={showLabel}
    >
      {value ? `${value}/10` : "No rating"}
    </div>
  ),
}));

// Mock lib/env config
vi.mock("@/lib/env", () => ({
  config: {
    staticUrl: "http://localhost:8000",
  },
}));

// Helper to create mock platform
const createMockPlatform = (overrides: { id: string; display_name: string; short_name: string }) => ({
  id: overrides.id,
  name: overrides.id,
  display_name: overrides.display_name,
  short_name: overrides.short_name,
  is_active: true,
  source: "official",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
});

// Helper to create mock storefront
const createMockStorefront = (overrides: { id: string; display_name: string; short_name: string }) => ({
  id: overrides.id,
  name: overrides.id,
  display_name: overrides.display_name,
  short_name: overrides.short_name,
  is_active: true,
  source: "official",
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
});

// Helper to create mock game data
const createMockUserGame = (overrides: Partial<UserGame> = {}): UserGame => ({
  id: "game-uuid-1234-5678-abcd-123456789012" as UserGameId,
  game: {
    id: 12345 as GameId,
    title: "The Legend of Zelda",
    description: "An epic adventure game",
    genre: "Action-Adventure",
    developer: "Nintendo",
    publisher: "Nintendo",
    release_date: "2023-05-12",
    cover_art_url: "https://example.com/cover.jpg",
    rating_average: 9.5,
    rating_count: 1000,
    estimated_playtime_hours: 50,
    howlongtobeat_main: 30,
    howlongtobeat_extra: 50,
    howlongtobeat_completionist: 100,
    igdb_slug: "the-legend-of-zelda",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    ...overrides.game,
  },
  ownership_status: "owned" as OwnershipStatus,
  personal_rating: 9,
  is_loved: true,
  play_status: "completed" as PlayStatus,
  hours_played: 45,
  personal_notes: "<p>Amazing game!</p>",
  acquired_date: "2023-06-01",
  platforms: [
    {
      id: "platform-1",
      platform_id: "nintendo-switch",
      platform: createMockPlatform({ id: "nintendo-switch", display_name: "Nintendo Switch", short_name: "Switch" }),
      storefront: createMockStorefront({ id: "eshop", display_name: "Nintendo eShop", short_name: "eShop" }),
      is_available: true,
      created_at: "2024-01-01T00:00:00Z",
    } as UserGamePlatform,
  ],
  tags: [],
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
  ...overrides,
});

describe("GameDetailPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockParams.mockReturnValue({ id: "game-uuid-1234-5678-abcd-123456789012" });
    mockUseDeleteUserGame.mockReturnValue({
      mutateAsync: mockMutateAsync,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("loading state", () => {
    it("displays skeleton loading state while fetching game data", () => {
      mockUseUserGame.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      });

      render(<GameDetailPage />);

      // Skeleton components have animate-pulse class
      const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
      expect(skeletons.length).toBeGreaterThan(0);
    });
  });

  describe("error state", () => {
    it("displays error message when game is not found", () => {
      mockUseUserGame.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error("Game not found"),
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Game not found")).toBeInTheDocument();
      expect(
        screen.getByText("The requested game could not be found in your collection.")
      ).toBeInTheDocument();
    });

    it("displays back to games button in error state", () => {
      mockUseUserGame.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error("Game not found"),
      });

      render(<GameDetailPage />);

      expect(screen.getByRole("button", { name: /back to games/i })).toBeInTheDocument();
    });

    it("navigates back to games list when back button is clicked in error state", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: new Error("Game not found"),
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /back to games/i }));

      expect(mockPush).toHaveBeenCalledWith("/games");
    });

    it("displays error state when data is null", () => {
      mockUseUserGame.mockReturnValue({
        data: null,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Game not found")).toBeInTheDocument();
    });
  });

  describe("header and navigation", () => {
    it("displays back to games button", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByRole("button", { name: /back to games/i })).toBeInTheDocument();
    });

    it("navigates back to games list when back button is clicked", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /back to games/i }));

      expect(mockPush).toHaveBeenCalledWith("/games");
    });

    it("displays edit button", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
    });

    it("navigates to edit page when edit button is clicked", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /edit/i }));

      expect(mockPush).toHaveBeenCalledWith("/games/game-uuid-1234-5678-abcd-123456789012/edit");
    });

    it("displays remove button", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByRole("button", { name: /remove/i })).toBeInTheDocument();
    });
  });

  describe("game information display", () => {
    it("displays game title", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByRole("heading", { name: "The Legend of Zelda" })).toBeInTheDocument();
    });

    it("displays developer when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      // Developer and publisher are both "Nintendo", so use getAllByText
      const nintendoElements = screen.getAllByText("Nintendo");
      expect(nintendoElements.length).toBeGreaterThanOrEqual(1);
    });

    it("displays publisher when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Publisher")).toBeInTheDocument();
      // Publisher value is "Nintendo" which is same as developer
    });

    it("displays genre when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Genre")).toBeInTheDocument();
      expect(screen.getByText("Action-Adventure")).toBeInTheDocument();
    });

    it("displays release date when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Release Date")).toBeInTheDocument();
    });

    it("displays description when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Description")).toBeInTheDocument();
      expect(screen.getByText("An epic adventure game")).toBeInTheDocument();
    });

    it("does not display description section when not available", () => {
      const mockGame = createMockUserGame();
      mockGame.game.description = undefined;
      mockUseUserGame.mockReturnValue({
        data: mockGame,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText("Description")).not.toBeInTheDocument();
    });
  });

  describe("cover art display", () => {
    it("displays cover art when URL is available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      const coverImage = screen.getByAltText("The Legend of Zelda");
      expect(coverImage).toBeInTheDocument();
      expect(coverImage).toHaveAttribute("src", "https://example.com/cover.jpg");
    });

    it("displays cover art with resolved local URL", () => {
      const mockGame = createMockUserGame();
      mockGame.game.cover_art_url = "/covers/zelda.jpg";
      mockUseUserGame.mockReturnValue({
        data: mockGame,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      const coverImage = screen.getByAltText("The Legend of Zelda");
      expect(coverImage).toHaveAttribute("src", "http://localhost:8000/covers/zelda.jpg");
    });

    it("displays placeholder when cover art is not available", () => {
      const mockGame = createMockUserGame();
      mockGame.game.cover_art_url = undefined;
      mockUseUserGame.mockReturnValue({
        data: mockGame,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("No Cover")).toBeInTheDocument();
    });
  });

  describe("love indicator", () => {
    it("displays heart icon when game is loved", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ is_loved: true }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      // Heart icon should be present (filled)
      const heartIcon = document.querySelector('[class*="text-red-500"]');
      expect(heartIcon).toBeInTheDocument();
    });

    it("does not display heart icon when game is not loved", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ is_loved: false }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      // Heart icon should not be present
      const heartIcon = document.querySelector('[class*="fill-red-500"]');
      expect(heartIcon).not.toBeInTheDocument();
    });
  });

  describe("play status display", () => {
    it.each([
      ["not_started", "Not Started"],
      ["in_progress", "In Progress"],
      ["completed", "Completed"],
      ["mastered", "Mastered"],
      ["dominated", "Dominated"],
      ["shelved", "Shelved"],
      ["dropped", "Dropped"],
      ["replay", "Replay"],
    ] as const)("displays %s status as %s", (status, label) => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ play_status: status as PlayStatus }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getAllByText(label).length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("ownership status display", () => {
    it.each([
      ["owned", "Owned"],
      ["borrowed", "Borrowed"],
      ["rented", "Rented"],
      ["subscription", "Subscription"],
      ["no_longer_owned", "No Longer Owned"],
    ] as const)("displays %s ownership as %s", (status, label) => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ ownership_status: status as OwnershipStatus }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getAllByText(label).length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("rating display", () => {
    it("displays star rating component with correct value", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ personal_rating: 9 }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      const starRatings = screen.getAllByTestId("star-rating");
      expect(starRatings.length).toBeGreaterThanOrEqual(1);
      expect(starRatings[0]).toHaveAttribute("data-value", "9");
    });

    it("displays star rating as readonly", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      const starRatings = screen.getAllByTestId("star-rating");
      expect(starRatings[0]).toHaveAttribute("data-readonly", "true");
    });
  });

  describe("platforms display", () => {
    it("displays platforms section when platforms are available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Platforms")).toBeInTheDocument();
      expect(screen.getByText("Nintendo Switch (Nintendo eShop)")).toBeInTheDocument();
    });

    it("displays multiple platforms", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({
          platforms: [
            {
              id: "platform-1",
              platform: createMockPlatform({ id: "switch", display_name: "Nintendo Switch", short_name: "Switch" }),
              storefront: createMockStorefront({ id: "eshop", display_name: "eShop", short_name: "eShop" }),
              is_available: true,
              created_at: "2024-01-01T00:00:00Z",
            } as UserGamePlatform,
            {
              id: "platform-2",
              platform: createMockPlatform({ id: "pc", display_name: "PC", short_name: "PC" }),
              storefront: createMockStorefront({ id: "steam", display_name: "Steam", short_name: "Steam" }),
              is_available: true,
              created_at: "2024-01-01T00:00:00Z",
            } as UserGamePlatform,
          ],
        }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Nintendo Switch (eShop)")).toBeInTheDocument();
      expect(screen.getByText("PC (Steam)")).toBeInTheDocument();
    });

    it("displays platform without storefront", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({
          platforms: [
            {
              id: "platform-1",
              platform: createMockPlatform({ id: "switch", display_name: "Nintendo Switch", short_name: "Switch" }),
              storefront: undefined,
              is_available: true,
              created_at: "2024-01-01T00:00:00Z",
            } as UserGamePlatform,
          ],
        }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Nintendo Switch")).toBeInTheDocument();
    });

    it("does not display platforms section when no platforms", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ platforms: [] }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText("Platforms")).not.toBeInTheDocument();
    });
  });

  describe("How Long to Beat display", () => {
    it("displays How Long to Beat section when data is available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("How Long to Beat")).toBeInTheDocument();
    });

    it("displays main story time", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Main Story")).toBeInTheDocument();
      expect(screen.getByText("30h")).toBeInTheDocument();
    });

    it("displays main + extra time", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Main + Extra")).toBeInTheDocument();
      expect(screen.getByText("50h")).toBeInTheDocument();
    });

    it("displays completionist time", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Completionist")).toBeInTheDocument();
      expect(screen.getByText("100h")).toBeInTheDocument();
    });

    it("does not display How Long to Beat section when no data", () => {
      const mockGame = createMockUserGame();
      mockGame.game.howlongtobeat_main = undefined;
      mockGame.game.howlongtobeat_extra = undefined;
      mockGame.game.howlongtobeat_completionist = undefined;
      mockUseUserGame.mockReturnValue({
        data: mockGame,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText("How Long to Beat")).not.toBeInTheDocument();
    });
  });

  describe("IGDB link display", () => {
    it("displays IGDB link when slug is available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("IGDB")).toBeInTheDocument();
      const igdbLink = screen.getByRole("link", { name: /view/i });
      expect(igdbLink).toHaveAttribute(
        "href",
        "https://www.igdb.com/games/the-legend-of-zelda"
      );
      expect(igdbLink).toHaveAttribute("target", "_blank");
      expect(igdbLink).toHaveAttribute("rel", "noopener noreferrer");
    });

    it("does not display IGDB link when slug is not available", () => {
      const mockGame = createMockUserGame();
      mockGame.game.igdb_slug = undefined;
      mockUseUserGame.mockReturnValue({
        data: mockGame,
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText("IGDB")).not.toBeInTheDocument();
    });
  });

  describe("personal information card", () => {
    it("displays Your Information section", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Your Information")).toBeInTheDocument();
    });

    it("displays hours played", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ hours_played: 45 }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Hours Played")).toBeInTheDocument();
      expect(screen.getByText("45h")).toBeInTheDocument();
    });

    it("displays 0h when hours played is 0 or undefined", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ hours_played: 0 }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("0h")).toBeInTheDocument();
    });

    it("displays acquired date when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ acquired_date: "2023-06-01" }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      // The date will be formatted by toLocaleDateString
      expect(screen.getByText(/Acquired:/)).toBeInTheDocument();
    });

    it("does not display acquired date when not available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ acquired_date: undefined }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText(/Acquired:/)).not.toBeInTheDocument();
    });

    it("displays personal notes when available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ personal_notes: "<p>Amazing game!</p>" }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.getByText("Personal Notes")).toBeInTheDocument();
      expect(screen.getByText("Amazing game!")).toBeInTheDocument();
    });

    it("does not display personal notes section when not available", () => {
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame({ personal_notes: undefined }),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      expect(screen.queryByText("Personal Notes")).not.toBeInTheDocument();
    });
  });

  describe("delete game functionality", () => {
    it("shows confirmation dialog when remove button is clicked", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /remove/i }));

      expect(screen.getByText("Remove from collection?")).toBeInTheDocument();
      // The dialog text uses typographic quotes which render as Unicode characters
      expect(
        screen.getByText(/Are you sure you want to remove.*The Legend of Zelda.*from your collection/i)
      ).toBeInTheDocument();
    });

    it("shows cancel button in confirmation dialog", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /remove/i }));

      expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument();
    });

    it("closes dialog when cancel is clicked", async () => {
      const user = userEvent.setup();
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /remove/i }));
      await user.click(screen.getByRole("button", { name: /cancel/i }));

      await waitFor(() => {
        expect(screen.queryByText("Remove from collection?")).not.toBeInTheDocument();
      });
    });

    it("calls deleteGame and navigates when confirmed", async () => {
      const user = userEvent.setup();
      mockMutateAsync.mockResolvedValue(undefined);
      mockUseUserGame.mockReturnValue({
        data: createMockUserGame(),
        isLoading: false,
        error: null,
      });

      render(<GameDetailPage />);

      await user.click(screen.getByRole("button", { name: /remove/i }));

      // Find the Remove button inside the dialog (AlertDialogAction)
      const dialogButtons = screen.getAllByRole("button", { name: /remove/i });
      const confirmButton = dialogButtons.find(
        (btn) => btn.closest('[role="alertdialog"]')
      );

      if (confirmButton) {
        await user.click(confirmButton);
      }

      await waitFor(() => {
        expect(mockMutateAsync).toHaveBeenCalledWith("game-uuid-1234-5678-abcd-123456789012");
      });

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith("/games");
      });
    });
  });

  describe("game ID from params", () => {
    it("passes correct game ID to useUserGame hook", () => {
      mockParams.mockReturnValue({ id: "different-game-id" });
      mockUseUserGame.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      });

      render(<GameDetailPage />);

      expect(mockUseUserGame).toHaveBeenCalledWith("different-game-id");
    });
  });
});
