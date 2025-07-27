import { vi } from 'vitest';

// Mock game metadata (separate from user-specific data)
export const mockGameMetadata = {
  id: 'game-1',
  title: 'Test Game',
  description: 'A test game description',
  genre: 'Action',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2024-01-01',
  cover_art_url: 'https://example.com/cover.jpg',
  rating_average: 4.5,
  rating_count: 100,
  game_metadata: '{}',
  estimated_playtime_hours: 25,
  howlongtobeat_main: 18,
  howlongtobeat_extra: 28,
  howlongtobeat_completionist: 45,
  igdb_id: 'igdb-123',
  is_verified: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z'
};

// Mock user game (includes game metadata + user-specific data)
export const mockUserGame = {
  id: 'user-game-1',
  game: mockGameMetadata,
  ownership_status: 'owned' as const,
  is_physical: false,
  physical_location: undefined,
  personal_rating: 4,
  is_loved: true,
  play_status: 'completed' as const,
  hours_played: 25,
  personal_notes: 'Great game!',
  acquired_date: '2024-01-01',
  platforms: [],
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z'
};

export const mockUserGames = [
  mockUserGame,
  {
    ...mockUserGame,
    id: 'user-game-2',
    game: {
      ...mockGameMetadata,
      id: 'game-2',
      title: 'Another Game',
      cover_art_url: 'https://example.com/cover2.jpg'
    },
    play_status: 'in_progress' as const,
    personal_rating: 5,
    hours_played: 10
  },
  {
    ...mockUserGame,
    id: 'user-game-3',
    game: {
      ...mockGameMetadata,
      id: 'game-3',
      title: 'Third Game',
      cover_art_url: 'https://example.com/cover3.jpg'
    },
    play_status: 'not_started' as const,
    personal_rating: null,
    hours_played: 0,
    is_loved: false
  }
];

// For backwards compatibility
export const mockGames = mockUserGames;

// Mock user games store
export const mockUserGamesStore = {
  value: {
    userGames: mockUserGames,
    currentUserGame: null,
    stats: null,
    isLoading: false,
    error: null,
    filters: {},
    pagination: {
      page: 1,
      per_page: 20,
      total: 3,
      pages: 1
    }
  },
  fetchUserGames: vi.fn(),
  loadUserGames: vi.fn(),
  addUserGame: vi.fn(),
  addGameToCollection: vi.fn(),
  updateUserGame: vi.fn(),
  updateProgress: vi.fn(),
  bulkUpdateStatus: vi.fn(),
  deleteUserGame: vi.fn(),
  clearError: vi.fn()
};

// Mock IGDB candidates for games store
export const mockIGDBCandidates = [
  {
    igdb_id: 'igdb-123',
    title: 'Test IGDB Game',
    release_date: '2024-01-01',
    cover_art_url: 'https://example.com/igdb-cover.jpg',
    description: 'A test game from IGDB',
    platforms: ['PC', 'PlayStation 5'],
    howlongtobeat_main: 12,  // Main story completion time in hours
    howlongtobeat_extra: 20,  // Main + extras completion time in hours
    howlongtobeat_completionist: 35  // Completionist time in hours
  }
];

// Mock games store
export const mockGamesStore = {
  value: {
    games: [],
    searchResults: [],
    igdbCandidates: mockIGDBCandidates,
    isLoading: false,
    isSearching: false,
    error: null
  },
  fetchGames: vi.fn(),
  fetchGame: vi.fn(),
  searchIGDB: vi.fn(),
  createFromIGDB: vi.fn(),
  createGame: vi.fn(),
  importFromIGDB: vi.fn(),
  addGame: vi.fn(),
  updateGame: vi.fn(),
  deleteGame: vi.fn(),
  refreshMetadata: vi.fn(),
  clearSearchResults: vi.fn(),
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
  loadPlatforms: vi.fn(),
  loadStorefronts: vi.fn(),
  loadAll: vi.fn(),
  fetchPlatforms: vi.fn(),
  fetchStorefronts: vi.fn(),
  clearError: vi.fn(),
  getActivePlatforms: vi.fn(() => [
    { id: 'pc', name: 'PC', display_name: 'PC' },
    { id: 'ps5', name: 'PlayStation 5', display_name: 'PlayStation 5' }
  ]),
  getActiveStorefronts: vi.fn(() => [
    { id: 'steam', name: 'Steam', display_name: 'Steam' },
    { id: 'epic', name: 'Epic Games Store', display_name: 'Epic Games Store' }
  ])
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
    sidebarOpen: false,
    mobileMenuOpen: false
  },
  toggleSidebar: vi.fn(),
  toggleMobileMenu: vi.fn()
};

