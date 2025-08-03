import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Set up Svelte runes mock before any other imports that might use them
if (typeof globalThis !== 'undefined' && !(globalThis as any).$state) {
	(globalThis as any).$state = vi.fn((initialValue: any) => {
		if (typeof initialValue === 'object' && initialValue !== null) {
			const state = { ...initialValue };
			return new Proxy(state, {
				get(target, prop) {
					return target[prop as keyof typeof target];
				},
				set(target, prop, value) {
					(target as any)[prop] = value;
					return true;
				}
			});
		}
		return { value: initialValue };
	});
	(globalThis as any).$derived = vi.fn((fn: () => any) => fn());
	(globalThis as any).$effect = vi.fn(() => () => {});
	(globalThis as any).$props = vi.fn(() => ({}));
}

// Import enums first
const PlayStatus = {
	NOT_STARTED: 'not_started',
	IN_PROGRESS: 'in_progress',
	COMPLETED: 'completed',
	MASTERED: 'mastered',
	DOMINATED: 'dominated',
	SHELVED: 'shelved',
	DROPPED: 'dropped',
	REPLAY: 'replay'
} as const;

const OwnershipStatus = {
	OWNED: 'owned',
	BORROWED: 'borrowed',
	RENTED: 'rented',
	SUBSCRIPTION: 'subscription'
} as const;

// Mock localStorage
const localStorageMock = {
	getItem: vi.fn(),
	setItem: vi.fn(),
	removeItem: vi.fn(),
	clear: vi.fn(),
};
Object.defineProperty(window, 'localStorage', {
	value: localStorageMock
});

// Mock the dynamic imports
vi.mock('./games.svelte', () => ({
	games: {
		value: {
			games: [
				{
					id: '1',
					title: 'Test Game',
					description: 'A test game',
					genre: 'Action',
					developer: 'Test Developer',
					publisher: 'Test Publisher',
					release_date: '2023-01-01',
					cover_art_url: 'https://example.com/cover.jpg',
					rating_count: 100,
					game_metadata: '{}',
					is_verified: true,
					created_at: '2023-01-01T00:00:00Z',
					updated_at: '2023-01-01T00:00:00Z'
				}
			]
		},
		loadGames: vi.fn().mockResolvedValue(undefined)
	}
}));

vi.mock('./user-games.svelte', () => ({
	userGames: {
		value: {
			userGames: [
				{
					id: '1',
					game: {
						id: '1',
						title: 'User Game',
						description: 'A user game',
						genre: 'RPG',
						developer: 'User Developer',
						publisher: 'User Publisher',
						release_date: '2023-02-01',
						cover_art_url: 'https://example.com/user-cover.jpg',
						rating_count: 50,
						game_metadata: '{}',
						is_verified: true,
						created_at: '2023-02-01T00:00:00Z',
						updated_at: '2023-02-01T00:00:00Z'
					},
					ownership_status: 'owned',
					is_physical: false,
					personal_rating: 4.5,
					is_loved: true,
					play_status: 'in_progress',
					hours_played: 25,
					personal_notes: 'Great game!',
					acquired_date: '2023-02-01',
					platforms: [],
					created_at: '2023-02-01T00:00:00Z',
					updated_at: '2023-02-01T00:00:00Z'
				}
			]
		},
		loadUserGames: vi.fn().mockResolvedValue(undefined)
	},
	PlayStatus,
	OwnershipStatus
}));

// Mock the app environment
vi.mock('$app/environment', () => ({
	browser: true,
	dev: false
}));

