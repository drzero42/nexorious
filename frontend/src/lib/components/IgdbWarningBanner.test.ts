import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import IgdbWarningBanner from './IgdbWarningBanner.svelte';

// Mock the app-status store
const mockAppStatusValue = vi.fn();
vi.mock('$lib/stores', () => ({
	appStatus: {
		get value() {
			return mockAppStatusValue();
		}
	}
}));

describe('IgdbWarningBanner', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('should not render when IGDB is configured', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: true,
			hasFetched: true,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('should not render when status has not been fetched yet', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: false,
			hasFetched: false,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('should render warning banner when IGDB is not configured', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: false,
			hasFetched: true,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		const alert = screen.getByRole('alert');
		expect(alert).toBeDefined();
		expect(alert.textContent).toContain('IGDB API Not Configured');
		expect(alert.textContent).toContain('Game search and import features are unavailable');
	});

	it('should include setup guide link', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: false,
			hasFetched: true,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		const link = screen.getByText(/View Setup Guide/);
		expect(link).toBeDefined();
		expect(link.getAttribute('href')).toBe('/docs/igdb-setup.md');
		expect(link.getAttribute('target')).toBe('_blank');
		expect(link.getAttribute('rel')).toContain('noopener');
	});

	it('should have correct accessibility attributes', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: false,
			hasFetched: true,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		const alert = screen.getByRole('alert');
		expect(alert.getAttribute('aria-live')).toBe('polite');
	});

	it('should have amber/warning styling classes', () => {
		mockAppStatusValue.mockReturnValue({
			igdbConfigured: false,
			hasFetched: true,
			isLoading: false,
			error: null
		});

		render(IgdbWarningBanner);

		const alert = screen.getByRole('alert');
		expect(alert.className).toContain('bg-amber-50');
		expect(alert.className).toContain('border-amber-200');
	});
});
