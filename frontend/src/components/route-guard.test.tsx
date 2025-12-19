import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { RouteGuard } from "./route-guard";

// Mock the useAuth hook
const mockUseAuth = vi.fn();
vi.mock("@/providers", () => ({
  useAuth: () => mockUseAuth(),
}));

// Mock next/navigation with a spy for router.replace
const mockReplace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mockReplace,
    push: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Mock auth API
const mockCheckSetupStatus = vi.fn();
vi.mock("@/api/auth", () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
}));

describe("RouteGuard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: setup complete
    mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("loading state", () => {
    it("shows loading spinner while checking setup status", () => {
      mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      const { container } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should show loading spinner
      expect(container.querySelector(".animate-spin")).toBeInTheDocument();
      expect(screen.queryByTestId("children")).not.toBeInTheDocument();
    });

    it("shows loading spinner while auth is loading", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: true,
        isAuthenticated: false,
      });

      const { container } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(container.querySelector(".animate-spin")).toBeInTheDocument();
      });
      expect(screen.queryByTestId("children")).not.toBeInTheDocument();
    });

    it("does not redirect while loading", () => {
      mockUseAuth.mockReturnValue({
        isLoading: true,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      expect(mockReplace).not.toHaveBeenCalled();
    });
  });

  describe("setup status checks", () => {
    it("redirects to /setup when setup is needed", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/setup");
      });
    });

    it("does not render children when setup is needed", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/setup");
      });

      expect(screen.queryByTestId("children")).not.toBeInTheDocument();
    });

    it("proceeds to auth check when setup is not needed", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(screen.getByTestId("children")).toBeInTheDocument();
      });

      expect(mockReplace).not.toHaveBeenCalled();
    });

    it("continues to login redirect if setup check fails", async () => {
      mockCheckSetupStatus.mockRejectedValue(new Error("Network error"));
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should fall through to auth check and redirect to login
      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });
    });
  });

  describe("unauthenticated state", () => {
    it("redirects to login when not authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });
    });

    it("renders nothing while redirecting", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Wait for setup check to complete
      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });

      // Should not render children
      expect(screen.queryByTestId("children")).not.toBeInTheDocument();
    });
  });

  describe("authenticated state", () => {
    it("renders children when authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(screen.getByTestId("children")).toBeInTheDocument();
      });
      expect(screen.getByText("Protected Content")).toBeInTheDocument();
    });

    it("does not redirect when authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      // Wait for setup check to complete
      await waitFor(() => {
        expect(mockReplace).not.toHaveBeenCalled();
      });
    });

    it("does not show loading spinner when authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>
      );

      await waitFor(() => {
        const spinner = document.querySelector(".animate-spin");
        expect(spinner).not.toBeInTheDocument();
      });
    });
  });

  describe("state transitions", () => {
    it("transitions from loading to authenticated", async () => {
      // Start in loading state
      mockUseAuth.mockReturnValue({
        isLoading: true,
        isAuthenticated: false,
      });

      const { rerender } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should show loading spinner initially
      expect(document.querySelector(".animate-spin")).toBeInTheDocument();
      expect(screen.queryByTestId("children")).not.toBeInTheDocument();

      // Transition to authenticated
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      rerender(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should now show children
      await waitFor(() => {
        expect(screen.getByTestId("children")).toBeInTheDocument();
      });
      expect(document.querySelector(".animate-spin")).not.toBeInTheDocument();
    });

    it("transitions from loading to unauthenticated", async () => {
      // Start in loading state
      mockUseAuth.mockReturnValue({
        isLoading: true,
        isAuthenticated: false,
      });

      const { rerender } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should show loading spinner initially
      expect(document.querySelector(".animate-spin")).toBeInTheDocument();

      // Transition to unauthenticated
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: false,
      });

      rerender(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>
      );

      // Should redirect to login
      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });
    });
  });

  describe("children rendering", () => {
    it("renders multiple children when authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div data-testid="child1">Child 1</div>
          <div data-testid="child2">Child 2</div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(screen.getByTestId("child1")).toBeInTheDocument();
      });
      expect(screen.getByTestId("child2")).toBeInTheDocument();
    });

    it("renders nested components when authenticated", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });
      mockUseAuth.mockReturnValue({
        isLoading: false,
        isAuthenticated: true,
      });

      render(
        <RouteGuard>
          <div data-testid="parent">
            <div data-testid="nested">Nested Content</div>
          </div>
        </RouteGuard>
      );

      await waitFor(() => {
        expect(screen.getByTestId("parent")).toBeInTheDocument();
      });
      expect(screen.getByTestId("nested")).toBeInTheDocument();
    });
  });
});
