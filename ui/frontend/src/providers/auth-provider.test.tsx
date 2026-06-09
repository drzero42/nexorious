import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { server } from '@/test/mocks/server';
import { AuthProvider, useAuth } from './auth-provider';

// In test environment, NODE_ENV is 'test' so apiUrl defaults to '/api'
// MSW intercepts relative URLs as absolute URLs with the origin
const API_URL = '/api';

// Mock user data
const mockApiUser = {
  id: 'test-user-id',
  username: 'testuser',
  is_admin: false,
};

// Test component that uses the auth context
function TestConsumer() {
  const auth = useAuth();

  const handleLogin = async () => {
    try {
      await auth.login('testuser', 'password123');
    } catch {
      // Error is set in context, no need to handle here
    }
  };

  return (
    <div>
      <div data-testid="loading">{auth.isLoading ? 'loading' : 'not-loading'}</div>
      <div data-testid="authenticated">
        {auth.isAuthenticated ? 'authenticated' : 'not-authenticated'}
      </div>
      <div data-testid="user">{auth.user?.username ?? 'no-user'}</div>
      <div data-testid="error">{auth.error ?? 'no-error'}</div>
      <button onClick={handleLogin}>Login</button>
      <button onClick={() => void auth.logout()}>Logout</button>
      <button onClick={() => auth.clearError()}>Clear Error</button>
    </div>
  );
}

// Test component that throws when used outside provider
function TestConsumerWithoutProvider() {
  let errorMessage: string | null = null;
  try {
    useAuth();
  } catch (error) {
    errorMessage = (error as Error).message;
  }
  if (errorMessage !== null) {
    return <div data-testid="error-thrown">{errorMessage}</div>;
  }
  return <div>Should have thrown</div>;
}

describe('AuthProvider', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('initial state', () => {
    it('starts in loading state', () => {
      // Override getMe to hang so we can observe the loading state
      server.use(
        http.get(`${API_URL}/auth/me`, async () => {
          await new Promise(() => {}); // never resolves
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      expect(screen.getByTestId('loading')).toHaveTextContent('loading');
    });

    it('renders not-authenticated when no session exists (getMe returns 401)', async () => {
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
      expect(screen.getByTestId('user')).toHaveTextContent('no-user');
    });

    it('restores session when getMe succeeds (cookie is valid)', async () => {
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      expect(screen.getByTestId('user')).toHaveTextContent('testuser');
    });
  });

  describe('useAuth hook', () => {
    it('throws error when used outside AuthProvider', () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

      render(<TestConsumerWithoutProvider />);

      expect(screen.getByTestId('error-thrown')).toHaveTextContent(
        'useAuth must be used within an AuthProvider',
      );

      consoleSpy.mockRestore();
    });
  });

  describe('login', () => {
    it('successfully logs in user', async () => {
      const user = userEvent.setup();

      // No active session initially
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      // Login sets a cookie server-side and returns the user
      server.use(
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json(mockApiUser);
        }),
      );

      await user.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });

      expect(screen.getByTestId('user')).toHaveTextContent('testuser');
    });

    it('sets error state on login failure', async () => {
      const user = userEvent.setup();

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
        }),
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: 'Invalid credentials' }, { status: 401 });
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      await user.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent('Invalid credentials');
      });

      expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
    });

    it('clears error state on successful login after failure', async () => {
      const user = userEvent.setup();
      let loginAttempts = 0;

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
        }),
        http.post(`${API_URL}/auth/login`, () => {
          loginAttempts++;
          if (loginAttempts === 1) {
            return HttpResponse.json({ detail: 'Invalid credentials' }, { status: 401 });
          }
          return HttpResponse.json(mockApiUser);
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      // First login attempt fails
      await user.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent('Invalid credentials');
      });

      // Second login attempt succeeds
      await user.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent('no-error');
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });
    });
  });

  describe('logout', () => {
    it('clears auth state and calls logout API', async () => {
      const user = userEvent.setup();

      // Active session
      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });

      let logoutCalled = false;
      server.use(
        http.post(`${API_URL}/auth/logout`, () => {
          logoutCalled = true;
          return HttpResponse.json({ message: 'Logged out' });
        }),
      );

      await user.click(screen.getByText('Logout'));

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
      });

      expect(logoutCalled).toBe(true);
      expect(screen.getByTestId('user')).toHaveTextContent('no-user');
      expect(screen.getByTestId('error')).toHaveTextContent('no-error');
    });

    it('clears local state even when logout API call fails', async () => {
      const user = userEvent.setup();

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json(mockApiUser);
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });

      server.use(
        http.post(`${API_URL}/auth/logout`, () => {
          return HttpResponse.json({ detail: 'Server error' }, { status: 500 });
        }),
      );

      await user.click(screen.getByText('Logout'));

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
      });

      expect(screen.getByTestId('user')).toHaveTextContent('no-user');
    });
  });

  describe('clearError', () => {
    it('clears the error state', async () => {
      const user = userEvent.setup();

      server.use(
        http.get(`${API_URL}/auth/me`, () => {
          return HttpResponse.json({ detail: 'Not authenticated' }, { status: 401 });
        }),
        http.post(`${API_URL}/auth/login`, () => {
          return HttpResponse.json({ detail: 'Login failed' }, { status: 401 });
        }),
      );

      render(
        <AuthProvider>
          <TestConsumer />
        </AuthProvider>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      // Trigger an error
      await user.click(screen.getByText('Login'));

      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent('Login failed');
      });

      // Clear the error
      await user.click(screen.getByText('Clear Error'));

      expect(screen.getByTestId('error')).toHaveTextContent('no-error');
    });
  });
});
