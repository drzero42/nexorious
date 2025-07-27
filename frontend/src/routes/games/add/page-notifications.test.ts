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
import { mockGoto, resetNavigationMocks } from '../../../test-utils/navigation-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import GameAddPage from './+page.svelte';

// Mock the notifications store
const mockNotifications = {
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showInfo: vi.fn(),
  showApiError: vi.fn(),
  remove: vi.fn(),
  clear: vi.fn(),
  items: []
};

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

vi.mock('$lib/stores/notifications.svelte', () => ({
  notifications: mockNotifications
}));

describe('Game Addition Page - Notifications Integration', () => {

  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetNavigationMocks();
    resetAuthMocks();
    setupFetchMock();
    setAuthenticatedState();
    vi.useFakeTimers();
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
      mockPlatformsStore.fetchPlatforms.mockRejectedValue(new Error('Platform load failed'));
      
      render(GameAddPage);
      
      await waitFor(() => {
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Failed to load platforms and storefronts. Some features may not work properly.'
        );
      });
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
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith('Game metadata imported successfully from IGDB');
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
    it('should handle IGDB import failure and fallback to manual entry', async () => {
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
          'Failed to import game from IGDB. You can add it manually with custom details.'
        );
        // Should fallback to manual entry step
        expect(screen.getByText(/manual entry/i)).toBeInTheDocument();
      });
    });

    it('should handle collection addition failure', async () => {
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
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith('Game metadata imported successfully from IGDB');
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Game was imported but couldn\'t be added to your collection. You can try adding it manually from your games list.'
        );
      });
      
      // Should redirect with longer delay for error
      vi.advanceTimersByTime(2000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });
  });

  describe('Manual Game Creation Flow', () => {
    beforeEach(() => {
      mockGamesStore.createGame.mockResolvedValue(mockGame);
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: mockGame.id
      });
    });

    it('should show success notifications for manual game creation', async () => {
      render(GameAddPage);
      
      // Go to manual entry
      await fireEvent.click(screen.getByRole('button', { name: /add game manually/i }));
      
      // Fill out form
      await waitFor(() => {
        expect(screen.getByLabelText(/game title/i)).toBeInTheDocument();
      });
      
      await fireEvent.input(screen.getByLabelText(/game title/i), { 
        target: { value: 'Manual Test Game' } 
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith('Game created successfully');
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith(`"${mockGame.title}" successfully added to your collection!`);
      });
    });

    it('should handle manual game creation failure', async () => {
      mockGamesStore.createGame.mockRejectedValue(new Error('Game creation failed'));
      
      render(GameAddPage);
      
      // Go to manual entry
      await fireEvent.click(screen.getByRole('button', { name: /add game manually/i }));
      
      await waitFor(() => {
        expect(screen.getByLabelText(/game title/i)).toBeInTheDocument();
      });
      
      await fireEvent.input(screen.getByLabelText(/game title/i), { 
        target: { value: 'Manual Test Game' } 
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showApiError).toHaveBeenCalledWith(
          expect.any(Error),
          'Failed to create game. Please check your information and try again.'
        );
      });
    });

    it('should handle manual game creation with collection failure', async () => {
      mockUserGamesStore.addGameToCollection.mockRejectedValue(new Error('Collection add failed'));
      
      render(GameAddPage);
      
      // Go to manual entry and complete
      await fireEvent.click(screen.getByRole('button', { name: /add game manually/i }));
      
      await waitFor(() => {
        expect(screen.getByLabelText(/game title/i)).toBeInTheDocument();
      });
      
      await fireEvent.input(screen.getByLabelText(/game title/i), { 
        target: { value: 'Manual Test Game' } 
      });
      
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
      await waitFor(() => {
        expect(mockNotifications.showSuccess).toHaveBeenCalledWith('Game created successfully');
        expect(mockNotifications.showError).toHaveBeenCalledWith(
          'Game was created but couldn\'t be added to your collection. You can try adding it manually from your games list.'
        );
      });
    });
  });

  describe('Redirect Timing', () => {
    it('should delay redirect for success to show message', async () => {
      mockGamesStore.createFromIGDB.mockResolvedValue(mockGame);
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: mockGame.id
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
      
      // Should not redirect immediately
      expect(mockGoto).not.toHaveBeenCalled();
      
      // Should redirect after 1 second
      vi.advanceTimersByTime(1000);
      expect(mockGoto).toHaveBeenCalledWith('/games');
    });

    it('should have longer delay for error redirects', async () => {
      mockGamesStore.createFromIGDB.mockResolvedValue(mockGame);
      mockUserGamesStore.addGameToCollection.mockRejectedValue(new Error('Collection failed'));
      
      render(GameAddPage);
      
      // Complete flow that will partially fail
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText(mockIGDBCandidates[0]!.title)).toBeInTheDocument();
      });
      
      await fireEvent.click(screen.getByText(mockIGDBCandidates[0]!.title));
      await fireEvent.click(screen.getByRole('button', { name: /add to collection/i }));
      
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