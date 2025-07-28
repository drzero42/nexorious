import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';

// Mock dependencies before importing
vi.mock('./auth.svelte', () => ({
	auth: {
		value: {
			accessToken: 'test-access-token',
			refreshToken: 'test-refresh-token',
			user: { id: '1', username: 'admin', isAdmin: true }
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
import { platforms } from './platforms.svelte';
import type { Platform, Storefront, PlatformCreateRequest, PlatformUpdateRequest, StorefrontCreateRequest, StorefrontUpdateRequest } from './platforms.svelte';

describe('Platforms Store', () => {
	let mockFetch = vi.fn();

	// Mock platform data for testing
	const mockPlatform: Platform = {
		id: '1',
		name: 'test_platform',
		display_name: 'Test Platform',
		icon_url: 'https://example.com/icon.png',
		is_active: true,
		source: 'user',
		version_added: '1.0.0',
		created_at: '2023-01-01T00:00:00Z',
		updated_at: '2023-01-01T00:00:00Z'
	};

	const mockStorefront: Storefront = {
		id: '1',
		name: 'test_storefront',
		display_name: 'Test Storefront',
		icon_url: 'https://example.com/storefront-icon.png',
		base_url: 'https://store.example.com',
		is_active: true,
		source: 'user',
		version_added: '1.0.0',
		created_at: '2023-01-01T00:00:00Z',
		updated_at: '2023-01-01T00:00:00Z'
	};

	beforeEach(async () => {
		vi.clearAllMocks();
		global.fetch = mockFetch;
		
		// Get the mocked auth module
		const { auth } = await import('./auth.svelte');
		
		// Reset auth mock values
		auth.value.accessToken = 'test-access-token';
		auth.value.user = { id: '1', username: 'admin', isAdmin: true };
		(auth.refreshAuth as any).mockReset().mockResolvedValue(true);
		
		// Reset store state
		platforms.__reset();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	describe('Store Structure', () => {
		it('should have correct initial state', () => {
			const state = get(platforms);
			
			expect(state).toMatchObject({
				platforms: [],
				storefronts: [],
				isLoading: false,
				error: null
			});
		});

		it('should have all required methods', () => {
			const requiredMethods = [
				'fetchPlatforms',
				'fetchStorefronts',
				'fetchAll',
				'createPlatform',
				'updatePlatform',
				'deletePlatform',
				'createStorefront',
				'updateStorefront',
				'deleteStorefront',
				'getActivePlatforms',
				'getActiveStorefronts',
				'clearError',
				'__reset'
			];

			requiredMethods.forEach(method => {
				expect(typeof (platforms as any)[method]).toBe('function');
			});
		});
	});

	describe('Authentication Handling', () => {
		it('should throw error when not authenticated for fetchPlatforms', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.accessToken = null;

			await expect(platforms.fetchPlatforms()).rejects.toThrow('Not authenticated');
		});

		it('should throw error when user is not admin for fetchPlatforms', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.user = { id: '1', username: 'user', isAdmin: false };

			await expect(platforms.fetchPlatforms()).rejects.toThrow('Admin access required');
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
						platforms: [mockPlatform]
					})
				});

			(auth.refreshAuth as any).mockResolvedValueOnce(true);

			await platforms.fetchPlatforms();

			expect(auth.refreshAuth).toHaveBeenCalled();
			expect(mockFetch).toHaveBeenCalledTimes(2);
		});
	});

	describe('Fetch Platforms', () => {
		it('should fetch platforms successfully', async () => {
			const mockResponse = {
				platforms: [mockPlatform]
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockResponse)
			});

			const result = await platforms.fetchPlatforms();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/?active_only=false',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual([mockPlatform]);
			
			const state = get(platforms);
			expect(state.platforms).toEqual([mockPlatform]);
			expect(state.isLoading).toBe(false);
		});

		it('should handle fetch platforms error', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 500,
				statusText: 'Internal Server Error'
			});

			await expect(platforms.fetchPlatforms()).rejects.toThrow('HTTP 500: Internal Server Error');

			const state = get(platforms);
			expect(state.isLoading).toBe(false);
			expect(state.error).toBe('HTTP 500: Internal Server Error');
		});
	});

	describe('Fetch Storefronts', () => {
		it('should fetch storefronts successfully', async () => {
			const mockResponse = {
				storefronts: [mockStorefront]
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockResponse)
			});

			const result = await platforms.fetchStorefronts();

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/?active_only=false',
				expect.objectContaining({
					headers: expect.objectContaining({
						'Authorization': 'Bearer test-access-token'
					})
				})
			);

			expect(result).toEqual([mockStorefront]);
			
			const state = get(platforms);
			expect(state.storefronts).toEqual([mockStorefront]);
			expect(state.isLoading).toBe(false);
		});

		it('should require admin access for fetchStorefronts', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.user = { id: '1', username: 'user', isAdmin: false };

			await expect(platforms.fetchStorefronts()).rejects.toThrow('Admin access required');
		});
	});

	describe('Fetch All', () => {
		it('should fetch both platforms and storefronts', async () => {
			const platformsResponse = { platforms: [mockPlatform] };
			const storefrontsResponse = { storefronts: [mockStorefront] };

			mockFetch
				.mockResolvedValueOnce({
					ok: true,
					json: () => Promise.resolve(platformsResponse)
				})
				.mockResolvedValueOnce({
					ok: true,
					json: () => Promise.resolve(storefrontsResponse)
				});

			const result = await platforms.fetchAll();

			expect(mockFetch).toHaveBeenCalledTimes(2);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/?active_only=false',
				expect.any(Object)
			);
			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/?active_only=false',
				expect.any(Object)
			);

			expect(result).toEqual({
				platforms: [mockPlatform],
				storefronts: [mockStorefront]
			});

			const state = get(platforms);
			expect(state.platforms).toEqual([mockPlatform]);
			expect(state.storefronts).toEqual([mockStorefront]);
		});

		it('should handle fetchAll error', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(platforms.fetchAll()).rejects.toThrow('Network error');

			const state = get(platforms);
			expect(state.error).toBe('Network error');
		});
	});

	describe('Create Platform', () => {
		it('should create platform successfully', async () => {
			const platformData: PlatformCreateRequest = {
				name: 'new_platform',
				display_name: 'New Platform',
				icon_url: 'https://example.com/new-icon.png',
				is_active: true
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockPlatform)
			});

			const result = await platforms.createPlatform(platformData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(platformData)
				})
			);

			expect(result).toEqual(mockPlatform);
			
			const state = get(platforms);
			expect(state.platforms).toContain(mockPlatform);
		});

		it('should clean icon_url by trimming and setting to undefined if empty', async () => {
			const platformData: PlatformCreateRequest = {
				name: 'new_platform',
				display_name: 'New Platform',
				icon_url: '   ',
				is_active: true
			};

			const expectedCleanedData = {
				name: 'new_platform',
				display_name: 'New Platform',
				icon_url: undefined,
				is_active: true
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockPlatform)
			});

			await platforms.createPlatform(platformData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/',
				expect.objectContaining({
					body: JSON.stringify(expectedCleanedData)
				})
			);
		});

		it('should require admin access for createPlatform', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.user = { id: '1', username: 'user', isAdmin: false };

			const platformData: PlatformCreateRequest = {
				name: 'new_platform',
				display_name: 'New Platform'
			};

			await expect(platforms.createPlatform(platformData)).rejects.toThrow('Admin access required');
		});
	});

	describe('Update Platform', () => {
		it('should update platform successfully', async () => {
			// Set initial state with the platform
			platforms.__reset();
			const initialState = get(platforms);
			initialState.platforms = [mockPlatform];

			const updateData: PlatformUpdateRequest = {
				display_name: 'Updated Platform',
				is_active: false
			};

			const updatedPlatform = { ...mockPlatform, ...updateData };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedPlatform)
			});

			const result = await platforms.updatePlatform('1', updateData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/1',
				expect.objectContaining({
					method: 'PUT',
					body: JSON.stringify(updateData)
				})
			);

			expect(result).toEqual(updatedPlatform);
		});

		it('should require admin access for updatePlatform', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.user = { id: '1', username: 'user', isAdmin: false };

			await expect(platforms.updatePlatform('1', {})).rejects.toThrow('Admin access required');
		});
	});

	describe('Delete Platform', () => {
		it('should delete platform successfully', async () => {
			// Set initial state with the platform
			platforms.__reset();
			const initialState = get(platforms);
			initialState.platforms = [mockPlatform];

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await platforms.deletePlatform('1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			const state = get(platforms);
			expect(state.platforms).toEqual([]);
		});

		it('should require admin access for deletePlatform', async () => {
			const { auth } = await import('./auth.svelte');
			auth.value.user = { id: '1', username: 'user', isAdmin: false };

			await expect(platforms.deletePlatform('1')).rejects.toThrow('Admin access required');
		});
	});

	describe('Create Storefront', () => {
		it('should create storefront successfully', async () => {
			const storefrontData: StorefrontCreateRequest = {
				name: 'new_storefront',
				display_name: 'New Storefront',
				icon_url: 'https://example.com/new-storefront-icon.png',
				base_url: 'https://newstore.example.com',
				is_active: true
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockStorefront)
			});

			const result = await platforms.createStorefront(storefrontData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/',
				expect.objectContaining({
					method: 'POST',
					body: JSON.stringify(storefrontData)
				})
			);

			expect(result).toEqual(mockStorefront);
			
			const state = get(platforms);
			expect(state.storefronts).toContain(mockStorefront);
		});

		it('should clean URL fields by trimming and setting to undefined if empty', async () => {
			const storefrontData: StorefrontCreateRequest = {
				name: 'new_storefront',
				display_name: 'New Storefront',
				icon_url: '   ',
				base_url: '\t\n',
				is_active: true
			};

			const expectedCleanedData = {
				name: 'new_storefront',
				display_name: 'New Storefront',
				icon_url: undefined,
				base_url: undefined,
				is_active: true
			};

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(mockStorefront)
			});

			await platforms.createStorefront(storefrontData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/',
				expect.objectContaining({
					body: JSON.stringify(expectedCleanedData)
				})
			);
		});
	});

	describe('Update Storefront', () => {
		it('should update storefront successfully', async () => {
			const updateData: StorefrontUpdateRequest = {
				display_name: 'Updated Storefront',
				base_url: 'https://updated.example.com'
			};

			const updatedStorefront = { ...mockStorefront, ...updateData };

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve(updatedStorefront)
			});

			const result = await platforms.updateStorefront('1', updateData);

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/1',
				expect.objectContaining({
					method: 'PUT',
					body: JSON.stringify(updateData)
				})
			);

			expect(result).toEqual(updatedStorefront);
		});
	});

	describe('Delete Storefront', () => {
		it('should delete storefront successfully', async () => {
			// Set initial state with the storefront
			platforms.__reset();
			const initialState = get(platforms);
			initialState.storefronts = [mockStorefront];

			mockFetch.mockResolvedValueOnce({
				ok: true
			});

			await platforms.deleteStorefront('1');

			expect(mockFetch).toHaveBeenCalledWith(
				'http://localhost:8000/api/platforms/storefronts/1',
				expect.objectContaining({
					method: 'DELETE'
				})
			);

			const state = get(platforms);
			expect(state.storefronts).toEqual([]);
		});
	});

	describe('Client-side Filtering Methods', () => {
		it('should throw error for getActivePlatforms method', () => {
			expect(() => platforms.getActivePlatforms()).toThrow('Use store subscription with client-side filtering instead');
		});

		it('should throw error for getActiveStorefronts method', () => {
			expect(() => platforms.getActiveStorefronts()).toThrow('Use store subscription with client-side filtering instead');
		});
	});

	describe('State Management', () => {
		it('should clear error', () => {
			// Set error state
			platforms.__reset();
			const state = get(platforms);
			state.error = 'Some error';

			platforms.clearError();

			const clearedState = get(platforms);
			expect(clearedState.error).toBeNull();
		});

		it('should have __reset method for testing', () => {
			// Verify the __reset method exists and is a function
			expect(typeof (platforms as any).__reset).toBe('function');
			
			// Call reset to ensure it doesn't throw
			expect(() => platforms.__reset()).not.toThrow();
		});
	});

	describe('Loading States', () => {
		it('should set loading state during API calls', async () => {
			let resolvePromise: (value: any) => void;
			const loadingPromise = new Promise(resolve => { resolvePromise = resolve; });

			mockFetch.mockReturnValueOnce(loadingPromise);

			const fetchPromise = platforms.fetchPlatforms();

			// Check loading state is true during the request
			const loadingState = get(platforms);
			expect(loadingState.isLoading).toBe(true);

			resolvePromise!({
				ok: true,
				json: () => Promise.resolve({ platforms: [] })
			});

			await fetchPromise;

			// Check loading state is false after completion
			const completedState = get(platforms);
			expect(completedState.isLoading).toBe(false);
		});
	});

	describe('Error Handling', () => {
		it('should handle API errors gracefully', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 400,
				statusText: 'Bad Request'
			});

			await expect(platforms.createPlatform({
				name: 'invalid',
				display_name: 'Invalid Platform'
			})).rejects.toThrow('HTTP 400: Bad Request');

			const state = get(platforms);
			expect(state.error).toBe('HTTP 400: Bad Request');
			expect(state.isLoading).toBe(false);
		});

		it('should handle network errors', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await expect(platforms.fetchPlatforms()).rejects.toThrow('Network error');

			const state = get(platforms);
			expect(state.error).toBe('Network error');
		});
	});
});