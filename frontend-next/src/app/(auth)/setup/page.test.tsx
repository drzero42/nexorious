import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SetupPage from "./page";

// Mock auth API
const mockCheckSetupStatus = vi.fn();
const mockCreateInitialAdmin = vi.fn();

vi.mock("@/api/auth", () => ({
  checkSetupStatus: () => mockCheckSetupStatus(),
  createInitialAdmin: (username: string, password: string) =>
    mockCreateInitialAdmin(username, password),
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

describe("SetupPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("loading state", () => {
    it("shows loading state while checking setup status", () => {
      // Make the check never resolve to keep loading state
      mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));

      render(<SetupPage />);

      expect(screen.getByText("Checking setup status...")).toBeInTheDocument();
    });

    it("does not show the setup form while checking status", () => {
      mockCheckSetupStatus.mockImplementation(() => new Promise(() => {}));

      render(<SetupPage />);

      expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
      expect(screen.queryByLabelText("Confirm Password")).not.toBeInTheDocument();
    });
  });

  describe("setup not needed redirect", () => {
    it("redirects to /login when setup is not needed", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });

      render(<SetupPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });
    });

    it("renders nothing when redirecting to login", async () => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: false });

      const { container } = render(<SetupPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });

      // After redirect, should render nothing
      expect(container.firstChild).toBeNull();
    });
  });

  describe("setup check error", () => {
    it("renders nothing when setup check fails (component limitation)", async () => {
      // Note: The component has a limitation where errors from setup check
      // are stored in state but never displayed because the form only renders
      // when needsSetup is true, which is only set on successful check.
      mockCheckSetupStatus.mockRejectedValue(new Error("Network error"));

      const { container } = render(<SetupPage />);

      // Wait for the check to complete
      await waitFor(() => {
        // The component should have finished loading
        expect(screen.queryByText("Checking setup status...")).not.toBeInTheDocument();
      });

      // Due to component limitation, nothing is rendered when check fails
      // React renders null as an empty container (firstChild is null or empty)
      expect(container.firstChild).toBeNull();
    });
  });

  describe("setup form rendering", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("renders the setup form with all elements when setup is needed", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      // Check title and description are present (CardTitle renders as div, not heading)
      expect(screen.getAllByText("Create Admin Account")).toHaveLength(2); // Title and button
      expect(
        screen.getByText("Set up your administrator account to get started")
      ).toBeInTheDocument();
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
      expect(screen.getByLabelText("Password")).toBeInTheDocument();
      expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Create Admin Account" })
      ).toBeInTheDocument();
    });

    it("renders username input with correct attributes", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      const usernameInput = screen.getByLabelText("Username");
      expect(usernameInput).toHaveAttribute("type", "text");
      expect(usernameInput).toHaveAttribute("placeholder", "Choose a username");
      expect(usernameInput).toHaveAttribute("autocomplete", "username");
      expect(usernameInput).toBeRequired();
      expect(usernameInput).toHaveAttribute("minlength", "3");
    });

    it("renders password input with correct attributes", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Password")).toBeInTheDocument();
      });

      const passwordInput = screen.getByLabelText("Password");
      expect(passwordInput).toHaveAttribute("type", "password");
      expect(passwordInput).toHaveAttribute("placeholder", "Enter a secure password");
      expect(passwordInput).toHaveAttribute("autocomplete", "new-password");
      expect(passwordInput).toBeRequired();
      expect(passwordInput).toHaveAttribute("minlength", "8");
    });

    it("renders confirm password input with correct attributes", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
      });

      const confirmInput = screen.getByLabelText("Confirm Password");
      expect(confirmInput).toHaveAttribute("type", "password");
      expect(confirmInput).toHaveAttribute("placeholder", "Confirm your password");
      expect(confirmInput).toHaveAttribute("autocomplete", "new-password");
      expect(confirmInput).toBeRequired();
    });

    it("shows password requirement hints", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByText("Must be at least 3 characters")).toBeInTheDocument();
      });

      expect(screen.getByText("Must be at least 8 characters")).toBeInTheDocument();
    });

    it("shows admin privileges notice", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(
          screen.getByText("This account will have full administrative privileges")
        ).toBeInTheDocument();
      });
    });

    it("disables submit button when fields are empty", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: "Create Admin Account" })
        ).toBeInTheDocument();
      });

      const submitButton = screen.getByRole("button", { name: "Create Admin Account" });
      expect(submitButton).toBeDisabled();
    });

    it("disables submit button when only username is filled", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");

      const submitButton = screen.getByRole("button", { name: "Create Admin Account" });
      expect(submitButton).toBeDisabled();
    });

    it("disables submit button when only username and password are filled", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");

      const submitButton = screen.getByRole("button", { name: "Create Admin Account" });
      expect(submitButton).toBeDisabled();
    });

    it("enables submit button when all fields are filled", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");

      const submitButton = screen.getByRole("button", { name: "Create Admin Account" });
      expect(submitButton).toBeEnabled();
    });
  });

  describe("form interaction", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("updates username field on input", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      const usernameInput = screen.getByLabelText("Username");
      await user.type(usernameInput, "newadmin");

      expect(usernameInput).toHaveValue("newadmin");
    });

    it("updates password field on input", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Password")).toBeInTheDocument();
      });

      const passwordInput = screen.getByLabelText("Password");
      await user.type(passwordInput, "securepassword");

      expect(passwordInput).toHaveValue("securepassword");
    });

    it("updates confirm password field on input", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
      });

      const confirmInput = screen.getByLabelText("Confirm Password");
      await user.type(confirmInput, "securepassword");

      expect(confirmInput).toHaveValue("securepassword");
    });
  });

  describe("form validation", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("shows error when username is too short", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "ab");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(
          screen.getByText("Username must be at least 3 characters")
        ).toBeInTheDocument();
      });

      expect(mockCreateInitialAdmin).not.toHaveBeenCalled();
    });

    it("shows error when password is too short", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "short");
      await user.type(screen.getByLabelText("Confirm Password"), "short");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(
          screen.getByText("Password must be at least 8 characters")
        ).toBeInTheDocument();
      });

      expect(mockCreateInitialAdmin).not.toHaveBeenCalled();
    });

    it("shows error when passwords do not match", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "differentpassword");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(screen.getByText("Passwords do not match")).toBeInTheDocument();
      });

      expect(mockCreateInitialAdmin).not.toHaveBeenCalled();
    });

    it("clears previous error when submitting again", async () => {
      const user = userEvent.setup();
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      // First submission with mismatched passwords
      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "differentpassword");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(screen.getByText("Passwords do not match")).toBeInTheDocument();
      });

      // Clear and fix passwords
      await user.clear(screen.getByLabelText("Confirm Password"));
      await user.type(screen.getByLabelText("Confirm Password"), "password123");

      mockCreateInitialAdmin.mockResolvedValue({ id: "1", username: "admin", isAdmin: true });
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      // Error should be cleared and API called
      await waitFor(() => {
        expect(screen.queryByText("Passwords do not match")).not.toBeInTheDocument();
      });
    });
  });

  describe("form submission", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("calls createInitialAdmin with username and password on valid submit", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockResolvedValue({
        id: "1",
        username: "admin",
        isAdmin: true,
      });

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(mockCreateInitialAdmin).toHaveBeenCalledWith("admin", "password123");
      });
    });

    it("shows creating account state while submitting", async () => {
      const user = userEvent.setup();
      // Make create never resolve to keep submitting state
      mockCreateInitialAdmin.mockImplementation(() => new Promise(() => {}));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      expect(
        screen.getByRole("button", { name: "Creating Account..." })
      ).toBeInTheDocument();
    });

    it("disables inputs while submitting", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockImplementation(() => new Promise(() => {}));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      expect(screen.getByLabelText("Username")).toBeDisabled();
      expect(screen.getByLabelText("Password")).toBeDisabled();
      expect(screen.getByLabelText("Confirm Password")).toBeDisabled();
    });

    it("disables submit button while submitting", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockImplementation(() => new Promise(() => {}));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      expect(screen.getByRole("button", { name: "Creating Account..." })).toBeDisabled();
    });

    it("navigates to /login on successful account creation", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockResolvedValue({
        id: "1",
        username: "admin",
        isAdmin: true,
      });

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(mockPush).toHaveBeenCalledWith("/login");
      });
    });

    it("shows error message when account creation fails", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockRejectedValue(new Error("Username already exists"));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(screen.getByText("Username already exists")).toBeInTheDocument();
      });
    });

    it("shows generic error when account creation fails with non-Error", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockRejectedValue("Some string error");

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(screen.getByText("Failed to create admin account")).toBeInTheDocument();
      });
    });

    it("re-enables form after failed account creation", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockRejectedValue(new Error("Server error"));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeEnabled();
        expect(screen.getByLabelText("Password")).toBeEnabled();
        expect(screen.getByLabelText("Confirm Password")).toBeEnabled();
        expect(
          screen.getByRole("button", { name: "Create Admin Account" })
        ).toBeEnabled();
      });
    });
  });

  describe("error display", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("displays error in an alert", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockRejectedValue(new Error("Something went wrong"));

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123");
      await user.click(screen.getByRole("button", { name: "Create Admin Account" }));

      await waitFor(() => {
        const alert = screen.getByRole("alert");
        expect(alert).toBeInTheDocument();
        expect(alert).toHaveTextContent("Something went wrong");
      });
    });

    it("does not display error alert when no error", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("has accessible form structure", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      // Labels are properly associated with inputs
      expect(screen.getByLabelText("Username")).toBeInTheDocument();
      expect(screen.getByLabelText("Password")).toBeInTheDocument();
      expect(screen.getByLabelText("Confirm Password")).toBeInTheDocument();
    });

    it("focuses username input when setup is needed", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      // Allow focus to be set in the useEffect
      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toHaveFocus();
      });
    });

    it("form is contained in a card with title and description", async () => {
      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      // CardTitle renders as div, not heading - check both title and description
      expect(screen.getAllByText("Create Admin Account")).toHaveLength(2); // Title and button
      expect(
        screen.getByText("Set up your administrator account to get started")
      ).toBeInTheDocument();
    });
  });

  describe("form submission via keyboard", () => {
    beforeEach(() => {
      mockCheckSetupStatus.mockResolvedValue({ needs_setup: true });
    });

    it("submits form when pressing Enter in confirm password field", async () => {
      const user = userEvent.setup();
      mockCreateInitialAdmin.mockResolvedValue({
        id: "1",
        username: "admin",
        isAdmin: true,
      });

      render(<SetupPage />);

      await waitFor(() => {
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });

      await user.type(screen.getByLabelText("Username"), "admin");
      await user.type(screen.getByLabelText("Password"), "password123");
      await user.type(screen.getByLabelText("Confirm Password"), "password123{enter}");

      await waitFor(() => {
        expect(mockCreateInitialAdmin).toHaveBeenCalledWith("admin", "password123");
      });
    });
  });

  describe("state transitions", () => {
    it("transitions from loading to form when setup is needed", async () => {
      let resolveSetupCheck: (value: { needs_setup: boolean }) => void;
      mockCheckSetupStatus.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveSetupCheck = resolve;
          })
      );

      render(<SetupPage />);

      expect(screen.getByText("Checking setup status...")).toBeInTheDocument();
      expect(screen.queryByLabelText("Username")).not.toBeInTheDocument();

      // Resolve the setup check
      resolveSetupCheck!({ needs_setup: true });

      await waitFor(() => {
        expect(screen.queryByText("Checking setup status...")).not.toBeInTheDocument();
        expect(screen.getByLabelText("Username")).toBeInTheDocument();
      });
    });

    it("transitions from loading to redirect when setup is not needed", async () => {
      let resolveSetupCheck: (value: { needs_setup: boolean }) => void;
      mockCheckSetupStatus.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveSetupCheck = resolve;
          })
      );

      render(<SetupPage />);

      expect(screen.getByText("Checking setup status...")).toBeInTheDocument();

      // Resolve the setup check indicating no setup needed
      resolveSetupCheck!({ needs_setup: false });

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith("/login");
      });
    });
  });
});
