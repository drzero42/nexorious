import { browser } from '$app/environment';
import { loggers } from '$lib/services/logger';
import type { Game, GameSearchFilters } from './games.svelte';
import type { UserGame, UserGameFilters, PlayStatus, OwnershipStatus } from './user-games.svelte';

const log = loggers.ui;

export interface SearchQuery {
  q: string;
  filters: Record<string, any>;
  sortBy: string;
  sortOrder: 'asc' | 'desc';
}

export interface SavedSearch {
  id: string;
  name: string;
  query: SearchQuery;
  searchType: 'games' | 'user-games';
  created_at: string;
}

export interface SearchHistory {
  id: string;
  query: string;
  searchType: 'games' | 'user-games';
  timestamp: string;
}

export interface SearchState {
  // Current search
  currentQuery: string;
  currentFilters: Record<string, any>;
  currentSortBy: string;
  currentSortOrder: 'asc' | 'desc';
  searchType: 'games' | 'user-games';
  
  // Results
  isSearching: boolean;
  searchResults: (Game | UserGame)[];
  searchError: string | null;
  
  // Saved searches
  savedSearches: SavedSearch[];
  
  // Search history
  searchHistory: SearchHistory[];
  
  // Quick filters
  quickFilters: {
    games: GameSearchFilters;
    'user-games': UserGameFilters;
  };
}

const initialState: SearchState = {
  currentQuery: '',
  currentFilters: {},
  currentSortBy: 'title',
  currentSortOrder: 'asc',
  searchType: 'games',
  isSearching: false,
  searchResults: [],
  searchError: null,
  savedSearches: [],
  searchHistory: [],
  quickFilters: {
    games: {},
    'user-games': {}
  }
};

