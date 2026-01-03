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
  startMetadataRefreshJob: vi.fn(),
}));

// Mock the hooks
vi.mock('@/hooks', () => ({
  useActiveJob: vi.fn(),
  useCancelJob: vi.fn(),
  useJobs: vi.fn(() => ({
    data: { jobs: [], total: 0, page: 1, perPage: 50, pages: 0 },
    isLoading: false,
  })),
  useJobItems: vi.fn(() => ({
    data: null,
    isLoading: false,
    refetch: vi.fn(),
  })),
  useRetryFailedItems: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
  useRetryJobItem: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
  useResolveJobItem: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
  useSkipJobItem: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
}));

import { useAuth } from '@/providers';
import * as adminApi from '@/api/admin';
import { useActiveJob, useCancelJob } from '@/hooks';
import { toast } from 'sonner';

const mockedUseAuth = vi.mocked(useAuth);
const mockedLoadSeedData = vi.mocked(adminApi.loadSeedData);
const mockedStartMetadataRefreshJob = vi.mocked(adminApi.startMetadataRefreshJob);
const mockedUseActiveJob = vi.mocked(useActiveJob);
const mockedUseCancelJob = vi.mocked(useCancelJob);

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

    // Default mock implementations for hooks
    mockedUseActiveJob.mockReturnValue({
      data: null,
      refetch: vi.fn(),
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useActiveJob>);

    mockedUseCancelJob.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    } as unknown as ReturnType<typeof useCancelJob>);
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

    it('renders refresh button', async () => {
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

      expect(screen.getByRole('button', { name: /refresh all game metadata/i })).toBeInTheDocument();
    });

    it('starts metadata refresh job when button is clicked', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedStartMetadataRefreshJob.mockResolvedValue({
        success: true,
        message: 'Metadata refresh job started successfully',
        jobId: 'job-123',
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      const refreshButton = screen.getByRole('button', { name: /refresh all game metadata/i });
      await userEvent.click(refreshButton);

      await waitFor(() => {
        expect(mockedStartMetadataRefreshJob).toHaveBeenCalled();
      });

      expect(toast.success).toHaveBeenCalledWith('Metadata refresh job started');
    });

    it('refetches job status after starting metadata refresh', async () => {
      const mockRefetch = vi.fn();
      mockedUseActiveJob.mockReturnValue({
        data: null,
        refetch: mockRefetch,
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useActiveJob>);

      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedStartMetadataRefreshJob.mockResolvedValue({
        success: true,
        message: 'Metadata refresh job started successfully',
        jobId: 'job-123',
      });

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      const refreshButton = screen.getByRole('button', { name: /refresh all game metadata/i });
      await userEvent.click(refreshButton);

      await waitFor(() => {
        expect(mockRefetch).toHaveBeenCalled();
      });
    });

    it('shows error toast when metadata refresh fails', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedStartMetadataRefreshJob.mockRejectedValue(new Error('Failed to start metadata refresh'));

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      const refreshButton = screen.getByRole('button', { name: /refresh all game metadata/i });
      await userEvent.click(refreshButton);

      await waitFor(() => {
        expect(toast.error).toHaveBeenCalledWith('Failed to start metadata refresh');
      });
    });

    it('shows loading state while job is starting', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null,
        clearError: vi.fn(),
      });

      mockedStartMetadataRefreshJob.mockImplementation(() => new Promise(() => {})); // Never resolves

      render(<MaintenancePage />);

      await waitFor(() => {
        expect(screen.getByText('IGDB Data Refresh')).toBeInTheDocument();
      });

      const refreshButton = screen.getByRole('button', { name: /refresh all game metadata/i });
      await userEvent.click(refreshButton);

      await waitFor(() => {
        expect(screen.getByText('Starting...')).toBeInTheDocument();
      });
    });

    it('displays job progress card when there is an active job', async () => {
      mockedUseActiveJob.mockReturnValue({
        data: {
          id: 'job-123',
          jobType: 'maintenance',
          source: 'system',
          status: 'processing',
          progress: {
            total: 100,
            completed: 25,
            failed: 0,
            pending: 75,
            processing: 0,
            skipped: 0,
            pendingReview: 0,
            percent: 25,
          },
          isTerminal: false,
          totalItems: 100,
          errorMessage: null,
          filePath: null,
          createdAt: '2024-01-01T00:00:00Z',
          startedAt: '2024-01-01T00:00:00Z',
          completedAt: null,
          durationSeconds: null,
          userId: 'user-1',
          priority: 'high',
        },
        refetch: vi.fn(),
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useActiveJob>);

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
        // Should show Cancel button for in-progress job
        expect(screen.getByRole('button', { name: /cancel/i })).toBeInTheDocument();
      });

      // IGDB Data Refresh card should be hidden when job is in progress
      expect(screen.queryByRole('button', { name: /refresh all game metadata/i })).not.toBeInTheDocument();
    });

    it('shows Start New button when job is completed', async () => {
      mockedUseActiveJob.mockReturnValue({
        data: {
          id: 'job-123',
          jobType: 'maintenance',
          source: 'system',
          status: 'completed',
          progress: {
            total: 100,
            completed: 100,
            failed: 0,
            pending: 0,
            processing: 0,
            skipped: 0,
            pendingReview: 0,
            percent: 100,
          },
          isTerminal: true,
          totalItems: 100,
          errorMessage: null,
          filePath: null,
          createdAt: '2024-01-01T00:00:00Z',
          startedAt: '2024-01-01T00:00:00Z',
          completedAt: '2024-01-01T00:01:00Z',
          durationSeconds: 60,
          userId: 'user-1',
          priority: 'high',
        },
        refetch: vi.fn(),
        isLoading: false,
        error: null,
      } as unknown as ReturnType<typeof useActiveJob>);

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
        expect(screen.getByRole('button', { name: /start new/i })).toBeInTheDocument();
      });
    });
  });

  describe('Recent Maintenance Jobs Section', () => {
    it('renders recent activity card', async () => {
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
        expect(screen.getByText('Recent Activity')).toBeInTheDocument();
      });
    });

    it('shows empty state when no recent jobs', async () => {
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
        expect(screen.getByText('Recent Activity')).toBeInTheDocument();
      });

      expect(screen.getByText('No recent activity')).toBeInTheDocument();
    });
  });
});
