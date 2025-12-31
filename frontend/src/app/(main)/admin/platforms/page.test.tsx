import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@/test/test-utils';
import userEvent from '@testing-library/user-event';
import AdminPlatformsPage from './page';

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

// Mock the platforms API
vi.mock('@/api/platforms', () => ({
  getPlatforms: vi.fn(),
  getStorefronts: vi.fn(),
  getPlatformStorefronts: vi.fn(),
  createPlatform: vi.fn(),
  updatePlatform: vi.fn(),
  deletePlatform: vi.fn(),
  createStorefront: vi.fn(),
  updateStorefront: vi.fn(),
  deleteStorefront: vi.fn(),
  createPlatformStorefrontAssociation: vi.fn(),
  deletePlatformStorefrontAssociation: vi.fn(),
}));

import { useAuth } from '@/providers';
import * as platformsApi from '@/api/platforms';
import { toast } from 'sonner';

const mockedUseAuth = vi.mocked(useAuth);
const mockedGetPlatforms = vi.mocked(platformsApi.getPlatforms);
const mockedGetStorefronts = vi.mocked(platformsApi.getStorefronts);
const mockedGetPlatformStorefronts = vi.mocked(platformsApi.getPlatformStorefronts);
const mockedCreatePlatform = vi.mocked(platformsApi.createPlatform);
const mockedUpdatePlatform = vi.mocked(platformsApi.updatePlatform);
const mockedDeletePlatform = vi.mocked(platformsApi.deletePlatform);
const mockedCreateStorefront = vi.mocked(platformsApi.createStorefront);
const mockedUpdateStorefront = vi.mocked(platformsApi.updateStorefront);
const mockedDeleteStorefront = vi.mocked(platformsApi.deleteStorefront);

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

const mockPlatforms = [
  {
    name: 'pc',
    display_name: 'PC',
    icon_url: '/icons/pc.svg',
    is_active: true,
    source: 'official',
    default_storefront: 'steam',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'playstation',
    display_name: 'PlayStation',
    icon_url: '/icons/ps.svg',
    is_active: false,
    source: 'custom',
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-02T00:00:00Z',
  },
];

const mockStorefronts = [
  {
    name: 'steam',
    display_name: 'Steam',
    icon_url: '/icons/steam.svg',
    base_url: 'https://store.steampowered.com',
    is_active: true,
    source: 'official',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    name: 'epic',
    display_name: 'Epic Games Store',
    icon_url: '/icons/epic.svg',
    base_url: 'https://store.epicgames.com',
    is_active: true,
    source: 'official',
    created_at: '2024-01-02T00:00:00Z',
    updated_at: '2024-01-02T00:00:00Z',
  },
  {
    name: 'gog',
    display_name: 'GOG',
    is_active: false,
    source: 'custom',
    created_at: '2024-01-03T00:00:00Z',
    updated_at: '2024-01-03T00:00:00Z',
  },
];

