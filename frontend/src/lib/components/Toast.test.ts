import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import Toast from './Toast.svelte';
import type { NotificationType } from '../stores/notifications.svelte';

// Mock onMount for controlled testing
vi.mock('svelte', async () => {
	const actual = await vi.importActual('svelte');
	return {
		...actual,
		onMount: vi.fn((fn) => fn())
	};
});

describe('Toast Component', () => {
	const defaultProps = {
		id: 'test-toast-1',
		type: 'info' as NotificationType,
		message: 'Test message',
		duration: 5000,
		onRemove: vi.fn()
	};

	beforeEach(() => {
		vi.clearAllMocks();
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('should render with correct message', () => {
		render(Toast, { props: defaultProps });
		
		expect(screen.getByText('Test message')).toBeInTheDocument();
	});

	it('should render with info type styling by default', () => {
		render(Toast, { props: defaultProps });
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('bg-blue-50', 'border-blue-200', 'text-blue-800');
	});

	it('should render with success type styling', () => {
		render(Toast, { 
			props: { 
				...defaultProps, 
				type: 'success' as NotificationType 
			} 
		});
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('bg-green-50', 'border-green-200', 'text-green-800');
	});

	it('should render with error type styling', () => {
		render(Toast, { 
			props: { 
				...defaultProps, 
				type: 'error' as NotificationType 
			} 
		});
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('bg-red-50', 'border-red-200', 'text-red-800');
	});

	it('should render with warning type styling', () => {
		render(Toast, { 
			props: { 
				...defaultProps, 
				type: 'warning' as NotificationType 
			} 
		});
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('bg-yellow-50', 'border-yellow-200', 'text-yellow-800');
	});

	it('should display correct icon for each type', () => {
		const types: NotificationType[] = ['success', 'error', 'warning', 'info'];
		const expectedIcons = ['✓', '✕', '⚠', 'ℹ'];

		types.forEach((type, index) => {
			const { unmount } = render(Toast, { 
				props: { 
					...defaultProps, 
					type,
					id: `test-${type}`
				} 
			});
			
			expect(screen.getByText(expectedIcons[index])).toBeInTheDocument();
			unmount();
		});
	});

	it('should call onRemove when close button is clicked', async () => {
		const onRemove = vi.fn();
		render(Toast, { 
			props: { 
				...defaultProps, 
				onRemove 
			} 
		});
		
		const closeButton = screen.getByLabelText('Close notification');
		await fireEvent.click(closeButton);
		
		// Wait for the animation delay
		vi.advanceTimersByTime(300);
		
		expect(onRemove).toHaveBeenCalledWith('test-toast-1');
	});

	it('should auto-dismiss after duration', async () => {
		const onRemove = vi.fn();
		render(Toast, { 
			props: { 
				...defaultProps, 
				duration: 1000,
				onRemove 
			} 
		});
		
		// Advance past the duration
		vi.advanceTimersByTime(1000);
		
		// Wait for the animation delay
		vi.advanceTimersByTime(300);
		
		expect(onRemove).toHaveBeenCalledWith('test-toast-1');
	});

	it('should not auto-dismiss when duration is 0', async () => {
		const onRemove = vi.fn();
		render(Toast, { 
			props: { 
				...defaultProps, 
				duration: 0,
				onRemove 
			} 
		});
		
		// Advance time significantly
		vi.advanceTimersByTime(10000);
		
		expect(onRemove).not.toHaveBeenCalled();
	});

	it('should have proper accessibility attributes', () => {
		render(Toast, { props: defaultProps });
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveAttribute('aria-live', 'assertive');
		expect(toast).toHaveAttribute('aria-atomic', 'true');
	});

	it('should start with opacity 0 and transition to visible', async () => {
		render(Toast, { props: defaultProps });
		
		const toast = screen.getByRole('alert');
		
		// Initially should have translate-x-full (off-screen)
		expect(toast).toHaveClass('translate-x-full', 'opacity-0');
		
		// After onMount trigger, should become visible
		vi.advanceTimersByTime(10);
		
		// Wait for the reactive update
		await waitFor(() => {
			expect(toast).toHaveClass('translate-x-0', 'opacity-100');
		});
	});

	it('should handle long messages properly', () => {
		const longMessage = 'This is a very long message that should be displayed properly in the toast notification without breaking the layout or causing any visual issues';
		
		render(Toast, { 
			props: { 
				...defaultProps, 
				message: longMessage 
			} 
		});
		
		expect(screen.getByText(longMessage)).toBeInTheDocument();
	});

	it('should have correct z-index for proper layering', () => {
		render(Toast, { props: defaultProps });
		
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('z-50');
	});

	it('should call dismiss when manually triggered', async () => {
		const onRemove = vi.fn();
		render(Toast, { 
			props: { 
				...defaultProps, 
				onRemove 
			} 
		});
		
		const closeButton = screen.getByLabelText('Close notification');
		await fireEvent.click(closeButton);
		
		// Should start exit animation
		const toast = screen.getByRole('alert');
		expect(toast).toHaveClass('translate-x-full', 'opacity-0');
		
		// Complete the animation
		vi.advanceTimersByTime(300);
		
		expect(onRemove).toHaveBeenCalledWith('test-toast-1');
	});

	it('should handle multiple rapid dismiss calls gracefully', async () => {
		const onRemove = vi.fn();
		render(Toast, { 
			props: { 
				...defaultProps, 
				onRemove 
			} 
		});
		
		const closeButton = screen.getByLabelText('Close notification');
		
		// Click multiple times rapidly
		await fireEvent.click(closeButton);
		await fireEvent.click(closeButton);
		await fireEvent.click(closeButton);
		
		// Complete the animation
		vi.advanceTimersByTime(300);
		
		// Should only be called once due to isDismissing guard
		expect(onRemove).toHaveBeenCalledOnce();
		expect(onRemove).toHaveBeenCalledWith('test-toast-1');
	});
});