import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  mockConfig,
  mockIGDBCandidates
} from '../../../test-utils/api-mocks.js';
import { mockGamesStore, resetStoresMocks } from '../../../test-utils/stores-mocks.js';
import { resetNavigationMocks } from '../../../test-utils/navigation-mocks.js';
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

describe('Game Addition Page - PR Focused Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetStoresMocks();
    resetNavigationMocks();
    
    // Set up successful IGDB search mock
    mockGamesStore.searchIGDB.mockResolvedValue({
      games: mockIGDBCandidates,
      total: mockIGDBCandidates.length
    });
  });

  describe('IGDB Response Structure (PR Fix)', () => {
    it('should use games property from IGDB search response', async () => {
      render(GameAddPage);
      
      // Trigger search
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test game' } });
      await fireEvent.click(searchButton);
      
      // Wait for results to display
      await waitFor(() => {
        expect(screen.getByText('Test IGDB Game')).toBeInTheDocument();
      });
      
      // Verify the response structure was handled correctly
      expect(mockGamesStore.searchIGDB).toHaveBeenCalledWith('test game', 10);
    });

    it('should display search results from games property (not candidates)', async () => {
      // Mock response with explicit games property structure
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: [
          {
            igdb_id: 'test-123',
            title: 'Game from Games Property',
            release_date: '2024-01-01',
            cover_art_url: 'https://example.com/cover.jpg',
            description: 'Test game description',
            platforms: ['PC'],
            howlongtobeat_main: 15
          }
        ],
        total: 1
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText('Game from Games Property')).toBeInTheDocument();
      });
    });

    it('should handle empty games array in response', async () => {
      mockGamesStore.searchIGDB.mockResolvedValue({
        games: [],
        total: 0
      });

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'nonexistent' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        expect(screen.getByText(/no games found/i)).toBeInTheDocument();
      });
    });
  });

  describe('Component Integration', () => {
    it('should render search form without errors', () => {
      render(GameAddPage);
      
      expect(screen.getByPlaceholderText(/search for a game/i)).toBeInTheDocument();
      expect(screen.getByRole('button', { name: /search/i })).toBeInTheDocument();
    });

    it('should handle search submission', async () => {
      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test search' } });
      await fireEvent.click(searchButton);
      
      expect(mockGamesStore.searchIGDB).toHaveBeenCalledWith('test search', 10);
    });
  });

  describe('Error Handling', () => {
    it('should handle IGDB search failures gracefully', async () => {
      mockGamesStore.searchIGDB.mockRejectedValue(new Error('Search failed'));

      render(GameAddPage);
      
      const searchInput = screen.getByPlaceholderText(/search for a game/i);
      const searchButton = screen.getByRole('button', { name: /search/i });
      
      await fireEvent.input(searchInput, { target: { value: 'test' } });
      await fireEvent.click(searchButton);
      
      await waitFor(() => {
        // Should handle error without crashing
        expect(screen.getByText(/search/i)).toBeInTheDocument();
      });
    });
  });
});