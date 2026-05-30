import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { RouteGuard } from './route-guard';

const mockUseAuth = vi.fn();
vi.mock('@/providers', () => ({
  useAuth: () => mockUseAuth(),
}));

const mockNavigate = vi.fn();
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe('RouteGuard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('loading state', () => {
    it('shows loading spinner while auth is loading', () => {
      mockUseAuth.mockReturnValue({ isLoading: true, isAuthenticated: false });

      const { container } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      expect(container.querySelector('.animate-spin')).toBeInTheDocument();
      expect(screen.queryByTestId('children')).not.toBeInTheDocument();
    });

    it('does not redirect while loading', async () => {
      mockUseAuth.mockReturnValue({ isLoading: true, isAuthenticated: false });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(mockNavigate).not.toHaveBeenCalled();
      });
    });
  });

  describe('unauthenticated state', () => {
    it('redirects to login when not authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: false });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith({ to: '/login', replace: true });
      });
    });

    it('renders nothing while redirecting', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: false });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith({ to: '/login', replace: true });
      });
      expect(screen.queryByTestId('children')).not.toBeInTheDocument();
    });
  });

  describe('authenticated state', () => {
    it('renders children when authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: true });

      render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('children')).toBeInTheDocument();
      });
      expect(screen.getByText('Protected Content')).toBeInTheDocument();
    });

    it('does not redirect when authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: true });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(mockNavigate).not.toHaveBeenCalled();
      });
    });

    it('does not show loading spinner when authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: true });

      render(
        <RouteGuard>
          <div>Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(document.querySelector('.animate-spin')).not.toBeInTheDocument();
      });
    });

    it('renders multiple children when authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: true });

      render(
        <RouteGuard>
          <div data-testid="child1">Child 1</div>
          <div data-testid="child2">Child 2</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('child1')).toBeInTheDocument();
      });
      expect(screen.getByTestId('child2')).toBeInTheDocument();
    });
  });

  describe('state transitions', () => {
    it('transitions from loading to authenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: true, isAuthenticated: false });

      const { rerender } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      expect(document.querySelector('.animate-spin')).toBeInTheDocument();
      expect(screen.queryByTestId('children')).not.toBeInTheDocument();

      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: true });
      rerender(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(screen.getByTestId('children')).toBeInTheDocument();
      });
      expect(document.querySelector('.animate-spin')).not.toBeInTheDocument();
    });

    it('transitions from loading to unauthenticated', async () => {
      mockUseAuth.mockReturnValue({ isLoading: true, isAuthenticated: false });

      const { rerender } = render(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      expect(document.querySelector('.animate-spin')).toBeInTheDocument();

      mockUseAuth.mockReturnValue({ isLoading: false, isAuthenticated: false });
      rerender(
        <RouteGuard>
          <div data-testid="children">Protected Content</div>
        </RouteGuard>,
      );

      await waitFor(() => {
        expect(mockNavigate).toHaveBeenCalledWith({ to: '/login', replace: true });
      });
    });
  });
});
