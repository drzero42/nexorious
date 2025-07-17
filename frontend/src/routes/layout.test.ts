import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { cleanup, screen } from '@testing-library/svelte';
import Layout from './+layout.svelte';
import {
  renderComponent,
  createUserEvent,
  setMobileViewport,
  setDesktopViewport,
  testAccessibility,
  testKeyboardNavigation
} from '../test-utils/test-helpers';
import {
  mockAuthStore,
  setAuthenticatedState,
  setUnauthenticatedState,
  resetAuthMocks
} from '../test-utils/auth-mocks';
import { resetNavigationMocks } from '../test-utils/navigation-mocks';

// Mock PWA components
vi.mock('$lib/components/PWAInstallButton.svelte', () => ({
  default: vi.fn(() => ({ $$: {} }))
}));

vi.mock('$lib/components/PWAUpdateNotification.svelte', () => ({
  default: vi.fn(() => ({ $$: {} }))
}));

vi.mock('$lib/components/OfflineIndicator.svelte', () => ({
  default: vi.fn(() => ({ $$: {} }))
}));

// Mock PWA functions
vi.mock('$lib/pwa', () => ({
  initializePWA: vi.fn(),
  initializeInstallPrompt: vi.fn()
}));

describe('Layout Component', () => {
  const userEvent = createUserEvent();

  beforeEach(() => {
    resetAuthMocks();
    resetNavigationMocks();
    setDesktopViewport();
  });

  afterEach(() => {
    cleanup();
  });

  describe('Basic Rendering', () => {
    it('should render the main layout structure', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(Layout);

      expect(container.querySelector('header')).toBeInTheDocument();
      expect(container.querySelector('main')).toBeInTheDocument();
      expect(screen.getByText('Nexorious')).toBeInTheDocument();
    });

    it('should render the correct page title', () => {
      setUnauthenticatedState();
      renderComponent(Layout);

      expect(document.title).toBe('Nexorious Game Collection');
    });

    it('should include proper meta tags', () => {
      setUnauthenticatedState();
      renderComponent(Layout);

      const description = document.querySelector('meta[name="description"]');
      expect(description?.getAttribute('content')).toBe('Self-hostable game collection management');

      const viewport = document.querySelector('meta[name="viewport"]');
      expect(viewport?.getAttribute('content')).toBe('width=device-width, initial-scale=1');
    });
  });

  describe('Unauthenticated State', () => {
    beforeEach(() => {
      setUnauthenticatedState();
    });

    it('should show login link when not authenticated', () => {
      renderComponent(Layout);

      const loginLink = screen.getByText('Login');
      expect(loginLink).toBeInTheDocument();
      expect(loginLink.getAttribute('href')).toBe('/login');
    });

    it('should not show navigation menu when not authenticated', () => {
      renderComponent(Layout);

      expect(screen.queryByText('My Games')).not.toBeInTheDocument();
      expect(screen.queryByText('Dashboard')).not.toBeInTheDocument();
    });

    it('should not show user welcome message when not authenticated', () => {
      renderComponent(Layout);

      expect(screen.queryByText(/Welcome,/)).not.toBeInTheDocument();
    });

    it('should not show mobile menu button when not authenticated', () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[class*="md:hidden"]');
      expect(mobileMenuButton).not.toBeInTheDocument();
    });
  });

  describe('Authenticated State', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should show navigation menu when authenticated', () => {
      renderComponent(Layout);

      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should show user welcome message when authenticated', () => {
      renderComponent(Layout);

      expect(screen.getByText('Welcome, testuser')).toBeInTheDocument();
    });

    it('should show logout button when authenticated', () => {
      renderComponent(Layout);

      expect(screen.getByText('Logout')).toBeInTheDocument();
    });

    it('should have correct navigation links', () => {
      renderComponent(Layout);

      const gamesLink = screen.getByText('My Games').closest('a');
      const dashboardLink = screen.getByText('Dashboard').closest('a');

      expect(gamesLink?.getAttribute('href')).toBe('/games');
      expect(dashboardLink?.getAttribute('href')).toBe('/dashboard');
    });

    it('should call logout function when logout button is clicked', async () => {
      renderComponent(Layout);

      const logoutButton = screen.getByText('Logout');
      await userEvent.click(logoutButton);

      expect(mockAuthStore.logout).toHaveBeenCalledOnce();
    });
  });

  describe('Mobile Navigation', () => {
    beforeEach(() => {
      setAuthenticatedState();
      setMobileViewport();
    });

    it('should show mobile menu button on mobile when authenticated', () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden');
      expect(mobileMenuButton).toBeInTheDocument();
    });

    it('should hide desktop navigation on mobile', () => {
      const { container } = renderComponent(Layout);

      const desktopNav = container.querySelector('nav.hidden.md\\:flex');
      expect(desktopNav).toBeInTheDocument();
    });

    it('should toggle mobile menu when button is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      
      // Initially no mobile menu should be visible
      expect(container.querySelector('.md\\:hidden.bg-white')).not.toBeInTheDocument();

      // Click to open menu
      await userEvent.click(mobileMenuButton);
      
      // Mobile menu should now be visible
      expect(container.querySelector('.md\\:hidden.bg-white')).toBeInTheDocument();
      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Sign out')).toBeInTheDocument();
    });

    it('should show user avatar in mobile menu', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Should show user's first letter
      expect(screen.getByText('T')).toBeInTheDocument(); // First letter of 'testuser'
      expect(screen.getByText('testuser')).toBeInTheDocument();
    });

    it('should close mobile menu when navigation link is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Click on a navigation link in mobile menu
      const mobileGamesLink = container.querySelector('.md\\:hidden .space-y-1 a[href="/games"]') as HTMLAnchorElement;
      await userEvent.click(mobileGamesLink);

      // Mobile menu should be closed (not visible)
      expect(container.querySelector('.md\\:hidden.bg-white')).not.toBeInTheDocument();
    });

    it('should logout and close mobile menu when sign out is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      const signOutButton = screen.getByText('Sign out');
      await userEvent.click(signOutButton);

      expect(mockAuthStore.logout).toHaveBeenCalledOnce();
      expect(container.querySelector('.md\\:hidden.bg-white')).not.toBeInTheDocument();
    });

    it('should show hamburger icon when menu is closed', () => {
      const { container } = renderComponent(Layout);

      const menuIcon = container.querySelector('path[d*="M4 6h16M4 12h16M4 18h16"]');
      expect(menuIcon).toBeInTheDocument();
    });

    it('should show close icon when menu is open', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      const closeIcon = container.querySelector('path[d*="M6 18L18 6M6 6l12 12"]');
      expect(closeIcon).toBeInTheDocument();
    });
  });

  describe('Accessibility', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should have proper accessibility attributes', () => {
      const { container } = renderComponent(Layout);
      testAccessibility(container);
    });

    it('should support keyboard navigation', async () => {
      const { container } = renderComponent(Layout);
      await testKeyboardNavigation(container, userEvent);
    });

    it('should have accessible mobile menu button', () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      expect(mobileMenuButton).toHaveAttribute('aria-expanded', 'false');
    });

    it('should update aria-expanded when mobile menu is toggled', async () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button.md\\:hidden') as HTMLButtonElement;
      
      await userEvent.click(mobileMenuButton);
      expect(mobileMenuButton).toHaveAttribute('aria-expanded', 'true');
    });
  });

  describe('PWA Integration', () => {
    it('should initialize PWA on mount', () => {
      const { initializePWA, initializeInstallPrompt } = require('$lib/pwa');
      
      setUnauthenticatedState();
      renderComponent(Layout);

      expect(initializePWA).toHaveBeenCalledOnce();
      expect(initializeInstallPrompt).toHaveBeenCalledOnce();
    });

    it('should refresh auth token on mount if tokens exist', () => {
      mockAuthStore.value = {
        user: null,
        accessToken: 'existing-token',
        refreshToken: 'existing-refresh-token',
        isLoading: false,
        error: null
      };

      renderComponent(Layout);

      expect(mockAuthStore.refreshAuth).toHaveBeenCalledOnce();
    });
  });

  describe('Responsive Design', () => {
    beforeEach(() => {
      setAuthenticatedState();
    });

    it('should show desktop navigation on desktop', () => {
      setDesktopViewport();
      const { container } = renderComponent(Layout);

      const desktopNav = container.querySelector('nav.hidden.md\\:flex');
      expect(desktopNav).toBeInTheDocument();
      
      const desktopUserInfo = container.querySelector('.hidden.md\\:flex.items-center.space-x-4');
      expect(desktopUserInfo).toBeInTheDocument();
    });

    it('should hide desktop elements on mobile', () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      // Desktop navigation should have hidden class
      const desktopNav = container.querySelector('nav.hidden.md\\:flex');
      expect(desktopNav).toBeInTheDocument();
      
      // Desktop user info should have hidden class
      const desktopUserInfo = container.querySelector('.hidden.md\\:flex.items-center.space-x-4');
      expect(desktopUserInfo).toBeInTheDocument();
    });
  });

  describe('Layout Slot', () => {
    it('should render slot content in main element', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(Layout, {}, {
        context: new Map([['$$slots', { default: () => '<div>Test Content</div>' }]])
      });

      const main = container.querySelector('main');
      expect(main).toBeInTheDocument();
      expect(main?.classList.contains('max-w-7xl')).toBe(true);
      expect(main?.classList.contains('mx-auto')).toBe(true);
    });
  });
});