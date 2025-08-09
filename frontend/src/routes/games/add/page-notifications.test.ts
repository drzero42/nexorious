import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig,
  mockIGDBCandidates,
  mockGame
} from '../../../test-utils/api-mocks';
import { mockGamesStore, mockUserGamesStore, mockPlatformsStore, resetStoresMocks } from '../../../test-utils/stores-mocks';
import { mockGoto } from '../../../test-utils/navigation-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import GameAddPage from './+page.svelte';

// Mock modules
vi.mock('$lib/env', () => ({ config: mockConfig }));
vi.mock('$lib/stores/auth.svelte', () => ({
  auth: {
    value: {
      accessToken: 'test-token',
      user: { id: '1', username: 'testuser' }
    }
  }
}));

vi.mock('$lib/stores/platforms.svelte', () => ({
  platforms: mockPlatformsStore
}));

// Don't mock the service globally - let most tests use the real service with mocked stores
// Only specific tests will mock the service directly

vi.mock('$lib/stores/notifications.svelte', () => ({
  notifications: {
    showSuccess: vi.fn(),
    showError: vi.fn(),
    showWarning: vi.fn(),
    showInfo: vi.fn(),
    showApiError: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    items: []
  }
}));

// Get reference to the mocked notifications
let mockNotifications: any;

