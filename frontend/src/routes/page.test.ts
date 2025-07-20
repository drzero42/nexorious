import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { cleanup, screen } from '@testing-library/svelte';
import HomePage from './+page.svelte';
import {
  renderComponent,
  createUserEvent,
  setMobileViewport,
  setDesktopViewport,
  testAccessibility
} from '../test-utils/test-helpers';
import {
  setAuthenticatedState,
  setUnauthenticatedState,
  resetAuthMocks
} from '../test-utils/auth-mocks';

describe('Home Page', () => {
  const userEvent = createUserEvent();

  beforeEach(() => {
    resetAuthMocks();
    setDesktopViewport();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Basic Rendering', () => {
    it('should render the welcome message', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
      expect(screen.getByText(/Your self-hosted game collection management service/)).toBeInTheDocument();
    });

    it('should render the home page content', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
    });

    it('should have main content container', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(HomePage);

      const contentContainer = container.querySelector('div');
      expect(contentContainer).toBeInTheDocument();
    });
  });

  describe('Unauthenticated State', () => {
    beforeEach(() => {
      setUnauthenticatedState();
    });

    it('should show login prompt for unauthenticated users', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Please log in to start managing your game collection')).toBeInTheDocument();
    });

    it('should show login and register buttons', () => {
      renderComponent(HomePage);

      const loginLink = screen.getByText('Login');
      const registerLink = screen.getByText('Register');

      expect(loginLink).toBeInTheDocument();
      expect(registerLink).toBeInTheDocument();
      expect(loginLink.closest('a')?.getAttribute('href')).toBe('/login');
      expect(registerLink.closest('a')?.getAttribute('href')).toBe('/register');
    });

    it('should have functional auth buttons', () => {
      renderComponent(HomePage);

      const loginLink = screen.getByText('Login').closest('a');
      const registerLink = screen.getByText('Register').closest('a');

      expect(loginLink).toBeInTheDocument();
      expect(registerLink).toBeInTheDocument();
    });

    it('should not show navigation buttons for unauthenticated users', () => {
      renderComponent(HomePage);

      expect(screen.queryByText('My Games')).not.toBeInTheDocument();
      expect(screen.queryByText('Add Game')).not.toBeInTheDocument();
      expect(screen.queryByText('Dashboard')).not.toBeInTheDocument();
    });
  });

  describe('Authenticated State', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should show personalized welcome message for authenticated users', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Welcome back, testuser!')).toBeInTheDocument();
      expect(screen.getByText('Ready to manage your game collection?')).toBeInTheDocument();
    });

    it('should show all navigation buttons for authenticated users', () => {
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

    it('should have functional navigation buttons', () => {
      renderComponent(HomePage);

      const myGamesLink = screen.getByText('My Games').closest('a');
      const addGameLink = screen.getByText('Add Game').closest('a');
      const dashboardLink = screen.getByText('Dashboard').closest('a');

      expect(myGamesLink).toBeInTheDocument();
      expect(addGameLink).toBeInTheDocument();
      expect(dashboardLink).toBeInTheDocument();
    });

    it('should not show login/register buttons for authenticated users', () => {
      renderComponent(HomePage);

      expect(screen.queryByText('Login')).not.toBeInTheDocument();
      expect(screen.queryByText('Register')).not.toBeInTheDocument();
    });
  });

  describe('Responsive Design', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should show navigation buttons on mobile', () => {
      setMobileViewport();
      renderComponent(HomePage);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should show navigation buttons on desktop', () => {
      setDesktopViewport();
      renderComponent(HomePage);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should have action buttons container', () => {
      renderComponent(HomePage);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should have proper accessibility attributes', () => {
      const { container } = renderComponent(HomePage);
      testAccessibility(container);
    });

    it('should have accessible link text', () => {
      renderComponent(HomePage);

      const links = screen.getAllByRole('link');
      links.forEach(link => {
        expect(link.textContent?.trim()).toBeTruthy();
      });
    });

    it('should support keyboard navigation', async () => {
      renderComponent(HomePage);

      const links = screen.getAllByRole('link');
      
      for (const link of links) {
        link.focus();
        expect(document.activeElement).toBe(link);
        
        // Test Enter key navigation
        await userEvent.keyDown(link, 'Enter');
      }
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

  describe('Content Structure', () => {
    it('should have proper heading hierarchy', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      const mainHeading = screen.getByRole('heading', { level: 1 });
      expect(mainHeading).toHaveTextContent('Welcome to Nexorious');
    });

    it('should have descriptive content', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText(/Your self-hosted game collection management service/)).toBeInTheDocument();
    });

    it('should provide clear calls to action', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Please log in to start managing your game collection')).toBeInTheDocument();
    });

    it('should encourage user engagement when authenticated', () => {
      setAuthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Welcome back, testuser!')).toBeInTheDocument();
      expect(screen.getByText('Ready to manage your game collection?')).toBeInTheDocument();
    });
  });

  describe('Layout and Styling', () => {
    it('should have main content layout', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(HomePage);

      const mainContainer = container.querySelector('div');
      expect(mainContainer).toBeInTheDocument();
    });

    it('should have feature sections', () => {
      setAuthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Organize Your Library')).toBeInTheDocument();
      expect(screen.getByText('Track Progress')).toBeInTheDocument();
      expect(screen.getByText('Self-Hosted Privacy')).toBeInTheDocument();
    });

    it('should have proper heading structure', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
      expect(screen.getByText('Welcome to Nexorious')).toBeInTheDocument();
    });

  });
});