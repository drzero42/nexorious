import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import AdminPlatformsPage from './+page.svelte';
// Import types for use in mock definitions
import type { Platform, Storefront } from '$lib/stores/platforms.svelte';
import { platforms } from '$lib/stores';
import { goto } from '$app/navigation';

// Mock navigation
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));

// Mock RouteGuard with the existing mock component
vi.mock('$lib/components', async () => {
  const MockRouteGuard = await import('../../../test-utils/MockRouteGuard.svelte');
  return {
    RouteGuard: MockRouteGuard.default
  };
});

// Mock stores - define these inside vi.mock to avoid hoisting issues
vi.mock('$lib/stores', () => {
  const { writable } = require('svelte/store');
  
  const mockPlatform: Platform = {
    id: '1',
    name: 'pc_windows',
    display_name: 'PC (Windows)',
    icon_url: 'https://example.com/pc-icon.png',
    is_active: true,
    source: 'official',
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z',
    updated_at: '2023-01-01T00:00:00.000Z'
  };

  const mockInactivePlatform: Platform = {
    id: '2',
    name: 'old_console',
    display_name: 'Old Console',
    icon_url: 'https://example.com/old-icon.png',
    is_active: false,
    source: 'user',
    version_added: '1.0.0',
    created_at: '2023-01-02T00:00:00Z',
    updated_at: '2023-01-02T00:00:00Z'
  };

  const mockStorefront: Storefront = {
    id: '1',
    name: 'steam',
    display_name: 'Steam',
    icon_url: 'https://example.com/steam-icon.png',
    base_url: 'https://store.steampowered.com',
    is_active: true,
    source: 'official',
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z',
    updated_at: '2023-01-01T00:00:00.000Z'
  };

  const mockInactiveStorefront: Storefront = {
    id: '2',
    name: 'old_store',
    display_name: 'Old Store',
    icon_url: 'https://example.com/old-store-icon.png',
    base_url: 'https://oldstore.example.com',
    is_active: false,
    source: 'user',
    version_added: '1.0.0',
    created_at: '2023-01-02T00:00:00Z',
    updated_at: '2023-01-02T00:00:00Z'
  };

  const platformsStore = writable({
    platforms: [mockPlatform, mockInactivePlatform],
    storefronts: [mockStorefront, mockInactiveStorefront],
    isLoading: false,
    error: null
  });

  return {
    platforms: {
      subscribe: platformsStore.subscribe,
      fetchAll: vi.fn().mockImplementation(async () => {
        // Simulate the actual fetchAll behavior by updating the store
        platformsStore.set({
          platforms: [mockPlatform, mockInactivePlatform],
          storefronts: [mockStorefront, mockInactiveStorefront],  
          isLoading: false,
          error: null
        });
        return {
          platforms: [mockPlatform, mockInactivePlatform],
          storefronts: [mockStorefront, mockInactiveStorefront]
        };
      }),
      createPlatform: vi.fn().mockResolvedValue(mockPlatform),
      updatePlatform: vi.fn().mockResolvedValue(mockPlatform),
      deletePlatform: vi.fn().mockResolvedValue(undefined),
      createStorefront: vi.fn().mockResolvedValue(mockStorefront),
      updateStorefront: vi.fn().mockResolvedValue(mockStorefront),
      deleteStorefront: vi.fn().mockResolvedValue(undefined),
      clearError: vi.fn(() => platformsStore.update((state: any) => ({ ...state, error: null }))),
      __updateStore: (newState: any) => platformsStore.update((state: any) => ({ ...state, ...newState }))
    },
    auth: {
      value: {
        user: { id: '1', username: 'admin', isAdmin: true },
        accessToken: 'test-token',
        isAuthenticated: true
      }
    }
  };
});

