import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import LoginPage from "./page";

// Mock useAuth hook
const mockLogin = vi.fn();
const mockClearError = vi.fn();
const mockUseAuth = vi.fn();

vi.mock("@/providers", () => ({
  useAuth: () => mockUseAuth(),
}));

// Mock next/navigation
const mockPush = vi.fn();
const mockReplace = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
    prefetch: vi.fn(),
  }),
}));

describe("LoginPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      login: mockLogin,
      isAuthenticated: false,
      isLoading: false,
      error: null,
      clearError: mockClearError,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("loading state", () => {
    it("shows loading state while auth is loading", () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: true,
        error: null,
        clearError: mockClearError,
      });

      render(<LoginPage />);

      expect(screen.getByText("Loading...")).toBeInTheDocument();
      expect(screen.queryByRole("form")).not.toBeInTheDocument();
    });

    it("does not show the login form while loading", () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: true,
        error: null,
        clearError: mockClearError,
      });

      render(<LoginPage />);

      expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
    });
  });

  describe("authenticated redirect", () => {
    it("redirects to /games when already authenticated", async () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        clearError: mockClearError,
      });

      render(<LoginPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/games");
      });
    });

    it("renders nothing when authenticated (redirect in progress)", () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        clearError: mockClearError,
      });

      const { container } = render(<LoginPage />);

      expect(container.firstChild).toBeNull();
    });
  });

  describe("login form rendering", () => {
    it("renders the login form with all elements", () => {
      render(<LoginPage />);

      expect(screen.getByText("Nexorious")).toBeInTheDocument();
      expect(screen.getByText("Sign in to your account to continue")).toBeInTheDocument();
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
      expect(screen.getByLabelText("Password")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
    });

    it("renders username input with correct attributes", () => {
      render(<LoginPage />);

      const usernameInput = screen.getByLabelText("Username");
      expect(usernameInput).toHaveAttribute("type", "text");
      expect(usernameInput).toHaveAttribute("placeholder", "Enter your username");
      expect(usernameInput).toHaveAttribute("autocomplete", "username");
      expect(usernameInput).toBeRequired();
    });

    it("renders password input with correct attributes", () => {
      render(<LoginPage />);

      const passwordInput = screen.getByLabelText("Password");
      expect(passwordInput).toHaveAttribute("type", "password");
      expect(passwordInput).toHaveAttribute("placeholder", "Enter your password");
      expect(passwordInput).toHaveAttribute("autocomplete", "current-password");
      expect(passwordInput).toBeRequired();
    });

    it("disables submit button when fields are empty", () => {
      render(<LoginPage />);

      const submitButton = screen.getByRole("button", { name: "Sign in" });
      expect(submitButton).toBeDisabled();
    });

    it("disables submit button when only username is filled", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");

      const submitButton = screen.getByRole("button", { name: "Sign in" });
      expect(submitButton).toBeDisabled();
    });

    it("disables submit button when only password is filled", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);

      await user.type(screen.getByLabelText("Password"), "password123");

      const submitButton = screen.getByRole("button", { name: "Sign in" });
      expect(submitButton).toBeDisabled();
    });

    it("enables submit button when both fields are filled", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");

      const submitButton = screen.getByRole("button", { name: "Sign in" });
      expect(submitButton).toBeEnabled();
    });
  });

  describe("form interaction", () => {
    it("updates username field on input", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);

      const usernameInput = screen.getByLabelText("Username");
      await user.type(usernameInput, "testuser");

      expect(usernameInput).toHaveValue("testuser");
    });

    it("updates password field on input", async () => {
      const user = userEvent.setup();
      render(<LoginPage />);

      const passwordInput = screen.getByLabelText("Password");
      await user.type(passwordInput, "password123");

      expect(passwordInput).toHaveValue("password123");
    });
  });

  describe("form submission", () => {
    it("calls login with username and password on submit", async () => {
      const user = userEvent.setup();
      mockLogin.mockResolvedValue(undefined);

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      expect(mockLogin).toHaveBeenCalledWith("testuser", "password123");
    });

    it("clears error before login attempt", async () => {
      const user = userEvent.setup();
      mockLogin.mockResolvedValue(undefined);

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      expect(mockClearError).toHaveBeenCalled();
    });

    it("shows signing in state while submitting", async () => {
      const user = userEvent.setup();
      // Make login never resolve to keep submitting state
      mockLogin.mockImplementation(() => new Promise(() => {}));

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      expect(screen.getByRole("button", { name: "Signing in..." })).toBeInTheDocument();
    });

    it("disables inputs while submitting", async () => {
      const user = userEvent.setup();
      // Make login never resolve to keep submitting state
      mockLogin.mockImplementation(() => new Promise(() => {}));

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      expect(screen.getByLabelText("Username")).toBeDisabled();
      expect(screen.getByLabelText("Password")).toBeDisabled();
    });

    it("disables submit button while submitting", async () => {
      const user = userEvent.setup();
      // Make login never resolve to keep submitting state
      mockLogin.mockImplementation(() => new Promise(() => {}));

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      expect(screen.getByRole("button", { name: "Signing in..." })).toBeDisabled();
    });

    it("navigates to /games on successful login", async () => {
      const user = userEvent.setup();
      mockLogin.mockResolvedValue(undefined);

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith("/games");
      });
    });

    it("re-enables form after failed login", async () => {
      const user = userEvent.setup();
      mockLogin.mockRejectedValue(new Error("Invalid credentials"));

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "wrongpassword");
      await user.click(screen.getByRole("button", { name: "Sign in" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeEnabled();
        expect(screen.getByLabelText("Password")).toBeEnabled();
        expect(screen.getByRole("button", { name: "Sign in" })).toBeEnabled();
      });
    });
  });

  describe("error display", () => {
    it("displays error message when error exists", () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: false,
        error: "Invalid credentials",
        clearError: mockClearError,
      });

      render(<LoginPage />);

      expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
    });

    it("displays error in an alert", () => {
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: false,
        error: "Something went wrong",
        clearError: mockClearError,
      });

      render(<LoginPage />);

      const alert = screen.getByRole("alert");
      expect(alert).toBeInTheDocument();
      expect(alert).toHaveTextContent("Something went wrong");
    });

    it("does not display error alert when no error", () => {
      render(<LoginPage />);

      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    it("has accessible form structure", () => {
      render(<LoginPage />);

      // Labels are properly associated with inputs
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
      expect(screen.getByLabelText("Password")).toBeInTheDocument();
    });

    it("username input has autofocus", () => {
      render(<LoginPage />);

      const usernameInput = screen.getByLabelText("Username");
      // React renders autoFocus as the DOM attribute autofocus=""
      expect(usernameInput).toHaveFocus();
    });

    it("form is contained in a card", () => {
      render(<LoginPage />);

      // The card should have the title and description
      expect(screen.getByText("Nexorious")).toBeInTheDocument();
      expect(screen.getByText("Sign in to your account to continue")).toBeInTheDocument();
    });
  });

  describe("form submission via keyboard", () => {
    it("submits form when pressing Enter in password field", async () => {
      const user = userEvent.setup();
      mockLogin.mockResolvedValue(undefined);

      render(<LoginPage />);

      await user.type(screen.getByLabelText("Username"), "testuser");
      await user.type(screen.getByLabelText("Password"), "password123{enter}");

      expect(mockLogin).toHaveBeenCalledWith("testuser", "password123");
    });
  });

  describe("state transitions", () => {
    it("transitions from loading to form", async () => {
      // Start in loading state
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: true,
        error: null,
        clearError: mockClearError,
      });

      const { rerender } = render(<LoginPage />);

      expect(screen.getByText("Loading...")).toBeInTheDocument();
      expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();

      // Transition to not loading
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        clearError: mockClearError,
      });

      rerender(<LoginPage />);

      expect(screen.queryByText("Loading...")).not.toBeInTheDocument();
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
    });

    it("transitions from form to redirect when authenticated", async () => {
      // Start unauthenticated
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        clearError: mockClearError,
      });

      const { rerender } = render(<LoginPage />);

      expect(screen.getByLabelText("Username")).toBeInTheDocument();

      // Become authenticated
      mockUseAuth.mockReturnValue({
        login: mockLogin,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        clearError: mockClearError,
      });

      rerender(<LoginPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/games");
      });
    });
  });
});
