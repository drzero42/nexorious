import { vi } from 'vitest';

// Mock game metadata (separate from user-specific data)
export const mockGameMetadata = {
  id: 'game-1',
  title: 'Test Game',
  description: 'A test game description',
  genre: 'Action',
  developer: 'Test Developer',
  publisher: 'Test Publisher',
  release_date: '2023-01-01',
  cover_art_url: 'https://example.com/cover.jpg',
  rating_average: 85.5,  // IGDB rating out of 100
  rating_count: 100,
  game_metadata: '{}',
  estimated_playtime_hours: 25,
  howlongtobeat_main: 18,
  howlongtobeat_extra: 28,
  howlongtobeat_completionist: 45,
  igdb_id: 'igdb-123',
  igdb_slug: 'test-game-slug',
  created_at: '2023-01-01T00:00:00.000Z',
  updated_at: '2023-01-01T00:00:00.000Z'
};

// Mock user game (includes game metadata + user-specific data)
export const mockUserGame = {
  id: 'user-game-1',
  game: mockGameMetadata,
  ownership_status: 'owned' as const,
  personal_rating: 4,
  is_loved: true,
  play_status: 'completed' as const,
  hours_played: 25,
  personal_notes: 'Great game!',
  acquired_date: '2023-01-01',
  platforms: [
    {
      id: 'platform-1',
      platform: {
        id: 'pc-windows',
        name: 'pc-windows',
        display_name: 'PC (Windows)',
        icon_url: 'https://example.com/pc-icon.png',
        is_active: true,
        source: 'official',
        version_added: '1.0.0',
        created_at: '2023-01-01T00:00:00.000Z',
        updated_at: '2023-01-01T00:00:00.000Z'
      },
      storefront: {
        id: 'steam',
        name: 'steam',
        display_name: 'Steam',
        icon_url: 'https://example.com/steam-icon.png',
        base_url: 'https://store.steampowered.com',
        is_active: true,
        source: 'official',
        version_added: '1.0.0',
        created_at: '2023-01-01T00:00:00.000Z',
        updated_at: '2023-01-01T00:00:00.000Z'
      },
      store_game_id: 'steam-123',
      is_available: true,
      created_at: '2023-01-01T00:00:00.000Z'
    }
  ],
  created_at: '2023-01-01T00:00:00.000Z',
  updated_at: '2023-01-01T00:00:00.000Z'
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

// Mock user games store with entityState support
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
  entityState: {
    optimisticUpdates: {
      isPending: false,
      isPendingFor: vi.fn(() => false)
    },
    bulkOperations: {
      isProcessing: false
    }
  },
  selectors: {
    byId: vi.fn((id) => mockUserGames.find(g => g.id === id) || null)
  },
  fetchUserGames: vi.fn(),
  loadUserGames: vi.fn(),
  getUserGame: vi.fn(),
  addUserGame: vi.fn(),
  addGameToCollection: vi.fn(),
  updateUserGame: vi.fn(),
  updateProgress: vi.fn(),
  bulkUpdateStatus: vi.fn(),
  deleteUserGame: vi.fn(),
  clearError: vi.fn(),
  clearFilters: vi.fn(),
  clearCurrentUserGame: vi.fn(),
  getGamesByStatus: vi.fn((status) => mockUserGames.filter(g => g.play_status === status)),
  getLovedGames: vi.fn(() => mockUserGames.filter(g => g.is_loved)),
  getGamesByRating: vi.fn((rating) => mockUserGames.filter(g => g.personal_rating === rating)),
  getPileOfShame: vi.fn(() => mockUserGames.filter(g => g.play_status === 'not_started')),
  __testSetData: vi.fn((games) => {
    mockUserGamesStore.value.userGames = games;
  }),
  on: vi.fn(),
  off: vi.fn(),
  emit: vi.fn()
};