describe('Admin Platforms Page', () => {
  const user = userEvent.setup();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('Basic Component Tests', () => {
    it('renders without crashing', () => {
      expect(() => {
        render(AdminPlatformsPage);
      }).not.toThrow();
    });

    it('displays the page header correctly', () => {
      render(AdminPlatformsPage);
      expect(screen.getByText('Platform & Storefront Management')).toBeInTheDocument();
      expect(screen.getByText('Manage available platforms and storefronts for the application. Click on status badges to toggle between active and inactive states.')).toBeInTheDocument();
    });

    it('shows loading state when store is loading', async () => {
      // Update the store to show loading state
      (platforms as any).__updateStore({ isLoading: true });
      
      render(AdminPlatformsPage);
      expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument();
    });

    it('shows platforms data when loaded', async () => {
      render(AdminPlatformsPage);
      
      // Wait for the fetchAll to complete and loading to finish
      await waitFor(() => {
        expect(screen.queryByRole('status', { name: /loading/i })).not.toBeInTheDocument();
      });
      
      // Now check that platform data is shown
      expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
    });
  });

  describe('Authentication & Access Control Tests', () => {
    it('allows admin users to access the page', async () => {
      render(AdminPlatformsPage);
      
      expect(goto).not.toHaveBeenCalled();
      expect(platforms.fetchAll).toHaveBeenCalled();
    });
  });

  describe('Data Loading & Store Integration Tests', () => {
    it('calls fetchAll on mount for admin users', async () => {
      render(AdminPlatformsPage);
      
      await waitFor(() => {
        expect(platforms.fetchAll).toHaveBeenCalled();
      });
    });

    it('displays error messages when API calls fail', async () => {
      render(AdminPlatformsPage);
      
      // Wait for the component to mount and fetchAll to complete
      await waitFor(() => {
        expect(screen.queryByRole('status', { name: /loading/i })).not.toBeInTheDocument();
      });
      
      // Now set error state after the initial mount/fetchAll has completed
      (platforms as any).__updateStore({
        platforms: [],
        storefronts: [],
        isLoading: false,
        error: 'Failed to load data'
      });
      
      await waitFor(() => {
        expect(screen.getByText('Error')).toBeInTheDocument();
        expect(screen.getByText('Failed to load data')).toBeInTheDocument();
      });
    });

    it('clears error when dismiss button is clicked', async () => {
      render(AdminPlatformsPage);
      
      // Wait for the component to mount and fetchAll to complete
      await waitFor(() => {
        expect(screen.queryByRole('status', { name: /loading/i })).not.toBeInTheDocument();
      });
      
      // Now set error state after the initial mount/fetchAll has completed
      (platforms as any).__updateStore({
        platforms: [],
        storefronts: [],
        isLoading: false,
        error: 'Test error'
      });
      
      await waitFor(() => {
        expect(screen.getByText('Dismiss')).toBeInTheDocument();
      });
      
      const dismissButton = screen.getByText('Dismiss');
      await user.click(dismissButton);
      
      expect(platforms.clearError).toHaveBeenCalled();
    });
  });

  describe('Tab Navigation Tests', () => {
    it('shows platforms tab as active by default', async () => {
      render(AdminPlatformsPage);
      
      const platformsTab = screen.getByRole('button', { name: /🎮 Platforms/i });
      expect(platformsTab).toHaveClass('border-primary-500', 'text-primary-600');
    });

    it('switches to storefronts tab when clicked', async () => {
      render(AdminPlatformsPage);
      
      const storefrontsTab = screen.getByRole('button', { name: /🏪 Storefronts/i });
      await user.click(storefrontsTab);
      
      expect(storefrontsTab).toHaveClass('border-primary-500', 'text-primary-600');
      // Check for the specific header in the content area, not the tab
      expect(screen.getByRole('heading', { name: 'Storefronts' })).toBeInTheDocument();
    });

    it('displays correct content for each tab', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      // Switch to storefronts tab
      const storefrontsTab = screen.getByRole('button', { name: /🏪 Storefronts/i });
      await user.click(storefrontsTab);
      
      await waitFor(() => {
        expect(screen.getByText('Steam')).toBeInTheDocument();
      });
    });
  });

  describe('Search & Filtering Tests', () => {
    it('filters platforms by search query', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      const searchInput = screen.getByPlaceholderText('Search platforms...');
      await user.type(searchInput, 'PC');
      
      expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      expect(screen.queryByText('Old Console')).not.toBeInTheDocument();
    });

    it('filters platforms by status', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      const statusFilter = screen.getByDisplayValue('All Status');
      await user.selectOptions(statusFilter, 'active');
      
      expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      expect(screen.queryByText('Old Console')).not.toBeInTheDocument();
    });

    it('shows empty state when no platforms match filters', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getByText('PC (Windows)')).toBeInTheDocument();
      });
      
      const searchInput = screen.getByPlaceholderText('Search platforms...');
      await user.type(searchInput, 'nonexistent');
      
      expect(screen.getByText('No platforms found')).toBeInTheDocument();
    });
  });

  describe('Platform Management Tests', () => {
    it('opens create platform modal when add button is clicked', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('Add Platform')).toBeInTheDocument();
      });
      
      const addButton = screen.getByText('Add Platform');
      await user.click(addButton);
      
      expect(screen.getByText('Create New Platform')).toBeInTheDocument();
      expect(screen.getByLabelText('Platform Name')).toBeInTheDocument();
    });

    it('submits platform creation form', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('Add Platform')).toBeInTheDocument();
      });
      
      const addButton = screen.getByText('Add Platform');
      await user.click(addButton);
      
      const nameInput = screen.getByLabelText('Platform Name');
      const displayNameInput = screen.getByLabelText('Display Name');
      const submitButton = screen.getByText('Create');
      
      await user.type(nameInput, 'new_platform');
      await user.type(displayNameInput, 'New Platform');
      await user.click(submitButton);
      
      expect(platforms.createPlatform).toHaveBeenCalledWith({
        name: 'new_platform',
        display_name: 'New Platform',
        icon_url: '',
        is_active: true
      });
    });

    it('toggles platform status when status button is clicked', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getAllByText('Active').length).toBeGreaterThan(0);
      });
      
      const statusButtons = screen.getAllByText('Active');
      await user.click(statusButtons[0]!);
      
      expect(platforms.updatePlatform).toHaveBeenCalledWith('1', { is_active: false });
    });

    it('shows delete confirmation modal for platforms', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getAllByText('Delete').length).toBeGreaterThan(0);
      });
      
      const deleteButton = screen.getAllByText('Delete')[0]!;
      await user.click(deleteButton);
      
      expect(screen.getByText('Confirm Deletion')).toBeInTheDocument();
      expect(screen.getByText('Are you sure you want to delete the platform "PC (Windows)"?')).toBeInTheDocument();
    });
  });

  describe('Storefront Management Tests', () => {
    it('opens create storefront modal when add button is clicked', async () => {
      render(AdminPlatformsPage);
      
      // Switch to storefronts tab first
      const storefrontsTab = screen.getByRole('button', { name: /🏪 Storefronts/i });
      await user.click(storefrontsTab);
      
      // Wait for storefront data to load
      await waitFor(() => {
        expect(screen.getByText('Add Storefront')).toBeInTheDocument();
      });
      
      const addButton = screen.getByText('Add Storefront');
      await user.click(addButton);
      
      expect(screen.getByText('Create New Storefront')).toBeInTheDocument();
      expect(screen.getByLabelText('Storefront Name')).toBeInTheDocument();
      expect(screen.getByLabelText('Base URL (Optional)')).toBeInTheDocument();
    });

    it('displays storefront base URLs as links', async () => {
      render(AdminPlatformsPage);
      
      // Switch to storefronts tab first
      const storefrontsTab = screen.getByRole('button', { name: /🏪 Storefronts/i });
      await user.click(storefrontsTab);
      
      // Wait for storefront data to load
      await waitFor(() => {
        expect(screen.getByRole('link', { name: 'https://store.steampowered.com' })).toBeInTheDocument();
      });
      
      const steamStoreLink = screen.getByRole('link', { name: 'https://store.steampowered.com' });
      expect(steamStoreLink).toHaveAttribute('href', 'https://store.steampowered.com');
      expect(steamStoreLink).toHaveAttribute('target', '_blank');
    });
  });

  describe('Modal Interaction Tests', () => {
    it('closes platform modal when cancel button is clicked', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('Add Platform')).toBeInTheDocument();
      });
      
      const addButton = screen.getByText('Add Platform');
      await user.click(addButton);
      
      expect(screen.getByText('Create New Platform')).toBeInTheDocument();
      
      const cancelButton = screen.getByText('Cancel');
      await user.click(cancelButton);
      
      expect(screen.queryByText('Create New Platform')).not.toBeInTheDocument();
    });
  });

  describe('Utility Function Tests', () => {
    it('formats dates correctly', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getByText(/Jan 1, 2023/)).toBeInTheDocument();
      });
    });

    it('displays source badges correctly', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getByText('official')).toBeInTheDocument();
      });
      
      const officialBadge = screen.getByText('official');
      expect(officialBadge).toHaveClass('bg-blue-100', 'text-blue-800');
      
      const userBadge = screen.getByText('user');
      expect(userBadge).toHaveClass('bg-purple-100', 'text-purple-800');
    });

    it('shows appropriate status badges', async () => {
      render(AdminPlatformsPage);
      
      // Wait for data to load first
      await waitFor(() => {
        expect(screen.getAllByText('Active').length).toBeGreaterThan(0);
      });
      
      const activeButtons = screen.getAllByText('Active');
      expect(activeButtons[0]).toHaveClass('bg-green-100', 'text-green-800');
      
      const inactiveButtons = screen.getAllByText('Inactive');
      expect(inactiveButtons[0]).toHaveClass('bg-gray-100', 'text-gray-800');
    });
  });
});