function createSearchStore() {
  let state = $state<SearchState>(initialState);

  // Initialize data from localStorage
  function initializeData() {
    if (!browser) return;
    
    const storedSearches = localStorage.getItem('saved-searches');
    if (storedSearches) {
      try {
        const parsedSearches = JSON.parse(storedSearches);
        state = { ...state, savedSearches: parsedSearches };
      } catch (error) {
        log.error('Failed to parse saved searches', error);
      }
    }

    const storedHistory = localStorage.getItem('search-history');
    if (storedHistory) {
      try {
        const parsedHistory = JSON.parse(storedHistory);
        state = { ...state, searchHistory: parsedHistory };
      } catch (error) {
        log.error('Failed to parse search history', error);
      }
    }
  }

  // Call initialization
  initializeData();

  // Function to save searches to localStorage
  function saveSavedSearches() {
    if (!browser) return;
    localStorage.setItem('saved-searches', JSON.stringify(state.savedSearches));
  }

  // Function to save history to localStorage
  function saveSearchHistory() {
    if (!browser) return;
    localStorage.setItem('search-history', JSON.stringify(state.searchHistory));
  }

  // Function to add search to history
  function addToHistory(query: string, searchType: 'games' | 'user-games') {
    if (!query.trim()) return;

    const historyItem: SearchHistory = {
      id: Math.random().toString(36).substring(2, 9),
      query,
      searchType,
      timestamp: new Date().toISOString()
    };

    // Remove duplicate if exists
    const filteredHistory = state.searchHistory.filter(
      item => !(item.query === query && item.searchType === searchType)
    );

    // Add to beginning and limit to 50 items
    const newHistory = [historyItem, ...filteredHistory].slice(0, 50);

    state = { ...state, searchHistory: newHistory };
    saveSearchHistory();
  }

  const searchStore = {
    get value() {
      return state;
    },

    // Set search query
    setQuery: (query: string) => {
      state = { ...state, currentQuery: query };
    },

    // Set search filters
    setFilters: (filters: Record<string, any>) => {
      state = { ...state, currentFilters: filters };
    },

    // Set sort options
    setSorting: (sortBy: string, sortOrder: 'asc' | 'desc' = 'asc') => {
      state = { ...state, currentSortBy: sortBy, currentSortOrder: sortOrder };
    },

    // Set search type
    setSearchType: (searchType: 'games' | 'user-games') => {
      state = { ...state, searchType };
    },

    // Perform search
    performSearch: async (query?: string, filters?: Record<string, any>) => {
      const searchQuery = query !== undefined ? query : state.currentQuery;
      const searchFilters = filters !== undefined ? filters : state.currentFilters;

      state = { ...state, isSearching: true, searchError: null };

      try {
        // Add to history
        addToHistory(searchQuery, state.searchType);

        // Import the appropriate store dynamically to avoid circular dependencies
        if (state.searchType === 'games') {
          const { games } = await import('./games.svelte');
          await games.loadGames(searchFilters as GameSearchFilters);
          state = { ...state, searchResults: games.value.games, isSearching: false };
        } else {
          const { userGames } = await import('./user-games.svelte');
          await userGames.loadUserGames(searchFilters as UserGameFilters);
          state = { ...state, searchResults: userGames.value.userGames, isSearching: false };
        }
      } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Search failed';
        state = { ...state, isSearching: false, searchError: errorMessage };
        throw error;
      }
    },

    // Quick search (debounced)
    quickSearch: (() => {
      let timeoutId: ReturnType<typeof setTimeout>;
      
      return (query: string, delay: number = 300) => {
        clearTimeout(timeoutId);
        state = { ...state, currentQuery: query };
        
        timeoutId = setTimeout(() => {
          if (query.trim()) {
            searchStore.performSearch(query);
          } else {
            state = { ...state, searchResults: [] };
          }
        }, delay);
      };
    })(),

    // Clear search
    clearSearch: () => {
      state = {
        ...state,
        currentQuery: '',
        currentFilters: {},
        searchResults: [],
        searchError: null
      };
    },

    // Save current search
    saveCurrentSearch: (name: string) => {
      if (!name.trim()) return;

      const savedSearch: SavedSearch = {
        id: Math.random().toString(36).substring(2, 9),
        name,
        query: {
          q: state.currentQuery,
          filters: state.currentFilters,
          sortBy: state.currentSortBy,
          sortOrder: state.currentSortOrder
        },
        searchType: state.searchType,
        created_at: new Date().toISOString()
      };

      state = {
        ...state,
        savedSearches: [...state.savedSearches, savedSearch]
      };

      saveSavedSearches();
      return savedSearch;
    },

    // Load saved search
    loadSavedSearch: (id: string) => {
      const savedSearch = state.savedSearches.find(s => s.id === id);
      if (!savedSearch) return;

      state = {
        ...state,
        currentQuery: savedSearch.query.q,
        currentFilters: savedSearch.query.filters,
        currentSortBy: savedSearch.query.sortBy,
        currentSortOrder: savedSearch.query.sortOrder,
        searchType: savedSearch.searchType
      };

      // Perform the search
      searchStore.performSearch();
    },

    // Delete saved search
    deleteSavedSearch: (id: string) => {
      state = {
        ...state,
        savedSearches: state.savedSearches.filter(s => s.id !== id)
      };
      saveSavedSearches();
    },

    // Clear search history
    clearSearchHistory: () => {
      state = { ...state, searchHistory: [] };
      saveSearchHistory();
    },

    // Remove specific history item
    removeFromHistory: (id: string) => {
      state = {
        ...state,
        searchHistory: state.searchHistory.filter(h => h.id !== id)
      };
      saveSearchHistory();
    },

    // Set quick filters
    setQuickFilters: (type: 'games' | 'user-games', filters: Record<string, any>) => {
      state = {
        ...state,
        quickFilters: {
          ...state.quickFilters,
          [type]: filters
        }
      };
    },

    // Apply quick filter
    applyQuickFilter: (type: 'games' | 'user-games', filterKey: string, filterValue: any) => {
      const currentFilters = state.quickFilters[type];
      const newFilters = { ...currentFilters, [filterKey]: filterValue };
      
      state = {
        ...state,
        quickFilters: {
          ...state.quickFilters,
          [type]: newFilters
        },
        currentFilters: newFilters,
        searchType: type
      };

      // Perform search with new filters
      searchStore.performSearch();
    },

    // Remove quick filter
    removeQuickFilter: (type: 'games' | 'user-games', filterKey: string) => {
      const currentFilters = state.quickFilters[type];
      const newFilters = { ...currentFilters };
      delete (newFilters as any)[filterKey];
      
      state = {
        ...state,
        quickFilters: {
          ...state.quickFilters,
          [type]: newFilters
        },
        currentFilters: newFilters,
        searchType: type
      };

      // Perform search with updated filters
      searchStore.performSearch();
    },

    // Predefined quick filters for user games
    filterByPlayStatus: (status: PlayStatus) => {
      searchStore.applyQuickFilter('user-games', 'play_status', status);
    },

    filterByOwnershipStatus: (status: OwnershipStatus) => {
      searchStore.applyQuickFilter('user-games', 'ownership_status', status);
    },

    filterByLovedGames: () => {
      searchStore.applyQuickFilter('user-games', 'is_loved', true);
    },

    filterByPlatform: (platformId: string) => {
      searchStore.applyQuickFilter('user-games', 'platform_id', platformId);
    },

    filterByRating: (minRating: number, maxRating?: number) => {
      searchStore.applyQuickFilter('user-games', 'rating_min', minRating);
      if (maxRating) {
        searchStore.applyQuickFilter('user-games', 'rating_max', maxRating);
      }
    },

    // Predefined quick filters for games
    filterByGenre: (genre: string) => {
      searchStore.applyQuickFilter('games', 'genre', genre);
    },

    filterByDeveloper: (developer: string) => {
      searchStore.applyQuickFilter('games', 'developer', developer);
    },


    // Clear all filters
    clearAllFilters: () => {
      state = {
        ...state,
        currentFilters: {},
        quickFilters: {
          games: {},
          'user-games': {}
        }
      };
    },

    // Clear error
    clearError: () => {
      state = { ...state, searchError: null };
    }
  };
  
  return searchStore;
}

export const search = createSearchStore();