// Mock IGDB candidates for games store
export const mockIGDBCandidates = [
  {
    igdb_id: 'igdb-123',
    igdb_slug: 'test-igdb-game-slug',
    title: 'Test IGDB Game',
    release_date: '2023-01-01',
    cover_art_url: 'https://example.com/igdb-cover.jpg',
    description: 'A test game from IGDB',
    platforms: ['PC (Windows)', 'PlayStation 5'], // Updated to match mock platform display_names
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
  importFromIGDB: vi.fn(),
  addGame: vi.fn(),
  updateGame: vi.fn(),
  deleteGame: vi.fn(),
  refreshMetadata: vi.fn(),
  clearSearchResults: vi.fn(),
  clearError: vi.fn()
};

// Mock storefronts (first, since platforms reference them)
export const mockStorefronts = [
  { 
    id: 'steam', 
    name: 'steam', 
    display_name: 'Steam', 
    icon_url: 'https://example.com/steam-icon.png',
    base_url: 'https://store.steampowered.com',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'epic-games-store', 
    name: 'epic-games-store', 
    display_name: 'Epic Games Store', 
    icon_url: 'https://example.com/epic-icon.png',
    base_url: 'https://store.epicgames.com',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'playstation-store', 
    name: 'playstation-store', 
    display_name: 'PlayStation Store', 
    icon_url: 'https://example.com/ps-icon.png',
    base_url: 'https://store.playstation.com',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'nintendo-eshop', 
    name: 'nintendo-eshop', 
    display_name: 'Nintendo eShop', 
    icon_url: 'https://example.com/nintendo-icon.png',
    base_url: 'https://www.nintendo.com/store',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'physical', 
    name: 'physical', 
    display_name: 'Physical', 
    icon_url: 'https://example.com/physical-icon.png',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  }
];

// Mock platforms with realistic default storefront relationships
export const mockPlatforms = [
  { 
    id: 'pc-windows', 
    name: 'pc-windows', 
    display_name: 'PC (Windows)', 
    icon_url: 'https://example.com/pc-icon.png',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    default_storefront_id: 'steam', // PC defaults to Steam
    storefronts: [
      mockStorefronts.find(s => s.id === 'steam'),
      mockStorefronts.find(s => s.id === 'epic-games-store'),
      mockStorefronts.find(s => s.id === 'physical')
    ].filter(Boolean),
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'playstation-5', 
    name: 'playstation-5', 
    display_name: 'PlayStation 5', 
    icon_url: 'https://example.com/ps5-icon.png',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    default_storefront_id: 'playstation-store', // PS5 defaults to PlayStation Store
    storefronts: [
      mockStorefronts.find(s => s.id === 'playstation-store'),
      mockStorefronts.find(s => s.id === 'physical')
    ].filter(Boolean),
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'nintendo-switch', 
    name: 'nintendo-switch', 
    display_name: 'Nintendo Switch', 
    icon_url: 'https://example.com/switch-icon.png',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    default_storefront_id: 'nintendo-eshop', // Switch defaults to Nintendo eShop
    storefronts: [
      mockStorefronts.find(s => s.id === 'nintendo-eshop'),
      mockStorefronts.find(s => s.id === 'physical')
    ].filter(Boolean),
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  },
  { 
    id: 'mobile-android', 
    name: 'mobile-android', 
    display_name: 'Android', 
    icon_url: 'https://example.com/android-icon.png',
    is_active: true, 
    source: 'official', 
    version_added: '1.0.0',
    default_storefront_id: null, // No default storefront for Android (to test platforms without defaults)
    storefronts: [],
    created_at: '2023-01-01T00:00:00.000Z', 
    updated_at: '2023-01-01T00:00:00.000Z' 
  }
];

// Mock platforms store with proper Svelte store pattern
export const mockPlatformsStore = {
  subscribe: vi.fn((callback) => {
    // Store the callback for later triggering
    const storeState = {
      platforms: mockPlatforms,
      storefronts: mockStorefronts,
      isLoading: false,
      error: null
    };
    
    // Call callback immediately
    callback(storeState);
    
    // Also call it after a microtask to ensure reactivity
    Promise.resolve().then(() => callback(storeState));
    
    // Return an unsubscribe function
    return () => {};
  }),
  fetchPlatforms: vi.fn().mockResolvedValue(mockPlatforms),
  fetchStorefronts: vi.fn().mockResolvedValue(mockStorefronts),
  fetchAll: vi.fn().mockResolvedValue({
    platforms: mockPlatforms,
    storefronts: mockStorefronts
  }),
  createPlatform: vi.fn(),
  updatePlatform: vi.fn(),
  deletePlatform: vi.fn(),
  createStorefront: vi.fn(),
  updateStorefront: vi.fn(),
  deleteStorefront: vi.fn(),
  fetchActivePlatformsAndStorefronts: vi.fn().mockResolvedValue({
    platforms: mockPlatforms,
    storefronts: mockStorefronts
  }),
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
    notifications: [],
    modals: [],
    isLoading: false,
    loadingMessage: undefined,
    sidebar: {
      isOpen: false,
      isPinned: false
    },
    preferences: {
      density: 'comfortable' as const,
      animations: true,
      pageSize: 20
    }
  },
  toggleSidebar: vi.fn(),
  toggleMobileMenu: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn(),
  showInfo: vi.fn(),
  addNotification: vi.fn(),
  removeNotification: vi.fn(),
  clearNotifications: vi.fn(),
  openModal: vi.fn(),
  closeModal: vi.fn(),
  setLoading: vi.fn(),
  updatePreferences: vi.fn()
};

