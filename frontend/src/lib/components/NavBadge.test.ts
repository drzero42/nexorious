import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import NavBadge from './NavBadge.svelte';

// Mock $app/navigation
vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

import { goto } from '$app/navigation';

describe('NavBadge', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe('Visibility', () => {
		it('should not render when count is 0', () => {
			render(NavBadge, { props: { count: 0 } });
			expect(screen.queryByTestId('nav-badge')).not.toBeInTheDocument();
		});

		it('should not render when count is negative', () => {
			render(NavBadge, { props: { count: -1 } });
			expect(screen.queryByTestId('nav-badge')).not.toBeInTheDocument();
		});

		it('should render when count is 1', () => {
			render(NavBadge, { props: { count: 1 } });
			expect(screen.getByTestId('nav-badge')).toBeInTheDocument();
			expect(screen.getByText('1')).toBeInTheDocument();
		});

		it('should render when count is greater than 1', () => {
			render(NavBadge, { props: { count: 42 } });
			expect(screen.getByTestId('nav-badge')).toBeInTheDocument();
			expect(screen.getByText('42')).toBeInTheDocument();
		});
	});

	describe('Display', () => {
		it('should display the count value', () => {
			render(NavBadge, { props: { count: 5 } });
			expect(screen.getByText('5')).toBeInTheDocument();
		});

		it('should display large count values', () => {
			render(NavBadge, { props: { count: 999 } });
			expect(screen.getByText('999')).toBeInTheDocument();
		});
	});

	describe('Rendering as button vs span', () => {
		it('should render as span when no href provided', () => {
			render(NavBadge, { props: { count: 5 } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge.tagName.toLowerCase()).toBe('span');
		});

		it('should render as button when href is provided', () => {
			render(NavBadge, { props: { count: 5, href: '/review' } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge.tagName.toLowerCase()).toBe('button');
		});
	});

	describe('Navigation', () => {
		it('should navigate on click when href is provided', async () => {
			render(NavBadge, { props: { count: 3, href: '/review?source=import' } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.click(badge);

			expect(goto).toHaveBeenCalledWith('/review?source=import');
		});

		it('should not navigate on click when href is not provided', async () => {
			render(NavBadge, { props: { count: 3 } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.click(badge);

			expect(goto).not.toHaveBeenCalled();
		});

		it('should navigate on Enter key press', async () => {
			render(NavBadge, { props: { count: 3, href: '/review?source=sync' } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.keyDown(badge, { key: 'Enter' });

			expect(goto).toHaveBeenCalledWith('/review?source=sync');
		});

		it('should navigate on Space key press', async () => {
			render(NavBadge, { props: { count: 3, href: '/review?source=sync' } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.keyDown(badge, { key: ' ' });

			expect(goto).toHaveBeenCalledWith('/review?source=sync');
		});

		it('should not navigate on other key presses', async () => {
			render(NavBadge, { props: { count: 3, href: '/review' } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.keyDown(badge, { key: 'Tab' });
			await fireEvent.keyDown(badge, { key: 'Escape' });
			await fireEvent.keyDown(badge, { key: 'a' });

			expect(goto).not.toHaveBeenCalled();
		});
	});

	describe('Accessibility', () => {
		it('should have default aria-label', () => {
			render(NavBadge, { props: { count: 5 } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveAttribute('aria-label', '5 pending');
		});

		it('should use custom label when provided', () => {
			render(NavBadge, { props: { count: 3, label: '3 items need review' } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveAttribute('aria-label', '3 items need review');
		});

		it('should have button type attribute when href is provided', () => {
			render(NavBadge, { props: { count: 3, href: '/review' } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveAttribute('type', 'button');
		});
	});

	describe('Styling', () => {
		it('should apply custom class', () => {
			render(NavBadge, { props: { count: 5, class: 'custom-badge-class' } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveClass('custom-badge-class');
		});

		it('should have base styling classes', () => {
			render(NavBadge, { props: { count: 5 } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveClass('inline-flex');
			expect(badge).toHaveClass('items-center');
			expect(badge).toHaveClass('justify-center');
			expect(badge).toHaveClass('rounded-full');
		});

		it('should have primary color classes', () => {
			render(NavBadge, { props: { count: 5 } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveClass('bg-primary-500');
			expect(badge).toHaveClass('text-white');
		});

		it('should have hover classes when clickable', () => {
			render(NavBadge, { props: { count: 5, href: '/review' } });
			const badge = screen.getByTestId('nav-badge');
			expect(badge).toHaveClass('hover:bg-primary-600');
		});
	});

	describe('Edge Cases', () => {
		it('should handle very large numbers', () => {
			render(NavBadge, { props: { count: 99999 } });
			expect(screen.getByText('99999')).toBeInTheDocument();
		});

		it('should handle href with query parameters', async () => {
			render(NavBadge, { props: { count: 5, href: '/review?source=import&status=pending' } });

			const badge = screen.getByTestId('nav-badge');
			await fireEvent.click(badge);

			expect(goto).toHaveBeenCalledWith('/review?source=import&status=pending');
		});

		it('should handle empty string href', async () => {
			render(NavBadge, { props: { count: 5, href: '' } });

			const badge = screen.getByTestId('nav-badge');
			// Should render as span since href is empty string (falsy)
			expect(badge.tagName.toLowerCase()).toBe('span');
		});
	});
});
