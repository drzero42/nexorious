import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@/test/test-utils";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { server } from "@/test/mocks/server";
import { AuthProvider, useAuth } from "./auth-provider";
import { localStorageMock } from "@/test/setup";
import type { User, LoginResponse } from "@/types";

// In test environment, NODE_ENV is 'test' so apiUrl defaults to '/api'
// MSW intercepts relative URLs as absolute URLs with the origin
const API_URL = "/api";

// Mock user data
const mockUser: User = {
  id: "test-user-id",
  username: "testuser",
  isAdmin: false,
};

const mockApiUser = {
  id: "test-user-id",
  username: "testuser",
  is_admin: false,
};

const mockTokens: LoginResponse = {
  access_token: "test-access-token",
  refresh_token: "test-refresh-token",
  token_type: "bearer",
  expires_in: 3600,
};

// Test component that uses the auth context
function TestConsumer() {
  const auth = useAuth();

  const handleLogin = async () => {
    try {
      await auth.login("testuser", "password123");
    } catch {
      // Error is set in context, no need to handle here
    }
  };

  return (
    <div>
      <div data-testid="loading">{auth.isLoading ? "loading" : "not-loading"}</div>
      <div data-testid="authenticated">{auth.isAuthenticated ? "authenticated" : "not-authenticated"}</div>
      <div data-testid="user">{auth.user?.username ?? "no-user"}</div>
      <div data-testid="error">{auth.error ?? "no-error"}</div>
      <button onClick={handleLogin}>Login</button>
      <button onClick={() => auth.logout()}>Logout</button>
      <button onClick={() => auth.clearError()}>Clear Error</button>
    </div>
  );
}

// Test component that throws when used outside provider
function TestConsumerWithoutProvider() {
  try {
    useAuth();
    return <div>Should have thrown</div>;
  } catch (error) {
    return <div data-testid="error-thrown">{(error as Error).message}</div>;
  }
}

describe("AuthProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorageMock.getItem.mockReturnValue(null);
    localStorageMock.setItem.mockImplementation(() => {});
    localStorageMock.removeItem.mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("initial state", () => {
    it("renders children and provides auth context", async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
      expect(screen.getByTestId("user")).toHaveTextContent("no-user");
    });

    it("starts in loading state", () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      // Initially should be loading
      expect(screen.getByTestId("loading")).toHaveTextContent("loading");
    });
  });

  describe("useAuth hook", () => {
    it("throws error when used outside AuthProvider", () => {
      // Suppress console.error for this test since we expect an error
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      render(<TestConsumerWithoutProvider />);

      expect(screen.getByTestId("error-thrown")).toHaveTextContent(
        "useAuth must be used within an AuthProvider"
      );

      consoleSpy.mockRestore();
    });
  });

  describe("localStorage initialization", () => {
    it("restores auth state from localStorage on mount", async () => {
      const storedAuth = {
        accessToken: "stored-access-token",
        refreshToken: "stored-refresh-token",
        user: mockUser,
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(storedAuth));

      // Set up handler for getMe validation
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      expect(screen.getByTestId("user")).toHaveTextContent("testuser");
    });

    it("clears invalid stored auth when getMe fails", async () => {
      const storedAuth = {
        accessToken: "invalid-token",
        refreshToken: "invalid-refresh-token",
        user: mockUser,
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(storedAuth));

      // Set up handler that returns 401 for both getMe and refresh
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: "Invalid token" }, { status: 401 });
        }),
        http.post(`${API_URL}/auth/refresh`, () => {
          return HttpResponse.json({ detail: "Invalid refresh token" }, { status: 401 });
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
      expect(localStorageMock.removeItem).toHaveBeenCalledWith("auth");
    });

    it("handles invalid JSON in localStorage", async () => {
      localStorageMock.getItem.mockReturnValue("invalid-json{");

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
      expect(localStorageMock.removeItem).toHaveBeenCalledWith("auth");
    });

    it("handles incomplete stored auth data", async () => {
      // Missing required fields
      const incompleteAuth = {
        accessToken: "test-token",
        // missing refreshToken and user
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(incompleteAuth));

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
    });
  });

  describe("login", () => {
    it("successfully logs in user", async () => {
      const user = userEvent.setup();

      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json(mockTokens);
        }),
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      await user.click(screen.getByText("Login"));

      await waitFor(() => {
        expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      });

      expect(screen.getByTestId("user")).toHaveTextContent("testuser");
      expect(localStorageMock.setItem).toHaveBeenCalledWith(
        "auth",
        expect.stringContaining("test-access-token")
      );
    });

    it("sets error state on login failure", async () => {
      const user = userEvent.setup();

      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: "Invalid credentials" }, { status: 401 });
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      await user.click(screen.getByText("Login"));

      await waitFor(() => {
        expect(screen.getByTestId("error")).toHaveTextContent("Invalid credentials");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
    });

    it("clears error state on successful login after failure", async () => {
      const user = userEvent.setup();
      let loginAttempts = 0;

      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          loginAttempts++;
          if (loginAttempts === 1) {
            return HttpResponse.json({ detail: "Invalid credentials" }, { status: 401 });
          }
          return HttpResponse.json(mockTokens);
        }),
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      // First login attempt fails
      await user.click(screen.getByText("Login"));

      await waitFor(() => {
        expect(screen.getByTestId("error")).toHaveTextContent("Invalid credentials");
      });

      // Second login attempt succeeds
      await user.click(screen.getByText("Login"));

      await waitFor(() => {
        expect(screen.getByTestId("error")).toHaveTextContent("no-error");
        expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      });
    });
  });

  describe("logout", () => {
    it("clears auth state and removes from localStorage", async () => {
      const user = userEvent.setup();

      const storedAuth = {
        accessToken: "stored-access-token",
        refreshToken: "stored-refresh-token",
        user: mockUser,
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(storedAuth));

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      });

      await user.click(screen.getByText("Logout"));

      await waitFor(() => {
        expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
      });

      expect(localStorageMock.removeItem).toHaveBeenCalledWith("auth");
      expect(screen.getByTestId("user")).toHaveTextContent("no-user");
      expect(screen.getByTestId("error")).toHaveTextContent("no-error");
    });
  });

  describe("clearError", () => {
    it("clears the error state", async () => {
      const user = userEvent.setup();

      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: "Login failed" }, { status: 401 });
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      // Trigger an error
      await user.click(screen.getByText("Login"));

      await waitFor(() => {
        expect(screen.getByTestId("error")).toHaveTextContent("Login failed");
      });

      // Clear the error
      await user.click(screen.getByText("Clear Error"));

      expect(screen.getByTestId("error")).toHaveTextContent("no-error");
    });
  });

  describe("token refresh", () => {
    it("initializes with stored auth and validates token", async () => {
      const storedAuth = {
        accessToken: "old-access-token",
        refreshToken: "valid-refresh-token",
        user: mockUser,
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(storedAuth));

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      });

      // Verify that localStorage was updated with validated user data
      expect(localStorageMock.setItem).toHaveBeenCalled();
    });
  });

  describe("isAuthenticated", () => {
    it("returns true only when both user and accessToken exist", async () => {
      const storedAuth = {
        accessToken: "test-token",
        refreshToken: "test-refresh",
        user: mockUser,
      };
      localStorageMock.getItem.mockReturnValue(JSON.stringify(storedAuth));

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        })
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("authenticated")).toHaveTextContent("authenticated");
      });
    });

    it("returns false when no user exists", async () => {
      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId("loading")).toHaveTextContent("not-loading");
      });

      expect(screen.getByTestId("authenticated")).toHaveTextContent("not-authenticated");
    });
  });
});