// Mock auth store (importing from auth mocks)
import { mockAuthStore } from './auth-mocks';

// Mock all stores
vi.mock('$lib/stores', () => ({
  auth: mockAuthStore,
  userGames: mockUserGamesStore,
  games: mockGamesStore,
  platforms: mockPlatformsStore,
  search: mockSearchStore,
  ui: mockUIStore
}));

// Mock user-games store specifically
vi.mock('$lib/stores/user-games.svelte', () => ({
  userGames: mockUserGamesStore,
  OwnershipStatus: {
    OWNED: 'owned',
    BORROWED: 'borrowed',
    RENTED: 'rented',
    SUBSCRIPTION: 'subscription'
  },
  PlayStatus: {
    NOT_STARTED: 'not_started',
    IN_PROGRESS: 'in_progress',
    COMPLETED: 'completed',
    MASTERED: 'mastered',
    DOMINATED: 'dominated',
    SHELVED: 'shelved',
    DROPPED: 'dropped',
    REPLAY: 'replay'
  }
}));

// Reset functions for test cleanup
export function resetStoresMocks() {
  // Reset user games store
  mockUserGamesStore.fetchUserGames.mockClear();
  mockUserGamesStore.loadUserGames.mockClear();
  mockUserGamesStore.addUserGame.mockClear();
  mockUserGamesStore.updateUserGame.mockClear();
  mockUserGamesStore.updateProgress.mockClear();
  mockUserGamesStore.bulkUpdateStatus.mockClear();
  mockUserGamesStore.deleteUserGame.mockClear();
  mockUserGamesStore.clearError.mockClear();
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
      total: 3,
      pages: 1
    }
  };

  // Reset games store
  mockGamesStore.fetchGames.mockClear();
  mockGamesStore.fetchGame.mockClear();
  mockGamesStore.searchIGDB.mockClear();
  mockGamesStore.createFromIGDB.mockClear();
  mockGamesStore.createGame.mockClear();
  mockGamesStore.importFromIGDB.mockClear();
  mockGamesStore.addGame.mockClear();
  mockGamesStore.updateGame.mockClear();
  mockGamesStore.deleteGame.mockClear();
  mockGamesStore.refreshMetadata.mockClear();
  mockGamesStore.clearSearchResults.mockClear();
  mockGamesStore.clearError.mockClear();
  mockGamesStore.value = {
    games: [],
    searchResults: [],
    igdbCandidates: mockIGDBCandidates,
    isLoading: false,
    isSearching: false,
    error: null
  };

  // Reset other stores
  mockPlatformsStore.loadPlatforms.mockClear();
  mockPlatformsStore.loadStorefronts.mockClear();
  mockPlatformsStore.loadAll.mockClear();
  mockPlatformsStore.fetchPlatforms.mockClear();
  mockPlatformsStore.fetchStorefronts.mockClear();
  mockPlatformsStore.clearError.mockClear();
  mockPlatformsStore.getActivePlatforms.mockClear();
  mockPlatformsStore.getActiveStorefronts.mockClear();

  mockSearchStore.setQuery.mockClear();
  mockSearchStore.setFilters.mockClear();
  mockSearchStore.search.mockClear();
  mockSearchStore.clearResults.mockClear();
  mockSearchStore.clearError.mockClear();

  mockUIStore.toggleSidebar.mockClear();
  mockUIStore.toggleMobileMenu.mockClear();
}