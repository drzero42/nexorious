import { vi } from 'vitest';

// Mock game data
export const mockGame = {
  id: 'game-1',
  title: 'Test Game',
  description: 'A test game description',
  genre: 'Action',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2024-01-01',
  cover_art_url: 'https://example.com/cover.jpg',
  personal_rating: 4,
  play_status: 'completed' as const,
  hours_played: 25,
  personal_notes: 'Great game!',
  is_loved: true,
  ownership_status: 'owned' as const,
  is_physical: false,
  last_played: '2024-01-15',
  acquired_date: '2024-01-01'
};

export const mockGames = [
  mockGame,
  {
    ...mockGame,
    id: 'game-2',
    title: 'Another Game',
    play_status: 'in_progress' as const,
    personal_rating: 5,
    hours_played: 10
  },
  {
    ...mockGame,
    id: 'game-3',
    title: 'Third Game',
    play_status: 'not_started' as const,
    personal_rating: null,
    hours_played: 0,
    is_loved: false
  }
];

// Mock user games store
export const mockUserGamesStore = {
  value: {
    games: mockGames,
    isLoading: false,
    error: null
  },
  fetchUserGames: vi.fn(),
  addUserGame: vi.fn(),
  updateUserGame: vi.fn(),
  deleteUserGame: vi.fn(),
  clearError: vi.fn()
};

// Mock games store
export const mockGamesStore = {
  value: {
    games: [],
    searchResults: [],
    isLoading: false,
    isSearching: false,
    error: null
  },
  searchGames: vi.fn(),
  addGame: vi.fn(),
  clearSearchResults: vi.fn(),
  clearError: vi.fn()
};

// Mock wishlist store
export const mockWishlistStore = {
  value: {
    games: [mockGame],
    isLoading: false,
    error: null
  },
  fetchWishlist: vi.fn(),
  addToWishlist: vi.fn(),
  removeFromWishlist: vi.fn(),
  clearError: vi.fn()
};

// Mock platforms store
export const mockPlatformsStore = {
  value: {
    platforms: [
      { id: 'pc', name: 'PC', display_name: 'PC' },
      { id: 'ps5', name: 'PlayStation 5', display_name: 'PlayStation 5' }
    ],
    storefronts: [
      { id: 'steam', name: 'Steam', display_name: 'Steam' },
      { id: 'epic', name: 'Epic Games Store', display_name: 'Epic Games Store' }
    ],
    isLoading: false,
    error: null
  },
  fetchPlatforms: vi.fn(),
  fetchStorefronts: vi.fn(),
  clearError: vi.fn()
};

// Mock search store
export const mockSearchStore = {
  value: {
    query: '',
    filters: {},
    results: [],
    isLoading: false,
    error: null
  },
  setQuery: vi.fn(),
  setFilters: vi.fn(),
  search: vi.fn(),
  clearResults: vi.fn(),
  clearError: vi.fn()
};

// Mock UI store
export const mockUIStore = {
  value: {
    theme: 'light' as const,
    sidebarOpen: false,
    mobileMenuOpen: false
  },
  setTheme: vi.fn(),
  toggleSidebar: vi.fn(),
  toggleMobileMenu: vi.fn()
};

// Mock all stores
vi.mock('$lib/stores', () => ({
  userGames: mockUserGamesStore,
  games: mockGamesStore,
  wishlist: mockWishlistStore,
  platforms: mockPlatformsStore,
  search: mockSearchStore,
  ui: mockUIStore
}));

// Reset functions for test cleanup
export function resetStoresMocks() {
  // Reset user games store
  mockUserGamesStore.fetchUserGames.mockClear();
  mockUserGamesStore.addUserGame.mockClear();
  mockUserGamesStore.updateUserGame.mockClear();
  mockUserGamesStore.deleteUserGame.mockClear();
  mockUserGamesStore.clearError.mockClear();
  mockUserGamesStore.value = {
    games: mockGames,
    isLoading: false,
    error: null
  };

  // Reset games store
  mockGamesStore.searchGames.mockClear();
  mockGamesStore.addGame.mockClear();
  mockGamesStore.clearSearchResults.mockClear();
  mockGamesStore.clearError.mockClear();
  mockGamesStore.value = {
    games: [],
    searchResults: [],
    isLoading: false,
    isSearching: false,
    error: null
  };

  // Reset wishlist store
  mockWishlistStore.fetchWishlist.mockClear();
  mockWishlistStore.addToWishlist.mockClear();
  mockWishlistStore.removeFromWishlist.mockClear();
  mockWishlistStore.clearError.mockClear();
  mockWishlistStore.value = {
    games: [mockGame],
    isLoading: false,
    error: null
  };

  // Reset other stores
  mockPlatformsStore.fetchPlatforms.mockClear();
  mockPlatformsStore.fetchStorefronts.mockClear();
  mockPlatformsStore.clearError.mockClear();

  mockSearchStore.setQuery.mockClear();
  mockSearchStore.setFilters.mockClear();
  mockSearchStore.search.mockClear();
  mockSearchStore.clearResults.mockClear();
  mockSearchStore.clearError.mockClear();

  mockUIStore.setTheme.mockClear();
  mockUIStore.toggleSidebar.mockClear();
  mockUIStore.toggleMobileMenu.mockClear();
}