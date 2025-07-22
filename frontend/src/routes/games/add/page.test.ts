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
} from '../../../test-utils/api-mocks.js';
import { mockGamesStore, resetStoresMocks } from '../../../test-utils/stores-mocks.js';
import { mockGoto, resetNavigationMocks } from '../../../test-utils/navigation-mocks.js';
import GameAddPage from './+page.svelte';

// Mock the config module
vi.mock('$lib/env', () => ({
  config: mockConfig
}));

// Mock the auth module
vi.mock('$lib/stores/auth.svelte.js', () => ({
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
    mockFetch = setupFetchMock();
  });

  describe('IGDB Search Flow', () => {
    it('should render search form initially', () => {
      render(GameAddPage);
      
      expect(screen.getByPlaceholderText(/search for a game/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /search/i })).toBeInTheDocument();
    });

    it('should trigger IGDB search when form is submitted', async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
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
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
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
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
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
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'nonexistent game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText(/no games found/i)).toBeInTheDocument();
      });
    });

    it('should handle IGDB search errors gracefully', async () => {
      mockGamesStore.searchIGDB.mockRejectedValue(new Error('Search failed'));

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText(/search failed/i)).toBeInTheDocument();
      });
    });
  });

  describe('Game Selection and Confirmation', () => {
    beforeEach(async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
    });

    it('should show confirmation step after selecting a game', async () => {
      render(GameAddPage);
      
      // Perform search
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });

      // Select first game
      const selectButton = screen.getByRole('button', { name: /select this game/i });
      await fireEvent.click(selectButton);
      
      await waitFor(() => {
        expect(screen.getByText(/confirm game details/i)).toBeInTheDocument();
      });
    });

    it('should display complete game metadata in confirmation screen', async () => {
      render(GameAddPage);
      
      // Perform search and select game
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        const selectButton = screen.getByRole('button', { name: /select this game/i });
        fireEvent.click(selectButton);
      });
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
        expect(screen.getByText('A test game from IGDB')).toBeInTheDocument();
        expect(screen.getByText('PC, PlayStation 5')).toBeInTheDocument();
      });
    });

    it('should allow editing game details before confirmation', async () => {
      render(GameAddPage);
      
      // Navigate to confirmation step
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        const titleInput = screen.getByDisplayValue('Test IGDB Game');
        expect(titleInput).toBeInTheDocument();
        
        fireEvent.input(titleInput, { target: { value: 'Modified Game Title' } });
        expect(screen.getByDisplayValue('Modified Game Title')).toBeInTheDocument();
      });
    });

    it('should go back to search results when back button is clicked', async () => {
      render(GameAddPage);
      
      // Navigate to confirmation step
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        expect(screen.getByText(/confirm game details/i)).toBeInTheDocument();
      });

      // Click back button
      const backButton = screen.getByRole('button', { name: /back/i });
      await fireEvent.click(backButton);
      
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /select this game/i })).toBeInTheDocument();
      });
    });
  });

  describe('Game Addition Process', () => {
    beforeEach(async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: mockIGDBCandidates,
        total: mockIGDBCandidates.length
      });
      mockGamesStore.importFromIGDB.mockResolvedValue(mockGame);
    });

    it('should call importFromIGDB when confirming game addition', async () => {
      render(GameAddPage);
      
      // Navigate to confirmation step
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        const confirmButton = screen.getByRole('button', { name: /add game/i });
        fireEvent.click(confirmButton);
      });
      
      expect(mockGamesStore.importFromIGDB).toHaveBeenCalledWith(
        'igdb-123',
        'Test IGDB Game',
        expect.any(Array)
      );
    });

    it('should navigate to games list after successful addition', async () => {
      render(GameAddPage);
      
      // Navigate to confirmation and add game
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /add game/i }));
      });
      
      await waitFor(() => {
        expect(mockGoto).toHaveBeenCalledWith('/games');
      });
    });

    it('should show success message after game addition', async () => {
      render(GameAddPage);
      
      // Complete the flow
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /add game/i }));
      });
      
      await waitFor(() => {
        expect(screen.getByText(/game added successfully/i)).toBeInTheDocument();
      });
    });

    it('should handle game addition errors appropriately', async () => {
      mockGamesStore.importFromIGDB.mockRejectedValue(new Error('Failed to add game'));

      render(GameAddPage);
      
      // Navigate to confirmation and attempt to add game
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /add game/i }));
      });
      
      await waitFor(() => {
        expect(screen.getByText(/failed to add game/i)).toBeInTheDocument();
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

    it('should validate required fields in confirmation form', async () => {
      render(GameAddPage);
      
      // Navigate to confirmation step
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        const titleInput = screen.getByDisplayValue('Test IGDB Game');
        fireEvent.input(titleInput, { target: { value: '' } });
        
        const addButton = screen.getByRole('button', { name: /add game/i });
        fireEvent.click(addButton);
      });
      
      expect(mockGamesStore.importFromIGDB).not.toHaveBeenCalled();
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
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
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
      
      mockGamesStore.importFromIGDB.mockReturnValue(pendingPromise);

      render(GameAddPage);
      
      // Navigate to confirmation
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(screen.getByRole('button', { name: /search/i }));
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /select this game/i }));
      });
      
      await waitFor(() => {
        fireEvent.click(screen.getByRole('button', { name: /add game/i }));
      });
      
      expect(screen.getByText(/adding game/i)).toBeInTheDocument();
      
      resolvePromise!(mockGame);
    });
  });
});