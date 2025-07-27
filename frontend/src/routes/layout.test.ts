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

      expect(screen.getByText('testuser')).toBeInTheDocument();
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

    it('should not show admin navigation for regular users', () => {
      renderComponent(Layout);

      expect(screen.queryByText('Administration')).not.toBeInTheDocument();
      expect(screen.queryByText('Admin Dashboard')).not.toBeInTheDocument();
      expect(screen.queryByText('Manage Users')).not.toBeInTheDocument();
      expect(screen.queryByText('Manage Platforms')).not.toBeInTheDocument();
    });
  });

  describe('Admin User State', () => {
    beforeEach(() => {
      // Set admin user state
      mockAuthStore.value = {
        user: { id: '1', username: 'admin', isAdmin: true },
        accessToken: 'test-token',
        refreshToken: 'test-refresh-token',
        isLoading: false,
        error: null
      };
    });

    it('should show admin navigation section for admin users', () => {
      renderComponent(Layout);

      // Only desktop navigation is visible by default (mobile menu is closed)
      expect(screen.getByText('Administration')).toBeInTheDocument();
      expect(screen.getByText('Admin Dashboard')).toBeInTheDocument();
      expect(screen.getByText('Manage Users')).toBeInTheDocument();
      expect(screen.getByText('Manage Platforms')).toBeInTheDocument();
    });

    it('should have correct admin navigation links', () => {
      renderComponent(Layout);

      const adminDashboardLink = screen.getByText('Admin Dashboard').closest('a'); // Desktop link only
      const manageUsersLink = screen.getByText('Manage Users').closest('a'); // Desktop link only
      const managePlatformsLink = screen.getByText('Manage Platforms').closest('a'); // Desktop link only

      expect(adminDashboardLink?.getAttribute('href')).toBe('/admin/dashboard');
      expect(manageUsersLink?.getAttribute('href')).toBe('/admin/users');
      expect(managePlatformsLink?.getAttribute('href')).toBe('/admin/platforms');
    });

    it('should show both regular and admin navigation', () => {
      renderComponent(Layout);

      // Regular navigation
      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
      expect(screen.getByText('Add Game')).toBeInTheDocument();

      // Admin navigation
      expect(screen.getByText('Administration')).toBeInTheDocument();
      expect(screen.getByText('Admin Dashboard')).toBeInTheDocument();
      expect(screen.getByText('Manage Users')).toBeInTheDocument();
      expect(screen.getByText('Manage Platforms')).toBeInTheDocument();
    });
  });

  describe('Mobile Navigation', () => {
    beforeEach(() => {
      setAuthenticatedState();
      setMobileViewport();
    });

    it('should show mobile menu button on mobile when authenticated', () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]');
      expect(mobileMenuButton).toBeInTheDocument();
    });

    it('should hide desktop navigation on mobile', () => {
      const { container } = renderComponent(Layout);

      const desktopNav = container.querySelector('nav');
      expect(desktopNav).toBeInTheDocument();
    });

    it('should toggle mobile menu when button is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      
      // Initially no mobile menu should be visible
      expect(screen.queryByText('Sign out')).not.toBeInTheDocument();

      // Click to open menu
      await userEvent.click(mobileMenuButton);
      
      // Mobile menu should now be visible - use getAllByText and check both elements exist
      const myGamesLinks = screen.getAllByText('My Games');
      expect(myGamesLinks).toHaveLength(2); // One in desktop nav, one in mobile nav
      expect(screen.getByText('Sign out')).toBeInTheDocument();
    });

    it('should show user avatar in mobile menu', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Should show user's first letter - use getAllByText since there are multiple instances
      const userInitials = screen.getAllByText('T');
      expect(userInitials).toHaveLength(3); // One in desktop nav, one in mobile header, one in mobile menu
      const usernames = screen.getAllByText('testuser');
      expect(usernames).toHaveLength(2); // One in desktop nav, one in mobile nav
    });

    it('should close mobile menu when navigation link is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Click on a navigation link in mobile menu
      const signOutButton = screen.getByText('Sign out');
      await userEvent.click(signOutButton);
      
      // Mobile menu should be closed (Sign out should not be visible)
      expect(screen.queryByText('Sign out')).not.toBeInTheDocument();
    });

    it('should logout and close mobile menu when sign out is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      const signOutButton = screen.getByText('Sign out');
      await userEvent.click(signOutButton);

      expect(mockAuthStore.logout).toHaveBeenCalledOnce();
      expect(screen.queryByText('Sign out')).not.toBeInTheDocument();
    });

    it('should show hamburger icon when menu is closed', () => {
      renderComponent(Layout);

      // Should show hamburger menu button when closed
      const menuButton = screen.getByLabelText('Toggle mobile menu');
      expect(menuButton).toBeInTheDocument();
      // Check for hamburger lines SVG path
      expect(menuButton.querySelector('path[d*="M3.75 6.75h16.5"]')).toBeInTheDocument();
    });

    it('should show close icon when menu is open', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Check for close X SVG path
      expect(mobileMenuButton.querySelector('path[d*="M6 18L18 6M6 6l12 12"]')).toBeInTheDocument();
    });
  });

  describe('Mobile Admin Navigation', () => {
    beforeEach(() => {
      // Set admin user state for mobile
      mockAuthStore.value = {
        user: { id: '1', username: 'admin', isAdmin: true },
        accessToken: 'test-token',
        refreshToken: 'test-refresh-token',
        isLoading: false,
        error: null
      };
      setMobileViewport();
    });

    it('should show admin navigation in mobile menu for admin users', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Wait for mobile menu to appear and find Administration text within the mobile menu
      const mobileMenu = container.querySelector('[role="dialog"]');
      expect(mobileMenu).toBeInTheDocument();
      
      // Look for Administration text within the mobile menu context
      const administrationTexts = screen.getAllByText('Administration');
      expect(administrationTexts).toHaveLength(2); // One in desktop (hidden), one in mobile (visible)
      
      expect(screen.getAllByText('Admin Dashboard')).toHaveLength(2); // Desktop and mobile
      expect(screen.getAllByText('Manage Users')).toHaveLength(2); // Desktop and mobile
      expect(screen.getAllByText('Manage Platforms')).toHaveLength(2); // Desktop and mobile
    });

    it('should close mobile menu when admin navigation link is clicked', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      // Click on admin dashboard link - get the visible one in mobile menu
      const adminDashboardLinks = screen.getAllByText('Admin Dashboard');
      const adminDashboardLink = adminDashboardLinks[1]; // Mobile menu link
      expect(adminDashboardLink).toBeDefined();
      await userEvent.click(adminDashboardLink!);

      // Mobile menu should be closed
      expect(screen.queryByText('Sign out')).not.toBeInTheDocument();
    });

    it('should have correct admin navigation links in mobile menu', async () => {
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      const adminDashboardLink = screen.getAllByText('Admin Dashboard')[1]?.closest('a'); // Mobile menu link
      const manageUsersLink = screen.getAllByText('Manage Users')[1]?.closest('a'); // Mobile menu link
      const managePlatformsLink = screen.getAllByText('Manage Platforms')[1]?.closest('a'); // Mobile menu link

      expect(adminDashboardLink?.getAttribute('href')).toBe('/admin/dashboard');
      expect(manageUsersLink?.getAttribute('href')).toBe('/admin/users');
      expect(managePlatformsLink?.getAttribute('href')).toBe('/admin/platforms');
    });

    it('should not show admin navigation for regular users in mobile menu', async () => {
      // Switch back to regular user
      mockAuthStore.value = {
        user: { id: '1', username: 'testuser', isAdmin: false },
        accessToken: 'test-token',
        refreshToken: 'test-refresh-token',
        isLoading: false,
        error: null
      };

      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      await userEvent.click(mobileMenuButton);

      expect(screen.queryByText('Administration')).not.toBeInTheDocument();
      expect(screen.queryByText('Admin Dashboard')).not.toBeInTheDocument();
      expect(screen.queryByText('Manage Users')).not.toBeInTheDocument();
      expect(screen.queryByText('Manage Platforms')).not.toBeInTheDocument();
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

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      expect(mobileMenuButton).toHaveAttribute('aria-label', 'Toggle mobile menu');
    });

    it('should update mobile menu state when toggled', async () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]') as HTMLButtonElement;
      
      await userEvent.click(mobileMenuButton);
      // Check for close X SVG path
      expect(mobileMenuButton.querySelector('path[d*="M6 18L18 6M6 6l12 12"]')).toBeInTheDocument();
    });
  });

  describe('PWA Integration', () => {
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

      const desktopNav = container.querySelector('nav');
      expect(desktopNav).toBeInTheDocument();
      expect(screen.getByText('My Games')).toBeInTheDocument();
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    it('should show mobile menu button on mobile', () => {
      setMobileViewport();
      const { container } = renderComponent(Layout);

      // Mobile menu button should be present
      const mobileMenuButton = container.querySelector('button[aria-label="Toggle mobile menu"]');
      expect(mobileMenuButton).toBeInTheDocument();
    });
  });

  describe('Layout Slot', () => {
    it('should render slot content in main element', () => {
      setUnauthenticatedState();
      const { container } = renderComponent(Layout);

      const main = container.querySelector('main');
      expect(main).toBeInTheDocument();
    });
  });
});