import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import AdminDashboardPage from './page';

// Mock next/navigation
const mockPush = vi.fn();
const mockReplace = vi.fn();
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: mockReplace,
  }),
}));

// Mock sonner toast
vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock the useAuth hook
vi.mock('@/providers', () => ({
  useAuth: vi.fn(),
}));

// Mock the admin API
vi.mock('@/api/admin', () => ({
  getAdminStatistics: vi.fn(),
  loadSeedData: vi.fn(),
}));

import { useAuth } from '@/providers';
import * as adminApi from '@/api/admin';
import { toast } from 'sonner';

const mockedUseAuth = vi.mocked(useAuth);
const mockedGetAdminStatistics = vi.mocked(adminApi.getAdminStatistics);
const mockedLoadSeedData = vi.mocked(adminApi.loadSeedData);

const mockAdminUser = {
  id: 'user-1',
  username: 'admin',
  isAdmin: true,
  isActive: true,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
};

const mockNonAdminUser = {
  id: 'user-2',
  username: 'regular',
  isAdmin: false,
  isActive: true,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
};

const mockStatistics = {
  totalUsers: 10,
  totalAdmins: 2,
  totalGames: 50,
  recentUsers: [
    {
      id: 'user-1',
      username: 'newest_user',
      isAdmin: false,
      isActive: true,
      createdAt: '2024-01-15T10:00:00Z',
      updatedAt: '2024-01-15T10:00:00Z',
    },
    {
      id: 'user-2',
      username: 'admin_user',
      isAdmin: true,
      isActive: true,
      createdAt: '2024-01-14T10:00:00Z',
      updatedAt: '2024-01-14T10:00:00Z',
    },
    {
      id: 'user-3',
      username: 'inactive_user',
      isAdmin: false,
      isActive: false,
      createdAt: '2024-01-13T10:00:00Z',
      updatedAt: '2024-01-13T10:00:00Z',
    },
  ],
};