// Mock Steam store
export const mockSteamStore = {
  value: {
    config: null,
    isLoading: false,
    isVerifying: false,
    isResolvingVanity: false,
    error: null,
    verificationResult: null
  },
  reset: vi.fn(),
  getConfig: vi.fn().mockResolvedValue({
    has_api_key: false,
    api_key_masked: null,
    steam_id: null,
    is_verified: false,
    configured_at: null
  }),
  setConfig: vi.fn().mockResolvedValue({
    has_api_key: true,
    api_key_masked: 'ABCD1234****5678',
    steam_id: '76561198123456789',
    is_verified: true,
    configured_at: new Date()
  }),
  deleteConfig: vi.fn().mockResolvedValue(true),
  verify: vi.fn().mockResolvedValue({
    is_valid: true,
    error_message: null,
    steam_user_info: {
      steam_id: '76561198123456789',
      persona_name: 'TestUser',
      profile_url: 'https://steamcommunity.com/profiles/76561198123456789',
      avatar: 'https://example.com/avatar.jpg',
      avatar_medium: 'https://example.com/avatar_medium.jpg',
      avatar_full: 'https://example.com/avatar_full.jpg'
    }
  }),
  resolveVanityUrl: vi.fn().mockResolvedValue({
    success: true,
    steam_id: '76561198123456789',
    error_message: null
  }),
  clearVerification: vi.fn(),
  clearError: vi.fn()
};

// Mock tags for the tags store
export const mockTags = [
  {
    id: 'tag-1',
    user_id: 'user-1',
    name: 'Action',
    color: '#FF0000',
    description: 'Action games',
    game_count: 2,
    created_at: '2023-01-01T00:00:00.000Z',
    updated_at: '2023-01-01T00:00:00.000Z'
  },
  {
    id: 'tag-2',
    user_id: 'user-1',
    name: 'RPG',
    color: '#00FF00',
    description: 'Role playing games',
    game_count: 1,
    created_at: '2023-01-01T00:00:00.000Z',
    updated_at: '2023-01-01T00:00:00.000Z'
  },
  {
    id: 'tag-3',
    user_id: 'user-1',
    name: 'Strategy',
    color: '#0000FF',
    description: 'Strategy games',
    game_count: 0,
    created_at: '2023-01-01T00:00:00.000Z',
    updated_at: '2023-01-01T00:00:00.000Z'
  }
];

