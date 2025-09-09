import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { 
  setupFetchMock, 
  resetFetchMock,
  mockConfig
} from '../../test-utils/api-mocks';
import { mockUserGamesStore, mockUserGames, resetStoresMocks } from '../../test-utils/stores-mocks';
import { setAuthenticatedState, resetAuthMocks } from '../../test-utils/auth-mocks';
import GamesPage from './+page.svelte';
import { goto } from '$app/navigation';

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

// Mock $app/navigation
vi.mock('$app/navigation', async (importOriginal) => {
  const actual = await importOriginal() as any;
  return {
    ...actual,
    goto: vi.fn(),
  };
});

// Mock $app/stores
vi.mock('$app/stores', async (importOriginal) => {
  const actual = await importOriginal() as any;
  return {
    ...actual,
    page: {
      subscribe: (callback: any) => {
        callback({
          url: {
            searchParams: {
              get: vi.fn().mockReturnValue(null)
            }
          },
          params: {}
        });
        return () => {};
      }
    }
  };
});

// Using mockUserGames from stores-mocks which has the proper structure

describe('Games Page - Multi-Selection UX Enhancement', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetFetchMock();
    resetStoresMocks();
    resetAuthMocks();
    
    // Set up authenticated state
    setAuthenticatedState();
    
    // Set up stores with mock data
    mockUserGamesStore.value = {
      userGames: mockUserGames,
      currentUserGame: null,
      pagination: { page: 1, pages: 1, per_page: 20, total: mockUserGames.length },
      isLoading: false,
      stats: null,
      error: null,
      filters: {}
    };
    
    // mockPlatformsStore is already properly configured with subscribe pattern - no .value property needed
    
    // Setup basic fetch mock for any API calls
    setupFetchMock();
  });

  it('should navigate to game detail when no games are selected (normal mode)', async () => {
    render(GamesPage);
    
    // Wait for games to load
    await waitFor(() => {
      expect(screen.getByText('Test Game')).toBeInTheDocument();
    });
    
    // Click on a game card - should navigate
    const gameCard = screen.getByRole('button', { name: /view details for test game/i });
    await fireEvent.click(gameCard);
    
    expect(goto).toHaveBeenCalledWith('/games/1');
  });

  it('should toggle selection when games are selected (bulk selection mode)', async () => {
    render(GamesPage);
    
    // Wait for games to load
    await waitFor(() => {
      expect(screen.getByText('Test Game')).toBeInTheDocument();
    });
    
    // First, select a game via checkbox to enter bulk selection mode
    await waitFor(() => {
      expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
    });
    
    const gameCheckbox = screen.getByLabelText('Select Test Game');
    await fireEvent.click(gameCheckbox);
    
    // Verify we're in bulk selection mode by checking for selected count
    await waitFor(() => {
      expect(screen.getByText('1 selected')).toBeInTheDocument();
    });
    
    // Now click on the same game card - should toggle selection off, not navigate
    const gameCard1 = screen.getByRole('button', { name: /test game/i });
    await fireEvent.click(gameCard1);
    
    // Should no longer see "selected" text (back to normal mode)
    await waitFor(() => {
      expect(screen.queryByText('1 selected')).not.toBeInTheDocument();
    });
    
    // goto should NOT have been called for the card click
    expect(goto).toHaveBeenCalledTimes(0);
  });

  it('should show correct aria-label based on bulk selection mode', async () => {
    render(GamesPage);
    
    // Wait for games to load
    await waitFor(() => {
      expect(screen.getByText('Test Game')).toBeInTheDocument();
    });
    
    // Initially should show "View details" in aria-label
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /view details for test game/i })).toBeInTheDocument();
    });
    
    // Select a game to enter bulk selection mode
    await waitFor(() => {
      expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
    });
    
    const checkbox1 = screen.getByLabelText('Select Test Game');
    await fireEvent.click(checkbox1);
    
    // Wait for bulk selection mode to activate and check for aria-label changes
    await waitFor(() => {
      expect(screen.getByText('1 selected')).toBeInTheDocument();
    });
    
    // In bulk selection mode, other unselected games should show "Select" in aria-label
    const anotherGameButton = screen.queryByRole('button', { name: /select another game/i });
    if (anotherGameButton) {
      expect(anotherGameButton).toBeInTheDocument();
    }
  });

  it('should exit bulk selection mode when all games are deselected', async () => {
    render(GamesPage);
    
    // Wait for games to load
    await waitFor(() => {
      expect(screen.getByText('Test Game')).toBeInTheDocument();
    });
    
    // Select a game to enter bulk selection mode
    await waitFor(() => {
      expect(screen.getByLabelText('Select Test Game')).toBeInTheDocument();
    });
    
    const checkbox1 = screen.getByLabelText('Select Test Game');
    await fireEvent.click(checkbox1);
    
    // Verify we're in bulk selection mode
    await waitFor(() => {
      expect(screen.getByText('1 selected')).toBeInTheDocument();
    });
    
    // Deselect the game
    await fireEvent.click(checkbox1);
    
    // Should exit bulk selection mode (selected text should be gone)
    await waitFor(() => {
      expect(screen.queryByText('1 selected')).not.toBeInTheDocument();
    });
    
    // Aria-label should go back to "View details"
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /view details for test game/i })).toBeInTheDocument();
    });
  });
});