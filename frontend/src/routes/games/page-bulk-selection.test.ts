import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../test-utils/api-mocks';
import { mockUserGamesStore, mockPlatformsStore, mockUserGames, resetStoresMocks } from '../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../test-utils/auth-mocks';
import GamesPage from './+page.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

// Mock image-url utility
vi.mock('$lib/utils/image-url', () => ({
  resolveImageUrl: vi.fn((url) => url)
}));

describe('Games Page - Bulk Selection - Working Tests', () => {
  
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();

    // Setup mock data for user games with correct structure
    mockUserGamesStore.value = {
      userGames: mockUserGames,
      currentUserGame: null,
      stats: null,
      isLoading: false,
      error: null,
      filters: {},
      pagination: {
        page: 1,
        per_page: 20,
        total: 2,
        pages: 1
      }
    };

    // Add required methods to mocks
    mockUserGamesStore.fetchUserGames = vi.fn().mockResolvedValue(undefined);
    mockUserGamesStore.loadUserGames = vi.fn().mockResolvedValue(undefined);
    mockUserGamesStore.updateProgress = vi.fn().mockResolvedValue([]);
    mockUserGamesStore.bulkUpdateStatus = vi.fn().mockResolvedValue([]);
    mockPlatformsStore.fetchPlatforms = vi.fn().mockResolvedValue(undefined);
  });

  describe('Basic Rendering', () => {
    it('should render games page successfully', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText('My Games')).toBeInTheDocument();
        expect(screen.getByText(/2 unique games across all platforms in your collection/)).toBeInTheDocument();
      });
    });

    it('should show select all button', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });
    });

    it('should show game titles', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText('Test Game')).toBeInTheDocument();
        expect(screen.getByText('Another Game')).toBeInTheDocument();
        expect(screen.getByText('Third Game')).toBeInTheDocument();
      });
    });
  });

  describe('Checkbox Functionality', () => {
    it('should have game selection checkboxes in grid view', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        // Look for checkboxes with aria-label that includes "Select"
        const gameCheckboxes = screen.getAllByRole('checkbox').filter(checkbox => 
          checkbox.getAttribute('aria-label')?.includes('Select')
        );
        expect(gameCheckboxes).toHaveLength(3); // One for each game
      });
    });

    it('should check individual game checkbox when clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        const testGameCheckbox = screen.getByLabelText('Select Test Game');
        expect(testGameCheckbox).not.toBeChecked();
      });

      const testGameCheckbox = screen.getByLabelText('Select Test Game');
      await fireEvent.click(testGameCheckbox);
      
      expect(testGameCheckbox).toBeChecked();
    });

    it('should work with multiple game selections', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
        expect(screen.getByLabelText('Select Another Game')).toBeInTheDocument();
      });

      const testGameCheckbox = screen.getByLabelText('Select Test Game');
      const anotherGameCheckbox = screen.getByLabelText('Select Another Game');
      
      // Select first game
      await fireEvent.click(testGameCheckbox);
      expect(testGameCheckbox).toBeChecked();
      expect(anotherGameCheckbox).not.toBeChecked();
      
      // Select second game
      await fireEvent.click(anotherGameCheckbox);
      expect(testGameCheckbox).toBeChecked();
      expect(anotherGameCheckbox).toBeChecked();
      
      // Deselect first game
      await fireEvent.click(testGameCheckbox);
      expect(testGameCheckbox).not.toBeChecked();
      expect(anotherGameCheckbox).toBeChecked();
    });
  });

  describe('Select All Functionality', () => {
    it('should show deselect all button after select all is clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      // After clicking select all, should show "Deselect All" button
      await waitFor(() => {
        expect(screen.getByText(/deselect all/i)).toBeInTheDocument();
      });
      
      // Select All button should no longer be visible since all games are selected
      expect(screen.queryByText('Select All')).not.toBeInTheDocument();
    });

    it('should check all game checkboxes when select all is clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      
      // Initially, no checkboxes should be checked
      const gameCheckboxes = [
        screen.getByLabelText('Select Test Game'),
        screen.getByLabelText('Select Another Game'),
        screen.getByLabelText('Select Third Game')
      ];
      
      gameCheckboxes.forEach(checkbox => {
        expect(checkbox).not.toBeChecked();
      });
      
      // Click select all
      await fireEvent.click(selectAllButton);
      
      // All game checkboxes should now be checked
      gameCheckboxes.forEach(checkbox => {
        expect(checkbox).toBeChecked();
      });
    });

    it('should show selected count and bulk edit after selecting games', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      // Should show selected count and bulk edit
      await waitFor(() => {
        expect(screen.getByText(/3 selected/)).toBeInTheDocument();
        expect(screen.getByText(/bulk edit/i)).toBeInTheDocument();
      });
    });
  });

  describe('List View', () => {
    it('should switch to list view and maintain checkbox functionality', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      // Switch to list view
      const listViewButton = screen.getByRole('button', { name: /list view/i });
      await fireEvent.click(listViewButton);
      
      await waitFor(() => {
        // Should have table structure
        expect(screen.getByRole('table')).toBeInTheDocument();
        
        // Should still have game checkboxes
        expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
        expect(screen.getByLabelText('Select Another Game')).toBeInTheDocument();
        expect(screen.getByLabelText('Select Third Game')).toBeInTheDocument();
        
        // Should have select all checkbox in header
        expect(screen.getByLabelText('Select all games')).toBeInTheDocument();
      });
    });

    it('should work with header select all checkbox in list view', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      // Switch to list view
      const listViewButton = screen.getByRole('button', { name: /list view/i });
      await fireEvent.click(listViewButton);
      
      await waitFor(() => {
        expect(screen.getByLabelText('Select all games')).toBeInTheDocument();
      });

      const headerSelectAllCheckbox = screen.getByLabelText('Select all games');
      await fireEvent.click(headerSelectAllCheckbox);
      
      // All game checkboxes should be checked
      await waitFor(() => {
        expect(screen.getByLabelText('Select Test Game')).toBeChecked();
        expect(screen.getByLabelText('Select Another Game')).toBeChecked();
        expect(screen.getByLabelText('Select Third Game')).toBeChecked();
      });
    });
  });

  describe('Bulk Operations Modal', () => {
    it('should open bulk operations modal when bulk edit button is clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      // Select all games
      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      await waitFor(() => {
        expect(screen.getByText(/bulk edit/i)).toBeInTheDocument();
      });

      // Click bulk edit button
      const bulkEditButton = screen.getByText(/bulk edit/i);
      await fireEvent.click(bulkEditButton);
      
      // Modal should open
      await waitFor(() => {
        expect(screen.getByText('Bulk Operations')).toBeInTheDocument();
        expect(screen.getByText(/update 3 selected games/i)).toBeInTheDocument();
      });
    });

    it('should have form controls in bulk operations modal', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      const bulkEditButton = screen.getByText(/bulk edit/i);
      await fireEvent.click(bulkEditButton);
      
      await waitFor(() => {
        expect(screen.getByText('Bulk Operations')).toBeInTheDocument();
        
        // Should have play status dropdown in modal (check by ID)
        expect(document.getElementById('bulkStatus')).toBeInTheDocument();
        
        // Should have rating dropdown in modal (check by ID)
        expect(document.getElementById('bulkRating')).toBeInTheDocument();
        
        // Should have loved checkbox
        expect(screen.getByLabelText(/mark as loved/i)).toBeInTheDocument();
        
        // Should have apply changes button
        expect(screen.getByRole('button', { name: /apply changes/i })).toBeInTheDocument();
      });
    });

    it('should call bulkUpdateStatus when applying bulk operations', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      const bulkEditButton = screen.getByText(/bulk edit/i);
      await fireEvent.click(bulkEditButton);
      
      await waitFor(() => {
        expect(screen.getByText('Bulk Operations')).toBeInTheDocument();
      });

      // Set play status to completed (use specific selector for modal)
      const statusSelect = document.getElementById('bulkStatus') as HTMLSelectElement;
      await fireEvent.change(statusSelect, { target: { value: 'completed' } });
      
      // Apply changes
      const applyButton = screen.getByRole('button', { name: /apply changes/i });
      await fireEvent.click(applyButton);
      
      // Should call bulkUpdateStatus
      await waitFor(() => {
        expect(mockUserGamesStore.bulkUpdateStatus).toHaveBeenCalledWith({
          user_game_ids: ['user-game-1', 'user-game-2', 'user-game-3'],
          play_status: 'completed'
        });
      });
    });
  });

  describe('Accessibility', () => {
    it('should have proper ARIA labels for all checkboxes', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
        expect(screen.getByLabelText('Select Another Game')).toBeInTheDocument();
        expect(screen.getByLabelText('Select Third Game')).toBeInTheDocument();
      });
    });

    it('should have proper modal accessibility attributes', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      const bulkEditButton = screen.getByText(/bulk edit/i);
      await fireEvent.click(bulkEditButton);
      
      await waitFor(() => {
        const modal = screen.getByRole('dialog');
        expect(modal).toHaveAttribute('aria-modal', 'true');
        expect(modal).toHaveAttribute('aria-labelledby', 'modal-title');
        
        const modalTitle = screen.getByText('Bulk Operations');
        expect(modalTitle).toHaveAttribute('id', 'modal-title');
      });
    });
  });
});