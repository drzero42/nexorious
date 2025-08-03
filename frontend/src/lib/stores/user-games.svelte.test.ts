import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock dependencies before importing
vi.mock('./auth.svelte', () => ({
	auth: {
		value: {
			accessToken: 'test-access-token',
			refreshToken: 'test-refresh-token',
			user: { id: '1', username: 'testuser', isAdmin: false }
		},
		refreshAuth: vi.fn(() => Promise.resolve(true))
	}
}));

vi.mock('$lib/env', () => ({
	config: {
		apiUrl: 'http://localhost:8000/api'
	}
}));

// Import after mocking
import { PlayStatus, OwnershipStatus } from './user-games.svelte';
import type { UserGame, UserGameFilters, CollectionStats } from './user-games.svelte';

describe('UserGames Store', () => {
	let userGamesStore: any;
	let mockFetch = vi.fn();

	// Mock user game data for testing
	const mockUserGame: UserGame = {
		id: '1',
		game: {
			id: '1',
			title: 'Test Game',
			description: 'A test game',
			genre: 'Action',
			developer: 'Test Dev',
			publisher: 'Test Pub',
			release_date: '2023-01-01',
			cover_art_url: 'https://example.com/cover.jpg',
			rating_count: 0,
			game_metadata: '{}',
			created_at: '2023-01-01T00:00:00Z',
			updated_at: '2023-01-01T00:00:00Z'
		},
		ownership_status: OwnershipStatus.OWNED,
		is_physical: false,
		personal_rating: 4.5,
		is_loved: true,
		play_status: PlayStatus.COMPLETED,
		hours_played: 25,
		personal_notes: 'Great game!',
		acquired_date: '2023-01-01',
		platforms: [],
		created_at: '2023-01-01T00:00:00Z',
		updated_at: '2023-01-01T00:00:00Z'
	};

	const mockCollectionStats: CollectionStats = {
		total_games: 10,
		by_status: {
			[PlayStatus.NOT_STARTED]: 3,
			[PlayStatus.IN_PROGRESS]: 2,
			[PlayStatus.COMPLETED]: 4,
			[PlayStatus.MASTERED]: 1,
			[PlayStatus.DOMINATED]: 0,
			[PlayStatus.SHELVED]: 0,
			[PlayStatus.DROPPED]: 0,
			[PlayStatus.REPLAY]: 0
		},
		by_platform: { 'PC': 5, 'PlayStation 5': 3, 'Nintendo Switch': 2 },
		by_rating: { '5': 2, '4': 3, '3': 2, '2': 1, '1': 0 },
		pile_of_shame: 3,
		completion_rate: 50.0,
		average_rating: 4.2,
		total_hours_played: 150
	};

	beforeEach(async () => {
		// Reset all mocks
		vi.clearAllMocks();
		global.fetch = mockFetch;
		
		// Get the mocked auth module
		const { auth } = await import('./auth.svelte');
		
		// Reset auth mock values
		auth.value.accessToken = 'test-access-token'; 
		(auth.refreshAuth as any).mockReset().mockResolvedValue(true);
		
		// Dynamic import to get fresh store instance
		const module = await import('./user-games.svelte');
		userGamesStore = module.userGames;
		
		// Clear store state
		userGamesStore.clearFilters();
		userGamesStore.clearCurrentUserGame();
		userGamesStore.clearError();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('Store Structure', () => {
		it('should have correct initial state', () => {
			const state = userGamesStore.value;
			
			expect(state).toMatchObject({
				userGames: [],
				currentUserGame: null,
				stats: null,
				isLoading: false,
				error: null,
				filters: {},
				pagination: {
					page: 1,
					per_page: 20,
					total: 0,
					pages: 0
				}
			});
		});

		it('should have all required methods', () => {
			const requiredMethods = [
				'loadUserGames',
				'getUserGame',
				'addGameToCollection',
				'updateUserGame',
				'updateProgress',
				'removeFromCollection',
				'addPlatformToUserGame',
				'removePlatformFromUserGame',
				'bulkUpdateStatus',
				'getCollectionStats',
				'getGamesByStatus',
				'getLovedGames',
				'getGamesByRating',
				'getPileOfShame',
				'clearCurrentUserGame',
				'clearFilters',
				'clearError',
				'fetchUserGames'
			];

			requiredMethods.forEach(method => {
				expect(typeof userGamesStore[method]).toBe('function');
			});
		});
	});

	describe('Authentication Handling', () => {
		it('should throw error when not authenticated', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.accessToken = null;

			await expect(userGamesStore.loadUserGames()).rejects.toThrow('Not authenticated');
		});

		it('should retry request after token refresh on 401', async () => {
			const { auth } = await import('./auth.svelte');
			
			mockFetch
				.mockResolvedValueOnce({
					ok: false,
					status: 401,
					statusText: 'Unauthorized'
				})
				.mockResolvedValueOnce({
					ok: true,
					json: () => Promise.resolve({
						user_games: [mockUserGame],
						total: 1,
						page: 1,
						per_page: 20,
						pages: 1
					})
				});

			(auth.refreshAuth as any).mockResolvedValueOnce(true);

			await userGamesStore.loadUserGames();

			expect(auth.refreshAuth).toHaveBeenCalled();
			expect(mockFetch).toHaveBeenCalledTimes(2);
		});
	});

	describe('Load User Games', () => {
		it('should load user games successfully', async () => {
			const mockResponse = {
				user_games: [mockUserGame],
				total: 1,
				page: 1,
				per_page: 20,
				pages: 1
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockResponse)
			});

			await userGamesStore.loadUserGames();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/?page=1&per_page=20',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			const state = userGamesStore.value;
			expect(state.userGames).toEqual([mockUserGame]);
			expect(state.pagination).toEqual({
				page: 1,
				per_page: 20,
				total: 1,
				pages: 1
			});
			expect(state.isLoading).toBe(false);
		});

		it('should apply filters when loading user games', async () => {
			const filters: UserGameFilters = {
				play_status: PlayStatus.COMPLETED,
				is_loved: true,
				q: 'test'
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({
					user_games: [],
					total: 0,
					page: 1,
					per_page: 20,
					pages: 0
				})
			});

			await userGamesStore.loadUserGames(filters);

			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('play_status=completed'),
				expect.any(Object)
			);
			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('is_loved=true'),
				expect.any(Object)
			);
			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('q=test'),
				expect.any(Object)
			);
		});

		it('should handle load user games error', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 500,
				statusText: 'Internal Server Error',
				json: () => Promise.resolve({ detail: 'Server error' })
			});

			await expect(userGamesStore.loadUserGames()).rejects.toThrow('Server error');

			const state = userGamesStore.value;
			expect(state.isLoading).toBe(false);
			expect(state.error).toBe('Server error');
		});
	});

	describe('Get User Game', () => {
		it('should get a specific user game', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockUserGame)
			});

			const result = await userGamesStore.getUserGame('1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual(mockUserGame);
			expect(userGamesStore.value.currentUserGame).toEqual(mockUserGame);
		});
	});

	describe('Add Game to Collection', () => {
		it('should add a game to collection', async () => {
			const gameData = {
				game_id: '1',
				ownership_status: OwnershipStatus.OWNED,
				platforms: ['platform-1']
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockUserGame)
			});

			const result = await userGamesStore.addGameToCollection(gameData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(gameData)
				})
			);

			expect(result).toEqual(mockUserGame);
			expect(userGamesStore.value.userGames).toHaveLength(1);
			expect(userGamesStore.value.userGames[0]).toEqual(mockUserGame);
		});
	});

	describe('Update User Game', () => {
		it('should update user game', async () => {
			// Set initial state with the game
			userGamesStore.value.userGames = [mockUserGame];

			const updateData = {
				personal_rating: 5,
				is_loved: false
			};

			const updatedGame = { ...mockUserGame, ...updateData };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedGame)
			});

			const result = await userGamesStore.updateUserGame('1', updateData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1',
				expect.objectContaining({
					method: 'PUT',
					body: JSON.stringify(updateData)
				})
			);

			expect(result).toEqual(updatedGame);
			expect(userGamesStore.value.userGames[0]).toEqual(updatedGame);
		});
	});

	describe('Update Progress', () => {
		it('should update game progress', async () => {
			const progressData = {
				play_status: PlayStatus.MASTERED,
				hours_played: 50,
				personal_notes: 'Finally mastered it!'
			};

			const updatedGame = { ...mockUserGame, ...progressData };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedGame)
			});

			const result = await userGamesStore.updateProgress('1', progressData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1/progress',
				expect.objectContaining({
					method: 'PUT',
					body: JSON.stringify(progressData)
				})
			);

			expect(result).toEqual(updatedGame);
		});
	});

	describe('Remove from Collection', () => {
		it('should remove game from collection', async () => {
			// Set initial state with the game
			userGamesStore.value.userGames = [mockUserGame];
			userGamesStore.value.currentUserGame = mockUserGame;

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await userGamesStore.removeFromCollection('1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			expect(userGamesStore.value.userGames).toEqual([]);
			expect(userGamesStore.value.currentUserGame).toBeNull();
		});
	});

	describe('Collection Stats', () => {
		it('should get collection statistics', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockCollectionStats)
			});

			const result = await userGamesStore.getCollectionStats();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/stats',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual(mockCollectionStats);
			expect(userGamesStore.value.stats).toEqual(mockCollectionStats);
		});
	});

	describe('Filter and Query Methods', () => {
		beforeEach(() => {
			// Set up test data
			const testGames: UserGame[] = [
				{ ...mockUserGame, id: '1', play_status: PlayStatus.COMPLETED, is_loved: true, personal_rating: 5 },
				{ ...mockUserGame, id: '2', play_status: PlayStatus.NOT_STARTED, is_loved: false, personal_rating: 3 },
				{ ...mockUserGame, id: '3', play_status: PlayStatus.IN_PROGRESS, is_loved: true, personal_rating: 4 },
				{ ...mockUserGame, id: '4', play_status: PlayStatus.NOT_STARTED, is_loved: false, personal_rating: 5 }
			];
			
			userGamesStore.value.userGames = testGames;
		});

		it('should get games by status', () => {
			const completedGames = userGamesStore.getGamesByStatus(PlayStatus.COMPLETED);
			const notStartedGames = userGamesStore.getGamesByStatus(PlayStatus.NOT_STARTED);

			expect(completedGames).toHaveLength(1);
			expect(completedGames[0].id).toBe('1');
			expect(notStartedGames).toHaveLength(2);
		});

		it('should get loved games', () => {
			const lovedGames = userGamesStore.getLovedGames();

			expect(lovedGames).toHaveLength(2);
			expect(lovedGames.map((g: UserGame) => g.id)).toEqual(['1', '3']);
		});

		it('should get games by rating', () => {
			const fiveStarGames = userGamesStore.getGamesByRating(5);
			const threeStarGames = userGamesStore.getGamesByRating(3);

			expect(fiveStarGames).toHaveLength(2);
			expect(threeStarGames).toHaveLength(1);
		});

		it('should get pile of shame', () => {
			const pileOfShame = userGamesStore.getPileOfShame();

			expect(pileOfShame).toHaveLength(2);
			expect(pileOfShame.every((g: UserGame) => g.play_status === PlayStatus.NOT_STARTED)).toBe(true);
		});
	});

	describe('Bulk Operations', () => {
		it('should perform bulk status update', async () => {
			const bulkData = {
				user_game_ids: ['1', '2'],
				play_status: PlayStatus.COMPLETED,
				is_loved: true
			};

			const successResponse = {
				success: true,
				message: 'Bulk update completed successfully',
				updated_count: 2,
				failed_count: 0
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(successResponse)
			});

			// Set initial state
			userGamesStore.value.userGames = [
				{ ...mockUserGame, id: '1' },
				{ ...mockUserGame, id: '2' },
				{ ...mockUserGame, id: '3' }
			];

			const result = await userGamesStore.bulkUpdateStatus(bulkData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/bulk-update',
				expect.objectContaining({
					method: 'PUT',
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token',
						'Content-Type': 'application/json'
					}),
					body: JSON.stringify(bulkData)
				})
			);

			expect(result).toEqual(successResponse);
		});
	});

	describe('Platform Management', () => {
		it('should add platform to user game', async () => {
			const platformData = {
				platform_id: 'platform-1',
				storefront_id: 'storefront-1',
				store_game_id: 'store-123'
			};

			const updatedGame = { ...mockUserGame };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedGame)
			});

			const result = await userGamesStore.addPlatformToUserGame('1', platformData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1/platforms',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(platformData)
				})
			);

			expect(result).toEqual(updatedGame);
		});

		it('should remove platform from user game', async () => {
			const testGame = {
				...mockUserGame,
				platforms: [
					{ id: 'platform-1', platform: { id: '1', name: 'PC' }, is_available: true, created_at: '2023-01-01T00:00:00Z' }
				]
			};

			userGamesStore.value.userGames = [testGame];
			userGamesStore.value.currentUserGame = testGame;

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await userGamesStore.removePlatformFromUserGame('1', 'platform-1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1/platforms/platform-1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			expect(userGamesStore.value.userGames[0].platforms).toEqual([]);
		});
	});

	describe('State Management', () => {
		it('should clear current user game', () => {
			userGamesStore.value.currentUserGame = mockUserGame;

			userGamesStore.clearCurrentUserGame();

			expect(userGamesStore.value.currentUserGame).toBeNull();
		});

		it('should clear filters', () => {
			userGamesStore.value.filters = { play_status: PlayStatus.COMPLETED };

			userGamesStore.clearFilters();

			expect(userGamesStore.value.filters).toEqual({});
		});

		it('should clear error', () => {
			userGamesStore.value.error = 'Some error';

			userGamesStore.clearError();

			expect(userGamesStore.value.error).toBeNull();
		});
	});

	describe('Loading States', () => {
		it('should set loading state during API calls', async () => {
			let resolvePromise: (value: any) => void;
			const loadingPromise = new Promise(resolve => { resolvePromise = resolve; });

			mockFetch.mockReturnValueOnce(loadingPromise);

			const loadPromise = userGamesStore.loadUserGames();

			expect(userGamesStore.value.isLoading).toBe(true);

			resolvePromise!({
				ok: true,
				json: () => Promise.resolve({
					user_games: [],
					total: 0,
					page: 1,
					per_page: 20,
					pages: 0
				})
			});

			await loadPromise;

			expect(userGamesStore.value.isLoading).toBe(false);
		});
	});

	describe('Error Handling', () => {
		it('should handle API errors gracefully', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 400,
				statusText: 'Bad Request',
				json: () => Promise.resolve({ detail: 'Invalid data' })
			});

			await expect(userGamesStore.addGameToCollection({ game_id: 'invalid' })).rejects.toThrow('Invalid data');

			expect(userGamesStore.value.error).toBe('Invalid data');
			expect(userGamesStore.value.isLoading).toBe(false);
		});

		it('should handle network errors', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(userGamesStore.loadUserGames()).rejects.toThrow('Network error');

			expect(userGamesStore.value.error).toBe('Network error');
		});
	});

	describe('Backward Compatibility', () => {
		it('should have fetchUserGames alias', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({
					user_games: [],
					total: 0,
					page: 1,
					per_page: 20,
					pages: 0
				})
			});

			await userGamesStore.fetchUserGames();

			expect(mockFetch).toHaveBeenCalled();
		});
	});
});