describe('AdminDashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPush.mockClear();
    mockReplace.mockClear();
  });

  describe('Access Control', () => {
    it('renders nothing when user is not admin', () => {
      mockedUseAuth.mockReturnValue({
        user: mockNonAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      const { container } = render(<AdminDashboardPage />);

      expect(container.firstChild).toBeNull();
    });

    it('redirects non-admin users to dashboard', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockNonAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/dashboard');
      });
    });
  });

  describe('Loading State', () => {
    it('renders loading skeleton when statistics are loading', () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockImplementation(() => new Promise(() => {})); // Never resolves

      render(<AdminDashboardPage />);

      // Should show skeleton loaders
      const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
      expect(skeletons.length).toBeGreaterThan(0);
    });
  });

  describe('Error State', () => {
    it('displays error message when statistics loading fails', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockRejectedValue(new Error('Failed to fetch statistics'));

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText(/error loading dashboard/i)).toBeInTheDocument();
      });

      expect(screen.getByText('Failed to fetch statistics')).toBeInTheDocument();
    });

    it('can dismiss error message', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockRejectedValue(new Error('Failed to fetch statistics'));

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText(/error loading dashboard/i)).toBeInTheDocument();
      });

      const dismissButton = screen.getByRole('button', { name: /dismiss/i });
      await userEvent.click(dismissButton);

      expect(screen.queryByText(/error loading dashboard/i)).not.toBeInTheDocument();
    });
  });

  describe('Page Header', () => {
    it('renders page title and description', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Admin Dashboard')).toBeInTheDocument();
      });

      expect(screen.getByText(/system overview and management tools/i)).toBeInTheDocument();
    });
  });

  describe('Statistics Cards', () => {
    it('displays correct statistics values', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Total Users')).toBeInTheDocument();
      });

      // Total Users: 10
      expect(screen.getByText('10')).toBeInTheDocument();

      // Admin Users: 2
      expect(screen.getByText('Admin Users')).toBeInTheDocument();
      expect(screen.getByText('2')).toBeInTheDocument();

      // Total Games: 50
      expect(screen.getByText('Total Games')).toBeInTheDocument();
      expect(screen.getByText('50')).toBeInTheDocument();

      // System Status: Healthy
      expect(screen.getByText('System Status')).toBeInTheDocument();
      expect(screen.getByText('Healthy')).toBeInTheDocument();
    });

    it('shows N/A for games when count is 0', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue({
        ...mockStatistics,
        totalGames: 0,
      });

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Total Games')).toBeInTheDocument();
      });

      expect(screen.getByText('N/A')).toBeInTheDocument();
    });
  });

  describe('Recent Users Section', () => {
    it('displays recent users', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Recent Users')).toBeInTheDocument();
      });

      expect(screen.getByText('newest_user')).toBeInTheDocument();
      expect(screen.getByText('admin_user')).toBeInTheDocument();
      expect(screen.getByText('inactive_user')).toBeInTheDocument();
    });

    it('displays Admin badge for admin users', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('admin_user')).toBeInTheDocument();
      });

      expect(screen.getByText('Admin')).toBeInTheDocument();
    });

    it('displays Inactive badge for inactive users', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('inactive_user')).toBeInTheDocument();
      });

      expect(screen.getByText('Inactive')).toBeInTheDocument();
    });

    it('has View button for each user', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Recent Users')).toBeInTheDocument();
      });

      // There are 3 View buttons for users + 1 "View all users" link = 4 total
      // Filter to only get the individual user View buttons (exact match)
      const viewButtons = screen.getAllByRole('link', { name: /^view$/i });
      expect(viewButtons).toHaveLength(3);
    });

    it('has View all users link', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Recent Users')).toBeInTheDocument();
      });

      expect(screen.getByRole('link', { name: /view all users/i })).toBeInTheDocument();
    });

    it('does not display Recent Users section when empty', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue({
        ...mockStatistics,
        recentUsers: [],
      });

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Admin Dashboard')).toBeInTheDocument();
      });

      expect(screen.queryByText('Recent Users')).not.toBeInTheDocument();
    });
  });

  describe('Quick Actions', () => {
    it('renders all quick action buttons', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      expect(screen.getByRole('link', { name: /create user/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /manage users/i })).toBeInTheDocument();
      expect(screen.getByRole('link', { name: /manage platforms/i })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /load seed data/i })).toBeInTheDocument();
    });

    it('navigates to correct pages from quick actions', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const createUserLink = screen.getByRole('link', { name: /create user/i });
      expect(createUserLink).toHaveAttribute('href', '/admin/users/new');

      const manageUsersLink = screen.getByRole('link', { name: /manage users/i });
      expect(manageUsersLink).toHaveAttribute('href', '/admin/users');

      const managePlatformsLink = screen.getByRole('link', { name: /manage platforms/i });
      expect(managePlatformsLink).toHaveAttribute('href', '/admin/platforms');
    });
  });

  describe('Seed Data Loading', () => {
    it('shows confirmation dialog before loading seed data', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);

      // Mock window.confirm to return false (user cancels)
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const loadSeedDataButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadSeedDataButton);

      expect(confirmSpy).toHaveBeenCalled();
      expect(mockedLoadSeedData).not.toHaveBeenCalled();

      confirmSpy.mockRestore();
    });

    it('loads seed data when confirmed', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);
      mockedLoadSeedData.mockResolvedValue({
        platformsAdded: 5,
        storefrontsAdded: 10,
        mappingsCreated: 15,
        totalChanges: 30,
        message: 'Seed data loaded successfully',
      });

      // Mock window.confirm to return true (user confirms)
      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const loadSeedDataButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadSeedDataButton);

      await waitFor(() => {
        expect(mockedLoadSeedData).toHaveBeenCalled();
      });

      expect(toast.success).toHaveBeenCalledWith('Seed data loaded successfully');

      confirmSpy.mockRestore();
    });

    it('displays seed data result after successful load', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);
      mockedLoadSeedData.mockResolvedValue({
        platformsAdded: 5,
        storefrontsAdded: 10,
        mappingsCreated: 15,
        totalChanges: 30,
        message: 'Seed data loaded successfully',
      });

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const loadSeedDataButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadSeedDataButton);

      await waitFor(() => {
        expect(screen.getByText('Seed Data Loading Complete')).toBeInTheDocument();
      });

      expect(screen.getByText(/5 platforms added/i)).toBeInTheDocument();
      expect(screen.getByText(/10 storefronts added/i)).toBeInTheDocument();
      expect(screen.getByText(/15 default mappings created/i)).toBeInTheDocument();

      confirmSpy.mockRestore();
    });

    it('shows error toast when seed data load fails', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);
      mockedLoadSeedData.mockRejectedValue(new Error('Failed to load seed data'));

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const loadSeedDataButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadSeedDataButton);

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('Failed to load seed data');
      });

      confirmSpy.mockRestore();
    });

    it('can dismiss seed data result', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetAdminStatistics.mockResolvedValue(mockStatistics);
      mockedLoadSeedData.mockResolvedValue({
        platformsAdded: 5,
        storefrontsAdded: 10,
        mappingsCreated: 15,
        totalChanges: 30,
        message: 'Seed data loaded successfully',
      });

      const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

      render(<AdminDashboardPage />);

      await waitFor(() => {
        expect(screen.getByText('Quick Actions')).toBeInTheDocument();
      });

      const loadSeedDataButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadSeedDataButton);

      await waitFor(() => {
        expect(screen.getByText('Seed Data Loading Complete')).toBeInTheDocument();
      });

      const dismissButton = screen.getByRole('button', { name: /dismiss/i });
      await userEvent.click(dismissButton);

      expect(screen.queryByText('Seed Data Loading Complete')).not.toBeInTheDocument();

      confirmSpy.mockRestore();
    });
  });
});
