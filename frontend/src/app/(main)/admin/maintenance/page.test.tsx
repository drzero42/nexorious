import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import MaintenancePage from './page';

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
  loadSeedData: vi.fn(),
}));

import { useAuth } from '@/providers';
import * as adminApi from '@/api/admin';
import { toast } from 'sonner';

const mockedUseAuth = vi.mocked(useAuth);
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

describe('MaintenancePage', () => {
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
        error: null,
        clearError: vi.fn(),
      });

      const { container } = render(<MaintenancePage />);

      expect(container.firstChild).toBeNull();
    });

    it('redirects non-admin users to dashboard', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockNonAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/dashboard');
      });
    });
  });

  describe('Loading State', () => {
    it('renders loading skeleton when not yet loaded', () => {
      mockedUseAuth.mockReturnValue({
        user: null,
        isLoading: true,
        isAuthenticated: false,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      const { container } = render(<MaintenancePage />);

      // Should show nothing since user is null and not admin
      expect(container.firstChild).toBeNull();
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
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('System Maintenance')).toBeInTheDocument();
      });

      expect(
        screen.getByText(/administrative tools for database maintenance and data management/i)
      ).toBeInTheDocument();
    });

    it('renders breadcrumb navigation', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('System Maintenance')).toBeInTheDocument();
      });

      expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/dashboard');
      expect(screen.getByRole('link', { name: 'Admin' })).toHaveAttribute('href', '/admin');
      expect(screen.getByText('Maintenance')).toBeInTheDocument();
    });
  });

  describe('Seed Data Section', () => {
    it('renders seed data card with title and description', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      expect(
        screen.getByText(/load official platforms, storefronts, and default mappings/i)
      ).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /load seed data/i })).toBeInTheDocument();
    });

    it('shows idempotent message', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      expect(
        screen.getByText(/this operation is idempotent and safe to run multiple times/i)
      ).toBeInTheDocument();
    });

    it('loads seed data when button is clicked', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedLoadSeedData.mockResolvedValue({
        platformsAdded: 5,
        storefrontsAdded: 10,
        mappingsCreated: 15,
        totalChanges: 30,
        message: 'Seed data loaded successfully',
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      const loadButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadButton);

      await waitFor(() => {
        expect(mockedLoadSeedData).toHaveBeenCalled();
      });

      expect(toast.success).toHaveBeenCalledWith('Seed data loaded successfully');
    });

    it('displays seed data result after successful load', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedLoadSeedData.mockResolvedValue({
        platformsAdded: 5,
        storefrontsAdded: 10,
        mappingsCreated: 15,
        totalChanges: 30,
        message: 'Seed data loaded successfully',
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      const loadButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadButton);

      await waitFor(() => {
        expect(screen.getByText('Success')).toBeInTheDocument();
      });

      expect(screen.getByText(/5 platforms/i)).toBeInTheDocument();
      expect(screen.getByText(/10 storefronts/i)).toBeInTheDocument();
      expect(screen.getByText(/15 mappings/i)).toBeInTheDocument();
    });

    it('shows error toast when seed data load fails', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedLoadSeedData.mockRejectedValue(new Error('Failed to load seed data'));

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      const loadButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadButton);

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('Failed to load seed data');
      });
    });

    it('shows loading state while seed data is loading', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedLoadSeedData.mockImplementation(() => new Promise(() => {})); // Never resolves

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Seed Data')).toBeInTheDocument();
      });

      const loadButton = screen.getByRole('button', { name: /load seed data/i });
      await userEvent.click(loadButton);

      await waitFor(() => {
        expect(screen.getByText('Loading...')).toBeInTheDocument();
      });
    });
  });

  describe('Database Cleanup Section', () => {
    it('renders database cleanup card with title and description', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Database Cleanup')).toBeInTheDocument();
      });

      expect(screen.getByText(/remove orphaned data and expired records/i)).toBeInTheDocument();
    });

    it('renders orphaned files cleanup option', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Database Cleanup')).toBeInTheDocument();
      });

      expect(screen.getByText('Orphaned Files')).toBeInTheDocument();
      expect(screen.getByText(/remove cover art not linked to any game/i)).toBeInTheDocument();
    });

    it('renders expired jobs cleanup option', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Database Cleanup')).toBeInTheDocument();
      });

      expect(screen.getByText('Expired Jobs')).toBeInTheDocument();
      expect(screen.getByText(/clean up job data older than 7 days/i)).toBeInTheDocument();
    });

    it('shows coming soon buttons for cleanup options', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Database Cleanup')).toBeInTheDocument();
      });

      const comingSoonButtons = screen.getAllByRole('button', { name: /coming soon/i });
      expect(comingSoonButtons).toHaveLength(2);
      comingSoonButtons.forEach((button) => {
        expect(button).toBeDisabled();
      });
    });
  });

  describe('IGDB Data Refresh Section', () => {
    it('renders IGDB refresh card with title and description', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      expect(
        screen.getByText(/update game metadata from igdb across your collection/i)
      ).toBeInTheDocument();
    });

    it('shows coming soon alert for IGDB refresh', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      // Check for the alert title (h5 element) - use getAllByText since "Coming Soon" appears multiple times
      const comingSoonElements = screen.getAllByText('Coming Soon');
      // There should be at least 3: 2 buttons + 1 alert title
      expect(comingSoonElements.length).toBeGreaterThanOrEqual(3);
      expect(
        screen.getByText(/igdb data refresh functionality will be available in a future update/i)
      ).toBeInTheDocument();
    });
  });

  describe('Recent Maintenance Jobs Section', () => {
    it('renders recent maintenance jobs card', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Recent Maintenance Jobs')).toBeInTheDocument();
      });

      expect(screen.getByText(/maintenance operations from the last 7 days/i)).toBeInTheDocument();
    });

    it('shows empty state for maintenance jobs', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('Recent Maintenance Jobs')).toBeInTheDocument();
      });

      expect(screen.getByText('No recent maintenance jobs')).toBeInTheDocument();
      expect(
        screen.getByText(/jobs will appear here after running maintenance tasks/i)
      ).toBeInTheDocument();
    });
  });
});