describe('AdminPlatformsPage', () => {
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

      const { container } = render(<AdminPlatformsPage />);

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

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(mockReplace).toHaveBeenCalledWith('/dashboard');
      });
    });
  });

  describe('Loading State', () => {
    it('renders loading skeleton when data is loading', () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockImplementation(() => new Promise(() => {}));
      mockedGetStorefronts.mockImplementation(() => new Promise(() => {}));

      render(<AdminPlatformsPage />);

      const skeletons = document.querySelectorAll('[class*="animate-pulse"]');
      expect(skeletons.length).toBeGreaterThan(0);
    });
  });

  describe('Error State', () => {
    it('displays error message when loading fails', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockRejectedValue(new Error('Failed to load platforms'));
      mockedGetStorefronts.mockResolvedValue({
        storefronts: [],
        total: 0,
        page: 1,
        perPage: 100,
        pages: 1,
      });

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('Failed to load platforms')).toBeInTheDocument();
      });
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

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('Platform & Storefront Management')).toBeInTheDocument();
      });

      expect(
        screen.getByText(/manage available platforms and storefronts/i)
      ).toBeInTheDocument();
    });
  });

  describe('Tabs Navigation', () => {
    it('renders all three tabs', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByRole('tab', { name: /platforms/i })).toBeInTheDocument();
      });

      expect(screen.getByRole('tab', { name: /storefronts/i })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: /associations/i })).toBeInTheDocument();
    });

    it('shows platforms tab by default', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('Platforms (2)')).toBeInTheDocument();
      });

      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.getByText('PlayStation')).toBeInTheDocument();
    });
  });

  describe('Platforms Tab', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('displays all platforms in a table', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      expect(screen.getByText('PlayStation')).toBeInTheDocument();
    });

    it('shows status badges for platforms', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Check for Active and Inactive badges - there are multiple because of the status toggle buttons
      const activeButtons = screen.getAllByRole('button', { name: /active/i });
      const inactiveButtons = screen.getAllByRole('button', { name: /inactive/i });
      expect(activeButtons.length).toBeGreaterThan(0);
      expect(inactiveButtons.length).toBeGreaterThan(0);
    });

    it('shows source badges for platforms', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      expect(screen.getByText('official')).toBeInTheDocument();
      expect(screen.getByText('custom')).toBeInTheDocument();
    });

    it('has Add Platform button', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      expect(screen.getByRole('button', { name: /add platform/i })).toBeInTheDocument();
    });

    it('opens create dialog when Add Platform is clicked', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const addButton = screen.getByRole('button', { name: /add platform/i });
      await userEvent.click(addButton);

      await waitFor(() => {
        expect(screen.getByText('Create New Platform')).toBeInTheDocument();
      });

      // Use document.getElementById since labels have specific IDs
      expect(document.getElementById('platform-name')).toBeInTheDocument();
      expect(document.getElementById('platform-display-name')).toBeInTheDocument();
    });

    it('filters platforms by search query', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText(/search platforms/i);
      await userEvent.type(searchInput, 'PC');

      expect(screen.getByText('PC')).toBeInTheDocument();
      expect(screen.queryByText('PlayStation')).not.toBeInTheDocument();
    });

    it('shows empty state when no platforms match search', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const searchInput = screen.getByPlaceholderText(/search platforms/i);
      await userEvent.type(searchInput, 'NonExistent');

      expect(screen.getByText('No platforms found')).toBeInTheDocument();
    });
  });

  describe('Storefronts Tab', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('switches to storefronts tab when clicked', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const storefrontsTab = screen.getByRole('tab', { name: /storefronts/i });
      await userEvent.click(storefrontsTab);

      await waitFor(() => {
        expect(screen.getByText('Storefronts (3)')).toBeInTheDocument();
      });

      expect(screen.getByText('Steam')).toBeInTheDocument();
      expect(screen.getByText('Epic Games Store')).toBeInTheDocument();
      expect(screen.getByText('GOG')).toBeInTheDocument();
    });

    it('displays base URLs for storefronts', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const storefrontsTab = screen.getByRole('tab', { name: /storefronts/i });
      await userEvent.click(storefrontsTab);

      await waitFor(() => {
        expect(screen.getByText('Steam')).toBeInTheDocument();
      });

      expect(screen.getByText('https://store.steampowered.com')).toBeInTheDocument();
    });

    it('has Add Storefront button', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const storefrontsTab = screen.getByRole('tab', { name: /storefronts/i });
      await userEvent.click(storefrontsTab);

      await waitFor(() => {
        expect(screen.getByRole('button', { name: /add storefront/i })).toBeInTheDocument();
      });
    });

    it('opens create dialog when Add Storefront is clicked', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      const storefrontsTab = screen.getByRole('tab', { name: /storefronts/i });
      await userEvent.click(storefrontsTab);

      await waitFor(() => {
        expect(screen.getByText('Steam')).toBeInTheDocument();
      });

      const addButton = screen.getByRole('button', { name: /add storefront/i });
      await userEvent.click(addButton);

      expect(screen.getByText('Create New Storefront')).toBeInTheDocument();
      expect(screen.getByLabelText(/storefront name/i)).toBeInTheDocument();
    });
  });

  describe('Status Filter', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('filters by active status', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Open status filter dropdown - use the select trigger by its placeholder text
      const statusSelect = screen.getByRole('combobox');
      await userEvent.click(statusSelect);

      // Select Active Only - there may be multiple, pick the first one
      await waitFor(() => {
        const options = screen.getAllByRole('option', { name: /active only/i });
        expect(options.length).toBeGreaterThan(0);
      });
      const activeOptions = screen.getAllByRole('option', { name: /active only/i });
      await userEvent.click(activeOptions[0]);

      // Wait for filter to apply
      await waitFor(() => {
        expect(screen.queryByText('PlayStation')).not.toBeInTheDocument();
      });

      // Should only show active platform (PC)
      expect(screen.getByText('PC')).toBeInTheDocument();
    });

    it('filters by inactive status', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Open status filter dropdown
      const statusSelect = screen.getByRole('combobox');
      await userEvent.click(statusSelect);

      // Select Inactive Only
      const inactiveOption = screen.getByRole('option', { name: /inactive only/i });
      await userEvent.click(inactiveOption);

      // Should only show inactive platform (PlayStation)
      expect(screen.queryByText('PC')).not.toBeInTheDocument();
      expect(screen.getByText('PlayStation')).toBeInTheDocument();
    });
  });

  describe('Create Platform', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('creates a new platform successfully', async () => {
      const newPlatform = {
        name: 'new_platform',
        display_name: 'New Platform',
        is_active: true,
        source: 'custom',
        created_at: '2024-01-05T00:00:00Z',
        updated_at: '2024-01-05T00:00:00Z',
      };

      mockedCreatePlatform.mockResolvedValue(newPlatform);

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Open create dialog
      const addButton = screen.getByRole('button', { name: /add platform/i });
      await userEvent.click(addButton);

      await waitFor(() => {
        expect(screen.getByText('Create New Platform')).toBeInTheDocument();
      });

      // Fill form using specific element IDs
      const nameInput = document.getElementById('platform-name') as HTMLInputElement;
      const displayNameInput = document.getElementById('platform-display-name') as HTMLInputElement;
      await userEvent.type(nameInput, 'new_platform');
      await userEvent.type(displayNameInput, 'New Platform');

      // Submit
      const createButton = screen.getByRole('button', { name: /^create$/i });
      await userEvent.click(createButton);

      await waitFor(() => {
        expect(mockedCreatePlatform).toHaveBeenCalledWith(
          expect.objectContaining({
            name: 'new_platform',
            display_name: 'New Platform',
          })
        );
      });

      expect(toast.success).toHaveBeenCalledWith('Platform created successfully');
    });

    it('shows validation error when required fields are empty', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Open create dialog
      const addButton = screen.getByRole('button', { name: /add platform/i });
      await userEvent.click(addButton);

      await waitFor(() => {
        expect(screen.getByText('Create New Platform')).toBeInTheDocument();
      });

      // Try to submit without filling form
      const createButton = screen.getByRole('button', { name: /^create$/i });
      await userEvent.click(createButton);

      expect(toast.error).toHaveBeenCalledWith('Name and display name are required');
      expect(mockedCreatePlatform).not.toHaveBeenCalled();
    });
  });

  describe('Edit Platform', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('opens edit dialog with pre-filled data', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Find and click the first edit button (for PC)
      const table = screen.getByRole('table');
      const rows = within(table).getAllByRole('row');
      // First row is header, second row is PC
      const pcRow = rows[1];
      const buttons = within(pcRow).getAllByRole('button');
      // First button with Pencil icon
      const editBtnWithPencil = buttons.find(
        (btn) => btn.querySelector('svg.lucide-pencil')
      );
      expect(editBtnWithPencil).toBeTruthy();
      if (editBtnWithPencil) {
        await userEvent.click(editBtnWithPencil);
      }

      await waitFor(() => {
        expect(screen.getByText('Edit Platform')).toBeInTheDocument();
      });

      // Use document.getElementById for the display name input
      const displayNameInput = document.getElementById('platform-display-name') as HTMLInputElement;
      expect(displayNameInput.value).toBe('PC');
    });
  });

  describe('Delete Platform', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('shows delete confirmation dialog', async () => {
      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Find and click the delete button for PC
      const table = screen.getByRole('table');
      const rows = within(table).getAllByRole('row');
      const pcRow = rows[1];
      const buttons = within(pcRow).getAllByRole('button');
      // Find button with Trash2 icon
      const deleteBtnWithTrash = buttons.find(
        (btn) => btn.querySelector('svg.lucide-trash-2')
      );
      if (deleteBtnWithTrash) {
        await userEvent.click(deleteBtnWithTrash);
      }

      await waitFor(() => {
        expect(screen.getByText('Confirm Deletion')).toBeInTheDocument();
      });

      expect(screen.getByText(/are you sure you want to delete/i)).toBeInTheDocument();
    });

    it('deletes platform when confirmed', async () => {
      mockedDeletePlatform.mockResolvedValue(undefined);

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Find and click the delete button for PC
      const table = screen.getByRole('table');
      const rows = within(table).getAllByRole('row');
      const pcRow = rows[1];
      const buttons = within(pcRow).getAllByRole('button');
      const deleteBtnWithTrash = buttons.find(
        (btn) => btn.querySelector('svg.lucide-trash-2')
      );
      if (deleteBtnWithTrash) {
        await userEvent.click(deleteBtnWithTrash);
      }

      await waitFor(() => {
        expect(screen.getByText('Confirm Deletion')).toBeInTheDocument();
      });

      // Click the Delete button in the confirmation dialog
      const confirmDeleteBtn = screen.getByRole('button', { name: /^delete$/i });
      await userEvent.click(confirmDeleteBtn);

      await waitFor(() => {
        expect(mockedDeletePlatform).toHaveBeenCalledWith('pc');
      });

      expect(toast.success).toHaveBeenCalledWith('Platform deleted');
    });
  });

  describe('Toggle Status', () => {
    beforeEach(() => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: mockPlatforms,
        total: 2,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: mockStorefronts,
        total: 3,
        page: 1,
        perPage: 100,
        pages: 1,
      });
    });

    it('toggles platform status when badge is clicked', async () => {
      const updatedPlatform = {
        ...mockPlatforms[0],
        is_active: false,
      };
      mockedUpdatePlatform.mockResolvedValue(updatedPlatform);

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('PC')).toBeInTheDocument();
      });

      // Find the Active badge button in the table (first one is for PC)
      const activeButtons = screen.getAllByRole('button', { name: /active/i });
      // The first Active button should be for the PC platform
      await userEvent.click(activeButtons[0]);

      await waitFor(() => {
        expect(mockedUpdatePlatform).toHaveBeenCalledWith('pc', {
          is_active: false,
        });
      });
    });
  });

  describe('Empty State', () => {
    it('shows empty state when no platforms exist', async () => {
      mockedUseAuth.mockReturnValue({
        user: mockAdminUser,
        isLoading: false,
        isAuthenticated: true,
        login: vi.fn(),
        logout: vi.fn(),
        error: null, clearError: vi.fn(),
      });

      mockedGetPlatforms.mockResolvedValue({
        platforms: [],
        total: 0,
        page: 1,
        perPage: 100,
        pages: 1,
      });
      mockedGetStorefronts.mockResolvedValue({
        storefronts: [],
        total: 0,
        page: 1,
        perPage: 100,
        pages: 1,
      });

      render(<AdminPlatformsPage />);

      await waitFor(() => {
        expect(screen.getByText('No platforms found')).toBeInTheDocument();
      });
    });
  });
});
