import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { toGameId } from '$lib/types/game';
import type { UserGameId } from '$lib/types/game';

// Mock dependencies before importing - use vi.hoisted to ensure mock is available before import
const mockAuth = vi.hoisted(() => ({
	auth: {
		value: {
			accessToken: 'test-access-token' as string | null,
			refreshToken: 'test-refresh-token' as string | null,
			user: { id: '1', username: 'testuser', isAdmin: false } as { id: string; username: string; isAdmin: boolean } | null
		},
		refreshAuth: vi.fn(() => Promise.resolve(true))
	}
}));

vi.mock('./auth.svelte', () => mockAuth);

vi.mock('$lib/env', () => ({
	config: {
		apiUrl: 'http://localhost:8000/api'
	}
}));

// Import after mocking
import { userGames, PlayStatus, OwnershipStatus } from './user-games.svelte';
import type { UserGame, UserGameFilters, CollectionStats } from './user-games.svelte';

// Helper to create UserGameId for tests (bypasses UUID validation for test simplicity)
const testUserGameId = (id: string): UserGameId => id as unknown as UserGameId;

describe('UserGames Store', () => {
	const mockFetch = vi.fn();

	// Mock user game data for testing
	const mockUserGame: UserGame = {
		id: testUserGameId('1'),
		game: {
			id: toGameId(1),
			title: 'Test Game',
			description: 'A test game',
			genre: 'Action',
			developer: 'Test Dev',
			publisher: 'Test Pub',
			release_date: '2023-01-01',
			cover_art_url: 'https://example.com/cover.jpg',
			rating_count: 0,
			game_metadata: '{}',
			created_at: '2023-01-01T00:00:00.000Z',
			updated_at: '2023-01-01T00:00:00.000Z'
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
		created_at: '2023-01-01T00:00:00.000Z',
		updated_at: '2023-01-01T00:00:00.000Z'
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

	beforeEach(() => {
		// Reset all mocks
		vi.clearAllMocks();
		global.fetch = mockFetch;

		// Reset auth mock to default authenticated state
		mockAuth.auth.value.accessToken = 'test-access-token';
		mockAuth.auth.value.refreshToken = 'test-refresh-token';
		mockAuth.auth.value.user = { id: '1', username: 'testuser', isAdmin: false };
		mockAuth.auth.refreshAuth.mockReset().mockResolvedValue(true);

		// Reset store to initial state
		userGames.reset();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('Store Structure', () => {
		it('should have correct initial state', () => {
			const state = userGames.value;
			
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
			] as const;

			requiredMethods.forEach(method => {
				expect(typeof userGames[method]).toBe('function');
			});
		});
	});

	describe('Authentication Handling', () => {
		it('should throw error when not authenticated', async () => {
			// Set auth mock to unauthenticated state
			mockAuth.auth.value.accessToken = null;

			await expect(userGames.loadUserGames()).rejects.toThrow('Not authenticated');
		});

		it('should retry request after token refresh on 401', async () => {
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

			mockAuth.auth.refreshAuth.mockResolvedValueOnce(true);

			await userGames.loadUserGames();

			expect(mockAuth.auth.refreshAuth).toHaveBeenCalled();
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

			await userGames.loadUserGames();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/?page=1&per_page=20',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			const state = userGames.value;
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

			await userGames.loadUserGames(filters);

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
			const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 500,
				statusText: 'Internal Server Error',
				json: () => Promise.resolve({ detail: 'Server error' })
			});

			await expect(userGames.loadUserGames()).rejects.toThrow('Server error');

			const state = userGames.value;
			expect(state.isLoading).toBe(false);
			expect(state.error).toBe('Server error');

			// Verify the expected error was logged
			expect(consoleSpy).toHaveBeenCalledWith(
				expect.stringContaining('API call failed'),
				expect.objectContaining({ status: 500, errorMessage: 'Server error' })
			);
			consoleSpy.mockRestore();
		});
	});

	describe('Get User Game', () => {
		it('should get a specific user game', async () => {
			// getUserGame calls loadUserGames first, so this response comes first
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({
					user_games: [mockUserGame],
					total: 1,
					page: 1,
					per_page: 20,
					pages: 1
				})
			});

			const result = await userGames.getUserGame(toGameId(1));

			// Should have called the list endpoint to load games
			expect(mockFetch).toHaveBeenCalledWith(
				expect.stringContaining('/user-games'),
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual(mockUserGame);
			expect(userGames.value.currentUserGame).toEqual(mockUserGame);
		});
	});

	describe('Add Game to Collection', () => {
		it('should add a game to collection', async () => {
			const gameData = {
				game_id: toGameId(1),
				ownership_status: OwnershipStatus.OWNED,
				platforms: [{ platform_id: 'platform-1', is_available: true }]
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockUserGame)
			});

			const result = await userGames.addGameToCollection(gameData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(gameData)
				})
			);

			expect(result).toEqual(mockUserGame);
			expect(userGames.value.userGames).toHaveLength(1);
			expect(userGames.value.userGames[0]).toEqual(mockUserGame);
		});
	});

	describe('Update User Game', () => {
		it('should update user game', async () => {
			// Set initial state with the game
			userGames.__testSetData([mockUserGame]);

			const updateData = {
				personal_rating: 5,
				is_loved: false
			};

			const updatedGame = { ...mockUserGame, ...updateData };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedGame)
			});

			const result = await userGames.updateUserGame(testUserGameId('1'), updateData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1',
				expect.objectContaining({
					method: 'PUT',
					body: JSON.stringify(updateData)
				})
			);

			expect(result).toEqual(updatedGame);
			expect(userGames.value.userGames[0]).toEqual(updatedGame);
		});
	});

	describe('Update Progress', () => {
		it('should update game progress', async () => {
			// Set initial state with the game
			userGames.__testSetData([mockUserGame]);

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

			const result = await userGames.updateProgress(testUserGameId('1'), progressData);

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
			userGames.__testSetData([mockUserGame]);
			userGames.value.currentUserGame = mockUserGame;

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await userGames.removeFromCollection(testUserGameId('1'));

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			expect(userGames.value.userGames).toEqual([]);
			expect(userGames.value.currentUserGame).toBeNull();
		});
	});

	describe('Collection Stats', () => {
		it('should get collection statistics', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockCollectionStats)
			});

			const result = await userGames.getCollectionStats();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/stats',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual(mockCollectionStats);
			expect(userGames.value.stats).toEqual(mockCollectionStats);
		});
	});

	describe('Filter and Query Methods', () => {
		beforeEach(() => {
			// Set up test data
			const testGames: UserGame[] = [
				{ ...mockUserGame, id: testUserGameId('1'), play_status: PlayStatus.COMPLETED, is_loved: true, personal_rating: 5 },
				{ ...mockUserGame, id: testUserGameId('2'), play_status: PlayStatus.NOT_STARTED, is_loved: false, personal_rating: 3 },
				{ ...mockUserGame, id: testUserGameId('3'), play_status: PlayStatus.IN_PROGRESS, is_loved: true, personal_rating: 4 },
				{ ...mockUserGame, id: testUserGameId('4'), play_status: PlayStatus.NOT_STARTED, is_loved: false, personal_rating: 5 }
			];

			userGames.__testSetData(testGames);
		});

		it('should get games by status', () => {
			const completedGames = userGames.getGamesByStatus(PlayStatus.COMPLETED);
			const notStartedGames = userGames.getGamesByStatus(PlayStatus.NOT_STARTED);

			expect(completedGames).toHaveLength(1);
			expect(completedGames[0]?.id).toBe(testUserGameId('1'));
			expect(notStartedGames).toHaveLength(2);
		});

		it('should get loved games', () => {
			const lovedGames = userGames.getLovedGames();

			expect(lovedGames).toHaveLength(2);
			expect(lovedGames.map((g: UserGame) => g.id)).toEqual([testUserGameId('1'), testUserGameId('3')]);
		});

		it('should get games by rating', () => {
			const fiveStarGames = userGames.getGamesByRating(5);
			const threeStarGames = userGames.getGamesByRating(3);

			expect(fiveStarGames).toHaveLength(2);
			expect(threeStarGames).toHaveLength(1);
		});

		it('should get pile of shame', () => {
			const pileOfShame = userGames.getPileOfShame();

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
			userGames.__testSetData([
				{ ...mockUserGame, id: testUserGameId('1') },
				{ ...mockUserGame, id: testUserGameId('2') },
				{ ...mockUserGame, id: testUserGameId('3') }
			]);

			const result = await userGames.bulkUpdateStatus(bulkData);

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
			// Set initial state with the game
			userGames.__testSetData([mockUserGame]);

			const platformData = {
				platform_id: 'platform-1',
				storefront_id: 'storefront-1',
				store_game_id: 'store-123',
				is_available: true
			};

			const updatedGame = { ...mockUserGame };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedGame)
			});

			const result = await userGames.addPlatformToUserGame(testUserGameId('1'), platformData);

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
			const testGame: UserGame = {
				...mockUserGame,
				platforms: [
					{
						id: 'platform-1',
						platform: {
							id: '1',
							name: 'pc-windows',
							display_name: 'PC',
							is_active: true,
							source: 'seed',
							created_at: '2023-01-01T00:00:00.000Z',
							updated_at: '2023-01-01T00:00:00.000Z'
						},
						is_available: true,
						created_at: '2023-01-01T00:00:00.000Z'
					}
				]
			};

			userGames.__testSetData([testGame]);

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await userGames.removePlatformFromUserGame(testUserGameId('1'), 'platform-1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/user-games/1/platforms/platform-1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			expect(userGames.value.userGames[0]?.platforms).toEqual([]);
		});
	});

	describe('State Management', () => {
		it('should clear current user game', () => {
			userGames.value.currentUserGame = mockUserGame;

			userGames.clearCurrentUserGame();

			expect(userGames.value.currentUserGame).toBeNull();
		});

		it('should clear filters', () => {
			userGames.value.filters = { play_status: PlayStatus.COMPLETED };

			userGames.clearFilters();

			expect(userGames.value.filters).toEqual({});
		});

		it('should clear error', () => {
			userGames.value.error = 'Some error';

			userGames.clearError();

			expect(userGames.value.error).toBeNull();
		});
	});

	describe('Loading States', () => {
		it('should set loading state during API calls', async () => {
			let resolvePromise: (value: any) => void;
			const loadingPromise = new Promise(resolve => { resolvePromise = resolve; });

			mockFetch.mockReturnValueOnce(loadingPromise);

			const loadPromise = userGames.loadUserGames();

			expect(userGames.value.isLoading).toBe(true);

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

			expect(userGames.value.isLoading).toBe(false);
		});
	});

	describe('Error Handling', () => {
		it('should handle API errors gracefully', async () => {
			const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 400,
				statusText: 'Bad Request',
				json: () => Promise.resolve({ detail: 'Invalid data' })
			});

			await expect(userGames.addGameToCollection({ game_id: toGameId(999999) })).rejects.toThrow('Invalid data');

			expect(userGames.value.error).toBe('Invalid data');
			expect(userGames.value.isLoading).toBe(false);

			// Verify the expected error was logged
			expect(consoleSpy).toHaveBeenCalledWith(
				expect.stringContaining('API call failed'),
				expect.objectContaining({ status: 400, errorMessage: 'Invalid data' })
			);
			consoleSpy.mockRestore();
		});

		it('should handle network errors', async () => {
			// Network errors don't reach the API error logging code path
			// since fetch itself throws before we get a response
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(userGames.loadUserGames()).rejects.toThrow('Network error');

			expect(userGames.value.error).toBe('Network error');
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

			await userGames.fetchUserGames();

			expect(mockFetch).toHaveBeenCalled();
		});
	});
});