describe('Search Store', () => {
	let search: any;
	let mockGamesStore: any;
	let mockUserGamesStore: any;

	beforeEach(async () => {
		vi.clearAllMocks();
		localStorageMock.getItem.mockReturnValue(null);
		
		// Get references to mocked stores
		const gamesModule = await import('./games.svelte');
		const userGamesModule = await import('./user-games.svelte');
		mockGamesStore = gamesModule.games;
		mockUserGamesStore = userGamesModule.userGames;
		
		// Clear module cache and reimport
		vi.resetModules();
		const module = await import('./search.svelte');
		search = module.search;
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('Store Structure', () => {
		it('should have correct initial state', () => {
			const state = search.value;
			
			expect(state).toMatchObject({
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
			});
		});

		it('should have all required methods', () => {
			const requiredMethods = [
				'setQuery',
				'setFilters',
				'setSorting',
				'setSearchType',
				'performSearch',
				'quickSearch',
				'clearSearch',
				'saveCurrentSearch',
				'loadSavedSearch',
				'deleteSavedSearch',
				'clearSearchHistory',
				'removeFromHistory',
				'setQuickFilters',
				'applyQuickFilter',
				'removeQuickFilter',
				'filterByPlayStatus',
				'filterByOwnershipStatus',
				'filterByLovedGames',
				'filterByPlatform',
				'filterByRating',
				'filterByGenre',
				'filterByDeveloper',
				'clearAllFilters',
				'clearError'
			];

			requiredMethods.forEach(method => {
				expect(typeof search[method]).toBe('function');
			});
		});
	});

	describe('LocalStorage Initialization', () => {
		it('should load saved searches from localStorage', async () => {
			const savedSearches = [
				{
					id: 'search1',
					name: 'My Search',
					query: { q: 'test', filters: {}, sortBy: 'title', sortOrder: 'asc' },
					searchType: 'games',
					created_at: '2023-01-01T00:00:00Z'
				}
			];

			localStorageMock.getItem.mockImplementation((key) => {
				if (key === 'saved-searches') return JSON.stringify(savedSearches);
				return null;
			});

			// Reimport to trigger initialization
			vi.resetModules();
			const module = await import('./search.svelte');
			const freshSearch = module.search;

			expect(freshSearch.value.savedSearches).toEqual(savedSearches);
		});

		it('should load search history from localStorage', async () => {
			const searchHistory = [
				{
					id: 'history1',
					query: 'test game',
					searchType: 'games',
					timestamp: '2023-01-01T00:00:00Z'
				}
			];

			localStorageMock.getItem.mockImplementation((key) => {
				if (key === 'search-history') return JSON.stringify(searchHistory);
				return null;
			});

			// Reimport to trigger initialization
			vi.resetModules();
			const module = await import('./search.svelte');
			const freshSearch = module.search;

			expect(freshSearch.value.searchHistory).toEqual(searchHistory);
		});

		it('should handle localStorage parsing errors gracefully', async () => {
			const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
			localStorageMock.getItem.mockReturnValue('invalid json');

			// Reimport to trigger initialization
			vi.resetModules();
			const module = await import('./search.svelte');
			const freshSearch = module.search;

			expect(freshSearch.value.savedSearches).toEqual([]);
			expect(freshSearch.value.searchHistory).toEqual([]);
			expect(consoleSpy).toHaveBeenCalled();

			consoleSpy.mockRestore();
		});
	});

	describe('Basic Search Operations', () => {
		it('should set query', () => {
			search.setQuery('test query');
			expect(search.value.currentQuery).toBe('test query');
		});

		it('should set filters', () => {
			const filters = { genre: 'Action', developer: 'Test Dev' };
			search.setFilters(filters);
			expect(search.value.currentFilters).toEqual(filters);
		});

		it('should set sorting', () => {
			search.setSorting('rating', 'desc');
			expect(search.value.currentSortBy).toBe('rating');
			expect(search.value.currentSortOrder).toBe('desc');
		});

		it('should set sorting with default order', () => {
			search.setSorting('title');
			expect(search.value.currentSortBy).toBe('title');
			expect(search.value.currentSortOrder).toBe('asc');
		});

		it('should set search type', () => {
			search.setSearchType('user-games');
			expect(search.value.searchType).toBe('user-games');
		});
	});

	describe('Perform Search', () => {
		it('should perform games search successfully', async () => {
			mockGamesStore.loadGames.mockResolvedValueOnce(undefined);
			
			search.setSearchType('games');
			search.setQuery('test');
			
			await search.performSearch();

			expect(mockGamesStore.loadGames).toHaveBeenCalledWith({});
			expect(search.value.searchResults).toEqual(mockGamesStore.value.games);
			expect(search.value.isSearching).toBe(false);
			expect(search.value.searchError).toBeNull();
		});

		it('should perform user-games search successfully', async () => {
			mockUserGamesStore.loadUserGames.mockResolvedValueOnce(undefined);
			
			search.setSearchType('user-games');
			search.setQuery('user test');
			
			await search.performSearch();

			expect(mockUserGamesStore.loadUserGames).toHaveBeenCalledWith({});
			expect(search.value.searchResults).toEqual(mockUserGamesStore.value.userGames);
			expect(search.value.isSearching).toBe(false);
		});

		it('should handle search errors', async () => {
			const error = new Error('Search failed');
			mockGamesStore.loadGames.mockRejectedValueOnce(error);
			
			search.setSearchType('games');
			
			await expect(search.performSearch('test')).rejects.toThrow('Search failed');
			expect(search.value.isSearching).toBe(false);
			expect(search.value.searchError).toBe('Search failed');
		});

		it('should add query to search history', async () => {
			mockGamesStore.loadGames.mockResolvedValueOnce(undefined);
			
			await search.performSearch('test query');

			expect(search.value.searchHistory).toHaveLength(1);
			expect(search.value.searchHistory[0]).toMatchObject({
				query: 'test query',
				searchType: 'games'
			});
		});

		it('should not add empty queries to history', async () => {
			mockGamesStore.loadGames.mockResolvedValueOnce(undefined);
			
			await search.performSearch('   ');

			expect(search.value.searchHistory).toHaveLength(0);
		});
	});

	describe('Quick Search', () => {
		beforeEach(() => {
			vi.useFakeTimers();
		});

		afterEach(() => {
			vi.useRealTimers();
		});

		it('should debounce search calls', async () => {
			const performSearchSpy = vi.spyOn(search, 'performSearch');
			
			search.quickSearch('test1');
			search.quickSearch('test2');
			search.quickSearch('test3');

			// Fast forward past the debounce delay
			vi.advanceTimersByTime(300);

			expect(performSearchSpy).toHaveBeenCalledTimes(1);
			expect(performSearchSpy).toHaveBeenCalledWith('test3');
		});

		it('should clear results for empty query', async () => {
			search.quickSearch('');
			vi.advanceTimersByTime(300);

			expect(search.value.searchResults).toEqual([]);
		});

		it('should use custom delay', async () => {
			const performSearchSpy = vi.spyOn(search, 'performSearch');
			
			search.quickSearch('test', 500);
			vi.advanceTimersByTime(300);
			expect(performSearchSpy).not.toHaveBeenCalled();
			
			vi.advanceTimersByTime(200);
			expect(performSearchSpy).toHaveBeenCalledWith('test');
		});
	});

	describe('Clear Operations', () => {
		it('should clear search', () => {
			search.setQuery('test');
			search.setFilters({ genre: 'Action' });
			search.value.searchResults = [{ id: '1', title: 'Test' }];
			search.value.searchError = 'Some error';

			search.clearSearch();

			expect(search.value.currentQuery).toBe('');
			expect(search.value.currentFilters).toEqual({});
			expect(search.value.searchResults).toEqual([]);
			expect(search.value.searchError).toBeNull();
		});

		it('should clear error', () => {
			search.value.searchError = 'Some error';
			search.clearError();
			expect(search.value.searchError).toBeNull();
		});

		it('should clear all filters', () => {
			search.setFilters({ genre: 'Action' });
			search.setQuickFilters('games', { developer: 'Test' });

			search.clearAllFilters();

			expect(search.value.currentFilters).toEqual({});
			expect(search.value.quickFilters.games).toEqual({});
			expect(search.value.quickFilters['user-games']).toEqual({});
		});
	});

	describe('Saved Searches', () => {
		it('should save current search', () => {
			search.setQuery('test query');
			search.setFilters({ genre: 'Action' });
			search.setSorting('rating', 'desc');
			search.setSearchType('games');

			const savedSearch = search.saveCurrentSearch('My Search');

			expect(savedSearch).toMatchObject({
				name: 'My Search',
				query: {
					q: 'test query',
					filters: { genre: 'Action' },
					sortBy: 'rating',
					sortOrder: 'desc'
				},
				searchType: 'games'
			});
			expect(search.value.savedSearches).toHaveLength(1);
			expect(search.value.savedSearches[0]).toMatchObject(savedSearch);
			expect(localStorageMock.setItem).toHaveBeenCalledWith(
				'saved-searches',
				JSON.stringify(search.value.savedSearches)
			);
		});

		it('should not save search with empty name', () => {
			const result = search.saveCurrentSearch('   ');
			expect(result).toBeUndefined();
			expect(search.value.savedSearches).toHaveLength(0);
		});

		it('should load saved search', async () => {
			const savedSearch = {
				id: 'search1',
				name: 'My Search',
				query: {
					q: 'test query',
					filters: { genre: 'Action' },
					sortBy: 'rating',
					sortOrder: 'desc'
				},
				searchType: 'games',
				created_at: '2023-01-01T00:00:00Z'
			};

			search.value.savedSearches = [savedSearch];
			const performSearchSpy = vi.spyOn(search, 'performSearch');

			search.loadSavedSearch('search1');

			expect(search.value.currentQuery).toBe('test query');
			expect(search.value.currentFilters).toEqual({ genre: 'Action' });
			expect(search.value.currentSortBy).toBe('rating');
			expect(search.value.currentSortOrder).toBe('desc');
			expect(search.value.searchType).toBe('games');
			expect(performSearchSpy).toHaveBeenCalled();
		});

		it('should handle loading non-existent saved search', () => {
			search.loadSavedSearch('nonexistent');
			expect(search.value.currentQuery).toBe('');
		});

		it('should delete saved search', () => {
			const savedSearch = {
				id: 'search1',
				name: 'My Search',
				query: { q: 'test', filters: {}, sortBy: 'title', sortOrder: 'asc' },
				searchType: 'games',
				created_at: '2023-01-01T00:00:00Z'
			};

			search.value.savedSearches = [savedSearch];
			search.deleteSavedSearch('search1');

			expect(search.value.savedSearches).toHaveLength(0);
			expect(localStorageMock.setItem).toHaveBeenCalledWith('saved-searches', '[]');
		});
	});

	describe('Search History', () => {
		it('should clear search history', () => {
			search.value.searchHistory = [
				{ id: '1', query: 'test', searchType: 'games', timestamp: '2023-01-01T00:00:00Z' }
			];

			search.clearSearchHistory();

			expect(search.value.searchHistory).toEqual([]);
			expect(localStorageMock.setItem).toHaveBeenCalledWith('search-history', '[]');
		});

		it('should remove specific history item', () => {
			const historyItem = { id: '1', query: 'test', searchType: 'games', timestamp: '2023-01-01T00:00:00Z' };
			search.value.searchHistory = [historyItem];

			search.removeFromHistory('1');

			expect(search.value.searchHistory).toEqual([]);
			expect(localStorageMock.setItem).toHaveBeenCalledWith('search-history', '[]');
		});
	});

	describe('Quick Filters', () => {
		it('should set quick filters', () => {
			const filters = { genre: 'Action', developer: 'Test' };
			search.setQuickFilters('games', filters);

			expect(search.value.quickFilters.games).toEqual(filters);
		});

		it('should apply quick filter', async () => {
			const performSearchSpy = vi.spyOn(search, 'performSearch');
			
			search.applyQuickFilter('games', 'genre', 'Action');

			expect(search.value.quickFilters.games).toEqual({ genre: 'Action' });
			expect(search.value.currentFilters).toEqual({ genre: 'Action' });
			expect(search.value.searchType).toBe('games');
			expect(performSearchSpy).toHaveBeenCalled();
		});

		it('should remove quick filter', async () => {
			const performSearchSpy = vi.spyOn(search, 'performSearch');
			
			search.setQuickFilters('games', { genre: 'Action', developer: 'Test' });
			search.removeQuickFilter('games', 'genre');

			expect(search.value.quickFilters.games).toEqual({ developer: 'Test' });
			expect(performSearchSpy).toHaveBeenCalled();
		});
	});

	describe('Predefined Quick Filters - User Games', () => {
		it('should filter by play status', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByPlayStatus(PlayStatus.COMPLETED);

			expect(applySpy).toHaveBeenCalledWith('user-games', 'play_status', PlayStatus.COMPLETED);
		});

		it('should filter by ownership status', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByOwnershipStatus(OwnershipStatus.OWNED);

			expect(applySpy).toHaveBeenCalledWith('user-games', 'ownership_status', OwnershipStatus.OWNED);
		});

		it('should filter by loved games', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByLovedGames();

			expect(applySpy).toHaveBeenCalledWith('user-games', 'is_loved', true);
		});

		it('should filter by platform', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByPlatform('platform-1');

			expect(applySpy).toHaveBeenCalledWith('user-games', 'platform_id', 'platform-1');
		});

		it('should filter by rating range', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByRating(4, 5);

			expect(applySpy).toHaveBeenCalledWith('user-games', 'rating_min', 4);
			expect(applySpy).toHaveBeenCalledWith('user-games', 'rating_max', 5);
		});

		it('should filter by minimum rating only', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByRating(3);

			expect(applySpy).toHaveBeenCalledWith('user-games', 'rating_min', 3);
			expect(applySpy).toHaveBeenCalledTimes(1);
		});
	});

	describe('Predefined Quick Filters - Games', () => {
		it('should filter by genre', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByGenre('Action');

			expect(applySpy).toHaveBeenCalledWith('games', 'genre', 'Action');
		});

		it('should filter by developer', async () => {
			const applySpy = vi.spyOn(search, 'applyQuickFilter').mockImplementation(() => Promise.resolve());
			
			search.filterByDeveloper('Test Developer');

			expect(applySpy).toHaveBeenCalledWith('games', 'developer', 'Test Developer');
		});

	});

	describe('Non-browser Environment', () => {
		it('should handle non-browser environment gracefully', async () => {
			// Mock browser as false
			vi.doMock('$app/environment', () => ({
				browser: false
			}));

			// Reimport to test initialization without browser
			vi.resetModules();
			const module = await import('./search.svelte');
			const nonBrowserSearch = module.search;

			// Should not crash and should have initial state
			expect(nonBrowserSearch.value.savedSearches).toEqual([]);
			expect(nonBrowserSearch.value.searchHistory).toEqual([]);
		});
	});
});