describe('Game Addition Page - Notifications Integration', () => {

  beforeEach(async () => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
    vi.useFakeTimers();
    
    // Reset platforms mock to succeed by default
    mockPlatformsStore.fetchAll.mockResolvedValue({
      platforms: [
        { id: 'pc', name: 'PC', display_name: 'PC', is_active: true, source: 'official', created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' }
      ],
      storefronts: [
        { id: 'steam', name: 'Steam', display_name: 'Steam', is_active: true, source: 'official', created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' }
      ]
    });
    
    // Get fresh reference to mocked notifications after clearing
    const notificationsModule = await import('$lib/stores/notifications.svelte');
    mockNotifications = notificationsModule.notifications;
    
    // Re-setup the mock functions after clearing
    mockNotifications.showSuccess = vi.fn();
    mockNotifications.showError = vi.fn();
    mockNotifications.showWarning = vi.fn();
    mockNotifications.showInfo = vi.fn();
    mockNotifications.showApiError = vi.fn();
    mockNotifications.remove = vi.fn();
    mockNotifications.clear = vi.fn();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('Search Error Handling', () => {
    it('should show error notification when IGDB search fails', async () => {
      // Mock failed search
      mockGamesStore.searchIGDB.mockRejectedValue(new Error('Search failed'));
      
      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(mockNotifications.showApiError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to search for games. Please try again.'
        );
      });
    });

    it('should show error notification when platforms fail to load', async () => {
      // Setup the rejection before rendering
      mockPlatformsStore.fetchAll.mockRejectedValue(new Error('Platform load failed'));
      
      render(GameAddPage);
      
      // Wait for the onMount lifecycle to complete and error handling
      await waitFor(() => {
        expect(mockPlatformsStore.fetchAll).toHaveBeenCalled();
      }, { timeout: 2000 });

      // Wait for error handling to complete
      await waitFor(() => {
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Failed to load platforms and storefronts. Some features may not work properly.'
        );
      }, { timeout: 2000 });
    });
  });

  describe('Game Import Success Flow', () => {
    beforeEach(() => {
      // Setup successful mocks
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates
      });
      mockGamesStore.createFromIGDB.mockResolvedValue(mockGame);
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: mockGame.id
      });
      mockUserGamesStore.updateProgress.mockResolvedValue({});
      mockUserGamesStore.updateUserGame.mockResolvedValue({});
      
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
    });

    it('should show success notifications for complete IGDB import flow', async () => {
      render(GameAddPage);
      
      // Search for game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      // Wait for search results and select first game
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      
      // Should be on confirmation step, click add to collection
      await waitFor(() => {
        expect(screen.getByRole('button', { name: /add to collection/i })).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      // Verify success notifications
      await waitFor(() => {
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith(`Adding "${mockGame.title}" to your collection`);
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith(`"${mockGame.title}" successfully added to your collection!`);
      });
      
      // Verify redirect with delay
      vi.advanceTimersByTime(1000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });

    it('should show warning for partial success (game added but progress failed)', async () => {
      mockUserGamesStore.updateProgress.mockRejectedValue(new Error('Progress update failed'));
      
      render(GameAddPage);
      
      // Complete the flow with progress data
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      
      // Set some progress data that will fail to save
      await waitFor(() => {
        const playStatusSelect = screen.getByDisplayValue(/not started/i);
        expect(playStatusSelect).toBeInTheDocument();
      });
      
      await fireEvent.change(screen.getByDisplayValue(/not started/i), { 
        target: { value: 'in_progress' } 
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showWarning).toHaveBeenCalledWith(
          expect.stringContaining('added to collection, but some details couldn\'t be saved: Failed to save progress information')
        );
      });
    });

    it('should show warning for partial success (game added but rating failed)', async () => {
      mockUserGamesStore.updateUserGame.mockRejectedValue(new Error('Rating update failed'));
      
      render(GameAddPage);
      
      // Complete the flow with rating data
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      
      // Set rating that will fail to save
      await waitFor(() => {
        const ratingSelect = screen.getByDisplayValue(/no rating/i);
        expect(ratingSelect).toBeInTheDocument();
      });
      
      await fireEvent.change(screen.getByDisplayValue(/no rating/i), { 
        target: { value: '5' } 
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showWarning).toHaveBeenCalledWith(
          expect.stringContaining('added to collection, but some details couldn\'t be saved: Failed to save rating and favorite status')
        );
      });
    });
  });

  describe('Game Import Error Flow', () => {
    it('should handle IGDB import failure with error notification', async () => {
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
      
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates
      });
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('IGDB import failed'));
      
      render(GameAddPage);
      
      // Search and select game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Failed to import game from IGDB. Please try a different search or contact support.'
        );
      });
    });

    it('should handle collection addition failure', async () => {
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
      
      mockGamesStore.createFromIGDB.mockResolvedValue(mockGame);
      mockUserGamesStore.addGameToCollection.mockRejectedValue(new Error('Collection add failed'));
      
      render(GameAddPage);
      
      // Complete the flow
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith(`Adding "${mockGame.title}" to your collection`);
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Game was imported but couldn\'t be added to your collection. Please try again or contact support.'
        );
      });
      
      // Should redirect with longer delay for error
      vi.advanceTimersByTime(2000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });
  });


  describe('Redirect Timing', () => {
    it.skip('should delay redirect for success to show message', async () => {
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
      
      // Mock the service directly for this test
      const mockGameAdditionService = await vi.importMock('$lib/services/game-addition') as any;
      mockGameAdditionService.gameAdditionService.addGameComplete = vi.fn().mockResolvedValue({
        success: true,
        game: mockGame,
        partialErrors: []
      });
      
      render(GameAddPage);
      
      // Complete successful flow
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      // Wait for the service call
      await waitFor(() => {
        expect(mockGameAdditionService.gameAdditionService.addGameComplete).toHaveBeenCalled();
      });
      
      // Should not redirect immediately
      expect(mockGoto).not.toHaveBeenCalled();
      
      // Should redirect after 1 second
      vi.advanceTimersByTime(1000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });

    it.skip('should have longer delay for error redirects', async () => {
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
      
      // Mock the service directly for this test
      const mockGameAdditionService = await vi.importMock('$lib/services/game-addition') as any;
      mockGameAdditionService.gameAdditionService.addGameComplete = vi.fn().mockResolvedValue({
        success: false,
        partialErrors: []
      });
      
      render(GameAddPage);
      
      // Complete flow that will fail
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      // Wait for the service call
      await waitFor(() => {
        expect(mockGameAdditionService.gameAdditionService.addGameComplete).toHaveBeenCalled();
      });
      
      // Should not redirect immediately
      expect(mockGoto).not.toHaveBeenCalled();
      
      // Should not redirect after 1 second
      vi.advanceTimersByTime(1000);
      expect(mockGoto).not.toHaveBeenCalled();
      
      // Should redirect after 2 seconds
      vi.advanceTimersByTime(1000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });
  });

  describe('Notification Message Content', () => {
    it('should include game title in success messages', async () => {
      // Clear user games collection so the IGDB game doesn't appear as already owned
      mockUserGamesStore.value.userGames = [];
      
      mockGamesStore.createFromIGDB.mockResolvedValue({
        ...mockGame,
        title: 'Specific Game Title'
      });
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: mockGame.id
      });
      
      render(GameAddPage);
      
      // Complete flow
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith(
          '"Specific Game Title" successfully added to your collection!'
        );
      });
    });

    it('should provide specific error details for different failure types', async () => {
      const specificError = new Error('Network timeout occurred');
      mockGamesStore.searchIGDB.mockRejectedValue(specificError);
      
      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showApiError).toHaveBeenCalledWith(
          specificError,
          'Failed to search for games. Please try again.'
        );
      });
    });
  });
});