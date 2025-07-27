import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../test-utils/api-mocks';
import { mockUserGamesStore, mockPlatformsStore, mockGames, resetStoresMocks } from '../../test-utils/stores-mocks';
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
      games: mockGames.map((game, index) => ({
        id: `user-game-${index + 1}`,
        game: {
          id: game.id,
          title: game.title,
          description: game.description,
          genre: game.genre,
          developer: game.developer,
          publisher: game.publisher,
          release_date: game.release_date,
          cover_art_url: game.cover_art_url
        },
        ownership_status: game.ownership_status,
        is_physical: game.is_physical,
        personal_rating: game.personal_rating,
        is_loved: game.is_loved,
        play_status: game.play_status,
        hours_played: game.hours_played,
        personal_notes: game.personal_notes,
        acquired_date: game.acquired_date,
        last_played: game.last_played,
        platforms: [],
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'
      })),
      currentUserGame: null,
      stats: null,
      isLoading: false,
      error: null,
      filters: {},
      pagination: {
        page: 1,
        per_page: 20,
        total: mockGames.length,
        pages: 1
      }
    };

    // Add required methods to mocks
    mockUserGamesStore.fetchUserGames = vi.fn().mockResolvedValue(undefined);
    mockUserGamesStore.updateProgress = vi.fn().mockResolvedValue([]);
    mockPlatformsStore.loadAll = vi.fn().mockResolvedValue(undefined);
  });

  describe('Basic Rendering', () => {
    it('should render games page successfully', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText('My Games')).toBeInTheDocument();
        expect(screen.getByText(/3 games in your collection/)).toBeInTheDocument();
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
    it('should change button text when select all is clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      // After clicking select all, it should change to "Clear All"
      await waitFor(() => {
        expect(screen.getByText(/clear all/i)).toBeInTheDocument();
      });
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

    it('should show selected count and bulk actions after selecting games', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      // Should show selected count and bulk actions
      await waitFor(() => {
        expect(screen.getByText(/3 selected/)).toBeInTheDocument();
        expect(screen.getByText(/bulk actions/i)).toBeInTheDocument();
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
    it('should open bulk operations modal when bulk actions button is clicked', async () => {
      render(GamesPage);
      
      await waitFor(() => {
        expect(screen.getByText(/select all/i)).toBeInTheDocument();
      });

      // Select all games
      const selectAllButton = screen.getByText(/select all/i);
      await fireEvent.click(selectAllButton);
      
      await waitFor(() => {
        expect(screen.getByText(/bulk actions/i)).toBeInTheDocument();
      });

      // Click bulk actions button
      const bulkActionsButton = screen.getByText(/bulk actions/i);
      await fireEvent.click(bulkActionsButton);
      
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
      
      const bulkActionsButton = screen.getByText(/bulk actions/i);
      await fireEvent.click(bulkActionsButton);
      
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
      
      const bulkActionsButton = screen.getByText(/bulk actions/i);
      await fireEvent.click(bulkActionsButton);
      
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
      
      const bulkActionsButton = screen.getByText(/bulk actions/i);
      await fireEvent.click(bulkActionsButton);
      
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