// Mock tags store
export const mockTagsStore = {
  value: {
    tags: mockTags,
    usageStats: {
      total_tags: 3,
      total_tagged_games: 2,
      average_tags_per_game: 1.5,
      tag_usage: {
        'tag-1': 2,
        'tag-2': 1,
        'tag-3': 0
      },
      popular_tags: [mockTags[0], mockTags[1]],
      unused_tags: [mockTags[2]]
    },
    isLoading: false,
    error: null
  },
  fetchTags: vi.fn().mockResolvedValue(mockTags),
  createTag: vi.fn().mockResolvedValue(mockTags[0]),
  updateTag: vi.fn().mockResolvedValue(mockTags[0]),
  deleteTag: vi.fn().mockResolvedValue(true),
  createOrGetTag: vi.fn().mockResolvedValue({ tag: mockTags[0], created: true }),
  assignTagsToGame: vi.fn().mockResolvedValue(true),
  removeTagsFromGame: vi.fn().mockResolvedValue(true),
  getTagUsageStats: vi.fn().mockResolvedValue({
    total_tags: 3,
    total_tagged_games: 2,
    average_tags_per_game: 1.5,
    tag_usage: { 'tag-1': 2, 'tag-2': 1, 'tag-3': 0 },
    popular_tags: [mockTags[0], mockTags[1]],
    unused_tags: [mockTags[2]]
  }),
  suggestColor: vi.fn().mockReturnValue('#6B7280'),
  clearError: vi.fn()
};

// Mock auth store (importing from auth mocks)
import { mockAuthStore } from './auth-mocks';

// Mock all stores
vi.mock('$lib/stores', () => ({
  auth: mockAuthStore,
  userGames: mockUserGamesStore,
  games: mockGamesStore,
  tags: mockTagsStore,
  platforms: mockPlatformsStore,
  search: mockSearchStore,
  ui: mockUIStore,
  steam: mockSteamStore
}));

