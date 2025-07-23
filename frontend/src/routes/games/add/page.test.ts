import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  APIResponseMock, 
  setupFetchMock, 
  resetFetchMock,
  mockConfig,
  mockIGDBSearchResponse,
  mockIGDBCandidates,
  mockGame
} from '../../../test-utils/api-mocks';
import { mockGamesStore, mockUserGamesStore, resetStoresMocks } from '../../../test-utils/stores-mocks';
import { mockGoto, resetNavigationMocks } from '../../../test-utils/navigation-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../../test-utils/auth-mocks';
import GameAddPage from './+page.svelte';

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

describe('Game Addition Page', () => {
  let mockFetch: any;
  
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetNavigationMocks();
    resetAuthMocks();
    mockFetch = setupFetchMock();
    setAuthenticatedState();
  });

  describe('IGDB Search Flow', () => {
    it('should render search form initially', () => {
      render(GameAddPage);
      
      expect(screen.getByPlaceholderText(/enter game title/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /search/i })).toBeInTheDocument();
    });

    it('should trigger IGDB search when form is submitted', async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      expect(mockGamesStore.searchIGDB).toHaveBeenCalledWith('test game', 10);
    });

    it('should use games property from IGDB response for search results', async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });
    });

    it('should display multiple game candidates when available', async () => {
      const multipleGames = [
        ...mockIGDBCandidates,
        {
          igdb_id: 'igdb-456',
          title: 'Another Test Game',
          release_date: '2024-03-01',
          cover_art_url: 'https://example.com/cover2.jpg',
          description: 'Another test game',
          platforms: ['PC'],
          howlongtobeat_main: 16  // Realistic completion time in hours
        }
      ];

      mockGamesStore.searchIGDB.mockResolvedValue({
        games: multipleGames,
        total: multipleGames.length
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
        expect(screen.getByText('Another Test Game')).toBeInTheDocument();
      });
    });

    it('should show no results message when IGDB search returns empty', async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: [],
        total: 0
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'nonexistent game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText(/no games found/i)).toBeInTheDocument();
      });
    });

  });

  describe('Game Selection and Confirmation', () => {
    beforeEach(async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
      mockGamesStore.createFromIGDB.mockResolvedValue({...mockGame, id: 'game-1'});
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: 'game-1',
        ...mockGame
      });
    });

    it('should show metadata confirmation after selecting a game', async () => {
      render(GameAddPage);
      
      // Perform search
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game (the whole game card is clickable)
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Should show metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
        expect(screen.getByText('Review the game information before adding to your collection')).toBeInTheDocument();
      });

      // Should not call createFromIGDB yet (only after confirmation)
      expect(mockGamesStore.createFromIGDB).not.toHaveBeenCalled();
    });

    it('should add game to collection after confirming metadata', async () => {
      render(GameAddPage);
      
      // Perform search
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Should call createFromIGDB when confirmed
      await waitFor(() => {
        expect(mockGamesStore.createFromIGDB).toHaveBeenCalledWith('igdb-123', {});
      });
    });

    it('should display complete game metadata in search results', async () => {
      render(GameAddPage);
      
      // Perform search
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });
      
      // Check that game metadata is displayed
      expect(screen.getAllByText('A test game from IGDB').length).toBeGreaterThan(0);
      expect(screen.getAllByText('PC').length).toBeGreaterThan(0);
      expect(screen.getAllByText('PlayStation 5').length).toBeGreaterThan(0);
    });

    it('should fallback to manual entry when IGDB import fails after confirmation', async () => {
      // Mock IGDB import to fail
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to search results
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button (which should fail and redirect to manual entry)
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Should fallback to manual entry with pre-filled data
      await waitFor(() => {
        expect(screen.getByText('Game Details')).toBeInTheDocument();
        const titleInput = screen.getByDisplayValue('Test IGDB Game');
        expect(titleInput).toBeInTheDocument();
      });
    });

    it('should go back to metadata confirmation when back button is clicked from manual entry', async () => {
      // Mock IGDB import to fail so we get to manual entry step
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to search results and select a game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button (which should fail and redirect to manual entry)
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Should fallback to manual entry
      await waitFor(() => {
        expect(screen.getByText('Game Details')).toBeInTheDocument();
        expect(screen.getByDisplayValue('Test IGDB Game')).toBeInTheDocument();
      });

      // Click back button from manual entry - should go back to metadata confirmation
      const backButton = screen.getByRole('button', { name: /back to selection/i });
      await fireEvent.click(backButton);
      
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });
    });
  });

  describe('Game Addition Process', () => {
    beforeEach(async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
      mockGamesStore.createFromIGDB.mockResolvedValue({...mockGame, id: 'game-1'});
      mockUserGamesStore.addGameToCollection.mockResolvedValue({
        id: 'user-game-1',
        game_id: 'game-1',
        ...mockGame
      });
    });

    it('should call createFromIGDB when confirming a game from metadata screen', async () => {
      render(GameAddPage);
      
      // Navigate to search results
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      await waitFor(() => {
        expect(mockGamesStore.createFromIGDB).toHaveBeenCalledWith('igdb-123', {});
      });
    });

    it('should navigate to games list after successful addition', async () => {
      render(GameAddPage);
      
      // Navigate to search results and select game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/games');
      });
    });


    it('should fallback to manual entry when game addition fails', async () => {
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Failed to add game'));

      render(GameAddPage);
      
      // Navigate to search results and attempt to select game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button (which should fail and redirect to manual entry)
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Should fallback to manual entry form with pre-filled data
      await waitFor(() => {
        expect(screen.getByText('Game Details')).toBeInTheDocument();
        expect(screen.getByDisplayValue('Test IGDB Game')).toBeInTheDocument();
      });
      
      expect(mockGoto).not.toHaveBeenCalled();
    });
  });

  describe('Form Validation', () => {
    it('should require search query before submitting search', async () => {
      render(GameAddPage);
      
      const searchButton = screen.getByRole('button', { name: /search/i });
      await fireEvent.click(searchButton);
      
      expect(mockGamesStore.searchIGDB).not.toHaveBeenCalled();
    });

    it('should validate required fields in manual entry form', async () => {
      // Mock IGDB search to succeed but import to fail so we get to manual entry step
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to manual entry form via failed IGDB import
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button (which should fail and redirect to manual entry)
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Should fallback to manual entry form
      await waitFor(() => {
        expect(screen.getByText('Game Details')).toBeInTheDocument();
        const titleInput = screen.getByDisplayValue('Test IGDB Game');
        fireEvent.input(titleInput, { target: { value: '' } });
        
        const addButton = screen.getByRole('button', { name: /add game to collection/i });
        fireEvent.click(addButton);
      });
      
      expect(mockGamesStore.createGame).not.toHaveBeenCalled();
    });
  });

  describe('Loading States', () => {
    it('should show loading state during IGDB search', async () => {
      let resolvePromise: (value: any) => void;
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      
      mockGamesStore.searchIGDB.mockReturnValue(pendingPromise);

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      expect(screen.getByText(/searching/i)).toBeInTheDocument();
      
      resolvePromise!({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
    });

    it('should show loading state during game addition', async () => {
      let resolvePromise: (value: any) => void;
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      
      mockGamesStore.createFromIGDB.mockReturnValue(pendingPromise);

      render(GameAddPage);
      
      // Navigate to search results and select game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select game to go to metadata confirmation
      const selectButton = screen.getByRole('button', { name: /test igdb game/i });
      await fireEvent.click(selectButton);
      
      // Wait for metadata confirmation screen
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
      });

      // Click confirm button to trigger the loading state
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Now should see loading state
      expect(screen.getAllByText(/adding to collection/i)).toHaveLength(1);
      
      resolvePromise!(mockGame);
    });

    it('should only show loading state on clicked game card when multiple games exist', async () => {
      const multipleGames = [
        ...mockIGDBCandidates,
        {
          igdb_id: 'igdb-456',
          title: 'Another Test Game',
          release_date: '2024-03-01',
          cover_art_url: 'https://example.com/cover2.jpg',
          description: 'Another test game',
          platforms: ['PC'],
          howlongtobeat_main: 16
        },
        {
          igdb_id: 'igdb-789',
          title: 'Third Test Game',
          release_date: '2023-05-15',
          cover_art_url: 'https://example.com/cover3.jpg',
          description: 'A third test game',
          platforms: ['PlayStation 5'],
          howlongtobeat_main: 24
        }
      ];

      // Mock the search to return multiple games
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: multipleGames,
        total: multipleGames.length
      });

      let resolvePromise: (value: any) => void;
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      
      mockGamesStore.createFromIGDB.mockReturnValue(pendingPromise);

      render(GameAddPage);
      
      // Navigate to search results
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      // Wait for all games to be rendered
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
        expect(screen.getByText('Another Test Game')).toBeInTheDocument();
        expect(screen.getByText('Third Test Game')).toBeInTheDocument();
      });

      // Click on the second game (Another Test Game)
      const secondGameButton = screen.getByRole('button', { name: /another test game/i });
      await fireEvent.click(secondGameButton);
      
      // Should go to metadata confirmation screen, not show loading yet
      await waitFor(() => {
        expect(screen.getByText('Confirm Game Details')).toBeInTheDocument();
        expect(screen.getByText('Another Test Game')).toBeInTheDocument(); // Should show the selected game
      });

      // Click confirm button to trigger the loading state
      const confirmButton = screen.getByRole('button', { name: /confirm and add to collection/i });
      await fireEvent.click(confirmButton);
      
      // Now should see loading state
      const loadingMessages = screen.getAllByText(/adding to collection/i);
      expect(loadingMessages).toHaveLength(1);
      
      // Verify that the createFromIGDB was called with the correct IGDB ID for the second game
      expect(mockGamesStore.createFromIGDB).toHaveBeenCalledWith('igdb-456', {});
      
      resolvePromise!(mockGame);
    });
  });

  // Isolated error handling tests to prevent interference from other describe blocks
  describe('Error Handling (Isolated)', () => {
    beforeEach(() => {
      // Ensure clean state for error tests
      vi.clearAllMocks();
      resetStoresMocks();
      resetNavigationMocks();
      resetAuthMocks();
      setAuthenticatedState();
    });

    it('should handle IGDB search errors gracefully', async () => {
      // Pre-set the error state to simulate what happens when search fails
      mockGamesStore.value = {
        ...mockGamesStore.value,
        error: 'Failed to search IGDB',
        isLoading: false
      };
      
      // Mock searchIGDB to fail
      mockGamesStore.searchIGDB.mockRejectedValue(new Error('Search failed'));

      render(GameAddPage);
      
      // Since error is already set, it should be visible immediately
      await waitFor(() => {
        expect(screen.getByText(/failed to search igdb/i)).toBeInTheDocument();
      });

      // Verify we're on the search step
      expect(screen.getByPlaceholderText(/enter game title/i)).toBeInTheDocument();
    });
  });
});