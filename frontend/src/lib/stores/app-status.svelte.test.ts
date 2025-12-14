import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { AppStatusState } from './app-status.svelte';

// Mock fetch globally
global.fetch = vi.fn();

// Mock the config module
vi.mock('$lib/env', () => ({
	config: {
		apiUrl: 'http://localhost:8000/api'
	}
}));

// Mock $app/environment
vi.mock('$app/environment', () => ({
	browser: true
}));

describe('App Status Store', () => {
	let appStatus: any;
	let mockFetch: any;

	beforeEach(async () => {
		// Reset fetch mock
		mockFetch = vi.mocked(fetch);
		mockFetch.mockClear();

		// Clear module cache and re-import
		vi.resetModules();

		// Dynamic import to get fresh store instance
		const module = await import('./app-status.svelte');
		appStatus = module.appStatus;

		// Reset the store state
		appStatus.reset();
	});

	describe('Store Structure', () => {
		it('should have correct structure and methods', () => {
			expect(appStatus).toBeDefined();
			expect(appStatus.value).toBeDefined();
			expect(typeof appStatus.fetchStatus).toBe('function');
			expect(typeof appStatus.reset).toBe('function');
		});

		it('should have correct initial state structure', () => {
			expect(appStatus.value).toMatchObject({
				igdbConfigured: true, // Default assumption
				isLoading: false,
				error: null,
				hasFetched: false
			});
		});

		it('should have correct state types', () => {
			const state = appStatus.value as AppStatusState;
			expect(typeof state.igdbConfigured).toBe('boolean');
			expect(typeof state.isLoading).toBe('boolean');
			expect(state.error === null || typeof state.error === 'string').toBe(true);
			expect(typeof state.hasFetched).toBe('boolean');
		});
	});

	describe('fetchStatus', () => {
		it('should fetch status and update state when IGDB is configured', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: true })
			});

			await appStatus.fetchStatus();

			expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/api/status');
			expect(appStatus.value.igdbConfigured).toBe(true);
			expect(appStatus.value.isLoading).toBe(false);
			expect(appStatus.value.error).toBeNull();
			expect(appStatus.value.hasFetched).toBe(true);
		});

		it('should fetch status and update state when IGDB is not configured', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: false })
			});

			await appStatus.fetchStatus();

			expect(mockFetch).toHaveBeenCalledWith('http://localhost:8000/api/status');
			expect(appStatus.value.igdbConfigured).toBe(false);
			expect(appStatus.value.isLoading).toBe(false);
			expect(appStatus.value.error).toBeNull();
			expect(appStatus.value.hasFetched).toBe(true);
		});

		it('should not fetch again if already fetched', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: true })
			});

			await appStatus.fetchStatus();
			expect(mockFetch).toHaveBeenCalledTimes(1);

			// Try to fetch again
			await appStatus.fetchStatus();
			expect(mockFetch).toHaveBeenCalledTimes(1); // Should not have called again
		});

		it('should handle fetch errors', async () => {
			mockFetch.mockRejectedValueOnce(new Error('Network error'));

			await appStatus.fetchStatus();

			expect(appStatus.value.isLoading).toBe(false);
			expect(appStatus.value.error).toBe('Network error');
			expect(appStatus.value.hasFetched).toBe(true);
		});

		it('should handle non-ok responses', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: false,
				status: 500
			});

			await appStatus.fetchStatus();

			expect(appStatus.value.isLoading).toBe(false);
			expect(appStatus.value.error).toContain('Failed to fetch status');
			expect(appStatus.value.hasFetched).toBe(true);
		});
	});

	describe('reset', () => {
		it('should reset store to initial state', async () => {
			// First, change the state
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: false })
			});
			await appStatus.fetchStatus();

			expect(appStatus.value.hasFetched).toBe(true);
			expect(appStatus.value.igdbConfigured).toBe(false);

			// Reset the store
			appStatus.reset();

			expect(appStatus.value).toMatchObject({
				igdbConfigured: true,
				isLoading: false,
				error: null,
				hasFetched: false
			});
		});

		it('should allow fetching again after reset', async () => {
			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: true })
			});

			await appStatus.fetchStatus();
			expect(mockFetch).toHaveBeenCalledTimes(1);

			appStatus.reset();

			mockFetch.mockResolvedValueOnce({
				ok: true,
				json: () => Promise.resolve({ igdb_configured: false })
			});

			await appStatus.fetchStatus();
			expect(mockFetch).toHaveBeenCalledTimes(2);
			expect(appStatus.value.igdbConfigured).toBe(false);
		});
	});
});