// Mock platforms store specifically
vi.mock('$lib/stores/platforms.svelte', () => ({
  platforms: mockPlatformsStore
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

// Mock ui store specifically
vi.mock('$lib/stores/ui.svelte', () => ({
  ui: mockUIStore
}));

// Mock steam store specifically
vi.mock('$lib/stores/steam.svelte', () => ({
  steam: mockSteamStore
}));

vi.mock('$lib/stores/tags.svelte', () => ({
  tags: mockTagsStore
}));

// Reset functions for test cleanup
export function resetStoresMocks() {
  // Reset user games store
  mockUserGamesStore.fetchUserGames.mockClear();
  mockUserGamesStore.loadUserGames.mockClear();
  mockUserGamesStore.getUserGame.mockClear();
  mockUserGamesStore.addUserGame.mockClear();
  mockUserGamesStore.addGameToCollection.mockClear();
  mockUserGamesStore.updateUserGame.mockClear();
  mockUserGamesStore.updateProgress.mockClear();
  mockUserGamesStore.bulkUpdateStatus.mockClear();
  mockUserGamesStore.deleteUserGame.mockClear();
  mockUserGamesStore.clearError.mockClear();
  
  // Reset to resolved promises by default
  mockUserGamesStore.getUserGame.mockImplementation((gameId: string) => {
    const game = mockUserGames.find(g => g.id === gameId);
    return Promise.resolve(game || null);
  });
  mockUserGamesStore.addGameToCollection.mockResolvedValue({
    id: 'user-game-1',
    game_id: 'game-1'
  });
  mockUserGamesStore.updateProgress.mockResolvedValue({});
  mockUserGamesStore.updateUserGame.mockResolvedValue({});
  
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

  // Reset entityState
  mockUserGamesStore.entityState = {
    optimisticUpdates: {
      isPending: false,
      isPendingFor: vi.fn(() => false)
    },
    bulkOperations: {
      isProcessing: false
    }
  };

  // Reset selectors
  mockUserGamesStore.selectors = {
    byId: vi.fn((id) => mockUserGames.find(g => g.id === id) || null)
  };

  // Reset additional methods
  mockUserGamesStore.clearFilters.mockClear();
  mockUserGamesStore.clearCurrentUserGame.mockClear();
  mockUserGamesStore.getGamesByStatus.mockClear();
  mockUserGamesStore.getLovedGames.mockClear();
  mockUserGamesStore.getGamesByRating.mockClear();
  mockUserGamesStore.getPileOfShame.mockClear();
  mockUserGamesStore.__testSetData.mockClear();

  // Reset implementations
  mockUserGamesStore.getGamesByStatus.mockImplementation((status) => mockUserGames.filter(g => g.play_status === status));
  mockUserGamesStore.getLovedGames.mockImplementation(() => mockUserGames.filter(g => g.is_loved));
  mockUserGamesStore.getGamesByRating.mockImplementation((rating) => mockUserGames.filter(g => g.personal_rating === rating));
  mockUserGamesStore.getPileOfShame.mockImplementation(() => mockUserGames.filter(g => g.play_status === 'not_started'));
  mockUserGamesStore.__testSetData.mockImplementation((games) => {
    mockUserGamesStore.value.userGames = games;
  });

  // Reset event methods
  mockUserGamesStore.on.mockClear();
  mockUserGamesStore.off.mockClear();
  mockUserGamesStore.emit.mockClear();

  // Reset games store
  mockGamesStore.fetchGames.mockClear();
  mockGamesStore.fetchGame.mockClear();
  mockGamesStore.searchIGDB.mockClear();
  mockGamesStore.createFromIGDB.mockClear();
  mockGamesStore.importFromIGDB.mockClear();
  mockGamesStore.addGame.mockClear();
  mockGamesStore.updateGame.mockClear();
  mockGamesStore.deleteGame.mockClear();
  mockGamesStore.refreshMetadata.mockClear();
  mockGamesStore.clearSearchResults.mockClear();
  mockGamesStore.clearError.mockClear();
  
  // Reset to resolved promises by default
  mockGamesStore.searchIGDB.mockResolvedValue({
    games: mockIGDBCandidates
  });
  mockGamesStore.createFromIGDB.mockResolvedValue(mockGameMetadata);
  
  mockGamesStore.value = {
    games: [],
    searchResults: [],
    igdbCandidates: mockIGDBCandidates,
    isLoading: false,
    isSearching: false,
    error: null
  };

  // Reset platforms store
  mockPlatformsStore.fetchPlatforms.mockClear();
  mockPlatformsStore.fetchStorefronts.mockClear();
  mockPlatformsStore.fetchAll.mockClear();
  mockPlatformsStore.createPlatform.mockClear();
  mockPlatformsStore.updatePlatform.mockClear();
  mockPlatformsStore.deletePlatform.mockClear();
  mockPlatformsStore.createStorefront.mockClear();
  mockPlatformsStore.updateStorefront.mockClear();
  mockPlatformsStore.deleteStorefront.mockClear();
  mockPlatformsStore.fetchActivePlatformsAndStorefronts.mockClear();
  mockPlatformsStore.clearError.mockClear();

  // Reset other stores
  mockSearchStore.setQuery.mockClear();
  mockSearchStore.setFilters.mockClear();
  mockSearchStore.search.mockClear();
  mockSearchStore.clearResults.mockClear();
  mockSearchStore.clearError.mockClear();

  mockUIStore.toggleSidebar.mockClear();
  mockUIStore.toggleMobileMenu.mockClear();
  mockUIStore.showSuccess.mockClear();
  mockUIStore.showError.mockClear();
  mockUIStore.showWarning.mockClear();
  mockUIStore.showInfo.mockClear();
  mockUIStore.addNotification.mockClear();
  mockUIStore.removeNotification.mockClear();
  mockUIStore.clearNotifications.mockClear();
  mockUIStore.openModal.mockClear();
  mockUIStore.closeModal.mockClear();
  mockUIStore.setLoading.mockClear();
  mockUIStore.updatePreferences.mockClear();

  // Reset UI store value to default state
  mockUIStore.value = {
    notifications: [],
    modals: [],
    isLoading: false,
    loadingMessage: undefined,
    sidebar: {
      isOpen: false,
      isPinned: false
    },
    preferences: {
      density: 'comfortable' as const,
      animations: true,
      pageSize: 20
    }
  };

  // Reset steam store
  mockSteamStore.reset.mockClear();
  mockSteamStore.getConfig.mockClear();
  mockSteamStore.setConfig.mockClear();
  mockSteamStore.deleteConfig.mockClear();
  mockSteamStore.verify.mockClear();
  mockSteamStore.resolveVanityUrl.mockClear();
  mockSteamStore.clearVerification.mockClear();
  mockSteamStore.clearError.mockClear();

  // Reset to resolved promises by default
  mockSteamStore.getConfig.mockResolvedValue({
    has_api_key: false,
    api_key_masked: null,
    steam_id: null,
    is_verified: false,
    configured_at: null
  });
  mockSteamStore.setConfig.mockResolvedValue({
    has_api_key: true,
    api_key_masked: 'ABCD1234****5678',
    steam_id: '76561198123456789',
    is_verified: true,
    configured_at: new Date()
  });
  mockSteamStore.deleteConfig.mockResolvedValue(true);
  mockSteamStore.verify.mockResolvedValue({
    is_valid: true,
    error_message: null,
    steam_user_info: {
      steam_id: '76561198123456789',
      persona_name: 'TestUser',
      profile_url: 'https://steamcommunity.com/profiles/76561198123456789',
      avatar: 'https://example.com/avatar.jpg',
      avatar_medium: 'https://example.com/avatar_medium.jpg',
      avatar_full: 'https://example.com/avatar_full.jpg'
    }
  });
  mockSteamStore.resolveVanityUrl.mockResolvedValue({
    success: true,
    steam_id: '76561198123456789',
    error_message: null
  });

  mockSteamStore.value = {
    config: null,
    isLoading: false,
    isVerifying: false,
    isResolvingVanity: false,
    error: null,
    verificationResult: null
  };

  // Reset tags store
  mockTagsStore.fetchTags.mockClear();
  mockTagsStore.createTag.mockClear();
  mockTagsStore.updateTag.mockClear();
  mockTagsStore.deleteTag.mockClear();
  mockTagsStore.createOrGetTag.mockClear();
  mockTagsStore.assignTagsToGame.mockClear();
  mockTagsStore.removeTagsFromGame.mockClear();
  mockTagsStore.getTagUsageStats.mockClear();
  mockTagsStore.suggestColor.mockClear();
  mockTagsStore.clearError.mockClear();

  // Reset to resolved promises by default
  mockTagsStore.fetchTags.mockResolvedValue(mockTags);
  mockTagsStore.createTag.mockResolvedValue(mockTags[0]);
  mockTagsStore.updateTag.mockResolvedValue(mockTags[0]);
  mockTagsStore.deleteTag.mockResolvedValue(true);
  mockTagsStore.createOrGetTag.mockResolvedValue({ tag: mockTags[0], created: true });
  mockTagsStore.assignTagsToGame.mockResolvedValue(true);
  mockTagsStore.removeTagsFromGame.mockResolvedValue(true);
  mockTagsStore.getTagUsageStats.mockResolvedValue({
    total_tags: 3,
    total_tagged_games: 2,
    average_tags_per_game: 1.5,
    tag_usage: { 'tag-1': 2, 'tag-2': 1, 'tag-3': 0 },
    popular_tags: [mockTags[0], mockTags[1]],
    unused_tags: [mockTags[2]]
  });
  mockTagsStore.suggestColor.mockReturnValue('#6B7280');

  mockTagsStore.value = {
    tags: mockTags,
    usageStats: {
      total_tags: 3,
      total_tagged_games: 2,
      average_tags_per_game: 1.5,
      tag_usage: {
        'tag-1': 2,
        'tag-2': 1,
        'tag-3': 0
      },
      popular_tags: [mockTags[0], mockTags[1]],
      unused_tags: [mockTags[2]]
    },
    isLoading: false,
    error: null
  };
}