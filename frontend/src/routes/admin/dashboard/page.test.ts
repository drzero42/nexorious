import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import AdminDashboard from './+page.svelte';
import { admin, auth } from '$lib/stores';

// Mock the stores
vi.mock('$lib/stores', () => ({
  admin: {
    value: {
      users: [],
      statistics: null,
      isLoading: false,
      error: null
    },
    fetchStatistics: vi.fn(),
    clearError: vi.fn()
  },
  auth: {
    value: {
      user: { id: '1', username: 'admin', isAdmin: true },
      accessToken: 'mock-token',
      refreshToken: 'mock-refresh-token',
      isLoading: false,
      error: null
    }
  }
}));

// Mock $app/navigation
vi.mock('$app/navigation', () => ({
  goto: vi.fn()
}));


describe('Admin Dashboard Page', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    
    // Reset admin store mock
    vi.mocked(admin).value = {
      users: [],
      statistics: null,
      isLoading: false,
      error: null
    };
    
    // Reset auth store mock
    vi.mocked(auth).value = {
      user: { id: '1', username: 'admin', isAdmin: true },
      accessToken: 'mock-token',
      refreshToken: 'mock-refresh-token',
      isLoading: false,
      error: null
    };
  });

  it('should render admin dashboard title', async () => {
    render(AdminDashboard);

    expect(screen.getByText('Admin Dashboard')).toBeInTheDocument();
    expect(screen.getByText('System overview and management tools')).toBeInTheDocument();
  });

  it('should redirect non-admin users', async () => {
    // Mock non-admin user
    vi.mocked(auth).value.user = { id: '1', username: 'user', isAdmin: false };

    render(AdminDashboard);

    await waitFor(() => {
      expect(goto).toHaveBeenCalledWith('/dashboard');
    });
  });

  it('should show loading state', async () => {
    vi.mocked(admin).value.isLoading = true;

    render(AdminDashboard);

    expect(screen.getByRole('status', { name: /loading/i })).toBeInTheDocument();
  });

  it('should display error message', async () => {
    vi.mocked(admin).value.error = 'Failed to load dashboard';

    render(AdminDashboard);

    expect(screen.getByText('Error loading dashboard')).toBeInTheDocument();
    expect(screen.getByText('Failed to load dashboard')).toBeInTheDocument();
  });

  it('should display statistics when loaded', async () => {
    const mockStatistics = {
      totalUsers: 5,
      totalAdmins: 2,
      totalGames: 0,
      recentUsers: [
        {
          id: '1',
          username: 'admin',
          isAdmin: true,
          isActive: true,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        },
        {
          id: '2',
          username: 'user1',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-02T00:00:00Z',
          updatedAt: '2023-01-02T00:00:00Z'
        }
      ]
    };

    // Mock fetchStatistics to resolve immediately
    vi.mocked(admin.fetchStatistics).mockResolvedValue(mockStatistics);
    vi.mocked(admin).value.statistics = mockStatistics;
    vi.mocked(admin).value.isLoading = false;

    render(AdminDashboard);

    // Wait for the component to finish loading
    await waitFor(() => {
      expect(screen.getByText('Total Users')).toBeInTheDocument();
    });

    // Check statistics cards
    expect(screen.getByText('5')).toBeInTheDocument();
    expect(screen.getByText('Admin Users')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('Total Games')).toBeInTheDocument();
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('System Status')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();

    // Check recent users section
    expect(screen.getByText('Recent Users')).toBeInTheDocument();
    expect(screen.getByText('admin')).toBeInTheDocument();
    expect(screen.getByText('user1')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument(); // Admin badge
  });

  it('should display inactive user badge', async () => {
    const mockStatistics = {
      totalUsers: 1,
      totalAdmins: 0,
      totalGames: 0,
      recentUsers: [
        {
          id: '1',
          username: 'inactiveuser',
          isAdmin: false,
          isActive: false,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        }
      ]
    };

    vi.mocked(admin.fetchStatistics).mockResolvedValue(mockStatistics);
    vi.mocked(admin).value.statistics = mockStatistics;
    vi.mocked(admin).value.isLoading = false;

    render(AdminDashboard);

    await waitFor(() => {
      expect(screen.getByText('Inactive')).toBeInTheDocument();
    });
  });

  it('should display quick actions', async () => {
    const mockStatistics = {
      totalUsers: 1,
      totalAdmins: 1,
      totalGames: 0,
      recentUsers: []
    };

    vi.mocked(admin.fetchStatistics).mockResolvedValue(mockStatistics);
    vi.mocked(admin).value.statistics = mockStatistics;
    vi.mocked(admin).value.isLoading = false;

    render(AdminDashboard);

    await waitFor(() => {
      expect(screen.getByText('Quick Actions')).toBeInTheDocument();
    });

    expect(screen.getByText('Create User')).toBeInTheDocument();
    expect(screen.getByText('Manage Users')).toBeInTheDocument();
    expect(screen.getByText('Manage Platforms')).toBeInTheDocument();
  });

  it('should call fetchStatistics on mount', async () => {
    render(AdminDashboard);

    await waitFor(() => {
      expect(admin.fetchStatistics).toHaveBeenCalled();
    });
  });

  it('should clear error when dismiss button is clicked', async () => {
    vi.mocked(admin).value.error = 'Some error';

    render(AdminDashboard);

    const dismissButton = screen.getByText('Dismiss');
    await dismissButton.click();

    expect(admin.clearError).toHaveBeenCalled();
  });

  it('should format dates correctly', async () => {
    const mockStatistics = {
      totalUsers: 1,
      totalAdmins: 0,
      totalGames: 0,
      recentUsers: [
        {
          id: '1',
          username: 'testuser',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-06-15T14:30:00Z',
          updatedAt: '2023-06-15T14:30:00Z'
        }
      ]
    };

    vi.mocked(admin.fetchStatistics).mockResolvedValue(mockStatistics);
    vi.mocked(admin).value.statistics = mockStatistics;
    vi.mocked(admin).value.isLoading = false;

    render(AdminDashboard);

    // Check that date is formatted (exact format may vary by locale)
    await waitFor(() => {
      expect(screen.getByText(/Created.*2023/)).toBeInTheDocument();
    });
  });

  it('should show view all users link when recent users exist', async () => {
    const mockStatistics = {
      totalUsers: 1,
      totalAdmins: 0,
      totalGames: 0,
      recentUsers: [
        {
          id: '1',
          username: 'testuser',
          isAdmin: false,
          isActive: true,
          createdAt: '2023-01-01T00:00:00Z',
          updatedAt: '2023-01-01T00:00:00Z'
        }
      ]
    };

    vi.mocked(admin.fetchStatistics).mockResolvedValue(mockStatistics);
    vi.mocked(admin).value.statistics = mockStatistics;
    vi.mocked(admin).value.isLoading = false;

    render(AdminDashboard);

    await waitFor(() => {
      expect(screen.getByText('View all users')).toBeInTheDocument();
    });
  });

  it('should not show recent users section when no recent users', async () => {
    const mockStatistics = {
      totalUsers: 0,
      totalAdmins: 0,
      totalGames: 0,
      recentUsers: []
    };

    vi.mocked(admin).value.statistics = mockStatistics;

    render(AdminDashboard);

    expect(screen.queryByText('Recent Users')).not.toBeInTheDocument();
  });
});