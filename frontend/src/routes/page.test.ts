import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { cleanup, screen } from '@testing-library/svelte';
import HomePage from './+page.svelte';
import {
  renderComponent,
  setDesktopViewport,
  testAccessibility
} from '../test-utils/test-helpers';
import {
  setAuthenticatedState,
  setUnauthenticatedState,
  setSetupNeeded,
  setSetupNotNeeded,
  setSetupStatusError,
  resetAuthMocks,
  mockAuthStore
} from '../test-utils/auth-mocks';

// Mock SvelteKit navigation in this test file
vi.mock('$app/navigation', () => {
  return {
    goto: vi.fn()
  };
});

// Import the mocked module to get access to the mock
import { goto } from '$app/navigation';
const mockGoto = vi.mocked(goto);

describe('Home Page', () => {
  beforeEach(() => {
    resetAuthMocks();
    mockGoto.mockClear();
    setDesktopViewport();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Authenticated Users', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should render welcome content for authenticated users', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      expect(screen.getByText('Welcome back, testuser!')).toBeInTheDocument();
      expect(screen.getByText('Ready to manage your game collection?')).toBeInTheDocument();
    });

    it('should show navigation quick actions', () => {
      renderComponent(HomePage);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should have correct navigation links', () => {
      renderComponent(HomePage);

      const myGamesLink = screen.getByText('My Games').closest('a');
      const addGameLink = screen.getByText('Add Game').closest('a');
      const dashboardLink = screen.getByText('Dashboard').closest('a');

      expect(myGamesLink?.getAttribute('href')).toBe('/games');
      expect(addGameLink?.getAttribute('href')).toBe('/games/add');
      expect(dashboardLink?.getAttribute('href')).toBe('/dashboard');
    });

    it('should show feature sections', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Organize Your Library')).toBeInTheDocument();
      expect(screen.getByText('Track Progress')).toBeInTheDocument();
      expect(screen.getByText('Self-Hosted Privacy')).toBeInTheDocument();
    });

    it('should not redirect when authenticated', () => {
      renderComponent(HomePage);
      
      expect(mockGoto).not.toHaveBeenCalled();
    });
  });

  describe('Unauthenticated Users - Redirect Behavior', () => {
    beforeEach(() => {
      setUnauthenticatedState();
    });

    it('should call checkSetupStatus when unauthenticated', async () => {
      setSetupNotNeeded();
      renderComponent(HomePage);

      // Wait for the async onMount logic to complete
      await vi.waitFor(
        () => {
          expect(mockAuthStore.checkSetupStatus).toHaveBeenCalled();
        },
        { timeout: 2000 }
      );
    });

    it('should redirect to setup when setup is needed', async () => {
      setSetupNeeded();
      renderComponent(HomePage);

      // Wait for the async onMount logic to complete
      await vi.waitFor(
        () => {
          expect(mockGoto).toHaveBeenCalledWith('/setup');
        },
        { timeout: 2000 }
      );
    });

    it('should redirect to login when setup is not needed', async () => {
      setSetupNotNeeded();
      renderComponent(HomePage);

      // Wait for the async onMount logic to complete
      await vi.waitFor(
        () => {
          expect(mockGoto).toHaveBeenCalledWith('/login');
        },
        { timeout: 2000 }
      );
    });

    it('should redirect to login on setup status error', async () => {
      setSetupStatusError();
      renderComponent(HomePage);

      // Wait for the async onMount logic to complete
      await vi.waitFor(
        () => {
          expect(mockGoto).toHaveBeenCalledWith('/login');
        },
        { timeout: 2000 }
      );
    });

    it('should show loading state before redirect', () => {
      setSetupNotNeeded();
      renderComponent(HomePage);

      // Should show loading content while checking setup status
      expect(screen.getByText('Checking authentication status...')).toBeInTheDocument();
    });

    it('should show fallback content when not redirecting yet', () => {
      setSetupNotNeeded();
      renderComponent(HomePage);

      // Should show either loading or fallback content
      const loadingText = screen.queryByText('Checking authentication status...');
      const fallbackText = screen.queryByText('Redirecting to login...');
      
      expect(loadingText || fallbackText).toBeInTheDocument();
    });
  });

  describe('Layout and Structure', () => {
    it('should have proper main content container', () => {
      setAuthenticatedState();
      const { container } = renderComponent(HomePage);

      const mainContainer = container.querySelector('div');
      expect(mainContainer).toBeInTheDocument();
    });

    it('should have proper heading hierarchy', () => {
      setAuthenticatedState();
      renderComponent(HomePage);

      const mainHeading = screen.getByRole('heading', { level: 1 });
      expect(mainHeading).toHaveTextContent('Welcome to Nexorious');
    });

    it('should have proper accessibility attributes', () => {
      setAuthenticatedState();
      const { container } = renderComponent(HomePage);
      testAccessibility(container);
    });
  });

  describe('Content Structure', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should have descriptive content', () => {
      renderComponent(HomePage);

      expect(screen.getByText(/Your self-hosted game collection management service/)).toBeInTheDocument();
    });

    it('should provide clear user engagement', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Welcome back, testuser!')).toBeInTheDocument();
      expect(screen.getByText('Ready to manage your game collection?')).toBeInTheDocument();
    });

    it('should have accessible navigation links', () => {
      renderComponent(HomePage);

      const buttons = [
        screen.getByText('My Games'),
        screen.getByText('Add Game'),
        screen.getByText('Dashboard')
      ];

      buttons.forEach(button => {
        const link = button.closest('a');
        expect(link).toBeInTheDocument();
        expect(link?.getAttribute('href')).toBeTruthy();
      });
    });
  });
});