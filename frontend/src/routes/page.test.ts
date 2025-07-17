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
      expect(screen.getByText('Your self-hosted game collection management service')).toBeInTheDocument();
    });

    it('should set the correct page title', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      // The title should be set in the svelte:head
      const titleElements = document.querySelectorAll('title');
      const hasNexoriousTitle = Array.from(titleElements).some(
        title => title.textContent === 'Nexorious Game Collection'
      );
      expect(hasNexoriousTitle).toBe(true);
    });

    it('should have a centered layout with dashed border', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(HomePage);

      const contentBox = container.querySelector('.border-4.border-dashed');
      expect(contentBox).toBeInTheDocument();
      expect(contentBox?.classList.contains('rounded-lg')).toBe(true);
      expect(contentBox?.classList.contains('p-8')).toBe(true);
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

    it('should have proper styling for auth buttons', () => {
      renderComponent(HomePage);

      const loginLink = screen.getByText('Login').closest('a');
      const registerLink = screen.getByText('Register').closest('a');

      expect(loginLink?.classList.contains('bg-blue-500')).toBe(true);
      expect(loginLink?.classList.contains('hover:bg-blue-600')).toBe(true);
      expect(registerLink?.classList.contains('bg-gray-500')).toBe(true);
      expect(registerLink?.classList.contains('hover:bg-gray-600')).toBe(true);
    });

    it('should not show navigation buttons for unauthenticated users', () => {
      renderComponent(HomePage);

      expect(screen.queryByText('My Games')).not.toBeInTheDocument();
      expect(screen.queryByText('Add Game')).not.toBeInTheDocument();
      expect(screen.queryByText('Wishlist')).not.toBeInTheDocument();
      expect(screen.queryByText('Dashboard')).not.toBeInTheDocument();
    });
  });

  describe('Authenticated State', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should show personalized welcome message for authenticated users', () => {
      renderComponent(HomePage);

      expect(screen.getByText('Hello, testuser! Ready to manage your game collection?')).toBeInTheDocument();
    });

    it('should show all navigation buttons for authenticated users', () => {
      renderComponent(HomePage);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();
      expect(screen.getByText('Wishlist')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should have correct navigation links', () => {
      renderComponent(HomePage);

      const myGamesLink = screen.getByText('My Games').closest('a');
      const addGameLink = screen.getByText('Add Game').closest('a');
      const wishlistLink = screen.getByText('Wishlist').closest('a');
      const dashboardLink = screen.getByText('Dashboard').closest('a');

      expect(myGamesLink?.getAttribute('href')).toBe('/games');
      expect(addGameLink?.getAttribute('href')).toBe('/games/add');
      expect(wishlistLink?.getAttribute('href')).toBe('/wishlist');
      expect(dashboardLink?.getAttribute('href')).toBe('/dashboard');
    });

    it('should have proper button styling', () => {
      renderComponent(HomePage);

      const myGamesLink = screen.getByText('My Games').closest('a');
      const addGameLink = screen.getByText('Add Game').closest('a');
      const wishlistLink = screen.getByText('Wishlist').closest('a');
      const dashboardLink = screen.getByText('Dashboard').closest('a');

      // Check common styling
      [myGamesLink, addGameLink, wishlistLink, dashboardLink].forEach(link => {
        expect(link?.classList.contains('px-6')).toBe(true);
        expect(link?.classList.contains('py-2')).toBe(true);
        expect(link?.classList.contains('rounded-lg')).toBe(true);
        expect(link?.classList.contains('transition-colors')).toBe(true);
        expect(link?.classList.contains('inline-block')).toBe(true);
        expect(link?.classList.contains('text-center')).toBe(true);
      });

      // Check specific colors
      expect(myGamesLink?.classList.contains('bg-blue-500')).toBe(true);
      expect(addGameLink?.classList.contains('bg-green-500')).toBe(true);
      expect(wishlistLink?.classList.contains('bg-purple-500')).toBe(true);
      expect(dashboardLink?.classList.contains('bg-gray-500')).toBe(true);
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

    it('should stack buttons vertically on mobile', () => {
      setMobileViewport();
      const { container } = renderComponent(HomePage);

      const buttonContainer = container.querySelector('.flex.flex-col.sm\\:flex-row');
      expect(buttonContainer).toBeInTheDocument();
      expect(buttonContainer?.classList.contains('gap-4')).toBe(true);
    });

    it('should arrange buttons horizontally on desktop', () => {
      setDesktopViewport();
      const { container } = renderComponent(HomePage);

      const buttonContainer = container.querySelector('.flex.flex-col.sm\\:flex-row');
      expect(buttonContainer).toBeInTheDocument();
    });

    it('should maintain proper spacing on all screen sizes', () => {
      const { container } = renderComponent(HomePage);

      const buttonContainer = container.querySelector('.justify-center.gap-4');
      expect(buttonContainer).toBeInTheDocument();
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

    it('should have proper color contrast', () => {
      renderComponent(HomePage);

      // All navigation buttons should have white text on colored backgrounds
      const buttons = [
        screen.getByText('My Games'),
        screen.getByText('Add Game'),
        screen.getByText('Wishlist'),
        screen.getByText('Dashboard')
      ];

      buttons.forEach(button => {
        const link = button.closest('a');
        expect(link?.classList.contains('text-white')).toBe(true);
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

      expect(screen.getByText('Your self-hosted game collection management service')).toBeInTheDocument();
    });

    it('should provide clear calls to action', () => {
      setUnauthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Please log in to start managing your game collection')).toBeInTheDocument();
    });

    it('should encourage user engagement when authenticated', () => {
      setAuthenticatedState();
      renderComponent(HomePage);

      expect(screen.getByText('Hello, testuser! Ready to manage your game collection?')).toBeInTheDocument();
    });
  });

  describe('Layout and Styling', () => {
    it('should have centered content layout', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(HomePage);

      const textCenter = container.querySelector('.text-center');
      expect(textCenter).toBeInTheDocument();
    });

    it('should have proper spacing between elements', () => {
      setAuthenticatedState();
      const { container } = renderComponent(HomePage);

      const spacedContainer = container.querySelector('.space-y-4');
      expect(spacedContainer).toBeInTheDocument();
    });

    it('should use consistent typography', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(HomePage);

      const mainTitle = container.querySelector('.text-4xl.font-bold');
      const subtitle = container.querySelector('.text-lg');
      
      expect(mainTitle).toBeInTheDocument();
      expect(subtitle).toBeInTheDocument();
    });

  });
});