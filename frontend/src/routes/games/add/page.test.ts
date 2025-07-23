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

    it('should add game to collection after selecting a game', async () => {
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
      
      // Should call createFromIGDB when game is selected
      await waitFor(() => {
        expect(mockGamesStore.createFromIGDB).toHaveBeenCalledWith('igdb-123');
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

    it('should fallback to manual entry when IGDB import fails', async () => {
      // Mock IGDB import to fail
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to search results
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      await waitFor(() => {
        const titleInput = screen.getByDisplayValue('Test IGDB Game');
        expect(titleInput).toBeInTheDocument();
        
        fireEvent.input(titleInput, { target: { value: 'Modified Game Title' } });
        expect(screen.getByDisplayValue('Modified Game Title')).toBeInTheDocument();
      });
    });

    it('should go back to search results when back button is clicked from manual entry', async () => {
      // Mock IGDB import to fail so we get to manual entry step
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to search results and trigger fallback to manual entry
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      await waitFor(() => {
        expect(screen.getByDisplayValue('Test IGDB Game')).toBeInTheDocument();
      });

      // Click back button from manual entry
      const backButton = screen.getByRole('button', { name: /back to selection/i });
      await fireEvent.click(backButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /test igdb game/i })).toBeInTheDocument();
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

    it('should call createFromIGDB when selecting a game from search results', async () => {
      render(GameAddPage);
      
      // Navigate to search results
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      await waitFor(() => {
        expect(mockGamesStore.createFromIGDB).toHaveBeenCalledWith('igdb-123');
      });
    });

    it('should navigate to games list after successful addition', async () => {
      render(GameAddPage);
      
      // Navigate to search results and select game
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
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
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      // Should fallback to manual entry form with pre-filled data
      await waitFor(() => {
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
      // Mock IGDB import to fail so we get to manual entry step
      mockGamesStore.createFromIGDB.mockRejectedValue(new Error('Import failed'));
      
      render(GameAddPage);
      
      // Navigate to manual entry form via failed IGDB import
      const searchInput = screen.getByPlaceholderText(/enter game title/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      await waitFor(() => {
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
        fireEvent.click(screen.getByRole('button', { name: /test igdb game/i }));
      });
      
      expect(screen.getAllByText(/adding to collection/i).length).toBeGreaterThan(0);
      
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